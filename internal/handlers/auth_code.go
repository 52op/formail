package handlers

import (
	"database/sql"
	"fmt"
	"strings"

	"formail/internal/services"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type sendCodeReq struct {
	Email      string `json:"email"`
	Purpose    string `json:"purpose"`
	CaptchaID  string `json:"captcha_id"`
	CaptchaAns string `json:"captcha_answer"`
}

type loginCodeReq struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func (h *Handler) SendEmailCode(c *gin.Context) {
	var req sendCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Purpose = strings.TrimSpace(strings.ToLower(req.Purpose))
	req.CaptchaID = strings.TrimSpace(req.CaptchaID)
	req.CaptchaAns = strings.TrimSpace(req.CaptchaAns)
	if !isValidEmail(req.Email) {
		utils.Fail(c, 400, "邮箱格式无效")
		return
	}
	if req.Purpose != "register" && req.Purpose != "login" {
		utils.Fail(c, 400, "用途必须是 register 或 login")
		return
	}

	if getSettingBool(h.DB, "captcha_enabled", true) {
		if req.CaptchaID == "" || req.CaptchaAns == "" {
			utils.Fail(c, 400, "请完成图形验证码")
			return
		}
		ok, refreshRequired, err := services.VerifyCaptcha(h.DB, req.CaptchaID, req.CaptchaAns, c.ClientIP())
		if err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		if !ok {
			if refreshRequired {
				utils.Fail(c, 429, "失败次数过多，已为您自动刷新验证码，请重试")
				return
			}
			utils.Fail(c, 400, "验证码不正确")
			return
		}
	}

	cooldownSec := getSettingInt(h.DB, "email_code_cooldown_seconds", 60)
	if cooldownSec <= 0 {
		cooldownSec = 60
	}

	if req.Purpose == "register" {
		if getSetting(h.DB, "allow_register", "0") != "1" {
			utils.Fail(c, 403, "注册已关闭")
			return
		}
		var cnt int
		if err := h.DB.QueryRow(`SELECT COUNT(1) FROM users WHERE username=?`, req.Email).Scan(&cnt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		if cnt > 0 {
			utils.Fail(c, 400, "邮箱已被注册")
			return
		}
	} else {
		var status int
		err := h.DB.QueryRow(`SELECT status FROM users WHERE username=?`, req.Email).Scan(&status)
		if err == sql.ErrNoRows {
			utils.OK(c, gin.H{"sent": true, "cooldown_seconds": cooldownSec})
			return
		}
		if err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		if status != 1 {
			utils.Fail(c, 403, "账号已禁用")
			return
		}
	}

	fp := fingerprintFrom(c)
	fpMax := getSettingInt(h.DB, "fingerprint_max_requests_10m", 30)
	if fpMax <= 0 {
		fpMax = 30
	}
	var fpCount int
	if err := h.DB.QueryRow(`SELECT COUNT(1) FROM code_request_logs WHERE fingerprint=? AND created_at >= datetime('now','-10 minutes')`, fp).Scan(&fpCount); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if fpCount >= fpMax {
		utils.Fail(c, 429, "device request limit reached, try later")
		return
	}

	var recentCount int
	if err := h.DB.QueryRow(`SELECT COUNT(1) FROM verification_codes WHERE email=? AND purpose=? AND created_at >= datetime('now', ?)`, req.Email, req.Purpose, fmt.Sprintf("-%d seconds", cooldownSec)).Scan(&recentCount); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if recentCount > 0 {
		utils.Fail(c, 429, fmt.Sprintf("please wait %d seconds before requesting again", cooldownSec))
		return
	}

	tooMany, err := services.TooManyRecentCodes(h.DB, req.Email, c.ClientIP())
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if tooMany {
		utils.Fail(c, 429, "too many requests, try later")
		return
	}

	code, err := services.GenerateNumericCode(6)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if err := services.CreateVerificationCode(h.DB, req.Email, req.Purpose, code, c.ClientIP(), 10); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	_, _ = h.DB.Exec(`INSERT INTO code_request_logs(email,ip,fingerprint,purpose) VALUES(?,?,?,?)`, req.Email, c.ClientIP(), fp, req.Purpose)
	subTpl := getSetting(h.DB, "email_code_subject_template", "Formail 验证码")
	bodyTpl := getSetting(h.DB, "email_code_body_template", "你的验证码是: {{code}}\\n用途: {{purpose}}\\n{{ttl_minutes}}分钟内有效。若非本人操作请忽略。")
	signature := getSetting(h.DB, "email_signature", "--\\nFormail Team")
	sub := strings.ReplaceAll(subTpl, "{{purpose}}", req.Purpose)
	body := bodyTpl
	body = strings.ReplaceAll(body, "{{code}}", code)
	body = strings.ReplaceAll(body, "{{purpose}}", req.Purpose)
	body = strings.ReplaceAll(body, "{{ttl_minutes}}", "10")
	body = strings.ReplaceAll(body, "\\n", "\n")
	if strings.TrimSpace(signature) != "" {
		body += "\n\n" + strings.ReplaceAll(signature, "\\n", "\n")
	}
	if err := h.enqueueServiceMail(req.Email, sub, body, "email_code:"+req.Purpose); err != nil {
		utils.Fail(c, 500, "enqueue code mail failed: "+err.Error())
		return
	}
	utils.OK(c, gin.H{"sent": true, "cooldown_seconds": cooldownSec})
}

func (h *Handler) LoginByEmailCode(c *gin.Context) {
	var req loginCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Code = strings.TrimSpace(req.Code)
	if !isValidEmail(req.Email) || req.Code == "" {
		utils.Fail(c, 400, "请输入邮箱和验证码")
		return
	}
	ok, err := services.VerifyCodeOnce(h.DB, req.Email, "login", req.Code, 5)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if !ok {
		utils.Fail(c, 400, "验证码无效或已过期")
		return
	}

	var id int64
	var role string
	var status int
	if err := h.DB.QueryRow(`SELECT id,role,status FROM users WHERE username=?`, req.Email).Scan(&id, &role, &status); err == sql.ErrNoRows {
		utils.Fail(c, 404, "用户不存在")
		return
	} else if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if status != 1 {
		utils.Fail(c, 403, "账号已禁用")
		return
	}
	token, err := utils.GenerateToken(h.Cfg.Security.JWTSecret, id, req.Email, role)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"token": token, "username": req.Email, "role": role})
}
