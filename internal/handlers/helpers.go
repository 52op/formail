package handlers

import (
	"database/sql"
	"fmt"
	"net/mail"
	"strconv"
	"strings"

	"formail/internal/db"

	"github.com/gin-gonic/gin"
)

func isValidEmail(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	_, err := mail.ParseAddress(v)
	return err == nil
}

func getSetting(db *sql.DB, key, fallback string) string {
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return fallback
	}
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func getSettingBool(db *sql.DB, key string, fallback bool) bool {
	def := "0"
	if fallback {
		def = "1"
	}
	v := strings.TrimSpace(strings.ToLower(getSetting(db, key, def)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func getSettingInt(db *sql.DB, key string, fallback int) int {
	v := strings.TrimSpace(getSetting(db, key, strconv.Itoa(fallback)))
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func (h *Handler) resolveAdminNotifyEmail() string {
	adminEmail := strings.TrimSpace(strings.ToLower(getSetting(h.DB, "admin_notify_email", "")))
	if isValidEmail(adminEmail) {
		return adminEmail
	}

	var username, email string
	err := h.DB.QueryRow(`SELECT username,email FROM users WHERE role='admin' AND status=1 ORDER BY id ASC LIMIT 1`).Scan(&username, &email)
	if err == nil {
		em := strings.TrimSpace(strings.ToLower(email))
		if isValidEmail(em) {
			return em
		}
		u := strings.TrimSpace(strings.ToLower(username))
		if isValidEmail(u) {
			return u
		}
	}
	return ""
}

func (h *Handler) loadServiceMailChannel() (*db.Channel, error) {
	adminID, err := h.primaryAdminUserID()
	if err != nil {
		return nil, err
	}
	idStr := strings.TrimSpace(getSetting(h.DB, "service_mail_channel_id", ""))
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err == nil && id > 0 {
			ch, err := h.getChannelByID(id)
			if err == nil && ch.Enabled && ch.OwnerUserID == adminID {
				return &ch, nil
			}
		}
	}

	channels, err := h.Mailer.ListEnabledChannelsByOwner(adminID)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("no enabled channels")
	}
	return &channels[0], nil
}

func (h *Handler) getChannelByID(id int64) (db.Channel, error) {
	var ch db.Channel
	var useTLS, enabled int
	err := h.DB.QueryRow(`SELECT id,owner_user_id,name,type,provider,protocol,host,port,username,password_enc,from_email,use_tls,priority,enabled,created_at,updated_at FROM channels WHERE id=?`, id).
		Scan(&ch.ID, &ch.OwnerUserID, &ch.Name, &ch.Type, &ch.Provider, &ch.Protocol, &ch.Host, &ch.Port, &ch.Username, &ch.PasswordEnc, &ch.FromEmail, &useTLS, &ch.Priority, &enabled, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		return db.Channel{}, err
	}
	ch.UseTLS = useTLS == 1
	ch.Enabled = enabled == 1
	return ch, nil
}

func (h *Handler) sendServiceMail(to, subject, body string) error {
	ch, err := h.loadServiceMailChannel()
	if err != nil {
		return err
	}
	return h.Mailer.SendWithOneChannel(*ch, to, subject, body)
}

func (h *Handler) enqueueServiceMail(to, subject, body, purpose string) error {
	if h.MailQueue == nil {
		return h.sendServiceMail(to, subject, body)
	}
	_, err := h.MailQueue.Enqueue(to, subject, body, purpose)
	return err
}

func (h *Handler) SendServiceMail(to, subject, body string) error {
	return h.sendServiceMail(to, subject, body)
}

func (h *Handler) currentUserID(c *gin.Context) int64 {
	return c.GetInt64("user_id")
}

func (h *Handler) currentUserRole(c *gin.Context) string {
	return strings.TrimSpace(strings.ToLower(c.GetString("role")))
}

func (h *Handler) currentUserIsAdmin(c *gin.Context) bool {
	return h.currentUserRole(c) == "admin"
}

func (h *Handler) primaryAdminUserID() (int64, error) {
	var id int64
	err := h.DB.QueryRow(`SELECT id FROM users WHERE role='admin' ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	err = h.DB.QueryRow(`SELECT id FROM users ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}
