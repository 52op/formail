package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

type MailQueueService struct {
	DB     *sql.DB
	Sender func(to, subject, body string) error
}

type MailJob struct {
	ID          int64
	To          string
	Subject     string
	Body        string
	Purpose     string
	Attempts    int
	MaxAttempts int
}

func (q *MailQueueService) Enqueue(to, subject, body, purpose string) (int64, error) {
	to = strings.TrimSpace(to)
	subject = strings.TrimSpace(subject)
	if to == "" || subject == "" || strings.TrimSpace(body) == "" {
		return 0, fmt.Errorf("invalid mail job payload")
	}
	res, err := q.DB.Exec(`INSERT INTO mail_jobs(mail_to,subject,body,purpose,status,attempts,max_attempts,last_error,next_retry_at,updated_at) VALUES(?,?,?,?, 'pending',0,5,'',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, to, subject, body, strings.TrimSpace(purpose))
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (q *MailQueueService) Start(workerCount int, pollInterval time.Duration, stopCh <-chan struct{}) {
	if workerCount <= 0 {
		workerCount = 1
	}
	if pollInterval <= 0 {
		pollInterval = 800 * time.Millisecond
	}
	for i := 0; i < workerCount; i++ {
		go q.workerLoop(i+1, pollInterval, stopCh)
	}
}

func (q *MailQueueService) workerLoop(workerID int, pollInterval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			job, err := q.claimNextJob()
			if err != nil {
				if !errors.Is(err, sql.ErrNoRows) {
					log.Printf("mail queue worker %d claim error: %v", workerID, err)
				}
				continue
			}
			if job == nil {
				continue
			}
			if q.Sender == nil {
				_ = q.markJobRetry(job.ID, job.Attempts+1, job.MaxAttempts, "mail sender not configured")
				continue
			}
			if err := q.Sender(job.To, job.Subject, job.Body); err != nil {
				_ = q.markJobRetry(job.ID, job.Attempts+1, job.MaxAttempts, err.Error())
				continue
			}
			_ = q.markJobDone(job.ID)
		}
	}
}

func (q *MailQueueService) claimNextJob() (*MailJob, error) {
	tx, err := q.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRow(`SELECT id,mail_to,subject,body,purpose,attempts,max_attempts FROM mail_jobs WHERE status='pending' AND datetime(next_retry_at) <= datetime('now') ORDER BY id ASC LIMIT 1`)
	var j MailJob
	if err := row.Scan(&j.ID, &j.To, &j.Subject, &j.Body, &j.Purpose, &j.Attempts, &j.MaxAttempts); err != nil {
		if err == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}

	res, err := tx.Exec(`UPDATE mail_jobs SET status='sending', updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='pending'`, j.ID)
	if err != nil {
		return nil, err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if aff == 0 {
		return nil, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &j, nil
}

func (q *MailQueueService) markJobDone(id int64) error {
	_, err := q.DB.Exec(`UPDATE mail_jobs SET status='done', last_error='', updated_at=CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

func retryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 15 * time.Second
	}
	if attempt == 2 {
		return 30 * time.Second
	}
	if attempt == 3 {
		return 60 * time.Second
	}
	if attempt == 4 {
		return 2 * time.Minute
	}
	return 5 * time.Minute
}

func (q *MailQueueService) markJobRetry(id int64, attempts, maxAttempts int, errMsg string) error {
	errMsg = strings.TrimSpace(errMsg)
	if errMsg == "" {
		errMsg = "unknown send error"
	}
	if attempts >= maxAttempts {
		_, err := q.DB.Exec(`UPDATE mail_jobs SET status='failed', attempts=?, last_error=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, attempts, errMsg, id)
		return err
	}
	d := retryDelay(attempts)
	_, err := q.DB.Exec(`UPDATE mail_jobs SET status='pending', attempts=?, last_error=?, next_retry_at=datetime('now', ?), updated_at=CURRENT_TIMESTAMP WHERE id=?`, attempts, errMsg, fmt.Sprintf("+%d seconds", int(d.Seconds())), id)
	return err
}
