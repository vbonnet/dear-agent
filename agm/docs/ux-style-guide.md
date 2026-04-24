# AGM UX Style Guide

**Version**: 1.0
**Last Updated**: 2026-01-24

## Overview

This guide defines consistent user experience patterns across all AGM CLI commands.

## Flag Naming Conventions

### Standard Flags

All commands SHOULD support these standard flags:

- `--help` / `-h`: Display command help
- `--debug` / `-d`: Enable debug logging
- `--yes` / `-y`: Skip confirmation prompts (automation mode)
- `--format`: Output format (table, json, yaml)
- `--output` / `-o`: Output file path

### Naming Rules

1. **Use kebab-case** for multi-word flags: `--session-name`, not `--sessionName`
2. **Single-letter shortcuts** for common flags: `-d` for `--debug`, `-o` for `--output`
3. **Consistent names** across commands:
   - Session identifier: `--session`, not `--id` or `--name`
   - Filter: `--filter`, not `--query` or `--search`
   - Time range: `--since`, `--until`

### Examples

```bash
# Good
agm list --format table --filter active --since 1h

# Bad
agm list --outputFormat table --q active --from 1h
```

## Output Formatting

### Success Messages

Use green checkmark prefix:

```
✅ Session created successfully
✅ Phase S8 completed
```

### Error Messages

Use red X prefix and follow template:

```
❌ Error: [Short description]

Context: [Why this error occurred]

To fix:
- [Actionable step 1]
- [Actionable step 2]

For more info: [doc link or command]
```

### Warning Messages

Use yellow triangle prefix:

```
⚠️  Warning: Session has uncommitted changes
```

### Informational Messages

Use blue info icon:

```
ℹ️  No sessions found matching filter
```

## Table Formatting

### Headers

- Use Title Case for column headers
- Align headers with column content
- Add separator line under headers

### Example

```
Session Name     Status    Created
──────────────────────────────────
my-project       active    2h ago
bug-fix-123      stopped   1d ago
```

## Lists

### Bulleted Lists

Use hyphens for unordered items:

```
Available commands:
- new: Create a new session
- list: List all sessions
- attach: Attach to a session
```

### Numbered Lists

Use numbers for ordered steps:

```
To fix:
1. Stop the session
2. Edit the configuration
3. Restart the session
```

## Command Behavior

### Help Text

All commands MUST:
- Support `--help` flag
- Show usage examples
- Document all flags
- Provide See Also section for related commands

### Exit Codes

- `0`: Success
- `1`: General error
- `2`: Invalid arguments
- `3`: Resource not found
- `4`: Permission denied

### Confirmation Prompts

Interactive commands MUST:
- Show clear prompt: `Delete session 'my-project'? (y/N):`
- Support `--yes` flag to skip
- Default to safe option (No for destructive operations)

## Error Message Examples

### Good Error Messages

```
❌ Error: Session 'my-project' not found

Context: No session exists with this name in ~/.agm/sessions/

To fix:
- List available sessions: agm list
- Create a new session: agm new my-project
- Check if session was archived: agm list --archived

For more info: agm help sessions
```

### Bad Error Messages

```
Error: not found
```

## Color Usage

### When to Use Color

- Success: Green (`✅`)
- Error: Red (`❌`)
- Warning: Yellow (`⚠️`)
- Info: Blue (`ℹ️`)
- Emphasis: Bold text

### When NOT to Use Color

- In `--format json` or `--format yaml` output
- When `NO_COLOR` environment variable is set
- When output is piped to non-TTY

## Consistency Checklist

Use this checklist when adding or modifying commands:

- [ ] Command supports `--help` flag
- [ ] Command supports `--debug` flag
- [ ] Flag names follow kebab-case convention
- [ ] Success messages use ✅ prefix
- [ ] Error messages follow template format
- [ ] Exit codes are appropriate
- [ ] Interactive prompts support `--yes`
- [ ] Output formatting is consistent with other commands
- [ ] Help text includes examples
- [ ] Help text includes See Also section

## References

- Cobra CLI framework: https://github.com/spf13/cobra
- Terminal colors: https://en.wikipedia.org/wiki/ANSI_escape_code
- Exit codes: https://tldp.org/LDP/abs/html/exitcodes.html
