package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
}

// DefaultRetryConfig returns the default retry configuration
// Exponential backoff: 1, 2, 4, 8, 16 minutes with max 5 attempts
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Minute,
		MaxBackoff:     16 * time.Minute,
		Multiplier:     2.0,
	}
}

// APIError represents an error from the GitHub API
type APIError struct {
	StatusCode int
	Message    string
	URL        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API error (HTTP %d): %s [%s]", e.StatusCode, e.Message, e.URL)
}

// IsRetryable returns true if the error is retryable
func (e *APIError) IsRetryable() bool {
	// Retry on 5xx errors and 429 (rate limit)
	return e.StatusCode >= 500 || e.StatusCode == 429
}

// IsUnauthorizedError checks if an error is a 401 Unauthorized
func IsUnauthorizedError(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == 401
}

// IsNotFoundError checks if an error is a 404 Not Found
func IsNotFoundError(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == 404
}

// IsForbiddenError checks if an error is a 403 Forbidden
func IsForbiddenError(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == 403
}

// IsRateLimitError checks if an error is a 429 Rate Limit
func IsRateLimitError(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == 429
}

// doRequestWithRetry executes an HTTP request with retry logic
func (c *Client) doRequestWithRetry(ctx context.Context, req *http.Request, result interface{}) error {
	var lastErr error
	backoff := c.retryConfig.InitialBackoff

	for attempt := 1; attempt <= c.retryConfig.MaxAttempts; attempt++ {
		err := c.doRequest(ctx, req, result)

		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		apiErr, ok := err.(*APIError)
		if !ok || !apiErr.IsRetryable() {
			// Not a retryable error, return immediately
			return err
		}

		// Last attempt - don't wait
		if attempt == c.retryConfig.MaxAttempts {
			c.logger.Warn("max retry attempts reached",
				zap.Int("attempts", attempt),
				zap.Error(err))
			break
		}

		// Log retry attempt
		c.logger.Warn("request failed, retrying",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", c.retryConfig.MaxAttempts),
			zap.Duration("backoff", backoff),
			zap.Error(err))

		// Wait with exponential backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Calculate next backoff
			backoff = time.Duration(float64(backoff) * c.retryConfig.Multiplier)
			if backoff > c.retryConfig.MaxBackoff {
				backoff = c.retryConfig.MaxBackoff
			}
		}
	}

	return lastErr
}

// doRequest executes a single HTTP request
func (c *Client) doRequest(ctx context.Context, req *http.Request, result interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Update rate limit info
	c.updateRateLimitInfo(resp)

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for errors
	if resp.StatusCode >= 400 {
		// Try to parse error message from response
		var errorResp struct {
			Message string `json:"message"`
		}
		json.Unmarshal(body, &errorResp)

		message := errorResp.Message
		if message == "" {
			message = string(body)
		}

		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    message,
			URL:        req.URL.String(),
		}
	}

	// Parse successful response
	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
	}

	return nil
}
