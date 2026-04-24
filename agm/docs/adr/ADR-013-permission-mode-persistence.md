# ADR-013: Permission Mode Persistence for Claude Code Sessions

**Status**: Implemented
**Date**: 2026-03-20
**Authors**: AGM Team
**Related**: Migration 008, Mode Restoration Feature

## Context

Claude Code supports multiple permission modes (`default`, `plan`, `ask`, `allow`) that control how the AI assistant interacts with the filesystem and tools. Users toggle between modes using `Shift+Tab`.

**Problem**: When AGM resumes a Claude Code session, the permission mode always resets to `default`, regardless of what mode the user had selected before. This breaks user workflow, especially for:

- **Plan Mode Sessions**: Users working in exploratory/read-only mode lose this setting
- **Ask Mode Sessions**: Users requiring extra confirmation for sensitive operations must reconfigure
- **Multi-Session Workflows**: Mode preference doesn't persist across session lifecycle

## Decision

We will implement **automatic permission mode persistence** using a three-layer architecture:

### 1. Detection Layer (Hook-Based)
- **Hook**: `PreToolUse` hook monitors `permission_mode` field in JSON input
- **Caching**: File-based cache (`~/.agm/mode-cache/{session-id}`) prevents redundant DB writes
- **Update**: Calls `agm session update-mode` when mode changes detected
- **Performance**: <5ms overhead per tool call, non-blocking background writes

### 2. Persistence Layer (Dolt Database)
- **Migration 008**: Adds 3 columns to `agm_sessions`:
  - `permission_mode VARCHAR(20) DEFAULT 'default'`
  - `permission_mode_updated_at TIMESTAMP NULL`
  - `permission_mode_source VARCHAR(50) DEFAULT 'hook'`
- **Manifest**: Adds corresponding fields to `Manifest` struct with `yaml:",omitempty"`
- **CRUD**: All session operations (Create/Update/Get/List) handle mode fields

### 3. Restoration Layer (tmux Integration)
- **Detection Point**: After `WaitForPromptSimple()` succeeds in `resume.go`
- **Calculation**: `calculateShiftTabCount()` maps mode to keystroke count
- **Execution**: Sends `n` `Shift+Tab` via `tmux.SendKeys("S-Tab")`
- **Timing**: 100ms delay between keystrokes for tmux processing
- **Error Handling**: Failures logged as warnings, don't block attach

## Alternatives Considered

### Alternative 1: Direct Claude Code API
**Rejected**: Claude Code doesn't expose an API to programmatically set permission mode. Only keyboard shortcuts work.

### Alternative 2: Env Variable Injection
**Rejected**: Permission mode is not controlled via environment variables in Claude Code.

### Alternative 3: Config File Modification
**Rejected**: Mode is runtime state, not persistent configuration. Would require Claude Code restart.

### Alternative 4: Manual User Action
**Rejected**: Defeats the purpose of session persistence. Users expect seamless mode restoration.

## Consequences

### Positive
✅ **Transparent UX**: Mode automatically restored on resume
✅ **Backward Compatible**: Old sessions without mode data default gracefully
✅ **Low Overhead**: Hook adds <5ms per tool call
✅ **Agent-Aware**: Only attempts restoration for Claude (skips Gemini/OpenCode)
✅ **Error Resilient**: Detection/restoration failures don't block normal operation

### Negative
⚠️ **tmux Dependency**: Restoration requires tmux `send-keys` (doesn't work with other backends)
⚠️ **Timing Sensitivity**: 100ms delay might need tuning for slow systems
⚠️ **Hook Performance**: PreToolUse fires frequently (10-50x per session)

### Neutral
- **Migration Required**: Users must run `agm admin migrate` to apply migration 008
- **Hook Installation**: Users must run `agm admin install-hooks` to enable detection
- **Cache Maintenance**: Mode cache grows with session count (1 file per session)

## Implementation Details

### Files Modified
1. **Migration**: `internal/dolt/migrations/008_add_permission_mode.sql`
2. **Manifest**: `internal/manifest/manifest.go`
3. **Dolt Layer**: `internal/dolt/sessions.go`
4. **Command**: `cmd/agm/session_update_mode.go`
5. **Hook**: `cmd/agm/hooks/pretool-agm-mode-tracker`
6. **Resume**: `cmd/agm/resume.go`

### Testing Strategy
- **Unit Tests**: `calculateShiftTabCount()`, mode validation
- **Integration Tests**: End-to-end mode persistence workflow
- **Golden Tests**: Manifest schema updated for new fields
- **BDD Tests**: `INTEGRATION-TEST-MODE-PERSISTENCE.md`

### Rollback Plan
If issues arise:
1. Disable hook: Remove from `~/.config/claude/config.yaml`
2. Skip restoration: Set `SKIP_MODE_RESTORE=1` env var (future feature)
3. Rollback migration: Dolt migrations are transactional

## Monitoring

### Metrics to Track
- Mode distribution across sessions (analytics)
- Mode change frequency (performance)
- Restoration success rate (reliability)
- Hook execution time (performance)

### Debug Commands
```bash
# View mode for session
agm session list --format json | jq '.[] | {name, mode: .permission_mode}'

# View mode cache
ls -lh ~/.agm/mode-cache/

# View hook logs
journalctl -t agm-mode-tracker | tail -20
```

## Security Considerations

- **Mode Validation**: Whitelist-based (default/plan/ask/allow only)
- **Tmux Injection**: Only literal key name "S-Tab" sent (no user input)
- **Hook Safety**: Read-only operations, background writes
- **Error Handling**: Failures don't expose sensitive data

## Future Enhancements

1. **Per-Project Defaults**: Allow users to set default mode per project
2. **Mode History**: Track mode changes over time for analytics
3. **Smart Suggestions**: Recommend mode based on task type
4. **Mode Sync**: Share mode preference across related sessions
5. **UI Indicator**: Show current mode in AGM status line
6. **Mode Policies**: Enforce certain modes for sensitive projects

## References

- Implementation Plan: `~/.claude/plans/fuzzy-shimmying-ullman.md`
- Migration 008: `internal/dolt/migrations/008_add_permission_mode.sql`
- Integration Tests: `docs/INTEGRATION-TEST-MODE-PERSISTENCE.md`
- Swarm ROADMAP: `engram-research/swarm/projects/agm-mode-persistence/`

## Related ADRs

- ADR-007: Hook-Based State Detection (mode detection uses PreToolUse hook)
- ADR-004: Tmux Integration Strategy (mode restoration uses tmux send-keys)
- ADR-005: Manifest Versioning Strategy (new fields use omitempty pattern)

## Approval

- [x] Implemented
- [x] Tested (all tests passing)
- [x] Documented
- [x] Reviewed
- [ ] Deployed (pending merge to main)
