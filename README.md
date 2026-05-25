# Remora

A GitHub App that turns `remora <time>` comments on issues and pull requests into scheduled reminders.

When someone writes `remora in 2 days` in an issue comment, Remora reacts to the comment (👀), waits the requested duration, and posts a reminder back on the issue.

## Features

- Natural-language time expressions (`in 2 hours`, `tomorrow at 9am`, `next Monday`, `30 minutes`)
- Optional timezone in the command (`remora in 1 hour CET`)
- Cancel a pending reminder by deleting the original comment
- Works on issues and pull requests
- Supports PostgreSQL, MySQL, and SQLite
- Single-binary Go service, distroless container image
- Webhook signature verification, retry with exponential backoff, graceful shutdown
- Optional admin API for querying reminder state

## Quick start (Docker Compose)

```bash
cp .env.example .env
# Edit .env and set GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, GITHUB_WEBHOOK_SECRET
docker compose -f deployments/docker/docker-compose.yml up --build
```

The service listens on `http://localhost:8080/webhook`. Expose it publicly with a tunnel (e.g. ngrok) and point your GitHub App webhook URL at it.

## GitHub App setup

Create a GitHub App with these permissions (see [spec/api-design.md](spec/api-design.md) for full detail):

| Permission | Access |
|---|---|
| Issues | Read & Write |
| Pull Requests | Read & Write |
| Metadata | Read |

Subscribe to the **Issue comment** event. Generate a private key, set a webhook secret, and install the App on the repositories you want to enable.

## Command syntax

```
remora <time-expression> [timezone]
```

Examples (see [spec/comments.md](spec/comments.md) for the full grammar):

```
remora 30 minutes
remora in 2 hours
remora tomorrow at 9am
remora next Monday EST
remora in 1 week
```

Rules: minimum delay 15 minutes, maximum 13 months (395 days). The command can appear anywhere in the comment; only the first match is used. Default timezone is UTC.

## Configuration

All configuration is via environment variables. See [`.env.example`](.env.example) for the full list with defaults.

Key variables:

| Variable | Required | Default | Notes |
|---|---|---|---|
| `GITHUB_APP_ID` | yes | — | Numeric App ID from GitHub |
| `GITHUB_APP_PRIVATE_KEY` | yes | — | PEM-encoded private key |
| `GITHUB_WEBHOOK_SECRET` | yes | — | HMAC secret for webhook verification |
| `DATABASE_TYPE` | no | `sqlite` | One of `postgresql`, `mysql`, `sqlite` |
| `DATABASE_HOST` | conditional | — | Required for postgresql/mysql |
| `DATABASE_NAME` | conditional | — | Required for postgresql/mysql |
| `DATABASE_USER` | conditional | — | Required for postgresql/mysql |
| `DATABASE_PASSWORD` | conditional | — | Required for postgresql/mysql |
| `DATABASE_SQLITE_PATH` | no | `./data/remora.db` | Used when `DATABASE_TYPE=sqlite` |
| `REMORA_PORT` | no | `8080` | HTTP port |
| `REMORA_SCHEDULER_INTERVAL` | no | `5` | Minutes between scheduler polls |
| `REMORA_ERROR_MODE` | no | `reaction_only` | Or `reaction_and_comment` |
| `REMORA_ENABLE_API` | no | `false` | Enables the admin query API |
| `REMORA_API_SECRET` | conditional | — | Required when `REMORA_ENABLE_API=true` |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error`, `fatal` |

## Deployment

- **Docker**: [`deployments/docker/`](deployments/docker/) — Dockerfile (distroless runtime) and Compose file.
- **Kubernetes**: [`deployments/kubernetes/`](deployments/kubernetes/) — Kustomize-based manifests including an in-cluster Postgres StatefulSet.

See [`deployments/DEPLOYMENT.md`](deployments/DEPLOYMENT.md) for environment-specific guidance.

## Development

```bash
make build              # build the binary
make test               # unit tests
make test-integration   # integration tests (mock GitHub server)
make lint               # golangci-lint
make run                # run locally against env vars
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full dev workflow.

## Documentation

| Document | Purpose |
|---|---|
| [spec/architecture.md](spec/architecture.md) | System architecture overview |
| [spec/api-design.md](spec/api-design.md) | HTTP endpoints and GitHub integration |
| [spec/comments.md](spec/comments.md) | Command grammar and supported time formats |
| [spec/database-schema.md](spec/database-schema.md) | Reminder table and indexes |
| [spec/edge-cases.md](spec/edge-cases.md) | Behavior for unusual inputs |
| [spec/observability.md](spec/observability.md) | Logging and monitoring design |
| [spec/development-roadmap.md](spec/development-roadmap.md) | Phased delivery plan |

## Status

Phases 0–8 of the [development roadmap](spec/development-roadmap.md) are complete: core service, scheduler, GitHub integration, admin API, and Docker/Kubernetes deployment artifacts. Phase 9–11 (final QA, docs, production launch) are in progress.

## License

TBD.
