package handlers

import (
	"database/sql"
	"fmt"
	"html"
	"net/http"
	"strings"

	"formail/internal/db"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type formReq struct {
	Name                 string `json:"name"`
	RecipientEmail       string `json:"recipient_email"`
	ChannelID            int64  `json:"channel_id"`
	SuccessRedirect      string `json:"success_redirect"`
	SuccessMessage       string `json:"success_message"`
	SuccessTheme         string `json:"success_theme"`
	AllowedOrigins       string `json:"allowed_origins"`
	AutoReplyEnabled     bool   `json:"auto_reply_enabled"`
	AutoReplySubject     string `json:"auto_reply_subject"`
	AutoReplyBody        string `json:"auto_reply_body"`
	EmailSubjectTemplate string `json:"email_subject_template"`
	EmailBodyTemplate    string `json:"email_body_template"`
	HoneypotField        string `json:"honeypot_field"`
	WebhookURL           string `json:"webhook_url"`
	WebhookSecret        string `json:"webhook_secret"`
	FieldsSchema         string `json:"fields_schema"`
	Active               bool   `json:"active"`
}

func (h *Handler) channelBindableByUser(ownerUserID, channelID int64, role string) (bool, error) {
	if channelID <= 0 {
		return true, nil
	}
	var ch db.Channel
	var useTLS, enabled, shareEnabled int
	err := h.DB.QueryRow(`SELECT id,owner_user_id,name,type,provider,protocol,host,port,username,password_enc,from_email,use_tls,priority,enabled,share_enabled,share_max_bindings_per_user,share_max_total_bindings,created_at,updated_at FROM channels WHERE id=?`, channelID).
		Scan(&ch.ID, &ch.OwnerUserID, &ch.Name, &ch.Type, &ch.Provider, &ch.Protocol, &ch.Host, &ch.Port, &ch.Username, &ch.PasswordEnc, &ch.FromEmail, &useTLS, &ch.Priority, &enabled, &shareEnabled, &ch.ShareMaxBindingsPerUser, &ch.ShareMaxTotalBindings, &ch.CreatedAt, &ch.UpdatedAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ch.UseTLS = useTLS == 1
	ch.Enabled = enabled == 1
	ch.ShareEnabled = shareEnabled == 1
	if !ch.Enabled {
		return false, nil
	}
	if ch.OwnerUserID == ownerUserID {
		return true, nil
	}
	if strings.ToLower(role) == "admin" {
		return false, nil
	}
	adminID, err := h.primaryAdminUserID()
	if err != nil {
		return false, err
	}
	if ch.OwnerUserID != adminID || !ch.ShareEnabled {
		return false, nil
	}
	var userBindCnt int
	if err := h.DB.QueryRow(`SELECT COUNT(1) FROM forms WHERE owner_user_id=? AND channel_id=?`, ownerUserID, ch.ID).Scan(&userBindCnt); err != nil {
		return false, err
	}
	if ch.ShareMaxBindingsPerUser > 0 && userBindCnt >= ch.ShareMaxBindingsPerUser {
		return false, nil
	}
	var totalSharedCnt int
	if err := h.DB.QueryRow(`SELECT COUNT(1) FROM forms WHERE channel_id=? AND owner_user_id<>?`, ch.ID, ch.OwnerUserID).Scan(&totalSharedCnt); err != nil {
		return false, err
	}
	if ch.ShareMaxTotalBindings > 0 && totalSharedCnt >= ch.ShareMaxTotalBindings {
		return false, nil
	}
	return true, nil
}

func (h *Handler) ListForms(c *gin.Context) {
	uid := h.currentUserID(c)
	isAdmin := h.currentUserIsAdmin(c)

	where := `WHERE owner_user_id=?`
	args := []interface{}{uid}

	if isAdmin {
		filterUserID := c.Query("user_id")
		if filterUserID != "" {
			where = `WHERE owner_user_id=?`
			args = []interface{}{filterUserID}
		} else {
			where = ""
			args = []interface{}{}
		}
	}

	rows, err := h.DB.Query(`SELECT id,owner_user_id,channel_id,name,token,recipient_email,email_verified,verify_token,success_redirect,success_message,success_theme,allowed_origins,auto_reply_enabled,auto_reply_subject,auto_reply_body,email_subject_template,email_body_template,honeypot_field,webhook_url,webhook_secret,fields_schema,active,created_at,updated_at FROM forms `+where+` ORDER BY id DESC`, args...)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()
	forms := make([]db.Form, 0)
	for rows.Next() {
		var f db.Form
		var autoReply, active, emailVerified int
		if err := rows.Scan(&f.ID, &f.OwnerUserID, &f.ChannelID, &f.Name, &f.Token, &f.RecipientEmail, &emailVerified, &f.VerifyToken, &f.SuccessRedirect, &f.SuccessMessage, &f.SuccessTheme, &f.AllowedOrigins, &autoReply, &f.AutoReplySubject, &f.AutoReplyBody, &f.EmailSubjectTemplate, &f.EmailBodyTemplate, &f.HoneypotField, &f.WebhookURL, &f.WebhookSecret, &f.FieldsSchema, &active, &f.CreatedAt, &f.UpdatedAt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		f.EmailVerified = emailVerified == 1
		f.AutoReplyEnabled = autoReply == 1
		f.Active = active == 1
		forms = append(forms, f)
	}
	utils.OK(c, forms)
}

func (h *Handler) CreateForm(c *gin.Context) {
	var req formReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.RecipientEmail = strings.TrimSpace(req.RecipientEmail)
	if req.Name == "" || req.RecipientEmail == "" {
		utils.Fail(c, 400, "name and recipient_email are required")
		return
	}
	if req.EmailSubjectTemplate == "" {
		req.EmailSubjectTemplate = "新表单提交: {{form_name}}"
	}
	if req.EmailBodyTemplate == "" {
		req.EmailBodyTemplate = "{{fields}}"
	}
	if req.HoneypotField == "" {
		req.HoneypotField = "_gotcha"
	}
	if req.SuccessTheme == "" {
		req.SuccessTheme = "blue"
	}
	token, err := utils.Token(18)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	verifyToken, err := utils.Token(32)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	active := 0
	if req.Active {
		active = 1
	}
	autoReply := 0
	if req.AutoReplyEnabled {
		autoReply = 1
	}
	uid := h.currentUserID(c)
	ok, err := h.channelBindableByUser(uid, req.ChannelID, h.currentUserRole(c))
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if !ok {
		utils.Fail(c, 400, "channel is not bindable for current user")
		return
	}
	res, err := h.DB.Exec(`INSERT INTO forms(owner_user_id,channel_id,name,token,recipient_email,email_verified,verify_token,success_redirect,success_message,success_theme,allowed_origins,auto_reply_enabled,auto_reply_subject,auto_reply_body,email_subject_template,email_body_template,honeypot_field,webhook_url,webhook_secret,fields_schema,active) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, req.ChannelID, req.Name, token, req.RecipientEmail, 0, verifyToken, req.SuccessRedirect, req.SuccessMessage, req.SuccessTheme, req.AllowedOrigins, autoReply, req.AutoReplySubject, req.AutoReplyBody, req.EmailSubjectTemplate, req.EmailBodyTemplate, req.HoneypotField, strings.TrimSpace(req.WebhookURL), strings.TrimSpace(req.WebhookSecret), strings.TrimSpace(req.FieldsSchema), active)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()

	verifyURL := h.buildVerifyURL(c, verifyToken)
	go func() {
		subject := "请验证您的 Formail 表单邮箱"
		body := fmt.Sprintf(`您好！

您在 Formail 创建了一个表单，需要验证邮箱后才能接收表单提交。

表单名称：%s
接收邮箱：%s

点击下方链接完成验证：
%s

如果您没有创建此表单，请忽略此邮件。

感谢使用 Formail！`, req.Name, req.RecipientEmail, verifyURL)
		_, _ = h.Mailer.SendWithFallbackByOwner(uid, req.RecipientEmail, subject, body, 0)
	}()

	utils.OK(c, gin.H{"id": id, "token": token, "submit_url": "/f/" + token, "email_verified": false, "message": "验证邮件已发送到您的邮箱，请点击链接完成验证"})
}

func (h *Handler) UpdateForm(c *gin.Context) {
	id := c.Param("id")
	var req formReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	if req.Name == "" || req.RecipientEmail == "" {
		utils.Fail(c, 400, "name and recipient_email are required")
		return
	}
	if req.SuccessTheme == "" {
		req.SuccessTheme = "blue"
	}
	autoReply := 0
	if req.AutoReplyEnabled {
		autoReply = 1
	}
	active := 0
	if req.Active {
		active = 1
	}
	uid := h.currentUserID(c)
	isAdmin := h.currentUserIsAdmin(c)

	if !isAdmin {
		ok, err := h.channelBindableByUser(uid, req.ChannelID, h.currentUserRole(c))
		if err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		if !ok {
			utils.Fail(c, 400, "channel is not bindable for current user")
			return
		}
	}

	ownerWhere := `WHERE id=? AND owner_user_id=?`
	ownerArgs := []interface{}{id, uid}
	if isAdmin {
		ownerWhere = `WHERE id=?`
		ownerArgs = []interface{}{id}
	}

	var existingEmail, existingToken string
	err := h.DB.QueryRow(`SELECT recipient_email, verify_token FROM forms `+ownerWhere, ownerArgs...).Scan(&existingEmail, &existingToken)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	updateWhere := `WHERE id=?`
	updateArgs := []interface{}{id}
	if !isAdmin {
		updateWhere += ` AND owner_user_id=?`
		updateArgs = append(updateArgs, uid)
	}

	if existingEmail != req.RecipientEmail {
		newVerifyToken, _ := utils.Token(32)
		_, err = h.DB.Exec(`UPDATE forms SET channel_id=?,name=?,recipient_email=?,email_verified=0,verify_token=?,success_redirect=?,success_message=?,success_theme=?,allowed_origins=?,auto_reply_enabled=?,auto_reply_subject=?,auto_reply_body=?,email_subject_template=?,email_body_template=?,honeypot_field=?,webhook_url=?,webhook_secret=?,fields_schema=?,active=?,updated_at=CURRENT_TIMESTAMP `+updateWhere,
			append([]interface{}{req.ChannelID, req.Name, req.RecipientEmail, newVerifyToken, req.SuccessRedirect, req.SuccessMessage, req.SuccessTheme, req.AllowedOrigins, autoReply, req.AutoReplySubject, req.AutoReplyBody, req.EmailSubjectTemplate, req.EmailBodyTemplate, req.HoneypotField, strings.TrimSpace(req.WebhookURL), strings.TrimSpace(req.WebhookSecret), strings.TrimSpace(req.FieldsSchema), active}, updateArgs...)...)

		verifyURL := h.buildVerifyURL(c, newVerifyToken)
		go func() {
			subject := "请验证您的 Formail 表单邮箱"
			body := fmt.Sprintf(`您好！

您在 Formail 更新了表单的接收邮箱，需要重新验证。

表单名称：%s
新接收邮箱：%s

点击下方链接完成验证：
%s

如果您没有进行此操作，请忽略此邮件。

感谢使用 Formail！`, req.Name, req.RecipientEmail, verifyURL)
			_, _ = h.Mailer.SendWithFallbackByOwner(uid, req.RecipientEmail, subject, body, 0)
		}()
	} else {
		_, err = h.DB.Exec(`UPDATE forms SET channel_id=?,name=?,recipient_email=?,success_redirect=?,success_message=?,success_theme=?,allowed_origins=?,auto_reply_enabled=?,auto_reply_subject=?,auto_reply_body=?,email_subject_template=?,email_body_template=?,honeypot_field=?,webhook_url=?,webhook_secret=?,fields_schema=?,active=?,updated_at=CURRENT_TIMESTAMP `+updateWhere,
			append([]interface{}{req.ChannelID, req.Name, req.RecipientEmail, req.SuccessRedirect, req.SuccessMessage, req.SuccessTheme, req.AllowedOrigins, autoReply, req.AutoReplySubject, req.AutoReplyBody, req.EmailSubjectTemplate, req.EmailBodyTemplate, req.HoneypotField, strings.TrimSpace(req.WebhookURL), strings.TrimSpace(req.WebhookSecret), strings.TrimSpace(req.FieldsSchema), active}, updateArgs...)...)
	}

	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"updated": true})
}

func (h *Handler) DeleteForm(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)
	query := `DELETE FROM forms WHERE id=?`
	args := []interface{}{id}
	if !h.currentUserIsAdmin(c) {
		query += ` AND owner_user_id=?`
		args = append(args, uid)
	}
	_, err := h.DB.Exec(query, args...)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"deleted": true})
}

func (h *Handler) GetForm(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)
	var f db.Form
	var autoReply, active, emailVerified int
	query := `SELECT id,owner_user_id,channel_id,name,token,recipient_email,email_verified,verify_token,success_redirect,success_message,success_theme,allowed_origins,auto_reply_enabled,auto_reply_subject,auto_reply_body,email_subject_template,email_body_template,honeypot_field,webhook_url,webhook_secret,fields_schema,active,created_at,updated_at FROM forms WHERE id=?`
	args := []interface{}{id}
	if !h.currentUserIsAdmin(c) {
		query += ` AND owner_user_id=?`
		args = append(args, uid)
	}
	err := h.DB.QueryRow(query, args...).
		Scan(&f.ID, &f.OwnerUserID, &f.ChannelID, &f.Name, &f.Token, &f.RecipientEmail, &emailVerified, &f.VerifyToken, &f.SuccessRedirect, &f.SuccessMessage, &f.SuccessTheme, &f.AllowedOrigins, &autoReply, &f.AutoReplySubject, &f.AutoReplyBody, &f.EmailSubjectTemplate, &f.EmailBodyTemplate, &f.HoneypotField, &f.WebhookURL, &f.WebhookSecret, &f.FieldsSchema, &active, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		utils.Fail(c, 404, "not found")
		return
	}
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	f.EmailVerified = emailVerified == 1
	f.AutoReplyEnabled = autoReply == 1
	f.Active = active == 1
	utils.OK(c, f)
}

func (h *Handler) VerifyEmail(c *gin.Context) {
	token := c.Param("token")

	var f db.Form
	var emailVerified int
	err := h.DB.QueryRow(`SELECT id,name,recipient_email,email_verified FROM forms WHERE verify_token=?`, token).
		Scan(&f.ID, &f.Name, &f.RecipientEmail, &emailVerified)
	if err == sql.ErrNoRows {
		c.String(http.StatusBadRequest, "无效的验证链接")
		return
	}
	if err != nil {
		c.String(http.StatusInternalServerError, "服务器内部错误")
		return
	}

	if emailVerified == 1 {
		c.Header("Content-Type", "text/html; charset=utf-8")
		c.String(http.StatusOK, `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>验证成功</title>
    <style>
        body { display: flex; justify-content: center; align-items: center; min-height: 100vh; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { background: white; border-radius: 16px; padding: 48px; text-align: center; box-shadow: 0 20px 60px rgba(0,0,0,0.15); }
        .checkmark { width: 80px; height: 80px; background: linear-gradient(135deg, #10B981 0%, #059669 100%); border-radius: 50%; display: flex; justify-content: center; align-items: center; margin: 0 auto 20px; font-size: 40px; color: white; }
        h1 { margin: 0 0 8px; color: #1F2937; }
        p { color: #6B7280; margin: 0 0 20px; }
        a { display: inline-block; padding: 10px 24px; background: linear-gradient(135deg, #3B82F6 0%, #2563EB 100%); color: white; text-decoration: none; border-radius: 8px; font-weight: 500; }
    </style>
</head>
<body>
    <div class="container">
        <div class="checkmark">✓</div>
        <h1>邮箱已验证</h1>
        <p>您的邮箱 <strong>` + html.EscapeString(f.RecipientEmail) + `</strong> 已成功验证！</p>
        <p>表单「` + html.EscapeString(f.Name) + `」现已生效，可以开始接收提交。</p>
        <a href="/dashboard/forms">返回表单管理</a>
    </div>
</body>
</html>
		`)
		return
	}

	_, err = h.DB.Exec(`UPDATE forms SET email_verified=1, updated_at=CURRENT_TIMESTAMP WHERE verify_token=?`, token)
	if err != nil {
		c.String(http.StatusInternalServerError, "验证失败，请重试")
		return
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>验证成功</title>
    <style>
        body { display: flex; justify-content: center; align-items: center; min-height: 100vh; background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
        .container { background: white; border-radius: 16px; padding: 48px; text-align: center; box-shadow: 0 20px 60px rgba(0,0,0,0.15); }
        .checkmark { width: 80px; height: 80px; background: linear-gradient(135deg, #10B981 0%, #059669 100%); border-radius: 50%; display: flex; justify-content: center; align-items: center; margin: 0 auto 20px; font-size: 40px; color: white; }
        h1 { margin: 0 0 8px; color: #1F2937; }
        p { color: #6B7280; margin: 0 0 20px; }
        a { display: inline-block; padding: 10px 24px; background: linear-gradient(135deg, #3B82F6 0%, #2563EB 100%); color: white; text-decoration: none; border-radius: 8px; font-weight: 500; }
    </style>
</head>
<body>
    <div class="container">
        <div class="checkmark">✓</div>
        <h1>验证成功！</h1>
        <p>您的邮箱 <strong>` + html.EscapeString(f.RecipientEmail) + `</strong> 已成功验证。</p>
        <p>表单「` + html.EscapeString(f.Name) + `」现已生效，可以开始接收提交。</p>
        <a href="/dashboard/forms">返回表单管理</a>
    </div>
</body>
</html>
	`)
}

func (h *Handler) buildVerifyURL(c *gin.Context, verifyToken string) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := c.Request.Host
	if host == "" {
		host = "localhost" + h.Cfg.Server.Address
	}
	return fmt.Sprintf("%s://%s/verify/%s", scheme, host, verifyToken)
}

func (h *Handler) ResendFormVerifyEmail(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)

	var name, recipientEmail, verifyToken string
	var channelID int64
	err := h.DB.QueryRow(`SELECT name, recipient_email, verify_token, channel_id FROM forms WHERE id=? AND owner_user_id=?`, id, uid).Scan(&name, &recipientEmail, &verifyToken, &channelID)
	if err == sql.ErrNoRows {
		utils.Fail(c, 404, "表单不存在")
		return
	}
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	verifyURL := h.buildVerifyURL(c, verifyToken)
	subject := "请验证您的 Formail 表单邮箱"
	body := fmt.Sprintf(`您好！

您在 Formail 创建了一个表单，需要验证邮箱后才能接收表单提交。

表单名称：%s
接收邮箱：%s

点击下方链接完成验证：
%s

如果您没有创建此表单，请忽略此邮件。

感谢使用 Formail！`, name, recipientEmail, verifyURL)

	var emailSent bool
	var sendErr string
	var channelName string

	if channelID > 0 {
		ch, err := h.getChannelByID(channelID)
		if err == nil && ch.Enabled {
			channelName = ch.Name
			if err := h.Mailer.SendWithOneChannel(ch, recipientEmail, subject, body); err != nil {
				sendErr = fmt.Sprintf("渠道[%s]发送失败: %s", ch.Name, err.Error())
			} else {
				emailSent = true
			}
		} else {
			sendErr = "绑定的邮件渠道不可用"
		}
	}

	if !emailSent && sendErr == "" {
		emailSent, sendErr = h.Mailer.SendWithFallbackByOwner(uid, recipientEmail, subject, body, 0)
		if emailSent {
			channelName = "自动选择"
		}
	}

	if !emailSent {
		utils.Fail(c, 500, "邮件发送失败: "+sendErr)
		return
	}

	utils.OK(c, gin.H{
		"message":       "验证邮件已发送",
		"channel_name":  channelName,
		"recipient":     recipientEmail,
	})
}
