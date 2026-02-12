# Astrocyte Messaging Module - Usage Guide

**Module:** `astrocyte_messaging.py`
**Purpose:** Centralized message sending with source attribution and logging
**Status:** Production-ready (implemented 2026-02-04)

---

## Table of Contents

1. [Quick Start](#quick-start)
2. [API Reference](#api-reference)
3. [Migration Guide](#migration-guide)
4. [Troubleshooting](#troubleshooting)
5. [Best Practices](#best-practices)

---

## Quick Start

### Basic Usage

```python
from astrocyte_messaging import send_tagged_message

# Send a diagnosis message
send_tagged_message(
    session_name="my-claude-session",
    message="System detected stuck state - investigating",
    message_type="diagnosis"
)

# Send a violation prompt
send_tagged_message(
    session_name="my-claude-session",
    message="Please use the correct tool instead of bash",
    message_type="violation_prompt"
)

# Send a notification
send_tagged_message(
    session_name="my-claude-session",
    message="Session recovered successfully",
    message_type="notification"
)
```

### What Happens

1. **Formatting:** Message wrapped in `<system-reminder>` block with metadata
2. **Validation:** Inputs checked (fail-fast on errors)
3. **Logging:** Send logged to `~/.agm/astrocyte/logs/messages.log`
4. **Sending:** Message sent via `csm send` command

### Message Format

Messages are delivered as:

```markdown
<system-reminder>
**This message is from Astrocyte Daemon** (automated monitoring system)

[Your message content here]

---
Source: astrocyte-daemon
Type: diagnosis
Session: my-claude-session
Timestamp: 2026-02-04T20:30:00Z
</system-reminder>
```

---

## API Reference

### `send_tagged_message(session_name, message, message_type)`

Send a tagged message to a Claude Code session.

**Parameters:**
- `session_name` (str): Target Claude session name (from `csm list`)
- `message` (str): Message content (can be multi-line, supports all Unicode)
- `message_type` (str): One of:
  - `"diagnosis"` - System diagnosis/investigation messages
  - `"violation_prompt"` - Tool usage violation prompts
  - `"notification"` - Status/recovery notifications

**Returns:** None

**Raises:**
- `ValueError`: Invalid inputs (empty message, invalid type, empty session)
- `subprocess.CalledProcessError`: `csm send` command failed

**Example:**

```python
try:
    send_tagged_message(
        session_name="stuck-session-123",
        message="Attempting ESC recovery...",
        message_type="diagnosis"
    )
    print("Message sent successfully")
except ValueError as e:
    print(f"Invalid inputs: {e}")
except subprocess.CalledProcessError as e:
    print(f"Send failed: {e}")
```

---

## Migration Guide

### Updating Existing Send Functions

**Before (direct `csm send` call):**

```python
def send_diagnosis_prompt_via_csm(session_name: str, prompt: str) -> bool:
    try:
        # Write prompt to temp file
        prompt_file = Path.home() / ".agm/astrocyte/prompts" / f"{session_name}-diagnosis.txt"
        with open(prompt_file, "w") as f:
            f.write(prompt)

        # Send via csm
        result = subprocess.run(
            ["csm", "send", session_name, "--prompt-file", str(prompt_file)],
            check=True
        )
        return result.returncode == 0
    except Exception:
        return False
```

**After (delegate to wrapper):**

```python
def send_diagnosis_prompt_via_csm(session_name: str, prompt: str) -> bool:
    try:
        from astrocyte_messaging import send_tagged_message
        send_tagged_message(session_name, prompt, "diagnosis")
        return True
    except Exception as e:
        print(f"Failed to send: {e}", file=sys.stderr)
        return False
```

**Changes:**
- ✅ Backward compatible (same function signature)
- ✅ Automatic tagging (no code changes needed in callers)
- ✅ Automatic logging (audit trail created)
- ✅ Cleaner code (20 lines → 7 lines)

### Migration Checklist

1. ✅ **Phase 1:** Wrapper module created (`astrocyte_messaging.py`)
2. ✅ **Phase 2:** Existing functions updated (3 functions)
3. ⬜ **Phase 3:** Update direct callers (if any)
4. ⬜ **Phase 4:** Add linting enforcement (pre-commit hook)

**Current Status:** Phase 2 complete (all existing functions use wrapper)

---

## Troubleshooting

### Error: "Message cannot be empty"

**Cause:** Empty or whitespace-only message

**Fix:**

```python
# ❌ Wrong
send_tagged_message("session", "", "diagnosis")
send_tagged_message("session", "   ", "diagnosis")

# ✅ Correct
send_tagged_message("session", "Valid message", "diagnosis")
```

---

### Error: "Invalid message type: warning"

**Cause:** Message type not in allowed list

**Fix:**

```python
# ❌ Wrong
send_tagged_message("session", "Warning!", "warning")

# ✅ Correct (use one of: diagnosis, violation_prompt, notification)
send_tagged_message("session", "Warning!", "notification")
```

**Valid types:**
- `diagnosis` - Investigation/diagnostic messages
- `violation_prompt` - Tool usage violation prompts
- `notification` - Status/recovery notifications

---

### Error: "Message missing attribution tag"

**Cause:** Direct call to `_send_via_csm()` with untagged message (internal API)

**Fix:** Use public API `send_tagged_message()` instead

```python
# ❌ Wrong (internal API, bypasses validation)
from astrocyte_messaging import _send_via_csm
_send_via_csm("session", "Untagged message")

# ✅ Correct (public API, enforces tagging)
from astrocyte_messaging import send_tagged_message
send_tagged_message("session", "Tagged message", "diagnosis")
```

---

### Warning: "Failed to log message: ..."

**Cause:** Log directory write failure (disk full, permissions)

**Impact:** Message is still sent (fail-safe behavior)

**Fix:**

1. Check disk space: `df -h ~/.agm/astrocyte/logs`
2. Check permissions: `ls -ld ~/.agm/astrocyte/logs`
3. Fix permissions: `chmod 0700 ~/.agm/astrocyte/logs`

**Note:** This warning is fail-safe - the message is still delivered. Logging is best-effort.

---

### Error: subprocess.CalledProcessError (csm send failed)

**Cause:** `csm send` command failed (session not found, tmux issue)

**Fix:**

1. Verify session exists: `csm list | grep session-name`
2. Verify tmux session: `tmux ls | grep session-name`
3. Check csm send manually: `csm send session-name --prompt "test"`

**Common causes:**
- Session name misspelled
- Session already terminated
- Tmux server not running

---

## Best Practices

### 1. Choose Correct Message Type

**Diagnosis:** System investigation, debugging info
```python
send_tagged_message(session, "Detected stuck state: pane has 5+ queued inputs", "diagnosis")
```

**Violation Prompt:** Tool usage corrections
```python
send_tagged_message(session, "Please use Read tool instead of cat", "violation_prompt")
```

**Notification:** Status updates, recovery notifications
```python
send_tagged_message(session, "Session recovered via ESC sequence", "notification")
```

### 2. Multi-Line Messages

Use triple quotes for multi-line content:

```python
violation_message = """You used bash chaining (cd && command).

Claude Code policy:
- Use absolute paths instead: git -C /path status
- Avoid cd: it triggers permission prompts

Please retry without chaining."""

send_tagged_message(session, violation_message, "violation_prompt")
```

### 3. Special Characters

All Unicode is supported (quotes, brackets, symbols):

```python
message = """Special chars test:
- Quotes: "double" 'single'
- Brackets: [square] {curly}
- Unicode: ✓ ✗ → ←"""

send_tagged_message(session, message, "notification")
```

### 4. Error Handling

Always wrap sends in try-except for production code:

```python
try:
    send_tagged_message(session, message, "diagnosis")
    # Success - continue with recovery
    attempt_recovery(session)
except ValueError as e:
    # Configuration error - log and skip
    print(f"Config error: {e}", file=sys.stderr)
except subprocess.CalledProcessError as e:
    # Send failed - session might be dead
    print(f"Send failed: {e}", file=sys.stderr)
    mark_session_as_failed(session)
```

### 5. Large Messages (>10KB)

Wrapper automatically handles large messages via temp file:

```python
# Large diagnostic report (15KB)
large_report = generate_diagnostic_report()  # Returns 15KB string

# Automatically uses --prompt-file (you don't need to do anything)
send_tagged_message(session, large_report, "diagnosis")
```

**Performance:**
- Small messages (<10KB): ~15ms overhead (format + validate + log)
- Large messages (≥10KB): ~50ms overhead (includes temp file write)

---

## Log File Reference

**Location:** `~/.agm/astrocyte/logs/messages.log`

**Permissions:** 0600 (owner read/write only)

**Rotation:** 10MB rotation, 5 backups (messages.log.1 through messages.log.5)

**Format:**

```
2026-02-04 20:30:00 UTC - astrocyte.messaging - INFO - SEND session=my-session type=diagnosis length=42 hash=a1b2c3d4
```

**Fields:**
- Timestamp (UTC)
- Logger name (astrocyte.messaging)
- Level (INFO)
- Session name
- Message type
- Message length (bytes)
- Message hash (first 8 chars of SHA-256)

**Query Examples:**

```bash
# View recent sends
tail -20 ~/.agm/astrocyte/logs/messages.log

# Find all diagnosis messages
grep "type=diagnosis" ~/.agm/astrocyte/logs/messages.log

# Find messages to specific session
grep "session=my-session" ~/.agm/astrocyte/logs/messages.log

# Count sends by type
grep -o "type=[^[:space:]]*" ~/.agm/astrocyte/logs/messages.log | sort | uniq -c
```

---

## Additional Resources

- **Architecture Decision Record:** `docs/ADR-001-message-attribution-and-logging.md`
- **Unit Tests:** `test_astrocyte_messaging.py`
- **Integration Tests:** `test_integration_astrocyte_messaging.py`
- **BDD Scenarios:** `features/*.feature`
- **Module Source:** `astrocyte_messaging.py`

---

**Last Updated:** 2026-02-04
**Module Version:** 1.0.0
