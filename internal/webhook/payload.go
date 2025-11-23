package webhook

import (
	"fmt"
	"time"
)

// IssueCommentEvent represents a GitHub issue_comment webhook event
type IssueCommentEvent struct {
	Action       string       `json:"action"`
	Comment      Comment      `json:"comment"`
	Issue        Issue        `json:"issue"`
	Repository   Repository   `json:"repository"`
	Installation Installation `json:"installation"`
	Sender       User         `json:"sender"`
}

// Comment represents a GitHub comment
type Comment struct {
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
	HTMLURL   string    `json:"html_url"`
	Body      string    `json:"body"`
	User      User      `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Issue represents a GitHub issue or pull request
type Issue struct {
	ID          int64     `json:"id"`
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	HTMLURL     string    `json:"html_url"`
	State       string    `json:"state"`
	User        User      `json:"user"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PullRequest *struct{} `json:"pull_request,omitempty"` // Present if it's a PR
}

// Repository represents a GitHub repository
type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    User   `json:"owner"`
	Private  bool   `json:"private"`
}

// User represents a GitHub user or organization
type User struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"` // "User" or "Organization"
}

// Installation represents a GitHub App installation
type Installation struct {
	ID int64 `json:"id"`
}

// Validate checks if the event has all required fields
func (e *IssueCommentEvent) Validate() error {
	if e.Action == "" {
		return fmt.Errorf("missing required field: action")
	}

	if e.Comment.ID == 0 {
		return fmt.Errorf("missing required field: comment.id")
	}

	if e.Comment.Body == "" {
		return fmt.Errorf("missing required field: comment.body")
	}

	if e.Comment.User.Login == "" {
		return fmt.Errorf("missing required field: comment.user.login")
	}

	if e.Comment.User.ID == 0 {
		return fmt.Errorf("missing required field: comment.user.id")
	}

	if e.Comment.HTMLURL == "" {
		return fmt.Errorf("missing required field: comment.html_url")
	}

	if e.Issue.Number == 0 {
		return fmt.Errorf("missing required field: issue.number")
	}

	if e.Repository.Owner.Login == "" {
		return fmt.Errorf("missing required field: repository.owner.login")
	}

	if e.Repository.Name == "" {
		return fmt.Errorf("missing required field: repository.name")
	}

	if e.Repository.FullName == "" {
		return fmt.Errorf("missing required field: repository.full_name")
	}

	if e.Installation.ID == 0 {
		return fmt.Errorf("missing required field: installation.id")
	}

	return nil
}

// IsPullRequest returns true if the issue is actually a pull request
func (i *Issue) IsPullRequest() bool {
	return i.PullRequest != nil
}

// IsOpen returns true if the issue/PR is open
func (i *Issue) IsOpen() bool {
	return i.State == "open"
}

// IsClosed returns true if the issue/PR is closed
func (i *Issue) IsClosed() bool {
	return i.State == "closed"
}
