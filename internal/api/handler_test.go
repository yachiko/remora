package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yachiko/remora/internal/logger"
	"github.com/yachiko/remora/internal/models"
)

type MockReminderRepository struct {
	mock.Mock
}

func (m *MockReminderRepository) Create(reminder *models.Reminder) error {
	args := m.Called(reminder)
	return args.Error(0)
}

func (m *MockReminderRepository) FindByID(id uint) (*models.Reminder, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Reminder), args.Error(1)
}

func (m *MockReminderRepository) FindByCommentID(commentID int64) (*models.Reminder, error) {
	args := m.Called(commentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Reminder), args.Error(1)
}

func (m *MockReminderRepository) FindByIssue(owner, repo string, issueNumber int) ([]*models.Reminder, error) {
	args := m.Called(owner, repo, issueNumber)
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockReminderRepository) FindDueReminders(limit int) ([]*models.Reminder, error) {
	args := m.Called(limit)
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockReminderRepository) GetAndLockDueReminders(limit int) ([]*models.Reminder, error) {
	args := m.Called(limit)
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockReminderRepository) UpdateStatus(id uint, status models.ReminderStatus) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockReminderRepository) MarkFired(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockReminderRepository) MarkFailed(id uint, errorMsg string) error {
	args := m.Called(id, errorMsg)
	return args.Error(0)
}

func (m *MockReminderRepository) IncrementRetry(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockReminderRepository) Cancel(id int64) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockReminderRepository) Delete(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

func TestListReminders_MethodNotAllowed(t *testing.T) {
	logger.Initialize("test", "info")
	mockRepo := new(MockReminderRepository)
	handler := NewHandler(mockRepo, logger.Logger)

	req := httptest.NewRequest("POST", "/api/v1/reminders", nil)
	rr := httptest.NewRecorder()

	handler.ListReminders(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	var response map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Equal(t, "method not allowed", response["error"])
}
