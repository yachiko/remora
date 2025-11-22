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
