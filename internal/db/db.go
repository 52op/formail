package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := migrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			display_name TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			status INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS forms (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_user_id INTEGER NOT NULL DEFAULT 1,
			channel_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			token TEXT NOT NULL UNIQUE,
			recipient_email TEXT NOT NULL,
			success_redirect TEXT NOT NULL DEFAULT '',
			success_message TEXT NOT NULL DEFAULT '提交成功',
			auto_reply_enabled INTEGER NOT NULL DEFAULT 0,
			auto_reply_subject TEXT NOT NULL DEFAULT '',
			auto_reply_body TEXT NOT NULL DEFAULT '',
			email_subject_template TEXT NOT NULL DEFAULT '新表单提交: {{form_name}}',
			email_body_template TEXT NOT NULL DEFAULT '{{fields}}',
			honeypot_field TEXT NOT NULL DEFAULT '_gotcha',
			active INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_user_id INTEGER NOT NULL DEFAULT 1,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			provider TEXT NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'smtp',
			host TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 465,
			username TEXT NOT NULL DEFAULT '',
			password_enc TEXT NOT NULL DEFAULT '',
			from_email TEXT NOT NULL DEFAULT '',
			use_tls INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 100,
			enabled INTEGER NOT NULL DEFAULT 1,
			share_enabled INTEGER NOT NULL DEFAULT 0,
			share_max_bindings_per_user INTEGER NOT NULL DEFAULT 0,
			share_max_total_bindings INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			form_id INTEGER NOT NULL,
			ip TEXT NOT NULL,
			user_agent TEXT NOT NULL,
			data_enc TEXT NOT NULL,
			is_spam INTEGER NOT NULL DEFAULT 0,
			email_sent INTEGER NOT NULL DEFAULT 0,
			send_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(form_id) REFERENCES forms(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS email_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			submission_id INTEGER NOT NULL,
			channel_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS verification_codes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			purpose TEXT NOT NULL,
			code_hash TEXT NOT NULL,
			ip TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			used INTEGER NOT NULL DEFAULT 0,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS review_token_uses (
			token_id_hash TEXT PRIMARY KEY,
			used_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS captcha_challenges (
			id TEXT PRIMARY KEY,
			scene TEXT NOT NULL,
			answer_hash TEXT NOT NULL,
			ip TEXT NOT NULL,
			used INTEGER NOT NULL DEFAULT 0,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS code_request_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL,
			ip TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			purpose TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS mail_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mail_to TEXT NOT NULL,
			subject TEXT NOT NULL,
			body TEXT NOT NULL,
			purpose TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 5,
			last_error TEXT NOT NULL DEFAULT '',
			next_retry_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			key_prefix TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_enc TEXT NOT NULL DEFAULT '',
			channel_id INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME,
			enabled INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS api_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			api_key_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			to_email TEXT NOT NULL,
			subject TEXT NOT NULL,
			status TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_api_logs_user ON api_logs(user_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_api_logs_key ON api_logs(api_key_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_forms_token ON forms(token);`,
		`CREATE INDEX IF NOT EXISTS idx_channels_priority ON channels(priority, enabled);`,
		`CREATE INDEX IF NOT EXISTS idx_submissions_form_id ON submissions(form_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_verification_codes_email_purpose ON verification_codes(email, purpose, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_captcha_ip_created ON captcha_challenges(ip, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_code_request_logs_fp_created ON code_request_logs(fingerprint, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_mail_jobs_pick ON mail_jobs(status, next_retry_at, id);`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id, enabled);`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(key_hash);`,
	}

	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate failed: %w, stmt=%s", err, stmt)
		}
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('allow_register','0')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('register_default_status','1')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('admin_notify_email','')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('email_code_subject_template','Formail 验证码')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('email_code_body_template','你的验证码是: {{code}}\n用途: {{purpose}}\n{{ttl_minutes}}分钟内有效。若非本人操作请忽略。')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('email_signature','--\nFormail Team')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('email_code_cooldown_seconds','60')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('captcha_enabled','1')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('captcha_mode','click_text')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('captcha_ttl_seconds','180')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('captcha_failure_limit','5')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('fingerprint_max_requests_10m','30')`); err != nil {
		return err
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES('service_mail_channel_id','')`); err != nil {
		return err
	}
	if err := ensureUserColumns(db); err != nil {
		return err
	}
	if err := ensureOwnershipColumns(db); err != nil {
		return err
	}
	if err := ensureFormChannelColumns(db); err != nil {
		return err
	}
	if err := ensureChannelShareColumns(db); err != nil {
		return err
	}
	if err := ensureCaptchaChallengeColumns(db); err != nil {
		return err
	}
	if err := ensureOwnershipIndexes(db); err != nil {
		return err
	}
	if err := ensureAPIKeyColumns(db); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET role='user' WHERE role IS NULL OR role=''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET status=1 WHERE status IS NULL`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET created_at=CURRENT_TIMESTAMP WHERE created_at IS NULL OR created_at=''`); err != nil {
		return err
	}
	var adminCount int
	if err := db.QueryRow(`SELECT COUNT(1) FROM users WHERE role='admin'`).Scan(&adminCount); err != nil {
		return err
	}
	if adminCount == 0 {
		if _, err := db.Exec(`UPDATE users SET role='admin' WHERE id=(SELECT MIN(id) FROM users)`); err != nil {
			return err
		}
	}
	adminID, err := primaryAdminID(db)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // fresh database, EnsureDefaultAdmin will handle seeding
		}
		return err
	}
	if _, err := db.Exec(`UPDATE channels SET owner_user_id=? WHERE owner_user_id IS NULL OR owner_user_id<=0`, adminID); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE forms SET owner_user_id=? WHERE owner_user_id IS NULL OR owner_user_id<=0`, adminID); err != nil {
		return err
	}
	return nil
}

func ensureUserColumns(db *sql.DB) error {
	required := []struct {
		Name string
		DDL  string
	}{
		{Name: "role", DDL: `ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user'`},
		{Name: "display_name", DDL: `ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`},
		{Name: "email", DDL: `ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT ''`},
		{Name: "status", DDL: `ALTER TABLE users ADD COLUMN status INTEGER NOT NULL DEFAULT 1`},
		{Name: "created_at", DDL: `ALTER TABLE users ADD COLUMN created_at DATETIME`},
	}
	for _, col := range required {
		has, err := tableHasColumn(db, "users", col.Name)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(col.DDL); err != nil {
				return fmt.Errorf("add users.%s failed: %w", col.Name, err)
			}
		}
	}
	return nil
}

func ensureOwnershipColumns(db *sql.DB) error {
	required := []struct {
		Table string
		Name  string
		DDL   string
	}{
		{Table: "forms", Name: "owner_user_id", DDL: `ALTER TABLE forms ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1`},
		{Table: "channels", Name: "owner_user_id", DDL: `ALTER TABLE channels ADD COLUMN owner_user_id INTEGER NOT NULL DEFAULT 1`},
	}
	for _, col := range required {
		has, err := tableHasColumn(db, col.Table, col.Name)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(col.DDL); err != nil {
				return fmt.Errorf("add %s.%s failed: %w", col.Table, col.Name, err)
			}
		}
	}
	return nil
}

func ensureFormChannelColumns(db *sql.DB) error {
	has, err := tableHasColumn(db, "forms", "channel_id")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN channel_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add forms.channel_id failed: %w", err)
		}
	}
	has, err = tableHasColumn(db, "forms", "success_theme")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN success_theme TEXT NOT NULL DEFAULT 'blue'`); err != nil {
			return fmt.Errorf("add forms.success_theme failed: %w", err)
		}
	}
	has, err = tableHasColumn(db, "forms", "allowed_origins")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN allowed_origins TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add forms.allowed_origins failed: %w", err)
		}
	}
	has, err = tableHasColumn(db, "forms", "email_verified")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add forms.email_verified failed: %w", err)
		}
	}
	has, err = tableHasColumn(db, "forms", "verify_token")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN verify_token TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add forms.verify_token failed: %w", err)
		}
	}
	has, err = tableHasColumn(db, "forms", "webhook_url")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN webhook_url TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add forms.webhook_url failed: %w", err)
		}
	}
	has, err = tableHasColumn(db, "forms", "webhook_secret")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN webhook_secret TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add forms.webhook_secret failed: %w", err)
		}
	}
	has, err = tableHasColumn(db, "forms", "fields_schema")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE forms ADD COLUMN fields_schema TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add forms.fields_schema failed: %w", err)
		}
	}
	return nil
}

func ensureChannelShareColumns(db *sql.DB) error {
	required := []struct {
		Name string
		DDL  string
	}{
		{Name: "share_enabled", DDL: `ALTER TABLE channels ADD COLUMN share_enabled INTEGER NOT NULL DEFAULT 0`},
		{Name: "share_max_bindings_per_user", DDL: `ALTER TABLE channels ADD COLUMN share_max_bindings_per_user INTEGER NOT NULL DEFAULT 0`},
		{Name: "share_max_total_bindings", DDL: `ALTER TABLE channels ADD COLUMN share_max_total_bindings INTEGER NOT NULL DEFAULT 0`},
	}
	for _, col := range required {
		has, err := tableHasColumn(db, "channels", col.Name)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(col.DDL); err != nil {
				return fmt.Errorf("add channels.%s failed: %w", col.Name, err)
			}
		}
	}
	return nil
}

func ensureCaptchaChallengeColumns(db *sql.DB) error {
	has, err := tableHasColumn(db, "captcha_challenges", "attempts")
	if err != nil {
		return err
	}
	if !has {
		if _, err := db.Exec(`ALTER TABLE captcha_challenges ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add captcha_challenges.attempts failed: %w", err)
		}
	}
	return nil
}

func ensureOwnershipIndexes(db *sql.DB) error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_forms_owner ON forms(owner_user_id, id);`,
		`CREATE INDEX IF NOT EXISTS idx_forms_owner_channel ON forms(owner_user_id, channel_id, id);`,
		`CREATE INDEX IF NOT EXISTS idx_channels_owner_priority ON channels(owner_user_id, enabled, priority, id);`,
	}
	for _, stmt := range indexes {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create ownership index failed: %w, stmt=%s", err, stmt)
		}
	}
	return nil
}

func primaryAdminID(db *sql.DB) (int64, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM users WHERE role='admin' ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	err = db.QueryRow(`SELECT id FROM users ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

var allowedTables = map[string]bool{
	"users": true, "forms": true, "channels": true,
	"submissions": true, "captcha_challenges": true, "settings": true,
	"api_keys": true,
}

func ensureAPIKeyColumns(db *sql.DB) error {
	cols := []struct {
		Name string
		DDL  string
	}{
		{Name: "channel_id", DDL: `ALTER TABLE api_keys ADD COLUMN channel_id INTEGER NOT NULL DEFAULT 0`},
		{Name: "key_enc", DDL: `ALTER TABLE api_keys ADD COLUMN key_enc TEXT NOT NULL DEFAULT ''`},
	}
	for _, col := range cols {
		has, err := tableHasColumn(db, "api_keys", col.Name)
		if err != nil {
			return err
		}
		if !has {
			if _, err := db.Exec(col.DDL); err != nil {
				return fmt.Errorf("add api_keys.%s failed: %w", col.Name, err)
			}
		}
	}
	return nil
}

func tableHasColumn(db *sql.DB, table, column string) (bool, error) {
	if !allowedTables[table] {
		return false, fmt.Errorf("table %q not in allowed list", table)
	}
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	return false, nil
}
