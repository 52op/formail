package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SignChallenge 为表单提交签发短时效签名令牌（fc_token）。
// 用途：证明提交曾加载过表单页面（跑过页面 JS），拦截直连 POST 的机器人。
// 格式：base64url(raw) + "." + base64url(sig)
// raw := token|exp|nonce|ip
// sig := HMAC-SHA256(key, raw)
func SignChallenge(key, formToken, ip string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	exp := time.Now().Add(ttl).Unix()
	raw := fmt.Sprintf("%s|%d|%s|%s", formToken, exp, base64.RawURLEncoding.EncodeToString(nonce), ip)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(raw))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + "." + sig, nil
}

// VerifyChallenge 校验 fc_token。
// 校验内容：格式、签名、未过期（宽容 ±60s）、IP 一致、表单 token 一致。
func VerifyChallenge(key, formToken, ip, fcToken string) bool {
	if fcToken == "" {
		return false
	}
	parts := strings.SplitN(fcToken, ".", 2)
	if len(parts) != 2 {
		return false
	}
	rawBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	raw := string(rawBytes)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(raw))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
	if !hmac.Equal([]byte(parts[1]), []byte(want)) {
		return false
	}

	segs := strings.SplitN(raw, "|", 4)
	if len(segs) != 4 {
		return false
	}
	token, expStr, _, tokIP := segs[0], segs[1], segs[2], segs[3]
	if token != formToken {
		return false
	}
	// IP 匹配：容忍空 IP（如无 JS 场景不签发），非空必须一致
	if tokIP != "" && tokIP != ip {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return false
	}
	// 允许 60s 时钟偏差
	if time.Now().Unix() > exp+60 {
		return false
	}
	return true
}