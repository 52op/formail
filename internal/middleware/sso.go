package middleware

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ssoClaims GoAuth JWT 载荷结构
type ssoClaims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// ParseRSAPublicKey 从 PEM 字符串解析 RSA 公钥（支持 \n 转义）
func ParseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	// 支持配置文件中用 \n 表示换行
	pemStr = strings.ReplaceAll(pemStr, `\n`, "\n")
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("failed to decode PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("not an RSA public key")
	}
	return rsaPub, nil
}

// verifyRS256Token 使用 RSA 公钥验证 JWT，返回 claims
func verifyRS256Token(tokenStr string, pub *rsa.PublicKey, issuer string) (*ssoClaims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &ssoClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return pub, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := t.Claims.(*ssoClaims)
	if !ok || !t.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token expired")
	}
	if issuer != "" && claims.Issuer != issuer {
		return nil, errors.New("invalid issuer")
	}
	return claims, nil
}

// extractBearerToken 从请求头或 Cookie 提取 token
func extractBearerToken(c *gin.Context, cookieName string) string {
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if cookieName != "" {
		if val, err := c.Cookie(cookieName); err == nil {
			return val
		}
	}
	return ""
}

// RequireAuthSSO SSO 模式下的认证中间件（用 GoAuth 公钥验证 RS256 JWT）
func RequireAuthSSO(pub *rsa.PublicKey, cookieName string, issuer string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractBearerToken(c, cookieName)
		if tokenStr == "" {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "unauthorized"})
			return
		}
		claims, err := verifyRS256Token(tokenStr, pub, issuer)
		if err != nil {
			c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": "invalid token"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}
