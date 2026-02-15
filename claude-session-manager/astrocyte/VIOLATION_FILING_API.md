# Violation Filing API Reference

## Overview

This document describes the violation filing API added in Work Stream 4 (Astrocyte Integration).

## Functions

### `load_patterns(pattern_type: str)`

Load anti-patterns database with caching.

**Parameters**:
- `pattern_type` (str): Type of pattern file to load (bash, beads, git)

**Returns**:
- `dict`: Pattern data dict with 'patterns' list, or None if unavailable

**Example**:
```python
bash_patterns = load_patterns('bash')
beads_patterns = load_patterns('beads')
git_patterns = load_patterns('git')
```

**Caching**:
- Patterns cached in memory with mtime tracking
- Automatically reloads if file modified
- Shared cache across all pattern types

---

### `load_bash_patterns()`

Backward compatibility wrapper for `load_patterns('bash')`.

**Returns**:
- `dict`: Bash pattern data dict, or None if unavailable

**Example**:
```python
patterns = load_bash_patterns()  # Same as load_patterns('bash')
```

---

### `file_violation(pattern_id, command, session_id, agent_type, pattern_type='bash')`

File a violation to violations directory.

**Parameters**:
- `pattern_id` (str): Pattern ID from pattern database (e.g., "cd-chaining")
- `command` (str): Exact command that violated
- `session_id` (str): Session name where violation occurred
- `agent_type` (str): Type of agent (general-purpose, explore, etc.)
- `pattern_type` (str): Pattern file type (bash, beads, git) - default: 'bash'

**Returns**:
- `str`: Path to violation file, or None if filing failed

**Output Location**:
- `~/src/ws/oss/repos/engram/violations/{pattern_type}/YYYY-MM-DD-{pattern-id}-{hash}.md`

**Example**:
```python
violation_file = file_violation(
    pattern_id='cd-chaining',
    command='cd /repo && git push',
    session_id='test-session',
    agent_type='general-purpose',
    pattern_type='bash'
)
# Returns: '~/src/ws/oss/repos/engram/violations/bash/2026-02-15-cd-chaining-a1b2c3d4.md'
```

**File Format**:
```yaml
---
id: 2026-02-15-cd-chaining-a1b2c3d4
date: 2026-02-15T10:23:45Z
type: cd_usage
severity: high
tier: "3_astrocyte"
pattern_id: cd-chaining
pattern_type: bash
session_id: test-session
agent_type: general-purpose
command: cd /repo && git push
---

# Violation Report: cd-chaining

## Context
[Auto-generated context]

## Violation Details
[Auto-generated details]

## Why It Happened
[Auto-generated analysis]

## Recovery
[Auto-generated recovery steps]

## Proposed Fix
[Auto-generated suggestions]
```

---

### `extract_command_from_pane(pane_content, pattern)`

Extract the violating command from pane content.

**Parameters**:
- `pane_content` (str): Pane text content
- `pattern` (dict): Pattern dict with compiled regex

**Returns**:
- `str`: Extracted command string (max 500 chars) or 'unknown'

**Example**:
```python
command = extract_command_from_pane(pane_content, pattern)
# Returns: "cd /repo && git push"
```

---

### `reject_with_bash_feedback(session_name, pattern_id)`

Reject session with bash violation feedback using agm session reject.

**Parameters**:
- `session_name` (str): Session name
- `pattern_id` (str): Pattern ID from database

**Returns**:
- `RecoveryResult`: Recovery result with success status

**Behavior**:
1. Captures pane state (before)
2. Loads pattern from database
3. Extracts command from pane
4. Files violation to disk
5. Sends rejection message with tier1_example
6. Waits for rejection to process
7. Captures pane state (after)
8. Verifies recovery success

**Rejection Message Format**:
```
{reason}

{alternative}

{tier1_example}

See bash tool guidance: bash-command-simplification.ai.md
```

**Example**:
```python
result = reject_with_bash_feedback('test-session', 'cd-chaining')
if result.success:
    print(f"Recovery successful in {result.duration_seconds}s")
```

---

## Usage Examples

### Basic Violation Filing
```python
# File a bash violation
violation_file = file_violation(
    pattern_id='cd-chaining',
    command='cd /repo && git push',
    session_id='myapp-session',
    agent_type='general-purpose',
    pattern_type='bash'
)

if violation_file:
    print(f"Violation filed: {violation_file}")
```

### Loading Multiple Pattern Types
```python
# Load all pattern types
bash_patterns = load_patterns('bash')
beads_patterns = load_patterns('beads')
git_patterns = load_patterns('git')

# Check loaded patterns
for pattern in bash_patterns['patterns']:
    print(f"{pattern['id']}: {pattern['reason']}")
```

### Recovery with Enhanced Feedback
```python
# Detect violation
pattern_id = detect_bash_violation(current_state)

if pattern_id:
    # Reject with enhanced feedback (includes tier1_example)
    result = reject_with_bash_feedback(session_name, pattern_id)

    if result.success:
        print("Session recovered with enhanced guidance")
    else:
        print("Recovery failed, escalating...")
```

---

## Pattern Database Schema

### Required Fields
```yaml
- id: pattern-id-kebab-case
  regex: 'pattern regex'
  reason: "Why this is bad"
  alternative: "What to do instead"
  examples:
    - "example 1"
    - "example 2"
  severity: high  # critical, high, medium, low
```

### Optional Fields
```yaml
  tier1_example: |
    ❌ BAD: incorrect usage
    ✅ GOOD: correct usage

  tier2_validation: true   # Enable PreTool hook validation
  tier3_rejection: true    # Enable Astrocyte recovery
```

---

## Error Handling

### Pattern Loading
- Returns `None` if file not found
- Returns `None` if YAML parse error
- Returns `None` if no 'patterns' key
- Handles regex compilation errors gracefully

### Violation Filing
- Returns `None` on any error (doesn't crash recovery)
- Creates subdirectories automatically
- Handles file write errors
- Safe to call even if patterns unavailable

### Command Extraction
- Returns `'unknown'` if extraction fails
- Truncates to 500 chars if too long
- Handles regex errors gracefully

---

## File Locations

### Code
- `~/src/ws/oss/repos/ai-tools/main/claude-session-manager/astrocyte/astrocyte.py`

### Pattern Databases
- `~/src/ws/oss/repos/engram/patterns/bash-anti-patterns.yaml`
- `~/src/ws/oss/repos/engram/patterns/beads-anti-patterns.yaml`
- `~/src/ws/oss/repos/engram/patterns/git-anti-patterns.yaml`

### Violation Outputs
- `~/src/ws/oss/repos/engram/violations/bash/`
- `~/src/ws/oss/repos/engram/violations/beads/`
- `~/src/ws/oss/repos/engram/violations/git/`

### Schema
- `~/src/ws/oss/repos/engram/violations/SCHEMA.yaml`

---

## Testing

### Unit Tests
```bash
cd ~/src/ws/oss/repos/ai-tools/main/claude-session-manager/astrocyte
python3 test_violation_filing.py
```

### Manual Testing
```python
# Load patterns
patterns = load_patterns('bash')
assert patterns is not None
assert 'patterns' in patterns

# File violation
filepath = file_violation(
    pattern_id='cd-chaining',
    command='cd /repo && git push',
    session_id='test',
    agent_type='general-purpose'
)
assert filepath is not None
assert os.path.exists(filepath)

# Clean up
os.remove(filepath)
```

---

## Future Enhancements

### WS5: Validator Implementation
- Add beads violation detection
- Add git violation detection
- Create pattern-specific rejection functions

### WS7: Automatic Analysis
- Parse violations from subdirectories
- Aggregate by pattern_type
- Generate trend reports

### Phase 8: Go Rewrite
- Port violation filing to Go
- Maintain same API contract
- Keep YAML format for interoperability
