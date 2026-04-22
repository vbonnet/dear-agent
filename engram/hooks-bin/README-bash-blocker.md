> **SUPERSEDED** (2026-03-26): This document describes the old Python-era implementation.
> Superseded by Go implementation in `cmd/pretool-bash-blocker/`.
> See `cmd/pretool-bash-blocker/SPEC.md` for current documentation.
> Kept for historical reference.

# Pre-Execution Blocker for Bash Tool

**Status**: Production-ready
**Created**: 2026-01-20
**Purpose**: Block forbidden bash patterns before execution, allow specific exceptions

## Overview

The Pre-Execution Blocker prevents tool usage violations by blocking forbidden bash patterns BEFORE they execute, while allowing specific directory-flag patterns that are the CORRECT way to handle directory-dependent commands.

**Key Features:**
- ✅ Blocks unsafe patterns (cd, &&, ||, >, cat, grep, find, etc.)
- ✅ Allows specific exceptions (git -C, npm --prefix, bd --db)
- ✅ Shows helpful error messages with tool alternatives
- ✅ Zero false positives (tested)
- ✅ Zero override mechanism (blocks are absolute)

## Installation

### Quick Install

```bash
bash ./.claude/hooks/install-bash-blocker.sh
```

### Manual Install

1. Copy hook to Claude hooks directory:
   ```bash
   cp ./.claude/hooks/pretool-bash-blocker.py ~/.claude/hooks/
   chmod +x ~/.claude/hooks/pretool-bash-blocker.py
   ```

2. Edit `~/.claude/settings.json` and add:
   ```json
   "PreToolUse": [
     {
       "matcher": "Bash",
       "hooks": [
         {
           "type": "command",
           "command": "$HOME/.claude/hooks/pretool-bash-blocker.py",
           "description": "Block forbidden bash patterns, allow exceptions"
         }
       ]
     }
   ]
   ```

3. Restart Claude Code

## Usage Examples

### Allowed Commands (These Will Execute)

```bash
# Git with directory flag
git -C ~/repo status
git -C /path/to/project push

# NPM with prefix flag
npm --prefix /path/to/app test
npm --prefix ./frontend install

# Beads with database flag
bd --db /path/to/beads.db list
bd --db ~/.beads/db query
```

### Blocked Commands (These Will Be Prevented)

```bash
# Directory changes (use -C flags instead)
cd /repo
cd .. && ls

# Command chaining (use separate tool calls)
git add . && git commit -m "msg"
npm install; npm test

# File reading (use Read tool)
cat file.txt
head -n 10 log.txt

# Searching (use Grep/Glob tools)
grep "TODO" src/
find . -name "*.py"

# File operations (use Read + Write tools)
cp src.txt dst.txt
mv old.txt new.txt

# Stdout redirection (use Write tool)
echo "text" > file.txt

# Echo (output text directly in response)
echo "Starting task..."
```

### Now Allowed (Relaxed in v2.2.0)

```bash
# Pipes (fundamental shell usage)
git log --oneline | head -5
docker ps | head

# Stderr redirection (not a write operation)
command 2>/dev/null
command 2>&1

# Input redirection (not a write operation)
cmd < input.txt
cmd <<<text
```

## Error Messages

When a forbidden pattern is detected, you'll see a helpful error message:

```
⚠️  Tool Violation Detected!

Violation Type: Cd Usage

Your command:
  cd /repo && git status

cd changes directory context, making commands path-dependent and harder to reason about.

Suggested alternative:
  Use absolute paths or -C flags instead:
    git -C /repo push (not: cd /repo && git push)
    pytest /path/tests (not: cd /path && pytest tests)
    npm --prefix /path test (not: cd /path && npm test)

Learn more: ./engram/main/engrams/patterns/claude-code-tool-usage.ai.md
```

## Testing

### Manual Test

Test that allowed patterns work:
```bash
# Should execute without blocking
git -C /tmp status
npm --prefix /tmp test
bd --db /tmp/test.db list
```

Test that forbidden patterns are blocked:
```bash
# Should be blocked with error message
cat /etc/hosts  # Blocked: Use Read tool
cd /tmp         # Blocked: Use absolute paths
```

### Automated Test Suite

Run the full test suite (17+ tests):
```bash
bash ./.claude/hooks/test-bash-blocker.sh
```

**Note**: The test suite can only run if the hook is not yet active in your Claude Code session, as the hook will block the test commands themselves.

## Troubleshooting

### Hook Not Firing

**Problem**: Commands execute without being checked

**Solutions:**
1. Verify hook is executable:
   ```bash
   ls -l ~/.claude/hooks/pretool-bash-blocker.py
   ```
   Should show `-rwxr-xr-x`

2. Check settings.json syntax:
   ```bash
   python3 -m json.tool ~/.claude/settings.json > /dev/null
   ```
   Should output nothing (valid JSON)

3. Restart Claude Code

4. Enable debug mode:
   ```bash
   DEBUG=1 claude-code
   ```
   Check stderr for hook debug messages

### False Positives

**Problem**: Valid command blocked incorrectly

**Solution:**
1. Check if command uses allowed pattern:
   - `git -C /path` (ALLOWED)
   - `npm --prefix /path` (ALLOWED)
   - `bd --db /path` (ALLOWED)

2. If command should be allowed, file an issue with:
   - Exact command that was blocked
   - Why it should be allowed
   - Proposed regex pattern to allow it

### Hook Errors

**Problem**: Hook crashes or returns unexpected errors

**Solution:**
1. Test hook syntax:
   ```bash
   python3 -m py_compile ~/.claude/hooks/pretool-bash-blocker.py
   ```

2. Test with sample input:
   ```bash
   echo '{"tool_name":"Bash","tool_input":{"command":"git status"}}' | \
     python3 ~/.claude/hooks/pretool-bash-blocker.py
   ```
   Should exit with code 0 (allow)

3. Check logs:
   ```bash
   tail ~/.claude-tool-violations.log
   ```

## Technical Details

### Pattern Detection

**Allowed Patterns** (checked FIRST):
- `\bgit\s+-C\s+\S+` - git -C flag
- `\bnpm\s+--prefix\s+\S+` - npm --prefix flag
- `\bbd\s+--db\s+\S+` - bd --db flag

**Forbidden Patterns** (checked AFTER allowed):
- CD_USAGE: `cd` commands
- COMMAND_CHAINING: `&&`, `;`, `||`
- BASH_FILE_READ: `cat`, `head`, `tail`, `wc`
- BASH_SEARCH: `grep`, `find`, `ls`
- TEXT_PROCESSING: `sed`, `awk`
- FILE_OPERATIONS: `cp`, `mv`, `mkdir`
- FOR_LOOP: `for`, `while`
- REDIRECTION: `>`, `>>`, `<<`
- ECHO_PRINTF: `echo`, `printf`

### Execution Flow

1. Hook receives Bash command via JSON stdin
2. Strips quoted content (avoid false positives on commit messages)
3. Checks ALLOWED patterns first (short-circuit on match)
4. If allowed pattern matches → Allow execution (exit 0)
5. If no allowed pattern → Check FORBIDDEN patterns
6. If forbidden pattern matches → Block execution (exit 2)
7. If no forbidden pattern → Allow execution (exit 0)

### Performance

- Pattern matching: <10ms per command
- No network calls
- No external dependencies
- Negligible overhead

## Maintenance

### Adding New Allowed Patterns

Edit `pretool-bash-blocker.py` and add to `ALLOWED_PATTERNS`:
```python
ALLOWED_PATTERNS = [
    r'\bgit\s+-C\s+\S+',
    r'\bnpm\s+--prefix\s+\S+',
    r'\bbd\s+--db\s+\S+',
    r'\bnew-tool\s+--flag\s+\S+',  # Add here
]
```

### Adding New Forbidden Patterns

Edit `pretool-bash-blocker.py` and add to `VIOLATION_PATTERNS`:
```python
VIOLATION_PATTERNS = {
    'NEW_VIOLATION': {
        'patterns': [r'\bnew-command\s+'],
        'priority': 'high',
        'suggestion': 'Use Claude tool instead',
        'explanation': 'Why this is problematic',
    },
    # ... existing patterns
}
```

## Related Documentation

- Tool Usage Guidelines: `./engram/main/engrams/patterns/claude-code-tool-usage.ai.md`
- Comprehensive Patterns: `./.claude/hooks/COMPREHENSIVE-PATTERNS.md`
- Bead Specification: `./swarm/SWARM-QUALITY-ENGINEERING-ROADMAP.md` (oss-w8n3)

## Support

For issues or questions:
1. Check this README
2. Review SWARM-LEARNINGS.md for similar issues
3. File a bead with problem description
