package handlers

import (
	"database/sql"
	"strings"

	"formail/internal/db"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type userCreateReq struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Status      int    `json:"status"`
}

type userUpdateReq struct {
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Status      int    `json:"status"`
}

type resetPwdReq struct {
	NewPassword string `json:"new_password"`
}

func normalizeRole(role string) string {
	r := strings.TrimSpace(strings.ToLower(role))
	if r != "admin" {
		return "user"
	}
	return r
}

func normalizeStatus(status int) int {
	if status != 1 {
		return 0
	}
	return 1
}

func (h *Handler) ListUsers(c *gin.Context) {
	rows, err := h.DB.Query(`SELECT id,username,password_hash,role,display_name,email,status,created_at,updated_at FROM users ORDER BY id ASC`)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()
	users := make([]db.User, 0)
	for rows.Next() {
		var u db.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		users = append(users, u)
	}
	utils.OK(c, users)
}

func (h *Handler) CreateUser(c *gin.Context) {
	var req userCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	req.Username = strings.TrimSpace(strings.ToLower(req.Username))
	if !isValidEmail(req.Username) || len(req.Password) < 6 {
		utils.Fail(c, 400, "username must be a valid email and password must be >= 6")
		return
	}
	if strings.TrimSpace(req.Email) == "" {
		req.Email = req.Username
	}
	role := normalizeRole(req.Role)
	status := normalizeStatus(req.Status)
	hash, err := utils.HashPassword(req.Password)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	res, err := h.DB.Exec(`INSERT INTO users(username,password_hash,role,display_name,email,status) VALUES(?,?,?,?,?,?)`,
		req.Username, hash, role, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Email), status)
	if err != nil {
		utils.Fail(c, 400, "create user failed: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	utils.OK(c, gin.H{"id": id})
}

func (h *Handler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var req userUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	role := normalizeRole(req.Role)
	status := normalizeStatus(req.Status)

	var oldRole string
	if err := h.DB.QueryRow(`SELECT role FROM users WHERE id=?`, id).Scan(&oldRole); err == sql.ErrNoRows {
		utils.Fail(c, 404, "user not found")
		return
	} else if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if oldRole == "admin" && role != "admin" {
		var adminCount int
		if err := h.DB.QueryRow(`SELECT COUNT(1) FROM users WHERE role='admin'`).Scan(&adminCount); err == nil && adminCount <= 1 {
			utils.Fail(c, 400, "at least one admin is required")
			return
		}
	}

	if _, err := h.DB.Exec(`UPDATE users SET role=?,display_name=?,email=?,status=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		role, strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Email), status, id); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"updated": true})
}

func (h *Handler) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	curr := c.GetInt64("user_id")
	if id == "" {
		utils.Fail(c, 400, "id required")
		return
	}
	var targetID int64
	var targetRole string
	if err := h.DB.QueryRow(`SELECT id,role FROM users WHERE id=?`, id).Scan(&targetID, &targetRole); err == sql.ErrNoRows {
		utils.Fail(c, 404, "user not found")
		return
	} else if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if targetID == curr {
		utils.Fail(c, 400, "cannot delete current login user")
		return
	}
	if targetRole == "admin" {
		var adminCount int
		if err := h.DB.QueryRow(`SELECT COUNT(1) FROM users WHERE role='admin'`).Scan(&adminCount); err == nil && adminCount <= 1 {
			utils.Fail(c, 400, "cannot delete last admin")
			return
		}
	}
	if _, err := h.DB.Exec(`DELETE FROM users WHERE id=?`, id); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"deleted": true})
}

func (h *Handler) ApproveUser(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.DB.Exec(`UPDATE users SET status=1,updated_at=CURRENT_TIMESTAMP WHERE id=?`, id); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"approved": true})
}

func (h *Handler) RejectUser(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.DB.Exec(`DELETE FROM users WHERE id=? AND status=0`, id); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"rejected": true})
}

func (h *Handler) ResetUserPassword(c *gin.Context) {
	id := c.Param("id")
	var req resetPwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	if len(req.NewPassword) < 6 {
		utils.Fail(c, 400, "new password must be at least 6 chars")
		return
	}
	hash, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, hash, id); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"reset": true})
}
