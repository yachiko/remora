package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func setupTestClient(t *testing.T) (*Client, *httptest.Server) {
	logger := zap.NewNop()
	privateKey := generateTestPrivateKey(t)
	client := NewClient(12345, privateKey, logger)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/123/access_tokens" {
			response := map[string]interface{}{
				"token":      "ghs_test_token",
				"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	client.baseURL = server.URL
	client.retryConfig.MaxAttempts = 1

	return client, server
}

func TestAddReaction_Success(t *testing.T) {
	client, server := setupTestClient(t)
	defer server.Close()

	reactionReceived := false
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/123/access_tokens" {
			response := map[string]interface{}{
				"token":      "ghs_test_token",
				"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		if r.URL.Path == "/repos/owner/repo/issues/comments/456/reactions" {
			if r.Method != "POST" {
				t.Errorf("expected POST method, got %s", r.Method)
			}

			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)

			if payload["content"] != "eyes" {
				t.Errorf("expected reaction 'eyes', got %s", payload["content"])
			}

			reactionReceived = true

			response := map[string]interface{}{
				"id":      789,
				"content": "eyes",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	ctx := context.Background()
	err := client.AddReaction(ctx, 123, "owner", "repo", 456, ReactionEyes)

	if err != nil {
		t.Fatalf("failed to add reaction: %v", err)
	}

	if !reactionReceived {
		t.Error("expected reaction to be sent to server")
	}
}

func TestAddReaction_AllTypes(t *testing.T) {
	reactions := []ReactionType{
		ReactionEyes,
		ReactionHooray,
		ReactionConfused,
		ReactionPlusOne,
		ReactionMinusOne,
	}

	for _, reaction := range reactions {
		t.Run(string(reaction), func(t *testing.T) {
			client, server := setupTestClient(t)
			defer server.Close()

			receivedReaction := ""
			server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/app/installations/123/access_tokens" {
					response := map[string]interface{}{
						"token":      "ghs_test_token",
						"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
					}
					json.NewEncoder(w).Encode(response)
					return
				}

				if r.URL.Path == "/repos/owner/repo/issues/comments/456/reactions" {
					var payload map[string]string
					json.NewDecoder(r.Body).Decode(&payload)
					receivedReaction = payload["content"]

					w.WriteHeader(http.StatusCreated)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"id":      789,
						"content": receivedReaction,
					})
					return
				}
			})

			ctx := context.Background()
			err := client.AddReaction(ctx, 123, "owner", "repo", 456, reaction)

			if err != nil {
				t.Fatalf("failed to add reaction: %v", err)
			}

			if receivedReaction != string(reaction) {
				t.Errorf("expected reaction %s, got %s", reaction, receivedReaction)
			}
		})
	}
}

func TestPostComment_Success(t *testing.T) {
	client, server := setupTestClient(t)
	defer server.Close()

	commentReceived := false
	expectedBody := "@alice reminder test"

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/123/access_tokens" {
			response := map[string]interface{}{
				"token":      "ghs_test_token",
				"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		if r.URL.Path == "/repos/owner/repo/issues/42/comments" {
			if r.Method != "POST" {
				t.Errorf("expected POST method, got %s", r.Method)
			}

			var payload map[string]string
			json.NewDecoder(r.Body).Decode(&payload)

			if payload["body"] != expectedBody {
				t.Errorf("expected body %q, got %q", expectedBody, payload["body"])
			}

			commentReceived = true

			response := map[string]interface{}{
				"id":       999,
				"html_url": "https://github.com/owner/repo/issues/42#issuecomment-999",
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	ctx := context.Background()
	commentID, err := client.PostComment(ctx, 123, "owner", "repo", 42, expectedBody)

	if err != nil {
		t.Fatalf("failed to post comment: %v", err)
	}

	if !commentReceived {
		t.Error("expected comment to be sent to server")
	}

	if commentID != 999 {
		t.Errorf("expected comment ID 999, got %d", commentID)
	}
}

func TestFormatReminderComment(t *testing.T) {
	result := FormatReminderComment("alice", "https://github.com/owner/repo/issues/1#issuecomment-123")

	expectedContents := []string{
		"@alice",
		"🔔 Reminder!",
		"https://github.com/owner/repo/issues/1#issuecomment-123",
	}

	for _, expected := range expectedContents {
		if !strings.Contains(result, expected) {
			t.Errorf("expected comment to contain %q, got: %s", expected, result)
		}
	}
}

func TestFormatOverdueReminderComment(t *testing.T) {
	tests := []struct {
		delay    time.Duration
		expected string
	}{
		{30 * time.Minute, "30 minutes late"},
		{90 * time.Minute, "1 hour 30 minutes late"},
		{2 * time.Hour, "2 hours late"},
		{25 * time.Hour, "1 day 1 hour late"},
		{48 * time.Hour, "2 days late"},
		{72 * time.Hour, "3 days late"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatOverdueReminderComment("bob", "https://github.com/test", tt.delay)

			if !strings.Contains(result, tt.expected) {
				t.Errorf("expected comment to contain %q, got: %s", tt.expected, result)
			}

			if !strings.Contains(result, "@bob") {
				t.Errorf("expected comment to contain @bob, got: %s", result)
			}

			if !strings.Contains(result, "🔔 Reminder") {
				t.Errorf("expected comment to contain emoji, got: %s", result)
			}
		})
	}
}

func TestAPIError_401_TokenInvalidation(t *testing.T) {
	client, server := setupTestClient(t)
	defer server.Close()

	attemptCount := 0

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/123/access_tokens" {
			response := map[string]interface{}{
				"token":      "ghs_test_token",
				"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
			}
			json.NewEncoder(w).Encode(response)
			return
		}

		if r.URL.Path == "/repos/owner/repo/issues/comments/456/reactions" {
			attemptCount++
			if attemptCount == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"message": "Bad credentials"})
			} else {
				w.WriteHeader(http.StatusCreated)
				json.NewEncoder(w).Encode(map[string]interface{}{"id": 789})
			}
			return
		}
	})

	ctx := context.Background()
	err := client.AddReaction(ctx, 123, "owner", "repo", 456, ReactionEyes)

	if err != nil {
		t.Fatalf("expected success after token refresh, got error: %v", err)
	}

	if attemptCount != 2 {
		t.Errorf("expected 2 attempts (401 + retry), got %d", attemptCount)
	}
}
