# Contributing to Remora

Thanks for your interest in helping. This document covers local setup, the test workflow, and the conventions this repo follows.

## Prerequisites

- Go **1.25** or newer (matches `go.mod`)
- Docker (for running integration tests against real databases and for building the deployment image)
- `golangci-lint` — install with `make install-tools`

## Repository layout

```
cmd/remora/         entry point
internal/           private packages (config, database, github, parser, scheduler, webhook, api, models, logger)
test/integration/   integration tests with a mock GitHub server
deployments/        Dockerfile, docker-compose, Kubernetes manifests
spec/               design specifications
```

See [spec/project-structure.md](spec/project-structure.md) for the rationale.

## Local development

```bash
cp .env.example .env
# Fill in GITHUB_APP_ID, GITHUB_APP_PRIVATE_KEY, GITHUB_WEBHOOK_SECRET

make build            # build ./bin/remora
make run              # run locally
make test             # unit tests
make test-integration # integration tests (gated by REMORA_INTEGRATION_TESTS=1)
make test-coverage    # tests + coverage.html
make lint             # golangci-lint
make fmt              # gofmt
```

The default `DATABASE_TYPE=sqlite` writes to `./data/remora.db`, so you can run locally without a database server.

## Test expectations

- Unit tests live next to the code they cover (`foo.go` → `foo_test.go`).
- Integration tests live in `test/integration/` and are guarded by the `REMORA_INTEGRATION_TESTS=1` environment variable so `go test ./...` stays fast.
- Aim for >80% line coverage (matches the project bar set in [spec/testing.md](spec/testing.md)).
- New behavior needs a test that fails before your change and passes after.

## Code style

- `golangci-lint` is the source of truth. Config lives in [`.golangci.yml`](.golangci.yml). CI runs it on every PR.
- Run `make fmt` and `make lint` before pushing.
- Keep packages focused; prefer small interfaces at consumption sites over large ones at definition sites.
- No comments unless the *why* is non-obvious. Code should be self-documenting.

## Commit messages

Follow the existing Conventional Commits style:

```
feat(scope): short description       # new functionality
fix(scope): short description        # bug fix
refactor(scope): short description   # behavior preserved
chore(scope): short description      # tooling, deps, formatting
ci: short description                # GitHub Actions
docs: short description              # README / spec changes
test: short description              # tests only
```

Scope is the package or area (`scheduler`, `webhook`, `deploy`, …). One logical change per commit.

## Pull requests

1. Branch from `main`.
2. Make atomic commits following the convention above.
3. Run `make lint test test-integration` locally and make sure CI passes.
4. Open a PR against `main` with:
   - what changed and why,
   - any spec/doc updates,
   - a brief test plan.
5. Address review feedback in additional commits; do not force-push during review.

## Reporting issues

Open a GitHub issue with:
- Remora version (`git rev-parse --short HEAD` for now; tagged releases coming).
- Database type and version.
- Relevant log output (`LOG_LEVEL=debug`).
- Steps to reproduce.

## Security

For suspected security issues, please do **not** open a public issue. Contact the maintainers directly first.
