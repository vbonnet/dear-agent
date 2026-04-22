# Corrupted History Test Fixtures

Test fixtures for history.jsonl corruption scenarios based on real production issues (commit 453ca30).

## Fixtures

### history-with-null-bytes.jsonl
Contains null bytes (\x00) embedded in JSON:
- Valid entries: uuid-valid-1, uuid-valid-2, uuid-valid-3
- Corrupted entries: uuid-corrupted-1 (null in project path), uuid-null-start (null at UUID start), uuid-middle-null (null in middle of UUID)

**Purpose**: Test null-byte resilience in history parser (requirement from commit 453ca30)

### history-malformed-json.jsonl
Contains various JSON syntax errors:
- Lines 2, 4, 5, 6, 8: Malformed JSON (missing quotes, brackets, extra commas)
- Line 7: Empty sessionId field
- Valid entries: uuid-valid-before, uuid-after-malformed, uuid-final-valid

**Purpose**: Test parser's ability to skip malformed lines and continue processing

### history-missing-fields.jsonl
Contains entries with missing required fields:
- Line 2: Missing project field
- Line 3: Missing sessionId field
- Line 4: Missing timestamp field
- Line 5: Empty object
- Valid entries: uuid-complete, uuid-valid-after-empty

**Purpose**: Test field validation and graceful handling of incomplete entries

### history-truncated.jsonl
File truncated mid-entry:
- Lines 1-2: Complete entries
- Line 3: Truncated JSON (simulates file corruption or incomplete write)

**Purpose**: Test parser recovery from EOF mid-entry

## Expected Behavior

All parsers should:
1. Log warnings for corrupted lines (stderr)
2. Skip invalid entries
3. Continue processing valid entries
4. Return ParseStats with SkippedErrors count
5. Never panic on malformed input

## Usage in Tests

```go
import "internal/claude"

// Test resilience
entries, stats, err := claude.ParseHistory("internal/testdata/corrupted-history/history-with-null-bytes.jsonl")
assert.NoError(t, err)
assert.Equal(t, 3, stats.ValidEntries) // Only valid UUIDs counted
assert.Greater(t, stats.SkippedErrors, 0) // Corrupted entries logged
```
