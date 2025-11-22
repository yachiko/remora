# Comment Formats and Specifications

## Overview

This document specifies all comment formats used in Remora: user commands for requesting reminders, reminder delivery comments, and error messages.

---

## User Command Format

### Basic Syntax

```
remora <time-expression> [timezone]
```

**Rules**:
- Command must start with `remora` (case-insensitive)
- Time expression is required
- Timezone is optional (defaults to UTC)
- Can appear anywhere in comment (Remora will detect it)
- First match only (multiple commands in one comment are ignored)

---

## Supported Time Expressions

### Relative Time

**Days/Weeks/Months**:
- `remora 2 days`
- `remora 3 weeks`
- `remora 1 month`
- `remora 1 week 3 days`
- `remora in 5 hours`

**Hours/Minutes**:
- `remora 30 minutes`
- `remora 2 hours`
- `remora in 45 minutes`

### Absolute Dates

**Day References**:
- `remora tomorrow`
- `remora next Monday`
- `remora next Friday`

**Specific Dates**:
- `remora December 25th`
- `remora Jan 1st`
- `remora March 15`

### Date + Time

**With Time**:
- `remora tomorrow at 3pm`
- `remora next Monday 9am`
- `remora December 25th at 2:30pm`
- `remora in 2 days at 10:00`

### With Timezone

**Timezone Abbreviations**:
- `remora 2 days PST`
- `remora tomorrow 3pm EST`
- `remora next Monday 9am CST`

**IANA Timezone Names**:
- `remora December 25th America/New_York`
- `remora next week Europe/London`
- `remora 3 days Asia/Tokyo`

**Default**: If no timezone specified, UTC is used

---

## Validation Rules

### Time Constraints

- **Minimum**: 15 minutes in the future (hardcoded)
- **Maximum**: 395 days / 13 months (hardcoded)
- **Must be future**: Past dates are rejected

### Examples

**Valid**:
```
✅ remora 2 days
✅ remora tomorrow at 3pm EST
✅ remora next Monday 9am
✅ remora December 25th
✅ remora in 30 minutes
✅ remora 1 year
```

**Invalid**:
```
❌ remora yesterday           (past date)
❌ remora 5 minutes           (< 15 min minimum)
❌ remora 2 years             (> 13 months maximum)
❌ remora                     (no time expression)
❌ @remora 2 days             (mentions not supported)
❌ remind me in 2 days        (wrong prefix)
❌ remora asap                (unparseable)
❌ remora sometime next week  (too vague)
```

---

## Reminder Delivery Comments

### Standard Format

When a reminder fires, Remora posts this comment:

```markdown
@{username} 🔔 Reminder!

You asked to be reminded about this issue.

Original request: {comment_url}
```

**Template Variables**:
- `{username}`: GitHub username of requester (e.g., `alice`)
- `{comment_url}`: Full URL to original comment where reminder was requested

**Example**:
```markdown
@alice 🔔 Reminder!

You asked to be reminded about this issue.

Original request: https://github.com/acme/project/issues/42#issuecomment-123456
```

---

### Overdue Reminder Format

If reminder fires late (after downtime/restart), annotate the delay:

```markdown
@{username} 🔔 Reminder!

You asked to be reminded about this issue {delay_description} ago (reminder was delayed).

Original request: {comment_url}
```

**Delay Descriptions**:
- `{delay_description}`: Human-readable delay (e.g., "2 hours", "45 minutes", "3 days")

**Examples**:
```markdown
@alice 🔔 Reminder!

You asked to be reminded about this issue 2 hours ago (reminder was delayed).

Original request: https://github.com/acme/project/issues/42#issuecomment-123456
```

```markdown
@bob 🔔 Reminder!

You asked to be reminded about this issue 15 minutes ago (reminder was delayed).

Original request: https://github.com/company/repo/issues/99#issuecomment-789012
```

**When to annotate**:
- Only if delay is significant (> 5 minutes)
- Skip annotation if fired within scheduler interval tolerance
- Reminders > 24 hours overdue are skipped entirely (not fired)

---

### Closed Issue Behavior

**Decision**: Post reminder comment regardless of issue/PR state (open/closed)

**Rationale**:
- User explicitly requested reminder
- Issue might have been closed prematurely
- User may want to revisit or reopen
- Configurable via `REMORA_POST_TO_CLOSED=true` (default)

**No special annotation for closed issues** - use standard reminder format.

---

## Error Comments

### Parse Error

Posted when time expression cannot be parsed (if `REMORA_ERROR_MODE=reaction_and_comment`):

```markdown
@{username} I couldn't parse your reminder request.

Please use the format: `remora <time-expression>`

Examples:
- remora 2 days
- remora tomorrow at 3pm
- remora next Monday 9am EST
- remora December 25th

For more help, see the documentation: {docs_url}
```

**When posted**:
- `olebedev/when` returns no match
- Unparseable time expression
- Malformed command

**Examples triggering this**:
- "remora asap"
- "remora sometime next week"
- "remora eventually"

---

### Past Date Error

Posted when parsed date is in the past:

```markdown
@{username} I couldn't set your reminder because the time is in the past.

You requested: `{original_command}`
Parsed as: {parsed_time} UTC

Please use a future date or time.

Examples:
- remora tomorrow
- remora in 2 hours
- remora next Monday 9am
```

**Template Variables**:
- `{original_command}`: User's original command text
- `{parsed_time}`: What the parser interpreted (for debugging)

---

### Too Soon Error

Posted when reminder is < 15 minutes in the future:

```markdown
@{username} I couldn't set your reminder because it's too soon.

Reminders must be at least 15 minutes in the future.

You requested: `{original_command}`

Please use a longer timeframe:
- remora 30 minutes
- remora 1 hour
- remora tomorrow
```

---

### Too Far Error

Posted when reminder is > 395 days (13 months) in the future:

```markdown
@{username} I couldn't set your reminder because it's too far in the future.

Reminders can be set up to 13 months (395 days) in advance.

You requested: `{original_command}`

Please use a shorter timeframe or set a reminder closer to the date.
```

---

### Generic Error

Posted when an unexpected error occurs:

```markdown
@{username} I encountered an error while setting your reminder.

Error: {error_message}

Please try again or contact support if the issue persists.
```

**When posted**:
- Database errors (rare)
- Unexpected parsing errors
- System errors

---

## Reaction-Only Mode

When `REMORA_ERROR_MODE=reaction_only` (default):

**No error comments are posted**. Only reactions are added:

- ✅ or 👀: Reminder accepted
- ❌: Error (any type)

**Rationale**:
- Reduces noise in issue threads
- User can still see something went wrong
- Suitable for high-traffic repositories

---

## Configuration

### Environment Variables

**Comment Behavior**:
- `REMORA_ERROR_MODE`: Error handling mode
  - `reaction_only` (default): Only add ❌ reaction on errors
  - `reaction_and_comment`: Add ❌ reaction + post error comment
  
- `REMORA_POST_TO_CLOSED`: Post reminders to closed issues
  - `true` (default): Always post
  - `false`: Skip closed issues

**Future Enhancement**:
- `REMORA_COMMENT_TEMPLATE`: Custom reminder comment template
  - Default: Standard format shown above
  - Supports variables: `{username}`, `{comment_url}`, `{delay_description}`

---

## Comment Length Limits

### GitHub Limits

- **Maximum comment length**: 65,536 characters
- **Remora's comments**: ~150-300 characters (well within limit)

### User Command Limits

**Question**: What if user's comment is very long (e.g., 50,000 characters) and "remora 2 days" appears at the end?

**Decision**: No limit enforced by Remora
- GitHub webhook payload includes full comment body
- Remora searches entire comment for "remora" prefix
- First match is processed regardless of position
- Extremely long comments (>1 MB) rejected by webhook payload validation

**Edge Case**: Comment is exactly at GitHub's 65,536 character limit
- Webhook delivers full payload
- Remora processes normally
- No special handling needed

---

## Multiple Commands in One Comment

**Decision**: First match only

**Example**:
```markdown
I need to remember this. remora 2 days

Also, remora 1 week
```

**Behavior**:
- Remora creates reminder for "remora 2 days" (first occurrence)
- "remora 1 week" is ignored
- Rationale: Simple, predictable, avoids accidental duplicates

**If user wants multiple reminders**:
- Post multiple comments
- Each comment can have one "remora" command

---

## Comment Editing

**Decision**: Ignore edits (from `edge-cases.md`)

**Scenario**: User posts "remora 2 days", then edits to "remora 3 days"

**Behavior**:
- Original reminder (2 days) persists unchanged
- Edited comment is ignored
- User must delete original comment and post new one to change reminder

**Rationale**:
- Simplicity: No need to listen for `issue_comment.edited` events
- Clear behavior: Reminders are locked once created
- Workaround exists: Delete and repost

---

## Comment Deletion

**Decision**: Cancel reminder (from `edge-cases.md`)

**Scenario**: User posts "remora 2 days", creates reminder, then deletes the comment

**Behavior**:
- Remora listens for `issue_comment.deleted` webhook event
- Finds reminder by `comment_id`
- Updates status to `cancelled`
- Reminder will not fire

**Rationale**:
- User control: Deleting comment signals intent to cancel
- Clean cancellation mechanism without extra commands
- Matches user expectations

---

## Case Sensitivity

**Decision**: Case-insensitive prefix

**All of these work**:
```
✅ remora 2 days
✅ Remora 2 days
✅ REMORA 2 days
✅ ReMoRa 2 days
```

**Rationale**:
- User-friendly
- Prevents errors from capitalization
- Easy to implement (lowercase comparison)

**Time expressions**: Case handled by `olebedev/when` library
- "tomorrow" and "Tomorrow" both work
- "DECEMBER 25TH" works
- Timezone names are case-insensitive ("EST", "est", "Est")

---

## Mentions

**Decision**: Mentions not supported

**Invalid**:
```
❌ @remora 2 days
❌ Hey @remora, remind me in 2 days
```

**Valid**:
```
✅ remora 2 days
✅ Hey, remora 2 days
```

**Rationale**:
- GitHub bots don't respond to @ mentions the same way as users
- Simpler to implement prefix-based detection
- Clearly documented limitation

**Future Enhancement**: Could add mention support if requested

---

## Examples in Context

### Typical Issue Comment

```markdown
This bug is critical but let's wait for the next release.

remora 2 weeks

We should revisit this after the release candidate is out.
```

**Result**: Reminder created for 2 weeks from now ✅

---

### Pull Request Comment

```markdown
This PR looks good but I'm on vacation next week.

Remora next Monday 9am PST

I'll review this when I'm back.
```

**Result**: Reminder created for next Monday at 9am PST ✅

---

### Multiple People in Thread

**User A**:
```markdown
We should revisit this issue. remora 1 month
```

**User B**:
```markdown
Good idea! remora 1 month
```

**Result**: 
- User A gets reminder in 1 month
- User B gets reminder in 1 month
- Two separate reminders created ✅

---

### Edge Case: Quoted Text

```markdown
> Someone said: "remora tomorrow"

I disagree, we should wait longer. remora 1 week
```

**Result**: 
- First match: "remora tomorrow" in quote
- Reminder created for tomorrow
- "remora 1 week" ignored (not first match)

**Note**: This is expected behavior (first match only). User should be careful with quoted text.

---

## Testing Recommendations

### Test Cases for Parser

1. **Valid commands**: All examples in "Supported Time Expressions"
2. **Invalid commands**: All examples in "Validation Rules"
3. **Edge cases**: 
   - Command at start of comment
   - Command at end of comment
   - Command in middle of long text
   - Multiple commands (first only)
   - Quoted commands
4. **Case variations**: "remora", "Remora", "REMORA"
5. **Timezone variations**: PST, EST, America/New_York, etc.
6. **Boundary values**: Exactly 15 minutes, exactly 395 days

### Test Cases for Comment Posting

1. **Standard reminder**: On-time delivery
2. **Overdue reminder**: Delayed by 30 minutes, 2 hours, 12 hours
3. **Very overdue**: > 24 hours (should be skipped)
4. **Error comments**: Each error type (if enabled)
5. **Closed issue**: Reminder on closed issue (if enabled)
6. **Reaction-only mode**: No error comments posted

---

## Future Enhancements

### Configurable Comment Templates

Allow custom reminder comment format:

```bash
REMORA_COMMENT_TEMPLATE="@{username} ping! {comment_url}"
```

### Custom Error Messages

Allow custom error messages per installation:

```bash
REMORA_ERROR_MESSAGE_URL="https://docs.example.com/remora"
```

### Reminder Editing via Commands

Support editing reminders with comment edits:

```
remora 2 days    # Original
remora 3 days    # Edit - should update reminder
```

Currently not supported, but could be added in Phase 2.

### Cancellation Command

Support explicit cancellation:

```
remora cancel
```

Currently requires deleting the original comment.

---

## Summary

This document specifies:
- ✅ User command syntax and validation
- ✅ Reminder delivery comment format (standard and overdue)
- ✅ Error comment formats (all error types)
- ✅ Configuration options (reaction modes, closed issues)
- ✅ Edge cases (multiple commands, editing, deletion, mentions)
- ✅ Testing recommendations
- ✅ Future enhancement ideas

All comment formats are production-ready and documented for implementation.
