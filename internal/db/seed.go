package db

import (
	"database/sql"

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
