# AGM Command Restructure - Decision Log

**Project:** AGM Command Architecture Restructure (v2.1)
**Date Started:** 2026-02-13
**Status:** Requirements Gathering Complete
**Methodology:** Engram Swarm

---

## Context

AGM's command structure has grown organically with several issues:

1. **Unused/optimistic commands** - Many commands built speculatively, never used in practice
2. **AI Agent confusion** - Too many commands/docs causing context rot for AI agents
3. **Incorrect grouping** - `agm session` mixes session lifecycle (new/resume/archive) with message sending (send/reject/recover)
4. **Lack of cross-cutting concerns** - send/reject/recover should share attribution logic (sender name, timestamp, logging)

---

## Decisions Made

### Decision 1: Command Usage Analysis

**Question:** Which commands are actually used vs built optimistically?

**Answer:**

**High-frequency usage (daily/multiple times per day):**
- `agm session new` - Create sessions daily
- `agm session resume` - Resume sessions daily
- `agm session list` - List sessions daily
- `agm session archive` - Archive sessions daily
- `agm session send` - Agents use constantly
- `agm session reject` - Agents use constantly
- `agm session recover` - Agents use constantly

**Medium-frequency usage (weekly):**
- `agm session kill` - Terminate deadlocked sessions when things go wrong
- `agm session unarchive` - Restore archived sessions few times/week

**Indirect usage (via agents):**
- `agm workflow list` - Used via agents for Gemini deep research

**Uncertain usage:**
- `agm session select-option` - Unclear if agents use this
- All other commands - Unknown if agents use them automatically

**Implementation:** Add built-in telemetry to track actual usage over 1-2 weeks before removing commands.

---

### Decision 2: Send Namespace Scope

**Question:** What should the scope of `agm send *` commands be?

**Answer:** `agm send [msg|recover|reject|select-option]`

**Rationale:**
- All operations that send interactions TO running sessions
- Includes text messages, control signals (ESC/CTRL-C), permission rejections, option selections
- `kill` stays in `agm session` namespace (it's session lifecycle, not interaction)
- All `agm send *` commands share unified attribution and logging infrastructure

**Examples:**
```bash
agm send msg <session> "Hello world"          # Send text message
agm send recover <session>                     # Send ESC/CTRL-C
agm send reject <session> "reason"             # Reject permission prompt
agm send select-option <session> <option-id>   # Select AskUserQuestion option
```

---

### Decision 3: Attribution Requirements

**Question:** What metadata should sender attribution capture?

**Answer:**

**Required fields (enforced):**
- **Sender name** (`--sender` flag or `AGM_SENDER` env var)
  - Examples: `vbonnet`, `astrocyte-daemon`, `claude-agent-a8eb424`
  - Mandatory for all `agm send *` commands
- **Timestamp** (auto-generated, ISO 8601 UTC)
  - Automatically captured on every send operation
- **Action type** (`msg`|`recover`|`reject`|`select-option`)
  - Derived from subcommand, enables log filtering

**Optional fields (contextual debugging):**
- **Reason** (`--reason` flag): Why this action was taken
- **Context** (`--context` flag): Additional context
- **Source location** (`--source-file`, `--source-line`): Where in code this originated
  - Example: `astrocyte monitoring loop line 42`

---

### Decision 4: Log Storage Format

**Question:** Where should unified interaction logs be stored?

**Answer:** Extend existing `~/.agm/logs/messages/` JSONL format

**Rationale:**
- All interactions (messages, control signals, rejections, selections) in one place
- Existing infrastructure for daily JSONL files (YYYY-MM-DD.jsonl)
- Easy to query with existing `agm session logs query` tooling
- Extends naturally with new fields (sender, action_type, context)

**JSONL Schema Extension:**
```jsonl
{
  "timestamp": "2026-02-13T06:42:00Z",
  "session_id": "abc-123",
  "action_type": "msg",
  "sender": "vbonnet",
  "content": "Hello world",
  "context": {
    "reason": "manual testing",
    "source_file": "test.sh",
    "source_line": 42
  }
}
```

---

### Decision 5: Breaking Change Tolerance

**Question:** What's the tolerance for breaking changes?

**Answer:** **Aggressive clean slate approach**

**Strategy:**
- Remove unused commands entirely (based on telemetry analysis)
- Rename `agm session send` → `agm send msg` (no backwards compatibility)
- Rename `agm session reject` → `agm send reject`
- Rename `agm session recover` → `agm send recover`
- No deprecation period, no aliases
- Users and agents must update
- Comprehensive migration guide in docs

**Justification:**
- User prefers clean architecture over backwards compatibility
- One-time pain now vs ongoing complexity
- v2.1 is major version, breaking changes expected

---

### Decision 6: Usage Tracking Implementation

**Question:** How should we track command usage to inform removal decisions?

**Answer:** **Built-in telemetry with opt-out**

**Implementation:**
- Add telemetry to AGM that logs command usage to `~/.agm/telemetry/`
- Track: command name, timestamp, caller type (human/agent/daemon)
- Privacy-focused: local only, no external reporting
- Opt-out: `--no-telemetry` flag or `AGM_TELEMETRY=false` env var
- Default: opt-in (enabled by default)

**Data Format:**
```jsonl
{
  "timestamp": "2026-02-13T06:42:00Z",
  "command": "agm session new",
  "caller_type": "human",
  "duration_ms": 1234
}
```

**Analysis Period:** 1-2 weeks of telemetry collection before Phase 3 (command removal)

---

### Decision 7: Agent Feedback Survey

**Question:** Should we implement post-command agent surveys ("Was this useful?")?

**Answer:** **Defer to v2.2 (later iteration)**

**Rationale:**
- Great idea for continuous improvement loop
- But adds complexity to initial restructure
- Focus on core command reorganization first (Phase 1+2)
- Add agent feedback mechanism in Phase 4 (future)

**Future Design (tentative):**
- Environment variable `AGM_FEEDBACK_MODE=true` for agents
- Structured JSON feedback requests after commands
- Agent responds with usefulness rating + suggestions
- Logs to `~/.agm/feedback/` for periodic review

---

### Decision 8: Project Methodology

**Question:** Use engram-swarm or wayfinder for execution?

**Answer:** **`/engram-swarm:start`**

**Rationale:**
- Complex refactor with multiple independent workstreams:
  - Telemetry infrastructure
  - Command namespace reorganization
  - Attribution/logging system
  - Migration documentation
- Swarm methodology better for parallel work (ROADMAP.md, beads, status tracking)
- Wayfinder is better for greenfield features (D1-D4 discovery overhead not needed here)

---

### Decision 9: Project Scope and Phasing

**Question:** Do everything at once or break into phases?

**Answer:** **Phase 1 + 2: Telemetry + Send Reorganization**

**Scope:**

**Phase 1: Add Usage Telemetry (v2.1-alpha)**
- Implement telemetry infrastructure (`~/.agm/telemetry/`)
- Track command usage (command name, timestamp, caller type)
- Opt-out mechanism (`--no-telemetry` flag)
- Run for 1-2 weeks to collect data

**Phase 2: Reorganize Send Namespace (v2.1-beta)**
- Create `agm send` namespace
- Implement sender attribution (required: sender, timestamp, action_type)
- Unified logging to `~/.agm/logs/messages/`
- Migrate commands:
  - `agm session send` → `agm send msg`
  - `agm session reject` → `agm send reject`
  - `agm session recover` → `agm send recover`
  - `agm session select-option` → `agm send select-option`
- Remove old `agm session send|reject|recover|select-option` commands
- Update documentation and migration guide

**Phase 3: Remove Unused Commands (v2.2 - Future)**
- Analyze telemetry data from Phase 1
- Identify commands with <1% usage
- Remove or deprecate unused commands
- Document removals

**Phase 4: Agent Feedback Loop (v2.3 - Future)**
- Implement agent survey mechanism
- Collect feedback on command usefulness
- Continuous improvement based on agent input

**Out of Scope (for now):**
- Command removal (waiting for telemetry data)
- Agent feedback surveys (deferred to v2.3)
- Major UI/UX redesigns
- Non-breaking enhancements to existing commands

---

## Implementation Plan

**Step 1: Requirements Gathering** ✅ COMPLETE
- Interview user using AskUserQuestion
- Document decisions in DECISION_LOG.md
- Create task tracking structure

**Step 2: Launch Engram Swarm** ✅ COMPLETE
- Ran swarm initialization
- Created ROADMAP.md with 21 tasks (Phase 0-2)
- Created 21 beads for parallel workstreams

**Step 3: Phase 1 Execution** ⏳ PENDING
- Implement telemetry infrastructure
- Deploy v2.1-alpha for data collection
- Run for 1-2 weeks

**Step 4: Phase 2 Execution** ⏳ PENDING
- Implement send namespace
- Implement attribution/logging
- Migrate commands
- Documentation and migration guide
- Deploy v2.1-beta

**Step 5: Stabilization and Release** ⏳ PENDING
- Testing and validation
- Release v2.1 stable
- Monitor for issues

---

## Open Questions

None - all requirements gathered and documented.

---

## References

- ADR-001: CLI Command Structure (partial supersede)
- ADR-004: Remove session name shortcut (related namespace work)
- Current command inventory: 35 commands across 7 namespaces

---

## Approval

- [x] User approval: Requirements gathering complete (2026-02-13)
- [x] Swarm roadmap approval: ROADMAP.md created with 21 tasks (2026-02-13)
- [x] Implementation approval: Phase 0 execution in progress (2026-02-13)

---

**Status:** Phase 0 execution in progress. Next: Complete Phase 0, then begin Phase 1 (Telemetry).
