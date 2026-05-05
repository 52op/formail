package middleware

import "github.com/gin-gonic/gin"

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" {
			c.AbortWithStatusJSON(403, gin.H{"code": 403, "message": "admin permission required"})
			return
		}
		c.Next()
	}
}
