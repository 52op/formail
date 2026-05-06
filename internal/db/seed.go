package db

import (
	"database/sql"
	"fmt"

	"formail/internal/utils"
)

func EnsureDefaultAdmin(db *sql.DB, username, password string) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := utils.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO users(username, password_hash, role, status) VALUES(?,?,'admin',1)`, username, hash)
	return err
}

func UpdateAdminAccount(db *sql.DB, username, password string) error {
	var id int
	var currentUsername string
	err := db.QueryRow(`SELECT id, username FROM users WHERE role='admin' ORDER BY id LIMIT 1`).Scan(&id, &currentUsername)
	if err != nil {
		return fmt.Errorf("未找到管理员账号: %w", err)
	}
	if username != "" && username != currentUsername {
		var cnt int
		if err := db.QueryRow(`SELECT COUNT(1) FROM users WHERE username=? AND id<>?`, username, id).Scan(&cnt); err != nil {
			return err
		}
		if cnt > 0 {
			return fmt.Errorf("用户名 %s 已被占用", username)
		}
		if _, err := db.Exec(`UPDATE users SET username=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, username, id); err != nil {
			return err
		}
	}
	if password != "" {
		hash, err := utils.HashPassword(password)
		if err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, hash, id); err != nil {
			return err
		}
	}
	return nil
}
