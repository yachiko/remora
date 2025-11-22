# Database Schema

## Overview

Remora uses GORM for database abstraction, supporting PostgreSQL, MySQL, and SQLite. The schema is designed to be simple and efficient for the core reminder functionality.

## Tables

### `reminders`

Primary table storing all reminder records.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | BIGINT/INTEGER | PRIMARY KEY, AUTO_INCREMENT | Unique reminder identifier |
| `created_at` | TIMESTAMP | NOT NULL | Record creation time (GORM default) |
| `updated_at` | TIMESTAMP | NOT NULL | Last update time (GORM default) |
| `deleted_at` | TIMESTAMP | NULL, INDEX | Soft delete timestamp (GORM default) |
| `repository_owner` | VARCHAR(255) | NOT NULL | GitHub repository owner (org or user) |
| `repository_name` | VARCHAR(255) | NOT NULL | GitHub repository name |
| `issue_number` | INTEGER | NOT NULL | GitHub issue or PR number |
| `comment_id` | BIGINT | NOT NULL | GitHub comment ID where reminder was requested |
| `comment_url` | VARCHAR(512) | NOT NULL | Full URL to the original comment |
| `requester_username` | VARCHAR(255) | NOT NULL | GitHub username who requested the reminder |
| `requester_id` | BIGINT | NOT NULL | GitHub user ID |
| `remind_at` | TIMESTAMP | NOT NULL, INDEX | When to fire the reminder |
| `original_command` | VARCHAR(512) | NOT NULL | Original "remora <time>" command text |
| `status` | VARCHAR(50) | NOT NULL, INDEX | Reminder status (see enum below) |
| `fired_at` | TIMESTAMP | NULL | When the reminder was actually fired |
| `error_message` | TEXT | NULL | Error details if reminder failed |
| `retry_count` | INTEGER | NOT NULL, DEFAULT 0 | Number of retry attempts |

### Status Enum Values

- `pending` - Reminder is waiting to be fired
- `processing` - Reminder is currently being processed (prevents duplicate processing)
- `fired` - Reminder successfully delivered
- `failed` - Reminder failed after retries
- `cancelled` - Reminder was cancelled (future feature)

## GORM Model Definition

```go
type Reminder struct {
    ID               uint      `gorm:"primaryKey"`
    CreatedAt        time.Time
    UpdatedAt        time.Time
    DeletedAt        gorm.DeletedAt `gorm:"index"`
    
    RepositoryOwner  string    `gorm:"size:255;not null;index:idx_repo"`
    RepositoryName   string    `gorm:"size:255;not null;index:idx_repo"`
    IssueNumber      int       `gorm:"not null;index:idx_repo"`
    CommentID        int64     `gorm:"not null"`
    CommentURL       string    `gorm:"size:512;not null"`
    
    RequesterUsername string   `gorm:"size:255;not null"`
    RequesterID       int64    `gorm:"not null"`
    
    RemindAt         time.Time `gorm:"not null;index:idx_remind_at"`
    OriginalCommand  string    `gorm:"size:512;not null"`
    
    Status           string    `gorm:"size:50;not null;index:idx_status"`
    FiredAt          *time.Time
    ErrorMessage     string    `gorm:"type:text"`
    RetryCount       int       `gorm:"not null;default:0"`
}
```

## Indexes

### Composite Indexes

1. **`idx_repo`**: (`repository_owner`, `repository_name`, `issue_number`)
   - **Purpose**: Efficiently query reminders for a specific issue/PR
   - **Use Case**: Listing all reminders for an issue, checking for duplicates

2. **`idx_remind_at`**: (`remind_at`)
   - **Purpose**: Scheduler queries for due reminders
   - **Use Case**: `WHERE remind_at <= NOW() AND status = 'pending'`

3. **`idx_status`**: (`status`)
   - **Purpose**: Filter by reminder status
   - **Use Case**: Dashboard, statistics, cleanup jobs

4. **`idx_scheduler_query`**: (`status`, `remind_at`)
   - **Purpose**: Optimized for scheduler's main query
   - **Use Case**: Finding pending reminders that are due

### Soft Delete Index

- **`idx_deleted_at`**: Automatically created by GORM for soft deletes
- **Purpose**: Filter out deleted records efficiently

## Query Patterns

### Scheduler Query (Most Critical)

```sql
SELECT * FROM reminders 
WHERE status = 'pending' 
  AND remind_at <= NOW()
  AND deleted_at IS NULL
ORDER BY remind_at ASC
LIMIT 100;
```

### Create Reminder

```sql
INSERT INTO reminders (
    repository_owner, repository_name, issue_number,
    comment_id, comment_url,
    requester_username, requester_id,
    remind_at, original_command,
    status, created_at, updated_at
) VALUES (...);
```

### Update Status After Firing

```sql
UPDATE reminders 
SET status = 'fired', 
    fired_at = NOW(),
    updated_at = NOW()
WHERE id = ?;
```

### Find Reminders for Issue

```sql
SELECT * FROM reminders
WHERE repository_owner = ?
  AND repository_name = ?
  AND issue_number = ?
  AND deleted_at IS NULL
ORDER BY remind_at DESC;
```

---

## Transaction Strategy

### Operations Requiring Transactions

#### 1. Scheduler: Query and Lock Reminders (Critical)

**Purpose**: Prevent duplicate processing even within single instance (protects against accidental multi-instance deployment)

**PostgreSQL**:
```sql
BEGIN;

UPDATE reminders 
SET status = 'processing', 
    updated_at = NOW()
WHERE id IN (
  SELECT id FROM reminders 
  WHERE status = 'pending' 
    AND remind_at <= NOW()
    AND deleted_at IS NULL
  ORDER BY remind_at ASC
  LIMIT 100
  FOR UPDATE SKIP LOCKED
)
RETURNING *;

COMMIT;
```

**MySQL**:
```sql
START TRANSACTION;

SELECT * FROM reminders
WHERE status = 'pending'
  AND remind_at <= NOW()
  AND deleted_at IS NULL
ORDER BY remind_at ASC
LIMIT 100
FOR UPDATE SKIP LOCKED;

-- Then update in application code
UPDATE reminders 
SET status = 'processing', updated_at = NOW()
WHERE id IN (...);

COMMIT;
```

**SQLite** (Development only):
```sql
-- SQLite doesn't support SKIP LOCKED
-- Use immediate transaction with simple update
BEGIN IMMEDIATE;

UPDATE reminders 
SET status = 'processing'
WHERE id IN (
  SELECT id FROM reminders 
  WHERE status = 'pending' 
    AND remind_at <= NOW()
  ORDER BY remind_at ASC
  LIMIT 100
);

SELECT * FROM reminders WHERE status = 'processing';

COMMIT;
```

**GORM Implementation**:
```go
func (r *ReminderRepository) GetAndLockDueReminders(limit int) ([]Reminder, error) {
    var reminders []Reminder
    
    err := r.db.Transaction(func(tx *gorm.DB) error {
        // For PostgreSQL/MySQL: use raw SQL with FOR UPDATE SKIP LOCKED
        if r.db.Dialector.Name() == "postgres" || r.db.Dialector.Name() == "mysql" {
            subQuery := tx.Model(&Reminder{}).
                Where("status = ? AND remind_at <= ?", "pending", time.Now()).
                Order("remind_at ASC").
                Limit(limit).
                Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
            
            return tx.Model(&Reminder{}).
                Where("id IN (?)", subQuery.Select("id")).
                Update("status", "processing").
                Update("updated_at", time.Now()).
                Find(&reminders).Error
        }
        
        // For SQLite: simple update without SKIP LOCKED
        return tx.Model(&Reminder{}).
            Where("status = ? AND remind_at <= ?", "pending", time.Now()).
            Order("remind_at ASC").
            Limit(limit).
            Update("status", "processing").
            Update("updated_at", time.Now()).
            Find(&reminders).Error
    })
    
    return reminders, err
}
```

**Why Needed**:
- Prevents race conditions within single instance (concurrent goroutines)
- Safety net against accidental multi-instance deployment
- `SKIP LOCKED` ensures non-blocking: if row is locked, skip it
- Atomic operation: query and update in single transaction

---

#### 2. Webhook Handler: Reminder Creation (No Transaction Needed)

**Current Approach**:
```go
// 1. Insert reminder (GORM handles transaction internally)
reminder := &Reminder{...}
db.Create(reminder)

// 2. Add GitHub reaction (separate operation, can fail independently)
githubClient.AddReaction(commentID, "eyes")
```

**Rationale**:
- INSERT is atomic on its own
- GitHub reaction failure is tolerable (reminder still created)
- If reaction fails, user can see reminder in database via admin API
- Wrapping both in transaction adds complexity without significant benefit

---

#### 3. Reminder Cancellation: Comment Deletion (No Transaction Needed)

**Operation**:
```sql
UPDATE reminders 
SET status = 'cancelled', 
    updated_at = NOW()
WHERE comment_id = ?;
```

**Rationale**:
- Single UPDATE operation
- GORM handles atomic update
- No multi-step operation requiring transaction

---

#### 4. Status Update After Firing (No Transaction Needed)

**Operation**:
```sql
UPDATE reminders 
SET status = 'fired',
    fired_at = NOW(),
    updated_at = NOW()
WHERE id = ?;
```

**Rationale**:
- Single UPDATE operation
- Already marked as 'processing' (locked by scheduler)
- No concurrent modification possible

---

#### 5. Retry Logic: Failed → Pending (Conditional Transaction)

**Operation**:
```go
db.Model(&reminder).
    Where("id = ? AND retry_count < ?", id, maxRetries).
    Updates(map[string]interface{}{
        "status": "pending",
        "updated_at": time.Now(),
    })
```

**Rationale**:
- Single UPDATE with WHERE condition
- GORM provides atomic update
- Condition prevents race: only updates if retry_count still valid

---

#### 6. Batch Cleanup: Old Reminders (Optional Transaction)

**Operation**:
```sql
-- Hard delete old fired/failed reminders
DELETE FROM reminders 
WHERE status IN ('fired', 'failed') 
  AND (fired_at < NOW() - INTERVAL '365 days'
       OR updated_at < NOW() - INTERVAL '365 days');
```

**Transaction Usage**:
```go
// Optional: wrap in transaction for safety
db.Transaction(func(tx *gorm.DB) error {
    result := tx.Unscoped(). // Hard delete
        Where("status IN ?", []string{"fired", "failed"}).
        Where("fired_at < ? OR updated_at < ?", 
            time.Now().AddDate(-1, 0, 0),
            time.Now().AddDate(-1, 0, 0)).
        Delete(&Reminder{})
    
    log.Info("Cleaned up old reminders", "count", result.RowsAffected)
    return nil
})
```

**Rationale**:
- Single DELETE but affects many rows
- Transaction provides rollback safety if error occurs mid-delete
- Not critical (can run again if fails)

---

### Transaction Isolation Levels

**Default**: Use database defaults
- PostgreSQL: `READ COMMITTED`
- MySQL: `REPEATABLE READ`
- SQLite: `SERIALIZABLE`

**Custom Isolation** (if needed):
```go
db.Transaction(func(tx *gorm.DB) error {
    tx.Exec("SET TRANSACTION ISOLATION LEVEL READ COMMITTED")
    // ... operations
}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
```

**Recommendation**: Stick with defaults unless specific issue arises

---

### Concurrency Strategy

**Scheduler Processing**:
- Single goroutine processes reminders sequentially
- Query locks up to 100 reminders
- Process each one-by-one
- No concurrent processing within scheduler cycle

**Rationale**:
- Simple and predictable
- `FOR UPDATE SKIP LOCKED` prevents external conflicts
- Sequential processing avoids complexity
- Sufficient for expected load (100 reminders per 5-minute cycle)

**Future Enhancement**:
- If processing >100 reminders per cycle becomes bottleneck
- Consider worker pool (5-10 goroutines)
- Each worker locks individual reminder before processing

---

### Connection Pool Settings

```go
sqlDB, _ := db.DB()

// Recommended settings
sqlDB.SetMaxIdleConns(10)           // Keep 10 idle connections
sqlDB.SetMaxOpenConns(100)          // Max 100 open connections
sqlDB.SetConnMaxLifetime(time.Hour) // Recycle connections hourly
sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Close idle after 10min
```

**Rationale**:
- Webhook handler: burst traffic, needs connection pool
- Scheduler: single goroutine, low connection usage
- 100 max connections is generous for expected load

---

## Database-Specific Considerations

### PostgreSQL
- Use `TIMESTAMP WITH TIME ZONE` for time columns
- Native UUID support if needed in future
- JSON column support for metadata (future enhancement)
- Excellent performance for indexed queries

### MySQL
- Use `DATETIME` or `TIMESTAMP` columns
- Ensure UTF8MB4 charset for full Unicode support
- InnoDB engine for transactions and foreign keys

### SQLite
- Use `DATETIME` stored as TEXT (ISO8601)
- Limited concurrent write performance
- Perfect for development and testing
- Not recommended for high-traffic production

## Migrations

### Initial Migration (Auto-Migration)

GORM's `AutoMigrate` will create the table on first run:

```go
db.AutoMigrate(&Reminder{})
```

### Future Schema Changes

- Add migration versioning (future enhancement)
- Consider using `gorm-migrate` or similar tool
- Keep backward compatibility in mind
- Test migrations against all three database types

## Data Retention

### Strategy

**Single Table Approach**: Keep all reminders in the `reminders` table with periodic cleanup. This approach is simpler and performs well at expected scale.

### Retention Policy

- **Fired Reminders**: Retain for 1 year for audit trail and analytics
- **Failed Reminders**: Retain for 1 year for debugging and analysis
- **Pending Reminders**: No expiration (user-controlled)

### Rationale

1. **Scale is Manageable**: Even at 1000 reminders/day, that's ~365k rows/year - trivial for modern databases
2. **Index Efficiency**: Composite index `(status, remind_at)` ensures scheduler only scans pending rows
3. **Simplicity**: No background jobs to move data between tables
4. **Analytics**: Easy to build dashboards and statistics over time
5. **Soft Deletes**: GORM handles filtering deleted records automatically

### Automated Cleanup

Implement a background job (cron-like) that runs daily to purge old records:

```go
// Delete reminders older than 1 year (hard delete)
db.Unscoped().
    Where("fired_at < ?", time.Now().AddDate(-1, 0, 0)).
    Where("status IN ?", []string{"fired", "failed"}).
    Delete(&Reminder{})
```

**Configuration**: Cleanup retention period should be configurable via environment variable:

```
REMORA_RETENTION_DAYS=365  # Default: 1 year
```

### Alternative Considered

**Separate History Table**: Move completed reminders to `reminder_history` table.
- **Rejected**: Adds complexity without significant benefit at our scale
- **Future Option**: If table grows beyond millions of rows, consider this approach

## Performance Considerations

### Expected Load

- **Write Volume**: Low (based on comment frequency)
- **Read Volume**: Moderate (scheduler polls every 1-5 minutes)
- **Data Size**: Small (typical: thousands to tens of thousands of records)

### Optimization Strategy

- Primary key on `id` for fast lookups
- Composite index on `(status, remind_at)` for scheduler
- Soft deletes prevent hard delete costs
- Pagination for large result sets (future)

### Connection Pooling

```go
// Recommended GORM connection pool settings
sqlDB, _ := db.DB()
sqlDB.SetMaxIdleConns(10)
sqlDB.SetMaxOpenConns(100)
sqlDB.SetConnMaxLifetime(time.Hour)
```

---

## Debugging Queries

### Find Stuck Reminders (Processing Too Long)

```sql
-- Reminders in 'processing' state for more than 1 hour
SELECT id, repository_owner, repository_name, issue_number,
       requester_username, remind_at, status, updated_at,
       EXTRACT(EPOCH FROM (NOW() - updated_at))/3600 as hours_stuck
FROM reminders
WHERE status = 'processing'
  AND updated_at < NOW() - INTERVAL '1 hour'
  AND deleted_at IS NULL
ORDER BY updated_at ASC;
```

### Find Failed Reminders with Errors

```sql
-- All failed reminders with error messages
SELECT id, repository_owner, repository_name, issue_number,
       requester_username, remind_at, retry_count, error_message,
       updated_at
FROM reminders
WHERE status = 'failed'
  AND deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT 50;
```

### Find Overdue Pending Reminders

```sql
-- Reminders that should have fired but are still pending
SELECT id, repository_owner, repository_name, issue_number,
       requester_username, remind_at, 
       EXTRACT(EPOCH FROM (NOW() - remind_at))/3600 as hours_overdue
FROM reminders
WHERE status = 'pending'
  AND remind_at < NOW()
  AND deleted_at IS NULL
ORDER BY remind_at ASC
LIMIT 100;
```

### Find Reminders for Specific User

```sql
-- All reminders created by a specific user
SELECT id, repository_owner, repository_name, issue_number,
       remind_at, status, created_at, fired_at
FROM reminders
WHERE requester_username = 'username'
  AND deleted_at IS NULL
ORDER BY created_at DESC;
```

### Find Reminders for Specific Repository

```sql
-- All reminders in a specific repository
SELECT id, issue_number, requester_username, remind_at, 
       status, created_at, original_command
FROM reminders
WHERE repository_owner = 'owner'
  AND repository_name = 'repo'
  AND deleted_at IS NULL
ORDER BY created_at DESC;
```

### Reminder Status Summary

```sql
-- Count of reminders by status
SELECT status, COUNT(*) as count
FROM reminders
WHERE deleted_at IS NULL
GROUP BY status
ORDER BY count DESC;
```

### Recent Reminder Activity

```sql
-- Reminders created, fired, or failed in last 24 hours
SELECT 
  DATE_TRUNC('hour', created_at) as hour,
  status,
  COUNT(*) as count
FROM reminders
WHERE created_at > NOW() - INTERVAL '24 hours'
  AND deleted_at IS NULL
GROUP BY DATE_TRUNC('hour', created_at), status
ORDER BY hour DESC, status;
```

### Find High Retry Reminders

```sql
-- Reminders that have been retried multiple times
SELECT id, repository_owner, repository_name, issue_number,
       requester_username, retry_count, error_message, status
FROM reminders
WHERE retry_count >= 3
  AND deleted_at IS NULL
ORDER BY retry_count DESC, updated_at DESC
LIMIT 50;
```

### Average Time to Fire

```sql
-- Average time between reminder creation and firing
SELECT 
  AVG(EXTRACT(EPOCH FROM (fired_at - created_at))) as avg_seconds,
  MIN(EXTRACT(EPOCH FROM (fired_at - created_at))) as min_seconds,
  MAX(EXTRACT(EPOCH FROM (fired_at - created_at))) as max_seconds
FROM reminders
WHERE status = 'fired'
  AND fired_at IS NOT NULL
  AND created_at > NOW() - INTERVAL '7 days';
```

### Find Reminders by Issue

```sql
-- All reminders for a specific issue (including cancelled/deleted)
SELECT id, requester_username, remind_at, status, 
       created_at, fired_at, deleted_at
FROM reminders
WHERE repository_owner = 'owner'
  AND repository_name = 'repo'
  AND issue_number = 123
ORDER BY created_at DESC;
```

### Check Index Usage (PostgreSQL)

```sql
-- Verify indexes are being used
EXPLAIN ANALYZE
SELECT * FROM reminders
WHERE status = 'pending' 
  AND remind_at <= NOW()
  AND deleted_at IS NULL
ORDER BY remind_at ASC
LIMIT 100;

-- Should show "Index Scan using idx_scheduler_query"
```

### Database Size Statistics

```sql
-- PostgreSQL: Table size and row count
SELECT 
  pg_size_pretty(pg_total_relation_size('reminders')) as total_size,
  COUNT(*) as total_rows,
  COUNT(*) FILTER (WHERE deleted_at IS NULL) as active_rows,
  COUNT(*) FILTER (WHERE status = 'pending') as pending_rows
FROM reminders;
```

---

## Testing Strategy

### Unit Tests

- Test GORM model validation
- Test custom methods on Reminder struct
- Mock database for repository tests

### Integration Tests

- Test actual database operations
- Use test containers for PostgreSQL/MySQL
- Use in-memory SQLite for fast tests
- Verify indexes are created correctly
- Test concurrent access patterns

## Future Enhancements

Potential schema additions:

1. **User Preferences Table**: Store per-user settings
2. **Reminder History Table**: Detailed audit log
3. **Recurring Reminders**: Add fields for recurrence patterns
4. **Reminder Groups**: Group related reminders
5. **Metadata Column**: JSON field for extensibility
