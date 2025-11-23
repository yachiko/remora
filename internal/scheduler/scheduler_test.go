package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yachiko/remora/internal/models"
	"go.uber.org/zap"
)

// MockReminderRepository is a mock implementation of database.ReminderRepository
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
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockReminderRepository) FindDueReminders(limit int) ([]*models.Reminder, error) {
	args := m.Called(limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.Reminder), args.Error(1)
}

func (m *MockReminderRepository) GetAndLockDueReminders(limit int) ([]*models.Reminder, error) {
	args := m.Called(limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
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

func (m *MockReminderRepository) Cancel(commentID int64) error {
	args := m.Called(commentID)
	return args.Error(0)
}

func (m *MockReminderRepository) Delete(id uint) error {
	args := m.Called(id)
	return args.Error(0)
}

// MockGitHubClient is a mock implementation of GitHubClient
type MockGitHubClient struct {
	mock.Mock
}

func (m *MockGitHubClient) PostComment(ctx context.Context, installationID int64, owner, repo string, issueNumber int, body string) error {
	args := m.Called(ctx, installationID, owner, repo, issueNumber, body)
	return args.Error(0)
}

func TestNew(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	t.Run("with default config", func(t *testing.T) {
		s := New(repo, github, logger, nil)
		assert.NotNil(t, s)
		assert.Equal(t, 5*time.Minute, s.interval)
		assert.Equal(t, 5, s.maxRetries)
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &Config{
			Interval:   10 * time.Minute,
			MaxRetries: 3,
		}
		s := New(repo, github, logger, cfg)
		assert.NotNil(t, s)
		assert.Equal(t, 10*time.Minute, s.interval)
		assert.Equal(t, 3, s.maxRetries)
	})
}

func TestScheduler_StartStop(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	// Mock FindDueReminders for startup processing
	repo.On("FindDueReminders", 1000).Return([]*models.Reminder{}, nil)

	s := New(repo, github, logger, &Config{
		Interval:   100 * time.Millisecond,
		MaxRetries: 5,
	})

	ctx := context.Background()

	// Start scheduler
	err := s.Start(ctx)
	assert.NoError(t, err)

	// Try to start again (should fail)
	err = s.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Stop scheduler
	err = s.Stop()
	assert.NoError(t, err)

	// Try to stop again (should fail)
	err = s.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")

	repo.AssertExpectations(t)
}

func TestScheduler_ProcessDueReminders_NoneFound(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	// Mock: No due reminders
	repo.On("GetAndLockDueReminders", 100).Return([]*models.Reminder{}, nil)

	ctx := context.Background()
	err := s.processDueReminders(ctx)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestScheduler_ProcessDueReminders_Success(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	now := time.Now()
	reminder := &models.Reminder{
		ID:                1,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		InstallationID:    456,
		RequesterUsername: "testuser",
		RemindAt:          now.Add(-5 * time.Minute),
		OriginalCommand:   "remora in 5 minutes",
		Status:            models.StatusProcessing,
	}

	// Mock: One due reminder
	repo.On("GetAndLockDueReminders", 100).Return([]*models.Reminder{reminder}, nil)

	// Mock: PostComment succeeds
	github.On("PostComment", mock.Anything, int64(456), "owner", "repo", 123, mock.MatchedBy(func(body string) bool {
		return assert.Contains(t, body, "@testuser") && assert.Contains(t, body, "Reminder")
	})).Return(nil)

	// Mock: MarkFired succeeds
	repo.On("MarkFired", uint(1)).Return(nil)

	ctx := context.Background()
	err := s.processDueReminders(ctx)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestScheduler_ProcessDueReminders_PostCommentFails(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	now := time.Now()
	reminder := &models.Reminder{
		ID:                1,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		InstallationID:    456,
		RequesterUsername: "testuser",
		RemindAt:          now.Add(-5 * time.Minute),
		OriginalCommand:   "remora in 5 minutes",
		Status:            models.StatusProcessing,
		RetryCount:        0,
	}

	// Mock: One due reminder
	repo.On("GetAndLockDueReminders", 100).Return([]*models.Reminder{reminder}, nil)

	// Mock: PostComment fails
	postErr := errors.New("github api error")
	github.On("PostComment", mock.Anything, int64(456), "owner", "repo", 123, mock.Anything).Return(postErr)

	// Mock: MarkFailed succeeds
	repo.On("MarkFailed", uint(1), "github api error").Return(nil)

	// Mock: IncrementRetry succeeds (for retry scheduling)
	repo.On("IncrementRetry", uint(1)).Return(nil)

	ctx := context.Background()
	err := s.processDueReminders(ctx)

	assert.NoError(t, err) // Processing continues despite individual reminder failure
	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestScheduler_ProcessDueReminders_MaxRetriesExceeded(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, &Config{
		Interval:   5 * time.Minute,
		MaxRetries: 3,
	})

	now := time.Now()
	reminder := &models.Reminder{
		ID:                1,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		InstallationID:    456,
		RequesterUsername: "testuser",
		RemindAt:          now.Add(-5 * time.Minute),
		OriginalCommand:   "remora in 5 minutes",
		Status:            models.StatusProcessing,
		RetryCount:        3, // Already at max retries
	}

	// Mock: One due reminder
	repo.On("GetAndLockDueReminders", 100).Return([]*models.Reminder{reminder}, nil)

	// Mock: PostComment fails
	postErr := errors.New("github api error")
	github.On("PostComment", mock.Anything, int64(456), "owner", "repo", 123, mock.Anything).Return(postErr)

	// Mock: MarkFailed succeeds
	repo.On("MarkFailed", uint(1), "github api error").Return(nil)

	// IncrementRetry should NOT be called because max retries exceeded

	ctx := context.Background()
	err := s.processDueReminders(ctx)

	assert.NoError(t, err) // Processing continues despite individual reminder failure
	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestScheduler_ProcessOverdueReminders_NoneFound(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	// Mock: No overdue reminders
	repo.On("FindDueReminders", 1000).Return([]*models.Reminder{}, nil)

	ctx := context.Background()
	err := s.processOverdueReminders(ctx)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestScheduler_ProcessOverdueReminders_RecentOverdue(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	now := time.Now()
	reminder := &models.Reminder{
		ID:                1,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		InstallationID:    456,
		RequesterUsername: "testuser",
		RemindAt:          now.Add(-2 * time.Hour), // 2 hours overdue
		OriginalCommand:   "remora in 5 minutes",
		Status:            models.StatusPending,
	}

	// Mock: One overdue reminder
	repo.On("FindDueReminders", 1000).Return([]*models.Reminder{reminder}, nil)

	// Mock: UpdateStatus to processing
	repo.On("UpdateStatus", uint(1), models.StatusProcessing).Return(nil)

	// Mock: PostComment succeeds with delay annotation
	github.On("PostComment", mock.Anything, int64(456), "owner", "repo", 123, mock.MatchedBy(func(body string) bool {
		return assert.Contains(t, body, "@testuser") &&
			assert.Contains(t, body, "Reminder") &&
			assert.Contains(t, body, "delayed by")
	})).Return(nil)

	// Mock: MarkFired succeeds
	repo.On("MarkFired", uint(1)).Return(nil)

	ctx := context.Background()
	err := s.processOverdueReminders(ctx)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestScheduler_ProcessOverdueReminders_Expired(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	now := time.Now()
	reminder := &models.Reminder{
		ID:                1,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		InstallationID:    456,
		RequesterUsername: "testuser",
		RemindAt:          now.Add(-48 * time.Hour), // 48 hours overdue (expired)
		OriginalCommand:   "remora in 5 minutes",
		Status:            models.StatusPending,
	}

	// Mock: One expired reminder
	repo.On("FindDueReminders", 1000).Return([]*models.Reminder{reminder}, nil)

	// Mock: MarkFailed with expiration message
	repo.On("MarkFailed", uint(1), "expired: reminder over 24 hours overdue").Return(nil)

	// PostComment should NOT be called for expired reminders

	ctx := context.Background()
	err := s.processOverdueReminders(ctx)

	assert.NoError(t, err)
	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestScheduler_BuildReminderComment(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	t.Run("on time", func(t *testing.T) {
		reminder := &models.Reminder{
			RequesterUsername: "testuser",
			RemindAt:          time.Now(),
			OriginalCommand:   "remora in 5 minutes",
		}

		comment := s.buildReminderComment(reminder)
		assert.Contains(t, comment, "@testuser")
		assert.Contains(t, comment, "Reminder")
		assert.Contains(t, comment, "remora in 5 minutes")
		assert.NotContains(t, comment, "delayed")
	})

	t.Run("delayed", func(t *testing.T) {
		reminder := &models.Reminder{
			RequesterUsername: "testuser",
			RemindAt:          time.Now().Add(-10 * time.Minute),
			OriginalCommand:   "remora in 5 minutes",
		}

		comment := s.buildReminderComment(reminder)
		assert.Contains(t, comment, "@testuser")
		assert.Contains(t, comment, "Reminder")
		assert.Contains(t, comment, "remora in 5 minutes")
		assert.Contains(t, comment, "delayed by")
	})
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "seconds",
			duration: 30 * time.Second,
			want:     "30 seconds",
		},
		{
			name:     "minutes",
			duration: 5 * time.Minute,
			want:     "5 minutes",
		},
		{
			name:     "hours",
			duration: 2 * time.Hour,
			want:     "2 hours",
		},
		{
			name:     "days",
			duration: 48 * time.Hour,
			want:     "2 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestScheduler_ScheduleRetry(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	tests := []struct {
		name       string
		retryCount int
	}{
		{
			name:       "first retry",
			retryCount: 0,
		},
		{
			name:       "second retry",
			retryCount: 1,
		},
		{
			name:       "third retry",
			retryCount: 2,
		},
		{
			name:       "fourth retry",
			retryCount: 3,
		},
		{
			name:       "fifth retry",
			retryCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reminder := &models.Reminder{
				ID:         1,
				RetryCount: tt.retryCount,
			}

			repo.On("IncrementRetry", uint(1)).Return(nil).Once()

			err := s.scheduleRetry(reminder)
			assert.NoError(t, err)

			repo.AssertExpectations(t)
		})
	}
}

func TestScheduler_ProcessReminder_MarkFiredFails(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	s := New(repo, github, logger, nil)

	now := time.Now()
	reminder := &models.Reminder{
		ID:                1,
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		InstallationID:    456,
		RequesterUsername: "testuser",
		RemindAt:          now,
		OriginalCommand:   "remora in 5 minutes",
		Status:            models.StatusProcessing,
	}

	// Mock: PostComment succeeds
	github.On("PostComment", mock.Anything, int64(456), "owner", "repo", 123, mock.Anything).Return(nil)

	// Mock: MarkFired fails
	markErr := errors.New("database error")
	repo.On("MarkFired", uint(1)).Return(markErr)

	ctx := context.Background()
	err := s.processReminder(ctx, reminder)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark reminder as fired")

	repo.AssertExpectations(t)
	github.AssertExpectations(t)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 5*time.Minute, cfg.Interval)
	assert.Equal(t, 5, cfg.MaxRetries)
}

func TestScheduler_GracefulShutdown(t *testing.T) {
	repo := new(MockReminderRepository)
	github := new(MockGitHubClient)
	logger := zap.NewNop()

	// Mock FindDueReminders for startup
	repo.On("FindDueReminders", 1000).Return([]*models.Reminder{}, nil)

	// Mock GetAndLockDueReminders for ticker calls (may be called multiple times)
	repo.On("GetAndLockDueReminders", 100).Return([]*models.Reminder{}, nil).Maybe()

	s := New(repo, github, logger, &Config{
		Interval:   50 * time.Millisecond,
		MaxRetries: 5,
	})

	ctx := context.Background()

	// Start scheduler
	err := s.Start(ctx)
	assert.NoError(t, err)

	// Let it run for a bit
	time.Sleep(150 * time.Millisecond)

	// Stop scheduler
	err = s.Stop()
	assert.NoError(t, err)

	// Verify it stopped cleanly
	assert.False(t, s.running)
}
