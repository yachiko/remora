// Package github provides a client for interacting with the GitHub API,
// including support for GitHub App authentication and retry logic.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ReactionType represents GitHub reaction types
type ReactionType string

// Reaction constants define the supported GitHub reactions.
const (
	// ReactionEyes represents the 👀 (eyes) reaction, used to indicate processing.
	ReactionEyes ReactionType = "eyes"
	// ReactionHooray represents the 🎉 (hooray) reaction, used to indicate success.
	ReactionHooray ReactionType = "hooray"
	// ReactionConfused represents the 😕 (confused) reaction, used to indicate an error.
	ReactionConfused ReactionType = "confused"
	// ReactionPlusOne represents the 👍 (+1) reaction, an alternative success indicator.
	ReactionPlusOne ReactionType = "+1"
	// ReactionMinusOne represents the 👎 (-1) reaction, an alternative error indicator.
	ReactionMinusOne ReactionType = "-1"
)

// AddReaction adds a reaction to a comment
func (c *Client) AddReaction(ctx context.Context, installationID int64, owner, repo string, commentID int64, reaction ReactionType) error {
	token, err := c.GetInstallationToken(ctx, installationID)
	if err != nil {
		return fmt.Errorf("failed to get installation token: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d/reactions", c.baseURL, owner, repo, commentID)

	payload := map[string]string{
		"content": string(reaction),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug("adding reaction to comment",
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.Int64("comment_id", commentID),
		zap.String("reaction", string(reaction)))

	var response map[string]interface{}
	err = c.doRequestWithRetry(ctx, req, &response)
	if err != nil {
		// Check if it's a 401 - invalidate token and retry once
		if IsUnauthorizedError(err) {
			c.logger.Warn("received 401, invalidating token and retrying",
				zap.Int64("installation_id", installationID))
			c.InvalidateToken(installationID)

			// Retry with fresh token
			token, err = c.GetInstallationToken(ctx, installationID)
			if err != nil {
				return fmt.Errorf("failed to refresh token: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			err = c.doRequestWithRetry(ctx, req, &response)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to add reaction: %w", err)
	}

	c.logger.Info("added reaction to comment",
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.Int64("comment_id", commentID),
		zap.String("reaction", string(reaction)))

	return nil
}

// PostComment posts a comment on an issue or pull request
func (c *Client) PostComment(ctx context.Context, installationID int64, owner, repo string, issueNumber int, body string) (int64, error) {
	token, err := c.GetInstallationToken(ctx, installationID)
	if err != nil {
		return 0, fmt.Errorf("failed to get installation token: %w", err)
	}

	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, issueNumber)

	payload := map[string]string{
		"body": body,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	c.logger.Debug("posting comment",
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.Int("issue_number", issueNumber))

	var response struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	}

	err = c.doRequestWithRetry(ctx, req, &response)
	if err != nil {
		// Check if it's a 401 - invalidate token and retry once
		if IsUnauthorizedError(err) {
			c.logger.Warn("received 401, invalidating token and retrying",
				zap.Int64("installation_id", installationID))
			c.InvalidateToken(installationID)

			// Retry with fresh token
			token, err = c.GetInstallationToken(ctx, installationID)
			if err != nil {
				return 0, fmt.Errorf("failed to refresh token: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			err = c.doRequestWithRetry(ctx, req, &response)
		}
	}

	if err != nil {
		return 0, fmt.Errorf("failed to post comment: %w", err)
	}

	c.logger.Info("posted comment",
		zap.String("owner", owner),
		zap.String("repo", repo),
		zap.Int("issue_number", issueNumber),
		zap.Int64("comment_id", response.ID),
		zap.String("url", response.HTMLURL))

	return response.ID, nil
}

// FormatReminderComment formats a reminder comment body
func FormatReminderComment(username, commentURL string) string {
	return fmt.Sprintf("@%s 🔔 Reminder!\n\nYou asked to be reminded about this issue.\n\nOriginal request: %s", username, commentURL)
}

// FormatOverdueReminderComment formats a reminder comment with delay annotation
func FormatOverdueReminderComment(username, commentURL string, delay time.Duration) string {
	delayStr := formatDuration(delay)
	return fmt.Sprintf("@%s 🔔 Reminder (%s late)!\n\nYou asked to be reminded about this issue.\n\nOriginal request: %s", username, delayStr, commentURL)
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours >= 24 {
		days := hours / 24
		hours %= 24
		if hours > 0 {
			return fmt.Sprintf("%d day%s %d hour%s", days, plural(days), hours, plural(hours))
		}
		return fmt.Sprintf("%d day%s", days, plural(days))
	}

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%d hour%s %d minute%s", hours, plural(hours), minutes, plural(minutes))
		}
		return fmt.Sprintf("%d hour%s", hours, plural(hours))
	}

	return fmt.Sprintf("%d minute%s", minutes, plural(minutes))
}

// plural returns "s" for values != 1
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
