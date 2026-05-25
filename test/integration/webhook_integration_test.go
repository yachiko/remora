// Package integration provides end-to-end integration tests for the Remora reminder service.
package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/yachiko/remora/internal/config"
	"github.com/yachiko/remora/internal/database"
	"github.com/yachiko/remora/internal/github"
	"github.com/yachiko/remora/internal/logger"
	"github.com/yachiko/remora/internal/models"
	"github.com/yachiko/remora/internal/webhook"
)

// testWebhookSecret is the secret used for signing test payloads.
const testWebhookSecret = "test-webhook-secret"

// MockGitHubClient is a mock implementation for integration tests.
type MockGitHubClient struct {
	reactions []reactionCall
	comments  []commentCall
}

type reactionCall struct {
	InstallationID int64
	Owner          string
	Repo           string
	CommentID      int64
	Reaction       github.ReactionType
}

type commentCall struct {
	InstallationID int64
	Owner          string
	Repo           string
	IssueNumber    int
	Body           string
}

func (m *MockGitHubClient) AddReaction(_ context.Context, installationID int64, owner, repo string, commentID int64, reaction github.ReactionType) error {
	m.reactions = append(m.reactions, reactionCall{
		InstallationID: installationID,
		Owner:          owner,
		Repo:           repo,
		CommentID:      commentID,
		Reaction:       reaction,
	})
	return nil
}

func (m *MockGitHubClient) PostComment(_ context.Context, installationID int64, owner, repo string, issueNumber int, body string) (int64, error) {
	m.comments = append(m.comments, commentCall{
		InstallationID: installationID,
		Owner:          owner,
		Repo:           repo,
		IssueNumber:    issueNumber,
		Body:           body,
	})
	return 999, nil
}

func setupIntegrationTest(t *testing.T) (database.ReminderRepository, func()) {
	t.Helper()

	// Initialize logger (set to fatal to suppress expected error logs in tests)
	if err := logger.Initialize("development", "fatal"); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Create a unique temp file for the database to avoid sharing state
	// between tests and issues with :memory: databases
	tempFile, err := os.CreateTemp("", "webhook_integration_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	dbPath := tempFile.Name()
	_ = tempFile.Close()

	// Initialize database with SQLite
	cfg := &config.Config{
		DatabaseType:            "sqlite",
		DatabaseSQLitePath:      dbPath,
		DatabaseMaxOpenConns:    25,
		DatabaseMaxIdleConns:    5,
		DatabaseConnMaxLifetime: 300,
		LogLevel:                "error",
	}

	if err := database.Initialize(cfg); err != nil {
		_ = os.Remove(dbPath)
		t.Fatalf("Failed to initialize database: %v", err)
	}

	repo := database.NewReminderRepository(database.DB)

	cleanup := func() {
		if err := database.Close(); err != nil {
			t.Errorf("Failed to close database: %v", err)
		}
		_ = os.Remove(dbPath)
	}

	return repo, cleanup
}

func createSignedPayload(t *testing.T, payload interface{}) ([]byte, string) {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	mac := hmac.New(sha256.New, []byte(testWebhookSecret))
	mac.Write(data)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return data, signature
}

func TestIntegration_WebhookFlow_CreateReminder(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create the webhook handler
	cfg := &config.Config{
		GitHubWebhookSecret: testWebhookSecret,
		ErrorMode:           "reaction_only",
	}

	mockGitHub := &MockGitHubClient{}

	handler := webhook.NewHandler(cfg, repo, mockGitHub, logger.Logger)

	// Create a webhook payload for creating a reminder
	event := map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         123456,
			"html_url":   "https://github.com/owner/repo/issues/1#issuecomment-123456",
			"body":       "remora 2 days",
			"user":       map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test Issue",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": 999,
		},
	}

	payloadBytes, signature := createSignedPayload(t, event)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-123")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Verify reminder was created
	reminders, err := repo.FindByIssue("owner", "repo", 1)
	if err != nil {
		t.Fatalf("Failed to find reminders: %v", err)
	}

	if len(reminders) != 1 {
		t.Errorf("Expected 1 reminder, got %d", len(reminders))
		return
	}

	reminder := reminders[0]
	if reminder.Status != models.StatusPending {
		t.Errorf("Expected status pending, got %s", reminder.Status)
	}
	if reminder.CommentID != 123456 {
		t.Errorf("Expected comment ID 123456, got %d", reminder.CommentID)
	}
}

func TestIntegration_WebhookFlow_CancelReminder(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create an existing reminder
	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       1,
		CommentID:         123456,
		CommentURL:        "https://github.com/owner/repo/issues/1#issuecomment-123456",
		InstallationID:    999,
		RequesterUsername: "testuser",
		RequesterID:       789,
		RemindAt:          time.Now().Add(24 * time.Hour),
		OriginalCommand:   "remora 1 day",
		Status:            models.StatusPending,
	}
	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Failed to create reminder: %v", err)
	}

	// Create webhook handler
	cfg := &config.Config{
		GitHubWebhookSecret: testWebhookSecret,
		ErrorMode:           "reaction_only",
	}

	mockGitHub := &MockGitHubClient{}

	handler := webhook.NewHandler(cfg, repo, mockGitHub, logger.Logger)

	// Create a delete event
	event := map[string]interface{}{
		"action": "deleted",
		"comment": map[string]interface{}{
			"id":       123456,
			"html_url": "https://github.com/owner/repo/issues/1#issuecomment-123456",
			"body":     "remora 1 day",
			"user":     map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
		},
		"issue": map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test Issue",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": 999,
		},
	}

	payloadBytes, signature := createSignedPayload(t, event)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-456")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Verify reminder was cancelled
	updated, err := repo.FindByID(reminder.ID)
	if err != nil {
		t.Fatalf("Failed to find reminder: %v", err)
	}

	if updated.Status != models.StatusCancelled {
		t.Errorf("Expected status cancelled, got %s", updated.Status)
	}
}

func TestIntegration_WebhookFlow_NonRemoraComment(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	repo, cleanup := setupIntegrationTest(t)
	defer cleanup()

	cfg := &config.Config{
		GitHubWebhookSecret: testWebhookSecret,
		ErrorMode:           "reaction_only",
	}

	mockGitHub := &MockGitHubClient{}

	handler := webhook.NewHandler(cfg, repo, mockGitHub, logger.Logger)

	// Create a comment without remora command
	event := map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         999999,
			"html_url":   "https://github.com/owner/repo/issues/1#issuecomment-999999",
			"body":       "This is just a regular comment without remora",
			"user":       map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     1,
			"number": 1,
			"title":  "Test Issue",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": 999,
		},
	}

	payloadBytes, signature := createSignedPayload(t, event)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-789")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	// Verify no reminder was created
	reminders, err := repo.FindByIssue("owner", "repo", 1)
	if err != nil {
		t.Fatalf("Failed to find reminders: %v", err)
	}

	if len(reminders) != 0 {
		t.Errorf("Expected 0 reminders, got %d", len(reminders))
	}
}
