package handlers

import (
	"strconv"
	"strings"

	"formail/internal/services"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type verifyCaptchaReq struct {
	CaptchaID string `json:"captcha_id"`
	Answer    string `json:"answer"`
}

func fingerprintFrom(c *gin.Context) string {
	fp := strings.TrimSpace(c.GetHeader("X-Device-Fingerprint"))
	if fp != "" {
		return fp
	}
	raw := c.ClientIP() + "|" + c.Request.UserAgent()
	return utils.SHA256Hex(raw)
}

func (h *Handler) GetCaptcha(c *gin.Context) {
	enabled := getSetting(h.DB, "captcha_enabled", "1") == "1"
	if !enabled {
		utils.OK(c, gin.H{"captcha_enabled": false})
		return
	}

	ttl, _ := strconv.Atoi(getSetting(h.DB, "captcha_ttl_seconds", "180"))
	modeStr := getSetting(h.DB, "captcha_mode", "click_text")
	mode := services.CaptchaMode(modeStr)

	if !mode.IsValid() {
		mode = services.CaptchaModeClickText
	}

	cp, err := services.GenerateCaptcha(h.DB, c.ClientIP(), "send_code", ttl, mode)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	utils.OK(c, gin.H{
		"captcha_enabled": true,
		"captcha_id":      cp.ID,
		"captcha_type":    cp.Type,
		"captcha_base64":  cp.Base64Data,
		"thumb_base64":    cp.ThumbBase64,
		"ttl_seconds":     cp.TTL,
		"hint":            cp.Hint,
		"chars":           cp.Chars,
		"thumb_x":         cp.ThumbX,
		"thumb_y":         cp.ThumbY,
		"thumb_width":     cp.ThumbWidth,
		"thumb_height":    cp.ThumbHeight,
		"angle":           cp.Angle,
		"init_x":          cp.InitX,
		"init_y":          cp.InitY,
	})
}

func (h *Handler) VerifyCaptcha(c *gin.Context) {
	var req verifyCaptchaReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	ok, refreshRequired, err := services.VerifyCaptchaWithConsume(h.DB, strings.TrimSpace(req.CaptchaID), strings.TrimSpace(req.Answer), c.ClientIP(), false)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if !ok {
		if refreshRequired {
			utils.Fail(c, 429, "失败次数过多，已为您自动刷新验证码，请重试")
			return
		}
		utils.Fail(c, 400, "captcha invalid")
		return
	}
	utils.OK(c, gin.H{"verified": true})
}
