package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type limiterEntry struct {
	Count   int
	ResetAt time.Time
}

type RateLimiter struct {
	mu     sync.Mutex
	store  map[string]*limiterEntry
	max    int
	window time.Duration
}

func NewRateLimiter(maxPerMinute int) *RateLimiter {
	if maxPerMinute <= 0 {
		maxPerMinute = 30
	}
	return &RateLimiter{
		store:  map[string]*limiterEntry{},
		max:    maxPerMinute,
		window: time.Minute,
	}
}

func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		now := time.Now()
		r.mu.Lock()
		ent, ok := r.store[ip]
		if !ok || now.After(ent.ResetAt) {
			ent = &limiterEntry{Count: 0, ResetAt: now.Add(r.window)}
			r.store[ip] = ent
		}
		ent.Count++
		blocked := ent.Count > r.max
		r.mu.Unlock()

		if blocked {
			c.AbortWithStatusJSON(429, gin.H{"code": 429, "message": "too many requests"})
			return
		}
		c.Next()
	}
}
