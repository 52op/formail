package utils

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ReviewClaims struct {
	TokenID string `json:"token_id"`
	UserID  int64  `json:"user_id"`
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

func GenerateReviewToken(secret string, userID int64, purpose string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	tokenID := SHA256Hex(purpose + ":" + time.Now().Format(time.RFC3339Nano) + ":" + strconv.FormatInt(userID, 10))
	claims := ReviewClaims{
		TokenID: tokenID,
		UserID:  userID,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

func ParseReviewToken(secret, token string) (*ReviewClaims, error) {
	t, err := jwt.ParseWithClaims(token, &ReviewClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("invalid jwt signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := t.Claims.(*ReviewClaims)
	if !ok || !t.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
