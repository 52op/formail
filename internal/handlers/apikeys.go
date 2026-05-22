package handlers

import (
	"database/sql"
	"strings"

	"formail/internal/db"
	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) ListAPIKeys(c *gin.Context) {
	uid := h.currentUserID(c)
	rows, err := h.DB.Query(
		`SELECT id, user_id, name, key_prefix, key_enc, channel_id, created_at, last_used_at, enabled FROM api_keys WHERE user_id=? ORDER BY id DESC`,
		uid,
	)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	defer rows.Close()

	keys := make([]gin.H, 0)
	for rows.Next() {
		var k db.APIKey
		var enabled int
		var lastUsed sql.NullString
		var keyEnc string
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &keyEnc, &k.ChannelID, &k.CreatedAt, &lastUsed, &enabled); err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		k.Enabled = enabled == 1
		k.LastUsedAt = lastUsed.String

		// 解密完整 key 以供前端复制
		fullKey, _ := h.Cryptor.Decrypt(keyEnc)

		keys = append(keys, gin.H{
			"id":          k.ID,
			"user_id":     k.UserID,
			"name":        k.Name,
			"key_prefix":  k.KeyPrefix,
			"key":         fullKey,
			"channel_id":  k.ChannelID,
			"created_at":  k.CreatedAt,
			"last_used_at": k.LastUsedAt,
			"enabled":     k.Enabled,
		})
	}
	utils.OK(c, keys)
}

func (h *Handler) CreateAPIKey(c *gin.Context) {
	var req struct {
		Name      string `json:"name"`
		ChannelID int64  `json:"channel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		utils.Fail(c, 400, "name is required")
		return
	}

	uid := h.currentUserID(c)

	// 如果指定了渠道，校验归属
	if req.ChannelID > 0 {
		ok, err := h.channelBindableByUser(uid, req.ChannelID, h.currentUserRole(c))
		if err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		if !ok {
			utils.Fail(c, 400, "channel is not bindable for current user")
			return
		}
	}

	raw, err := utils.Token(30)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	fullKey := "fm_" + raw
	keyPrefix := fullKey[:10] + "..."
	keyHash := utils.SHA256Hex(fullKey)

	keyEnc, err := h.Cryptor.Encrypt(fullKey)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	_, err = h.DB.Exec(
		`INSERT INTO api_keys(user_id, name, key_prefix, key_hash, key_enc, channel_id, enabled) VALUES(?,?,?,?,?,?,1)`,
		uid, req.Name, keyPrefix, keyHash, keyEnc, req.ChannelID,
	)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}

	utils.OK(c, gin.H{
		"key":        fullKey,
		"key_prefix": keyPrefix,
		"name":       req.Name,
		"channel_id": req.ChannelID,
	})
}

func (h *Handler) UpdateAPIKey(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)
	var req struct {
		ChannelID int64 `json:"channel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request")
		return
	}
	if req.ChannelID > 0 {
		ok, err := h.channelBindableByUser(uid, req.ChannelID, h.currentUserRole(c))
		if err != nil {
			utils.Fail(c, 500, err.Error())
			return
		}
		if !ok {
			utils.Fail(c, 400, "channel is not bindable for current user")
			return
		}
	}
	_, err := h.DB.Exec(`UPDATE api_keys SET channel_id=? WHERE id=? AND user_id=?`, req.ChannelID, id, uid)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"updated": true})
}

func (h *Handler) DeleteAPIKey(c *gin.Context) {
	id := c.Param("id")
	uid := h.currentUserID(c)
	_, err := h.DB.Exec(`DELETE FROM api_keys WHERE id=? AND user_id=?`, id, uid)
	if err != nil {
		utils.Fail(c, 500, err.Error())
		return
	}
	utils.OK(c, gin.H{"deleted": true})
}
