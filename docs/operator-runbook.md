# Operator Runbook

Day-to-day operations guide for running Remora in production. Pair it with [deployments/DEPLOYMENT.md](../deployments/DEPLOYMENT.md) (one-time setup) and [spec/observability.md](../spec/observability.md) (logging design).

---

## Initial production launch

### 1. Provision

Pick one and follow the matching section of [deployments/DEPLOYMENT.md](../deployments/DEPLOYMENT.md):

| Target | Notes |
|---|---|
| Docker host (single node) | Simplest. Use `deployments/docker/docker-compose.yml` as the base; replace dev passwords with secrets management. |
| Kubernetes | `kubectl apply -k deployments/kubernetes/` — kustomization includes the Postgres StatefulSet for in-cluster DB. |
| Managed (Fly, Render, ECS, Cloud Run) | Use the distroless image from GHCR. Expose port 8080. Provide env vars per [`.env.example`](../.env.example). |

A single-process deployment is the supported topology today; multi-instance horizontal scaling is on the roadmap as Phase 14.

### 2. Publish the GitHub App

- Create the App at https://github.com/settings/apps/new (or under your org).
- Permissions: Issues = Read & Write, Pull Requests = Read & Write, Metadata = Read.
- Subscribe to event: **Issue comment**.
- Webhook URL: `https://<your-host>/webhook`. Must be HTTPS.
- Webhook secret: generate with `openssl rand -hex 32`. Set the same value as `GITHUB_WEBHOOK_SECRET`.
- Private key: download the PEM, supply it as `GITHUB_APP_PRIVATE_KEY` (mind newlines if using ConfigMaps/Secrets).
- (Optional) Toggle "Public" once you're ready for outside installations.
- Install on the target repositories/org.

Verify the App is healthy by running [`docs/smoke-test.md`](smoke-test.md) end-to-end.

### 3. Configure monitoring

There is no `/metrics` endpoint yet (Phase 12). Until then, build alerts on:

- **Health probe**: `GET /health` — alert if non-200 for > 2 minutes.
- **Readiness probe**: `GET /ready` — alert if non-200 for > 5 minutes (DB connectivity).
- **HTTP 5xx rate on `/webhook`**: from your reverse proxy / ingress logs. Sustained > 1% is a problem.
- **Scheduler heartbeat in logs**: every `REMORA_SCHEDULER_INTERVAL` (default 5 min) the scheduler emits an `info` log. Alert if absent for > 3× the interval.
- **Reminder failure rate**: count `status="failed"` rows that have been retried 5 times. Threshold: > 5 in a 1-hour window.

Recommended dashboards (build from logs until metrics ship):

- Webhooks received per minute (filter on `event=webhook_received`).
- Reminders fired per minute (`event=reminder_fired`).
- Reminders cancelled per minute (`event=reminder_cancelled`).
- GitHub API rate-limit warnings (`level=warn` + `component=github`).

### 4. Log aggregation

Logs are structured JSON in production (see [spec/observability.md](../spec/observability.md)). Ship them to your aggregator of choice (Loki, Datadog, CloudWatch). Useful queries:

- `request_id=<id>` — trace a single webhook through parsing, DB write, and scheduler firing.
- `component=scheduler level>=warn` — scheduler errors and retries.
- `component=github level>=warn` — GitHub API issues.
- `level=fatal` — startup failures.

## Common failure modes

### "Webhook signature mismatch" floods the log

Cause: `GITHUB_WEBHOOK_SECRET` no longer matches the value configured on the GitHub App.

Fix: rotate the secret in the App settings, update the deployment secret, restart.

### Reminders are not firing

Check in order:

1. `GET /ready` — is the DB reachable?
2. Logs from the `scheduler` component — is the polling loop alive? Is the interval what you expect?
3. Query the DB: `SELECT status, COUNT(*) FROM reminders GROUP BY status;` — are reminders stuck in `processing` (worker died mid-flight) or piling up in `pending`?
4. GitHub App status page (https://www.githubstatus.com/) — if the API is degraded, retries will exhaust.

Stuck-in-`processing` rows from a previous crash: safe to manually reset to `pending` so the next tick picks them up.

### Out-of-memory on Postgres pod

Default StatefulSet limits memory to 512Mi. Bump in `deployments/kubernetes/postgres-statefulset.yaml` if the reminders table grows large or you increase pool sizes.

### GitHub rate limit warnings

Look for `rate_limit_remaining` in `component=github` logs. Each installation has its own bucket. If you're consistently low, check for retry storms (an issue commenting bot) or expand to multiple Apps / use rate-limit headers more aggressively.

## Routine operations

### Upgrade

1. Build/pull the new image (CI publishes on tag push — see `.github/workflows/publish.yaml`).
2. Roll the deployment (Docker: recreate container; K8s: `kubectl rollout restart deployment/remora`).
3. Watch logs for migration output and `scheduler started`.
4. Re-run a happy-path scenario from [docs/smoke-test.md](smoke-test.md).

### Database backup

Postgres (in-cluster):

```bash
kubectl exec -n remora postgres-0 -- pg_dump -U remora_user remora > backup-$(date +%F).sql
```

External Postgres: use your provider's snapshots.

SQLite: copy `DATABASE_SQLITE_PATH` while the process is stopped.

### Restore

Postgres:

```bash
kubectl exec -i -n remora postgres-0 -- psql -U remora_user remora < backup-YYYY-MM-DD.sql
```

After restore, restart Remora so reminder rows in `processing` reset cleanly.

### Rotating the GitHub App private key

1. Generate a new key from the App settings; keep the old one.
2. Deploy with the new key in `GITHUB_APP_PRIVATE_KEY`. Tokens are cached for ~1 hour, so brief overlap is fine.
3. Once you're sure the new key works (smoke scenarios 1 and 9), delete the old key in the App settings.

### Rotating the webhook secret

1. Deploy with both old and new secret accepted — **not supported today**; this is a brief gap.
2. Practical sequence: schedule a 30-second maintenance window. Update `GITHUB_WEBHOOK_SECRET` in your deployment and on the GitHub App simultaneously, then restart.
3. Replay any missed deliveries from the GitHub App "Advanced" tab.

## Future improvements (tracked separately)

- `/metrics` Prometheus endpoint (roadmap Phase 12).
- Reminder editing via comment edit, recurring reminders (Phase 13).
- Multi-instance with distributed locking (Phase 14).

When any of these land, this runbook should be updated alongside the change.
