# Edge Cases and Open Questions

## Overview

This document captures edge cases, unusual scenarios, and open questions that need to be addressed during implementation or in future iterations.

---

## 1. Closed/Merged Issues and Pull Requests

### Scenario
A reminder fires, but the issue or PR has been closed or merged.

### Questions
- Should Remora still post the reminder comment?
- Should Remora check the issue/PR status before posting?
- Should Remora silently skip closed issues or notify the user?

### Proposed Solutions

**Option A: Always Post (Simple)**
- Post the reminder regardless of issue/PR status
- **Rationale**: User asked for a reminder, they get it
- **Pro**: Simple, predictable
- **Con**: May clutter closed issues

**Option B: Check and Skip (Smart)**
- Check issue/PR status before posting
- Skip if closed, mark reminder as "skipped"
- **Rationale**: No point reminding about closed issues
- **Pro**: Cleaner, respects issue state
- **Con**: Extra GitHub API call, user might still want reminder

**Option C: Configurable (Flexible)**
- Environment variable: `REMORA_POST_TO_CLOSED` (default: `true`)
- **Rationale**: Let admins decide per installation
- **Pro**: Flexibility
- **Con**: More configuration

**Decision**: Option C (Configurable) - Default: always post (`REMORA_POST_TO_CLOSED=true`)

---

## 2. Overdue Reminders (After Restart)

### Scenario
Remora was down or restarted. Multiple reminders are now overdue (e.g., 10 minutes to 3 hours past due time).

### Questions
- Should Remora fire all overdue reminders immediately?
- Should Remora skip very old reminders (e.g., > 24 hours overdue)?
- Should Remora batch post them or rate limit?

### Proposed Solutions

**Option A: Fire All Immediately**
- On startup, fire all pending reminders with `remind_at <= NOW()`
- **Pro**: Honors user intent, simple logic
- **Con**: Could spam GitHub with many comments at once

**Option B: Fire with Delay**
- Fire overdue reminders with 5-10 second delays between each
- **Pro**: Avoids sudden spam
- **Con**: Adds complexity, still could be many

**Option C: Skip Very Old + Fire Recent**
- Skip reminders > 24 hours overdue (mark as "expired")
- Fire reminders < 24 hours overdue
- **Pro**: Reasonable compromise
- **Con**: Arbitrary cutoff

**Option D: Fire with Grace Period Annotation**
- Post all overdue reminders but annotate how late they are
- Example: "@username reminder from [link] (reminder was 2 hours late)"
- **Pro**: User aware of delay
- **Con**: More complex comment formatting

**Decision**: Option C (skip > 24 hours, fire recent) + Option D (annotate delay)

**Configuration**:
```bash
REMORA_MAX_OVERDUE_HOURS=24  # Skip reminders older than this (not configurable in Phase 1)
```

---

## 3. Rate Limiting

### Scenario
Multiple reminders fire at the same time, or GitHub API rate limits are approached.

### Questions
- How to handle GitHub API rate limits?
- Should Remora batch reminder deliveries?
- What happens if rate limit is hit?

### Proposed Solutions

**GitHub API Rate Limits**:
- GitHub Apps: 5000 requests/hour per installation
- Track remaining quota via `X-RateLimit-Remaining` header

**Rate Limit Strategy**:
1. **Check Before Posting**: Check rate limit before each comment/reaction
2. **Exponential Backoff**: If rate limited (429), wait and retry
3. **Mark as Failed**: After 3 retries, mark reminder as "failed" with error message
4. **Resume Later**: Scheduler will retry failed reminders on next cycle

**Decision**: No explicit rate limiting or batch processing configuration. Let GitHub API fail naturally if rate limits exceeded. Only API usage is posting comments/reactions, which should be well within limits for typical use.

---

## 4. Duplicate Reminders

### Scenario
User posts multiple "remora X" commands on the same issue.

### Questions
- Should Remora allow multiple reminders for the same user on the same issue?
- Should Remora deduplicate based on time proximity?

### Proposed Solutions

**Option A: Allow All (Permissive)**
- Create a reminder for every "remora" command
- **Pro**: User control, might want multiple reminders
- **Con**: Could clutter

**Option B: Deduplicate Exact Matches**
- If user requests "remora 2 days" twice within 1 minute, ignore second
- **Pro**: Prevents accidental duplicates
- **Con**: Complex logic, edge cases

**Option C: No Deduplication, Rely on User**
- Users responsible for not posting duplicates
- **Pro**: Simple
- **Con**: User error possible

**Decision**: Option A (allow all). Users can delete comments if they made a mistake.

---

## 5. Deleted Comments

### Scenario
User posts "remora 2 days", Remora creates reminder, then user deletes the original comment.

### Questions
- Should Remora cancel the reminder?
- Should Remora listen for `issue_comment.deleted` webhook events?

### Proposed Solutions

**Option A: Keep Reminder**
- Reminder persists even if comment deleted
- **Pro**: Simple, no extra webhook handling
- **Con**: No way to cancel via deletion

**Option B: Cancel on Delete**
- Listen for `issue_comment.deleted` events
- If deleted comment had a reminder, mark as "cancelled"
- **Pro**: User can cancel by deleting comment
- **Con**: More complexity, need to track comment_id

**Decision**: Option B (Cancel on Delete) - Listen for `issue_comment.deleted` webhook events and mark reminders as "cancelled".

---

## 6. Edited Comments

### Scenario
User posts "remora 2 days", then edits comment to "remora 3 days".

### Questions
- Should Remora update the reminder?
- Should Remora create a new reminder?
- Should Remora ignore edits?

### Proposed Solutions

**Option A: Ignore Edits**
- Reminder locked once created
- **Pro**: Simple, predictable
- **Con**: User can't fix mistakes

**Option B: Update on Edit**
- Listen for `issue_comment.edited` events
- If "remora" command changed, update reminder time
- **Pro**: User can correct mistakes
- **Con**: Complex, need to handle various edit scenarios

**Option C: Delete Old, Create New**
- On edit, delete old reminder and create new one
- **Pro**: Clean slate
- **Con**: Lose audit trail

**Decision**: Option A (ignore edits). Users can delete comment and post new one to change reminder.

---

## 7. Repository/Organization Deletion

### Scenario
GitHub App installed on a repo/org that later gets deleted.

### Questions
- What happens to pending reminders?
- Should Remora clean up data?

### Proposed Solutions

**Option A: Leave Data**
- Reminders remain in database
- When scheduler tries to fire, GitHub API returns 404
- Mark as "failed" with error message
- **Pro**: Simple
- **Con**: Orphaned data

**Option B: Listen for Deletion Events**
- Subscribe to `repository.deleted` webhook
- Delete all reminders for that repository
- **Pro**: Clean database
- **Con**: More webhooks to handle

**Option C: Periodic Cleanup Job**
- Daily job checks for failed reminders with 404 errors
- Delete reminders for deleted repos
- **Pro**: Automated cleanup
- **Con**: Delayed

**Decision**: Option A initially + Option C (periodic cleanup) - accepted

---

## 8. Permission Changes

### Scenario
GitHub App loses permissions (e.g., comment permission revoked, app uninstalled).

### Questions
- What happens to pending reminders?
- Should Remora detect and handle this?

### Proposed Solutions

**Handle at Execution Time**:
- When scheduler tries to post comment, GitHub API returns 403 (Forbidden)
- Mark reminder as "failed" with error: "Permission denied"
- Retry a few times (in case temporary issue)
- Eventually give up

**Listen for Installation Events**:
- `installation.deleted` - App uninstalled, cancel all reminders for that installation
- `installation.suspend` - App suspended, pause reminders
- **Pro**: Proactive
- **Con**: More complexity

**Decision**: Handle at execution time - log errors, mark reminders as "failed" with error message. No installation event handling in Phase 1.

---

## 9. Time Zone Edge Cases

### Scenario
User says "remora tomorrow at 9am EST" but it's currently 11pm EST (tomorrow is in 2 hours).

### Questions
- Is "tomorrow" the calendar next day, or 24 hours from now?
- How does `olebedev/when` interpret this?

### Proposed Solutions

**Decision**: Trust the library - `olebedev/when` handles relative dates. Extensive testing will validate various user inputs.

**Validation**:
- Ensure parsed time is at least 15 minutes in the future
- Reject if not: "Reminder must be at least 15 minutes in the future"

---

## 10. Very Long Timeframes

### Scenario
User posts "remora 5 years from now" or "remora December 31, 2030".

### Questions
- Should Remora support reminders years in the future?
- Should there be a maximum timeframe?

### Proposed Solutions

**Option A: No Limit**
- Allow any future date
- **Pro**: Maximum flexibility
- **Con**: Database clutter, unlikely to be useful

**Option B: Enforce Maximum**
- Reject reminders > 1 year in the future
- Return error reaction and comment
- **Pro**: Reasonable limit
- **Con**: Arbitrary restriction

**Decision**: Option B - Max 13 months (395 days), not configurable.

```bash
# Hardcoded: 395 days maximum
```

---

## 11. Minimum Timeframe

### Scenario
User posts "remora 1 minute" or "remora now".

### Questions
- Should there be a minimum timeframe?
- What's the point of immediate reminders?

### Proposed Solutions

**Decision**: Enforce minimum of 15 minutes in the future (not configurable).
- **Rationale**: Gives scheduler time to process, prevents immediate/impractical reminders

```bash
# Hardcoded: 15 minutes minimum
```

---

## 12. Multiple Time Expressions in One Comment

### Scenario
User posts "remora 2 days and also remora 1 week".

### Questions
- Should Remora create multiple reminders?
- Should Remora only parse the first occurrence?

### Proposed Solutions

**Option A: First Match Only**
- Parse first "remora" command, ignore rest
- **Pro**: Simple, clear
- **Con**: User might expect multiple

**Option B: All Matches**
- Create reminder for each "remora" command
- **Pro**: Maximum flexibility
- **Con**: Complex parsing, might be unintended

**Decision**: Option A (first match only). If user wants multiple reminders, post multiple comments.

---

## 13. Malformed or Ambiguous Commands

### Scenario
User posts "remora sometime next week" or "remora asap".

### Questions
- How to handle unparseable time expressions?
- What error message to show?

### Proposed Solutions

**Decision**: Handle as standard error
1. `olebedev/when` returns nil (no match)
2. Add ❌ reaction to comment
3. Post error comment (same as other errors)

**Error Message** (if posting comment):
```
@username I couldn't parse your reminder request.

Please use the format: `remora <time-expression>`

Examples:
- remora 2 days
- remora tomorrow at 3pm
- remora next Monday 9am EST
- remora December 25th

[View documentation](link)
```

---

## 14. Scheduler Polling Interval

### Scenario
Scheduler checks for due reminders every N minutes.

### Questions
- What's the optimal polling interval?
- Trade-offs between accuracy and database load?

### Proposed Solutions

**Polling Intervals**:
- **1 minute**: Most accurate, higher DB load
- **5 minutes**: Good balance
- **10 minutes**: Lower load, less accurate

**Decision**: 5 minutes default (configurable via `REMORA_SCHEDULER_INTERVAL`)

**Configuration**:
```bash
REMORA_SCHEDULER_INTERVAL=5  # Minutes between scheduler runs
```

**Accuracy Impact**:
- 5-minute interval means reminders could be up to 5 minutes late
- Acceptable for most use cases (e.g., "remora 2 days")
- Less acceptable for precise times (e.g., "remora 3:00pm")

---

## 15. Reminder Delivery Failures

### Scenario
Scheduler tries to post reminder, but GitHub API fails (network issue, API down, etc.).

### Questions
- How many times to retry?
- How long to wait between retries?
- When to give up?

### Proposed Solutions

**Decision**: Retry with exponential backoff
1. **Exponential Backoff**: 1 min, 2 min, 4 min, 8 min, 16 min
2. **Max Retries**: 5 attempts (hardcoded, not configurable)
3. **Mark as Failed**: After 5 attempts, mark reminder as "failed"
4. **Store Error**: Save error message for debugging

**Database Fields**:
- `retry_count` - Number of retry attempts
- `error_message` - Last error encountered
- `status` - `pending`, `processing`, `fired`, `failed`

```bash
# Hardcoded: 5 max retries with exponential backoff
```

---

## 16. Database Connection Loss

### Scenario
Scheduler is running but database connection is lost.

### Questions
- Should scheduler crash?
- Should scheduler retry connection?

### Proposed Solutions

**Connection Retry**:
- GORM automatically handles connection pooling and retries
- On connection error, log error and skip that scheduler cycle
- Health check reports unhealthy
- Kubernetes/Docker will restart container if health check fails repeatedly

**Decision**: Graceful degradation during runtime
- **Startup**: If initial database connection fails, container exits with error code
- **Runtime**: Webhook handler returns 500 if database unavailable
- Scheduler logs error but continues running
- Health endpoint returns 503

---

## 17. Concurrent Scheduler Instances

### Scenario
Multiple Remora instances running (e.g., for high availability).

### Questions
- How to prevent duplicate reminder deliveries?
- How to ensure only one instance processes each reminder?

### Proposed Solutions

**Single Instance (Current Design)**:
- Deploy single instance only
- Simple, no coordination needed

**Multi-Instance (Future Enhancement)**:
- Use database locking or distributed locks (Redis)
- Scheduler queries for `status = 'pending'`
- Immediately updates to `status = 'processing'` in same transaction
- Only one instance gets the reminder
- **Implementation**: Use `UPDATE ... WHERE status = 'pending' RETURNING *` pattern

**Decision**: Single instance only - no multi-instance support planned.

**Enforcement**: Document that only one instance should be deployed. No technical enforcement mechanism in Phase 1. Running multiple instances will result in duplicate reminder deliveries.

---

## 18. GitHub API Webhook Delivery Failures

### Scenario
GitHub fails to deliver webhook (network issue, timeout, etc.).

### Questions
- What happens to missed comments?
- Should Remora poll for missed comments?

### Proposed Solutions

**Decision**: Trust GitHub's webhook retry mechanism
- GitHub automatically retries webhook deliveries with exponential backoff for up to 3 days
- Accept that some webhooks might be permanently lost (rare edge case)
- No polling mechanism (too complex, high API usage)
- Document limitation

---

## 19. Installation on Private vs Public Repositories

### Scenario
GitHub App installed on private repository.

### Questions
- Any special handling needed?
- Permission differences?

### Proposed Solutions

**Decision**: No special handling - Remora works the same for private and public repos. GitHub App authentication handles all permissions.

---

## 20. Cross-Repository Reminders

### Scenario
User wants reminder on different issue: "remora 2 days on #123"

### Questions
- Should this be supported?
- How to parse cross-repo references?

### Proposed Solutions

**Phase 1: Same Issue Only**
- Reminder always fires on the same issue/PR where command was posted
- Simplest implementation

**Phase 2: Cross-Issue Support (Future)**
- Parse "on #123" syntax
- Fire reminder on specified issue in same repository
- Example: "remora 2 days on #456"

**Phase 3: Cross-Repo Support (Future)**
- Parse "on owner/repo#123" syntax
- Fire reminder on issue in different repository
- Requires checking GitHub App installation permissions

**Decision**: Phase 1 only (same issue/PR). Cross-issue and cross-repo reminders not planned.

---

## 21. Natural Language Parsing Errors

### Scenario
`olebedev/when` parses time expression but result is unexpected.

### Questions
- How to detect incorrect parses?
- Should Remora validate parsed results?

### Proposed Solutions

**Decision**: Validate parsed results
1. Parsed time must be in the future
2. Parsed time must be within max timeframe (13 months / 395 days)
3. Parsed time must be at least 15 minutes ahead

**On Validation Failure**:
- Add ❌ reaction
- Post error comment with specific failure reason
- Do not create reminder

---

## 22. User Mentions in Comments

### Scenario
User posts "@remora 2 days" instead of "remora 2 days".

### Questions
- Should mentions work as trigger?
- GitHub doesn't support bot mentions the same way as user mentions

### Proposed Solutions

**Decision**: Do not support mentions - only trigger on "remora" prefix (no @). Clearly document this behavior.

---

## Implementation Priority

### Must Implement (Phase 1)
1. ✅ Overdue reminders handling (skip > 24 hours, annotate delay)
2. ✅ Retry logic with exponential backoff (5 attempts)
3. ✅ Validation (future dates, 15 min minimum, 395 days maximum)
4. ✅ Malformed command error handling (reaction + comment)
5. ✅ GitHub API error handling (log and mark as failed)
6. ✅ Deleted comment handling (cancel reminder)
7. ✅ Closed issue/PR behavior (configurable, default: always post)
8. ✅ Database connection failure on startup (exit with error)
9. ✅ Allow duplicate reminders (no deduplication)
10. ✅ Ignore edited comments
11. ✅ First match only for multiple commands
12. ✅ No @ mention support

### Should Implement (Phase 2)
13. Database cleanup for deleted repos (periodic job)
14. Installation/permission change events
15. Enhanced error messages with documentation links

### Not Planned
16. ❌ Cross-repository reminders
17. ❌ Multi-instance support
18. ❌ Rate limiting configuration (rely on natural API limits)
19. ❌ Configurable retry attempts or backoff
20. ❌ Configurable min/max timeframes

---

## Configuration Summary

Environment variables for edge case handling:

```bash
# Configurable
REMORA_SCHEDULER_INTERVAL=5       # Minutes between scheduler runs
REMORA_POST_TO_CLOSED=true        # Post reminders to closed issues/PRs
REMORA_ERROR_MODE=reaction_and_comment  # Always post error comments

# Hardcoded (not configurable)
# - Max reminder: 395 days (13 months)
# - Min reminder: 15 minutes
# - Max overdue: 24 hours (skip older)
# - Max retry attempts: 5
# - Retry backoff: exponential (1, 2, 4, 8, 16 minutes)
```
