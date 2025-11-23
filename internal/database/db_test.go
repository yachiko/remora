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

	// Use SQLite for testing
	cfg := &config.Config{
		DatabaseType: "sqlite",
		DatabaseName: ":memory:",
		LogLevel:     "error",
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
				DatabaseType: "sqlite",
				DatabaseName: ":memory:",
				LogLevel:     "error",
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
