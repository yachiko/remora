package github

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
)

// rateLimiter tracks GitHub API rate limit status
type rateLimiter struct {
	mu        sync.RWMutex
	limit     int
	remaining int
	resetAt   time.Time
}

// updateRateLimitInfo updates rate limit information from response headers
func (c *Client) updateRateLimitInfo(resp *http.Response) {
	c.rateLimiter.mu.Lock()
	defer c.rateLimiter.mu.Unlock()

	// Parse rate limit headers
	if limitStr := resp.Header.Get("X-RateLimit-Limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			c.rateLimiter.limit = limit
		}
	}

	if remainingStr := resp.Header.Get("X-RateLimit-Remaining"); remainingStr != "" {
		if remaining, err := strconv.Atoi(remainingStr); err == nil {
			c.rateLimiter.remaining = remaining
		}
	}

	if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
		if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			c.rateLimiter.resetAt = time.Unix(resetUnix, 0)
		}
	}

	// Log warning if approaching rate limit
	if c.rateLimiter.remaining > 0 && c.rateLimiter.limit > 0 {
		percentRemaining := float64(c.rateLimiter.remaining) / float64(c.rateLimiter.limit) * 100

		if percentRemaining < 10 {
			c.logger.Warn("approaching GitHub API rate limit",
				zap.Int("remaining", c.rateLimiter.remaining),
				zap.Int("limit", c.rateLimiter.limit),
				zap.Float64("percent_remaining", percentRemaining),
				zap.Time("resets_at", c.rateLimiter.resetAt))
		} else if percentRemaining < 25 {
			c.logger.Info("GitHub API rate limit status",
				zap.Int("remaining", c.rateLimiter.remaining),
				zap.Int("limit", c.rateLimiter.limit),
				zap.Float64("percent_remaining", percentRemaining),
				zap.Time("resets_at", c.rateLimiter.resetAt))
		}
	}
}

// GetRateLimitStatus returns current rate limit status
func (c *Client) GetRateLimitStatus() (limit, remaining int, resetAt time.Time) {
	c.rateLimiter.mu.RLock()
	defer c.rateLimiter.mu.RUnlock()

	return c.rateLimiter.limit, c.rateLimiter.remaining, c.rateLimiter.resetAt
}
