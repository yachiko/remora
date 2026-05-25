package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func generateTestPrivateKey(t *testing.T) *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate test private key: %v", err)
	}
	return key
}

func TestNewClient(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)

	client := NewClient(12345, privateKey, logger)

	if client == nil {
		t.Fatal("expected client to be created")
	}
	if client.appID != 12345 {
		t.Errorf("expected appID to be 12345, got %d", client.appID)
	}
	if client.baseURL != "https://api.github.com" {
		t.Errorf("expected baseURL to be https://api.github.com, got %s", client.baseURL)
	}
	if client.tokenCache == nil {
		t.Error("expected tokenCache to be initialized")
	}
	if client.rateLimiter == nil {
		t.Error("expected rateLimiter to be initialized")
	}
}

func TestGenerateJWT(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	token, err := client.generateJWT()
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	if token == "" {
		t.Error("expected non-empty JWT token")
	}
}

func TestGetInstallationToken_Caching(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if r.URL.Path != testInstallationTokenPath {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		response := map[string]interface{}{
			"token":      "ghs_test_token",
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client.baseURL = server.URL

	ctx := context.Background()

	// First call - should hit the server
	token1, err := client.GetInstallationToken(ctx, 123)
	if err != nil {
		t.Fatalf("failed to get installation token: %v", err)
	}
	if token1 == "" {
		t.Error("expected non-empty token")
	}
	if callCount != 1 {
		t.Errorf("expected 1 server call, got %d", callCount)
	}

	// Second call - should use cache
	token2, err := client.GetInstallationToken(ctx, 123)
	if err != nil {
		t.Fatalf("failed to get installation token: %v", err)
	}
	if token2 != token1 {
		t.Error("expected cached token to be returned")
	}
	if callCount != 1 {
		t.Errorf("expected still 1 server call (cached), got %d", callCount)
	}
}

func TestGetInstallationToken_Expiration(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	// Pre-populate cache with expired token
	client.tokenCache.tokens[123] = &cachedToken{
		Token:     "expired_token",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		response := map[string]interface{}{
			"token":      "ghs_new_token",
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client.baseURL = server.URL
	ctx := context.Background()

	token, err := client.GetInstallationToken(ctx, 123)
	if err != nil {
		t.Fatalf("failed to get installation token: %v", err)
	}
	if token != "ghs_new_token" {
		t.Errorf("expected new token, got %s", token)
	}
}

func TestInvalidateToken(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	client.tokenCache.tokens[123] = &cachedToken{
		Token:     "test_token",
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	if _, exists := client.tokenCache.tokens[123]; !exists {
		t.Error("expected token to exist in cache")
	}

	client.InvalidateToken(123)

	if _, exists := client.tokenCache.tokens[123]; exists {
		t.Error("expected token to be removed from cache")
	}
}

func TestTokenCache_Concurrency(t *testing.T) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		time.Sleep(10 * time.Millisecond)
		response := map[string]interface{}{
			"token":      "ghs_concurrent_token",
			"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client.baseURL = server.URL
	ctx := context.Background()

	const numGoroutines = 10
	tokens := make(chan string, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			token, err := client.GetInstallationToken(ctx, 123)
			if err != nil {
				errors <- err
				return
			}
			tokens <- token
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errors:
			t.Errorf("unexpected error: %v", err)
		case token := <-tokens:
			if token == "" {
				t.Error("expected non-empty token")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for goroutines")
		}
	}

	if callCount > 3 {
		t.Errorf("expected at most 3 server calls with concurrent requests, got %d", callCount)
	}
}
