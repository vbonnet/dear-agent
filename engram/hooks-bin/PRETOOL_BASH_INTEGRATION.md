> **SUPERSEDED** (2026-03-26): This document describes the old Python-era implementation.
> The pretool-bash-validator Python hook was replaced by the Go `pretool-bash-blocker`.
> See `cmd/pretool-bash-blocker/SPEC.md` for current integration docs.
> Kept for historical reference.

# Pretool-Bash Validator Integration

## Overview

This document describes the integration of the enhanced error message template system into the pretool-bash-validator hook.

## Implementation

**Task**: 0.3 (oss-e0o5) - Integration with Pretool-Bash Hook
**Status**: Complete
**Files Modified**: `pretool-bash-validator`

## Features

### Enhanced Error Message Template

The validator now generates structured, actionable error messages using the same template format as the Go error_messages.go system:

```
❌ Bash Tool Usage Violation

Command: <command>
Violation: <PATTERN-ID> - <reason>

Why this is problematic:
  • <reason 1>
  • <reason 2>
  • <reason 3>

Correct approach:
  <suggested fix from pattern database>

Benefits:
  • <benefit 1>
  • <benefit 2>

See also:
  • Engram: <engram section reference>
  • Docs: <documentation link>
  • Pattern database: engram/patterns/bash-anti-patterns.yaml
```

### Pattern-Specific Details

The `get_pattern_details()` function maps pattern IDs to detailed error information:

| Pattern ID | Reasons | Benefits | Engram Ref |
|-----------|---------|----------|------------|
| `cd-chaining` | Directory context issues, serialization, debugging | Explicit params, no state, portability | §1 cd patterns |
| `cat-file-read` | Better errors, workflow integration, reviewability | Dedicated tool benefits | §6 alternatives |
| `grep-search` | Structured output, context, modes | Better results, error handling | §6 alternatives |
| `find-file-search` | Simpler syntax, parsing, integration | Glob patterns, speed | §6 alternatives |
| `echo-redirect` | Silent overwrites, safety, tracking | Explicit ops, better errors | §5 redirection |
| `cat-heredoc` | Design, syntax mixing, review | Clean content, explicitness | §10 heredoc |
| `cp-copy` | Explicitness, errors, verification | Explicit ops, messages | §8 file ops |

### Text Wrapping

The `wrap_text()` function ensures error messages fit within 80-character terminal width:

```python
def wrap_text(text: str, width: int, indent: str = "") -> str:
    """Wrap text at specified width with optional indentation"""
    # Wraps long lines while preserving indentation on continuation lines
```

### Color Coding

Error messages use ANSI color codes for better readability:
- **Red** (`\033[31m`): Violation header
- **Yellow** (`\033[33m`): Command display
- **Green** (`\033[32m`): Suggested fix
- **Reset** (`\033[0m`): Reset to default

## Testing

### Test Suite

`test_pretool_bash_validator.py` provides comprehensive testing:

```bash
python3 engram/hooks/test_pretool_bash_validator.py
```

**Test Coverage:**
- ✅ `test_wrap_text()` - Text wrapping at 80 chars
- ✅ `test_get_pattern_details()` - Pattern detail retrieval
- ✅ `test_generate_message()` - Complete message generation
- ✅ `test_cat_pattern()` - Cat pattern error message
- ✅ `test_grep_pattern()` - Grep pattern error message

**Results:** All 5 tests passing

### Example Output

```
❌ Bash Tool Usage Violation

Command: cd /repo && git status
Violation: CD-CHAINING - Command chaining with cd

Why this is problematic:
  • cd creates directory context that doesn't persist across tool calls
  • Chaining serializes operations that could run in parallel
  • Makes commands harder to debug and verify

Correct approach:
  Use tool-specific -C flag (e.g., git -C /path)

Benefits:
  • Explicit directory parameter (clearer intent)
  • No reliance on shell state
  • Can be run from any directory

See also:
  • Engram: bash-command-simplification §1 (cd patterns)
  • Docs: Use absolute paths or -C flags instead of cd
  • Pattern database: engram/patterns/bash-anti-patterns.yaml
```

## Integration Flow

1. **Pattern Detection**: `detect_violation()` checks command against YAML pattern database
2. **Detail Lookup**: `get_pattern_details()` fetches reason/benefit/reference info
3. **Message Generation**: `generate_message()` formats using template structure
4. **Display**: Hook returns formatted message to Claude Code, command is blocked

## Deliverables Checklist

- [x] Modify pretool-bash validator to use new error message system
- [x] Pattern detection logic calls appropriate error generator
- [x] Error messages displayed before bash tool rejection
- [x] Fallback generic message if pattern not recognized
- [x] Error messages match template format from error_messages.go
- [x] Correct alternatives are specific to the command context
- [x] References point to exact engram sections
- [x] Comprehensive test coverage (5 tests, all passing)

## Dependencies

### Satisfied
- ✅ Task 0.1 (oss-97d8): Error Message Template System
- ✅ Task 0.2 (oss-dckq): Pattern-Specific Error Messages

### Blocks
- Task 1.2 (oss-z4p9): Mental Model Reset Mechanism (depends on enhanced messages)
- Task 2.2 (oss-taoc): Integration Testing with Real Violations (tests this integration)

## Next Steps

1. **Track 0: Deploy to Production**
   - Copy validator to `~/.claude/hooks/`
   - Verify hooks are active in Claude Code settings
   - Test with real violations

2. **Track B: Continue Phase 0**
   - Task 0.4 (oss-3994): Document Concrete Violation Examples
   - Task 0.5 (oss-4ajx): Resolve Documentation Conflicts
   - Task 0.6 (oss-glr6): Add Parallel Execution Mental Model

## Files

- `pretool-bash-validator`: Enhanced with template system
- `test_pretool_bash_validator.py`: Test suite (5 tests, all passing)
- `PRETOOL_BASH_INTEGRATION.md`: This documentation

## Compatibility

- **Python**: 3.8+ (uses type hints, f-strings)
- **Terminal**: ANSI color support required for color output
- **Dependencies**: PyYAML (optional, falls back to safe mode)

## Performance

- **Message Generation**: <1ms per violation
- **Pattern Lookup**: O(1) dictionary lookup
- **Text Wrapping**: O(n) where n = text length

No performance impact on normal (non-violating) commands.
