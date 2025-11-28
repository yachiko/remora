package database

import (
	"testing"
	"time"

	"github.com/yachiko/remora/internal/config"
	"github.com/yachiko/remora/internal/logger"
	"github.com/yachiko/remora/internal/models"
)

func setupTestDB(t *testing.T) {
	t.Helper()

	// Initialize logger for tests
	if err := logger.Initialize("development", "error"); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Use SQLite in-memory for testing
	cfg := &config.Config{
		DatabaseType:       "sqlite",
		DatabaseSQLitePath: ":memory:",
		LogLevel:           "error",
	}

	if err := Initialize(cfg); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
}

func teardownTestDB(t *testing.T) {
	t.Helper()
	if err := Close(); err != nil {
		t.Errorf("Failed to close database: %v", err)
	}
}

func TestInitialize(t *testing.T) {
	if err := logger.Initialize("development", "error"); err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	tests := []struct {
		name    string
		cfg     *config.Config
		wantErr bool
	}{
		{
			name: "sqlite in-memory",
			cfg: &config.Config{
				DatabaseType:       "sqlite",
				DatabaseSQLitePath: ":memory:",
				LogLevel:           "error",
			},
			wantErr: false,
		},
		{
			name: "sqlite with debug logging",
			cfg: &config.Config{
				DatabaseType:       "sqlite",
				DatabaseSQLitePath: ":memory:",
				LogLevel:           "debug",
			},
			wantErr: false,
		},
		{
			name: "unsupported database type",
			cfg: &config.Config{
				DatabaseType: "mongodb",
				DatabaseName: "test",
				LogLevel:     "error",
			},
			wantErr: true,
		},
		{
			name: "postgresql invalid connection",
			cfg: &config.Config{
				DatabaseType:     "postgresql",
				DatabaseHost:     "invalid-host-that-does-not-exist.local",
				DatabasePort:     5432,
				DatabaseName:     "remora",
				DatabaseUser:     "testuser",
				DatabasePassword: "testpass",
				DatabaseSSLMode:  "disable",
				LogLevel:         "error",
			},
			wantErr: true,
		},
		{
			name: "postgres alias invalid connection",
			cfg: &config.Config{
				DatabaseType:     "postgres",
				DatabaseHost:     "invalid-host-that-does-not-exist.local",
				DatabasePort:     5432,
				DatabaseName:     "remora",
				DatabaseUser:     "testuser",
				DatabasePassword: "testpass",
				DatabaseSSLMode:  "disable",
				LogLevel:         "error",
			},
			wantErr: true,
		},
		{
			name: "mysql invalid connection",
			cfg: &config.Config{
				DatabaseType:     "mysql",
				DatabaseHost:     "invalid-host-that-does-not-exist.local",
				DatabasePort:     3306,
				DatabaseName:     "remora",
				DatabaseUser:     "testuser",
				DatabasePassword: "testpass",
				LogLevel:         "error",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Initialize(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Initialize() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify database is accessible
				if DB == nil {
					t.Error("Initialize() succeeded but DB is nil")
				}

				// Clean up
				if err := Close(); err != nil {
					t.Errorf("Failed to close database: %v", err)
				}
				DB = nil
			}
		})
	}
}

func TestAutoMigrate(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	// AutoMigrate is called in Initialize, so just verify table exists
	var count int64
	err := DB.Model(&models.Reminder{}).Count(&count).Error
	if err != nil {
		t.Errorf("Failed to count reminders (table may not exist): %v", err)
	}
}

func TestHealthCheck(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	if err := HealthCheck(); err != nil {
		t.Errorf("HealthCheck() failed: %v", err)
	}
}

func TestHealthCheck_NoConnection(t *testing.T) {
	// Don't initialize DB
	DB = nil

	err := HealthCheck()
	if err == nil {
		t.Error("HealthCheck() should fail when DB is nil")
	}
}

func TestRepositoryCreate(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment-456",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now().Add(24 * time.Hour),
		OriginalCommand:   "remora 1 day",
		Status:            models.StatusPending,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if reminder.ID == 0 {
		t.Error("Create() did not set ID")
	}
}

func TestRepositoryFindByID(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	// Create a reminder first
	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment-456",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now().Add(24 * time.Hour),
		OriginalCommand:   "remora 1 day",
		Status:            models.StatusPending,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Find it
	found, err := repo.FindByID(reminder.ID)
	if err != nil {
		t.Fatalf("FindByID() failed: %v", err)
	}

	if found.ID != reminder.ID {
		t.Errorf("FindByID() ID = %v, want %v", found.ID, reminder.ID)
	}
	if found.CommentID != reminder.CommentID {
		t.Errorf("FindByID() CommentID = %v, want %v", found.CommentID, reminder.CommentID)
	}
}

func TestRepositoryFindDueReminders(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	// Create reminders - some due, some not
	pastTime := time.Now().Add(-1 * time.Hour)
	futureTime := time.Now().Add(1 * time.Hour)

	reminders := []*models.Reminder{
		{
			RepositoryOwner:   "owner",
			RepositoryName:    "repo",
			IssueNumber:       1,
			CommentID:         100,
			CommentURL:        "https://github.com/owner/repo/issues/1#issuecomment-100",
			RequesterUsername: "user1",
			RequesterID:       1,
			RemindAt:          pastTime,
			OriginalCommand:   "remora 1 hour ago",
			Status:            models.StatusPending,
		},
		{
			RepositoryOwner:   "owner",
			RepositoryName:    "repo",
			IssueNumber:       2,
			CommentID:         200,
			CommentURL:        "https://github.com/owner/repo/issues/2#issuecomment-200",
			RequesterUsername: "user2",
			RequesterID:       2,
			RemindAt:          futureTime,
			OriginalCommand:   "remora 1 hour",
			Status:            models.StatusPending,
		},
	}

	for _, r := range reminders {
		if err := repo.Create(r); err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
	}

	// Find due reminders
	due, err := repo.FindDueReminders(10)
	if err != nil {
		t.Fatalf("FindDueReminders() failed: %v", err)
	}

	if len(due) != 1 {
		t.Errorf("FindDueReminders() returned %d reminders, want 1", len(due))
	}

	if len(due) > 0 && due[0].CommentID != 100 {
		t.Errorf("FindDueReminders() returned wrong reminder, got CommentID %d, want 100", due[0].CommentID)
	}
}

func TestRepositoryMarkFired(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment-456",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now(),
		OriginalCommand:   "remora now",
		Status:            models.StatusProcessing,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := repo.MarkFired(reminder.ID); err != nil {
		t.Fatalf("MarkFired() failed: %v", err)
	}

	// Verify status changed
	updated, err := repo.FindByID(reminder.ID)
	if err != nil {
		t.Fatalf("FindByID() failed: %v", err)
	}

	if updated.Status != models.StatusFired {
		t.Errorf("MarkFired() status = %v, want %v", updated.Status, models.StatusFired)
	}

	if updated.FiredAt == nil {
		t.Error("MarkFired() did not set FiredAt")
	}
}

func TestRepositoryGetAndLockDueReminders(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	// Create pending reminders
	pastTime := time.Now().Add(-1 * time.Hour)

	for i := 0; i < 3; i++ {
		reminder := &models.Reminder{
			RepositoryOwner:   "owner",
			RepositoryName:    "repo",
			IssueNumber:       i + 1,
			CommentID:         int64(100 + i),
			CommentURL:        "https://github.com/owner/repo/issues/1#issuecomment-100",
			RequesterUsername: "user",
			RequesterID:       789,
			RemindAt:          pastTime,
			OriginalCommand:   "remora 1 hour ago",
			Status:            models.StatusPending,
		}

		if err := repo.Create(reminder); err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
	}

	// Get and lock
	locked, err := repo.GetAndLockDueReminders(10)
	if err != nil {
		t.Fatalf("GetAndLockDueReminders() failed: %v", err)
	}

	if len(locked) != 3 {
		t.Errorf("GetAndLockDueReminders() returned %d reminders, want 3", len(locked))
	}

	// Verify all are marked as processing
	for _, r := range locked {
		if r.Status != models.StatusProcessing {
			t.Errorf("Locked reminder status = %v, want %v", r.Status, models.StatusProcessing)
		}
	}
}

func TestRepositoryFindByCommentID(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456789,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment-456789",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now().Add(24 * time.Hour),
		OriginalCommand:   "remora 1 day",
		Status:            models.StatusPending,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	found, err := repo.FindByCommentID(456789)
	if err != nil {
		t.Fatalf("FindByCommentID() failed: %v", err)
	}

	if found.ID != reminder.ID {
		t.Errorf("FindByCommentID() ID = %v, want %v", found.ID, reminder.ID)
	}
}

func TestRepositoryFindByCommentID_NotFound(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	_, err := repo.FindByCommentID(999999)
	if err == nil {
		t.Error("FindByCommentID() should return error for non-existent comment")
	}
}

func TestRepositoryFindByIssue(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	// Create reminders for same issue
	for i := 0; i < 3; i++ {
		reminder := &models.Reminder{
			RepositoryOwner:   "owner",
			RepositoryName:    "repo",
			IssueNumber:       123,
			CommentID:         int64(100 + i),
			CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment",
			RequesterUsername: "user",
			RequesterID:       789,
			RemindAt:          time.Now().Add(time.Duration(i+1) * time.Hour),
			OriginalCommand:   "remora test",
			Status:            models.StatusPending,
		}
		if err := repo.Create(reminder); err != nil {
			t.Fatalf("Create() failed: %v", err)
		}
	}

	// Create reminder for different issue
	other := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       456,
		CommentID:         999,
		CommentURL:        "https://github.com/owner/repo/issues/456#issuecomment",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now().Add(time.Hour),
		OriginalCommand:   "remora test",
		Status:            models.StatusPending,
	}
	if err := repo.Create(other); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	found, err := repo.FindByIssue("owner", "repo", 123)
	if err != nil {
		t.Fatalf("FindByIssue() failed: %v", err)
	}

	if len(found) != 3 {
		t.Errorf("FindByIssue() returned %d reminders, want 3", len(found))
	}
}

func TestRepositoryUpdateStatus(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now(),
		OriginalCommand:   "remora now",
		Status:            models.StatusPending,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := repo.UpdateStatus(reminder.ID, models.StatusProcessing); err != nil {
		t.Fatalf("UpdateStatus() failed: %v", err)
	}

	updated, err := repo.FindByID(reminder.ID)
	if err != nil {
		t.Fatalf("FindByID() failed: %v", err)
	}

	if updated.Status != models.StatusProcessing {
		t.Errorf("UpdateStatus() status = %v, want %v", updated.Status, models.StatusProcessing)
	}
}

func TestRepositoryMarkFailed(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now(),
		OriginalCommand:   "remora now",
		Status:            models.StatusProcessing,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	errorMsg := "API rate limit exceeded"
	if err := repo.MarkFailed(reminder.ID, errorMsg); err != nil {
		t.Fatalf("MarkFailed() failed: %v", err)
	}

	updated, err := repo.FindByID(reminder.ID)
	if err != nil {
		t.Fatalf("FindByID() failed: %v", err)
	}

	if updated.Status != models.StatusFailed {
		t.Errorf("MarkFailed() status = %v, want %v", updated.Status, models.StatusFailed)
	}

	if updated.ErrorMessage != errorMsg {
		t.Errorf("MarkFailed() error_message = %v, want %v", updated.ErrorMessage, errorMsg)
	}
}

func TestRepositoryIncrementRetry(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now(),
		OriginalCommand:   "remora now",
		Status:            models.StatusProcessing,
		RetryCount:        0,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := repo.IncrementRetry(reminder.ID); err != nil {
		t.Fatalf("IncrementRetry() failed: %v", err)
	}

	updated, err := repo.FindByID(reminder.ID)
	if err != nil {
		t.Fatalf("FindByID() failed: %v", err)
	}

	if updated.Status != models.StatusPending {
		t.Errorf("IncrementRetry() status = %v, want %v", updated.Status, models.StatusPending)
	}

	if updated.RetryCount != 1 {
		t.Errorf("IncrementRetry() retry_count = %v, want 1", updated.RetryCount)
	}
}

func TestRepositoryCancel(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now().Add(time.Hour),
		OriginalCommand:   "remora 1 hour",
		Status:            models.StatusPending,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := repo.Cancel(456); err != nil {
		t.Fatalf("Cancel() failed: %v", err)
	}

	updated, err := repo.FindByID(reminder.ID)
	if err != nil {
		t.Fatalf("FindByID() failed: %v", err)
	}

	if updated.Status != models.StatusCancelled {
		t.Errorf("Cancel() status = %v, want %v", updated.Status, models.StatusCancelled)
	}
}

func TestRepositoryDelete(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	reminder := &models.Reminder{
		RepositoryOwner:   "owner",
		RepositoryName:    "repo",
		IssueNumber:       123,
		CommentID:         456,
		CommentURL:        "https://github.com/owner/repo/issues/123#issuecomment",
		RequesterUsername: "user",
		RequesterID:       789,
		RemindAt:          time.Now(),
		OriginalCommand:   "remora now",
		Status:            models.StatusPending,
	}

	if err := repo.Create(reminder); err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	if err := repo.Delete(reminder.ID); err != nil {
		t.Fatalf("Delete() failed: %v", err)
	}

	// Soft delete - should not find with standard query
	_, err := repo.FindByID(reminder.ID)
	if err == nil {
		t.Error("Delete() should make record unfindable")
	}
}

func TestRepositoryFindByID_NotFound(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	repo := NewReminderRepository(DB)

	_, err := repo.FindByID(999999)
	if err == nil {
		t.Error("FindByID() should return error for non-existent ID")
	}
}
