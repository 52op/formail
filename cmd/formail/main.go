package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"formail/internal/config"
	"formail/internal/db"
	"formail/internal/handlers"
	"formail/internal/middleware"
	"formail/internal/utils"

	"github.com/caddyserver/certmagic"
	"github.com/gin-gonic/gin"
)

func main() {
	adminUser := flag.String("admin", "", "修改管理员用户名")
	adminPwd := flag.String("adminpwd", "", "修改管理员密码")
	flag.Parse()

	cfg, err := config.Load("config.toml")
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("open database failed: %v", err)
	}
	defer database.Close()

	// CLI 模式：仅修改管理员账号/密码，完成后退出
	if *adminUser != "" || *adminPwd != "" {
		if err := db.UpdateAdminAccount(database, *adminUser, *adminPwd); err != nil {
			log.Fatalf("修改管理员失败: %v", err)
		}
		fmt.Println("管理员账号已更新")
		return
	}

	logFile, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		gin.DefaultWriter = logFile
		gin.DefaultErrorWriter = logFile
	}

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
	serveStatic(r)
	r.Static("/uploads", "./data/uploads")

	r.GET("/", func(c *gin.Context) { servePage(c, "/landing.html") })
	r.GET("/docs", func(c *gin.Context) { servePage(c, "/docs.html") })
	r.GET("/login", func(c *gin.Context) { servePage(c, "/login.html") })
	r.GET("/dashboard", func(c *gin.Context) { servePage(c, "/forms.html") })
	r.GET("/dashboard/forms", func(c *gin.Context) { servePage(c, "/forms.html") })
	r.GET("/dashboard/channels", func(c *gin.Context) { servePage(c, "/channels.html") })
	r.GET("/dashboard/submissions", func(c *gin.Context) { servePage(c, "/submissions.html") })
	r.GET("/dashboard/users", func(c *gin.Context) { servePage(c, "/users.html") })
	r.GET("/dashboard/profile", func(c *gin.Context) { servePage(c, "/profile.html") })
	r.GET("/dashboard/settings", func(c *gin.Context) { servePage(c, "/settings.html") })
	r.GET("/dashboard/stats", func(c *gin.Context) { servePage(c, "/stats.html") })
	r.GET("/dashboard/queue", func(c *gin.Context) { servePage(c, "/queue.html") })

	r.GET("/api/app-config", func(c *gin.Context) {
		utils.OK(c, gin.H{
			"auth_mode": cfg.Security.AuthMode,
			"sso_url":   cfg.Security.SSOIssuer,
		})
	})

	r.POST("/api/auth/login", h.Login)
	r.POST("/api/auth/login-code", h.LoginByEmailCode)
	r.GET("/api/auth/captcha", h.GetCaptcha)
	r.POST("/api/auth/captcha", h.GetCaptcha)
	r.POST("/api/auth/send-code", h.SendEmailCode)
	r.POST("/api/auth/verify-captcha", h.VerifyCaptcha)
	r.POST("/api/auth/register", h.Register)
	r.GET("/api/auth/approve", h.ApproveRegistration)
	r.GET("/api/auth/reject", h.RejectRegistration)
	r.GET("/api/auth/reg-status", h.RegStatus)

	// 根据 auth_mode 选择认证中间件
	var authMiddleware gin.HandlerFunc
	if cfg.Security.AuthMode == "sso" {
		pub, err := middleware.ParseRSAPublicKey(cfg.Security.SSOPublicKey)
		if err != nil {
			log.Fatalf("解析 SSO 公钥失败: %v", err)
		}
		cookieName := cfg.Security.SSOCookieName
		if cookieName == "" {
			cookieName = "_goauth_token"
		}
		authMiddleware = middleware.RequireAuthSSO(pub, cookieName, cfg.Security.SSOIssuer)
		fmt.Println("✅ SSO 模式已启用，认证由 GoAuth 负责")
	} else {
		authMiddleware = middleware.RequireAuth(cfg.Security.JWTSecret)
	}

	auth := r.Group("/api")
	auth.Use(authMiddleware)
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

		auth.GET("/apikeys", h.ListAPIKeys)
		auth.POST("/apikeys", h.CreateAPIKey)
		auth.PUT("/apikeys/:id", h.UpdateAPIKey)
		auth.DELETE("/apikeys/:id", h.DeleteAPIKey)

		auth.GET("/api-logs", h.ListAPILogs)
		auth.GET("/stats/api", h.StatsAPIOverview)
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

	v1 := r.Group("/v1")
	v1.Use(middleware.RequireAPIKey(database))
	{
		v1.POST("/emails", h.SendEmailAPI)
	}

	r.GET("/dashboard/apikeys", func(c *gin.Context) { servePage(c, "/apikeys.html") })

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
