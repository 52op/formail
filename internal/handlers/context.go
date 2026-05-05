package handlers

import (
	"database/sql"

	"formail/internal/config"
	"formail/internal/services"
	"formail/internal/utils"
)

type Handler struct {
	DB        *sql.DB
	Cfg       config.Config
	Cryptor   *utils.Cryptor
	Mailer    *services.MailerService
	MailQueue *services.MailQueueService
	Spam      services.SpamChecker
}

func New(db *sql.DB, cfg config.Config) *Handler {
	cryptor := utils.NewCryptor(cfg.Security.EncryptionKey)
	mailer := &services.MailerService{DB: db, Cryptor: cryptor}
	mailQueue := &services.MailQueueService{DB: db}
	return &Handler{
		DB:        db,
		Cfg:       cfg,
		Cryptor:   cryptor,
		Mailer:    mailer,
		MailQueue: mailQueue,
		Spam:      services.SpamChecker{BlockedKeywords: cfg.Spam.BlockedKeywords},
	}
}
