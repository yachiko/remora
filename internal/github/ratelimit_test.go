package github

import (
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestUpdateRateLimitInfo(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	// Verify initial state
	limit, remaining, _ := client.GetRateLimitStatus()
	if limit != 0 {
		t.Errorf("expected initial limit 0, got %d", limit)
	}
	if remaining != 5000 {
		t.Errorf("expected initial remaining 5000, got %d", remaining)
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
	}
	resp.Header.Set("X-RateLimit-Limit", "5000")
	resp.Header.Set("X-RateLimit-Remaining", "4500")
	resp.Header.Set("X-RateLimit-Reset", "1700000000")

	client.updateRateLimitInfo(resp)

	limit, remaining, resetAt := client.GetRateLimitStatus()

	if limit != 5000 {
		t.Errorf("expected limit 5000, got %d", limit)
	}
	if remaining != 4500 {
		t.Errorf("expected remaining 4500, got %d", remaining)
	}
	if resetAt.Unix() != 1700000000 {
		t.Errorf("expected reset 1700000000, got %d", resetAt.Unix())
	}
}

func TestUpdateRateLimitInfo_MissingHeaders(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	client.rateLimiter.limit = 100
	client.rateLimiter.remaining = 50

	resp := &http.Response{
		Header: http.Header{},
	}

	client.updateRateLimitInfo(resp)

	limit, remaining, _ := client.GetRateLimitStatus()

	if limit != 100 {
		t.Errorf("expected limit unchanged at 100, got %d", limit)
	}
	if remaining != 50 {
		t.Errorf("expected remaining unchanged at 50, got %d", remaining)
	}
}

func TestUpdateRateLimitInfo_InvalidHeaders(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	resp := &http.Response{
		Header: http.Header{
			"X-RateLimit-Limit":     []string{"not-a-number"},
			"X-RateLimit-Remaining": []string{"also-invalid"},
			"X-RateLimit-Reset":     []string{"bad-timestamp"},
		},
	}

	client.updateRateLimitInfo(resp)

	limit, remaining, _ := client.GetRateLimitStatus()
	if limit != 0 {
		t.Errorf("expected default limit 0, got %d", limit)
	}
	if remaining != 5000 {
		t.Errorf("expected default remaining 5000, got %d", remaining)
	}
}

func TestGetRateLimitStatus(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	client.rateLimiter.limit = 1000
	client.rateLimiter.remaining = 750
	resetTime := time.Now().Add(1 * time.Hour)
	client.rateLimiter.resetAt = resetTime

	limit, remaining, resetAt := client.GetRateLimitStatus()

	if limit != 1000 {
		t.Errorf("expected limit 1000, got %d", limit)
	}
	if remaining != 750 {
		t.Errorf("expected remaining 750, got %d", remaining)
	}
	if !resetAt.Equal(resetTime) {
		t.Errorf("expected resetAt %v, got %v", resetTime, resetAt)
	}
}

func TestRateLimitConcurrency(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			resp := &http.Response{
				Header: http.Header{
					"X-RateLimit-Limit":     []string{"5000"},
					"X-RateLimit-Remaining": []string{"4500"},
					"X-RateLimit-Reset":     []string{"1700000000"},
				},
			}
			client.updateRateLimitInfo(resp)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		go func() {
			client.GetRateLimitStatus()
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for goroutines")
		}
	}

	limit, remaining, _ := client.GetRateLimitStatus()
	if limit < 0 {
		t.Errorf("expected non-negative limit, got %d", limit)
	}
	if remaining < 0 {
		t.Errorf("expected non-negative remaining, got %d", remaining)
	}
}
