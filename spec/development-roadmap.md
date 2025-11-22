# Development Roadmap

## Overview

This document outlines the phased implementation plan for Remora, from initial setup through production deployment and future enhancements.

---

## Phase 0: Project Setup

**Goal**: Establish development environment and project structure

### Tasks

1. **Initialize Go Module**
   - Create `go.mod` with Go 1.21+
   - Set module path: `github.com/owner/remora`

2. **Setup Directory Structure**
   - Follow standard Go project layout (see `project-structure.md`)
   - Create `/cmd/remora`, `/internal/*`, `/test`, `/deployments`, `/spec`

3. **Development Tools**
   - Setup `.gitignore` for Go projects
   - Create `.env.example` with all environment variables
   - Create `Makefile` for common tasks (build, test, run, docker)

4. **CI/CD Pipeline**
   - GitHub Actions workflow for linting (golangci-lint)
   - GitHub Actions workflow for tests
   - GitHub Actions workflow for building Docker images

5. **Documentation**
   - Create comprehensive `README.md`
   - Document environment variables
   - Add contribution guidelines

**Deliverables**:
- ✅ Runnable project scaffold
- ✅ CI/CD pipeline configured
- ✅ Development documentation

**Estimated Time**: 1-2 days

---

## Phase 1: Core Infrastructure

**Goal**: Build foundational components (config, logging, database)

### Tasks

1. **Configuration Management** (`internal/config`)
   - Load environment variables
   - Validate required settings
   - Provide typed config struct
   - **Tests**: Unit tests for validation logic

2. **Logging Setup** (`internal/logger`)
   - Initialize Zap logger
   - Configure log levels (dev vs production)
   - Structured logging utilities
   - **Tests**: Verify log output formats

3. **Database Layer** (`internal/database`)
   - GORM setup with PostgreSQL/MySQL/SQLite drivers
   - Connection pooling configuration
   - Auto-migration on startup
   - Health check (ping)
   - **Tests**: Integration tests with test containers (PostgreSQL, MySQL, SQLite)

4. **Models** (`internal/models`)
   - Define `Reminder` struct with GORM tags
   - Add validation tags
   - **Tests**: Model validation tests

5. **Repository Pattern** (`internal/database`)
   - Create `ReminderRepository` interface
   - Implement CRUD operations
   - Query methods (find due reminders, find by status, etc.)
   - **Tests**: Repository integration tests

**Deliverables**:
- ✅ Config loading from environment
- ✅ Structured logging
- ✅ Database connection with migrations
- ✅ Repository pattern for reminders
- ✅ >80% test coverage

**Estimated Time**: 3-4 days

---

## Phase 2: Time Parsing & Validation

**Goal**: Implement natural language time parsing with validation

### Tasks

1. **Time Parser Wrapper** (`internal/parser`)
   - Integrate `olebedev/when` library
   - Support UTC and explicit timezone parsing
   - Handle timezone extraction from command
   - **Tests**: Unit tests with various time expressions

2. **Command Parser** (`internal/parser`)
   - Detect "remora" prefix (case-insensitive)
   - Extract time expression
   - Parse first match only
   - **Tests**: Test various command formats

3. **Validation Logic** (`internal/parser`)
   - Ensure parsed time is in future
   - Validate minimum (15 minutes)
   - Validate maximum (395 days / 13 months)
   - Reject past dates
   - **Tests**: Comprehensive validation tests

4. **Error Messages**
   - Define clear error messages for each failure type
   - Past date error
   - Too soon error (< 15 min)
   - Too far error (> 13 months)
   - Unparseable error
   - **Tests**: Verify error message content

**Deliverables**:
- ✅ Natural language parsing with `olebedev/when`
- ✅ Timezone support (UTC + explicit timezones)
- ✅ All validation rules implemented
- ✅ Clear, actionable error messages
- ✅ Extensive test coverage (50+ test cases)

**Estimated Time**: 3-4 days

---

## Phase 3: GitHub Integration

**Goal**: Implement GitHub App authentication and API interactions

### Tasks

1. **GitHub Client** (`internal/github`)
   - GitHub App JWT generation
   - Installation token management (caching)
   - Token refresh logic
   - **Tests**: Unit tests with mocked GitHub API

2. **API Operations** (`internal/github`)
   - Add reaction to comment (`POST /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions`)
   - Post comment (`POST /repos/{owner}/{repo}/issues/{issue_number}/comments`)
   - Error handling (403, 404, 429, 5xx)
   - **Tests**: Integration tests against GitHub test repository

3. **Rate Limiting** (`internal/github`)
   - Track rate limit headers
   - Log warnings when approaching limits
   - Let natural failures occur (no artificial limiting)
   - **Tests**: Simulate rate limit responses

4. **Retry Logic** (`internal/github`)
   - Exponential backoff for retries (1, 2, 4, 8, 16 min)
   - Max 5 retry attempts
   - Log each retry attempt
   - **Tests**: Verify retry behavior with failures

**Deliverables**:
- ✅ GitHub App authentication
- ✅ Comment and reaction posting
- ✅ Error handling and retries
- ✅ Rate limit awareness
- ✅ Integration tests

**Estimated Time**: 3-4 days

---

## Phase 4: Webhook Handler

**Goal**: Receive and process GitHub webhook events

### Tasks

1. **Webhook Signature Validation** (`internal/webhook`)
   - HMAC-SHA256 signature verification
   - Reject invalid signatures
   - **Tests**: Valid and invalid signature tests

2. **Event Filtering** (`internal/webhook`)
   - Filter for `issue_comment.created` events
   - Filter for `issue_comment.deleted` events
   - Ignore other event types
   - **Tests**: Test with various event types

3. **Comment Processing** (`internal/webhook`)
   - Extract comment body, user, issue, repository
   - Check for "remora" command
   - Parse time expression
   - Validate parsed time
   - **Tests**: End-to-end webhook processing tests

4. **Reminder Creation** (`internal/webhook`)
   - Create reminder in database
   - Add success reaction (✅ or 👀)
   - Handle duplicate commands (allow all)
   - **Tests**: Database integration tests

5. **Reminder Cancellation** (`internal/webhook`)
   - Handle `issue_comment.deleted` events
   - Find reminder by comment_id
   - Mark as "cancelled"
   - **Tests**: Deletion flow tests

6. **Error Handling** (`internal/webhook`)
   - Add ❌ reaction on parse errors
   - Post error comment with explanation
   - Log all errors
   - **Tests**: Error scenario tests

**Deliverables**:
- ✅ Webhook endpoint with signature validation
- ✅ Comment parsing and reminder creation
- ✅ Deletion handling
- ✅ Error reactions and comments
- ✅ >85% test coverage

**Estimated Time**: 4-5 days

---

## Phase 5: Scheduler

**Goal**: Background process to fire reminders on time

### Tasks

1. **Scheduler Core** (`internal/scheduler`)
   - Ticker-based polling (every 5 minutes, configurable)
   - Query database for due reminders
   - Process reminders sequentially (no batching)
   - **Tests**: Unit tests with mocked time

2. **Reminder Execution** (`internal/scheduler`)
   - Update status to "processing"
   - Post reminder comment via GitHub API
   - Update status to "fired" on success
   - Update status to "failed" on error
   - Track retry attempts
   - **Tests**: Execution logic tests

3. **Overdue Reminder Handling** (`internal/scheduler`)
   - On startup, query overdue reminders
   - Skip reminders > 24 hours overdue (mark as "expired")
   - Fire recent overdue reminders
   - Annotate comment with delay (e.g., "2 hours late")
   - **Tests**: Overdue scenarios

4. **Error Recovery** (`internal/scheduler`)
   - Retry failed reminders (max 5 attempts)
   - Exponential backoff between retries
   - Store error message in database
   - **Tests**: Retry logic tests

5. **Graceful Shutdown** (`internal/scheduler`)
   - Handle SIGTERM/SIGINT signals
   - Wait for current reminder to complete
   - Clean shutdown
   - **Tests**: Shutdown behavior tests

**Deliverables**:
- ✅ Reliable scheduler with periodic polling
- ✅ Reminder firing logic
- ✅ Overdue handling with annotation
- ✅ Retry with exponential backoff
- ✅ Graceful shutdown
- ✅ >80% test coverage

**Estimated Time**: 4-5 days

---

## Phase 6: HTTP Server & Health Checks

**Goal**: HTTP server for webhooks and monitoring

### Tasks

1. **HTTP Server Setup** (`cmd/remora`)
   - Initialize HTTP server (standard library)
   - Configure port from environment
   - Graceful shutdown on signals
   - **Tests**: Server startup/shutdown tests

2. **Webhook Endpoint** (`cmd/remora`)
   - Route `POST /webhook` to webhook handler
   - Request timeout (10 seconds)
   - Request size limit (1 MB)
   - **Tests**: Integration tests

3. **Health Check Endpoint** (`cmd/remora`)
   - `GET /health` returns 200 if healthy, 503 if not
   - Check database connectivity
   - Include version, timestamp
   - **Tests**: Health check tests

4. **Readiness Endpoint** (`cmd/remora`)
   - `GET /ready` returns 200 when ready
   - Checks: migrations complete, scheduler started
   - **Tests**: Readiness tests

5. **Error Handling**
   - Standard error response format (JSON)
   - HTTP status codes
   - Logging all requests
   - **Tests**: Error response tests

**Deliverables**:
- ✅ HTTP server with webhook endpoint
- ✅ Health and readiness endpoints
- ✅ Request logging
- ✅ Graceful shutdown

**Estimated Time**: 2-3 days

---

## Phase 7: Admin API (Optional)

**Goal**: Query API for operational visibility

### Tasks

1. **Authentication Middleware** (`internal/api`)
   - Validate `X-API-Key` header
   - Reject unauthorized requests (401)
   - **Tests**: Auth middleware tests

2. **Reminder Query Endpoint** (`internal/api`)
   - `GET /api/v1/reminders` with filters
   - Query parameters: repository, issue, status, user
   - Pagination (limit, offset)
   - **Tests**: Query tests with various filters

3. **Response Formatting** (`internal/api`)
   - JSON response with reminder list
   - Include metadata (total, limit, offset)
   - **Tests**: Response format tests

**Deliverables**:
- ✅ Admin API with secret authentication
- ✅ Query endpoint with filters
- ✅ Pagination support

**Estimated Time**: 2-3 days

---

## Phase 8: Docker & Deployment

**Goal**: Containerize application and prepare for deployment

### Tasks

1. **Dockerfile** (`deployments/docker`)
   - Multi-stage build (builder + runtime)
   - Alpine base image
   - Non-root user
   - Static binary compilation
   - **Tests**: Build and run Docker image locally

2. **Docker Compose** (`deployments/docker`)
   - Remora service
   - PostgreSQL service (for local dev)
   - Environment variables
   - Volume mounts
   - **Tests**: Run full stack locally

3. **Kubernetes Manifests** (`deployments/kubernetes`)
   - Deployment YAML
   - Service YAML
   - ConfigMap for non-secret config
   - Secret for sensitive data
   - Health/readiness probes
   - **Tests**: Deploy to local Kubernetes (kind/minikube)

4. **Environment Configuration**
   - Document all environment variables
   - Provide examples for each database type
   - Secret management guidance
   - **Tests**: Verify all configs work

**Deliverables**:
- ✅ Production-ready Dockerfile
- ✅ Docker Compose for local development
- ✅ Kubernetes manifests
- ✅ Deployment documentation

**Estimated Time**: 2-3 days

---

## Phase 9: Testing & Quality Assurance

**Goal**: Comprehensive testing and quality checks

### Tasks

1. **Unit Test Coverage**
   - Achieve >80% coverage across all packages
   - Test edge cases
   - Test error paths
   - **Target**: 80%+ coverage

2. **Integration Tests** (`test/integration`)
   - End-to-end webhook → database → scheduler → GitHub flow
   - Test with all three database types
   - Test failure scenarios
   - **Target**: All critical paths tested

3. **Manual Testing**
   - Deploy to test GitHub repository
   - Test various time expressions
   - Test error scenarios
   - Test deleted/closed issues
   - Verify timezone handling

4. **Load Testing**
   - Simulate multiple concurrent webhooks
   - Test scheduler with many due reminders
   - Verify database performance
   - **Target**: Handle 100 concurrent webhooks

5. **Code Quality**
   - Run `golangci-lint` with strict rules
   - Fix all linter warnings
   - Code review checklist
   - **Target**: Zero linter errors

**Deliverables**:
- ✅ >80% test coverage
- ✅ All integration tests passing
- ✅ Manual testing complete
- ✅ Load testing results
- ✅ Clean linter output

**Estimated Time**: 3-4 days

---

## Phase 10: Documentation & Launch Prep

**Goal**: Complete documentation and prepare for launch

### Tasks

1. **README.md**
   - Project overview
   - Features list
   - Installation instructions
   - Configuration guide
   - Usage examples
   - Troubleshooting

2. **GitHub App Setup Guide**
   - How to create GitHub App
   - Required permissions
   - Webhook configuration
   - Installation instructions

3. **Deployment Guide**
   - Docker deployment
   - Kubernetes deployment
   - Environment variable reference
   - Database setup guides

4. **User Documentation**
   - Command syntax examples
   - Supported time formats
   - Timezone usage
   - Error messages explained
   - Limitations (single instance, etc.)

5. **Operator Documentation**
   - Monitoring and observability
   - Log formats
   - Health check usage
   - Backup and restore
   - Troubleshooting common issues

6. **Contributing Guide**
   - Development setup
   - Running tests
   - Code style guide
   - PR process

**Deliverables**:
- ✅ Complete user documentation
- ✅ Deployment guides
- ✅ Operator documentation
- ✅ Contributing guide

**Estimated Time**: 2-3 days

---

## Phase 11: Production Launch

**Goal**: Deploy to production and monitor

### Tasks

1. **Production Deployment**
   - Deploy to production environment
   - Configure monitoring/alerting
   - Set up log aggregation
   - Verify health checks

2. **GitHub App Publication**
   - Create production GitHub App
   - Configure webhook URL
   - Set permissions
   - Install on initial repositories

3. **Monitoring Setup**
   - Set up uptime monitoring
   - Configure alerting (PagerDuty, Slack, etc.)
   - Dashboard for key metrics
   - Log monitoring

4. **Announcement**
   - Publish blog post/announcement
   - Share on social media
   - Post to relevant communities

**Deliverables**:
- ✅ Production deployment
- ✅ Monitoring and alerting
- ✅ Public announcement

**Estimated Time**: 2-3 days

---

## Future Enhancements (Post-Launch)

### Phase 12: Metrics & Observability

- Prometheus metrics endpoint (`/metrics`)
- Grafana dashboards
- Trace sampling
- Performance profiling

### Phase 13: Advanced Features

- Recurring reminders
- Custom reminder messages
- Reminder editing (via comment edit)
- Cross-repository reminders
- Reminder cancellation command

### Phase 14: Multi-Instance Support

- Distributed locking (Redis)
- Leader election
- Horizontal scaling support
- Load balancing

### Phase 15: Web Dashboard

- User-facing web UI
- GitHub OAuth integration
- Reminder management interface
- Analytics and statistics

---

## Overall Timeline

| Phase | Focus Area | Duration | Dependencies |
|-------|-----------|----------|--------------|
| Phase 0 | Project Setup | 1-2 days | None |
| Phase 1 | Core Infrastructure | 3-4 days | Phase 0 |
| Phase 2 | Time Parsing | 3-4 days | Phase 1 |
| Phase 3 | GitHub Integration | 3-4 days | Phase 1 |
| Phase 4 | Webhook Handler | 4-5 days | Phase 2, 3 |
| Phase 5 | Scheduler | 4-5 days | Phase 3 |
| Phase 6 | HTTP Server | 2-3 days | Phase 4, 5 |
| Phase 7 | Admin API (Optional) | 2-3 days | Phase 6 |
| Phase 8 | Docker & Deployment | 2-3 days | Phase 6 |
| Phase 9 | Testing & QA | 3-4 days | Phase 8 |
| Phase 10 | Documentation | 2-3 days | Phase 9 |
| Phase 11 | Production Launch | 2-3 days | Phase 10 |

**Total Estimated Time**: 6-8 weeks (30-40 working days)

**With Admin API**: Add 2-3 days
**Without Admin API**: Can skip Phase 7

---

## Milestones

### Milestone 1: Foundation Complete (End of Phase 1)
- ✅ Project structure established
- ✅ Database layer working
- ✅ CI/CD pipeline active

### Milestone 2: Core Features Complete (End of Phase 5)
- ✅ Webhook processing
- ✅ Reminder creation and storage
- ✅ Scheduler firing reminders
- ✅ GitHub API integration

### Milestone 3: Production Ready (End of Phase 9)
- ✅ All features implemented
- ✅ Tests passing
- ✅ Docker images built
- ✅ Ready to deploy

### Milestone 4: Live in Production (End of Phase 11)
- ✅ Deployed and monitored
- ✅ Users can install GitHub App
- ✅ Public announcement

---

## Development Principles

### 1. Test-Driven Development
- Write tests before implementation
- Aim for >80% coverage
- Test edge cases and error paths

### 2. Iterative Development
- Build incrementally
- Deploy to staging frequently
- Get feedback early

### 3. Documentation as Code
- Update docs with code changes
- Keep specs in sync with implementation
- Document decisions in code comments

### 4. Quality Over Speed
- Don't skip tests to save time
- Refactor as needed
- Code reviews for all changes

### 5. Security First
- Validate all inputs
- Handle secrets securely
- Follow least-privilege principle

---

## Risk Management

### Technical Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| `olebedev/when` parsing errors | High | Extensive testing, validation layer |
| GitHub API rate limits | Medium | Monitor usage, exponential backoff |
| Database performance issues | Medium | Proper indexing, connection pooling |
| Scheduler reliability | High | Comprehensive tests, retry logic |
| Docker image size | Low | Multi-stage builds, Alpine base |

### Operational Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Downtime during deployment | Medium | Zero-downtime deployment, health checks |
| Data loss | High | Database backups, tested restore process |
| Security vulnerabilities | High | Dependency scanning, security reviews |
| Single instance limitation | Low | Document clearly, plan for future HA |

### Schedule Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Underestimated complexity | Medium | Buffer time in estimates, prioritize ruthlessly |
| Scope creep | Medium | Clear phase boundaries, defer enhancements |
| External dependencies | Low | Early integration tests, fallback plans |

---

## Success Criteria

### Technical Success
- ✅ All tests passing (>80% coverage)
- ✅ Zero critical bugs
- ✅ Clean linter output
- ✅ Performance targets met (handle 100 concurrent webhooks)

### Product Success
- ✅ Successfully creates and fires reminders
- ✅ Handles all documented time formats
- ✅ Clear error messages for invalid inputs
- ✅ Works with all three database types

### Operational Success
- ✅ Health checks working
- ✅ Logs structured and searchable
- ✅ Deployable via Docker/Kubernetes
- ✅ Monitoring and alerting configured

### User Success
- ✅ Clear documentation
- ✅ Easy to install GitHub App
- ✅ Intuitive command syntax
- ✅ Reliable reminder delivery
