package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yachiko/remora/internal/config"
	"github.com/yachiko/remora/internal/database"
	"github.com/yachiko/remora/internal/github"
	"github.com/yachiko/remora/internal/logger"
	"github.com/yachiko/remora/internal/models"
	"github.com/yachiko/remora/internal/scheduler"
	"github.com/yachiko/remora/internal/webhook"
)

// lifecycleTestEnv holds the test environment for lifecycle tests.
type lifecycleTestEnv struct {
	mockGitHub     *MockGitHubServer
	repo           database.ReminderRepository
	webhookCfg     *config.Config
	cleanup        func()
	privateKey     *rsa.PrivateKey
	appID          int64
	installationID int64
}

// setupLifecycleTest creates a complete test environment for lifecycle tests.
func setupLifecycleTest(t *testing.T) *lifecycleTestEnv {
	t.Helper()

	// Initialize logger (set to fatal to suppress expected error logs in tests)
	if err := logger.Initialize("development", "fatal"); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Generate a test RSA key for GitHub App authentication
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Create unique temp file for SQLite database to ensure test isolation
	tmpFile, err := os.CreateTemp("", "remora_test_*.db")
	if err != nil {
		t.Fatalf("Failed to create temp database file: %v", err)
	}
	dbPath := tmpFile.Name()
	_ = tmpFile.Close()

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

	// Create mock GitHub server
	mockGitHub := NewMockGitHubServer(t)

	// Note: We use adapters instead of the real GitHub client since we can't
	// easily override the base URL. The adapters route through our mock server.
	appID := int64(12345)

	// Create webhook config
	webhookCfg := &config.Config{
		GitHubWebhookSecret: testWebhookSecret,
		ErrorMode:           "reaction_only",
	}

	env := &lifecycleTestEnv{
		mockGitHub:     mockGitHub,
		repo:           repo,
		webhookCfg:     webhookCfg,
		privateKey:     privateKey,
		appID:          appID,
		installationID: 999,
		cleanup: func() {
			mockGitHub.Close()
			if err := database.Close(); err != nil {
				t.Errorf("Failed to close database: %v", err)
			}
			// Clean up temp database file
			_ = os.Remove(dbPath)
		},
	}

	return env
}

// gitHubClientAdapter wraps the lifecycle test environment to provide a GitHubClient interface
// that routes through the mock server.
type gitHubClientAdapter struct {
	mockServer *MockGitHubServer
}

func (a *gitHubClientAdapter) AddReaction(_ context.Context, _ int64, owner, repo string, commentID int64, reaction github.ReactionType) error {
	// Simulate the reaction by recording it directly in the mock server
	a.mockServer.mu.Lock()
	a.mockServer.ReactionRequests = append(a.mockServer.ReactionRequests, ReactionRequest{
		Owner:     owner,
		Repo:      repo,
		CommentID: commentID,
		Content:   string(reaction),
	})
	a.mockServer.mu.Unlock()
	return nil
}

func (a *gitHubClientAdapter) PostComment(_ context.Context, _ int64, owner, repo string, issueNumber int, body string) (int64, error) {
	a.mockServer.mu.Lock()
	defer a.mockServer.mu.Unlock()

	if a.mockServer.FailCommentRequest {
		return 0, &github.APIError{StatusCode: 500, Message: "Internal server error"}
	}

	a.mockServer.commentIDSeq++
	newCommentID := a.mockServer.commentIDSeq

	a.mockServer.CommentRequests = append(a.mockServer.CommentRequests, CommentRequest{
		Owner:       owner,
		Repo:        repo,
		IssueNumber: issueNumber,
		Body:        body,
	})

	return newCommentID, nil
}

// schedulerGitHubAdapter wraps the adapter for the scheduler interface.
type schedulerGitHubAdapter struct {
	*gitHubClientAdapter
}

func (a *schedulerGitHubAdapter) PostComment(ctx context.Context, installationID int64, owner, repo string, issueNumber int, body string) error {
	_, err := a.gitHubClientAdapter.PostComment(ctx, installationID, owner, repo, issueNumber, body)
	return err
}

func TestFullReminderLifecycle(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	env := setupLifecycleTest(t)
	defer env.cleanup()

	// Create an adapter for the webhook handler
	webhookGitHubAdapter := &gitHubClientAdapter{mockServer: env.mockGitHub}

	// Create webhook handler
	webhookHandler := webhook.NewHandler(env.webhookCfg, env.repo, webhookGitHubAdapter, logger.Logger)

	// ============================================
	// Step 1: Send webhook to create reminder
	// ============================================
	payload := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(123456),
			"body":       "remora 20 minutes", // Just above minimum (15 min)
			"html_url":   "https://github.com/owner/repo/issues/1#issuecomment-123456",
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
			"id": env.installationID,
		},
	})

	payloadBytes, signature := createSignedPayload(t, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-lifecycle")

	rr := httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "Webhook should return 200 OK")

	// ============================================
	// Step 2: Verify reminder created
	// ============================================
	reminders, err := env.repo.FindByIssue("owner", "repo", 1)
	require.NoError(t, err)
	require.Len(t, reminders, 1, "Should have created one reminder")

	reminder := reminders[0]
	assert.Equal(t, models.StatusPending, reminder.Status, "Reminder should be pending")
	assert.Equal(t, "testuser", reminder.RequesterUsername)
	assert.Equal(t, int64(123456), reminder.CommentID)

	// ============================================
	// Step 3: Verify eyes reaction was added
	// ============================================
	env.mockGitHub.AssertReactionAdded(t, "owner", "repo", 123456, "eyes")

	// ============================================
	// Step 4: Manipulate reminder time and start scheduler
	// ============================================
	// Since parser enforces minimum 15 minutes, we need to manually
	// set RemindAt to the past so the scheduler can pick it up.
	// We do this by cancelling the existing reminder and creating a new one
	// with the same details but a past RemindAt time.
	err = env.repo.Cancel(123456) // Cancel by comment ID
	require.NoError(t, err)

	// Create a new reminder directly with past RemindAt
	testReminder := &models.Reminder{
		CommentID:         123456,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       1,
		RequesterUsername: "testuser",
		RequesterID:       789,
		CommentURL:        "https://github.com/owner/repo/issues/1#issuecomment-123456",
		OriginalCommand:   "remora 20 minutes",
		RemindAt:          time.Now().Add(-1 * time.Second), // Past time
		Status:            models.StatusPending,
		InstallationID:    env.installationID,
	}
	err = env.repo.Create(testReminder)
	require.NoError(t, err)

	schedulerCfg := &scheduler.Config{
		Interval:   100 * time.Millisecond,
		MaxRetries: 3,
	}
	schedulerAdapter := &schedulerGitHubAdapter{gitHubClientAdapter: webhookGitHubAdapter}
	sched := scheduler.New(env.repo, schedulerAdapter, logger.Logger, schedulerCfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sched.Start(ctx)
	require.NoError(t, err)
	defer func() {
		if stopErr := sched.Stop(); stopErr != nil {
			t.Logf("Failed to stop scheduler: %v", stopErr)
		}
	}()

	// Scheduler polls every 100ms, give it time to process
	time.Sleep(500 * time.Millisecond)

	// Use the test reminder ID for verification
	reminder = testReminder

	// ============================================
	// Step 5: Verify reminder fired
	// ============================================
	reminder, err = env.repo.FindByID(reminder.ID)
	require.NoError(t, err)
	assert.Equal(t, models.StatusFired, reminder.Status, "Reminder should be fired")
	assert.NotNil(t, reminder.FiredAt, "FiredAt should be set")

	// ============================================
	// Step 6: Verify comment was posted
	// ============================================
	env.mockGitHub.AssertCommentPosted(t, "owner", "repo", 1, "@testuser")
}

func TestLifecycle_CancelByDeletion(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	env := setupLifecycleTest(t)
	defer env.cleanup()

	webhookGitHubAdapter := &gitHubClientAdapter{mockServer: env.mockGitHub}
	webhookHandler := webhook.NewHandler(env.webhookCfg, env.repo, webhookGitHubAdapter, logger.Logger)

	// Step 1: Create a reminder
	createPayload := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(999888),
			"body":       "remora 1 hour", // Long duration so it won't fire
			"html_url":   "https://github.com/owner/repo/issues/2#issuecomment-999888",
			"user":       map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     2,
			"number": 2,
			"title":  "Test Issue 2",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature := createSignedPayload(t, createPayload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-create")

	rr := httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify reminder created
	reminder, err := env.repo.FindByCommentID(999888)
	require.NoError(t, err)
	require.NotNil(t, reminder)
	assert.Equal(t, models.StatusPending, reminder.Status)

	// Step 2: Delete the comment (cancel the reminder)
	deletePayload := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "deleted",
		"comment": map[string]interface{}{
			"id":         int64(999888),
			"body":       "remora 1 hour",
			"html_url":   "https://github.com/owner/repo/issues/2#issuecomment-999888",
			"user":       map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     2,
			"number": 2,
			"title":  "Test Issue 2",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature = createSignedPayload(t, deletePayload)
	req = httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-delete")

	rr = httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Step 3: Verify reminder was cancelled
	reminder, err = env.repo.FindByCommentID(999888)
	require.NoError(t, err)
	require.NotNil(t, reminder)
	assert.Equal(t, models.StatusCancelled, reminder.Status, "Reminder should be cancelled")
}

func TestLifecycle_ParseError(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	env := setupLifecycleTest(t)
	defer env.cleanup()

	webhookGitHubAdapter := &gitHubClientAdapter{mockServer: env.mockGitHub}
	webhookHandler := webhook.NewHandler(env.webhookCfg, env.repo, webhookGitHubAdapter, logger.Logger)

	// Send a webhook with an invalid command
	payload := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(777666),
			"body":       "remora invalid-time-expression",
			"html_url":   "https://github.com/owner/repo/issues/3#issuecomment-777666",
			"user":       map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     3,
			"number": 3,
			"title":  "Test Issue 3",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature := createSignedPayload(t, payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-parse-error")

	rr := httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)

	// Webhook should still return 200 (error handled internally)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify no reminder was created
	reminders, err := env.repo.FindByIssue("owner", "repo", 3)
	require.NoError(t, err)
	assert.Len(t, reminders, 0, "No reminder should be created for invalid command")

	// Verify confused reaction was added (error indicator)
	env.mockGitHub.AssertReactionAdded(t, "owner", "repo", 777666, "confused")
}

func TestLifecycle_RetryOnFailure(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	env := setupLifecycleTest(t)
	defer env.cleanup()

	webhookGitHubAdapter := &gitHubClientAdapter{mockServer: env.mockGitHub}
	webhookHandler := webhook.NewHandler(env.webhookCfg, env.repo, webhookGitHubAdapter, logger.Logger)

	// Create a reminder first (before starting scheduler)
	payload := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(555444),
			"body":       "remora 20 minutes",
			"html_url":   "https://github.com/owner/repo/issues/4#issuecomment-555444",
			"user":       map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     4,
			"number": 4,
			"title":  "Test Issue 4",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature := createSignedPayload(t, payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-retry")

	rr := httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Get the reminder and cancel it, then create a new one with past RemindAt
	reminder, err := env.repo.FindByCommentID(555444)
	require.NoError(t, err)
	require.NotNil(t, reminder)

	err = env.repo.Cancel(555444)
	require.NoError(t, err)

	// Create new reminder with past RemindAt
	testReminder := &models.Reminder{
		CommentID:         555444,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       4,
		RequesterUsername: "testuser",
		RequesterID:       789,
		CommentURL:        "https://github.com/owner/repo/issues/4#issuecomment-555444",
		OriginalCommand:   "remora 20 minutes",
		RemindAt:          time.Now().Add(-1 * time.Second),
		Status:            models.StatusPending,
		InstallationID:    env.installationID,
	}
	err = env.repo.Create(testReminder)
	require.NoError(t, err)

	// Configure mock to fail comment requests
	env.mockGitHub.mu.Lock()
	env.mockGitHub.FailCommentRequest = true
	env.mockGitHub.mu.Unlock()

	// Now start scheduler
	schedulerCfg := &scheduler.Config{
		Interval:   100 * time.Millisecond,
		MaxRetries: 3,
	}
	schedulerAdapter := &schedulerGitHubAdapter{gitHubClientAdapter: webhookGitHubAdapter}
	sched := scheduler.New(env.repo, schedulerAdapter, logger.Logger, schedulerCfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = sched.Start(ctx)
	require.NoError(t, err)
	defer func() {
		if stopErr := sched.Stop(); stopErr != nil {
			t.Logf("Failed to stop scheduler: %v", stopErr)
		}
	}()

	// Wait for scheduler to attempt processing
	time.Sleep(500 * time.Millisecond)

	// Check that the reminder was marked as failed (use FindByID with testReminder.ID)
	reminder, err = env.repo.FindByID(testReminder.ID)
	require.NoError(t, err)
	require.NotNil(t, reminder)
	assert.Equal(t, models.StatusFailed, reminder.Status, "Reminder should be marked as failed")
	assert.NotEmpty(t, reminder.ErrorMessage, "Error message should be set")

	// Re-enable successful comments
	env.mockGitHub.mu.Lock()
	env.mockGitHub.FailCommentRequest = false
	env.mockGitHub.mu.Unlock()
}

func TestLifecycle_NonRemoraComment(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	env := setupLifecycleTest(t)
	defer env.cleanup()

	webhookGitHubAdapter := &gitHubClientAdapter{mockServer: env.mockGitHub}
	webhookHandler := webhook.NewHandler(env.webhookCfg, env.repo, webhookGitHubAdapter, logger.Logger)

	// Send a webhook with a non-remora comment
	payload := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(333222),
			"body":       "This is just a regular comment",
			"html_url":   "https://github.com/owner/repo/issues/5#issuecomment-333222",
			"user":       map[string]interface{}{"id": 789, "login": "testuser", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     5,
			"number": 5,
			"title":  "Test Issue 5",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature := createSignedPayload(t, payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-non-remora")

	rr := httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Verify no reminder was created
	reminders, err := env.repo.FindByIssue("owner", "repo", 5)
	require.NoError(t, err)
	assert.Len(t, reminders, 0, "No reminder should be created for non-remora comment")

	// Verify no reactions were added
	env.mockGitHub.mu.Lock()
	reactionCount := len(env.mockGitHub.ReactionRequests)
	env.mockGitHub.mu.Unlock()
	assert.Equal(t, 0, reactionCount, "No reactions should be added for non-remora comment")
}

func TestLifecycle_BotComment(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	env := setupLifecycleTest(t)
	defer env.cleanup()

	webhookGitHubAdapter := &gitHubClientAdapter{mockServer: env.mockGitHub}
	webhookHandler := webhook.NewHandler(env.webhookCfg, env.repo, webhookGitHubAdapter, logger.Logger)

	// Send a webhook from a bot user
	// Note: Currently, bot comments are processed the same as user comments
	// This test verifies the current behavior
	payload := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(111000),
			"body":       "remora 1 day",
			"html_url":   "https://github.com/owner/repo/issues/6#issuecomment-111000",
			"user":       map[string]interface{}{"id": 789, "login": "some-bot[bot]", "type": "Bot"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     6,
			"number": 6,
			"title":  "Test Issue 6",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature := createSignedPayload(t, payload)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-bot")

	rr := httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Bot comments are currently processed (no bot filtering implemented)
	// This verifies the current behavior
	reminders, err := env.repo.FindByIssue("owner", "repo", 6)
	require.NoError(t, err)
	assert.Len(t, reminders, 1, "Bot comments are currently processed as normal comments")
}

func TestLifecycle_MultipleRemindersOnSameIssue(t *testing.T) {
	if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
		t.Skip("Skipping integration test; set REMORA_INTEGRATION_TESTS=1 to run")
	}

	env := setupLifecycleTest(t)
	defer env.cleanup()

	webhookGitHubAdapter := &gitHubClientAdapter{mockServer: env.mockGitHub}
	webhookHandler := webhook.NewHandler(env.webhookCfg, env.repo, webhookGitHubAdapter, logger.Logger)

	// Create first reminder
	payload1 := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(100001),
			"body":       "remora 1 hour",
			"html_url":   "https://github.com/owner/repo/issues/7#issuecomment-100001",
			"user":       map[string]interface{}{"id": 789, "login": "user1", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     7,
			"number": 7,
			"title":  "Test Issue 7",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature := createSignedPayload(t, payload1)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-multi-1")

	rr := httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Create second reminder on same issue from different user
	payload2 := createWebhookPayloadMap(t, map[string]interface{}{
		"action": "created",
		"comment": map[string]interface{}{
			"id":         int64(100002),
			"body":       "remora 2 hours",
			"html_url":   "https://github.com/owner/repo/issues/7#issuecomment-100002",
			"user":       map[string]interface{}{"id": 790, "login": "user2", "type": "User"},
			"created_at": time.Now().Format(time.RFC3339),
			"updated_at": time.Now().Format(time.RFC3339),
		},
		"issue": map[string]interface{}{
			"id":     7,
			"number": 7,
			"title":  "Test Issue 7",
			"state":  "open",
		},
		"repository": map[string]interface{}{
			"id":        456,
			"name":      "repo",
			"full_name": "owner/repo",
			"owner":     map[string]interface{}{"id": 111, "login": "owner", "type": "Organization"},
		},
		"installation": map[string]interface{}{
			"id": env.installationID,
		},
	})

	payloadBytes, signature = createSignedPayload(t, payload2)
	req = httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set("X-GitHub-Event", "issue_comment")
	req.Header.Set("X-Hub-Signature-256", signature)
	req.Header.Set("X-GitHub-Delivery", "test-delivery-multi-2")

	rr = httptest.NewRecorder()
	webhookHandler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify both reminders were created
	reminders, err := env.repo.FindByIssue("owner", "repo", 7)
	require.NoError(t, err)
	assert.Len(t, reminders, 2, "Should have two reminders on the same issue")

	// Verify different users
	usernames := make(map[string]bool)
	for _, r := range reminders {
		usernames[r.RequesterUsername] = true
	}
	assert.True(t, usernames["user1"], "Should have reminder from user1")
	assert.True(t, usernames["user2"], "Should have reminder from user2")
}

// createWebhookPayloadMap is a helper that returns the map for further processing.
func createWebhookPayloadMap(_ *testing.T, data map[string]interface{}) map[string]interface{} {
	return data
}
