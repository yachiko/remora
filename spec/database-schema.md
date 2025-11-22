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
