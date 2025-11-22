# Remora Project Structure

## Directory Layout

Following the standard Go project layout conventions:

```
remora/
├── cmd/
│   └── remora/
│       └── main.go                 # Application entry point
├── internal/
│   ├── config/
│   │   ├── config.go              # Configuration loading from env vars
│   │   └── config_test.go
│   ├── webhook/
│   │   ├── handler.go             # GitHub webhook handler
│   │   ├── validator.go           # Webhook signature validation
│   │   └── handler_test.go
│   ├── parser/
│   │   ├── parser.go              # Command parsing logic
│   │   ├── parser_test.go
│   │   └── time_parser.go         # Natural language time parsing wrapper
│   ├── models/
│   │   ├── reminder.go            # Reminder data model
│   │   └── models_test.go
│   ├── database/
│   │   ├── db.go                  # Database initialization
│   │   ├── migrations.go          # GORM auto-migrations
│   │   ├── repository.go          # Reminder repository interface
│   │   └── repository_test.go
│   ├── scheduler/
│   │   ├── scheduler.go           # In-process reminder scheduler
│   │   ├── scheduler_test.go
│   │   └── executor.go            # Reminder execution logic
│   ├── github/
│   │   ├── client.go              # GitHub API client wrapper
│   │   ├── reactions.go           # Reaction posting logic
│   │   ├── comments.go            # Comment posting logic
│   │   └── client_test.go
│   └── logger/
│       └── logger.go              # Zap logger initialization
├── pkg/                            # (Reserved for public libraries if needed)
├── spec/
│   ├── architecture.md
│   ├── project-structure.md
│   └── technical-decisions.md
├── test/
│   ├── integration/
│   │   ├── webhook_test.go        # End-to-end webhook tests
│   │   └── scheduler_test.go      # Integration tests with test DB
│   └── fixtures/
│       └── github_payloads.json   # Sample GitHub webhook payloads
├── deployments/
│   ├── docker/
│   │   └── Dockerfile
│   └── kubernetes/                 # (Optional: K8s manifests)
│       └── deployment.yaml
├── scripts/
│   ├── setup-dev.sh               # Local development setup
│   └── run-tests.sh               # Test execution script
├── .github/
│   └── workflows/
│       ├── ci.yml                 # CI/CD pipeline
│       └── release.yml
├── go.mod
├── go.sum
├── .gitignore
├── .env.example                   # Example environment variables
├── Makefile                       # Common tasks (build, test, run)
└── README.md                      # Project documentation
```

## Package Responsibilities

### `/cmd/remora`
- Application entry point
- Initializes all components
- Starts HTTP server and scheduler
- Handles graceful shutdown

### `/internal/config`
- Loads configuration from environment variables
- Validates required settings
- Provides config struct to other packages

### `/internal/webhook`
- HTTP handlers for GitHub webhooks
- Webhook signature validation
- Event routing and filtering

### `/internal/parser`
- Parses "remora <time>" commands from comments
- Wraps `olebedev/when` library
- Validates parsed times (future dates only)

### `/internal/models`
- GORM data models
- Reminder struct definition
- Database schema definition

### `/internal/database`
- Database connection management
- Repository pattern for data access
- GORM migrations
- Abstraction over PostgreSQL/MySQL/SQLite

### `/internal/scheduler`
- In-process ticker-based scheduler
- Polls database for due reminders
- Executes reminders via GitHub client
- Handles concurrency and state management

### `/internal/github`
- GitHub API client
- Authentication (GitHub App)
- Posting reactions and comments
- Rate limiting and retry logic

### `/internal/logger`
- Structured logging with Zap
- Log level configuration
- Context-aware logging utilities

### `/test`
- Integration tests with real database
- End-to-end webhook flow tests
- Test fixtures and helpers

## Build Artifacts

### Binary
- Output: `bin/remora`
- Platform: Linux (for Docker)
- Static linking for minimal container

### Docker Image
- Base: Alpine Linux (minimal size)
- Includes only binary and CA certificates
- Configurable via environment variables
- Non-root user for security

## Development Workflow

1. **Setup**: Run `make setup` to install dependencies
2. **Build**: Run `make build` to compile binary
3. **Test**: Run `make test` for unit tests, `make test-integration` for integration
4. **Run**: Run `make run` or `go run cmd/remora/main.go`
5. **Docker**: Run `make docker-build` and `make docker-run`

## Testing Strategy

- **Unit Tests**: Alongside each package (`*_test.go`)
- **Integration Tests**: In `/test/integration` with test database
- **Mocking**: Use interfaces for external dependencies (GitHub API, database)
- **Coverage Target**: Aim for >80% coverage
- **CI/CD**: Run all tests on every commit
