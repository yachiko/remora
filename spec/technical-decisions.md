# Technical Decisions

This document captures the key technical decisions made for the Remora project and their rationale.

## Programming Language

**Decision**: Go (Golang)

**Rationale**:
- Excellent for building robust, concurrent server applications
- Strong standard library for HTTP servers and webhooks
- Easy Docker containerization with static binaries
- Great tooling for testing and dependency management
- Similar to Atlantis, providing a proven pattern

## GitHub Integration Model

**Decision**: GitHub App with webhook endpoint

**Rationale**:
- Provides proper authentication and authorization model
- Fine-grained permissions (only needs comment and reaction access)
- Scales to multiple repositories and organizations
- Real-time event processing via webhooks
- Industry standard approach (same as Atlantis, Dependabot, etc.)
- Better than personal access tokens for security and auditing

## Database Options

**Decision**: Support PostgreSQL, MySQL, and SQLite via GORM

**Rationale**:
- PostgreSQL/MySQL for production deployments
- SQLite for local development and testing
- GORM provides unified interface across all three
- Auto-migration support simplifies schema management
- Active community and mature ecosystem

## ORM Choice

**Decision**: GORM

**Rationale**:
- Mature and widely used in Go ecosystem
- Supports all required databases (PostgreSQL, MySQL, SQLite)
- Auto-migration capabilities
- Good balance between abstraction and control
- Strong community support and documentation
- Reduces boilerplate for multi-database support

## Natural Language Date Parsing

**Decision**: `olebedev/when` library

**Rationale**:
- Mature library with good track record
- Handles wide variety of natural language patterns
- Supports relative times ("in 2 days") and absolute dates ("December 25th")
- Timezone aware through base time parameter
- Better than building custom parser from scratch
- Active maintenance and community

## Timezone Handling

**Decision**: UTC default with optional timezone suffix support

**Implementation Strategy**:

### Phase 1: UTC Default
- All parsing uses UTC as base timezone (`time.Now().UTC()`)
- All database timestamps stored in UTC
- Scheduler operates in UTC
- Simple and predictable behavior

### Phase 2: Explicit Timezone Support
- Allow users to specify timezone in command
- Examples:
  - `remora 2 days PST`
  - `remora tomorrow 3pm EST`
  - `remora 25th December at 2pm America/New_York`
- Leverage `olebedev/when` library's base time parameter with specified timezone
- Fall back to UTC if timezone parsing fails

**Rationale**:
- GitHub API does not expose user timezone preferences
- User `location` field is unreliable (free text, often blank or joking)
- UTC default is honest and predictable
- Relative times ("in 2 days") work well in UTC
- Explicit timezone gives power users control when needed
- Both phases implemented in first version for completeness

**Technical Details**:
- Parse timezone abbreviations/names before passing to `when.Parse()`
- Use Go's `time.LoadLocation()` for timezone resolution
- Base time passed to parser determines output timezone
- Always convert to UTC before storing in database

```go
// Example implementation
func parseReminder(command string) (time.Time, error) {
    tz := extractTimezone(command) // Extract PST, EST, America/New_York, etc.
    location := time.UTC
    if tz != "" {
        location, _ = time.LoadLocation(tz)
    }
    baseTime := time.Now().In(location)
    result, _ := parser.Parse(command, baseTime)
    return result.Time.UTC(), nil // Always store in UTC
}
```

## Scheduler Design

**Decision**: In-process scheduler with periodic database polling

**Rationale**:
- Simple to implement and operate
- No external dependencies (Redis, RabbitMQ, etc.)
- Self-contained in single container
- Sufficient for expected scale (GitHub comment reminders)
- Easy to reason about for debugging
- Database is single source of truth

**Trade-offs**:
- Single instance limitation (horizontal scaling requires coordination)
- Polling adds latency (acceptable for reminder use case)
- Future enhancement: distributed locking for multi-instance

## Configuration Management

**Decision**: Environment variables only (12-factor app approach)

**Rationale**:
- Container-friendly and cloud-native
- Works seamlessly with Docker, Kubernetes, and cloud platforms
- No config file synchronization issues
- Secrets managed via environment (integrated with secret managers)
- Simple and explicit
- Easy to override for different environments

## Logging

**Decision**: Zap structured logging library

**Rationale**:
- High performance, structured logging
- JSON output for production (machine-parseable)
- Configurable log levels
- Rich context support
- Industry standard for Go applications
- Better than standard library for production apps

## Testing Approach

**Decision**: Test-Driven Development (TDD) with unit and integration tests

**Rationale**:
- TDD ensures testable, well-designed code
- Unit tests for individual component logic
- Integration tests for database and webhook flows
- High confidence in code correctness
- Easier refactoring with test safety net
- CI/CD friendly

**Testing Stack**:
- Go standard testing package
- Testify for assertions and mocking
- Test containers for integration tests (real databases)

## Command Format

**Decision**: `remora <time expression>` prefix format

**Rationale**:
- Simple and unambiguous
- Doesn't conflict with GitHub's @ mentions or slash commands
- Easy to parse (prefix-based)
- Natural to type
- Examples: "remora 2 days", "remora December 25th"

## User Feedback Mechanism

**Decision**: Reactions for acknowledgment, optional comments for errors

**Rationale**:
- Reactions (👀/✅) provide immediate, non-intrusive feedback
- Minimal noise in issue/PR threads
- Error handling configurable: reactions only or reactions + explanatory comments
- Allows teams to choose verbosity level

**Reactions**:
- 👀 (eyes) or ✅ (checkmark): Reminder accepted
- ❌ (cross): Error (invalid format, past date, etc.)
- ⚠️ (warning): Potential issue but reminder still set

## Reminder Delivery Format

**Decision**: Simple ping with link to original comment

**Format**: `@username reminder from [link to original comment]`

**Rationale**:
- Minimal and focused
- User gets notification via GitHub mention
- Link provides context without cluttering comment
- Easy to understand at a glance
- Matches Reddit RemindMeBot behavior pattern

## Project Structure

**Decision**: Standard Go project layout

**Rationale**:
- Industry best practice for Go projects
- Clear separation of concerns (`/cmd`, `/internal`, `/pkg`)
- Easy for other Go developers to navigate
- Scales well as project grows
- Supports multiple commands if needed in future

## Deployment Model

**Decision**: Single Docker container

**Rationale**:
- Simple to deploy and manage
- Works in any container orchestration platform
- Self-contained with all dependencies
- Easy to version and rollback
- Minimal operational overhead

## Error Handling Configuration

**Decision**: Configurable error feedback (reactions-only vs reactions+comments)

**Rationale**:
- Different teams have different preferences
- Some want minimal noise (reactions only)
- Others want clear explanations (comments)
- Configuration via environment variable
- Flexibility without code changes

## Security Model

**Decision**: GitHub App authentication with webhook signature validation

**Rationale**:
- Webhook HMAC-SHA256 signature validation prevents spoofing
- GitHub App private key for API authentication
- No personal access tokens (better security model)
- Least-privilege permissions
- Secrets via environment variables (integration with secret managers)

## Dependencies Summary

Core dependencies:
- **Web Framework**: Standard library `net/http` (sufficient for webhooks)
- **Database ORM**: `gorm.io/gorm`
- **Date Parsing**: `github.com/olebedev/when`
- **Logging**: `go.uber.org/zap`
- **GitHub API**: `github.com/google/go-github/v57/github`
- **Testing**: `github.com/stretchr/testify`

## Future Considerations

Decisions deferred for later:
- Horizontal scaling with distributed locks
- Metrics and observability (Prometheus, etc.)
- Admin API for managing reminders
- User preferences and customization
- Advanced scheduling (recurring reminders)
