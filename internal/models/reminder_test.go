package models

import (
	"testing"
	"time"
)

func TestReminderTableName(t *testing.T) {
	r := Reminder{}
	if r.TableName() != "reminders" {
		t.Errorf("TableName() = %s, want reminders", r.TableName())
	}
}

func TestReminderIsValid(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		reminder Reminder
		want     bool
	}{
		{
			name: "valid reminder",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         456,
				InstallationID:    789,
				RequesterUsername: "user",
				RequesterID:       101,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: true,
		},
		{
			name: "missing repository owner",
			reminder: Reminder{
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         456,
				RequesterUsername: "user",
				RequesterID:       789,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "missing repository name",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				IssueNumber:       123,
				CommentID:         456,
				RequesterUsername: "user",
				RequesterID:       789,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "invalid issue number",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       0,
				CommentID:         456,
				RequesterUsername: "user",
				RequesterID:       789,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "missing comment id",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         0,
				RequesterUsername: "user",
				RequesterID:       789,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "missing requester username",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         456,
				RequesterUsername: "",
				RequesterID:       789,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "invalid requester id",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         456,
				RequesterUsername: "user",
				RequesterID:       0,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "zero remind at",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         456,
				RequesterUsername: "user",
				RequesterID:       789,
				RemindAt:          time.Time{},
				OriginalCommand:   "remora 2 days",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "missing original command",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         456,
				RequesterUsername: "user",
				RequesterID:       789,
				RemindAt:          now,
				OriginalCommand:   "",
				Status:            StatusPending,
			},
			want: false,
		},
		{
			name: "missing status",
			reminder: Reminder{
				RepositoryOwner:   "owner",
				RepositoryName:    "repo",
				IssueNumber:       123,
				CommentID:         456,
				RequesterUsername: "user",
				RequesterID:       789,
				RemindAt:          now,
				OriginalCommand:   "remora 2 days",
				Status:            "",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.reminder.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReminderStatusMethods(t *testing.T) {
	tests := []struct {
		name   string
		status ReminderStatus
		checks map[string]bool
	}{
		{
			name:   "pending status",
			status: StatusPending,
			checks: map[string]bool{
				"IsPending":    true,
				"IsProcessing": false,
				"IsFired":      false,
				"IsFailed":     false,
				"IsCancelled":  false,
			},
		},
		{
			name:   "processing status",
			status: StatusProcessing,
			checks: map[string]bool{
				"IsPending":    false,
				"IsProcessing": true,
				"IsFired":      false,
				"IsFailed":     false,
				"IsCancelled":  false,
			},
		},
		{
			name:   "fired status",
			status: StatusFired,
			checks: map[string]bool{
				"IsPending":    false,
				"IsProcessing": false,
				"IsFired":      true,
				"IsFailed":     false,
				"IsCancelled":  false,
			},
		},
		{
			name:   "failed status",
			status: StatusFailed,
			checks: map[string]bool{
				"IsPending":    false,
				"IsProcessing": false,
				"IsFired":      false,
				"IsFailed":     true,
				"IsCancelled":  false,
			},
		},
		{
			name:   "cancelled status",
			status: StatusCancelled,
			checks: map[string]bool{
				"IsPending":    false,
				"IsProcessing": false,
				"IsFired":      false,
				"IsFailed":     false,
				"IsCancelled":  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Reminder{Status: tt.status}

			if got := r.IsPending(); got != tt.checks["IsPending"] {
				t.Errorf("IsPending() = %v, want %v", got, tt.checks["IsPending"])
			}
			if got := r.IsProcessing(); got != tt.checks["IsProcessing"] {
				t.Errorf("IsProcessing() = %v, want %v", got, tt.checks["IsProcessing"])
			}
			if got := r.IsFired(); got != tt.checks["IsFired"] {
				t.Errorf("IsFired() = %v, want %v", got, tt.checks["IsFired"])
			}
			if got := r.IsFailed(); got != tt.checks["IsFailed"] {
				t.Errorf("IsFailed() = %v, want %v", got, tt.checks["IsFailed"])
			}
			if got := r.IsCancelled(); got != tt.checks["IsCancelled"] {
				t.Errorf("IsCancelled() = %v, want %v", got, tt.checks["IsCancelled"])
			}
		})
	}
}

func TestReminderIsDue(t *testing.T) {
	tests := []struct {
		name     string
		remindAt time.Time
		want     bool
	}{
		{
			name:     "past time is due",
			remindAt: time.Now().Add(-1 * time.Hour),
			want:     true,
		},
		{
			name:     "future time is not due",
			remindAt: time.Now().Add(1 * time.Hour),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Reminder{RemindAt: tt.remindAt}
			if got := r.IsDue(); got != tt.want {
				t.Errorf("IsDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReminderCanRetry(t *testing.T) {
	tests := []struct {
		name       string
		retryCount int
		maxRetries int
		want       bool
	}{
		{
			name:       "can retry when under limit",
			retryCount: 2,
			maxRetries: 5,
			want:       true,
		},
		{
			name:       "cannot retry at limit",
			retryCount: 5,
			maxRetries: 5,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Reminder{RetryCount: tt.retryCount}
			if got := r.CanRetry(tt.maxRetries); got != tt.want {
				t.Errorf("CanRetry(%d) = %v, want %v", tt.maxRetries, got, tt.want)
			}
		})
	}
}

func TestReminderRepositoryFullName(t *testing.T) {
	r := Reminder{
		RepositoryOwner: "owner",
		RepositoryName:  "repo",
	}
	if got := r.RepositoryFullName(); got != "owner/repo" {
		t.Errorf("RepositoryFullName() = %v, want owner/repo", got)
	}
}
