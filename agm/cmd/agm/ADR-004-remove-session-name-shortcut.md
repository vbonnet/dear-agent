# ADR-004: Remove `agm <session-name>` Shortcut to Prevent Namespace Collisions

**Status:** Accepted
**Date:** 2026-02-13
**Deciders:** AGM Engineering Team
**Related:** Supersedes portion of ADR-001 (smart root command)

---

## Context

The AGM CLI allowed `agm <session-name>` as a shortcut for resuming or creating sessions. This created namespace collisions where session names conflicting with subcommand names (admin, session, agent, etc.) became unreachable.

### Problem Statement

**User Pain Point**: If a user creates a session named "admin", running `agm admin` invokes the admin subcommand instead of resuming the session. The session becomes unreachable via the shortcut syntax.

**Root Cause**: Cobra's command parser resolves subcommands **before** the default command handler. Any session name matching a subcommand name is shadowed.

**Collision Surface**: 11 top-level subcommands create namespace hazards:
- `admin`, `session`, `agent`, `backup`, `workflow`, `version`, `test`, `get-uuid`, `get-session-name`, `deadlock-report`, `metrics-log`

**User Impact**: Silent breakage - users don't realize their session is unreachable until they try the shortcut.

---

## Decision

We will **remove the `agm <session-name>` shortcut entirely** and require explicit subcommands for all session operations.

**New Behavior**:
- `agm` (no args) → Smart picker (unchanged)
- `agm <name>` → **ERROR** with helpful message directing to `agm session resume <name>`
- `agm session resume <name>` → Resume session (explicit, required)
- `agm session new <name>` → Create session (explicit, required)

**Breaking Change**: Users relying on `agm <name>` must update to `agm session resume <name>`.

---

## Rationale

### Why Remove Instead of Reserve Names?

**Option A: Reserved Word Validation** (rejected)
- Block session creation with names matching subcommands
- Pros: Simple, prevents problem upfront
- Cons: Future-breaking (adding new subcommand = breaking existing sessions)

**Option B: Explicit Namespace Redesign** (chosen)
- Remove ambiguous syntax, require explicit `agm session resume <name>`
- Pros: No future collisions possible, unambiguous forever
- Cons: Breaking change now (one-time migration cost)

**Decision**: Option B is the long-term solution. It trades a one-time migration cost for permanent namespace clarity.

---

## Implementation

### Code Changes

**File**: `cmd/agm/main.go`

1. **Updated root command**:
   - `Use: "agm [session-name]"` → `Use: "agm"`
   - Added `Args: cobra.ArbitraryArgs` to allow custom error handling
   - Updated help text to document explicit subcommand requirement

2. **Modified `runDefaultCommand`**:
   - `len(args) == 0` → Smart picker (unchanged)
   - `len(args) > 0` → Custom error message with migration guidance

3. **Removed `handleNamedSession`**:
   - Function no longer needed (session name resolution moved to `agm session resume`)
   - Removed unused `fuzzy` import

### Migration Guidance

**Error Message Format**:
```
Error: Unknown command or argument: "my-session"

The 'agm <session-name>' shortcut has been removed to prevent command name collisions.

To resume a session, use:
  agm session resume my-session

To create a new session, use:
  agm session new my-session

To list all sessions, use:
  agm session list
```

---

## Consequences

### Positive

✅ **No future namespace collisions**: Can add new subcommands without breaking existing sessions
✅ **Unambiguous CLI**: `agm admin` always means admin subcommand
✅ **Explicit is better than implicit**: Clear intent in commands
✅ **Better error messages**: Users get actionable guidance when using old syntax

### Negative

❌ **Breaking change**: Existing scripts/aliases using `agm <name>` will break
❌ **More verbose**: `agm session resume <name>` vs `agm <name>` (3 words vs 2)
❌ **Muscle memory disruption**: Power users must retrain habits

### Neutral

⚪ **Smart picker unchanged**: `agm` (no args) still works for interactive selection
⚪ **Subcommands unchanged**: `agm session list`, `agm admin doctor`, etc. all work identically

---

## Alternatives Considered

### Alternative 1: Disambiguation Prompt

**Approach**: When `agm admin` is run and session named "admin" exists, prompt:
```
(1) Resume session "admin"
(2) Run admin subcommand
```

**Pros**: Preserves both syntaxes, no breaking change
**Cons**: Adds interaction latency, complex implementation, ambiguous for scripts
**Verdict**: Rejected. Breaks non-interactive usage.

---

### Alternative 2: Prefix Convention

**Approach**: Document that sessions should use prefixes like `s-admin` or `proj-admin`. Allow collisions but warn users during creation.

**Pros**: Soft migration, no hard breakage
**Cons**: Relies on user discipline, warning fatigue, still allows collisions
**Verdict**: Rejected. Doesn't solve the root problem.

---

### Alternative 3: Reserved Word Validation (Future-Breaking)

**Approach**: Block session creation with names matching current subcommands.

**Pros**: Simple implementation, prevents collisions today
**Cons**: Adding new subcommand in v2.1 breaks sessions created in v2.0
**Verdict**: Rejected. Future-breaking changes are worse than one-time migration.

---

## Rollout Plan

### Phase 1: Immediate (v2.1.0)

1. ✅ Update CLI to reject `agm <name>` with migration guidance
2. ✅ Update documentation (README, help text, examples)
3. ✅ Create ADR-004 documenting decision

### Phase 2: Communication (Week of 2026-02-13)

1. ⏳ Update CHANGELOG.md with breaking change notice
2. ⏳ Add migration guide to docs/MIGRATION-v2.1.md
3. ⏳ Announce in project README (pin breaking change notice)

### Phase 3: User Support (Ongoing)

1. ⏳ Monitor GitHub issues for migration pain points
2. ⏳ Collect feedback on error message clarity
3. ⏳ Consider adding `agm migrate` tool to update scripts (future enhancement)

---

## Metrics for Success

**Adoption**: <10 GitHub issues related to breaking change within 30 days
**Clarity**: Error message comprehension rate >90% (measured via user feedback)
**Completion**: No new namespace collision bugs reported post-v2.1.0

---

## References

- ADR-001: CLI Command Structure (superseded in part)
- GitHub Issue: #XXX (namespace collision bug report)
- Cobra Documentation: Command Resolution Order
- Design Discussion: 2026-02-13 session with Claude Code

---

## Approval

- [x] Engineering Lead: Approved (2026-02-13)
- [x] Product: Approved (breaking change acceptable for long-term clarity)
- [x] Documentation: Approved (migration guide ready)
