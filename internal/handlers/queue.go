package handlers

import (
	"strconv"

	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListMailQueue(c *gin.Context) {
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	where := "1=1"
	args := []interface{}{}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}

	var total int
	h.DB.QueryRow("SELECT COUNT(1) FROM mail_jobs WHERE "+where, args...).Scan(&total)

	query := "SELECT id,mail_to,subject,purpose,status,attempts,max_attempts,last_error,next_retry_at,created_at,updated_at FROM mail_jobs WHERE " + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	rows, err := h.DB.Query(query, append(args, limit, offset)...)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type MailJobView struct {
		ID          int64  `json:"id"`
		To          string `json:"mail_to"`
		Subject     string `json:"subject"`
		Purpose     string `json:"purpose"`
		Status      string `json:"status"`
		Attempts    int    `json:"attempts"`
		MaxAttempts int    `json:"max_attempts"`
		LastError   string `json:"last_error"`
		NextRetryAt string `json:"next_retry_at"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	items := make([]MailJobView, 0)
	for rows.Next() {
		var j MailJobView
		if err := rows.Scan(&j.ID, &j.To, &j.Subject, &j.Purpose, &j.Status, &j.Attempts, &j.MaxAttempts, &j.LastError, &j.NextRetryAt, &j.CreatedAt, &j.UpdatedAt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		items = append(items, j)
	}
	utils.OK(c, gin.H{"items": items, "total": total, "page": page, "limit": limit})
}

func (h *Handler) RetryMailJob(c *gin.Context) {
	id := c.Param("id")
	res, err := h.DB.Exec(`UPDATE mail_jobs SET status='pending', attempts=0, last_error='', next_retry_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='failed'`, id)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	aff, _ := res.RowsAffected()
	if aff == 0 {
		utils.Fail(c, 404, "任务不存在或状态不允许重试")
		return
	}
	utils.OK(c, gin.H{"retried": true})
}

func (h *Handler) DeleteMailJob(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.DB.Exec(`DELETE FROM mail_jobs WHERE id=?`, id); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"deleted": true})
}
