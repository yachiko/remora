package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/yachiko/remora/internal/database"
	"github.com/yachiko/remora/internal/logger"
	"github.com/yachiko/remora/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Run migrations
	err = db.AutoMigrate(&models.Reminder{})
	require.NoError(t, err)

	return db
}

func TestListReminders_MethodNotAllowed(t *testing.T) {
	_ = logger.Initialize("test", "info")
	mockRepo := new(MockReminderRepository)
	handler := NewHandler(mockRepo, logger.Logger)

	req := httptest.NewRequest("POST", "/api/v1/reminders", nil)
	rr := httptest.NewRecorder()

	handler.ListReminders(rr, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	var response map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &response)
	assert.Equal(t, "method not allowed", response["error"])
}

func TestListReminders_Success(t *testing.T) {
	_ = logger.Initialize("test", "info")
	db := setupTestDB(t)
	database.DB = db

	// Create test reminders
	now := time.Now()
	reminders := []*models.Reminder{
		{
			RepositoryOwner:   "owner1",
			RepositoryName:    "repo1",
			IssueNumber:       1,
			RequesterUsername: "user1",
			Status:            models.StatusPending,
			RemindAt:          now.Add(24 * time.Hour),
			OriginalCommand:   "remora tomorrow",
		},
		{
			RepositoryOwner:   "owner2",
			RepositoryName:    "repo2",
			IssueNumber:       2,
			RequesterUsername: "user2",
			Status:            models.StatusFired,
			RemindAt:          now.Add(-1 * time.Hour),
			OriginalCommand:   "remora 1 hour ago",
		},
	}

	for _, r := range reminders {
		err := db.Create(r).Error
		require.NoError(t, err)
	}

	mockRepo := new(MockReminderRepository)
	handler := NewHandler(mockRepo, logger.Logger)

	req := httptest.NewRequest("GET", "/api/v1/reminders", nil)
	rr := httptest.NewRecorder()

	handler.ListReminders(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	data := response["data"].([]interface{})
	assert.Len(t, data, 2)

	meta := response["meta"].(map[string]interface{})
	assert.Equal(t, float64(2), meta["total"])
	assert.Equal(t, float64(50), meta["limit"])
	assert.Equal(t, float64(0), meta["offset"])
}

func TestListReminders_WithFilters(t *testing.T) {
	_ = logger.Initialize("test", "info")
	db := setupTestDB(t)
	database.DB = db

	// Create test reminders
	now := time.Now()
	reminders := []*models.Reminder{
		{
			RepositoryOwner:   "owner1",
			RepositoryName:    "repo1",
			IssueNumber:       1,
			RequesterUsername: "user1",
			Status:            models.StatusPending,
			RemindAt:          now.Add(24 * time.Hour),
			OriginalCommand:   "remora tomorrow",
		},
		{
			RepositoryOwner:   "owner1",
			RepositoryName:    "repo1",
			IssueNumber:       2,
			RequesterUsername: "user2",
			Status:            models.StatusPending,
			RemindAt:          now.Add(48 * time.Hour),
			OriginalCommand:   "remora 2 days",
		},
		{
			RepositoryOwner:   "owner2",
			RepositoryName:    "repo2",
			IssueNumber:       3,
			RequesterUsername: "user1",
			Status:            models.StatusFired,
			RemindAt:          now.Add(-1 * time.Hour),
			OriginalCommand:   "remora yesterday",
		},
	}

	for _, r := range reminders {
		err := db.Create(r).Error
		require.NoError(t, err)
	}

	mockRepo := new(MockReminderRepository)
	handler := NewHandler(mockRepo, logger.Logger)

	tests := []struct {
		name          string
		query         string
		expectedCount int
	}{
		{
			name:          "filter by repository",
			query:         "?repository=owner1/repo1",
			expectedCount: 2,
		},
		{
			name:          "filter by status",
			query:         "?status=fired",
			expectedCount: 1,
		},
		{
			name:          "filter by user",
			query:         "?user=user1",
			expectedCount: 2,
		},
		{
			name:          "filter by issue",
			query:         "?issue=1",
			expectedCount: 1,
		},
		{
			name:          "combined filters",
			query:         "?repository=owner1/repo1&status=pending",
			expectedCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/reminders"+tt.query, nil)
			rr := httptest.NewRecorder()

			handler.ListReminders(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)

			var response map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			require.NoError(t, err)

			data := response["data"].([]interface{})
			assert.Len(t, data, tt.expectedCount)
		})
	}
}

func TestListReminders_Pagination(t *testing.T) {
	_ = logger.Initialize("test", "info")
	db := setupTestDB(t)
	database.DB = db

	// Create 15 test reminders
	now := time.Now()
	for i := 1; i <= 15; i++ {
		reminder := &models.Reminder{
			RepositoryOwner:   "owner",
			RepositoryName:    "repo",
			IssueNumber:       i,
			RequesterUsername: "user",
			Status:            models.StatusPending,
			RemindAt:          now.Add(time.Duration(i) * time.Hour),
			OriginalCommand:   "remora test",
		}
		err := db.Create(reminder).Error
		require.NoError(t, err)
	}

	mockRepo := new(MockReminderRepository)
	handler := NewHandler(mockRepo, logger.Logger)

	tests := []struct {
		name          string
		query         string
		expectedCount int
		expectedTotal float64
	}{
		{
			name:          "default limit",
			query:         "",
			expectedCount: 15,
			expectedTotal: 15,
		},
		{
			name:          "custom limit",
			query:         "?limit=5",
			expectedCount: 5,
			expectedTotal: 15,
		},
		{
			name:          "with offset",
			query:         "?limit=5&offset=10",
			expectedCount: 5,
			expectedTotal: 15,
		},
		{
			name:          "limit exceeds max",
			query:         "?limit=200",
			expectedCount: 15, // default 50, but only 15 exist
			expectedTotal: 15,
		},
		{
			name:          "invalid limit ignored",
			query:         "?limit=-1",
			expectedCount: 15, // uses default
			expectedTotal: 15,
		},
		{
			name:          "invalid offset ignored",
			query:         "?offset=-5",
			expectedCount: 15, // uses default offset 0
			expectedTotal: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/reminders"+tt.query, nil)
			rr := httptest.NewRecorder()

			handler.ListReminders(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)

			var response map[string]interface{}
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			require.NoError(t, err)

			data := response["data"].([]interface{})
			assert.Len(t, data, tt.expectedCount)

			meta := response["meta"].(map[string]interface{})
			assert.Equal(t, tt.expectedTotal, meta["total"])
		})
	}
}

func TestNewHandler(t *testing.T) {
	_ = logger.Initialize("test", "info")
	mockRepo := new(MockReminderRepository)
	handler := NewHandler(mockRepo, logger.Logger)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.logger)
}
