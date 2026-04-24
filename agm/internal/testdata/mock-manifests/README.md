# Mock AGM Manifest Test Fixtures

Test fixtures for AGM manifest validation and audit scenarios.

## Fixtures

### valid-manifest.yaml
Healthy v2 manifest with all required fields:
- UUID: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
- Workspace: oss
- Lifecycle: active (empty string)
- Tmux session: agm-oss-valid-session

**Purpose**: Baseline for valid manifest structure

### corrupted-manifest.yaml
Malformed YAML with syntax errors:
- Invalid indentation
- Unclosed quote in purpose field
- Malformed list item in tags
- Incomplete tmux section

**Purpose**: Test manifest validation and error reporting

### missing-tmux-manifest.yaml
Valid manifest but tmux session doesn't exist:
- UUID: bbbbbbbb-cccc-dddd-eeee-ffffffffffff
- Tmux session: agm-oss-nonexistent-tmux-session (not running)

**Purpose**: Test `agm admin audit` detection of missing tmux sessions

### stale-manifest.yaml
Valid manifest with >30 days inactivity:
- Created: Dec 1 2023
- Last updated: Dec 15 2023
- UUID: cccccccc-dddd-eeee-ffff-000000000000

**Purpose**: Test stale session detection in audit

### duplicate-uuid-1.yaml & duplicate-uuid-2.yaml
Two manifests with same Claude UUID:
- Both use UUID: duplicate-uuid-dddddddd-eeee-ffff-0000-111111111111
- Different session_ids (mock-duplicate-1, mock-duplicate-2)

**Purpose**: Test duplicate UUID detection (CRITICAL severity in audit)

### archived-manifest.yaml
Archived session (completed work):
- Lifecycle: archived
- UUID: eeeeeeee-ffff-0000-1111-222222222222
- Should be excluded from active session checks

**Purpose**: Test lifecycle filtering in audit and session list

## Expected Audit Results

### CRITICAL Issues
- Duplicate UUIDs (duplicate-uuid-1.yaml, duplicate-uuid-2.yaml)
- Corrupted manifest (corrupted-manifest.yaml)

### WARNING Issues
- Missing tmux session (missing-tmux-manifest.yaml)
- Stale session >30 days (stale-manifest.yaml)

### INFO
- Valid active session (valid-manifest.yaml)
- Archived session (archived-manifest.yaml) - excluded from most checks

## Usage in Tests

```go
import "internal/manifest"

// Load manifest
m, err := manifest.LoadFromFile("internal/testdata/mock-manifests/valid-manifest.yaml")
assert.NoError(t, err)
assert.Equal(t, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", m.Claude.UUID)

// Load all manifests
manifests, errors := manifest.LoadAllFromDir("internal/testdata/mock-manifests/")
assert.Len(t, errors, 1) // corrupted-manifest.yaml
assert.Len(t, manifests, 6) // All except corrupted

// Test duplicate detection
uuids := extractUUIDs(manifests)
assert.True(t, hasDuplicates(uuids)) // duplicate-uuid found
```

## Test Scenarios

### Scenario 1: Corrupted Manifest Detection
- Given: corrupted-manifest.yaml
- When: Load manifest
- Then: Return YAML parse error

### Scenario 2: Missing Tmux Session
- Given: missing-tmux-manifest.yaml
- When: Run `agm admin audit`
- Then: Report WARNING "Tmux session agm-oss-nonexistent-tmux-session not found"

### Scenario 3: Stale Session Detection
- Given: stale-manifest.yaml with updated_at from Dec 2023
- When: Run `agm admin audit`
- Then: Report WARNING "Session inactive for 67 days"

### Scenario 4: Duplicate UUID Detection
- Given: duplicate-uuid-1.yaml and duplicate-uuid-2.yaml
- When: Run `agm admin audit`
- Then: Report CRITICAL "Duplicate UUID duplicate-uuid-dddddddd-eeee-ffff-0000-111111111111 in sessions mock-duplicate-1, mock-duplicate-2"

### Scenario 5: Archived Session Filtering
- Given: archived-manifest.yaml
- When: Run `agm session list`
- Then: Exclude from active sessions (unless --all flag)
