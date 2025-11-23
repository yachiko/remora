package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/yachiko/remora/internal/config"
	"github.com/yachiko/remora/internal/database"
	"github.com/yachiko/remora/internal/github"
	"github.com/yachiko/remora/internal/models"
	"github.com/yachiko/remora/internal/parser"
	"go.uber.org/zap"
)

const (
	// Event types
	EventIssueComment = "issue_comment"

	// Event actions
	ActionCreated = "created"
	ActionDeleted = "deleted"
)

// GitHubClient defines the interface for GitHub API operations
type GitHubClient interface {
	AddReaction(ctx context.Context, installationID int64, owner, repo string, commentID int64, reaction github.ReactionType) error
	PostComment(ctx context.Context, installationID int64, owner, repo string, issueNumber int, body string) (int64, error)
}

// Handler processes GitHub webhook events
type Handler struct {
	validator  *Validator
	parser     *parser.Parser
	repo       database.ReminderRepository
	github     GitHubClient
	logger     *zap.Logger
	errorMode  string
	requestLog bool
}

// NewHandler creates a new webhook handler
func NewHandler(
	cfg *config.Config,
	repo database.ReminderRepository,
	githubClient GitHubClient,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		validator:  NewValidator(cfg.GitHubWebhookSecret),
		parser:     parser.NewParser(),
		repo:       repo,
		github:     githubClient,
		logger:     logger,
		errorMode:  cfg.ErrorMode,
		requestLog: true,
	}
}

// ServeHTTP handles incoming webhook requests
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		h.logger.Warn("invalid HTTP method for webhook",
			zap.String("method", r.Method),
			zap.String("remote_addr", r.RemoteAddr))
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get delivery ID and event type from headers
	deliveryID := GetDeliveryID(r)
	eventType := GetEventType(r)

	// Log webhook request
	if h.requestLog {
		h.logger.Info("webhook received",
			zap.String("request_id", deliveryID),
			zap.String("event_type", eventType),
			zap.String("remote_addr", r.RemoteAddr))
	}

	// Read and validate payload
	payload, err := h.validator.ReadAndValidatePayload(r)
	if err != nil {
		h.logger.Warn("webhook signature validation failed",
			zap.String("request_id", deliveryID),
			zap.Error(err),
			zap.String("remote_addr", r.RemoteAddr))
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Filter for issue_comment events
	if eventType != EventIssueComment {
		h.logger.Debug("ignoring non-issue_comment event",
			zap.String("request_id", deliveryID),
			zap.String("event_type", eventType))
		w.WriteHeader(http.StatusOK)
		return
	}

	// Parse the webhook payload
	var event IssueCommentEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		h.logger.Warn("webhook payload validation failed",
			zap.String("request_id", deliveryID),
			zap.Error(err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if err := event.Validate(); err != nil {
		h.logger.Warn("webhook payload validation failed",
			zap.String("request_id", deliveryID),
			zap.Error(err))
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Handle based on action
	ctx := context.Background()
	switch event.Action {
	case ActionCreated:
		if err := h.handleCommentCreated(ctx, deliveryID, &event); err != nil {
			h.logger.Error("failed to handle comment created event",
				zap.String("request_id", deliveryID),
				zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	case ActionDeleted:
		if err := h.handleCommentDeleted(ctx, deliveryID, &event); err != nil {
			h.logger.Error("failed to handle comment deleted event",
				zap.String("request_id", deliveryID),
				zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	default:
		h.logger.Debug("ignoring issue_comment event with unsupported action",
			zap.String("request_id", deliveryID),
			zap.String("action", event.Action))
	}

	w.WriteHeader(http.StatusOK)
}

// handleCommentCreated processes comment creation events
func (h *Handler) handleCommentCreated(ctx context.Context, requestID string, event *IssueCommentEvent) error {
	// Check if comment contains remora command
	if !parser.HasRemoraCommand(event.Comment.Body) {
		h.logger.Debug("comment does not contain remora command",
			zap.String("request_id", requestID),
			zap.String("repository", event.Repository.FullName),
			zap.Int("issue_number", event.Issue.Number))
		return nil
	}

	h.logger.Debug("remora command detected",
		zap.String("request_id", requestID),
		zap.String("repository", event.Repository.FullName),
		zap.Int("issue_number", event.Issue.Number),
		zap.Int64("comment_id", event.Comment.ID),
		zap.String("original_command", parser.ExtractCommand(event.Comment.Body)))

	// Add eyes reaction to indicate processing
	if err := h.github.AddReaction(ctx, event.Installation.ID, event.Repository.Owner.Login, event.Repository.Name, event.Comment.ID, github.ReactionEyes); err != nil {
		h.logger.Warn("failed to add eyes reaction",
			zap.String("request_id", requestID),
			zap.Error(err))
		// Continue processing even if reaction fails
	}

	// Parse the command
	cmd, err := h.parser.ParseComment(event.Comment.Body)
	if err != nil {
		h.logger.Warn("failed to parse remora command",
			zap.String("request_id", requestID),
			zap.String("repository", event.Repository.FullName),
			zap.Int("issue_number", event.Issue.Number),
			zap.Error(err))

		// Handle parsing error
		return h.handleParseError(ctx, requestID, event, err)
	}

	// Create reminder in database
	reminder := &models.Reminder{
		RepositoryOwner:   event.Repository.Owner.Login,
		RepositoryName:    event.Repository.Name,
		IssueNumber:       event.Issue.Number,
		CommentID:         event.Comment.ID,
		CommentURL:        event.Comment.HTMLURL,
		RequesterUsername: event.Comment.User.Login,
		RequesterID:       event.Comment.User.ID,
		RemindAt:          cmd.RemindAt,
		OriginalCommand:   cmd.OriginalCommand,
		Status:            models.StatusPending,
		RetryCount:        0,
	}

	if err := h.repo.Create(reminder); err != nil {
		h.logger.Error("failed to create reminder",
			zap.String("request_id", requestID),
			zap.String("repository", event.Repository.FullName),
			zap.Int("issue_number", event.Issue.Number),
			zap.Error(err))
		return fmt.Errorf("failed to create reminder: %w", err)
	}

	h.logger.Info("reminder created",
		zap.String("request_id", requestID),
		zap.Uint("reminder_id", reminder.ID),
		zap.String("repository", event.Repository.FullName),
		zap.Int("issue_number", event.Issue.Number),
		zap.String("requester", reminder.RequesterUsername),
		zap.Time("remind_at", reminder.RemindAt),
		zap.String("original_command", reminder.OriginalCommand))

	// Add success reaction (hooray)
	if err := h.github.AddReaction(ctx, event.Installation.ID, event.Repository.Owner.Login, event.Repository.Name, event.Comment.ID, github.ReactionHooray); err != nil {
		h.logger.Warn("failed to add success reaction",
			zap.String("request_id", requestID),
			zap.Uint("reminder_id", reminder.ID),
			zap.Error(err))
		// Continue even if reaction fails
	}

	return nil
}

// handleCommentDeleted processes comment deletion events
func (h *Handler) handleCommentDeleted(ctx context.Context, requestID string, event *IssueCommentEvent) error {
	// Find reminder by comment ID
	reminder, err := h.repo.FindByCommentID(event.Comment.ID)
	if err != nil {
		// No reminder found for this comment - this is expected for most deletions
		h.logger.Debug("no reminder found for deleted comment",
			zap.String("request_id", requestID),
			zap.Int64("comment_id", event.Comment.ID),
			zap.String("repository", event.Repository.FullName),
			zap.Int("issue_number", event.Issue.Number))
		return nil
	}

	// Cancel the reminder
	if err := h.repo.Cancel(event.Comment.ID); err != nil {
		h.logger.Error("failed to cancel reminder",
			zap.String("request_id", requestID),
			zap.Uint("reminder_id", reminder.ID),
			zap.Int64("comment_id", event.Comment.ID),
			zap.Error(err))
		return fmt.Errorf("failed to cancel reminder: %w", err)
	}

	h.logger.Info("reminder cancelled",
		zap.String("request_id", requestID),
		zap.Uint("reminder_id", reminder.ID),
		zap.String("repository", event.Repository.FullName),
		zap.Int("issue_number", event.Issue.Number),
		zap.String("requester", reminder.RequesterUsername))

	return nil
}

// handleParseError handles errors that occur during command parsing
func (h *Handler) handleParseError(ctx context.Context, requestID string, event *IssueCommentEvent, parseErr error) error {
	// Add confused reaction to indicate error
	if err := h.github.AddReaction(ctx, event.Installation.ID, event.Repository.Owner.Login, event.Repository.Name, event.Comment.ID, github.ReactionConfused); err != nil {
		h.logger.Warn("failed to add error reaction",
			zap.String("request_id", requestID),
			zap.Error(err))
	}

	// If error mode is reaction_and_comment, post explanatory comment
	if h.errorMode == "reaction_and_comment" {
		errorComment := h.formatErrorComment(event.Comment.User.Login, parseErr)
		_, err := h.github.PostComment(ctx, event.Installation.ID, event.Repository.Owner.Login, event.Repository.Name, event.Issue.Number, errorComment)
		if err != nil {
			h.logger.Error("failed to post error comment",
				zap.String("request_id", requestID),
				zap.String("repository", event.Repository.FullName),
				zap.Int("issue_number", event.Issue.Number),
				zap.Error(err))
			return fmt.Errorf("failed to post error comment: %w", err)
		}

		h.logger.Info("posted error comment",
			zap.String("request_id", requestID),
			zap.String("repository", event.Repository.FullName),
			zap.Int("issue_number", event.Issue.Number))
	}

	return nil
}

// formatErrorComment formats an error message for posting as a comment
func (h *Handler) formatErrorComment(username string, err error) string {
	// Check if it's a parser error with specific message
	if parseErr, ok := err.(*parser.ParseError); ok {
		return formatParseError(username, parseErr)
	}

	// Generic error
	return fmt.Sprintf(`@%s I couldn't parse your reminder request.

Please use the format: `+"`remora <time-expression>`"+`

Examples:
- remora 2 days
- remora tomorrow at 3pm
- remora next Monday 9am EST
- remora December 25th`, username)
}

// formatParseError formats a specific parse error into a user-friendly comment
func formatParseError(username string, err *parser.ParseError) string {
	// Use the parser's built-in user-facing message
	return err.GetUserFacingMessage(username)
}
