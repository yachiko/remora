# Security Policy

## Supported Versions

Remora is pre-1.0. Only the latest `0.x` release receives security fixes.

| Version | Supported |
| ------- | --------- |
| 0.x     | ✅        |
| < 0.1.0 | ❌        |

## Reporting a Vulnerability

Please report security vulnerabilities **privately** via GitHub Security Advisories:

1. Open the project's [Security tab](https://github.com/yachiko/remora/security/advisories).
2. Click **Report a vulnerability**.
3. Provide a clear description, reproduction steps, and the impact you observed.

Do not open a public issue or PR for suspected vulnerabilities.

## What to Expect

- **Acknowledgement** within 5 business days.
- **Initial assessment** (severity, scope, affected versions) within 10 business days.
- **Fix or mitigation timeline** communicated once the assessment is complete.
- **Public disclosure** coordinated with the reporter; advisory and patched release published together.

## Threat Model

Remora is a GitHub App that:

- Receives webhooks from GitHub (Issue comment events).
- Stores scheduled reminders in a database (PostgreSQL, MySQL, or SQLite).
- Calls the GitHub API on behalf of the installed App to post comments and add reactions.

Relevant threat surfaces:

- **Webhook signature verification.** Every incoming webhook is verified with HMAC-SHA256 against `GITHUB_WEBHOOK_SECRET` before the body is parsed. Requests with a missing or invalid signature are rejected with `401`.
- **GitHub App private key.** The PEM-encoded key in `GITHUB_APP_PRIVATE_KEY` mints installation tokens. Treat it like a root credential — store in a secret manager, mount read-only, rotate on compromise.
- **Admin API.** Disabled by default. When enabled with `REMORA_ENABLE_API=true`, every endpoint requires a bearer token (`REMORA_API_SECRET`). Run behind your existing network controls; the API is not designed for direct public exposure.
- **Database credentials.** Provided via environment variables; never logged. SQLite mode keeps data on local disk only.
- **Time-expression parser.** All command parsing happens after webhook signature verification, so untrusted input cannot reach the parser without a valid HMAC.

Remora does not store GitHub OAuth user tokens; all GitHub calls use installation tokens minted on demand from the App private key.

## Out of Scope

- Compromise of the host machine or container runtime.
- Bugs in upstream dependencies after Dependabot has had a chance to update them.
- Misuse of an exposed admin API endpoint when `REMORA_API_SECRET` has been shared.

## Defensive Measures

- Distroless container image (no shell, minimal attack surface).
- Webhook signature verification is mandatory and cannot be disabled.
- Trivy scans the published image on every tag (`CRITICAL` / `HIGH` severity fails the publish workflow).
- CodeQL `security-and-quality` query pack runs on every push/PR and weekly.
- All GitHub Actions in workflows are pinned to commit SHAs; Dependabot keeps them current.
- Graceful shutdown drains in-flight webhooks before exit.
