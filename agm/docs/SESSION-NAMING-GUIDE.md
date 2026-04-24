# Session Naming Guide

## Overview

Choosing good session names is important for AGM to work reliably. This guide explains which characters are safe to use and which ones can cause problems.

## Quick Reference

### ✅ Safe Characters (Always OK)

- **Letters**: `a-z`, `A-Z`
- **Numbers**: `0-9`
- **Dash**: `-`
- **Underscore**: `_`

### ❌ Unsafe Characters (Avoid These)

- **Dots**: `.` → converted to dashes by tmux
- **Colons**: `:` → converted to dashes by tmux
- **Spaces**: ` ` → converted to dashes by tmux

## Examples

### Good Session Names

```bash
agm new my-project            # ✅ Dashes
agm new coding_session        # ✅ Underscores
agm new project123            # ✅ Alphanumeric
agm new task-2024-refactor    # ✅ Mixed safe characters
```

### Problematic Session Names

```bash
agm new gemini-task-1.4       # ❌ Contains dots
# tmux will convert to: gemini-task-1-4
# This causes lookup failures!

agm new project:staging       # ❌ Contains colons
# tmux will convert to: project-staging

agm new my session            # ❌ Contains spaces
# tmux will convert to: my-session
```

## Why This Matters

### The BUG-001 Incident

On 2026-02-19, a session named `gemini-task-1.4` caused 40% message delivery failure during multi-session coordination.

**What happened:**

1. AGM stored the session name as: `gemini-task-1.4`
2. tmux automatically converted it to: `gemini-task-1-4`
3. AGM tried to find session `gemini-task-1.4` but tmux only knew about `gemini-task-1-4`
4. Result: Session lookups failed, messages couldn't be delivered

### How tmux Normalizes Names

When you create a tmux session with unsafe characters, tmux silently converts them to dashes:

```bash
# You type:
tmux new-session -s "project.v1:test"

# tmux creates:
# session name: "project-v1-test"
```

This normalization is automatic and **cannot be disabled**.

## AGM's Protection

Starting with Phase 2 of BUG-001 fixes, AGM now:

1. **Detects** unsafe characters when you create sessions
2. **Warns** you about the problem
3. **Suggests** a safe alternative name
4. **Allows override** if you need to use an existing session

### Interactive Warning Example

```bash
$ agm new gemini-task-1.4

❌ Session name 'gemini-task-1.4' contains characters that tmux will normalize

⚠️  Session name contains dots (.) which tmux converts to dashes (-)

💡 Suggested name: 'gemini-task-1-4'

Safe characters: alphanumeric (a-z, A-Z, 0-9), dash (-), underscore (_)
Unsafe characters: dots (.), colons (:), spaces

Background: tmux automatically converts unsafe characters to dashes,
which can cause session lookup failures and message delivery issues.

Use the suggested name to avoid these problems.

? Session name contains unsafe characters. What would you like to do?
  > Use suggested name: 'gemini-task-1-4'
    Continue with 'gemini-task-1.4' anyway (not recommended)
    Cancel and choose a different name
```

## Best Practices

### 1. Use Dashes for Word Separation

```bash
agm new my-coding-session     # ✅ Good
agm new my.coding.session     # ❌ Avoid
```

### 2. Use Underscores for Technical Names

```bash
agm new test_environment      # ✅ Good
agm new test:environment      # ❌ Avoid
```

### 3. Version Numbers

```bash
agm new project-v1-2-3        # ✅ Good (dashes)
agm new project_v1_2_3        # ✅ Good (underscores)
agm new project-v1.2.3        # ❌ Avoid (dots)
```

### 4. Environment Indicators

```bash
agm new api-staging           # ✅ Good (dash)
agm new api_staging           # ✅ Good (underscore)
agm new api:staging           # ❌ Avoid (colon)
```

### 5. Multi-Word Names

```bash
agm new deep-research-task    # ✅ Good (dashes)
agm new deep_research_task    # ✅ Good (underscores)
agm new deep research task    # ❌ Avoid (spaces)
```

## Migration Guide

### If You Have Existing Sessions with Unsafe Names

AGM will continue to work with existing sessions that have unsafe names. However:

1. **For lookups**, AGM automatically normalizes the name to match tmux
2. **For new operations**, you'll see warnings
3. **For consistency**, consider renaming sessions

#### How to Rename a Session

```bash
# 1. Check current sessions
agm list

# 2. Create new session with safe name
agm new project-v1-2-3

# 3. Migrate your work to the new session

# 4. Archive old session (optional)
# Sessions are stored in: ~/.agm/sessions/
```

## Technical Details

### Normalization Function

AGM uses `NormalizeTmuxSessionName()` to match tmux's behavior:

```go
func NormalizeTmuxSessionName(name string) string {
    name = strings.ReplaceAll(name, ".", "-")
    name = strings.ReplaceAll(name, ":", "-")
    name = strings.ReplaceAll(name, " ", "-")
    return name
}
```

### Where Normalization Happens

1. **Session lookups**: `tmux.HasSession()`, `tmux.AttachSession()`
2. **Command sending**: `tmux.SendCommand()`
3. **Session queries**: `tmux.ListClients()`, `tmux.GetCurrentWorkingDirectory()`

### Validation During Session Creation

When you run `agm new`, AGM calls `ValidateSessionName()` which:

1. Checks for dots, colons, and spaces
2. Generates warnings with detailed explanations
3. Suggests a normalized alternative
4. Prompts for user confirmation

## Troubleshooting

### "Session not found" errors

If you see errors like:

```
Error: session 'my-project-1.4' not found
```

But you know the session exists, the issue is likely name normalization:

```bash
# Check what tmux actually has:
tmux list-sessions

# You'll probably see:
# my-project-1-4: ...
# (note the dashes instead of dots)

# Use the normalized name:
agm resume my-project-1-4
```

### Preventing Future Issues

```bash
# Always use safe characters:
agm new my-project-v1-4-2     # ✅ Good

# AGM will warn you if you try:
agm new my-project-v1.4.2     # ⚠️  Warning + suggestion
```

## FAQ

### Q: Can I disable these warnings?

A: No. The warnings are there to prevent data loss and message delivery failures. However, you can choose to "Continue anyway" when prompted.

### Q: What about existing sessions with unsafe names?

A: They will continue to work. AGM automatically normalizes lookups to match tmux. You'll only see warnings when creating new sessions or associating existing ones.

### Q: Why doesn't AGM just fix the names automatically?

A: We want to give you control. The suggested name is just that - a suggestion. You can choose to use it or override it.

### Q: Are there other unsafe characters besides dots, colons, and spaces?

A: Currently, these are the main ones we've identified. If you discover others that tmux normalizes, please report them!

### Q: Can I use Unicode characters?

A: Technically yes, but they're not recommended. Stick to ASCII alphanumeric, dashes, and underscores for maximum compatibility.

## See Also

- [BUG-001 Documentation](../CSM-BUG-FIX-REPORT.md) - Full incident report
- [User Guide](USER-GUIDE.md) - General AGM usage
- [Troubleshooting Guide](TROUBLESHOOTING.md) - Common issues and solutions

## Version History

- **Phase 1** (2026-02-19): Added `NormalizeTmuxSessionName()` for lookups
- **Phase 2** (2026-02-19): Added validation warnings on session creation
