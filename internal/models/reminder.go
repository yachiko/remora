package models

import (
	"time"

	"gorm.io/gorm"
)

// ReminderStatus represents the state of a reminder
type ReminderStatus string

const (
	StatusPending    ReminderStatus = "pending"
	StatusProcessing ReminderStatus = "processing"
	StatusFired      ReminderStatus = "fired"
	StatusFailed     ReminderStatus = "failed"
	StatusCancelled  ReminderStatus = "cancelled"
)

// Reminder represents a scheduled reminder for a GitHub issue or PR
type Reminder struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// GitHub repository information
	RepositoryOwner string `gorm:"size:255;not null;index:idx_repo" json:"repository_owner"`
	RepositoryName  string `gorm:"size:255;not null;index:idx_repo" json:"repository_name"`
	IssueNumber     int    `gorm:"not null;index:idx_repo" json:"issue_number"`

	// GitHub comment information
	CommentID  int64  `gorm:"not null" json:"comment_id"`
	CommentURL string `gorm:"size:512;not null" json:"comment_url"`

	// GitHub App installation
	InstallationID int64 `gorm:"not null" json:"installation_id"`

	// Requester information
	RequesterUsername string `gorm:"size:255;not null" json:"requester_username"`
	RequesterID       int64  `gorm:"not null" json:"requester_id"`

	// Reminder scheduling
	RemindAt        time.Time `gorm:"not null;index:idx_remind_at,priority:2;index:idx_scheduler_query,priority:2" json:"remind_at"`
	OriginalCommand string    `gorm:"size:512;not null" json:"original_command"`

	// Status tracking
	Status       ReminderStatus `gorm:"size:50;not null;index:idx_status;index:idx_scheduler_query,priority:1" json:"status"`
	FiredAt      *time.Time     `json:"fired_at,omitempty"`
	ErrorMessage string         `gorm:"type:text" json:"error_message,omitempty"`
	RetryCount   int            `gorm:"not null;default:0" json:"retry_count"`
}

// TableName specifies the table name for the Reminder model
func (Reminder) TableName() string {
	return "reminders"
}

// IsValid checks if the reminder has valid required fields
func (r *Reminder) IsValid() bool {
	if r.RepositoryOwner == "" || r.RepositoryName == "" {
		return false
	}
	if r.IssueNumber <= 0 {
		return false
	}
	if r.CommentID <= 0 {
		return false
	}
	if r.RequesterUsername == "" || r.RequesterID <= 0 {
		return false
	}
	if r.RemindAt.IsZero() {
		return false
	}
	if r.OriginalCommand == "" {
		return false
	}
	if r.Status == "" {
		return false
	}
	return true
}

// IsPending returns true if the reminder is in pending status
func (r *Reminder) IsPending() bool {
	return r.Status == StatusPending
}

// IsProcessing returns true if the reminder is being processed
func (r *Reminder) IsProcessing() bool {
	return r.Status == StatusProcessing
}

// IsFired returns true if the reminder has been fired
func (r *Reminder) IsFired() bool {
	return r.Status == StatusFired
}

// IsFailed returns true if the reminder has failed
func (r *Reminder) IsFailed() bool {
	return r.Status == StatusFailed
}

// IsCancelled returns true if the reminder has been cancelled
func (r *Reminder) IsCancelled() bool {
	return r.Status == StatusCancelled
}

// IsDue returns true if the reminder is due to be fired
func (r *Reminder) IsDue() bool {
	return r.RemindAt.Before(time.Now()) || r.RemindAt.Equal(time.Now())
}

// CanRetry returns true if the reminder can be retried (based on max retry count)
func (r *Reminder) CanRetry(maxRetries int) bool {
	return r.RetryCount < maxRetries
}

// RepositoryFullName returns the full repository name (owner/name)
func (r *Reminder) RepositoryFullName() string {
	return r.RepositoryOwner + "/" + r.RepositoryName
}
