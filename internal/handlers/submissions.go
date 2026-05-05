package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"formail/internal/db"
	"formail/internal/services"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

func parseSubmissionData(c *gin.Context) (map[string]string, error) {
	result := map[string]string{}
	ct := c.ContentType()
	switch {
	case strings.Contains(ct, "application/json"):
		var m map[string]interface{}
		if err := c.ShouldBindJSON(&m); err != nil {
			return nil, err
		}
		for k, v := range m {
			result[k] = strings.TrimSpace(toString(v))
		}
	case strings.Contains(ct, "application/x-www-form-urlencoded") || strings.Contains(ct, "multipart/form-data"):
		if strings.Contains(ct, "multipart/form-data") {
			if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
				return nil, err
			}
		} else {
			if err := c.Request.ParseForm(); err != nil {
				return nil, err
			}
		}
		for k, vals := range c.Request.Form {
			if len(vals) > 0 {
				result[k] = strings.TrimSpace(vals[0])
			}
		}
	default:
		if err := c.Request.ParseForm(); err == nil {
			for k, vals := range c.Request.Form {
				if len(vals) > 0 {
					result[k] = strings.TrimSpace(vals[0])
				}
			}
		}
	}
	return result, nil
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func isSafeRedirectURL(url string) bool {
	if url == "" {
		return true
	}
	parsed, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false
	}
	scheme := parsed.URL.Scheme
	host := parsed.URL.Host
	if scheme != "https" && scheme != "http" {
		return false
	}
	localhostHosts := []string{"localhost", "127.0.0.1", "0.0.0.0"}
	isLocalhost := false
	for _, h := range localhostHosts {
		if strings.HasPrefix(host, h) {
			isLocalhost = true
			break
		}
	}
	if isLocalhost && scheme == "http" {
		return true
	}
	if scheme != "https" {
		return false
	}
	return true
}

func getThemeStyles(theme string) (bgGradient, iconGradient, buttonGradient string) {
	switch theme {
	case "green":
		return "linear-gradient(135deg, #ECFDF5 0%, #D1FAE5 100%)",
			"linear-gradient(135deg, #10B981 0%, #059669 100%)",
			"linear-gradient(135deg, #10B981 0%, #059669 100%)"
	case "purple":
		return "linear-gradient(135deg, #FAF5FF 0%, #F3E8FF 100%)",
			"linear-gradient(135deg, #8B5CF6 0%, #7C3AED 100%)",
			"linear-gradient(135deg, #8B5CF6 0%, #7C3AED 100%)"
	case "orange":
		return "linear-gradient(135deg, #FFF7ED 0%, #FFEDD5 100%)",
			"linear-gradient(135deg, #F97316 0%, #EA580C 100%)",
			"linear-gradient(135deg, #F97316 0%, #EA580C 100%)"
	case "dark":
		return "linear-gradient(135deg, #1F2937 0%, #111827 100%)",
			"linear-gradient(135deg, #60A5FA 0%, #3B82F6 100%)",
			"linear-gradient(135deg, #3B82F6 0%, #2563EB 100%)"
	default:
		return "linear-gradient(135deg, #EEF2FF 0%, #F5F7FB 100%)",
			"linear-gradient(135deg, #3B82F6 0%, #2563EB 100%)",
			"linear-gradient(135deg, #3B82F6 0%, #2563EB 100%)"
	}
}

var submitRateLimit = make(map[string]int64)
var rateLimitMu sync.Mutex

func init() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rateLimitMu.Lock()
			now := time.Now().Unix()
			for ip, t := range submitRateLimit {
				if now-t > 300 {
					delete(submitRateLimit, ip)
				}
			}
			rateLimitMu.Unlock()
		}
	}()
}

func checkRateLimit(ip string) bool {
	rateLimitMu.Lock()
	defer rateLimitMu.Unlock()
	now := time.Now().Unix()
	if lastTime, exists := submitRateLimit[ip]; exists {
		if now-lastTime < 5 {
			return false
		}
	}
	submitRateLimit[ip] = now
	return true
}

func isOriginAllowed(origin, allowedOrigins string) bool {
	if allowedOrigins == "" {
		return true
	}
	allowed := strings.Split(allowedOrigins, ",")
	for _, allowedOrigin := range allowed {
		allowedOrigin = strings.TrimSpace(allowedOrigin)
		if allowedOrigin == "*" {
			return true
		}
		if strings.EqualFold(allowedOrigin, origin) {
			return true
		}
		if strings.HasSuffix(allowedOrigin, "*") {
			prefix := strings.TrimSuffix(allowedOrigin, "*")
			if strings.HasPrefix(strings.ToLower(origin), strings.ToLower(prefix)) {
				return true
			}
		}
	}
	return false
}

func (h *Handler) SubmitForm(c *gin.Context) {
	token := c.Param("token")
	var f db.Form
	var autoReply, active, emailVerified int
	err := h.DB.QueryRow(`SELECT id,owner_user_id,channel_id,name,token,recipient_email,email_verified,success_redirect,success_message,success_theme,allowed_origins,auto_reply_enabled,auto_reply_subject,auto_reply_body,email_subject_template,email_body_template,honeypot_field,active,created_at,updated_at FROM forms WHERE token=?`, token).
		Scan(&f.ID, &f.OwnerUserID, &f.ChannelID, &f.Name, &f.Token, &f.RecipientEmail, &emailVerified, &f.SuccessRedirect, &f.SuccessMessage, &f.SuccessTheme, &f.AllowedOrigins, &autoReply, &f.AutoReplySubject, &f.AutoReplyBody, &f.EmailSubjectTemplate, &f.EmailBodyTemplate, &f.HoneypotField, &active, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		utils.Fail(c, 404, "form not found")
		return
	}
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	
	f.EmailVerified = emailVerified == 1
	if !f.EmailVerified {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusPreconditionRequired, `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>邮箱未验证</title>
    <style>
        body { display: flex; justify-content: center; align-items: center; min-height: 100vh; background: linear-gradient(135deg, #F59E0B 0%, #EA580C 100%); font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { background: white; border-radius: 16px; padding: 48px; text-align: center; box-shadow: 0 20px 60px rgba(0,0,0,0.15); max-width: 400px; }
        .warning { width: 80px; height: 80px; background: linear-gradient(135deg, #F59E0B 0%, #EA580C 100%); border-radius: 50%; display: flex; justify-content: center; align-items: center; margin: 0 auto 20px; font-size: 40px; color: white; }
        h1 { margin: 0 0 8px; color: #1F2937; }
        p { color: #6B7280; margin: 0 0 20px; line-height: 1.6; }
        .email { font-weight: 600; color: #3B82F6; }
    </style>
</head>
<body>
    <div class="container">
        <div class="warning">⚠</div>
        <h1>邮箱未验证</h1>
        <p>表单接收邮箱 <span class="email">` + html.EscapeString(f.RecipientEmail) + `</span> 尚未验证。</p>
        <p>请登录 Formail 账户，查看验证邮件并点击链接完成验证，表单才能正常接收提交。</p>
    </div>
</body>
</html>
		`)
		return
	}
	
	ip := c.ClientIP()
	if !checkRateLimit(ip) {
		utils.Fail(c, 429, "too many requests, please wait 5 seconds")
		return
	}
	
	origin := c.GetHeader("Origin")
	referer := c.GetHeader("Referer")
	if !isOriginAllowed(origin, f.AllowedOrigins) && !isOriginAllowed(referer, f.AllowedOrigins) {
		utils.Fail(c, 403, "request origin not allowed")
		return
	}
	f.AutoReplyEnabled = autoReply == 1
	f.Active = active == 1
	if !f.Active {
		utils.Fail(c, 400, "form is disabled")
		return
	}

	if f.SuccessRedirect != "" && !isSafeRedirectURL(f.SuccessRedirect) {
		utils.Fail(c, 400, "invalid redirect URL")
		return
	}

	data, err := parseSubmissionData(c)
	if err != nil {
		utils.Fail(c, 400, "invalid payload")
		return
	}

	if f.HoneypotField != "" {
		if hp, exists := data[f.HoneypotField]; exists && strings.TrimSpace(hp) != "" {
			utils.Fail(c, 400, "spam detected")
			return
		}
		delete(data, f.HoneypotField)
	}

	// 字段校验
	if strings.TrimSpace(f.FieldsSchema) != "" {
		if errMsg := validateFieldsSchema(f.FieldsSchema, data); errMsg != "" {
			utils.Fail(c, 400, errMsg)
			return
		}
	}

	isSpam, reason := h.Spam.Check(data)
	subJSON, _ := json.Marshal(data)
	enc, err := h.Cryptor.Encrypt(string(subJSON))
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	spamInt := 0
	if isSpam {
		spamInt = 1
	}
	res, err := h.DB.Exec(`INSERT INTO submissions(form_id,ip,user_agent,data_enc,is_spam,email_sent,send_error) VALUES(?,?,?,?,?,?,?)`,
		f.ID, c.ClientIP(), c.Request.UserAgent(), enc, spamInt, 0, reason)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	submissionID, _ := res.LastInsertId()

	emailSent := false
	sendErr := reason
	if !isSpam {
		fieldsText := services.FieldsToText(data)
		subject := services.RenderTemplate(f.EmailSubjectTemplate, map[string]string{
			"form_name": f.Name,
			"ip":        c.ClientIP(),
		})
		body := services.RenderTemplate(f.EmailBodyTemplate, map[string]string{
			"form_name": f.Name,
			"fields":    fieldsText,
			"ip":        c.ClientIP(),
		})
		if f.ChannelID > 0 {
			if ch, err := h.getChannelByID(f.ChannelID); err == nil && ch.Enabled {
				if err := h.Mailer.SendWithOneChannel(ch, f.RecipientEmail, subject, body); err == nil {
					emailSent = true
					sendErr = ""
					_, _ = h.DB.Exec(`INSERT INTO email_logs(submission_id, channel_id, status, message) VALUES(?,?,?,?)`, submissionID, ch.ID, "success", "sent (bound channel)")
				} else {
					_, _ = h.DB.Exec(`INSERT INTO email_logs(submission_id, channel_id, status, message) VALUES(?,?,?,?)`, submissionID, ch.ID, "failed", err.Error())
					emailSent, sendErr = h.Mailer.SendWithFallbackByOwner(f.OwnerUserID, f.RecipientEmail, subject, body, submissionID)
				}
			} else {
				emailSent, sendErr = h.Mailer.SendWithFallbackByOwner(f.OwnerUserID, f.RecipientEmail, subject, body, submissionID)
			}
		} else {
			emailSent, sendErr = h.Mailer.SendWithFallbackByOwner(f.OwnerUserID, f.RecipientEmail, subject, body, submissionID)
		}
		if f.AutoReplyEnabled {
			replyTo := strings.TrimSpace(data["email"])
			if replyTo != "" {
				rSub := services.RenderTemplate(f.AutoReplySubject, map[string]string{"form_name": f.Name})
				rBody := services.RenderTemplate(f.AutoReplyBody, map[string]string{"form_name": f.Name, "fields": fieldsText})
				if f.ChannelID > 0 {
					if ch, err := h.getChannelByID(f.ChannelID); err == nil && ch.Enabled {
						if err := h.Mailer.SendWithOneChannel(ch, replyTo, rSub, rBody); err != nil {
							_, _ = h.Mailer.SendWithFallbackByOwner(f.OwnerUserID, replyTo, rSub, rBody, submissionID)
						}
					} else {
						_, _ = h.Mailer.SendWithFallbackByOwner(f.OwnerUserID, replyTo, rSub, rBody, submissionID)
					}
				} else {
					_, _ = h.Mailer.SendWithFallbackByOwner(f.OwnerUserID, replyTo, rSub, rBody, submissionID)
				}
			}
		}
	}

	emailSentInt := 0
	if emailSent {
		emailSentInt = 1
	}
	_, _ = h.DB.Exec(`UPDATE submissions SET email_sent=?, send_error=? WHERE id=?`, emailSentInt, sendErr, submissionID)

	// Webhook 通知
	if strings.TrimSpace(f.WebhookURL) != "" {
		go func() {
			h.sendWebhook(f, data, c.ClientIP())
		}()
	}

	bgGradient, iconGradient, buttonGradient := getThemeStyles(f.SuccessTheme)
	isDark := f.SuccessTheme == "dark"
	textColor := "#1F2937"
	descColor := "#6B7280"
	cardBg := "white"
	if isDark {
		textColor = "#F9FAFB"
		descColor = "#D1D5DB"
		cardBg = "#374151"
	}

	if strings.Contains(c.GetHeader("Accept"), "text/html") && !strings.Contains(c.GetHeader("X-Requested-With"), "XMLHttpRequest") {
		if f.SuccessRedirect != "" && strings.TrimSpace(f.SuccessMessage) == "" {
			c.Redirect(http.StatusFound, f.SuccessRedirect)
			return
		}
		if strings.TrimSpace(f.SuccessMessage) == "" {
			c.String(200, "提交成功")
			return
		}
		if f.SuccessRedirect != "" {
			c.Header("Content-Type", "text/html")
			c.String(200, `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>提交成功</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: `+bgGradient+`;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        }
        .success-container {
            background: `+cardBg+`;
            border-radius: 20px;
            padding: 48px;
            text-align: center;
            box-shadow: 0 20px 60px rgba(59, 130, 246, 0.15);
            max-width: 480px;
            width: 90%;
            animation: slideUp 0.5s ease-out;
        }
        @keyframes slideUp {
            from { opacity: 0; transform: translateY(30px); }
            to { opacity: 1; transform: translateY(0); }
        }
        .success-icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: `+iconGradient+`;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            box-shadow: 0 10px 30px rgba(16, 185, 129, 0.3);
        }
        .success-icon svg {
            width: 40px;
            height: 40px;
            color: white;
        }
        h1 {
            font-size: 28px;
            font-weight: 700;
            color: `+textColor+`;
            margin-bottom: 12px;
        }
        p {
            font-size: 16px;
            color: `+descColor+`;
            line-height: 1.6;
            margin-bottom: 32px;
        }
        .countdown {
            font-size: 14px;
            color: #9CA3AF;
            margin-bottom: 16px;
        }
        .redirect-link {
            display: inline-block;
            padding: 12px 24px;
            background: `+buttonGradient+`;
            color: white;
            text-decoration: none;
            border-radius: 12px;
            font-weight: 500;
            transition: all 0.2s ease;
        }
        .redirect-link:hover {
            transform: translateY(-2px);
            box-shadow: 0 8px 20px rgba(59, 130, 246, 0.3);
        }
    </style>
</head>
<body>
    <div class="success-container">
        <div class="success-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                <polyline points="20 6 9 17 4 12"/>
            </svg>
        </div>
        <h1>提交成功</h1>
        <p>` + f.SuccessMessage + `</p>
        <div class="countdown" id="countdown">将在 <span id="seconds">5</span> 秒后自动跳转...</div>
        <a href="` + f.SuccessRedirect + `" class="redirect-link">立即跳转</a>
    </div>
    <script>
        let seconds = 5;
        const countdown = document.getElementById('seconds');
        const timer = setInterval(() => {
            seconds--;
            countdown.textContent = seconds;
            if (seconds <= 0) {
                clearInterval(timer);
                window.location.href = '` + f.SuccessRedirect + `';
            }
        }, 1000);
    </script>
</body>
</html>`)
			return
		}
		c.Header("Content-Type", "text/html")
		c.String(200, `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>提交成功</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            background: `+bgGradient+`;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
        }
        .success-container {
            background: `+cardBg+`;
            border-radius: 20px;
            padding: 48px;
            text-align: center;
            box-shadow: 0 20px 60px rgba(59, 130, 246, 0.15);
            max-width: 480px;
            width: 90%;
            animation: slideUp 0.5s ease-out;
        }
        @keyframes slideUp {
            from { opacity: 0; transform: translateY(30px); }
            to { opacity: 1; transform: translateY(0); }
        }
        .success-icon {
            width: 80px;
            height: 80px;
            margin: 0 auto 24px;
            background: `+iconGradient+`;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            box-shadow: 0 10px 30px rgba(16, 185, 129, 0.3);
        }
        .success-icon svg {
            width: 40px;
            height: 40px;
            color: white;
        }
        h1 {
            font-size: 28px;
            font-weight: 700;
            color: `+textColor+`;
            margin-bottom: 12px;
        }
        p {
            font-size: 16px;
            color: `+descColor+`;
            line-height: 1.6;
        }
    </style>
</head>
<body>
    <div class="success-container">
        <div class="success-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3">
                <polyline points="20 6 9 17 4 12"/>
            </svg>
        </div>
        <h1>提交成功</h1>
        <p>` + f.SuccessMessage + `</p>
    </div>
</body>
</html>`)
		return
	}
	utils.OK(c, gin.H{
		"message": f.SuccessMessage,
		"email_sent": emailSent,
		"is_spam": isSpam,
		"redirect_url": f.SuccessRedirect,
	})
}

func (h *Handler) ListSubmissions(c *gin.Context) {
	uid := h.currentUserID(c)
	formID := c.Query("form_id")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	where := `WHERE f.owner_user_id=?`
	args := []interface{}{uid}
	if formID != "" {
		if _, err := strconv.ParseInt(formID, 10, 64); err != nil {
			utils.Fail(c, 400, "invalid form_id")
			return
		}
		where += ` AND s.form_id=?`
		args = append(args, formID)
	}

	var total int
	countQuery := `SELECT COUNT(1) FROM submissions s JOIN forms f ON f.id=s.form_id ` + where
	if err := h.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	query := `SELECT s.id,s.form_id,s.ip,s.user_agent,s.data_enc,s.is_spam,s.email_sent,s.send_error,s.created_at FROM submissions s JOIN forms f ON f.id=s.form_id ` + where + ` ORDER BY s.id DESC LIMIT ? OFFSET ?`
	rows, err := h.DB.Query(query, append(args, limit, offset)...)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	items := []db.Submission{}
	for rows.Next() {
		var s db.Submission
		var enc string
		var spam, sent int
		if err := rows.Scan(&s.ID, &s.FormID, &s.IP, &s.UserAgent, &enc, &spam, &sent, &s.SendError, &s.CreatedAt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		plain, _ := h.Cryptor.Decrypt(enc)
		s.Data = map[string]string{}
		_ = json.Unmarshal([]byte(plain), &s.Data)
		s.IsSpam = spam == 1
		s.EmailSent = sent == 1
		items = append(items, s)
	}
	utils.OK(c, gin.H{"items": items, "total": total, "page": page, "limit": limit})
}

func (h *Handler) DeleteSubmission(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)
	if _, err := h.DB.Exec(`DELETE FROM submissions WHERE id IN (SELECT s.id FROM submissions s JOIN forms f ON f.id=s.form_id WHERE s.id=? AND f.owner_user_id=?)`, id, uid); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"deleted": true})
}

func (h *Handler) BatchDeleteSubmissions(c *gin.Context) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		utils.Fail(c, 400, "invalid request: ids required")
		return
	}
	if len(req.IDs) > 500 {
		utils.Fail(c, 400, "最多同时删除 500 条")
		return
	}
	uid := h.currentUserID(c)
	placeholders := make([]string, len(req.IDs))
	args := []interface{}{uid}
	for i, id := range req.IDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := `DELETE FROM submissions WHERE id IN (SELECT s.id FROM submissions s JOIN forms f ON f.id=s.form_id WHERE s.id IN (` + strings.Join(placeholders, ",") + `) AND f.owner_user_id=?)`
	res, err := h.DB.Exec(query, args...)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	utils.OK(c, gin.H{"deleted": affected})
}

func (h *Handler) ExportSubmissionsCSV(c *gin.Context) {
	uid := h.currentUserID(c)
	formID := c.Query("form_id")
	query := `SELECT s.id,s.form_id,s.ip,s.user_agent,s.data_enc,s.is_spam,s.email_sent,s.send_error,s.created_at FROM submissions s JOIN forms f ON f.id=s.form_id WHERE f.owner_user_id=?`
	args := []interface{}{uid}
	if formID != "" {
		if _, err := strconv.ParseInt(formID, 10, 64); err != nil {
			utils.Fail(c, 400, "invalid form_id")
			return
		}
		query += ` AND s.form_id=?`
		args = append(args, formID)
	}
	query += ` ORDER BY s.id DESC`
	rows, err := h.DB.Query(query, args...)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="submissions.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"id", "form_id", "ip", "user_agent", "is_spam", "email_sent", "send_error", "created_at", "data_json"})
	for rows.Next() {
		var id, fid int64
		var ip, ua, enc, errMsg, created string
		var spam, sent int
		if err := rows.Scan(&id, &fid, &ip, &ua, &enc, &spam, &sent, &errMsg, &created); err != nil {
			continue
		}
		plain, _ := h.Cryptor.Decrypt(enc)
		_ = w.Write([]string{
			strconv.FormatInt(id, 10),
			strconv.FormatInt(fid, 10),
			ip,
			ua,
			strconv.Itoa(spam),
			strconv.Itoa(sent),
			errMsg,
			created,
			plain,
		})
	}
	w.Flush()
}

type fieldDef struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

func validateFieldsSchema(schemaJSON string, data map[string]string) string {
	var fields []fieldDef
	if err := json.Unmarshal([]byte(schemaJSON), &fields); err != nil {
		return ""
	}
	for _, f := range fields {
		val := strings.TrimSpace(data[f.Name])
		if f.Required && val == "" {
			label := f.Label
			if label == "" {
				label = f.Name
			}
			return label + " 为必填项"
		}
		if f.Type == "email" && val != "" {
			if !strings.Contains(val, "@") || !strings.Contains(val, ".") {
				label := f.Label
				if label == "" {
					label = f.Name
				}
				return label + " 格式不正确"
			}
		}
	}
	return ""
}

func (h *Handler) sendWebhook(f db.Form, data map[string]string, ip string) {
	payload := map[string]interface{}{
		"form_id":       f.ID,
		"form_name":     f.Name,
		"submitted_at":  time.Now().Format(time.RFC3339),
		"data":          data,
		"ip":            ip,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", f.WebhookURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook request build error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Formail-Webhook/1.0")

	if secret := strings.TrimSpace(f.WebhookSecret); secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Webhook-Signature", "sha256="+sig)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook send error [%s]: %v", f.WebhookURL, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("webhook response error [%s]: HTTP %d", f.WebhookURL, resp.StatusCode)
	}
}
