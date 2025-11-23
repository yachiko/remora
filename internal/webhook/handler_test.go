package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yachiko/remora/internal/config"
	"github.com/yachiko/remora/internal/github"
	"github.com/yachiko/remora/internal/models"
	"github.com/yachiko/remora/internal/parser"
	"go.uber.org/zap/zaptest"
)

// MockRepository is a mock implementation of ReminderRepository
type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) Create(reminder *models.Reminder) error {
	args := m.Called(reminder)
	if args.Error(0) == nil {
		reminder.ID = 123
	}
	return args.Error(0)
}

func (m *MockRepository) FindByID(id uint) (*models.Reminder, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Reminder), args.Error(1)
}

func (m *MockRepository) FindByCommentID(commentID int64) (*models.Reminder, error) {
	args := m.Called(commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Reminder), args.Error(1)
}

func (m *MockRepository) FindByIssue(owner, repo string, issueNumber int) ([]*models.Reminder, error) {
	args := m.Called(owner, repo, issueNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockRepository) FindDueReminders(limit int) ([]*models.Reminder, error) {
	args := m.Called(limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockRepository) GetAndLockDueReminders(limit int) ([]*models.Reminder, error) {
	args := m.Called(limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockRepository) UpdateStatus(id uint, status models.ReminderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockRepository) MarkFired(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockRepository) MarkFailed(id uint, errorMsg string) error {
	args := m.Called(id, errorMsg)
	return args.Error(0)
}

func (m *MockRepository) IncrementRetry(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockRepository) Cancel(commentID int64) error {
	args := m.Called(commentID)
	return args.Error(0)
}

func (m *MockRepository) Delete(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

// MockGitHubClient is a mock implementation of GitHub client methods
type MockGitHubClient struct {
	mock.Mock
}

func (m *MockGitHubClient) AddReaction(ctx context.Context, installationID int64, owner, repo string, commentID int64, reaction github.ReactionType) error {
	args := m.Called(ctx, installationID, owner, repo, commentID, reaction)
	return args.Error(0)
}

func (m *MockGitHubClient) PostComment(ctx context.Context, installationID int64, owner, repo string, issueNumber int, body string) (int64, error) {
	args := m.Called(ctx, installationID, owner, repo, issueNumber, body)
	return args.Get(0).(int64), args.Error(1)
}

// Helper function to create a signed webhook payload
func createSignedPayload(t *testing.T, secret string, payload interface{}) ([]byte, string) {
	payloadBytes, err := json.Marshal(payload)
	assert.NoError(t, err)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payloadBytes)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return payloadBytes, signature
}

// Helper function to create a test handler
func createTestHandler(t *testing.T, mockRepo *MockRepository, mockGitHub *MockGitHubClient, errorMode string) *Handler {
	cfg := &config.Config{
		GitHubWebhookSecret: "test-secret",
		ErrorMode:           errorMode,
	}

	logger := zaptest.NewLogger(t)
	p := parser.NewParser()

	return &Handler{
		validator:  NewValidator(cfg.GitHubWebhookSecret),
		parser:     p,
		repo:       mockRepo,
		github:     mockGitHub,
		logger:     logger,
		errorMode:  cfg.ErrorMode,
		requestLog: true,
	}
}

func TestHandler_ServeHTTP_MethodNotAllowed(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGitHub := new(MockGitHubClient)
	handler := createTestHandler(t, mockRepo, mockGitHub, "reaction_only")

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
}

func TestHandler_ServeHTTP_InvalidSignature(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGitHub := new(MockGitHubClient)
	handler := createTestHandler(t, mockRepo, mockGitHub, "reaction_only")

	payload := map[string]string{"action": "created"}
	payloadBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set(EventTypeHeader, EventIssueComment)
	req.Header.Set(SignatureHeader, "sha256=invalidsignature")
	req.Header.Set(DeliveryHeader, "test-delivery-123")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHandler_ServeHTTP_IgnoreOtherEvents(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGitHub := new(MockGitHubClient)
	handler := createTestHandler(t, mockRepo, mockGitHub, "reaction_only")

	payload := map[string]string{"action": "opened"}
	payloadBytes, signature := createSignedPayload(t, "test-secret", payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set(EventTypeHeader, "issues") // Different event type
	req.Header.Set(SignatureHeader, signature)
	req.Header.Set(DeliveryHeader, "test-delivery-123")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestHandler_ServeHTTP_NoRemoraCommand(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGitHub := new(MockGitHubClient)
	handler := createTestHandler(t, mockRepo, mockGitHub, "reaction_only")

	// Create a comment without remora command
	event := IssueCommentEvent{
		Action: ActionCreated,
		Comment: Comment{
			ID:        123456,
			HTMLURL:   "https://github.com/owner/repo/issues/1#issuecomment-123456",
			Body:      "This is just a regular comment",
			User:      User{ID: 789, Login: "testuser", Type: "User"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Issue: Issue{
			ID:     1,
			Number: 1,
			Title:  "Test Issue",
			State:  "open",
		},
		Repository: Repository{
			ID:       456,
			Name:     "repo",
			FullName: "owner/repo",
			Owner:    User{ID: 111, Login: "owner", Type: "Organization"},
		},
		Installation: Installation{ID: 999},
	}

	payloadBytes, signature := createSignedPayload(t, "test-secret", event)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set(EventTypeHeader, EventIssueComment)
	req.Header.Set(SignatureHeader, signature)
	req.Header.Set(DeliveryHeader, "test-delivery-123")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	mockGitHub.AssertNotCalled(t, "AddReaction")
	mockRepo.AssertNotCalled(t, "Create")
}

func TestHandler_ServeHTTP_CommentDeleted_Success(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGitHub := new(MockGitHubClient)
	handler := createTestHandler(t, mockRepo, mockGitHub, "reaction_only")

	event := IssueCommentEvent{
		Action: ActionDeleted,
		Comment: Comment{
			ID:      123456,
			HTMLURL: "https://github.com/owner/repo/issues/1#issuecomment-123456",
			Body:    "remora 2 days",
			User:    User{ID: 789, Login: "testuser", Type: "User"},
		},
		Issue: Issue{
			ID:     1,
			Number: 1,
			Title:  "Test Issue",
			State:  "open",
		},
		Repository: Repository{
			ID:       456,
			Name:     "repo",
			FullName: "owner/repo",
			Owner:    User{ID: 111, Login: "owner", Type: "Organization"},
		},
		Installation: Installation{ID: 999},
	}

	payloadBytes, signature := createSignedPayload(t, "test-secret", event)

	existingReminder := &models.Reminder{
		ID:                123,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       1,
		CommentID:         123456,
		RequesterUsername: "testuser",
		Status:            models.StatusPending,
	}

	mockRepo.On("FindByCommentID", int64(123456)).Return(existingReminder, nil)
	mockRepo.On("Cancel", int64(123456)).Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set(EventTypeHeader, EventIssueComment)
	req.Header.Set(SignatureHeader, signature)
	req.Header.Set(DeliveryHeader, "test-delivery-123")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	mockRepo.AssertExpectations(t)
}

func TestHandler_ServeHTTP_CommentDeleted_NoReminder(t *testing.T) {
	mockRepo := new(MockRepository)
	mockGitHub := new(MockGitHubClient)
	handler := createTestHandler(t, mockRepo, mockGitHub, "reaction_only")

	event := IssueCommentEvent{
		Action: ActionDeleted,
		Comment: Comment{
			ID:      123456,
			HTMLURL: "https://github.com/owner/repo/issues/1#issuecomment-123456",
			Body:    "some comment",
			User:    User{ID: 789, Login: "testuser", Type: "User"},
		},
		Issue: Issue{
			ID:     1,
			Number: 1,
			Title:  "Test Issue",
			State:  "open",
		},
		Repository: Repository{
			ID:       456,
			Name:     "repo",
			FullName: "owner/repo",
			Owner:    User{ID: 111, Login: "owner", Type: "Organization"},
		},
		Installation: Installation{ID: 999},
	}

	payloadBytes, signature := createSignedPayload(t, "test-secret", event)

	mockRepo.On("FindByCommentID", int64(123456)).Return(nil, errors.New("not found"))

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payloadBytes))
	req.Header.Set(EventTypeHeader, EventIssueComment)
	req.Header.Set(SignatureHeader, signature)
	req.Header.Set(DeliveryHeader, "test-delivery-123")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	mockRepo.AssertExpectations(t)
	mockRepo.AssertNotCalled(t, "Cancel")
}
