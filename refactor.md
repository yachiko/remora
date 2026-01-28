# Remora Refactoring Guide

This document outlines improvements identified during code review, with clear implementation instructions for each item.

---

## Table of Contents

1. [High Priority](#high-priority)
2. [Medium Priority](#medium-priority)
3. [Low Priority](#low-priority)
4. [Future Enhancements](#future-enhancements)

---

## High Priority

### 1. Fix Context Propagation in Webhook Handler

**Problem**: The webhook handler creates a new `context.Background()` instead of using the request context, which breaks proper cancellation and timeout handling.

**Location**: `internal/webhook/handler.go:118`

**Current Code**:
```go
ctx := context.Background()
switch event.Action {
```

**Fixed Code**:
```go
ctx := r.Context()
switch event.Action {
```

**Implementation Steps**:
1. Open `internal/webhook/handler.go`
2. Find line ~118 where `ctx := context.Background()` is used
3. Replace with `ctx := r.Context()`
4. Run tests: `go test ./internal/webhook/...`

---

### 2. Fix SQLite Locking Strategy in Repository

**Problem**: The SQLite branch in `GetAndLockDueReminders` fetches ALL reminders with status `processing`, not just the ones updated in the current transaction. This could return reminders from other scheduler runs.

**Location**: `internal/database/repository.go:132-142`

**Current Code**:
```go
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
```

**Fixed Code**:
```go
// SQLite: Get IDs first, then update and return
var ids []uint
if err := tx.Model(&models.Reminder{}).
    Where("status = ? AND remind_at <= ?", models.StatusPending, time.Now()).
    Order("remind_at ASC").
    Limit(limit).
    Pluck("id", &ids).Error; err != nil {
    return err
}

if len(ids) == 0 {
    return nil
}

// Update only the selected reminders
if err := tx.Model(&models.Reminder{}).
    Where("id IN ?", ids).
    Update("status", models.StatusProcessing).Error; err != nil {
    return err
}

// Fetch only the reminders we just updated
return tx.Where("id IN ?", ids).Find(&reminders).Error
```

**Implementation Steps**:
1. Open `internal/database/repository.go`
2. Find the SQLite case in `GetAndLockDueReminders` function
3. Replace with the fixed code that tracks IDs explicitly
4. Run tests: `go test ./internal/database/...`

---

### 3. Add Installation ID Validation

**Problem**: No validation that `InstallationID` is positive in the Reminder model.

**Location**: `internal/models/reminder.go`

**Implementation**:
Add to the `IsValid()` method:

```go
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
    if r.InstallationID <= 0 {  // ADD THIS CHECK
        return false
    }
    if r.RequesterUsername == "" || r.RequesterID <= 0 {
        return false
    }
    // ... rest of validation
}
```

**Implementation Steps**:
1. Open `internal/models/reminder.go`
2. Add `InstallationID` check in `IsValid()` method after `CommentID` check
3. Add test case in `internal/models/reminder_test.go` for invalid installation ID
4. Run tests: `go test ./internal/models/...`

---

## Medium Priority

### 4. Add Database Connection Pool Configuration

**Problem**: GORM uses default connection pool settings which may not be optimal for production.

**Location**: `internal/config/config.go` and `internal/database/db.go`

**Implementation Steps**:

**Step 1**: Add config fields in `internal/config/config.go`:
```go
type Config struct {
    // ... existing fields ...
    
    // Database connection pool
    DatabaseMaxOpenConns    int
    DatabaseMaxIdleConns    int
    DatabaseConnMaxLifetime int // seconds
}
```

**Step 2**: Add to `Load()` function:
```go
DatabaseMaxOpenConns:    getEnvAsInt("DATABASE_MAX_OPEN_CONNS", 25),
DatabaseMaxIdleConns:    getEnvAsInt("DATABASE_MAX_IDLE_CONNS", 5),
DatabaseConnMaxLifetime: getEnvAsInt("DATABASE_CONN_MAX_LIFETIME", 300),
```

**Step 3**: Configure pool in `internal/database/db.go` after opening connection:
```go
sqlDB, err := DB.DB()
if err != nil {
    return fmt.Errorf("failed to get underlying sql.DB: %w", err)
}

sqlDB.SetMaxOpenConns(cfg.DatabaseMaxOpenConns)
sqlDB.SetMaxIdleConns(cfg.DatabaseMaxIdleConns)
sqlDB.SetConnMaxLifetime(time.Duration(cfg.DatabaseConnMaxLifetime) * time.Second)
```

**Step 4**: Update `.env.example`:
```bash
# Database Connection Pool
DATABASE_MAX_OPEN_CONNS=25
DATABASE_MAX_IDLE_CONNS=5
DATABASE_CONN_MAX_LIFETIME=300  # seconds
```

---

### 5. Add Request ID Middleware

**Problem**: Request IDs are only available in webhook handlers. Other endpoints lack request tracing.

**Location**: Create new file `internal/api/request_id.go`

**Implementation**:

```go
package api

import (
    "context"
    "net/http"

    "github.com/google/uuid"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        requestID := r.Header.Get("X-Request-ID")
        if requestID == "" {
            requestID = uuid.New().String()
        }
        
        // Add to response header
        w.Header().Set("X-Request-ID", requestID)
        
        // Add to context
        ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// GetRequestID retrieves request ID from context
func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(RequestIDKey).(string); ok {
        return id
    }
    return ""
}
```

**Step 2**: Add dependency:
```bash
go get github.com/google/uuid
```

**Step 3**: Apply middleware in `cmd/remora/main.go`:
```go
// Wrap mux with request ID middleware
handler := api.RequestIDMiddleware(loggingMiddleware(mux, logger.Logger))

server := &http.Server{
    Addr:    fmt.Sprintf(":%d", cfg.Port),
    Handler: handler,
    // ...
}
```

---

### 6. Add Admin API Pagination Metadata

**Problem**: Admin API lacks total count and pagination metadata.

**Location**: `internal/api/handler.go`

**Implementation**:

**Step 1**: Add response struct:
```go
type ListRemindersResponse struct {
    Reminders []*models.Reminder `json:"reminders"`
    Total     int64              `json:"total"`
    Limit     int                `json:"limit"`
    Offset    int                `json:"offset"`
    HasMore   bool               `json:"has_more"`
}
```

**Step 2**: Add `Count` method to repository interface in `internal/database/repository.go`:
```go
type ReminderRepository interface {
    // ... existing methods ...
    
    // Count returns total count of reminders matching filters
    Count(filters map[string]interface{}) (int64, error)
}
```

**Step 3**: Implement in repository:
```go
func (r *reminderRepository) Count(filters map[string]interface{}) (int64, error) {
    var count int64
    query := r.db.Model(&models.Reminder{})
    
    for key, value := range filters {
        query = query.Where(key+" = ?", value)
    }
    
    err := query.Count(&count).Error
    return count, err
}
```

**Step 4**: Update handler to return metadata:
```go
func (h *Handler) ListReminders(w http.ResponseWriter, r *http.Request) {
    // ... existing filter/query logic ...
    
    total, _ := h.repo.Count(filters)
    
    response := ListRemindersResponse{
        Reminders: reminders,
        Total:     total,
        Limit:     limit,
        Offset:    offset,
        HasMore:   int64(offset+len(reminders)) < total,
    }
    
    json.NewEncoder(w).Encode(response)
}
```

---

### 7. Extract Time Provider Interface

**Problem**: Direct use of `time.Now()` makes testing time-dependent code difficult.

**Implementation**:

**Step 1**: Create `internal/clock/clock.go`:
```go
package clock

import "time"

// Clock provides time operations
type Clock interface {
    Now() time.Time
}

// RealClock uses actual system time
type RealClock struct{}

func (RealClock) Now() time.Time {
    return time.Now()
}

// MockClock for testing
type MockClock struct {
    CurrentTime time.Time
}

func (m *MockClock) Now() time.Time {
    return m.CurrentTime
}

func (m *MockClock) Advance(d time.Duration) {
    m.CurrentTime = m.CurrentTime.Add(d)
}
```

**Step 2**: Update scheduler to accept clock:
```go
type Scheduler struct {
    // ... existing fields ...
    clock clock.Clock
}

func New(repo database.ReminderRepository, github GitHubClient, logger *zap.Logger, cfg *Config, clk clock.Clock) *Scheduler {
    if clk == nil {
        clk = clock.RealClock{}
    }
    return &Scheduler{
        // ...
        clock: clk,
    }
}
```

**Step 3**: Replace `time.Now()` calls with `s.clock.Now()`

---

## Low Priority

### 8. Remove Global Logger Variable

**Problem**: `logger.Logger` as a package-level global makes testing harder.

**Recommendation**: The logger is already passed via dependency injection to most components. Consider:
1. Deprecating the global `Logger` variable
2. Removing direct usage of `logger.Info()`, `logger.Error()` etc. helper functions
3. Always pass logger explicitly

**Implementation**: This is a larger refactor. For now, document that new code should use injected loggers.

---

### 9. Make Scheduler Retry Backoff Configurable

**Location**: `internal/scheduler/scheduler.go`

**Implementation**:

Update `Config` struct:
```go
type Config struct {
    Interval       time.Duration
    MaxRetries     int
    InitialBackoff time.Duration
    MaxBackoff     time.Duration
    BackoffMultiplier float64
}

func DefaultConfig() *Config {
    return &Config{
        Interval:          5 * time.Minute,
        MaxRetries:        5,
        InitialBackoff:    1 * time.Minute,
        MaxBackoff:        16 * time.Minute,
        BackoffMultiplier: 2.0,
    }
}
```

Update environment variable loading in config to support these fields.

---

### 10. Add Input Sanitization

**Problem**: User input (comment body) should be sanitized before storage.

**Location**: `internal/webhook/handler.go`

**Implementation**: Add sanitization before storing:
```go
import "html"

// Before creating reminder
sanitizedCommand := html.EscapeString(cmd.OriginalCommand)
```

Note: Be careful not to escape the original GitHub URL.

---

## Future Enhancements

### 11. Add Prometheus Metrics

Create `internal/metrics/metrics.go`:
```go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    RemindersCreated = promauto.NewCounter(prometheus.CounterOpts{
        Name: "remora_reminders_created_total",
        Help: "Total number of reminders created",
    })
    
    RemindersFired = promauto.NewCounter(prometheus.CounterOpts{
        Name: "remora_reminders_fired_total",
        Help: "Total number of reminders fired successfully",
    })
    
    RemindersFailed = promauto.NewCounter(prometheus.CounterOpts{
        Name: "remora_reminders_failed_total",
        Help: "Total number of reminders that failed",
    })
    
    WebhookDuration = promauto.NewHistogram(prometheus.HistogramOpts{
        Name:    "remora_webhook_duration_seconds",
        Help:    "Webhook processing duration",
        Buckets: prometheus.DefBuckets,
    })
    
    GitHubAPILatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
        Name:    "remora_github_api_latency_seconds",
        Help:    "GitHub API request latency",
        Buckets: prometheus.DefBuckets,
    }, []string{"endpoint", "status"})
)
```

Add `/metrics` endpoint using `promhttp.Handler()`.

---

### 12. Add Circuit Breaker for GitHub API

Use `github.com/sony/gobreaker`:

```go
import "github.com/sony/gobreaker"

type Client struct {
    // ... existing fields ...
    circuitBreaker *gobreaker.CircuitBreaker
}

func NewClient(...) *Client {
    cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        "github-api",
        MaxRequests: 3,
        Interval:    10 * time.Second,
        Timeout:     60 * time.Second,
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures > 5
        },
    })
    
    return &Client{
        // ...
        circuitBreaker: cb,
    }
}
```

---

### 13. Add Rate Limiter `ShouldWait` Method

**Location**: `internal/github/ratelimit.go`

```go
// ShouldWait returns true and wait duration if rate limit is too low
func (c *Client) ShouldWait() (bool, time.Duration) {
    c.rateLimiter.mu.RLock()
    defer c.rateLimiter.mu.RUnlock()
    
    // If less than 5% remaining, suggest waiting
    if c.rateLimiter.limit > 0 {
        percentRemaining := float64(c.rateLimiter.remaining) / float64(c.rateLimiter.limit) * 100
        if percentRemaining < 5 {
            waitTime := time.Until(c.rateLimiter.resetAt)
            if waitTime > 0 {
                return true, waitTime
            }
        }
    }
    return false, 0
}
```

---

## Implementation Priority Order

1. **Immediate** (bugs/correctness):
   - [ ] Fix context propagation (#1)
   - [ ] Fix SQLite locking (#2)
   - [ ] Add installation ID validation (#3)

2. **Short-term** (production readiness):
   - [ ] Add database connection pool config (#4)
   - [ ] Add request ID middleware (#5)
   - [ ] Add pagination metadata (#6)

3. **Medium-term** (maintainability):
   - [ ] Extract time provider (#7)
   - [ ] Remove global logger (#8)
   - [ ] Make retry backoff configurable (#9)
   - [ ] Add input sanitization (#10)

4. **Long-term** (observability & resilience):
   - [ ] Add Prometheus metrics (#11)
   - [ ] Add circuit breaker (#12)
   - [ ] Add proactive rate limiting (#13)

---

## Testing After Refactors

After implementing any refactor, ensure:

```bash
# Run all tests
go test ./...

# Run with race detector
go test -race ./...

# Run linter
golangci-lint run ./...

# Check coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```
