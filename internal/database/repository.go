package database

import (
	"time"

	"github.com/yachiko/remora/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReminderRepository defines the interface for reminder database operations
type ReminderRepository interface {
	// Create creates a new reminder
	Create(reminder *models.Reminder) error

	// FindByID retrieves a reminder by its ID
	FindByID(id uint) (*models.Reminder, error)

	// FindByCommentID retrieves a reminder by GitHub comment ID
	FindByCommentID(commentID int64) (*models.Reminder, error)

	// FindByIssue retrieves all reminders for a specific issue
	FindByIssue(owner, repo string, issueNumber int) ([]*models.Reminder, error)

	// FindDueReminders retrieves reminders that are due to be fired
	FindDueReminders(limit int) ([]*models.Reminder, error)

	// GetAndLockDueReminders atomically fetches and locks due reminders for processing
	GetAndLockDueReminders(limit int) ([]*models.Reminder, error)

	// UpdateStatus updates the status of a reminder
	UpdateStatus(id uint, status models.ReminderStatus) error

	// MarkFired marks a reminder as successfully fired
	MarkFired(id uint) error

	// MarkFailed marks a reminder as failed with error message
	MarkFailed(id uint, errorMsg string) error

	// IncrementRetry increments the retry count and resets to pending
	IncrementRetry(id uint) error

	// Cancel cancels a reminder
	Cancel(commentID int64) error

	// Delete soft-deletes a reminder
	Delete(id uint) error
}

// reminderRepository implements ReminderRepository
type reminderRepository struct {
	db *gorm.DB
}

// NewReminderRepository creates a new reminder repository
func NewReminderRepository(db *gorm.DB) ReminderRepository {
	return &reminderRepository{db: db}
}

// Create creates a new reminder
func (r *reminderRepository) Create(reminder *models.Reminder) error {
	return r.db.Create(reminder).Error
}

// FindByID retrieves a reminder by its ID
func (r *reminderRepository) FindByID(id uint) (*models.Reminder, error) {
	var reminder models.Reminder
	err := r.db.First(&reminder, id).Error
	if err != nil {
		return nil, err
	}
	return &reminder, nil
}

// FindByCommentID retrieves a reminder by GitHub comment ID
func (r *reminderRepository) FindByCommentID(commentID int64) (*models.Reminder, error) {
	var reminder models.Reminder
	err := r.db.Where("comment_id = ?", commentID).First(&reminder).Error
	if err != nil {
		return nil, err
	}
	return &reminder, nil
}

// FindByIssue retrieves all reminders for a specific issue
func (r *reminderRepository) FindByIssue(owner, repo string, issueNumber int) ([]*models.Reminder, error) {
	var reminders []*models.Reminder
	err := r.db.Where("repository_owner = ? AND repository_name = ? AND issue_number = ?",
		owner, repo, issueNumber).
		Order("remind_at DESC").
		Find(&reminders).Error
	return reminders, err
}

// FindDueReminders retrieves reminders that are due to be fired
func (r *reminderRepository) FindDueReminders(limit int) ([]*models.Reminder, error) {
	var reminders []*models.Reminder
	err := r.db.Where("status = ? AND remind_at <= ?", models.StatusPending, time.Now()).
		Order("remind_at ASC").
		Limit(limit).
		Find(&reminders).Error
	return reminders, err
}

// GetAndLockDueReminders atomically fetches and locks due reminders for processing
func (r *reminderRepository) GetAndLockDueReminders(limit int) ([]*models.Reminder, error) {
	var reminders []*models.Reminder

	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Determine database dialect
		dialect := tx.Name()

		if dialect == "postgres" || dialect == "mysql" {
			// Use FOR UPDATE SKIP LOCKED for PostgreSQL and MySQL
			subQuery := tx.Model(&models.Reminder{}).
				Where("status = ? AND remind_at <= ?", models.StatusPending, time.Now()).
				Order("remind_at ASC").
				Limit(limit).
				Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})

			// Get IDs from subquery
			var ids []uint
			if err := subQuery.Pluck("id", &ids).Error; err != nil {
				return err
			}

			if len(ids) == 0 {
				return nil // No reminders to process
			}

			// Update status to processing
			if err := tx.Model(&models.Reminder{}).
				Where("id IN ?", ids).
				Update("status", models.StatusProcessing).Error; err != nil {
				return err
			}

			// Fetch updated reminders
			return tx.Where("id IN ?", ids).Find(&reminders).Error
		}

		// SQLite: simple update without SKIP LOCKED
		if err := tx.Model(&models.Reminder{}).
			Where("status = ? AND remind_at <= ?", models.StatusPending, time.Now()).
			Order("remind_at ASC").
			Limit(limit).
			Update("status", models.StatusProcessing).Error; err != nil {
			return err
		}

		// Fetch updated reminders
		return tx.Where("status = ?", models.StatusProcessing).
			Order("remind_at ASC").
			Limit(limit).
			Find(&reminders).Error
	})

	return reminders, err
}

// UpdateStatus updates the status of a reminder
func (r *reminderRepository) UpdateStatus(id uint, status models.ReminderStatus) error {
	return r.db.Model(&models.Reminder{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// MarkFired marks a reminder as successfully fired
func (r *reminderRepository) MarkFired(id uint) error {
	now := time.Now()
	return r.db.Model(&models.Reminder{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":   models.StatusFired,
			"fired_at": now,
		}).Error
}

// MarkFailed marks a reminder as failed with error message
func (r *reminderRepository) MarkFailed(id uint, errorMsg string) error {
	return r.db.Model(&models.Reminder{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        models.StatusFailed,
			"error_message": errorMsg,
		}).Error
}

// IncrementRetry increments the retry count and resets to pending
func (r *reminderRepository) IncrementRetry(id uint) error {
	return r.db.Model(&models.Reminder{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      models.StatusPending,
			"retry_count": gorm.Expr("retry_count + 1"),
		}).Error
}

// Cancel cancels a reminder by comment ID
func (r *reminderRepository) Cancel(commentID int64) error {
	return r.db.Model(&models.Reminder{}).
		Where("comment_id = ?", commentID).
		Update("status", models.StatusCancelled).Error
}

// Delete soft-deletes a reminder
func (r *reminderRepository) Delete(id uint) error {
	return r.db.Delete(&models.Reminder{}, id).Error
}
