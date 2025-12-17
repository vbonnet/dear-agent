# Custom Session Naming Integration for CSM - Project Summary

**Location:** `~/src/repos/ai-tools/base/claude-session-manager/wayfinder-projects/custom-session-naming/`
**Status:** Discovery Phase (D1-D2 Complete)
**Created:** 2025-12-10

---

## Project Overview

### Goal

Enable CSM users to specify custom, meaningful session names instead of auto-generated directory-based names.

**Current:** `claude-myproject`, `claude-base-2`, `claude-base-3`
**Desired:** `feature-user-auth`, `bug-investigation-4532`, `research-deep-dive`

---

## Problem Validated ✅

### Key Pain Points

1. **Poor Discoverability**: Directory names don't convey session purpose
2. **Limited Control**: No way to specify meaningful names
3. **Manual Conflicts**: Users manually renaming tmux sessions breaks CSM tracking
4. **Session Clearing**: `/clear` command creates new UUID, CSM loses sync

### Impact

- **High Pain:** Power users with 5+ concurrent sessions
- **Medium Pain:** Users with 2-3 sessions needing mental mapping
- **Low Pain:** Single-session users (current naming sufficient)

---

## Solution Design ✅

### Recommended Approach: Hybrid Implementation

**Phase 1: CSM-Only Naming (MVP)**
- Add `--name` flag to `csm new` command
- Track custom names in manifest
- Works immediately, no Claude dependency

**Phase 2: `/clear` Handling**
- Detect UUID changes when user runs `/clear`
- Update manifest instead of creating duplicate session
- Preserve custom name and context

**Phase 3: `csm rename` Command**
- Allow renaming existing sessions
- Atomic update (tmux + manifest + directory)

**Phase 4: Claude Integration (Future)**
- If Claude Code supports custom names, integrate seamlessly
- CSM detects support and passes name via env var/flag
- Graceful degradation if not supported

---

## Technical Highlights

### Name Validation

```go
// Allowed: a-z, A-Z, 0-9, -, _
// Max length: 80 characters
// Must check conflicts with ALL tmux sessions
```

### `/clear` Detection Algorithm

```go
// Detect when:
// - UUID changed
// - Old UUID inactive
// - New UUID active
// → Update manifest (don't create duplicate)
```

### Manifest Structure

```yaml
name: feature-auth  # Custom name
tmux:
  session_name: feature-auth
context:
  custom_name: true  # Track if user-defined
```

---

## Implementation Roadmap

| Phase | Effort | Deliverable |
|-------|--------|-------------|
| Phase 1: `csm new --name` | ~2-3 hrs | Custom naming works |
| Phase 2: `/clear` handling | ~2 hrs | UUID updates preserved |
| Phase 3: `csm rename` | ~1.5 hrs | Rename existing sessions |
| Phase 4: Claude integration | ~2 hrs | Full sync (if supported) |
| **Total** | **~7.5-8.5 hrs** | Production-ready feature |

---

## Success Criteria

### P0 (Must-Have)
- ✅ `csm new --name "session"` creates custom-named session
- ✅ Name validation prevents invalid inputs
- ✅ Backward compatibility (auto-naming still works)
- ✅ Manifest tracks custom names correctly

### P1 (Should-Have)
- ✅ `/clear` detection and graceful handling
- ✅ `csm rename` command for existing sessions

### P2 (Nice-to-Have)
- ⏳ Claude Code integration (if feature available)
- ⏳ Naming convention suggestions

---

## Current Status

### Completed
- ✅ **D1-problem-validation.md**: Problem confirmed, scope defined
- ✅ **D2-solution-exploration.md**: Technical design complete

### Next Steps
1. **D3-requirements.md**: Detailed functional specifications
2. **D4-design.md**: Architecture diagrams, API design
3. **S5-plan.md**: Implementation plan with tasks
4. **S8-implementation.md**: Build the feature
5. **S9-validation.md**: Test and validate
6. **S10-retrospective.md**: Lessons learned

---

## Key Decisions

### Decision 1: Hybrid Approach
**Rationale:** Provides immediate value (CSM-only) while future-proofing for Claude integration

### Decision 2: Optional `--name` Flag
**Rationale:** Backward compatibility - existing workflows unaffected

### Decision 3: Track `custom_name` in Manifest
**Rationale:** Prevents accidental overwrite, enables different behavior for auto vs custom names

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Claude doesn't support custom names | Medium | CSM provides value independently |
| `/clear` detection fails | High | Manual `--force` flag as fallback |
| Name conflicts | Medium | Check all tmux sessions, clear errors |
| Manifest corruption | High | Atomic updates with backups (existing) |

---

## References

### Source Files
- `~/src/repos/ai-tools/base/claude-session-manager/cmd/csm/new.go:74`
- `~/src/repos/ai-tools/base/claude-session-manager/cmd/csm/resume.go:427-468`
- `~/src/repos/ai-tools/base/claude-session-manager/internal/manifest/manifest.go`

### Example Session
- `~/src/ws/sessions/claude-1-session/manifest.yaml`

### Community References
- Reddit: "claude code now unofficially supports custom session names"
- GitHub #2112: `--session-name` flag feature request
- GitHub #6006: Session renaming feature request

---

## Quick Start (When Implemented)

```bash
# Create session with custom name
csm new --name "feature-user-auth"

# Auto-generated name (backward compatible)
csm new ~/src/repos/myapp

# Rename existing session
csm rename claude-myapp feature-auth

# After /clear command
csm sync  # Automatically detects and updates UUID
```

---

**Project created by:** Claude Sonnet 4.5
**Wayfinder methodology:** Discovery → Design → Plan → Implement → Validate → Deploy
**Status:** Ready for D3 (Requirements Definition)
