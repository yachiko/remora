# Manual Smoke Test

End-to-end checklist for verifying Remora against a real GitHub App before launch. Run this whenever you deploy to a new environment, change webhook plumbing, upgrade the GitHub App permissions, or before tagging a release.

The integration suite (`test/integration/`) covers webhook → DB → scheduler with a mock GitHub server. This document covers the part the mock can't: signature delivery from real GitHub, real OAuth/JWT auth against `api.github.com`, and reactions/comments appearing on a real issue.

---

## Pre-flight

- [ ] A throwaway GitHub repo exists that you can post test comments on.
- [ ] A throwaway GitHub App is registered with the permissions from [spec/api-design.md](../spec/api-design.md):
      Issues = Read & Write, Pull Requests = Read & Write, Metadata = Read.
- [ ] The App is subscribed to the **Issue comment** event.
- [ ] The App is installed on the test repo.
- [ ] You have the App ID, the PEM private key, and the webhook secret.
- [ ] Remora is running and reachable at a public HTTPS URL. Options:
      - Local: `make run` + an ngrok tunnel pointing at `http://localhost:8080`.
      - Containerized: `docker compose -f deployments/docker/docker-compose.yml up --build`.
      - K8s: `kubectl apply -k deployments/kubernetes/` after editing the secret/configmap.
- [ ] The GitHub App webhook URL points at `<public-url>/webhook` and the
      webhook secret matches `GITHUB_WEBHOOK_SECRET`.
- [ ] `curl <public-url>/health` returns `200`.
- [ ] `curl <public-url>/ready` returns `200`.

## Scenarios

For each scenario, record what happened in the **Results** table at the bottom.

### 1 — Happy path (short delay)

1. Post a new issue comment containing exactly: `remora in 15 minutes`.
2. Within ~10 seconds: an 👀 (eyes) reaction should appear on your comment.
3. Wait 15 minutes (plus one scheduler tick — default 5 min).
4. A reminder comment should be posted on the issue, mentioning you.

### 2 — Cancellation via comment deletion

1. Post `remora in 30 minutes`.
2. Confirm the 👀 reaction.
3. Delete the comment.
4. Wait past the original delay. **No** reminder should be posted.
5. Check the DB (admin API or psql): the reminder row should have status `cancelled`.

### 3 — Past-date error

1. Post `remora yesterday`.
2. A ❌ (or 👎) reaction should appear on your comment.
3. If `REMORA_ERROR_MODE=reaction_and_comment`, an explanatory comment should also be posted.

### 4 — Too-soon error (< 15 min)

1. Post `remora in 5 minutes`.
2. Expected: ❌ reaction; if enabled, comment explaining the minimum is 15 minutes.

### 5 — Too-far error (> 13 months)

1. Post `remora in 14 months`.
2. Expected: ❌ reaction; if enabled, comment explaining the 13-month cap.

### 6 — Timezone parsing

1. Post `remora tomorrow at 9am CET`.
2. Confirm the 👀 reaction.
3. Check the DB: `remind_at` should be 09:00 in the Europe/Berlin offset, stored as UTC.

### 7 — Pull request comment

1. Open a PR, post `remora in 15 minutes` on it (top-level PR comment, not a review thread).
2. Expected behavior identical to scenario 1 — reminder fires on the PR.

### 8 — Closed-issue behavior

1. Post `remora in 15 minutes` on an open issue, then close the issue immediately.
2. Outcome depends on `REMORA_POST_TO_CLOSED`:
   - `true` (default): reminder still fires.
   - `false`: reminder is skipped, row marked `expired` or dropped.

### 9 — Webhook signature rejection

1. From the GitHub App settings, send a redelivery with a tampered body.
2. Expected: Remora returns 401 and logs "invalid webhook signature". No DB row created.

### 10 — Restart with overdue reminder

1. Stop Remora.
2. Post `remora in 5 minutes` from a fresh comment (it won't be processed yet).
   Or insert a reminder row manually with `remind_at` 1 hour in the past.
3. Restart Remora.
4. The reminder should fire on next scheduler tick with a "X hours late" annotation in the comment body (see [spec/comments.md](../spec/comments.md)).

---

## Results

Fill this in on each run. Keep the table in git so we have a record of which scenarios were last verified against which version.

| Date | Version (commit/tag) | Environment | Scenario | Pass/Fail | Notes |
|------|----------------------|-------------|----------|-----------|-------|
|      |                      |             | 1 — happy path |       |       |
|      |                      |             | 2 — cancellation |     |       |
|      |                      |             | 3 — past date |        |       |
|      |                      |             | 4 — too soon |         |       |
|      |                      |             | 5 — too far |          |       |
|      |                      |             | 6 — timezone |         |       |
|      |                      |             | 7 — PR comment |       |       |
|      |                      |             | 8 — closed issue |     |       |
|      |                      |             | 9 — bad signature |    |       |
|      |                      |             | 10 — overdue restart |  |       |

If any scenario fails, file an issue and link it from this table.
