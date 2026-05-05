package handlers

import (
	"database/sql"
	"strconv"
	"strings"

	"formail/internal/db"
	"formail/internal/services"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type channelReq struct {
	Name                    string `json:"name"`
	Type                    string `json:"type"`
	Provider                string `json:"provider"`
	Protocol                string `json:"protocol"`
	Host                    string `json:"host"`
	Port                    int    `json:"port"`
	Username                string `json:"username"`
	Password                string `json:"password"`
	FromEmail               string `json:"from_email"`
	UseTLS                  bool   `json:"use_tls"`
	Priority                int    `json:"priority"`
	Enabled                 bool   `json:"enabled"`
	ShareEnabled            bool   `json:"share_enabled"`
	ShareMaxBindingsPerUser int    `json:"share_max_bindings_per_user"`
	ShareMaxTotalBindings   int    `json:"share_max_total_bindings"`
}

type bindableChannelItem struct {
	ID                      int64  `json:"id"`
	Name                    string `json:"name"`
	OwnerUserID             int64  `json:"owner_user_id"`
	OwnerScope              string `json:"owner_scope"`
	Protocol                string `json:"protocol"`
	Enabled                 bool   `json:"enabled"`
	Priority                int    `json:"priority"`
	ShareEnabled            bool   `json:"share_enabled"`
	ShareMaxBindingsPerUser int    `json:"share_max_bindings_per_user"`
	ShareMaxTotalBindings   int    `json:"share_max_total_bindings"`
	CurrentUserBindings     int    `json:"current_user_bindings"`
	CurrentTotalShared      int    `json:"current_total_shared"`
	CanUse                  bool   `json:"can_use"`
	Reason                  string `json:"reason,omitempty"`
}

func (h *Handler) ListChannels(c *gin.Context) {
	uid := h.currentUserID(c)
	rows, err := h.DB.Query(`SELECT id,owner_user_id,name,type,provider,protocol,host,port,username,password_enc,from_email,use_tls,priority,enabled,share_enabled,share_max_bindings_per_user,share_max_total_bindings,created_at,updated_at FROM channels WHERE owner_user_id=? ORDER BY priority ASC,id DESC`, uid)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()
	list := []db.Channel{}
	for rows.Next() {
		var x db.Channel
		var useTLS, enabled, shareEnabled int
		if err := rows.Scan(&x.ID, &x.OwnerUserID, &x.Name, &x.Type, &x.Provider, &x.Protocol, &x.Host, &x.Port, &x.Username, &x.PasswordEnc, &x.FromEmail, &useTLS, &x.Priority, &enabled, &shareEnabled, &x.ShareMaxBindingsPerUser, &x.ShareMaxTotalBindings, &x.CreatedAt, &x.UpdatedAt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		x.UseTLS = useTLS == 1
		x.Enabled = enabled == 1
		x.ShareEnabled = shareEnabled == 1
		list = append(list, x)
	}
	utils.OK(c, list)
}

func (h *Handler) ListBindableChannels(c *gin.Context) {
	uid := h.currentUserID(c)
	role := h.currentUserRole(c)
	adminID, err := h.primaryAdminUserID()
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	rows, err := h.DB.Query(`SELECT id,owner_user_id,name,protocol,enabled,priority,share_enabled,share_max_bindings_per_user,share_max_total_bindings FROM channels WHERE enabled=1 ORDER BY priority ASC,id DESC`)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	raw := make([]bindableChannelItem, 0)
	for rows.Next() {
		var it bindableChannelItem
		var enabled, shareEnabled int
		if err := rows.Scan(&it.ID, &it.OwnerUserID, &it.Name, &it.Protocol, &enabled, &it.Priority, &shareEnabled, &it.ShareMaxBindingsPerUser, &it.ShareMaxTotalBindings); err != nil {
			_ = rows.Close()
			utils.Fail(c, 500, err.Error())
			return
		}
		it.Enabled = enabled == 1
		it.ShareEnabled = shareEnabled == 1
		raw = append(raw, it)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		utils.Fail(c, 500, err.Error())
		return
	}
	if err := rows.Close(); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	items := make([]bindableChannelItem, 0, len(raw))
	for _, it := range raw {
		if it.OwnerUserID == uid {
			it.OwnerScope = "self"
			it.CanUse = true
			items = append(items, it)
			continue
		}
		if role == "admin" {
			it.OwnerScope = "other"
			it.CanUse = false
			it.Reason = "admin only uses own channels"
			continue
		}
		if it.OwnerUserID != adminID {
			continue
		}
		it.OwnerScope = "admin_shared"
		if !it.ShareEnabled {
			it.CanUse = false
			it.Reason = "not shared by admin"
			continue
		}
		var userBindCnt int
		if err := h.DB.QueryRow(`SELECT COUNT(1) FROM forms WHERE owner_user_id=? AND channel_id=?`, uid, it.ID).Scan(&userBindCnt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		it.CurrentUserBindings = userBindCnt
		var totalSharedCnt int
		if err := h.DB.QueryRow(`SELECT COUNT(1) FROM forms WHERE channel_id=? AND owner_user_id<>?`, it.ID, it.OwnerUserID).Scan(&totalSharedCnt); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		it.CurrentTotalShared = totalSharedCnt
		if it.ShareMaxBindingsPerUser > 0 && userBindCnt >= it.ShareMaxBindingsPerUser {
			it.CanUse = false
			it.Reason = "per-user binding limit reached"
			items = append(items, it)
			continue
		}
		if it.ShareMaxTotalBindings > 0 && totalSharedCnt >= it.ShareMaxTotalBindings {
			it.CanUse = false
			it.Reason = "total shared binding limit reached"
			items = append(items, it)
			continue
		}
		it.CanUse = true
		items = append(items, it)
	}
	utils.OK(c, items)
}

func validateChannel(req *channelReq) string {
	if strings.TrimSpace(req.Name) == "" {
		return "name required"
	}
	if req.Type == "" {
		req.Type = "custom"
	}
	if req.Protocol == "" {
		req.Protocol = "smtp"
	}
	if req.Priority <= 0 {
		req.Priority = 100
	}
	if req.Type == "builtin" && req.Provider == "" {
		return "provider required for builtin channel"
	}
	if strings.TrimSpace(req.Username) == "" {
		return "username required"
	}
	if strings.TrimSpace(req.Password) == "" {
		return "password required"
	}
	if req.ShareMaxBindingsPerUser < 0 {
		req.ShareMaxBindingsPerUser = 0
	}
	if req.ShareMaxTotalBindings < 0 {
		req.ShareMaxTotalBindings = 0
	}
	return ""
}

func (h *Handler) CreateChannel(c *gin.Context) {
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	if msg := validateChannel(&req); msg != "" {
		utils.Fail(c, 400, msg)
		return
	}
	encPwd, err := h.Cryptor.Encrypt(req.Password)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	useTLS := 0
	if req.UseTLS {
		useTLS = 1
	}
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	shareEnabled := 0
	if req.ShareEnabled && h.currentUserIsAdmin(c) {
		shareEnabled = 1
	}
	uid := h.currentUserID(c)
	res, err := h.DB.Exec(`INSERT INTO channels(owner_user_id,name,type,provider,protocol,host,port,username,password_enc,from_email,use_tls,priority,enabled,share_enabled,share_max_bindings_per_user,share_max_total_bindings) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, req.Name, req.Type, req.Provider, req.Protocol, req.Host, req.Port, req.Username, encPwd, req.FromEmail, useTLS, req.Priority, enabled, shareEnabled, req.ShareMaxBindingsPerUser, req.ShareMaxTotalBindings)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	id, _ := res.LastInsertId()
	utils.OK(c, gin.H{"id": id})
}

func (h *Handler) UpdateChannel(c *gin.Context) {
	id := c.Param("id")
	var req channelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		utils.Fail(c, 400, "name required")
		return
	}
	if req.ShareMaxBindingsPerUser < 0 {
		req.ShareMaxBindingsPerUser = 0
	}
	if req.ShareMaxTotalBindings < 0 {
		req.ShareMaxTotalBindings = 0
	}
	uid := h.currentUserID(c)
	var oldPwdEnc string
	if err := h.DB.QueryRow(`SELECT password_enc FROM channels WHERE id=? AND owner_user_id=?`, id, uid).Scan(&oldPwdEnc); err != nil {
		utils.Fail(c, 404, "channel not found")
		return
	}
	pwdEnc := oldPwdEnc
	if strings.TrimSpace(req.Password) != "" {
		enc, err := h.Cryptor.Encrypt(req.Password)
		if err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		pwdEnc = enc
	}
	useTLS := 0
	if req.UseTLS {
		useTLS = 1
	}
	enabled := 0
	if req.Enabled {
		enabled = 1
	}
	shareEnabled := 0
	if req.ShareEnabled && h.currentUserIsAdmin(c) {
		shareEnabled = 1
	}
	if !h.currentUserIsAdmin(c) {
		req.ShareMaxBindingsPerUser = 0
		req.ShareMaxTotalBindings = 0
	}
	if _, err := h.DB.Exec(`UPDATE channels SET name=?,type=?,provider=?,protocol=?,host=?,port=?,username=?,password_enc=?,from_email=?,use_tls=?,priority=?,enabled=?,share_enabled=?,share_max_bindings_per_user=?,share_max_total_bindings=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND owner_user_id=?`,
		req.Name, req.Type, req.Provider, req.Protocol, req.Host, req.Port, req.Username, pwdEnc, req.FromEmail, useTLS, req.Priority, enabled, shareEnabled, req.ShareMaxBindingsPerUser, req.ShareMaxTotalBindings, id, uid); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"updated": true})
}

func (h *Handler) DeleteChannel(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)
	cid, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		utils.Fail(c, 400, "invalid id")
		return
	}
	if _, err := h.DB.Exec(`UPDATE forms SET channel_id=0 WHERE channel_id=?`, cid); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	if _, err := h.DB.Exec(`DELETE FROM channels WHERE id=? AND owner_user_id=?`, cid, uid); err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"deleted": true})
}

func (h *Handler) TestChannel(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)
	var ch db.Channel
	var useTLS, enabled, shareEnabled int
	err := h.DB.QueryRow(`SELECT id,owner_user_id,name,type,provider,protocol,host,port,username,password_enc,from_email,use_tls,priority,enabled,share_enabled,share_max_bindings_per_user,share_max_total_bindings,created_at,updated_at FROM channels WHERE id=? AND owner_user_id=?`, id, uid).
		Scan(&ch.ID, &ch.OwnerUserID, &ch.Name, &ch.Type, &ch.Provider, &ch.Protocol, &ch.Host, &ch.Port, &ch.Username, &ch.PasswordEnc, &ch.FromEmail, &useTLS, &ch.Priority, &enabled, &shareEnabled, &ch.ShareMaxBindingsPerUser, &ch.ShareMaxTotalBindings, &ch.CreatedAt, &ch.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			utils.Fail(c, 404, "channel not found")
			return
		}
		utils.Fail(c, 500, err.Error())
		return
	}
	ch.UseTLS = useTLS == 1
	ch.Enabled = enabled == 1
	ch.ShareEnabled = shareEnabled == 1
	to := ch.FromEmail
	if to == "" {
		to = ch.Username
	}
	if strings.ToLower(ch.Protocol) == "imap" {
		if err := services.TestIMAPConnectivity(ch.Host, ch.Port, ch.UseTLS); err != nil {
			utils.Fail(c, 400, "imap test failed: "+err.Error())
			return
		}
		utils.OK(c, gin.H{"success": true, "message": "imap connectivity ok"})
		return
	}

	if err := h.Mailer.SendWithOneChannel(ch, to, "Formail Channel Test", "This is a test email from Formail."); err != nil {
		utils.Fail(c, 400, "test failed: "+err.Error())
		return
	}
	utils.OK(c, gin.H{"success": true})
}
