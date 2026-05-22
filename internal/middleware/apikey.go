package middleware

import (
	"database/sql"
	"strings"

	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

func RequireAPIKey(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "unauthorized"})
			return
		}
		key := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if !strings.HasPrefix(key, "fm_") {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "unauthorized"})
			return
		}
		keyHash := utils.SHA256Hex(key)
		var keyID, userID int64
		var channelID int64
		var enabled int
		err := db.QueryRow(`SELECT id, user_id, channel_id, enabled FROM api_keys WHERE key_hash=?`, keyHash).
			Scan(&keyID, &userID, &channelID, &enabled)
		if err != nil || enabled != 1 {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "unauthorized"})
			return
		}
		go func() { //nolint
			_, _ = db.Exec(`UPDATE api_keys SET last_used_at=CURRENT_TIMESTAMP WHERE id=?`, keyID)
		}()
		c.Set("user_id", userID)
		c.Set("api_key_id", keyID)
		c.Set("api_channel_id", channelID)
		c.Next()
	}
}
