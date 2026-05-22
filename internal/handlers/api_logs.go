package handlers

import (
	"strconv"

	"formail/internal/db"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListAPILogs(c *gin.Context) {
	uid := h.currentUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	apiKeyID := c.Query("api_key_id")
	where := `WHERE l.user_id=?`
	args := []interface{}{uid}
	if apiKeyID != "" {
		where += ` AND l.api_key_id=?`
		args = append(args, apiKeyID)
	}

	var total int
	h.DB.QueryRow(`SELECT COUNT(1) FROM api_logs l `+where, args...).Scan(&total)

	args = append(args, limit, offset)
	rows, err := h.DB.Query(
		`SELECT l.id, l.api_key_id, k.name, l.user_id, l.to_email, l.subject, l.status, l.error, l.created_at
		 FROM api_logs l LEFT JOIN api_keys k ON k.id=l.api_key_id
		 `+where+` ORDER BY l.id DESC LIMIT ? OFFSET ?`,
		args...,
	)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type APILogRow struct {
		db.APILog
		KeyName string `json:"key_name"`
	}
	items := make([]APILogRow, 0)
	for rows.Next() {
		var r APILogRow
		var keyName string
		if err := rows.Scan(&r.ID, &r.APIKeyID, &keyName, &r.UserID, &r.To, &r.Subject, &r.Status, &r.Error, &r.CreatedAt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		r.KeyName = keyName
		items = append(items, r)
	}
	utils.OK(c, gin.H{"items": items, "total": total, "page": page})
}

func (h *Handler) StatsAPIOverview(c *gin.Context) {
	uid := h.currentUserID(c)

	var totalSent, totalFailed, todaySent int
	h.DB.QueryRow(`SELECT COUNT(1) FROM api_logs WHERE user_id=? AND status='success'`, uid).Scan(&totalSent)
	h.DB.QueryRow(`SELECT COUNT(1) FROM api_logs WHERE user_id=? AND status='failed'`, uid).Scan(&totalFailed)
	h.DB.QueryRow(`SELECT COUNT(1) FROM api_logs WHERE user_id=? AND status='success' AND DATE(created_at)=DATE('now')`, uid).Scan(&todaySent)

	utils.OK(c, gin.H{
		"total_sent":   totalSent,
		"total_failed": totalFailed,
		"today_sent":   todaySent,
	})
}
