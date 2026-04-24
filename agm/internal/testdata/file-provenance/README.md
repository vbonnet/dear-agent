# File Provenance Test Fixtures

Test fixtures for file provenance tracking (agm admin trace-files command).

## Background

The trace-files command answers "which sessions worked on this file?" by searching history.jsonl for file modification records.

## Fixtures

### history-with-file-modifications.jsonl
History entries with files_modified field (simulates actual Claude history format):

**Session: session-file-edit-001**
- Timestamp 1: Modified README.md, package.json (Feb 19 08:00)
- Timestamp 4: Modified README.md, CONTRIBUTING.md (Feb 19 11:20)

**Session: session-file-edit-002**
- Timestamp 2: Modified src/main.go, go.mod (Feb 19 10:46)
- Timestamp 6: Modified src/main.go (Feb 19 13:10)

**Session: session-file-edit-003**
- Timestamp 3: Modified config.yaml (different workspace: acme)

**Session: session-file-edit-004**
- Timestamp 5: Modified internal/manifest/manifest.go (Feb 19 12:10)

### Manifests

- `manifest-file-edit-001.yaml` - Maps to session-file-edit-001
- `manifest-file-edit-002.yaml` - Maps to session-file-edit-002

## Expected Query Results

### Query: README.md
```
agm admin trace-files ./README.md
```

Expected output:
```
File: ./README.md
Session: session-file-edit-001 (readme-updates)
  - 2024-02-19 08:00:00
  - 2024-02-19 11:20:00
```

### Query: src/main.go
```
agm admin trace-files ./main.go
```

Expected output:
```
File: ./main.go
Session: session-file-edit-002 (main-go-refactor)
  - 2024-02-19 10:46:40
  - 2024-02-19 13:10:00
```

### Query: Multiple files
```
agm admin trace-files ./README.md ./main.go
```

Expected output:
```
File: ./README.md
Session: session-file-edit-001 (readme-updates)
  - 2024-02-19 08:00:00
  - 2024-02-19 11:20:00

File: ./main.go
Session: session-file-edit-002 (main-go-refactor)
  - 2024-02-19 10:46:40
  - 2024-02-19 13:10:00
```

### Query: File not found
```
agm admin trace-files ./nonexistent.txt
```

Expected output:
```
File: ./nonexistent.txt
No sessions found
```

### Query: With date filter
```
agm admin trace-files ./README.md --since 2024-02-19T10:00:00Z
```

Expected output:
```
File: ./README.md
Session: session-file-edit-001 (readme-updates)
  - 2024-02-19 11:20:00
```

## Usage in Tests

```go
import (
    "internal/claude"
    "internal/manifest"
)

// Load history
entries, _, _ := claude.ParseHistory("internal/testdata/file-provenance/history-with-file-modifications.jsonl")

// Load manifests
manifests := manifest.LoadFromDir("internal/testdata/file-provenance/")

// Trace file
results := traceFile("./README.md", entries, manifests)

assert.Len(t, results, 2) // Two timestamps
assert.Equal(t, "session-file-edit-001", results[0].SessionID)
assert.Equal(t, "readme-updates", results[0].SessionName)
```

## Test Scenarios

### Scenario 1: Single File Trace
- Given: README.md modified by session-file-edit-001
- When: Run `agm admin trace-files ./README.md`
- Then: Return 2 timestamps from session-file-edit-001

### Scenario 2: Multiple File Trace
- Given: README.md and src/main.go
- When: Run `agm admin trace-files README.md src/main.go`
- Then: Return results for both files with different sessions

### Scenario 3: File Not Modified
- Given: File not in history
- When: Run `agm admin trace-files nonexistent.txt`
- Then: Return "No sessions found"

### Scenario 4: Date Filtering
- Given: README.md modified at multiple timestamps
- When: Run `agm admin trace-files README.md --since 2024-02-19T10:00:00Z`
- Then: Return only modifications after 10:00

### Scenario 5: Substring Matching
- Given: Multiple files matching pattern
- When: Run `agm admin trace-files --pattern "*.md"`
- Then: Return all markdown files and their sessions

## Implementation Notes

The files_modified field format in history.jsonl may vary:
- Array of absolute paths: `["file1", "file2"]`
- May need to handle relative paths in some Claude versions
- Null-byte resilience required (same as other history parsing)
