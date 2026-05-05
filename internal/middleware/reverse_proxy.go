package middleware

import (
	"net/http"

	"formail/internal/config"

	"github.com/gin-gonic/gin"
)

func ReverseProxyGuard(cfg config.DirectAccessConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cfg.Allow {
			c.Next()
			return
		}
		if cfg.KeyName == "" || cfg.Key == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "direct access denied"})
			return
		}
		if c.GetHeader(cfg.KeyName) != cfg.Key {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "direct access denied"})
			return
		}
		c.Next()
	}
}
