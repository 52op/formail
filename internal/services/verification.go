package services

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

func GenerateNumericCode(n int) (string, error) {
	if n <= 0 {
		n = 6
	}
	max := 1
	for i := 0; i < n; i++ {
		max *= 10
	}
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	v := int(buf[0])<<24 | int(buf[1])<<16 | int(buf[2])<<8 | int(buf[3])
	if v < 0 {
		v = -v
	}
	num := v % max
	return fmt.Sprintf("%0*d", n, num), nil
}

func HashCode(code string) string {
	h := sha256.Sum256([]byte(code))
	return hex.EncodeToString(h[:])
}

func CreateVerificationCode(db *sql.DB, email, purpose, code, ip string, ttlMinutes int) error {
	if ttlMinutes <= 0 {
		ttlMinutes = 10
	}
	expiresAt := time.Now().Add(time.Duration(ttlMinutes) * time.Minute).Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO verification_codes(email,purpose,code_hash,ip,expires_at) VALUES(?,?,?,?,?)`,
		email, purpose, HashCode(code), ip, expiresAt)
	return err
}

func TooManyRecentCodes(db *sql.DB, email, ip string) (bool, error) {
	var emailCnt, ipCnt int
	if err := db.QueryRow(`SELECT COUNT(1) FROM verification_codes WHERE email=? AND created_at >= datetime('now','-10 minutes')`, email).Scan(&emailCnt); err != nil {
		return false, err
	}
	if err := db.QueryRow(`SELECT COUNT(1) FROM verification_codes WHERE ip=? AND created_at >= datetime('now','-10 minutes')`, ip).Scan(&ipCnt); err != nil {
		return false, err
	}
	return emailCnt >= 5 || ipCnt >= 20, nil
}

func VerifyCodeOnce(db *sql.DB, email, purpose, code string, maxAttempts int) (bool, error) {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	rows, err := db.Query(`SELECT id,code_hash,attempts,expires_at,used FROM verification_codes WHERE email=? AND purpose=? ORDER BY id DESC LIMIT 10`, email, purpose)
	if err != nil {
		return false, err
	}

	now := time.Now()
	inHash := HashCode(code)
	var matched bool
	var targetID int64
	var nextAttempts int
	var shouldUpdate bool

	for rows.Next() {
		var id int64
		var codeHash, expiresAt string
		var attempts, used int
		if err := rows.Scan(&id, &codeHash, &attempts, &expiresAt, &used); err != nil {
			_ = rows.Close()
			return false, err
		}
		if used == 1 {
			continue
		}
		exp, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			continue
		}
		if now.After(exp) {
			continue
		}
		if attempts >= maxAttempts {
			continue
		}

		targetID = id
		if codeHash == inHash {
			matched = true
			shouldUpdate = true
			break
		}
		nextAttempts = attempts + 1
		shouldUpdate = true
		break
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}

	if !shouldUpdate {
		return false, nil
	}
	if matched {
		if _, err := db.Exec(`UPDATE verification_codes SET used=1 WHERE id=?`, targetID); err != nil {
			return false, err
		}
		return true, nil
	}
	if _, err := db.Exec(`UPDATE verification_codes SET attempts=? WHERE id=?`, nextAttempts, targetID); err != nil {
		return false, err
	}
	return false, nil
}
