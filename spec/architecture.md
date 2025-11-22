# Remora Architecture

## Overview

Remora is a GitHub-integrated reminder bot that allows users to set reminders on issues and pull requests using natural language. It runs as a self-contained Docker container with webhook-based GitHub App integration.

## System Components

### 1. GitHub App Webhook Handler
- **Purpose**: Receives and processes webhook events from GitHub
- **Responsibilities**:
  - Validates webhook signatures for security
  - Filters for comment events on issues and PRs
  - Extracts comment body and metadata
  - Routes valid "remora" commands to the parser

### 2. Command Parser
- **Purpose**: Parses natural language time expressions from comments
- **Responsibilities**:
  - Detects "remora" prefix in comments
  - Extracts time expression from command
  - Uses `olebedev/when` library for natural language date/time parsing
  - Validates parsed dates (must be in the future)
  - Returns structured reminder data

### 3. Database Layer
- **Purpose**: Persistent storage for reminders and app state
- **Technology**: GORM with support for PostgreSQL, MySQL, and SQLite
- **Responsibilities**:
  - Stores pending reminders with metadata
  - Tracks reminder status (pending, fired, failed)
  - Supports queries for due reminders
  - Handles database migrations

### 4. Scheduler
- **Purpose**: Monitors and executes due reminders
- **Implementation**: In-process Go ticker/scheduler
- **Responsibilities**:
  - Periodically polls database for due reminders
  - Marks reminders as processing to prevent duplicates
  - Triggers reminder delivery
  - Updates reminder status after execution

### 5. GitHub API Client
- **Purpose**: Interacts with GitHub API for reactions and comments
- **Responsibilities**:
  - Adds reactions (👀 or ✅) to acknowledge reminder requests
  - Posts reminder comments mentioning original user
  - Handles error reactions (❌) for invalid requests
  - Manages GitHub App authentication and token refresh

### 6. Configuration Manager
- **Purpose**: Loads and validates application configuration
- **Implementation**: Environment variables only
- **Key Settings**:
  - Database connection parameters
  - GitHub App credentials (App ID, private key, webhook secret)
  - Scheduler polling interval
  - Error handling behavior (reactions only vs reactions + comments)

## Data Flow

### Setting a Reminder

```
GitHub Comment
    ↓
Webhook Event → Webhook Handler
    ↓
Validate & Extract → Command Parser
    ↓
Parse Time Expression (olebedev/when)
    ↓
Store Reminder → Database (GORM)
    ↓
Acknowledge → GitHub API (add reaction: 👀 or ✅)
```

### Firing a Reminder

```
Scheduler (periodic check)
    ↓
Query Due Reminders → Database
    ↓
For each reminder:
    ↓
Format Comment (mention user, link to original)
    ↓
Post Comment → GitHub API
    ↓
Update Status → Database (mark as fired)
```

---

## State Machine

### Reminder Lifecycle

```
                    ┌─────────────┐
                    │   PENDING   │
                    └──────┬──────┘
                           │
                           │ Scheduler picks up
                           │ (remind_at <= NOW)
                           ▼
                    ┌─────────────┐
                    │ PROCESSING  │
                    └──────┬──────┘
                           │
                 ┌─────────┴─────────┐
                 │                   │
         Success │                   │ Failure
                 ▼                   ▼
          ┌───────────┐       ┌───────────┐
          │   FIRED   │       │  FAILED   │
          └───────────┘       └─────┬─────┘
                                    │
                                    │ retry_count < 5
                                    │ (exponential backoff)
                                    │
                                    ▼
                             ┌─────────────┐
                             │   PENDING   │
                             └─────────────┘
                                    │
                                    │ retry_count >= 5
                                    ▼
                             ┌─────────────┐
                             │   FAILED    │
                             │  (terminal) │
                             └─────────────┘

          ┌─────────────────────────────────────┐
          │  CANCELLED (via comment deletion)   │
          └─────────────────────────────────────┘
```

### State Transitions

| From | To | Trigger | Conditions |
|------|-------|---------|------------|
| N/A | `pending` | Webhook creates reminder | Valid time expression, future date |
| `pending` | `processing` | Scheduler query | `remind_at <= NOW()` |
| `processing` | `fired` | GitHub API success | Comment posted successfully |
| `processing` | `failed` | GitHub API error | Network error, API error, permission denied |
| `failed` | `pending` | Retry logic | `retry_count < 5`, wait for backoff period |
| `failed` | `failed` (terminal) | Max retries reached | `retry_count >= 5` |
| `pending` | `cancelled` | Comment deleted | `issue_comment.deleted` webhook event |
| Any | `cancelled` | Manual intervention | Admin cancellation (future feature) |

### State Descriptions

- **`pending`**: Reminder created, waiting for scheduled time
- **`processing`**: Scheduler has picked up reminder and is attempting delivery
- **`fired`**: Reminder successfully delivered to GitHub
- **`failed`**: Reminder delivery failed (after retries or terminal failure)
- **`cancelled`**: Reminder cancelled by user (comment deletion) or admin

---

## Sequence Diagrams

### Successful Reminder Creation

```
User          GitHub        Remora         Database      GitHub API
 │               │             │               │              │
 │──comment──────>│             │               │              │
 │ "remora 2d"   │             │               │              │
 │               │             │               │              │
 │               │──webhook────>│               │              │
 │               │  POST       │               │              │
 │               │             │               │              │
 │               │             │──validate─────│              │
 │               │             │  signature    │              │
 │               │             │               │              │
 │               │             │──parse────────│              │
 │               │             │  command      │              │
 │               │             │               │              │
 │               │             │──INSERT────────>│              │
 │               │             │  reminder     │              │
 │               │             │<──success──────│              │
 │               │             │               │              │
 │               │             │──add reaction──────────────────>│
 │               │             │  👀 or ✅     │              │
 │               │             │<──201 Created──────────────────│
 │               │             │               │              │
 │               │<──200 OK────│               │              │
 │               │             │               │              │
```

### Error Handling Flow

```
User          GitHub        Remora         Database      GitHub API
 │               │             │               │              │
 │──comment──────>│             │               │              │
 │"remora asap"  │             │               │              │
 │               │             │               │              │
 │               │──webhook────>│               │              │
 │               │             │               │              │
 │               │             │──validate─────│              │
 │               │             │               │              │
 │               │             │──parse────────│              │
 │               │             │  (fails)      │              │
 │               │             │               │              │
 │               │             │──add reaction──────────────────>│
 │               │             │  ❌           │              │
 │               │             │<──201─────────────────────────│
 │               │             │               │              │
 │               │             │──post error────────────────────>│
 │               │             │  comment      │              │
 │               │             │  (if enabled) │              │
 │               │             │<──201─────────────────────────│
 │               │             │               │              │
 │               │<──200 OK────│               │              │
 │               │             │               │              │
```

### Reminder Firing Flow

```
Scheduler     Database      GitHub API      GitHub
    │             │              │              │
    │──query──────>│              │              │
    │  due        │              │              │
    │  reminders  │              │              │
    │<──results───│              │              │
    │   [r1, r2]  │              │              │
    │             │              │              │
    │──UPDATE─────>│              │              │
    │  r1 to      │              │              │
    │  processing │              │              │
    │<──success───│              │              │
    │             │              │              │
    │──post comment───────────────>│              │
    │  "@user reminder"           │              │
    │<──201 Created───────────────│              │
    │             │              │              │
    │──UPDATE─────>│              │              │
    │  r1 to fired│              │              │
    │  set fired_at              │              │
    │<──success───│              │              │
    │             │              │              │
    │   (repeat for r2...)       │              │
    │             │              │              │
```

### Retry Flow on Failure

```
Scheduler     Database      GitHub API
    │             │              │
    │──UPDATE─────>│              │
    │  to         │              │
    │  processing │              │
    │             │              │
    │──post comment───────────────>│
    │             │              X (network error)
    │             │              │
    │──UPDATE─────>│              │
    │  to failed  │              │
    │  retry_count++            │
    │  error_msg  │              │
    │             │              │
    │   (wait 1 min - exponential backoff)
    │             │              │
    │──UPDATE─────>│              │
    │  to pending │              │
    │  (retry)    │              │
    │             │              │
    │   (scheduler picks up again)
    │             │              │
```

## Monitoring & Observability

### Metrics to Expose

**Reminder Metrics**:
- `remora_reminders_total{status}` (counter) - Total reminders created by status
- `remora_reminders_pending` (gauge) - Current count of pending reminders
- `remora_reminders_processing` (gauge) - Current count of processing reminders
- `remora_reminders_fired_total` (counter) - Total successfully fired reminders
- `remora_reminders_failed_total` (counter) - Total failed reminders
- `remora_reminders_cancelled_total` (counter) - Total cancelled reminders
- `remora_reminder_age_seconds{status}` (histogram) - Age of reminders by status

**Webhook Metrics**:
- `remora_webhook_requests_total{status}` (counter) - Webhook requests (success/error)
- `remora_webhook_duration_seconds` (histogram) - Webhook processing latency
- `remora_webhook_signature_failures_total` (counter) - Invalid signature attempts
- `remora_webhook_parse_errors_total` (counter) - Command parsing failures

**Scheduler Metrics**:
- `remora_scheduler_runs_total` (counter) - Total scheduler executions
- `remora_scheduler_duration_seconds` (histogram) - Scheduler cycle duration
- `remora_scheduler_reminders_processed` (counter) - Reminders processed per run
- `remora_scheduler_errors_total` (counter) - Scheduler execution errors

**GitHub API Metrics**:
- `remora_github_api_calls_total{endpoint,status}` (counter) - API calls by endpoint
- `remora_github_api_duration_seconds{endpoint}` (histogram) - API call latency
- `remora_github_api_rate_limit_remaining` (gauge) - Remaining API quota
- `remora_github_api_errors_total{type}` (counter) - API errors by type

**Database Metrics**:
- `remora_db_queries_total{operation}` (counter) - Database queries (SELECT/INSERT/UPDATE)
- `remora_db_query_duration_seconds{operation}` (histogram) - Query latency
- `remora_db_connection_pool_size` (gauge) - Active connections
- `remora_db_errors_total` (counter) - Database errors

**System Metrics**:
- `remora_uptime_seconds` (gauge) - Application uptime
- `remora_info{version,go_version}` (gauge) - Build information

### Logging Strategy

**Log Levels**:
- **DEBUG**: Detailed flow (parser output, cache hits, etc.)
- **INFO**: Normal operations (webhook received, reminder created, reminder fired)
- **WARN**: Recoverable issues (rate limit approaching, retry attempts)
- **ERROR**: Failures requiring attention (API errors, database errors)
- **FATAL**: Unrecoverable errors (startup failures)

**Structured Log Fields**:
```json
{
  "timestamp": "2025-11-22T10:30:00Z",
  "level": "info",
  "message": "reminder created",
  "reminder_id": 123,
  "repository": "owner/repo",
  "issue_number": 456,
  "requester": "username",
  "remind_at": "2025-11-24T10:30:00Z",
  "installation_id": 789
}
```

**Key Events to Log**:
- Webhook received (INFO)
- Reminder created (INFO)
- Reminder fired (INFO)
- Parsing errors (WARN)
- API errors (ERROR)
- Retry attempts (WARN)
- Database errors (ERROR)
- Scheduler runs (DEBUG/INFO)

### Tracing (Future)

**Distributed Tracing**:
- Span for webhook request → reminder creation
- Span for scheduler → reminder firing
- Trace IDs for end-to-end flow correlation
- Implementation: OpenTelemetry (Phase 12+)

### Health Check Details

**Liveness Probe** (`/health`):
- Application running
- Database connection active
- Returns 200 if alive, 503 if not

**Readiness Probe** (`/ready`):
- Migrations completed
- Scheduler started
- GitHub API reachable (cached check)
- Returns 200 if ready, 503 if not

### Alerting Recommendations

**Critical Alerts**:
- Database connection failures (alert immediately)
- GitHub API authentication failures (alert immediately)
- Scheduler stopped/crashed (alert after 10 minutes)
- High error rate (>5% of requests) (alert after 5 minutes)

**Warning Alerts**:
- GitHub API rate limit < 10% remaining
- Reminder queue growing (>1000 pending reminders)
- High retry rate (>20% of fired reminders)
- Database query latency >1s (P95)

**SLO/SLI Examples**:
- **Availability**: 99.5% uptime (measured via health checks)
- **Webhook Processing**: 95% processed within 2 seconds
- **Reminder Accuracy**: 99% fired within 5 minutes of scheduled time
- **Error Rate**: <1% of webhook requests fail

---

## Deployment Architecture

### Container Structure
- Single Docker container
- Exposes HTTPS endpoint for GitHub webhooks
- Connects to external database (PostgreSQL/MySQL) or embedded SQLite
- Stateless application (all state in database)

### Network Requirements
- Publicly accessible HTTPS endpoint for webhooks
- Outbound HTTPS to GitHub API (api.github.com)
- Database connection (depends on database choice)

## Resource Requirements

### Compute Resources

**Minimum (Development/Testing)**:
- **CPU**: 0.25 cores (250m)
- **Memory**: 256 MB
- **Disk**: 100 MB (application + logs)

**Recommended (Production)**:
- **CPU**: 0.5 - 1 core (500m - 1000m)
- **Memory**: 512 MB - 1 GB
- **Disk**: 1 GB (application + logs + buffer)

**Estimated Load Handling**:
- **Webhooks**: 100 concurrent requests
- **Reminders**: 10,000 pending reminders
- **Scheduler**: Process 100 reminders per cycle (5 minutes)
- **Database Connections**: 10-20 active connections

### Memory Breakdown

```
Application Binary:        ~20 MB
Go Runtime:               ~50 MB
Database Connection Pool:  ~10 MB
Token Cache:              ~5 MB (100 installations)
Active Goroutines:        ~10 MB (100 concurrent)
Logging Buffers:          ~5 MB
Operating Overhead:       ~50 MB
────────────────────────────────
Total:                    ~150 MB base + workload
```

**Recommended**: 512 MB for comfortable headroom

### Database Resources

**PostgreSQL/MySQL**:
- **Storage**: 10 MB + ~1 KB per reminder
  - 10,000 reminders ≈ 20 MB
  - 100,000 reminders ≈ 110 MB
- **Connections**: 10-20 from Remora
- **IOPS**: Low (scheduler queries + occasional writes)

**SQLite** (Development only):
- **Storage**: Single file, ~1 KB per reminder
- **Not recommended for production** (concurrent write limitations)

### Network

**Bandwidth**:
- **Inbound**: Minimal (webhook payloads ~2-10 KB each)
- **Outbound**: Minimal (GitHub API calls ~1-5 KB each)
- **Estimated**: <1 MB/hour for typical usage

**Connections**:
- Webhook endpoint (HTTPS): 443
- Database: Varies (5432 PostgreSQL, 3306 MySQL)
- GitHub API: Outbound HTTPS to api.github.com

### Scaling Considerations

**Vertical Scaling**:
- Increase CPU for higher webhook concurrency
- Increase memory for larger token cache

**Horizontal Scaling** (Future):
- Requires distributed locking (Redis/etcd)
- Each instance: Same resource requirements
- Load balancer for webhook distribution

**Database Scaling**:
- Read replicas not needed (low read volume)
- Vertical scaling sufficient (more CPU/memory)
- Sharding not needed at expected scale

### Performance Benchmarks

**Target Performance**:
- Webhook response time: <200ms (P95)
- Scheduler cycle: <30s for 100 reminders
- Database query time: <50ms (P95)
- GitHub API call: <500ms (P95, depends on GitHub)

**Stress Test Targets**:
- 1000 webhooks/minute sustained
- 10,000 pending reminders in database
- 100 reminders firing simultaneously

---

## Security Considerations

- Webhook signature validation (HMAC-SHA256)
- GitHub App private key stored as environment variable
- No user tokens stored (GitHub App authentication only)
- Database credentials via environment variables
- TLS/HTTPS required for webhook endpoint

## Scalability Notes

- Current design: Single instance with in-process scheduler
- Database is the single source of truth
- Horizontal scaling possible with distributed locking (future enhancement)
- GitHub API rate limits respected via exponential backoff
