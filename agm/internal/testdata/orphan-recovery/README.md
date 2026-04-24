# Orphaned Session Test Fixtures

Test fixtures for orphaned session detection and recovery scenarios.

## Background

Based on real production incident (Feb 17-19, 2026) where orphaned conversation 370980e1-e16c-48a1-9d17-caca0d3910ba was discovered with 20 uncommitted files.

## Orphan Definition

An orphaned session is a conversation that:
1. Has entries in Claude history.jsonl
2. Has conversation directory in ~/.claude/projects/
3. Does NOT have a corresponding AGM manifest

## Fixtures

### history-with-orphans.jsonl
Contains mix of tracked and orphaned sessions:

**Orphaned (no manifests)**:
- `370980e1-e16c-48a1-9d17-caca0d3910ba` - Real orphan from production incident (workspace: oss)
- `a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890` - Orphan in different workspace (workspace: acme)
- `orphan-multi-workspace-001` - Orphan in research workspace
- `orphan-stale-session-001` - Old orphan (30+ days, timestamp: Jan 21 2024)
- `orphan-recent-crash-001` - Recent orphan (simulates AGM crash)

**Tracked (have manifests)**:
- `tracked-session-uuid-001` - Has manifest tracked-manifest-001.yaml
- `tracked-session-uuid-002` - Has manifest tracked-manifest-002.yaml

### Manifests

- `tracked-manifest-001.yaml` - Valid v2 manifest for tracked-session-uuid-001 (workspace: oss)
- `tracked-manifest-002.yaml` - Valid v2 manifest for tracked-session-uuid-002 (workspace: acme)

## Expected Detection Results

### agm admin find-orphans
Should detect 5 orphans:
- 370980e1-e16c-48a1-9d17-caca0d3910ba (oss workspace, last activity: Feb 19 2024)
- a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890 (acme workspace, last activity: Feb 19 2024)
- orphan-multi-workspace-001 (research workspace, last activity: Feb 19 2024)
- orphan-stale-session-001 (oss workspace, last activity: Jan 21 2024, STALE flag)
- orphan-recent-crash-001 (oss workspace, last activity: Feb 19 2024)

### Workspace Filtering

- `agm admin find-orphans --workspace oss` - Should detect 3 orphans (370980e1, orphan-stale, orphan-recent-crash)
- `agm admin find-orphans --workspace acme` - Should detect 1 orphan (a1b2c3d4)
- `agm admin find-orphans --workspace research` - Should detect 1 orphan (orphan-multi-workspace)

## Usage in Tests

```go
import (
    "internal/claude"
    "internal/manifest"
)

// Load history
entries, _, _ := claude.ParseHistory("internal/testdata/orphan-recovery/history-with-orphans.jsonl")

// Load manifests
manifests := manifest.LoadFromDir("internal/testdata/orphan-recovery/")

// Detect orphans (UUIDs in history but not in manifests)
orphans := detectOrphans(entries, manifests)

assert.Len(t, orphans, 5)
assert.Contains(t, orphans, "370980e1-e16c-48a1-9d17-caca0d3910ba")
```

## Test Scenarios

### Scenario 1: Batch Orphan Detection
- Given: history-with-orphans.jsonl and 2 tracked manifests
- When: Run `agm admin find-orphans`
- Then: Detect 5 orphans, display table with UUID, project, last_modified, status

### Scenario 2: Auto-Import Mode
- Given: Orphaned session 370980e1-e16c-48a1-9d17-caca0d3910ba
- When: Run `agm admin find-orphans --auto-import`
- Then: Prompt to import orphan, create manifest, verify tmux name sanitization

### Scenario 3: Workspace Filtering
- Given: Orphans in multiple workspaces
- When: Run `agm admin find-orphans --workspace oss`
- Then: Only detect orphans in oss workspace

### Scenario 4: Single Session Import
- Given: Orphaned UUID a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890
- When: Run `agm session import a1b2c3d4-e5f6-4789-a1b2-c3d4e5f67890`
- Then: Create manifest, infer project from history, sanitize tmux name

### Scenario 5: Duplicate Prevention
- Given: Tracked session tracked-session-uuid-001
- When: Run `agm session import tracked-session-uuid-001`
- Then: Fail with error "Session already has manifest"
