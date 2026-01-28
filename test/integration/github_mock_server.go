// Package integration provides end-to-end integration tests for the Remora reminder service.
package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TokenRequest represents a captured token request.
type TokenRequest struct {
	InstallationID int64
	JWTToken       string
}

// ReactionRequest represents a captured reaction request.
type ReactionRequest struct {
	Owner     string
	Repo      string
	CommentID int64
	Content   string
}

// CommentRequest represents a captured comment request.
type CommentRequest struct {
	Owner       string
	Repo        string
	IssueNumber int
	Body        string
}

// MockGitHubServer provides a fake GitHub API for integration testing.
type MockGitHubServer struct {
	*httptest.Server
	t *testing.T

	mu sync.Mutex

	// Track calls for assertions
	TokenRequests    []TokenRequest
	ReactionRequests []ReactionRequest
	CommentRequests  []CommentRequest

	// Configure behavior
	FailNextRequest     bool
	RateLimitAt         int  // Return 429 after N requests
	ReturnStatusCode    int  // Override response status
	FailTokenRequest    bool // Fail installation token requests
	FailReactionRequest bool // Fail reaction requests
	FailCommentRequest  bool // Fail comment requests

	// Internal counters
	requestCount int
	commentIDSeq int64
}

// NewMockGitHubServer creates a new mock GitHub API server for testing.
func NewMockGitHubServer(t *testing.T) *MockGitHubServer {
	t.Helper()

	mock := &MockGitHubServer{
		t:            t,
		commentIDSeq: 1000,
	}

	mux := http.NewServeMux()

	// Installation token endpoint
	mux.HandleFunc("POST /app/installations/{installationID}/access_tokens", mock.handleInstallationToken)

	// Reactions endpoint
	mux.HandleFunc("POST /repos/{owner}/{repo}/issues/comments/{commentID}/reactions", mock.handleAddReaction)

	// Comments endpoint
	mux.HandleFunc("POST /repos/{owner}/{repo}/issues/{issueNumber}/comments", mock.handlePostComment)

	// Rate limit endpoint
	mux.HandleFunc("GET /rate_limit", mock.handleRateLimit)

	mock.Server = httptest.NewServer(mux)

	return mock
}

// TotalRequests returns the total number of requests made to the server.
func (m *MockGitHubServer) TotalRequests() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requestCount
}

// Reset clears all captured requests and resets configuration.
func (m *MockGitHubServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TokenRequests = nil
	m.ReactionRequests = nil
	m.CommentRequests = nil
	m.FailNextRequest = false
	m.RateLimitAt = 0
	m.ReturnStatusCode = 0
	m.FailTokenRequest = false
	m.FailReactionRequest = false
	m.FailCommentRequest = false
	m.requestCount = 0
}

func (m *MockGitHubServer) checkCommonFailures(w http.ResponseWriter) bool {
	m.mu.Lock()
	m.requestCount++
	failNext := m.FailNextRequest
	rateLimitAt := m.RateLimitAt
	statusOverride := m.ReturnStatusCode
	count := m.requestCount
	m.FailNextRequest = false // Reset after checking
	m.mu.Unlock()

	// Check for forced failure
	if failNext {
		http.Error(w, "connection reset", http.StatusServiceUnavailable)
		return true
	}

	// Check for rate limiting
	if rateLimitAt > 0 && count >= rateLimitAt {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(1*time.Minute).Unix(), 10))
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"message":           "API rate limit exceeded",
			"documentation_url": "https://docs.github.com/rest/overview/resources-in-the-rest-api#rate-limiting",
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return true
	}

	// Check for status override
	if statusOverride > 0 {
		w.WriteHeader(statusOverride)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"message": http.StatusText(statusOverride),
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return true
	}

	return false
}

func (m *MockGitHubServer) handleInstallationToken(w http.ResponseWriter, r *http.Request) {
	if m.checkCommonFailures(w) {
		return
	}

	// Parse installation ID from path
	installationIDStr := r.PathValue("installationID")
	installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid installation ID", http.StatusBadRequest)
		return
	}

	// Extract JWT from Authorization header
	authHeader := r.Header.Get("Authorization")
	jwtToken := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		jwtToken = authHeader[7:]
	}

	// Record the request
	m.mu.Lock()
	failToken := m.FailTokenRequest
	m.TokenRequests = append(m.TokenRequests, TokenRequest{
		InstallationID: installationID,
		JWTToken:       jwtToken,
	})
	m.mu.Unlock()

	if failToken {
		w.WriteHeader(http.StatusUnauthorized)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Bad credentials",
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Return a mock token
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"token":      fmt.Sprintf("ghs_mock_token_%d", installationID),
		"expires_at": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockGitHubServer) handleAddReaction(w http.ResponseWriter, r *http.Request) {
	if m.checkCommonFailures(w) {
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	commentIDStr := r.PathValue("commentID")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid comment ID", http.StatusBadRequest)
		return
	}

	// Parse request body
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Record the request
	m.mu.Lock()
	failReaction := m.FailReactionRequest
	m.ReactionRequests = append(m.ReactionRequests, ReactionRequest{
		Owner:     owner,
		Repo:      repo,
		CommentID: commentID,
		Content:   body.Content,
	})
	m.mu.Unlock()

	if failReaction {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Internal server error",
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Return a mock reaction response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"id":      12345,
		"content": body.Content,
		"user": map[string]interface{}{
			"login": "remora-bot",
			"id":    1,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockGitHubServer) handlePostComment(w http.ResponseWriter, r *http.Request) {
	if m.checkCommonFailures(w) {
		return
	}

	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	issueNumberStr := r.PathValue("issueNumber")
	issueNumber, err := strconv.ParseInt(issueNumberStr, 10, 32)
	if err != nil {
		http.Error(w, "invalid issue number", http.StatusBadRequest)
		return
	}

	// Parse request body
	var body struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Record the request
	m.mu.Lock()
	failComment := m.FailCommentRequest
	m.commentIDSeq++
	newCommentID := m.commentIDSeq
	m.CommentRequests = append(m.CommentRequests, CommentRequest{
		Owner:       owner,
		Repo:        repo,
		IssueNumber: int(issueNumber),
		Body:        body.Body,
	})
	m.mu.Unlock()

	if failComment {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Internal server error",
		}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
		return
	}

	// Return a mock comment response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	response := map[string]interface{}{
		"id":       newCommentID,
		"body":     body.Body,
		"html_url": fmt.Sprintf("https://github.com/%s/%s/issues/%d#issuecomment-%d", owner, repo, issueNumber, newCommentID),
		"user": map[string]interface{}{
			"login": "remora-bot",
			"id":    1,
		},
		"created_at": time.Now().Format(time.RFC3339),
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func (m *MockGitHubServer) handleRateLimit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"resources": map[string]interface{}{
			"core": map[string]interface{}{
				"limit":     5000,
				"remaining": 4999,
				"reset":     time.Now().Add(1 * time.Hour).Unix(),
			},
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

// AssertReactionAdded verifies that a reaction was added to a specific comment.
func (m *MockGitHubServer) AssertReactionAdded(t *testing.T, owner, repo string, commentID int64, content string) {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, req := range m.ReactionRequests {
		if req.Owner == owner && req.Repo == repo && req.CommentID == commentID && req.Content == content {
			return
		}
	}

	t.Errorf("expected reaction %q on %s/%s comment %d, but not found", content, owner, repo, commentID)
}

// AssertCommentPosted verifies that a comment was posted to a specific issue.
func (m *MockGitHubServer) AssertCommentPosted(t *testing.T, owner, repo string, issueNumber int, bodyPattern string) {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()

	re := regexp.MustCompile(bodyPattern)

	for _, req := range m.CommentRequests {
		if req.Owner == owner && req.Repo == repo && req.IssueNumber == issueNumber && re.MatchString(req.Body) {
			return
		}
	}

	t.Errorf("expected comment matching %q on %s/%s#%d, but not found", bodyPattern, owner, repo, issueNumber)
}

// AssertNoComments verifies that no comments were posted.
func (m *MockGitHubServer) AssertNoComments(t *testing.T) {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.CommentRequests) > 0 {
		t.Errorf("expected no comments, but got %d", len(m.CommentRequests))
	}
}

// AssertTokenRequested verifies that an installation token was requested.
func (m *MockGitHubServer) AssertTokenRequested(t *testing.T, installationID int64) {
	t.Helper()

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, req := range m.TokenRequests {
		if req.InstallationID == installationID {
			return
		}
	}

	t.Errorf("expected token request for installation %d, but not found", installationID)
}
