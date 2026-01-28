# Testing Strategy

## Overview

This document describes the testing approach for Remora, covering unit tests, integration tests, and end-to-end testing strategies. The goal is to maintain high test coverage (>80%) while ensuring the complete reminder flow works correctly.

---

## Test Categories

### 1. Unit Tests

**Location**: `internal/<package>/*_test.go`

Unit tests verify individual functions and methods in isolation. External dependencies (database, GitHub API) are mocked using interfaces.

**Packages with unit tests**:
| Package     | Coverage Target | Description                               |
| ----------- | --------------- | ----------------------------------------- |
| `config`    | >95%            | Configuration loading and validation      |
| `parser`    | >85%            | Time expression parsing and validation    |
| `models`    | 100%            | Model validation and helper methods       |
| `logger`    | >85%            | Logger initialization                     |
| `webhook`   | >80%            | Webhook validation and event handling     |
| `github`    | >85%            | GitHub client, retry logic, rate limiting |
| `scheduler` | >85%            | Scheduler logic and reminder execution    |
| `database`  | >80%            | Repository operations                     |
| `api`       | >85%            | Admin API handlers and middleware         |

**Running unit tests**:
```bash
go test ./...
go test -v ./internal/parser/...  # Single package
```

---

### 2. Integration Tests

**Location**: `test/integration/`

Integration tests verify multiple components working together with real (in-memory) databases but mocked external services.

**Running integration tests**:
```bash
REMORA_INTEGRATION_TESTS=1 go test ./test/integration/...
```

---

## Mock GitHub HTTP Server

To test the actual GitHub client code paths including HTTP serialization, error handling, and retry logic, we use a mock HTTP server that mimics GitHub's API.

### Purpose

- Test real HTTP client behavior (not just interface mocks)
- Verify request serialization and header handling
- Test error scenarios (rate limits, auth failures, network errors)
- Validate retry logic with realistic responses

### Implementation

**Location**: `test/integration/github_mock_server.go`

```go
// MockGitHubServer provides a fake GitHub API for integration testing
type MockGitHubServer struct {
    *httptest.Server
    
    // Track calls for assertions
    TokenRequests    []TokenRequest
    ReactionRequests []ReactionRequest
    CommentRequests  []CommentRequest
    
    // Configure behavior
    FailNextRequest  bool
    RateLimitAt      int  // Return 429 after N requests
    ReturnStatusCode int  // Override response status
}

// Endpoints implemented:
// POST /app/installations/{id}/access_tokens  - Installation token
// POST /repos/{owner}/{repo}/issues/comments/{id}/reactions - Add reaction  
// POST /repos/{owner}/{repo}/issues/{number}/comments - Post comment
// GET  /rate_limit - Rate limit status
```

### Supported Scenarios

| Scenario        | Configuration           | Purpose                       |
| --------------- | ----------------------- | ----------------------------- |
| Happy path      | Default                 | Verify normal operation       |
| Rate limiting   | `RateLimitAt: 5`        | Test 429 handling and backoff |
| Auth failure    | `ReturnStatusCode: 401` | Test token refresh            |
| Server error    | `ReturnStatusCode: 500` | Test retry logic              |
| Not found       | `ReturnStatusCode: 404` | Test error handling           |
| Network timeout | `FailNextRequest: true` | Test connection errors        |

### Usage Example

```go
func TestGitHubClient_WithMockServer(t *testing.T) {
    // Create mock server
    mockGitHub := NewMockGitHubServer(t)
    defer mockGitHub.Close()
    
    // Create client pointing to mock server
    client := github.NewClientWithBaseURL(appID, privateKey, logger, mockGitHub.URL)
    
    // Test adding reaction
    err := client.AddReaction(ctx, installationID, "owner", "repo", 123, github.ReactionEyes)
    assert.NoError(t, err)
    
    // Verify request was made correctly
    assert.Len(t, mockGitHub.ReactionRequests, 1)
    assert.Equal(t, "eyes", mockGitHub.ReactionRequests[0].Content)
}

func TestGitHubClient_RateLimitRetry(t *testing.T) {
    mockGitHub := NewMockGitHubServer(t)
    mockGitHub.RateLimitAt = 1  // Rate limit on first request
    defer mockGitHub.Close()
    
    client := github.NewClientWithBaseURL(appID, privateKey, logger, mockGitHub.URL)
    
    // Should succeed after retry
    err := client.AddReaction(ctx, installationID, "owner", "repo", 123, github.ReactionEyes)
    assert.NoError(t, err)
    
    // Verify retry happened
    assert.Equal(t, 2, mockGitHub.TotalRequests())
}
```

---

## Full Reminder Lifecycle Test

Tests the complete flow from webhook receipt through scheduler firing, verifying all components work together correctly.

### Flow Under Test

```
Webhook Received
      ↓
Signature Validated
      ↓
Command Parsed ("remora 2 days")
      ↓
Reminder Created in DB (status: pending)
      ↓
Eyes Reaction Added (👀)
      ↓
[Time passes / Scheduler runs]
      ↓
Reminder Fired (status: fired)
      ↓
Comment Posted (@user 🔔 Reminder!)
```

### Implementation

**Location**: `test/integration/lifecycle_test.go`

```go
func TestFullReminderLifecycle(t *testing.T) {
    if os.Getenv("REMORA_INTEGRATION_TESTS") == "" {
        t.Skip("Skipping integration test")
    }
    
    // Setup
    mockGitHub := NewMockGitHubServer(t)
    defer mockGitHub.Close()
    
    repo, cleanup := setupTestDatabase(t)
    defer cleanup()
    
    // Create GitHub client pointing to mock server
    githubClient := github.NewClientWithBaseURL(appID, key, logger, mockGitHub.URL)
    
    // Create webhook handler
    webhookHandler := webhook.NewHandler(cfg, repo, githubClient, logger)
    
    // Create scheduler with short interval for testing
    schedulerCfg := &scheduler.Config{
        Interval:   100 * time.Millisecond,
        MaxRetries: 3,
    }
    sched := scheduler.New(repo, githubClient, logger, schedulerCfg)
    
    // Start scheduler
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    sched.Start(ctx)
    defer sched.Stop()
    
    // ============================================
    // Step 1: Send webhook to create reminder
    // ============================================
    payload := createWebhookPayload(t, map[string]interface{}{
        "action": "created",
        "comment": map[string]interface{}{
            "id":       123456,
            "body":     "remora 1 second",  // Short duration for testing
            "html_url": "https://github.com/owner/repo/issues/1#issuecomment-123456",
            "user":     map[string]interface{}{"id": 789, "login": "testuser"},
        },
        "issue":        map[string]interface{}{"number": 1},
        "repository":   map[string]interface{}{"name": "repo", "owner": map[string]interface{}{"login": "owner"}},
        "installation": map[string]interface{}{"id": 12345},
    })
    
    req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(payload))
    req.Header.Set("X-GitHub-Event", "issue_comment")
    req.Header.Set("X-Hub-Signature-256", signPayload(payload))
    
    rr := httptest.NewRecorder()
    webhookHandler.ServeHTTP(rr, req)
    
    assert.Equal(t, http.StatusOK, rr.Code)
    
    // ============================================
    // Step 2: Verify reminder created
    // ============================================
    reminders, err := repo.FindByIssue("owner", "repo", 1)
    require.NoError(t, err)
    require.Len(t, reminders, 1)
    
    reminder := reminders[0]
    assert.Equal(t, models.StatusPending, reminder.Status)
    assert.Equal(t, "testuser", reminder.RequesterUsername)
    assert.Equal(t, int64(123456), reminder.CommentID)
    
    // ============================================
    // Step 3: Verify eyes reaction was added
    // ============================================
    assert.Len(t, mockGitHub.ReactionRequests, 1)
    assert.Equal(t, "eyes", mockGitHub.ReactionRequests[0].Content)
    
    // ============================================
    // Step 4: Wait for scheduler to fire reminder
    // ============================================
    time.Sleep(2 * time.Second)  // Wait for reminder to become due and fire
    
    // ============================================
    // Step 5: Verify reminder fired
    // ============================================
    reminder, err = repo.FindByID(reminder.ID)
    require.NoError(t, err)
    assert.Equal(t, models.StatusFired, reminder.Status)
    assert.NotNil(t, reminder.FiredAt)
    
    // ============================================
    // Step 6: Verify comment was posted
    // ============================================
    assert.Len(t, mockGitHub.CommentRequests, 1)
    comment := mockGitHub.CommentRequests[0]
    assert.Equal(t, "owner", comment.Owner)
    assert.Equal(t, "repo", comment.Repo)
    assert.Equal(t, 1, comment.IssueNumber)
    assert.Contains(t, comment.Body, "@testuser")
    assert.Contains(t, comment.Body, "🔔 Reminder")
}
```

### Additional Lifecycle Tests

| Test                               | Description                                     |
| ---------------------------------- | ----------------------------------------------- |
| `TestLifecycle_CancelByDeletion`   | Delete comment → reminder cancelled             |
| `TestLifecycle_ParseError`         | Invalid command → error reaction added          |
| `TestLifecycle_OverdueReminder`    | Reminder fires late → includes delay annotation |
| `TestLifecycle_RetryOnFailure`     | GitHub API fails → retry with backoff           |
| `TestLifecycle_MaxRetriesExceeded` | Persistent failure → marked as failed           |

---

## Test Fixtures

**Location**: `test/fixtures/`

Pre-built webhook payloads for consistent testing:

| File                           | Description                                |
| ------------------------------ | ------------------------------------------ |
| `webhook_comment_created.json` | Standard reminder creation                 |
| `webhook_comment_deleted.json` | Comment deletion for cancellation          |
| `remora_commands.json`         | Various time expressions for parsing tests |

### Using Fixtures

```go
func loadFixture(t *testing.T, name string) []byte {
    data, err := os.ReadFile(filepath.Join("test", "fixtures", name))
    require.NoError(t, err)
    return data
}

func TestWebhook_WithFixture(t *testing.T) {
    payload := loadFixture(t, "webhook_comment_created.json")
    // ... use payload in test
}
```

---

## Error Scenario Testing

### GitHub API Errors

```go
func TestGitHubClient_ErrorScenarios(t *testing.T) {
    tests := []struct {
        name           string
        statusCode     int
        expectRetry    bool
        expectError    bool
    }{
        {"rate_limited", 429, true, false},   // Retry succeeds
        {"server_error", 500, true, false},   // Retry succeeds
        {"unauthorized", 401, false, true},   // No retry, refresh token
        {"forbidden", 403, false, true},      // No retry
        {"not_found", 404, false, true},      // No retry
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockGitHub := NewMockGitHubServer(t)
            mockGitHub.ReturnStatusCode = tt.statusCode
            // ... test behavior
        })
    }
}
```

### Database Errors

```go
func TestScheduler_DatabaseFailure(t *testing.T) {
    // Use a mock repository that returns errors
    mockRepo := &MockReminderRepository{
        FindDueRemindersError: errors.New("connection lost"),
    }
    
    sched := scheduler.New(mockRepo, mockGitHub, logger, cfg)
    // Verify scheduler handles error gracefully and continues
}
```

---

## Performance Testing

### Scheduler Throughput

```go
func BenchmarkScheduler_ProcessReminders(b *testing.B) {
    repo, cleanup := setupBenchmarkDB(b)
    defer cleanup()
    
    // Create N reminders
    for i := 0; i < 1000; i++ {
        repo.Create(createTestReminder(i))
    }
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        sched.processDueReminders(ctx)
    }
}
```

### Webhook Throughput

```go
func BenchmarkWebhook_HandleRequest(b *testing.B) {
    handler := setupWebhookHandler(b)
    payload := loadFixture(b, "webhook_comment_created.json")
    
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        req := httptest.NewRequest("POST", "/webhook", bytes.NewReader(payload))
        // ... setup headers
        rr := httptest.NewRecorder()
        handler.ServeHTTP(rr, req)
    }
}
```

---

## Test Database Options

### SQLite In-Memory (Default)

Fast, isolated, perfect for unit and integration tests:

```go
cfg := &config.Config{
    DatabaseType:       "sqlite",
    DatabaseSQLitePath: ":memory:",
}
```

### Testcontainers (Optional)

For testing PostgreSQL/MySQL specific behavior:

```go
func TestWithPostgreSQL(t *testing.T) {
    if os.Getenv("TEST_WITH_POSTGRES") == "" {
        t.Skip("Skipping PostgreSQL test")
    }
    
    ctx := context.Background()
    postgres, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
        ContainerRequest: testcontainers.ContainerRequest{
            Image:        "postgres:16-alpine",
            ExposedPorts: []string{"5432/tcp"},
            Env: map[string]string{
                "POSTGRES_DB":       "remora_test",
                "POSTGRES_USER":     "test",
                "POSTGRES_PASSWORD": "test",
            },
            WaitingFor: wait.ForListeningPort("5432/tcp"),
        },
        Started: true,
    })
    require.NoError(t, err)
    defer postgres.Terminate(ctx)
    
    // Get connection details and run tests
    host, _ := postgres.Host(ctx)
    port, _ := postgres.MappedPort(ctx, "5432")
    
    cfg := &config.Config{
        DatabaseType: "postgresql",
        DatabaseHost: host,
        DatabasePort: port.Int(),
        // ...
    }
    
    // Run same tests against PostgreSQL
}
```

---

## CI/CD Integration

### GitHub Actions Configuration

```yaml
# .github/workflows/test.yml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
      
      - name: Run unit tests
        run: go test -v -race -coverprofile=coverage.out ./...
      
      - name: Run integration tests
        run: REMORA_INTEGRATION_TESTS=1 go test -v ./test/integration/...
      
      - name: Upload coverage
        uses: codecov/codecov-action@v4
        with:
          files: coverage.out
```

---

## Running Tests

### All Tests
```bash
make test
# or
go test ./...
```

### With Coverage
```bash
make test-coverage
# or
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration Tests Only
```bash
REMORA_INTEGRATION_TESTS=1 go test -v ./test/integration/...
```

### With Race Detector
```bash
go test -race ./...
```

### Specific Package
```bash
go test -v ./internal/parser/...
```

### Run Single Test
```bash
go test -v -run TestFullReminderLifecycle ./test/integration/...
```

---

## Coverage Goals

| Category       | Target     | Rationale                              |
| -------------- | ---------- | -------------------------------------- |
| Overall        | >80%       | Industry standard for production code  |
| Critical paths | >90%       | Webhook handling, scheduler, parser    |
| Error handling | >85%       | Ensure failures are handled gracefully |
| Edge cases     | Documented | All edge cases from spec have tests    |

---

## Test Naming Conventions

```go
// Unit tests: Test<Function>_<Scenario>
func TestParser_ValidTimeExpression(t *testing.T)
func TestParser_InvalidTimezone(t *testing.T)

// Integration tests: TestIntegration_<Flow>_<Scenario>  
func TestIntegration_WebhookFlow_CreateReminder(t *testing.T)
func TestIntegration_WebhookFlow_CancelReminder(t *testing.T)

// Lifecycle tests: TestLifecycle_<Scenario>
func TestLifecycle_ReminderFires(t *testing.T)
func TestLifecycle_RetryOnFailure(t *testing.T)

// Benchmarks: Benchmark<Component>_<Operation>
func BenchmarkScheduler_ProcessReminders(b *testing.B)
```
