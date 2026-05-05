package handlers

import (
	"database/sql"
	"strings"

	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePwdReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *Handler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "请求格式无效")
		return
	}
	inputUsername := strings.TrimSpace(req.Username)
	if inputUsername == "" || req.Password == "" {
		utils.Fail(c, 400, "请输入用户名和密码")
		return
	}
	lookupUsername := inputUsername
	if isValidEmail(inputUsername) {
		lookupUsername = strings.ToLower(inputUsername)
	}

	var id int64
	var dbUsername, hash, role string
	var status int
	err := h.DB.QueryRow(`SELECT id,username,password_hash,role,status FROM users WHERE username=?`, lookupUsername).Scan(&id, &dbUsername, &hash, &role, &status)
	if err == sql.ErrNoRows {
		utils.Fail(c, 401, "用户名或密码错误")
		return
	}
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if !utils.VerifyPassword(hash, req.Password) {
		utils.Fail(c, 401, "用户名或密码错误")
		return
	}
	if status != 1 {
		utils.Fail(c, 403, "账号已禁用")
		return
	}
	token, err := utils.GenerateToken(h.Cfg.Security.JWTSecret, id, dbUsername, role)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"token": token, "username": dbUsername, "role": role})
}

func (h *Handler) Me(c *gin.Context) {
	utils.OK(c, gin.H{
		"user_id":  c.GetInt64("user_id"),
		"username": c.GetString("username"),
		"role":     c.GetString("role"),
	})
}

func (h *Handler) ChangePassword(c *gin.Context) {
	var req changePwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "请求格式无效")
		return
	}
	if len(req.NewPassword) < 6 {
		utils.Fail(c, 400, "新密码长度至少6位")
		return
	}
	uid := c.GetInt64("user_id")

	var hash string
	if err := h.DB.QueryRow(`SELECT password_hash FROM users WHERE id=?`, uid).Scan(&hash); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if !utils.VerifyPassword(hash, req.OldPassword) {
		utils.Fail(c, 400, "旧密码不正确")
		return
	}
	newHash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, newHash, uid); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"changed": true})
}

func (h *Handler) RegStatus(c *gin.Context) {
	email := strings.TrimSpace(strings.ToLower(c.Query("email")))
	if !isValidEmail(email) {
		utils.Fail(c, 400, "请输入有效的邮箱地址")
		return
	}
	var status int
	err := h.DB.QueryRow(`SELECT status FROM users WHERE username=?`, email).Scan(&status)
	if err == sql.ErrNoRows {
		utils.OK(c, gin.H{"status": "not_found"})
		return
	}
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if status == 1 {
		utils.OK(c, gin.H{"status": "approved"})
		return
	}
	utils.OK(c, gin.H{"status": "pending"})
}
