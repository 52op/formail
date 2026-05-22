package handlers

import (
	"encoding/json"
	"strings"

	"formail/internal/utils"

	"github.com/gin-gonic/gin"
)

type emailAPIReq struct {
	To      json.RawMessage `json:"to"`
	Subject string          `json:"subject"`
	Text    string          `json:"text"`
	HTML    string          `json:"html"`
}

func parseEmailTo(raw json.RawMessage) ([]string, error) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var multi []string
	if err := json.Unmarshal(raw, &multi); err != nil {
		return nil, err
	}
	return multi, nil
}

func (h *Handler) SendEmailAPI(c *gin.Context) {
	var req emailAPIReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Fail(c, 400, "invalid request body: "+err.Error())
		return
	}

	recipients, err := parseEmailTo(req.To)
	if err != nil || len(recipients) == 0 {
		utils.Fail(c, 400, "to is required")
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" {
		utils.Fail(c, 400, "subject is required")
		return
	}
	if strings.TrimSpace(req.Text) == "" && strings.TrimSpace(req.HTML) == "" {
		utils.Fail(c, 400, "text or html is required")
		return
	}

	uid := h.currentUserID(c)
	apiKeyID := c.GetInt64("api_key_id")
	apiChannelID := c.GetInt64("api_channel_id")

	var failedList []string
	for _, to := range recipients {
		to = strings.TrimSpace(to)
		if !isValidEmail(to) {
			h.writeAPILog(apiKeyID, uid, to, req.Subject, "failed", "invalid address")
			failedList = append(failedList, to+": invalid address")
			continue
		}

		var sendErr string
		if apiChannelID > 0 {
			ch, err := h.getChannelByID(apiChannelID)
			if err == nil && ch.Enabled {
				if e := h.Mailer.SendMessageWithOneChannel(ch, to, req.Subject, req.Text, req.HTML); e != nil {
					sendErr = e.Error()
				}
			} else {
				sendErr = "bound channel unavailable"
			}
		}

		if sendErr != "" {
			// 绑定渠道失败，降级到自动选择
			ok, errMsg := h.Mailer.SendMessageWithFallbackByOwner(uid, to, req.Subject, req.Text, req.HTML)
			if !ok {
				h.writeAPILog(apiKeyID, uid, to, req.Subject, "failed", sendErr+"; fallback: "+errMsg)
				failedList = append(failedList, to+": "+errMsg)
				continue
			}
		} else if apiChannelID <= 0 {
			ok, errMsg := h.Mailer.SendMessageWithFallbackByOwner(uid, to, req.Subject, req.Text, req.HTML)
			if !ok {
				h.writeAPILog(apiKeyID, uid, to, req.Subject, "failed", errMsg)
				failedList = append(failedList, to+": "+errMsg)
				continue
			}
		}

		h.writeAPILog(apiKeyID, uid, to, req.Subject, "success", "")
	}

	if len(failedList) > 0 {
		utils.Fail(c, 500, "some emails failed: "+strings.Join(failedList, "; "))
		return
	}

	utils.OK(c, gin.H{"message": "sent"})
}

func (h *Handler) writeAPILog(apiKeyID, userID int64, to, subject, status, errMsg string) {
	_, _ = h.DB.Exec(
		`INSERT INTO api_logs(api_key_id, user_id, to_email, subject, status, error) VALUES(?,?,?,?,?,?)`,
		apiKeyID, userID, to, subject, status, errMsg,
	)
}
