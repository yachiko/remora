# Observability Strategy

## Overview

This document specifies the observability strategy for Remora, including structured logging, metrics, and operational monitoring.

---

## Structured Logging

### Log Format

**Console format** for human readability:

```
2025-11-23T10:30:00.123Z  INFO  [webhook] reminder created  reminder_id=123 repository=owner/repo issue_number=456 requester=username
```

**Format specification**:
- Timestamp: ISO 8601 with milliseconds
- Level: INFO, WARN, ERROR, DEBUG, FATAL
- Component: In brackets [component]
- Message: Human-readable description
- Fields: key=value pairs (space-separated)

### Log Levels

| Level     | Usage                        | Examples                                                |
| --------- | ---------------------------- | ------------------------------------------------------- |
| **DEBUG** | Detailed flow, parser output | Time expression parsed, cache hit/miss, query results   |
| **INFO**  | Normal operations            | Webhook received, reminder created, reminder fired      |
| **WARN**  | Recoverable issues           | Rate limit approaching, retry attempt, overdue reminder |
| **ERROR** | Failures requiring attention | API errors, database errors, parsing failures           |
| **FATAL** | Unrecoverable errors         | Database connection failed on startup, invalid config   |

### Standard Log Fields

**All log entries include**:
- `timestamp`: ISO 8601 format with milliseconds
- `level`: Log level (debug, info, warn, error, fatal)
- `message`: Human-readable message
- `component`: Component name (webhook, parser, scheduler, github, database)
- `request_id`: Unique ID for request correlation (webhooks only)

**Context-specific fields**:
- `reminder_id`: Reminder database ID
- `repository`: Repository full name (owner/repo)
- `issue_number`: Issue or PR number
- `requester`: GitHub username
- `installation_id`: GitHub App installation ID
- `comment_id`: GitHub comment ID
- `error`: Error message (for ERROR/FATAL levels)
- `stack_trace`: Stack trace (for ERROR/FATAL levels)
- `duration_ms`: Operation duration in milliseconds

---

## Log Event Catalog

### Webhook Events

#### 1. Webhook Received

**Level**: INFO  
**Component**: webhook  
**Message**: "webhook received"

```
2025-11-23T10:30:00.123Z  INFO  [webhook] webhook received  request_id=abc-123 event_type=issue_comment action=created installation_id=789 repository=owner/repo signature_valid=true
```

#### 2. Webhook Signature Invalid

**Level**: WARN  
**Component**: webhook  
**Message**: "webhook signature validation failed"

```
2025-11-23T10:30:00.123Z  WARN  [webhook] webhook signature validation failed  request_id=abc-123 repository=owner/repo remote_addr=192.0.2.1
```

#### 3. Webhook Payload Validation Failed

**Level**: WARN  
**Component**: webhook  
**Message**: "webhook payload validation failed"

```
2025-11-23T10:30:00.123Z  WARN  [webhook] webhook payload validation failed  request_id=abc-123 error="missing required field: comment.body"
```

---

### Parser Events

#### 4. Command Detected

**Level**: DEBUG  
**Component**: parser  
**Message**: "remora command detected"

```
2025-11-23T10:30:00.123Z  DEBUG [parser] remora command detected  request_id=abc-123 repository=owner/repo issue_number=456 comment_id=789012 original_command="remora 2 days"
```

#### 5. Time Expression Parsed

**Level**: DEBUG  
**Component**: parser  
**Message**: "time expression parsed"

```
2025-11-23T10:30:00.123Z  DEBUG [parser] time expression parsed  request_id=abc-123 original_command="remora 2 days" parsed_time=2025-11-25T10:30:00Z timezone=UTC
```

#### 6. Parsing Failed

**Level**: WARN  
**Component**: parser  
**Message**: "failed to parse time expression"

```
2025-11-23T10:30:00.123Z  WARN  [parser] failed to parse time expression  request_id=abc-123 repository=owner/repo issue_number=456 original_command="remora asap" error="no time match found"
```

#### 7. Validation Failed

**Level**: WARN  
**Component**: parser  
**Message**: "reminder validation failed"

```
2025-11-23T10:30:00.123Z  WARN  [parser] reminder validation failed  request_id=abc-123 repository=owner/repo issue_number=456 original_command="remora yesterday" parsed_time=2025-11-22T10:30:00Z validation_error="time must be in the future"
```

---

### Database Events

#### 8. Reminder Created

**Level**: INFO  
**Component**: database  
**Message**: "reminder created"

```
2025-11-23T10:30:00.123Z  INFO  [database] reminder created  request_id=abc-123 reminder_id=123 repository=owner/repo issue_number=456 requester=alice remind_at=2025-11-25T10:30:00Z original_command="remora 2 days"
```

#### 9. Database Error

**Level**: ERROR  
**Component**: database  
**Message**: "database operation failed"

```
2025-11-23T10:30:00.123Z  ERROR [database] database operation failed  operation=insert error="connection timeout" duration_ms=5000
```

---

### GitHub API Events

#### 10. Reaction Added

**Level**: INFO  
**Component**: github  
**Message**: "reaction added"

```
2025-11-23T10:30:00.123Z  INFO  [github] reaction added  request_id=abc-123 repository=owner/repo comment_id=789012 reaction=eyes status_code=201 duration_ms=450
```

#### 11. GitHub API Error

**Level**: ERROR  
**Component**: github  
**Message**: "github api request failed"

```
2025-11-23T10:30:00.123Z  ERROR [github] github api request failed  endpoint="POST /repos/owner/repo/issues/comments" status_code=403 error="Resource not accessible by integration" duration_ms=320
```

#### 12. Rate Limit Warning

**Level**: WARN  
**Component**: github  
**Message**: "github api rate limit low"

```
2025-11-23T10:30:00.123Z  WARN  [github] github api rate limit low  rate_limit_remaining=450 rate_limit_total=5000 rate_limit_reset=2025-11-23T11:30:00Z
```

---

### Scheduler Events

#### 13. Scheduler Cycle Started

**Level**: INFO  
**Component**: scheduler  
**Message**: "scheduler cycle started"

```
2025-11-23T10:30:00.123Z  INFO  [scheduler] scheduler cycle started  cycle_id=cycle-789
```

#### 14. Due Reminders Found

**Level**: INFO  
**Component**: scheduler  
**Message**: "due reminders found"

```
2025-11-23T10:30:00.123Z  INFO  [scheduler] due reminders found  cycle_id=cycle-789 count=5 oldest_remind_at=2025-11-23T09:00:00Z
```

#### 15. Reminder Fired

**Level**: INFO  
**Component**: scheduler  
**Message**: "reminder fired"

```
2025-11-23T10:30:00.123Z  INFO  [scheduler] reminder fired  reminder_id=123 repository=owner/repo issue_number=456 requester=alice remind_at=2025-11-23T10:30:00Z fired_at=2025-11-23T10:30:15Z delay_seconds=15
```

#### 16. Reminder Fired Late

**Level**: WARN  
**Component**: scheduler  
**Message**: "reminder fired late"

```
2025-11-23T10:30:00.123Z  WARN  [scheduler] reminder fired late  reminder_id=123 repository=owner/repo remind_at=2025-11-23T08:00:00Z fired_at=2025-11-23T10:30:00Z delay_hours=2.5
```

#### 17. Reminder Failed

**Level**: ERROR  
**Component**: scheduler  
**Message**: "reminder firing failed"

```
2025-11-23T10:30:00.123Z  ERROR [scheduler] reminder firing failed  reminder_id=123 repository=owner/repo issue_number=456 retry_count=2 error="github api error: 403 Forbidden" will_retry=true next_retry_at=2025-11-23T10:32:00Z
```

#### 18. Reminder Retry Exhausted

**Level**: ERROR  
**Component**: scheduler  
**Message**: "reminder retry exhausted"

```
2025-11-23T10:30:00.123Z  ERROR [scheduler] reminder retry exhausted  reminder_id=123 repository=owner/repo retry_count=5 error="github api error: 404 Not Found"
```

#### 19. Scheduler Cycle Completed

**Level**: INFO  
**Component**: scheduler  
**Message**: "scheduler cycle completed"

```
2025-11-23T10:30:00.123Z  INFO  [scheduler] scheduler cycle completed  cycle_id=cycle-789 duration_ms=2340 reminders_processed=5 reminders_fired=4 reminders_failed=1
```

---

### Application Events

#### 20. Application Started

**Level**: INFO  
**Component**: main  
**Message**: "application started"

```
2025-11-23T10:00:00.123Z  INFO  [main] application started  version=1.0.0 go_version=1.21.5 database=postgresql port=8080
```

#### 21. Health Check

**Level**: DEBUG  
**Component**: health  
**Message**: "health check"

```
2025-11-23T10:30:00.123Z  DEBUG [health] health check  status=healthy database=ok duration_ms=5
```

#### 22. Graceful Shutdown

**Level**: INFO  
**Component**: main  
**Message**: "graceful shutdown initiated"

```
2025-11-23T10:00:00.123Z  INFO  [main] graceful shutdown initiated  signal=SIGTERM
```

---

## Metrics

### Metrics Endpoint

**Endpoint**: `GET /metrics`  
**Format**: Prometheus text format  
**Library**: `github.com/prometheus/client_golang/prometheus`

### Implementation

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

var (
    // Counter: Total reminders created by status
    remindersTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "remora_reminders_total",
            Help: "Total number of reminders created",
        },
        []string{"status"}, // Labels: pending, fired, failed, cancelled
    )
    
    // Gauge: Current count of pending reminders
    remindersPending = promauto.NewGauge(
        prometheus.GaugeOpts{
            Name: "remora_reminders_pending",
            Help: "Current number of pending reminders",
        },
    )
)

// Usage examples:

// Increment counter when reminder created
remindersTotal.WithLabelValues("pending").Inc()

// Increment counter when reminder fired
remindersTotal.WithLabelValues("fired").Inc()

// Increment counter when reminder failed
remindersTotal.WithLabelValues("failed").Inc()

// Increment counter when reminder cancelled
remindersTotal.WithLabelValues("cancelled").Inc()

// Update gauge with current pending count
func updatePendingCount(db *gorm.DB) {
    var count int64
    db.Model(&Reminder{}).Where("status = ?", "pending").Count(&count)
    remindersPending.Set(float64(count))
}

// Call updatePendingCount() after each scheduler cycle
```

### Metric Specifications

#### 1. remora_reminders_total

**Type**: Counter  
**Labels**: `status` (pending, fired, failed, cancelled)  
**Description**: Total number of reminders created, incremented for each status transition

**Example Prometheus output**:
```
# HELP remora_reminders_total Total number of reminders created
# TYPE remora_reminders_total counter
remora_reminders_total{status="pending"} 1234
remora_reminders_total{status="fired"} 980
remora_reminders_total{status="failed"} 12
remora_reminders_total{status="cancelled"} 8
```

**Query Examples**:
```promql
# Rate of reminders created per minute
rate(remora_reminders_total{status="pending"}[5m])

# Success rate (fired / total)
rate(remora_reminders_total{status="fired"}[5m]) / 
  rate(remora_reminders_total[5m])

# Error rate
rate(remora_reminders_total{status="failed"}[5m])
```

#### 2. remora_reminders_pending

**Type**: Gauge  
**Labels**: None  
**Description**: Current count of reminders in pending state

**Example Prometheus output**:
```
# HELP remora_reminders_pending Current number of pending reminders
# TYPE remora_reminders_pending gauge
remora_reminders_pending 45
```

**Query Examples**:
```promql
# Current pending count
remora_reminders_pending

# Alert if pending count growing
delta(remora_reminders_pending[1h]) > 100
```

### Metric Collection Points

**Reminder Created**:
```go
// In webhook handler after successful database insert
remindersTotal.WithLabelValues("pending").Inc()
```

**Reminder Fired**:
```go
// In scheduler after successful GitHub API call
remindersTotal.WithLabelValues("fired").Inc()
```

**Reminder Failed**:
```go
// In scheduler after max retries exhausted
remindersTotal.WithLabelValues("failed").Inc()
```

**Reminder Cancelled**:
```go
// In webhook handler for comment deletion
remindersTotal.WithLabelValues("cancelled").Inc()
```

**Pending Count Update**:
```go
// At the end of each scheduler cycle
var count int64
db.Model(&Reminder{}).Where("status = ?", "pending").Count(&count)
remindersPending.Set(float64(count))
```

---

## Observability Deployment Checklist

### Pre-Deployment

- [ ] **Logging configured**
  - [ ] Console format enabled
  - [ ] Log level set appropriately (INFO for production, DEBUG for dev)
  - [ ] All log events emit required fields (timestamp, level, component, message)
  
- [ ] **Metrics endpoint exposed**
  - [ ] `/metrics` endpoint accessible
  - [ ] Prometheus can scrape endpoint
  - [ ] Metrics library initialized correctly
  
- [ ] **Health checks responding**
  - [ ] `/health` endpoint returns 200 when healthy
  - [ ] `/ready` endpoint returns 200 when ready
  - [ ] Kubernetes/load balancer probes configured

### Post-Deployment

- [ ] **Log aggregation configured**
  - [ ] Logs flowing to aggregation system (CloudWatch, Elasticsearch, Loki, etc.)
  - [ ] Log retention policy configured (recommend 30+ days)
  - [ ] Log queries tested (search by reminder_id, repository, requester)
  
- [ ] **Metrics collection active**
  - [ ] Prometheus scraping `/metrics` endpoint
  - [ ] Metrics appearing in Prometheus UI
  - [ ] Data retention configured (recommend 90+ days)
  
- [ ] **Dashboards created**
  - [ ] Reminder throughput dashboard
  - [ ] Pending reminders gauge
  - [ ] Error rate tracking
  - [ ] Scheduler performance
  
- [ ] **Alerts configured**
  - [ ] Critical: Database connection failures
  - [ ] Critical: GitHub API authentication failures
  - [ ] Warning: High pending reminder count (>1000)
  - [ ] Warning: Error rate >5%
  
- [ ] **On-call setup**
  - [ ] On-call rotation defined
  - [ ] Alert routing configured (PagerDuty, Slack, email)
  - [ ] Runbook created for common issues

### Operational Verification

- [ ] **Test log queries**
  - [ ] Search for specific reminder by ID
  - [ ] Find all errors in last 24 hours
  - [ ] Trace webhook → reminder → firing flow
  
- [ ] **Test metrics queries**
  - [ ] Verify reminder creation rate
  - [ ] Check pending count matches database
  - [ ] Confirm error metrics incrementing
  
- [ ] **Test alerts**
  - [ ] Trigger test alert (simulate error)
  - [ ] Verify alert reaches on-call
  - [ ] Confirm alert auto-resolves
  
- [ ] **Document**
  - [ ] Dashboard URLs documented
  - [ ] Alert runbook created
  - [ ] Common queries documented
  - [ ] Troubleshooting guide updated

---

## Grafana Dashboard Recommendations

### Panel 1: Reminder Creation Rate

**Metric**: `rate(remora_reminders_total{status="pending"}[5m])`  
**Visualization**: Graph (line chart)  
**Description**: Reminders created per second

### Panel 2: Pending Reminders

**Metric**: `remora_reminders_pending`  
**Visualization**: Gauge  
**Description**: Current count of pending reminders  
**Alert**: Warning if > 1000

### Panel 3: Reminder Status Breakdown

**Metrics**: 
- `remora_reminders_total{status="fired"}`
- `remora_reminders_total{status="failed"}`
- `remora_reminders_total{status="cancelled"}`

**Visualization**: Stacked area chart  
**Description**: Reminder outcomes over time

### Panel 4: Error Rate

**Metric**: `rate(remora_reminders_total{status="failed"}[5m])`  
**Visualization**: Graph (line chart)  
**Description**: Failed reminders per second  
**Alert**: Warning if > 0.05 (5% error rate)

---

## Common Log Queries

### Find All Errors in Last Hour

**grep/awk**:
```bash
grep "ERROR" app.log | awk '{if ($1 > "'$(date -u -d '1 hour ago' '+%Y-%m-%dT%H:%M:%S')'" ) print}'
```

### Trace Specific Reminder

**Find by reminder_id**:
```bash
grep "reminder_id=123" app.log
```

### Find Reminders for Repository

```bash
grep "repository=owner/repo" app.log | grep "reminder created"
```

### Scheduler Performance

```bash
grep "\[scheduler\] scheduler cycle completed" app.log
```

---

## Summary

This document specifies:
- ✅ Console log format with structured fields (key=value pairs)
- ✅ 22 log events with complete field specifications
- ✅ 2 core metrics (counter + gauge) with Prometheus implementation
- ✅ Observability deployment checklist (pre/post deployment, verification)
- ✅ Grafana dashboard recommendations
- ✅ Common log query examples (grep/awk)

All observability specifications are production-ready and ready for implementation in Phase 1.
