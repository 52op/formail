package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"formail/internal/config"
	"formail/internal/db"
	"formail/internal/handlers"
	"formail/internal/middleware"

	"github.com/caddyserver/certmagic"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load("config.toml")
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	logFile, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		gin.DefaultWriter = logFile
		gin.DefaultErrorWriter = logFile
	}

	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("open database failed: %v", err)
	}
	defer database.Close()

	if err := db.EnsureDefaultAdmin(database, cfg.Admin.DefaultUsername, cfg.Admin.DefaultPassword); err != nil {
		log.Fatalf("seed admin failed: %v", err)
	}

	h := handlers.New(database, cfg)

	mailStopCh := make(chan struct{})
	defer close(mailStopCh)
	if h.MailQueue != nil {
		h.MailQueue.Sender = h.SendServiceMail
		h.MailQueue.Start(2, 800*time.Millisecond, mailStopCh)
	}

	r := gin.New()
	r.Use(middleware.ReverseProxyGuard(cfg.DirectAccess), gin.Logger(), gin.Recovery(), middleware.CORS())
	r.Static("/assets", "./web/static")
	r.GET("/", func(c *gin.Context) { c.File("./web/static/pages/landing.html") })
	r.GET("/docs", func(c *gin.Context) { c.File("./web/static/pages/docs.html") })
	r.GET("/login", func(c *gin.Context) { c.File("./web/static/pages/login.html") })
	r.GET("/dashboard", func(c *gin.Context) { c.File("./web/static/pages/forms.html") })
	r.GET("/dashboard/forms", func(c *gin.Context) { c.File("./web/static/pages/forms.html") })
	r.GET("/dashboard/channels", func(c *gin.Context) { c.File("./web/static/pages/channels.html") })
	r.GET("/dashboard/submissions", func(c *gin.Context) { c.File("./web/static/pages/submissions.html") })
	r.GET("/dashboard/users", func(c *gin.Context) { c.File("./web/static/pages/users.html") })
	r.GET("/dashboard/profile", func(c *gin.Context) { c.File("./web/static/pages/profile.html") })
	r.GET("/dashboard/settings", func(c *gin.Context) { c.File("./web/static/pages/settings.html") })
	r.GET("/dashboard/stats", func(c *gin.Context) { c.File("./web/static/pages/stats.html") })
	r.GET("/dashboard/queue", func(c *gin.Context) { c.File("./web/static/pages/queue.html") })

	r.POST("/api/auth/login", h.Login)
	r.POST("/api/auth/login-code", h.LoginByEmailCode)
	r.GET("/api/auth/captcha", h.GetCaptcha)
	r.POST("/api/auth/captcha", h.GetCaptcha)
	r.POST("/api/auth/send-code", h.SendEmailCode)
	r.POST("/api/auth/verify-captcha", h.VerifyCaptcha)
	r.POST("/api/auth/register", h.Register)
	r.GET("/api/auth/approve", h.ApproveRegistration)
	r.GET("/api/auth/reject", h.RejectRegistration)

	auth := r.Group("/api")
	auth.Use(middleware.RequireAuth(cfg.Security.JWTSecret))
	{
		auth.GET("/auth/me", h.Me)
		auth.POST("/auth/change-password", h.ChangePassword)

		auth.GET("/forms", h.ListForms)
		auth.GET("/forms/:id", h.GetForm)
		auth.POST("/forms", h.CreateForm)
		auth.PUT("/forms/:id", h.UpdateForm)
		auth.DELETE("/forms/:id", h.DeleteForm)

		auth.GET("/channels", h.ListChannels)
		auth.GET("/channels/bindable", h.ListBindableChannels)
		auth.POST("/channels", h.CreateChannel)
		auth.PUT("/channels/:id", h.UpdateChannel)
		auth.DELETE("/channels/:id", h.DeleteChannel)
		auth.POST("/channels/:id/test", h.TestChannel)

		auth.GET("/submissions", h.ListSubmissions)
		auth.DELETE("/submissions/:id", h.DeleteSubmission)
		auth.POST("/submissions/batch-delete", h.BatchDeleteSubmissions)
		auth.GET("/submissions/export", h.ExportSubmissionsCSV)
		auth.POST("/forms/:id/resend-verify", h.ResendFormVerifyEmail)

		auth.GET("/stats/overview", h.StatsOverview)
		auth.GET("/stats/daily", h.StatsDaily)
		auth.GET("/stats/forms", h.StatsForms)
		auth.GET("/stats/channels", h.StatsChannels)
	}

	admin := auth.Group("")
	admin.Use(middleware.RequireAdmin())
	{
		admin.GET("/users", h.ListUsers)
		admin.POST("/users", h.CreateUser)
		admin.PUT("/users/:id", h.UpdateUser)
		admin.DELETE("/users/:id", h.DeleteUser)
		admin.POST("/users/:id/approve", h.ApproveUser)
		admin.POST("/users/:id/reject", h.RejectUser)
		admin.POST("/users/:id/reset-password", h.ResetUserPassword)
		admin.GET("/settings/registration", h.GetRegistrationSettings)
		admin.PUT("/settings/registration", h.UpdateRegistrationSettings)

		admin.GET("/mail-queue", h.ListMailQueue)
		admin.POST("/mail-queue/:id/retry", h.RetryMailJob)
		admin.DELETE("/mail-queue/:id", h.DeleteMailJob)
		
		r.GET("/api/site-settings", h.GetSiteSettings)
		admin.PUT("/site-settings", h.UpdateSiteSettings)
		admin.POST("/site-settings/upload", h.UploadSiteAsset)
	}

	rl := middleware.NewRateLimiter(cfg.Spam.RateLimitPerMinute)
	r.POST("/f/:token", rl.Middleware(), h.SubmitForm)
	r.GET("/verify/:token", h.VerifyEmail)

	if cfg.Server.AutoTLS {
		startAutoTLS(r, cfg)
	} else {
		fmt.Printf("Formail running on %s\n", cfg.Server.Address)
		if err := r.Run(cfg.Server.Address); err != nil {
			log.Fatalf("server start failed: %v", err)
		}
	}
}

func startAutoTLS(r *gin.Engine, cfg config.Config) {
	email := cfg.Server.ACMEmail
	if email == "" {
		log.Fatal("auto_tls enabled but acme_email is empty")
	}

	certDataDir := cfg.Server.CertDataDir
	if certDataDir == "" {
		certDataDir = "./certmagic_data"
	}
	httpsPort := cfg.Server.HTTPSPort
	if httpsPort == "" {
		httpsPort = "443"
	}
	httpPort := cfg.Server.HTTPPort
	if httpPort == "" {
		httpPort = "80"
	}

	certmagic.Default.Storage = &certmagic.FileStorage{Path: certDataDir}

	certmagic.DefaultACME.Email = email
	certmagic.DefaultACME.Agreed = true

	cm := certmagic.NewDefault()

	cm.OnDemand = &certmagic.OnDemandConfig{}

	tlsCfg := cm.TLSConfig()
	tlsCfg.NextProtos = []string{"h2", "http/1.1"}

	acmeIssuer, ok := cm.Issuers[0].(*certmagic.ACMEIssuer)
	if !ok {
		log.Fatal("unexpected issuer type")
	}

	httpSrv := &http.Server{
		Addr:              ":" + httpPort,
		Handler:           acmeIssuer.HTTPChallengeHandler(http.HandlerFunc(httpRedirectHandler)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       5 * time.Second,
	}
	go func() {
		log.Printf("HTTP challenge & redirect server on :%s", httpPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	httpsSrv := &http.Server{
		Addr:              ":" + httpsPort,
		Handler:           r,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       5 * time.Minute,
	}

	fmt.Printf("Formail AutoTLS running — HTTPS on :%s, HTTP on :%s\n", httpsPort, httpPort)
	if err := httpsSrv.ListenAndServeTLS("", ""); err != nil {
		log.Fatalf("HTTPS server failed: %v", err)
	}
}

func httpRedirectHandler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	toURL := "https://" + host + r.URL.RequestURI()
	http.Redirect(w, r, toURL, http.StatusMovedPermanently)
}
