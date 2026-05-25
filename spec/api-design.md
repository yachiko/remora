# API Design

## Overview

Remora exposes HTTP endpoints for GitHub webhook integration, health monitoring, and optionally querying reminders. All endpoints follow REST principles and use JSON for data exchange.

---

## GitHub App Configuration

### Required Permissions

The GitHub App requires the following permissions:

| Permission        | Access Level | Purpose                                                     |
| ----------------- | ------------ | ----------------------------------------------------------- |
| **Issues**        | Read & Write | Post reminder comments, add reactions to issue comments     |
| **Pull Requests** | Read & Write | Post reminder comments on PRs, add reactions to PR comments |
| **Metadata**      | Read         | Access repository information (owner, name)                 |

**Note**: GitHub API treats issue comments and PR comments similarly - both use the issue comments endpoint. However, requesting both permissions explicitly ensures clarity.

### Webhook Events

Subscribe to the following webhook events:

- `issue_comment` - Triggered when comments are created, edited, or deleted on issues
- `pull_request_review_comment` - Triggered for PR review comments (optional, if supporting review comments)

### GitHub App Manifest Example

```yaml
name: Remora
description: Set reminders on GitHub issues and pull requests
url: https://github.com/owner/remora
hook_attributes:
  url: https://your-domain.com/webhook
  active: true
public: true
default_permissions:
  issues: write
  pull_requests: write
  metadata: read
default_events:
  - issue_comment
```

### Installation Token Management

**Token Lifecycle**:
- GitHub installation tokens expire after **60 minutes**
- Tokens are scoped to specific installations (repository or organization)

**Caching Strategy**:

```go
type TokenCache struct {
    tokens map[int64]*CachedToken // Key: installation_id
    mu     sync.RWMutex
}

type CachedToken struct {
    Token     string
    ExpiresAt time.Time
}
```

**Cache Implementation**:
1. **Cache Key**: Installation ID (one token per installation)
2. **TTL**: Refresh tokens at **55 minutes** (5-minute safety buffer)
3. **Storage**: In-memory map with RWMutex for thread safety
4. **Refresh Logic**: Check expiration before each use, refresh if needed
5. **Cleanup**: Optional background goroutine to remove expired entries

**Token Refresh Flow**:
```go
func (c *TokenCache) GetToken(installationID int64) (string, error) {
    c.mu.RLock()
    cached, exists := c.tokens[installationID]
    c.mu.RUnlock()
    
    if exists && time.Now().Before(cached.ExpiresAt.Add(-5 * time.Minute)) {
        return cached.Token, nil
    }
    
    // Token expired or doesn't exist, fetch new one
    return c.refreshToken(installationID)
}
```

---

## Endpoints

### 1. GitHub Webhook Endpoint

**Purpose**: Receive events from GitHub (issue comments, PR comments)

```
POST /webhook
```

**Headers**:
- `Content-Type: application/json`
- `X-GitHub-Event: issue_comment` (or other event types)
- `X-GitHub-Delivery: <unique-id>`
- `X-Hub-Signature-256: sha256=<hmac>` (webhook signature for validation)

**Request Body**: GitHub webhook payload (varies by event type)

**Response**:
- `200 OK` - Webhook processed successfully
- `400 Bad Request` - Invalid payload
- `401 Unauthorized` - Invalid signature
- `500 Internal Server Error` - Processing error

**Example Issue Comment Payload** (relevant fields):
```json
{
  "action": "created",
  "comment": {
    "id": 987654321,
    "node_id": "IC_kwDOABcD...",
    "html_url": "https://github.com/owner/repo/issues/123#issuecomment-987654321",
    "user": {
      "login": "username",
      "id": 12345,
      "type": "User"
    },
    "body": "remora 2 days",
    "created_at": "2025-11-13T10:30:00Z",
    "updated_at": "2025-11-13T10:30:00Z"
  },
  "issue": {
    "id": 456789,
    "number": 123,
    "title": "Issue title",
    "html_url": "https://github.com/owner/repo/issues/123",
    "state": "open",
    "user": {
      "login": "issue-author",
      "id": 67890
    }
  },
  "repository": {
    "id": 123456789,
    "name": "repo",
    "full_name": "owner/repo",
    "owner": {
      "login": "owner",
      "id": 11111,
      "type": "Organization"
    },
    "private": false
  },
  "installation": {
    "id": 12345678
  },
  "sender": {
    "login": "username",
    "id": 12345,
    "type": "User"
  }
}
```

**Processing Flow**:
1. Validate webhook signature using `X-Hub-Signature-256` header
2. Validate payload structure and required fields
3. Filter for `issue_comment` or `pull_request_review_comment` events
4. Check if comment body starts with "remora "
5. Parse time expression
6. Store reminder in database
7. Add reaction (👀/✅) to comment via GitHub API
8. Return 200 OK

**Webhook Payload Validation**:

Beyond signature validation, Remora validates:

1. **Required Fields Presence**:
   - `action` field exists and is a string
   - `comment` object exists with `id`, `body`, `user`, `html_url`
   - `repository` object exists with `owner`, `name`, `full_name`
   - `issue` or `pull_request` object exists with `number`
   - `installation.id` exists

2. **Field Type Validation**:
   - `comment.id` is a number
   - `issue.number` or `pull_request.number` is a number
   - `comment.body` is a string

3. **Payload Size**:
   - Maximum 1 MB payload size
   - Reject larger payloads with 413 (Payload Too Large)

4. **Unknown Fields**:
   - Log warnings for unknown top-level fields
   - Continue processing (forward compatibility)

**Validation Error Responses**:
- Missing required fields → 400 Bad Request
- Invalid field types → 400 Bad Request  
- Payload too large → 413 Payload Too Large
- Invalid signature → 401 Unauthorized

**Security**:
- HMAC-SHA256 signature validation using webhook secret
- Reject requests with invalid signatures
- Payload structure validation (required fields, types)
- Request size limits (1 MB maximum)
- Request timeout (10 seconds to respond to GitHub)

---

### 2. Health Check Endpoint

**Purpose**: Monitor application health for load balancers and orchestrators

```
GET /health
```

**Response** (200 OK):
```json
{
  "status": "healthy",
  "timestamp": "2025-11-13T10:30:00Z",
  "version": "1.0.0",
  "checks": {
    "database": "ok",
    "github_api": "ok"
  }
}
```

**Response** (503 Service Unavailable - unhealthy):
```json
{
  "status": "unhealthy",
  "timestamp": "2025-11-13T10:30:00Z",
  "version": "1.0.0",
  "checks": {
    "database": "error: connection timeout",
    "github_api": "ok"
  }
}
```

**Health Checks Performed**:
- Database connectivity (ping)
- GitHub API reachability (optional, cached)

**Use Case**: Kubernetes liveness/readiness probes, load balancer health checks

---

### 3. Readiness Endpoint

**Purpose**: Indicate if app is ready to accept traffic (distinct from liveness)

```
GET /ready
```

**Response** (200 OK):
```json
{
  "ready": true
}
```

**Response** (503 Service Unavailable):
```json
{
  "ready": false,
  "reason": "database migrations pending"
}
```

**Checks**:
- Database migrations completed
- Scheduler started
- Configuration loaded

---

### 4. Reminder Query API (Optional - Security Discussion Needed)

**Purpose**: Allow users/admins to query their reminders

```
GET /api/v1/reminders
```

**Query Parameters**:
- `repository` - Filter by repository (format: `owner/repo`)
- `issue` - Filter by issue/PR number
- `status` - Filter by status (`pending`, `fired`, `failed`)
- `user` - Filter by username
- `limit` - Page size (default: 50, max: 100)
- `offset` - Pagination offset

**Response** (200 OK):
```json
{
  "reminders": [
    {
      "id": 123,
      "repository": "owner/repo",
      "issue_number": 456,
      "comment_url": "https://github.com/owner/repo/issues/456#issuecomment-789",
      "requester": "username",
      "remind_at": "2025-11-15T14:00:00Z",
      "status": "pending",
      "created_at": "2025-11-13T10:30:00Z"
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

**Security Options** (Discussion):

#### Option A: GitHub App Installation Token (Recommended)
- Require `Authorization: Bearer <github-token>` header
- Validate token with GitHub API
- Only return reminders for repos the token has access to
- **Pros**: Leverages GitHub's existing auth, proper access control
- **Cons**: Requires users to generate tokens, extra GitHub API calls

#### Option B: API Key per GitHub App Installation
- Generate unique API key per repository/org when GitHub App is installed
- Store in database, require in `X-API-Key` header
- **Pros**: Simple, no external validation
- **Cons**: Key management, rotation, revocation complexity

#### Option C: GitHub OAuth Flow
- Full OAuth flow for web-based access
- Session-based authentication
- **Pros**: Best UX for web dashboard
- **Cons**: Complex, requires web frontend

#### Option D: No Authentication (Repository-scoped only)
- Public endpoint but requires valid repository parameter
- Only return reminders for public repositories
- **Pros**: Simple
- **Cons**: Exposes reminder data, limited utility

#### Option E: Admin-only with Shared Secret
- Single shared secret for admin access
- Only for operational/debugging use
- **Pros**: Simple for admin tools
- **Cons**: Not suitable for end users

**Decision**: **Option E - Admin Secret** (admin-only with shared secret)

**Rationale**:
- Simple implementation (single environment variable)
- No key management infrastructure needed
- Sufficient for operational and debugging needs
- Can add user-facing auth later if needed

**Authentication**:
- Require `X-API-Key` header with admin secret
- Reject requests without valid key (401 Unauthorized)
- Provides global access to all reminders across repositories

**Example Request**:
```bash
curl -H "X-API-Key: your-admin-secret" \
  "https://remora.example.com/api/v1/reminders?repository=owner/repo&status=pending"
```

**Configuration**:
```bash
REMORA_API_SECRET=your-secure-random-secret-here
```

**Future Enhancement**: Can implement Option A (GitHub token validation) for user-facing access if needed.

---

### 5. Metrics Endpoint (Optional - Future)

**Purpose**: Prometheus-compatible metrics for monitoring

```
GET /metrics
```

**Response** (text/plain):
```
# HELP remora_reminders_total Total number of reminders created
# TYPE remora_reminders_total counter
remora_reminders_total 1234

# HELP remora_reminders_pending Number of pending reminders
# TYPE remora_reminders_pending gauge
remora_reminders_pending 45

# HELP remora_webhook_requests_total Total webhook requests received
# TYPE remora_webhook_requests_total counter
remora_webhook_requests_total{status="success"} 5678
remora_webhook_requests_total{status="failed"} 12
```

**Metrics to Track**:
- Total reminders created
- Pending reminders (gauge)
- Fired reminders count
- Failed reminders count
- Webhook requests (success/failure)
- GitHub API calls
- Parse errors
- Database query latency

---

## GitHub API Interactions

Remora interacts with GitHub's REST API for the following operations:

### 1. Add Reaction to Comment

**Endpoint**: `POST /repos/{owner}/{repo}/issues/comments/{comment_id}/reactions`

**Authentication**: GitHub App installation token

**Request Body**:
```json
{
  "content": "eyes"
}
```

**Reactions Used**:
- `eyes` (👀) - Reminder request received and parsing
- `+1` or `hooray` (✅ equivalent) - Reminder accepted
- `-1` or `confused` (❌ equivalent) - Error parsing or invalid request

**Rate Limiting**: Respect GitHub's rate limits (5000 req/hour for apps)

### 2. Post Comment

**Endpoint**: `POST /repos/{owner}/{repo}/issues/{issue_number}/comments`

**Authentication**: GitHub App installation token

**Request Body**:
```json
{
  "body": "@username 🔔 Reminder!\n\nYou asked to be reminded about this issue.\n\nOriginal request: https://github.com/owner/repo/issues/123#issuecomment-456"
}
```

**Reminder Comment Format**:

```markdown
@{username} 🔔 Reminder!

You asked to be reminded about this issue.

Original request: {comment_url}
```

**Example**:
```markdown
@alice 🔔 Reminder!

You asked to be reminded about this issue.

Original request: https://github.com/acme/project/issues/42#issuecomment-123456
```

**Template Variables**:
- `{username}`: GitHub username of requester (e.g., `alice`)
- `{comment_url}`: Full URL to original comment where reminder was requested

**Future Enhancement**: Comment format may become configurable via environment variable:
```bash
REMORA_COMMENT_TEMPLATE="@{username} reminder from {comment_url}"
```

**When Used**:
- Firing a reminder (always)
- Error explanation (if configured)

### 3. Get Installation Token

**Endpoint**: `POST /app/installations/{installation_id}/access_tokens`

**Authentication**: GitHub App JWT (signed with private key)

**Response**:
```json
{
  "token": "ghs_...",
  "expires_at": "2025-11-13T11:30:00Z"
}
```

**Token Management**:
- Cache tokens until near expiration
- Refresh automatically
- Handle per-installation (repository/org level)

---

## Comment Format Specification

### Valid Command Format

```
remora <time-expression> [timezone]
```

**Examples**:
- `remora 2 days`
- `remora tomorrow at 3pm`
- `remora 25th December`
- `remora next Monday 9am EST`
- `remora 1 week 3 days`

**Parsing Rules**:
1. Command must start with `remora` (case-insensitive)
2. Time expression follows after space
3. Optional timezone suffix (PST, EST, America/New_York, etc.)
4. Parsed time must be in the future (reject past dates)

**Invalid Examples**:
- `remora yesterday` (past date)
- `remora` (no time expression)
- `remind me in 2 days` (wrong prefix)
- `@remora 2 days` (mentions not supported initially)

### Error Handling

**Reaction Strategy** (configurable via `REMORA_ERROR_MODE`):

1. **Mode: `reaction_only`** (default)
   - Add ❌ reaction to comment
   - No explanatory comment

2. **Mode: `reaction_and_comment`**
   - Add ❌ reaction to comment
   - Post comment with error explanation:
     ```
     @username I couldn't parse your reminder request. 
     
     Please use the format: `remora <time-expression>`
     
     Examples:
     - remora 2 days
     - remora tomorrow at 3pm
     - remora next Monday 9am EST
     ```

---

## Configuration

### Environment Variables

API-related configuration:

- `REMORA_PORT` - HTTP server port (default: `8080`)
- `REMORA_WEBHOOK_PATH` - Webhook endpoint path (default: `/webhook`)
- `REMORA_WEBHOOK_SECRET` - GitHub webhook secret for signature validation
- `REMORA_HEALTH_PATH` - Health check path (default: `/health`)
- `REMORA_READY_PATH` - Readiness check path (default: `/ready`)
- `REMORA_ERROR_MODE` - Error handling mode (`reaction_only` or `reaction_and_comment`)
- `REMORA_ENABLE_API` - Enable reminder query API (default: `false`)
- `REMORA_API_SECRET` - Shared secret for admin API access (if enabled)
- `REMORA_RATE_LIMIT` - Max webhook requests per minute (default: `60`)

GitHub App configuration:

- `GITHUB_APP_ID` - GitHub App ID
- `GITHUB_APP_PRIVATE_KEY` - GitHub App private key (PEM format)
- `GITHUB_WEBHOOK_SECRET` - Webhook secret for validation

---

## Rate Limiting

### Webhook Endpoint

**Strategy**: Token bucket or simple counter

**Limits**:
- Per IP: 60 requests/minute (configurable)
- Global: Based on expected GitHub webhook volume

**Response** (429 Too Many Requests):
```json
{
  "error": "rate limit exceeded",
  "retry_after": 30
}
```

### GitHub API Calls

**Strategy**: Respect GitHub's rate limits

**Limits**:
- 5000 requests/hour for GitHub Apps
- Track remaining quota from `X-RateLimit-Remaining` header
- Exponential backoff on rate limit errors

---

## Error Responses

### Standard Error Format

```json
{
  "error": "error message",
  "details": "additional context (optional)",
  "timestamp": "2025-11-13T10:30:00Z"
}
```

### HTTP Status Codes

- `200 OK` - Success
- `400 Bad Request` - Invalid request payload
- `401 Unauthorized` - Invalid authentication
- `403 Forbidden` - Valid auth but insufficient permissions
- `404 Not Found` - Resource not found
- `429 Too Many Requests` - Rate limit exceeded
- `500 Internal Server Error` - Server error
- `503 Service Unavailable` - Service unhealthy

---

## Security Considerations

### Webhook Security

1. **Signature Validation**: Always validate `X-Hub-Signature-256` header
2. **HTTPS Only**: Reject non-HTTPS requests in production
3. **IP Whitelisting**: Optional - restrict to GitHub's webhook IPs
4. **Request Size Limits**: Max 1MB payload size
5. **Timeout**: 10-second request timeout

### API Security

1. **Authentication**: Required for all non-health endpoints
2. **HTTPS Only**: TLS 1.2+ required
3. **Rate Limiting**: Prevent abuse
4. **Input Validation**: Sanitize all user inputs
5. **CORS**: Disabled by default (can enable for web dashboard)

### Secret Management

- Never log secrets
- Rotate webhook secret periodically
- Store GitHub App private key securely (environment variable or secret manager)
- Use GitHub's secret scanning to detect leaked secrets

---

## Testing Strategy

### Webhook Testing

- Mock GitHub webhook payloads
- Test signature validation
- Test various comment formats
- Test rate limiting
- Test error scenarios

### Health Check Testing

- Simulate database failure
- Test during startup (before ready)
- Test degraded scenarios

### Integration Testing

- Real GitHub API interactions (test repository)
- End-to-end webhook → database → scheduler → comment flow

---

---

## Future Enhancements

- GraphQL API (alternative to REST)
- WebSocket for real-time reminder updates
- Batch operations (cancel multiple reminders)
- Reminder modification (reschedule, cancel)
- Dashboard UI (web frontend)
