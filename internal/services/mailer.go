package services

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"formail/internal/db"
	"formail/internal/utils"
)

type MailerService struct {
	DB      *sql.DB
	Cryptor *utils.Cryptor
}

const (
	smtpConnectTimeout = 10 * time.Second
	smtpOpTimeout      = 20 * time.Second
)

func smtpPreset(provider string) (host string, port int, tlsOn bool) {
	switch strings.ToLower(provider) {
	case "qq":
		return "smtp.qq.com", 465, true
	case "163":
		return "smtp.163.com", 465, true
	case "outlook", "hotmail":
		return "smtp.office365.com", 587, false
	default:
		return "", 0, true
	}
}

func buildMIMEMessage(from, to, subject, text, html string) []byte {
	if html == "" {
		return []byte("To: " + to + "\r\n" +
			"From: " + from + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
			text + "\r\n")
	}
	boundary := fmt.Sprintf("fm_boundary_%d", time.Now().UnixNano())
	return []byte("To: " + to + "\r\n" +
		"From: " + from + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		text + "\r\n\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" +
		html + "\r\n\r\n" +
		"--" + boundary + "--\r\n")
}

func (m *MailerService) resolveChannelSMTP(ch db.Channel) (host string, port int, useTLS bool, from, password string, err error) {
	host = ch.Host
	port = ch.Port
	useTLS = ch.UseTLS
	if ch.Type == "builtin" {
		ph, pp, pt := smtpPreset(ch.Provider)
		if host == "" {
			host = ph
		}
		if port == 0 {
			port = pp
		}
		useTLS = pt
	}
	if host == "" || port == 0 {
		err = fmt.Errorf("invalid smtp host/port")
		return
	}
	from = ch.FromEmail
	if from == "" {
		from = ch.Username
	}
	password, err = m.Cryptor.Decrypt(ch.PasswordEnc)
	if err != nil {
		err = fmt.Errorf("decrypt channel password failed: %w", err)
	}
	return
}

func (m *MailerService) dialAndSend(ch db.Channel, from, to string, msg []byte) error {
	host, port, useTLS, _, password, err := m.resolveChannelSMTP(ch)
	if err != nil {
		return err
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	auth := smtp.PlainAuth("", ch.Username, password, host)
	dialer := &net.Dialer{Timeout: smtpConnectTimeout}

	if useTLS {
		tlsCfg := &tls.Config{ServerName: host}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err != nil {
			return err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(smtpOpTimeout))
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			return err
		}
		defer c.Quit()
		if err := c.Auth(auth); err != nil {
			return err
		}
		if err := c.Mail(from); err != nil {
			return err
		}
		if err := c.Rcpt(to); err != nil {
			return err
		}
		w, err := c.Data()
		if err != nil {
			return err
		}
		if _, err := w.Write(msg); err != nil {
			return err
		}
		return w.Close()
	}

	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(smtpOpTimeout))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Quit()
	if err := c.Auth(auth); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	return w.Close()
}

func (m *MailerService) sendByChannel(ch db.Channel, to, subject, body string) error {
	if strings.ToLower(ch.Protocol) != "smtp" {
		return fmt.Errorf("protocol %s not supported for sending", ch.Protocol)
	}
	_, _, _, from, _, err := m.resolveChannelSMTP(ch)
	if err != nil {
		return err
	}
	msg := buildMIMEMessage(from, to, subject, body, "")
	return m.dialAndSend(ch, from, to, msg)
}

func (m *MailerService) sendMessageByChannel(ch db.Channel, to, subject, text, html string) error {
	if strings.ToLower(ch.Protocol) != "smtp" {
		return fmt.Errorf("protocol %s not supported for sending", ch.Protocol)
	}
	_, _, _, from, _, err := m.resolveChannelSMTP(ch)
	if err != nil {
		return err
	}
	msg := buildMIMEMessage(from, to, subject, text, html)
	return m.dialAndSend(ch, from, to, msg)
}

func (m *MailerService) SendWithFallback(to, subject, body string, submissionID int64) (bool, string) {
	channels, err := m.ListEnabledChannels()
	if err != nil {
		return false, err.Error()
	}
	if len(channels) == 0 {
		return false, "no enabled channels"
	}
	for _, ch := range channels {
		err := m.sendByChannel(ch, to, subject, body)
		if err == nil {
			_, _ = m.DB.Exec(`INSERT INTO email_logs(submission_id, channel_id, status, message) VALUES(?,?,?,?)`, submissionID, ch.ID, "success", "sent")
			return true, ""
		}
		_, _ = m.DB.Exec(`INSERT INTO email_logs(submission_id, channel_id, status, message) VALUES(?,?,?,?)`, submissionID, ch.ID, "failed", err.Error())
	}
	return false, "all channels failed"
}

func (m *MailerService) SendWithFallbackByOwner(ownerUserID int64, to, subject, body string, submissionID int64) (bool, string) {
	channels, err := m.ListEnabledChannelsByOwner(ownerUserID)
	if err != nil {
		return false, err.Error()
	}
	if len(channels) == 0 {
		return false, "no enabled channels"
	}
	for _, ch := range channels {
		err := m.sendByChannel(ch, to, subject, body)
		if err == nil {
			_, _ = m.DB.Exec(`INSERT INTO email_logs(submission_id, channel_id, status, message) VALUES(?,?,?,?)`, submissionID, ch.ID, "success", "sent")
			return true, ""
		}
		_, _ = m.DB.Exec(`INSERT INTO email_logs(submission_id, channel_id, status, message) VALUES(?,?,?,?)`, submissionID, ch.ID, "failed", err.Error())
	}
	return false, "all channels failed"
}

// SendMessageWithFallbackByOwner 支持 HTML 邮件，供 HTTP API 使用。
func (m *MailerService) SendMessageWithFallbackByOwner(ownerUserID int64, to, subject, text, html string) (bool, string) {
	channels, err := m.ListEnabledChannelsByOwner(ownerUserID)
	if err != nil {
		return false, err.Error()
	}
	if len(channels) == 0 {
		return false, "no enabled channels"
	}
	for _, ch := range channels {
		if err := m.sendMessageByChannel(ch, to, subject, text, html); err == nil {
			return true, ""
		}
	}
	return false, "all channels failed"
}

func (m *MailerService) SendWithOneChannel(ch db.Channel, to, subject, body string) error {
	return m.sendByChannel(ch, to, subject, body)
}

func (m *MailerService) SendMessageWithOneChannel(ch db.Channel, to, subject, text, html string) error {
	return m.sendMessageByChannel(ch, to, subject, text, html)
}

func (m *MailerService) ListEnabledChannels() ([]db.Channel, error) {
	rows, err := m.DB.Query(`SELECT id,owner_user_id,name,type,provider,protocol,host,port,username,password_enc,from_email,use_tls,priority,enabled,created_at,updated_at FROM channels WHERE enabled=1 ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]db.Channel, 0)
	for rows.Next() {
		var c db.Channel
		var useTLS, enabled int
		if err := rows.Scan(&c.ID, &c.OwnerUserID, &c.Name, &c.Type, &c.Provider, &c.Protocol, &c.Host, &c.Port, &c.Username, &c.PasswordEnc, &c.FromEmail, &useTLS, &c.Priority, &enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.UseTLS = useTLS == 1
		c.Enabled = enabled == 1
		out = append(out, c)
	}
	return out, nil
}

func (m *MailerService) ListEnabledChannelsByOwner(ownerUserID int64) ([]db.Channel, error) {
	rows, err := m.DB.Query(`SELECT id,owner_user_id,name,type,provider,protocol,host,port,username,password_enc,from_email,use_tls,priority,enabled,created_at,updated_at FROM channels WHERE owner_user_id=? AND enabled=1 ORDER BY priority ASC, id ASC`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]db.Channel, 0)
	for rows.Next() {
		var c db.Channel
		var useTLS, enabled int
		if err := rows.Scan(&c.ID, &c.OwnerUserID, &c.Name, &c.Type, &c.Provider, &c.Protocol, &c.Host, &c.Port, &c.Username, &c.PasswordEnc, &c.FromEmail, &useTLS, &c.Priority, &enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.UseTLS = useTLS == 1
		c.Enabled = enabled == 1
		out = append(out, c)
	}
	return out, nil
}
