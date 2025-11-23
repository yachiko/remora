package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRetryConfig_Default(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts != 5 {
		t.Errorf("expected max attempts 5, got %d", config.MaxAttempts)
	}
	if config.InitialBackoff != 1*time.Minute {
		t.Errorf("expected initial backoff 1m, got %v", config.InitialBackoff)
	}
	if config.MaxBackoff != 16*time.Minute {
		t.Errorf("expected max backoff 16m, got %v", config.MaxBackoff)
	}
	if config.Multiplier != 2.0 {
		t.Errorf("expected multiplier 2.0, got %v", config.Multiplier)
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		statusCode int
		retryable  bool
	}{
		{200, false},
		{201, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode}
			if err.IsRetryable() != tt.retryable {
				t.Errorf("status %d: expected retryable=%v, got %v",
					tt.statusCode, tt.retryable, err.IsRetryable())
			}
		})
	}
}

func TestAPIError_TypeChecks(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		checkFunc  func(error) bool
		expected   bool
	}{
		{"401 is unauthorized", 401, IsUnauthorizedError, true},
		{"403 is not unauthorized", 403, IsUnauthorizedError, false},
		{"404 is not found", 404, IsNotFoundError, true},
		{"403 is not not found", 403, IsNotFoundError, false},
		{"403 is forbidden", 403, IsForbiddenError, true},
		{"404 is not forbidden", 404, IsForbiddenError, false},
		{"429 is rate limit", 429, IsRateLimitError, true},
		{"500 is not rate limit", 500, IsRateLimitError, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode}
			if tt.checkFunc(err) != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, !tt.expected)
			}
		})
	}
}

func TestDoRequestWithRetry_Success(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client.baseURL = server.URL

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	var result map[string]string

	ctx := context.Background()
	err := client.doRequestWithRetry(ctx, req, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %s", result["status"])
	}
}

func TestDoRequestWithRetry_EventualSuccess(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	client.retryConfig.InitialBackoff = 10 * time.Millisecond
	client.retryConfig.MaxBackoff = 100 * time.Millisecond

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"message": "server error"})
		} else {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		}
	}))
	defer server.Close()

	client.baseURL = server.URL

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	var result map[string]string

	ctx := context.Background()
	err := client.doRequestWithRetry(ctx, req, &result)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", callCount)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %s", result["status"])
	}
}

func TestDoRequestWithRetry_MaxAttemptsReached(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	client.retryConfig.InitialBackoff = 1 * time.Millisecond
	client.retryConfig.MaxBackoff = 10 * time.Millisecond
	client.retryConfig.MaxAttempts = 3

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"message": "persistent error"})
	}))
	defer server.Close()

	client.baseURL = server.URL

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	var result map[string]string

	ctx := context.Background()
	err := client.doRequestWithRetry(ctx, req, &result)

	if err == nil {
		t.Fatal("expected error after max attempts")
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls (max attempts), got %d", callCount)
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}

	if apiErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
}

func TestDoRequestWithRetry_NonRetryableError(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"message": "not found"})
	}))
	defer server.Close()

	client.baseURL = server.URL

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	var result map[string]string

	ctx := context.Background()
	err := client.doRequestWithRetry(ctx, req, &result)

	if err == nil {
		t.Fatal("expected error for 404")
	}

	if callCount != 1 {
		t.Errorf("expected 1 call (no retry for 404), got %d", callCount)
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}

	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
}

func TestDoRequestWithRetry_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	client.retryConfig.InitialBackoff = 100 * time.Millisecond

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client.baseURL = server.URL

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	var result map[string]string

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- client.doRequestWithRetry(ctx, req, &result)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-errChan

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	if callCount > 2 {
		t.Errorf("expected at most 2 calls before cancellation, got %d", callCount)
	}
}

func TestExponentialBackoff(t *testing.T) {
	config := DefaultRetryConfig()

	backoff := config.InitialBackoff
	expected := []time.Duration{
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
		16 * time.Minute,
	}

	for i, exp := range expected {
		if backoff != exp {
			t.Errorf("iteration %d: expected backoff %v, got %v", i, exp, backoff)
		}

		backoff = time.Duration(float64(backoff) * config.Multiplier)
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}
}
