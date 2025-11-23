package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yachiko/remora/internal/database"
	"github.com/yachiko/remora/internal/models"
	"go.uber.org/zap"
)

// GitHubClient defines the interface for GitHub API operations
type GitHubClient interface {
	// PostComment posts a comment to a GitHub issue
	PostComment(ctx context.Context, installationID int64, owner, repo string, issueNumber int, body string) error
}

// Scheduler is responsible for firing reminders on time
type Scheduler struct {
	repo       database.ReminderRepository
	github     GitHubClient
	logger     *zap.Logger
	interval   time.Duration
	maxRetries int
	ticker     *time.Ticker
	done       chan struct{}
	wg         sync.WaitGroup
	mu         sync.Mutex
	running    bool
}

// Config holds scheduler configuration
type Config struct {
	Interval   time.Duration // How often to poll for due reminders
	MaxRetries int           // Maximum retry attempts for failed reminders
}

// DefaultConfig returns default scheduler configuration
func DefaultConfig() *Config {
	return &Config{
		Interval:   5 * time.Minute,
		MaxRetries: 5,
	}
}

// New creates a new scheduler instance
func New(repo database.ReminderRepository, github GitHubClient, logger *zap.Logger, cfg *Config) *Scheduler {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Scheduler{
		repo:       repo,
		github:     github,
		logger:     logger,
		interval:   cfg.Interval,
		maxRetries: cfg.MaxRetries,
		done:       make(chan struct{}),
	}
}

// Start begins the scheduler's polling loop
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info("starting scheduler",
		zap.Duration("interval", s.interval),
		zap.Int("max_retries", s.maxRetries))

	// Process overdue reminders on startup
	if err := s.processOverdueReminders(ctx); err != nil {
		s.logger.Error("failed to process overdue reminders on startup",
			zap.Error(err))
	}

	// Start ticker
	s.ticker = time.NewTicker(s.interval)

	// Start polling loop in goroutine
	s.wg.Add(1)
	go s.run(ctx)

	return nil
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return fmt.Errorf("scheduler not running")
	}
	s.mu.Unlock()

	s.logger.Info("stopping scheduler")

	// Stop ticker
	if s.ticker != nil {
		s.ticker.Stop()
	}

	// Signal goroutine to stop
	close(s.done)

	// Wait for goroutine to finish
	s.wg.Wait()

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	s.logger.Info("scheduler stopped")
	return nil
}

// run is the main polling loop
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-s.done:
			s.logger.Debug("scheduler loop stopped")
			return
		case <-s.ticker.C:
			s.logger.Debug("scheduler tick: checking for due reminders")
			if err := s.processDueReminders(ctx); err != nil {
				s.logger.Error("error processing due reminders",
					zap.Error(err))
			}
		case <-ctx.Done():
			s.logger.Info("scheduler context cancelled")
			return
		}
	}
}

// processDueReminders queries and processes all due reminders
func (s *Scheduler) processDueReminders(ctx context.Context) error {
	// Get due reminders with lock (atomically updates status to processing)
	reminders, err := s.repo.GetAndLockDueReminders(100)
	if err != nil {
		return fmt.Errorf("failed to get due reminders: %w", err)
	}

	if len(reminders) == 0 {
		s.logger.Debug("no due reminders to process")
		return nil
	}

	s.logger.Info("processing due reminders",
		zap.Int("count", len(reminders)))

	// Process reminders sequentially
	for _, reminder := range reminders {
		if err := s.processReminder(ctx, reminder); err != nil {
			s.logger.Error("failed to process reminder",
				zap.Uint("reminder_id", reminder.ID),
				zap.Error(err))
		}
	}

	return nil
}

// processReminder executes a single reminder
func (s *Scheduler) processReminder(ctx context.Context, reminder *models.Reminder) error {
	s.logger.Info("processing reminder",
		zap.Uint("reminder_id", reminder.ID),
		zap.String("repository", reminder.RepositoryFullName()),
		zap.Int("issue_number", reminder.IssueNumber),
		zap.String("requester", reminder.RequesterUsername),
		zap.Time("remind_at", reminder.RemindAt))

	// Build reminder comment
	comment := s.buildReminderComment(reminder)

	// Post comment via GitHub API
	err := s.github.PostComment(ctx, reminder.InstallationID, reminder.RepositoryOwner, reminder.RepositoryName, reminder.IssueNumber, comment)
	if err != nil {
		// Mark as failed
		if markErr := s.repo.MarkFailed(reminder.ID, err.Error()); markErr != nil {
			s.logger.Error("failed to mark reminder as failed",
				zap.Uint("reminder_id", reminder.ID),
				zap.Error(markErr))
		}

		// Schedule retry if eligible
		if reminder.CanRetry(s.maxRetries) {
			if retryErr := s.scheduleRetry(reminder); retryErr != nil {
				s.logger.Error("failed to schedule retry",
					zap.Uint("reminder_id", reminder.ID),
					zap.Error(retryErr))
			}
		} else {
			s.logger.Warn("reminder exceeded max retries",
				zap.Uint("reminder_id", reminder.ID),
				zap.Int("retry_count", reminder.RetryCount))
		}

		return fmt.Errorf("failed to post comment: %w", err)
	}

	// Mark as fired
	if err := s.repo.MarkFired(reminder.ID); err != nil {
		return fmt.Errorf("failed to mark reminder as fired: %w", err)
	}

	s.logger.Info("reminder fired successfully",
		zap.Uint("reminder_id", reminder.ID),
		zap.String("repository", reminder.RepositoryFullName()),
		zap.Int("issue_number", reminder.IssueNumber))

	return nil
}

// buildReminderComment creates the reminder comment text
func (s *Scheduler) buildReminderComment(reminder *models.Reminder) string {
	now := time.Now()
	delay := now.Sub(reminder.RemindAt)

	var delayNote string
	if delay > 1*time.Minute {
		delayNote = fmt.Sprintf(" *(delayed by %s)*", formatDuration(delay))
	}

	return fmt.Sprintf("@%s Reminder: %s%s",
		reminder.RequesterUsername,
		reminder.OriginalCommand,
		delayNote)
}

// processOverdueReminders handles reminders that are overdue on startup
func (s *Scheduler) processOverdueReminders(ctx context.Context) error {
	s.logger.Info("checking for overdue reminders on startup")

	// Find all pending reminders that are overdue
	reminders, err := s.repo.FindDueReminders(1000)
	if err != nil {
		return fmt.Errorf("failed to find overdue reminders: %w", err)
	}

	if len(reminders) == 0 {
		s.logger.Info("no overdue reminders found")
		return nil
	}

	s.logger.Info("found overdue reminders",
		zap.Int("count", len(reminders)))

	now := time.Now()
	expiredCount := 0
	processedCount := 0

	for _, reminder := range reminders {
		age := now.Sub(reminder.RemindAt)

		// Skip reminders older than 24 hours
		if age > 24*time.Hour {
			s.logger.Warn("reminder too old, marking as expired",
				zap.Uint("reminder_id", reminder.ID),
				zap.Duration("age", age))

			// Mark as failed with expiration message
			if err := s.repo.MarkFailed(reminder.ID, "expired: reminder over 24 hours overdue"); err != nil {
				s.logger.Error("failed to mark reminder as expired",
					zap.Uint("reminder_id", reminder.ID),
					zap.Error(err))
			}

			expiredCount++
			continue
		}

		// Process recent overdue reminders
		s.logger.Info("processing overdue reminder",
			zap.Uint("reminder_id", reminder.ID),
			zap.Duration("overdue_by", age))

		// Update status to processing
		if err := s.repo.UpdateStatus(reminder.ID, models.StatusProcessing); err != nil {
			s.logger.Error("failed to update reminder status",
				zap.Uint("reminder_id", reminder.ID),
				zap.Error(err))
			continue
		}

		if err := s.processReminder(ctx, reminder); err != nil {
			s.logger.Error("failed to process overdue reminder",
				zap.Uint("reminder_id", reminder.ID),
				zap.Error(err))
		} else {
			processedCount++
		}
	}

	s.logger.Info("overdue reminder processing complete",
		zap.Int("total", len(reminders)),
		zap.Int("processed", processedCount),
		zap.Int("expired", expiredCount))

	return nil
}

// scheduleRetry increments retry count and resets status to pending
func (s *Scheduler) scheduleRetry(reminder *models.Reminder) error {
	// Calculate backoff delay: 1, 2, 4, 8, 16 minutes
	backoffMinutes := 1 << reminder.RetryCount // 2^retry_count
	if backoffMinutes > 16 {
		backoffMinutes = 16
	}

	s.logger.Info("scheduling retry",
		zap.Uint("reminder_id", reminder.ID),
		zap.Int("retry_count", reminder.RetryCount+1),
		zap.Int("backoff_minutes", backoffMinutes))

	// Increment retry count and reset to pending
	if err := s.repo.IncrementRetry(reminder.ID); err != nil {
		return fmt.Errorf("failed to increment retry: %w", err)
	}

	return nil
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%d days", days)
}
