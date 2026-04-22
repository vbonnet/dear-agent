# Task 3.2 Complete: Update AGM Session Discovery for Workspace Paths

**Date**: 2026-03-13
**Task**: oss-tq6m (Phase 3 Task 3.2)
**Status**: ✅ COMPLETE
**Commit**: b23b7ac

## Summary

Successfully updated AGM session discovery to scan both workspace-specific paths and the new `~/.claude/sessions` fallback path, returning a unified list with workspace annotation.

## Changes Made

### Code Changes (1 file)
1. **internal/discovery/workspaces.go**:
   - Lines 116-120: Added `~/.claude/sessions` scan to config-based discovery
   - Lines 152-156: Added `~/.claude/sessions` scan to legacy discovery
   - Sessions in fallback path get empty workspace field (`""`)

### Test Changes (1 file)
2. **internal/discovery/workspaces_test.go**:
   - Added `TestFindSessionsAcrossWorkspaces_ClaudeFallback()` - Verifies legacy mode scans `~/.claude/sessions`
   - Added `TestFindSessionsAcrossWorkspaces_ConfigWithClaudeFallback()` - Verifies config mode scans both workspace + fallback

## Behavior

### Discovery Paths Scanned

**Config-based mode** (`~/.agm/config.yaml` exists):
1. For each enabled workspace:
   - `{output_dir}/sessions` (if output_dir differs from root)
   - `{root}/.agm/sessions` (standard)
   - `{root}/sessions` (legacy)
2. Fallback path: `~/.claude/sessions` (**NEW**)

**Legacy mode** (no config):
1. For each `~/src/ws/*` workspace:
   - `~/src/ws/{name}/.agm/sessions`
   - `~/src/ws/{name}/sessions`
2. Fallback path: `~/.claude/sessions` (**NEW**)

### Return Value

`DiscoveryResult.Locations` now includes:
- **Workspace sessions**: `workspace="oss"`, `workspace="acme"`, etc.
- **Fallback sessions**: `workspace=""` (empty = no workspace)

## Example Output

```go
result, _ := FindSessionsAcrossWorkspaces()

// Returns unified list:
// - SessionLocation{Workspace: "oss", Name: "project-x", ...}
// - SessionLocation{Workspace: "acme", Name: "medical-research", ...}
// - SessionLocation{Workspace: "", Name: "personal-notes", ...}  // from ~/.claude/sessions
```

## Testing

New tests verify:
- ✅ `~/.claude/sessions` is scanned in legacy mode
- ✅ `~/.claude/sessions` is scanned in config mode
- ✅ Fallback sessions have empty workspace field
- ✅ Both workspace and fallback sessions appear in unified list
- ✅ DirsSearched includes `~/.claude/sessions`

## Git Commit

```
commit b23b7ac
feat(agm): scan ~/.claude/sessions in session discovery

Update session discovery to scan ~/.claude/sessions fallback path
in addition to workspace-specific paths. This ensures sessions created
outside workspaces are discovered alongside workspace sessions.

Changes:
- Updated FindSessionsAcrossWorkspaces() config mode to scan ~/.claude/sessions
- Updated findSessionsLegacy() to scan ~/.claude/sessions
- Sessions in fallback path have empty workspace field
- Added 2 comprehensive tests for fallback discovery

Discovery now returns unified list:
- Workspace sessions: {workspace}/.agm/sessions/ (workspace="oss"|"acme"|etc)
- Fallback sessions: ~/.claude/sessions/ (workspace="")

Task: workspace-corpus-integration Phase 3 Task 3.2 (oss-tq6m)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

## Next Steps

- [x] Code changes implemented
- [x] Tests added and passing
- [x] Changes committed
- [ ] Move to Task 3.3: Add AGM migration command
