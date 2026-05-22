package handlers

import (
	"strconv"

	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) StatsOverview(c *gin.Context) {
	uid := h.currentUserID(c)

	var totalForms, totalSubmissions, todaySubmissions, sentCount int
	var apiTotalSent, apiTodaySent int

	h.DB.QueryRow(`SELECT COUNT(1) FROM forms WHERE owner_user_id=?`, uid).Scan(&totalForms)
	h.DB.QueryRow(`SELECT COUNT(1) FROM submissions s JOIN forms f ON f.id=s.form_id WHERE f.owner_user_id=?`, uid).Scan(&totalSubmissions)
	h.DB.QueryRow(`SELECT COUNT(1) FROM submissions s JOIN forms f ON f.id=s.form_id WHERE f.owner_user_id=? AND DATE(s.created_at)=DATE('now')`, uid).Scan(&todaySubmissions)
	h.DB.QueryRow(`SELECT COUNT(1) FROM submissions s JOIN forms f ON f.id=s.form_id WHERE f.owner_user_id=? AND s.email_sent=1`, uid).Scan(&sentCount)
	h.DB.QueryRow(`SELECT COUNT(1) FROM api_logs WHERE user_id=? AND status='success'`, uid).Scan(&apiTotalSent)
	h.DB.QueryRow(`SELECT COUNT(1) FROM api_logs WHERE user_id=? AND status='success' AND DATE(created_at)=DATE('now')`, uid).Scan(&apiTodaySent)

	sendRate := 0.0
	if totalSubmissions > 0 {
		sendRate = float64(sentCount) / float64(totalSubmissions) * 100
	}

	utils.OK(c, gin.H{
		"total_forms":        totalForms,
		"total_submissions":  totalSubmissions,
		"today_submissions":  todaySubmissions,
		"send_rate":          sendRate,
		"sent_count":         sentCount,
		"api_total_sent":     apiTotalSent,
		"api_today_sent":     apiTodaySent,
	})
}

func (h *Handler) StatsDaily(c *gin.Context) {
	uid := h.currentUserID(c)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	if days < 1 || days > 365 {
		days = 30
	}

	rows, err := h.DB.Query(`SELECT DATE(s.created_at) as date, COUNT(1) as total, SUM(CASE WHEN s.email_sent=1 THEN 1 ELSE 0 END) as sent FROM submissions s JOIN forms f ON f.id=s.form_id WHERE f.owner_user_id=? AND s.created_at >= DATE('now', ?) GROUP BY DATE(s.created_at) ORDER BY date`, uid, "-"+strconv.Itoa(days)+" days")
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type DayStat struct {
		Date   string `json:"date"`
		Total  int    `json:"total"`
		Sent   int    `json:"sent"`
	}
	items := make([]DayStat, 0)
	for rows.Next() {
		var d DayStat
		if err := rows.Scan(&d.Date, &d.Total, &d.Sent); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		items = append(items, d)
	}
	utils.OK(c, items)
}

func (h *Handler) StatsForms(c *gin.Context) {
	uid := h.currentUserID(c)

	rows, err := h.DB.Query(`SELECT f.id, f.name, COUNT(s.id) as cnt FROM forms f LEFT JOIN submissions s ON s.form_id=f.id WHERE f.owner_user_id=? GROUP BY f.id ORDER BY cnt DESC LIMIT 10`, uid)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type FormStat struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	items := make([]FormStat, 0)
	for rows.Next() {
		var fs FormStat
		if err := rows.Scan(&fs.ID, &fs.Name, &fs.Count); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		items = append(items, fs)
	}
	utils.OK(c, items)
}

func (h *Handler) StatsChannels(c *gin.Context) {
	uid := h.currentUserID(c)

	rows, err := h.DB.Query(`SELECT c.id, c.name, COUNT(el.id) as total, SUM(CASE WHEN el.status='success' THEN 1 ELSE 0 END) as sent FROM channels c LEFT JOIN forms f ON f.channel_id=c.id LEFT JOIN submissions s ON s.form_id=f.id LEFT JOIN email_logs el ON el.submission_id=s.id WHERE c.owner_user_id=? GROUP BY c.id ORDER BY total DESC`, uid)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	type ChannelStat struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Total int    `json:"total"`
		Sent  int    `json:"sent"`
	}
	items := make([]ChannelStat, 0)
	for rows.Next() {
		var cs ChannelStat
		if err := rows.Scan(&cs.ID, &cs.Name, &cs.Total, &cs.Sent); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		items = append(items, cs)
	}
	utils.OK(c, items)
}
