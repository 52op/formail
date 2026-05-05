package handlers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"formail/internal/services"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type registerReq struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
}

type registrationSettingsReq struct {
	AllowRegister          bool   `json:"allow_register"`
	RegisterDefaultStatus  int    `json:"register_default_status"`
	AdminNotifyEmail       string `json:"admin_notify_email"`
	EmailCodeSubjectTpl    string `json:"email_code_subject_template"`
	EmailCodeBodyTpl       string `json:"email_code_body_template"`
	EmailSignature         string `json:"email_signature"`
	EmailCodeCooldownSec   int    `json:"email_code_cooldown_seconds"`
	CaptchaEnabled         bool   `json:"captcha_enabled"`
	CaptchaMode            string `json:"captcha_mode"`
	CaptchaTTLSeconds      int    `json:"captcha_ttl_seconds"`
	CaptchaFailureLimit    int    `json:"captcha_failure_limit"`
	FingerprintMaxReq10Min int    `json:"fingerprint_max_requests_10m"`
	ServiceMailChannelID   int64  `json:"service_mail_channel_id"`
}

func (h *Handler) Register(c *gin.Context) {
	log.Printf("register start ip=%s", c.ClientIP())
	if !getSettingBool(h.DB, "allow_register", false) {
		utils.Fail(c, 403, "registration is disabled")
		return
	}

	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "请求格式无效")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if !isValidEmail(email) {
		utils.Fail(c, 400, "请输入有效的邮箱地址")
		return
	}
	if len(req.Password) < 6 {
		utils.Fail(c, 400, "密码长度至少6位")
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		utils.Fail(c, 400, "请输入验证码")
		return
	}

	log.Printf("register verify code email=%s", email)
	ok, err := services.VerifyCodeOnce(h.DB, email, "register", req.Code, 5)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if !ok {
		utils.Fail(c, 400, "验证码无效或已过期")
		return
	}

	log.Printf("register hash password email=%s", email)
	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	status := 1
	if getSettingInt(h.DB, "register_default_status", 1) == 0 {
		status = 0
	}
	log.Printf("register insert user email=%s status=%d", email, status)
	res, err := h.DB.Exec(`INSERT INTO users(username,password_hash,role,display_name,email,status) VALUES(?,?, 'user',?,?,?)`,
		email, hash, strings.TrimSpace(req.DisplayName), email, status)
	if err != nil {
		utils.Fail(c, 400, "注册失败: "+err.Error())
		return
	}
	uid, _ := res.LastInsertId()

	if status == 0 {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		baseURL := scheme + "://" + c.Request.Host
		h.notifyAdminForReview(uid, email, baseURL)
	}
	log.Printf("register success email=%s status=%d", email, status)
	utils.OK(c, gin.H{"registered": true, "status": status})
}

func (h *Handler) notifyAdminForReview(userID int64, newUserEmail, baseURL string) {
	adminEmail := h.resolveAdminNotifyEmail()
	if !isValidEmail(adminEmail) {
		return
	}
	token, err := utils.GenerateReviewToken(h.Cfg.Security.JWTSecret, userID, "register_approve", 48*time.Hour)
	if err != nil {
		return
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:8080"
	}
	approveLink := fmt.Sprintf("%s/api/auth/approve?token=%s", strings.TrimRight(baseURL, "/"), token)
	rejectLink := fmt.Sprintf("%s/api/auth/reject?token=%s", strings.TrimRight(baseURL, "/"), token)
	sub := "Formail 新用户待审核"
	body := "新用户注册待审核: " + newUserEmail + "\n审核通过: " + approveLink + "\n审核拒绝: " + rejectLink
	_ = h.enqueueServiceMail(adminEmail, sub, body, "register_review")
}

func (h *Handler) ApproveRegistration(c *gin.Context) {
	t := strings.TrimSpace(c.Query("token"))
	if t == "" {
		htmlResult(c, "审核失败", "缺少 token 参数", false)
		return
	}
	claims, err := utils.ParseReviewToken(h.Cfg.Security.JWTSecret, t)
	if err != nil || claims.Purpose != "register_approve" {
		htmlResult(c, "审核失败", "无效或过期的审核链接", false)
		return
	}
	used, err := h.markReviewTokenUsed(claims.TokenID)
	if err != nil {
		htmlResult(c, "审核失败", err.Error(), false)
		return
	}
	if used {
		htmlResult(c, "审核失败", "该审核链接已使用", false)
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET status=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, claims.UserID); err != nil {
		htmlResult(c, "审核失败", err.Error(), false)
		return
	}
	var userEmail string
	_ = h.DB.QueryRow(`SELECT email FROM users WHERE id=?`, claims.UserID).Scan(&userEmail)
	if isValidEmail(userEmail) {
		_ = h.enqueueServiceMail(userEmail, "Formail 注册审核已通过", "您的注册申请已通过审核，现在可以正常登录了。", "register_approved")
	}
	htmlResult(c, "审核通过", "用户已启用，可以正常登录。", true)
}

func (h *Handler) RejectRegistration(c *gin.Context) {
	t := strings.TrimSpace(c.Query("token"))
	if t == "" {
		htmlResult(c, "拒绝失败", "缺少 token 参数", false)
		return
	}
	claims, err := utils.ParseReviewToken(h.Cfg.Security.JWTSecret, t)
	if err != nil || claims.Purpose != "register_approve" {
		htmlResult(c, "拒绝失败", "无效或过期的审核链接", false)
		return
	}
	used, err := h.markReviewTokenUsed(claims.TokenID)
	if err != nil {
		htmlResult(c, "拒绝失败", err.Error(), false)
		return
	}
	if used {
		htmlResult(c, "拒绝失败", "该审核链接已使用", false)
		return
	}
	var userEmail string
	_ = h.DB.QueryRow(`SELECT email FROM users WHERE id=? AND status=0`, claims.UserID).Scan(&userEmail)
	if _, err := h.DB.Exec(`DELETE FROM users WHERE id=? AND status=0`, claims.UserID); err != nil {
		htmlResult(c, "拒绝失败", err.Error(), false)
		return
	}
	if isValidEmail(userEmail) {
		_ = h.enqueueServiceMail(userEmail, "Formail 注册审核未通过", "很抱歉，您的注册申请未能通过审核。", "register_rejected")
	}
	htmlResult(c, "已拒绝申请", "待审核用户已删除。", true)
}

func (h *Handler) markReviewTokenUsed(tokenID string) (bool, error) {
	if strings.TrimSpace(tokenID) == "" {
		return false, fmt.Errorf("empty token id")
	}
	res, err := h.DB.Exec(`INSERT OR IGNORE INTO review_token_uses(token_id_hash) VALUES(?)`, tokenID)
	if err != nil {
		return false, err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return aff == 0, nil
}

func htmlResult(c *gin.Context, title, msg string, success bool) {
	color := "#dc2626"
	if success {
		color = "#16a34a"
	}
	html := "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"UTF-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>" + title + "</title><style>body{font-family:Arial,Helvetica,sans-serif;background:#f5f7fb;padding:24px} .card{max-width:680px;margin:0 auto;background:#fff;border-radius:12px;padding:20px;box-shadow:0 8px 20px rgba(0,0,0,.06)} h1{margin-top:0;color:" + color + "} p{line-height:1.6}</style></head><body><div class=\"card\"><h1>" + title + "</h1><p>" + msg + "</p></div></body></html>"
	c.Data(200, "text/html; charset=utf-8", []byte(html))
}

func (h *Handler) GetRegistrationSettings(c *gin.Context) {
	allow := getSettingBool(h.DB, "allow_register", false)
	defStatus := getSettingInt(h.DB, "register_default_status", 1)
	adminNotifyEmail := getSetting(h.DB, "admin_notify_email", "")
	subTpl := getSetting(h.DB, "email_code_subject_template", "Formail 验证码")
	bodyTpl := getSetting(h.DB, "email_code_body_template", "你的验证码是: {{code}}\n用途: {{purpose}}\n{{ttl_minutes}}分钟内有效。若非本人操作请忽略。")
	signature := getSetting(h.DB, "email_signature", "--\nFormail Team")
	cooldown := getSettingInt(h.DB, "email_code_cooldown_seconds", 60)
	captchaEnabled := getSettingBool(h.DB, "captcha_enabled", true)
	captchaMode := getSetting(h.DB, "captcha_mode", "click_text")
	captchaTTL := getSettingInt(h.DB, "captcha_ttl_seconds", 180)
	captchaFailureLimit := getSettingInt(h.DB, "captcha_failure_limit", 5)
	fpMax := getSettingInt(h.DB, "fingerprint_max_requests_10m", 30)
	serviceMailChannelID := getSettingInt(h.DB, "service_mail_channel_id", 0)
	utils.OK(c, gin.H{
		"allow_register":               allow,
		"register_default_status":      defStatus,
		"admin_notify_email":           adminNotifyEmail,
		"email_code_subject_template":  subTpl,
		"email_code_body_template":     bodyTpl,
		"email_signature":              signature,
		"email_code_cooldown_seconds":  cooldown,
		"captcha_enabled":              captchaEnabled,
		"captcha_mode":                 captchaMode,
		"captcha_ttl_seconds":          captchaTTL,
		"captcha_failure_limit":        captchaFailureLimit,
		"fingerprint_max_requests_10m": fpMax,
		"service_mail_channel_id":      serviceMailChannelID,
	})
}

func (h *Handler) UpdateRegistrationSettings(c *gin.Context) {
	var req registrationSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	if req.RegisterDefaultStatus != 0 && req.RegisterDefaultStatus != 1 {
		utils.Fail(c, 400, "register_default_status must be 0 or 1")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.AdminNotifyEmail))
	if email != "" && !isValidEmail(email) {
		utils.Fail(c, 400, "admin_notify_email invalid")
		return
	}
	allow := "0"
	if req.AllowRegister {
		allow = "1"
	}
	if req.EmailCodeCooldownSec <= 0 {
		req.EmailCodeCooldownSec = 60
	}
	if req.CaptchaTTLSeconds <= 0 {
		req.CaptchaTTLSeconds = 180
	}
	if req.CaptchaFailureLimit <= 0 {
		req.CaptchaFailureLimit = 5
	}
	if req.FingerprintMaxReq10Min <= 0 {
		req.FingerprintMaxReq10Min = 30
	}
	if strings.TrimSpace(req.EmailCodeSubjectTpl) == "" {
		req.EmailCodeSubjectTpl = "Formail 验证码"
	}
	if strings.TrimSpace(req.EmailCodeBodyTpl) == "" {
		req.EmailCodeBodyTpl = "你的验证码是: {{code}}\n用途: {{purpose}}\n{{ttl_minutes}}分钟内有效。若非本人操作请忽略。"
	}
	if strings.TrimSpace(req.EmailSignature) == "" {
		req.EmailSignature = "--\nFormail Team"
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='allow_register'`, allow); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='register_default_status'`, fmt.Sprintf("%d", req.RegisterDefaultStatus)); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='admin_notify_email'`, email); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='email_code_subject_template'`, strings.TrimSpace(req.EmailCodeSubjectTpl)); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='email_code_body_template'`, req.EmailCodeBodyTpl); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='email_signature'`, req.EmailSignature); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='email_code_cooldown_seconds'`, fmt.Sprintf("%d", req.EmailCodeCooldownSec)); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	captchaEnabledVal := "0"
	if req.CaptchaEnabled {
		captchaEnabledVal = "1"
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='captcha_enabled'`, captchaEnabledVal); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	captchaModeVal := strings.TrimSpace(req.CaptchaMode)
	if captchaModeVal == "" {
		captchaModeVal = "click_text"
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='captcha_mode'`, captchaModeVal); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='captcha_ttl_seconds'`, fmt.Sprintf("%d", req.CaptchaTTLSeconds)); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='captcha_failure_limit'`, fmt.Sprintf("%d", req.CaptchaFailureLimit)); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='fingerprint_max_requests_10m'`, fmt.Sprintf("%d", req.FingerprintMaxReq10Min)); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if req.ServiceMailChannelID < 0 {
		req.ServiceMailChannelID = 0
	}
	if req.ServiceMailChannelID > 0 {
		ch, err := h.getChannelByID(req.ServiceMailChannelID)
		adminID, adminErr := h.primaryAdminUserID()
		if err != nil || adminErr != nil || !ch.Enabled || ch.OwnerUserID != adminID {
			utils.Fail(c, 400, "service_mail_channel_id invalid or not an enabled admin channel")
			return
		}
	}
	if _, err := h.DB.Exec(`UPDATE settings SET value=?, updated_at=CURRENT_TIMESTAMP WHERE key='service_mail_channel_id'`, fmt.Sprintf("%d", req.ServiceMailChannelID)); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{
		"allow_register":               req.AllowRegister,
		"register_default_status":      req.RegisterDefaultStatus,
		"admin_notify_email":           email,
		"email_code_subject_template":  strings.TrimSpace(req.EmailCodeSubjectTpl),
		"email_code_body_template":     req.EmailCodeBodyTpl,
		"email_signature":              req.EmailSignature,
		"email_code_cooldown_seconds":  req.EmailCodeCooldownSec,
		"captcha_enabled":              req.CaptchaEnabled,
		"captcha_mode":                 captchaModeVal,
		"captcha_ttl_seconds":          req.CaptchaTTLSeconds,
		"captcha_failure_limit":        req.CaptchaFailureLimit,
		"fingerprint_max_requests_10m": req.FingerprintMaxReq10Min,
		"service_mail_channel_id":      req.ServiceMailChannelID,
	})
}
