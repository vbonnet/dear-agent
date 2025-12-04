# D3: Approach Decision - Workspace & Session Management

**Date:** 2025-12-02

**Project:** Workspace & Session Management System

**Phase:** D3 - Approach Decision

**Status:** 🔄 In Progress

---

## Purpose

Make concrete decisions on workspace management approach based on D2 research.

**Input from D2:**
- 6 patterns discovered (all viable)
- gwq tool identified
- LangChain patterns integrated
- Multi-persona review completed
- 5 critical decisions identified

**Output:** Specific choices for directory structure, session tracking, automation, and migration

---

## Decision Framework

Each decision includes:
- **Options evaluated** (from D2 research)
- **Chosen approach** (specific, concrete)
- **Rationale** (why this choice)
- **Trade-offs** (what we're giving up)
- **Implementation notes** (how to build)

---

## Decision 1: Directory Structure

### Options Evaluated

**Option A: Flat structure**
```
~/repos/
├── engram/
├── engram-research/
└── dotfiles/
```
- ❌ No work/personal separation
- ❌ Doesn't scale (100+ repos = chaos)
- ✅ Simple

**Option B: Two-level (work/personal)**
```
~/repos/
├── personal/
│   ├── engram/
│   └── dotfiles/
└── work/
    └── project-x/
```
- ✅ Work/personal separation
- ⚠️ Repos with same name conflict
- ⚠️ Doesn't reflect platform (GitHub, GitLab)

**Option C: Hierarchical platform-mirroring** (D2 Pattern 1)
```
~/src/
├── github/
│   ├── vbonnet/          # Personal
│   │   ├── engram/
│   │   ├── engram-research/
│   │   └── dotfiles/
│   └── [REDACTED_EMPLOYER]-src/       # Work
│       └── project-x/
└── gitlab/
    └── organization/
        └── project-y/
```
- ✅ Mirrors online structure (familiar)
- ✅ Work/personal separation (different usernames)
- ✅ Scales to hundreds of repos
- ✅ No name conflicts
- ⚠️ Deeper nesting

### ✅ CHOSEN: Option C - Hierarchical Platform-Mirroring

**Format:** `~/src/{platform}/{username}/{repo}/`

**Rationale:**
- Proven pattern (GoLang community, 2025 dev practices)
- Natural mental model (mirrors GitHub)
- Scales indefinitely
- Scripts can target work vs personal easily

**Examples:**
```bash
~/src/github/vbonnet/engram/              # Personal repo
~/src/github/vbonnet/engram-research/     # Personal repo
~/src/github/[REDACTED_EMPLOYER]-src/project-x/        # Work repo
```

**Trade-offs:**
- Deeper paths (more typing)
- Migration required from current locations

**Implementation:**
- Main repos live in ~/src/
- Follows XDG-style organization
- Tab completion helps with deep paths

---

## Decision 2: Worktree Organization

### Options Evaluated

**Option A: Sibling to main repo**
```
~/src/github/vbonnet/engram/              # Main
~/src/github/vbonnet/engram-feature-x/    # Worktree
```
- ❌ Pollutes repo directory
- ❌ Hard to distinguish main vs worktree
- ✅ Close to main repo

**Option B: Flat worktree directory**
```
~/worktrees/
├── engram-feature-x/
├── engram-fix-y/
└── dotfiles-test/
```
- ⚠️ No structure for multiple repos
- ⚠️ Name conflicts possible
- ✅ All worktrees in one place

**Option C: Hierarchical mirroring** (D2 Pattern 2)
```
~/worktrees/
├── github/
│   ├── vbonnet/
│   │   ├── engram/
│   │   │   ├── feature-bash-guidance/
│   │   │   ├── fix-telemetry/
│   │   │   └── review-pr-123/
│   │   └── dotfiles/
│   │       └── test-branch/
│   └── [REDACTED_EMPLOYER]-src/
│       └── project-x/
│           └── feature-auth/
```
- ✅ Mirrors ~/src/ structure
- ✅ Clear organization per repo
- ✅ Work/personal separation maintained
- ⚠️ Deepest nesting

### ✅ CHOSEN: Option C - Hierarchical Mirroring

**Format:** `~/worktrees/{platform}/{username}/{repo}/{branch-name}/`

**Rationale:**
- Mirrors main repo location (consistency)
- Clear organization (all worktrees for repo in one place)
- gwq tool expects organized structure
- Scales with repo count

**Examples:**
```bash
# Main repo
~/src/github/vbonnet/engram/

# Worktrees for that repo
~/worktrees/github/vbonnet/engram/feature-bash-guidance/
~/worktrees/github/vbonnet/engram/fix-telemetry/
~/worktrees/github/vbonnet/engram/review-pr-123/
```

**Trade-offs:**
- Deep paths (mitigated by gwq fuzzy finder)
- Manual directory creation (script it)

**Implementation:**
- Helper script: `create-worktree <repo> <branch>`
- Auto-creates ~/worktrees/{platform}/{user}/{repo}/ if needed
- Uses gwq for navigation/cleanup

---

## Decision 3: Session Tracking System

### Options Evaluated

**Option A: Git-backed repo**
```
~/.claude/sessions/ → git repo
└── {session-id}/
    └── manifest.yaml
```
- ✅ Portable (git push/pull)
- ✅ History tracking
- ❌ High churn (frequent commits)
- ❌ Merge conflicts on multi-machine

**Option B: Local JSON files**
```
~/.claude/sessions/
└── {session-id}/
    └── manifest.json
```
- ✅ Fast (no git overhead)
- ✅ No merge conflicts
- ❌ Not portable
- ❌ No history

**Option C: SQLite database**
```
~/.claude/sessions.db
```
- ✅ Queryable (find active sessions)
- ✅ Fast
- ❌ Not human-readable
- ❌ Not portable
- ❌ Requires SQL skills

**Option D: Hybrid** (Multi-persona recommendation)
```
~/.claude/sessions/     # Local (active state)
└── {session-id}/
    ├── manifest.yaml   # Auto-updated
    ├── working/        # Scratch pad
    └── artifacts/      # Keep on archive

~/engram-research/session-archives/  # Git-backed (completed)
└── 2025-12-02/
    └── session-abc123/
        ├── manifest.yaml
        └── artifacts/
```
- ✅ Fast for active sessions (local)
- ✅ Portable for archives (git)
- ✅ No merge conflicts (archives timestamped)
- ✅ Human-readable (YAML)
- ⚠️ Two storage locations

### ✅ CHOSEN: Option D - Hybrid Local + Git-Backed

**Active sessions:** Local `~/.claude/sessions/{id}/`

**Completed sessions:** Git-backed `~/engram-research/session-archives/{date}/`

**Rationale:**
- Best of both worlds (fast + portable)
- Aligns with LangChain durable execution
- Separates high-churn (active) from stable (archived)
- Multi-persona consensus

**Structure:**
```
~/.claude/sessions/abc123/
├── manifest.yaml          # Session metadata
├── working/               # Scratch pad (ephemeral)
│   ├── tool-results/
│   ├── analysis/
│   └── temp/
└── artifacts/             # Keep on archive
    ├── D1-problem.md
    └── S7-plan.md
```

**Lifecycle:**
1. **Create:** Session starts → create ~/.claude/sessions/{id}/
2. **Active:** Auto-update manifest on events (tool call, artifact created)
3. **Complete:** Move to ~/engram-research/session-archives/{date}/{id}/
4. **Archive:** Delete working/, keep artifacts/ + manifest

**Trade-offs:**
- Two locations to manage
- Manual archive step (mitigated by helper script)

**Implementation:**
- Auto-create session directory on first tool call
- Auto-update manifest after events
- Archive command: `archive-session {id}` (move to engram-research)

---

## Decision 4: Session Manifest Schema

### Chosen Format: YAML

**Why YAML:**
- Human-readable and editable
- Git-friendly diffs
- Supports comments
- Standard for session configs (tmuxp, docker-compose)

### Schema Design

```yaml
# Session metadata
session_id: "claude-abc123-def456"
created: "2025-12-02T10:30:00Z"
last_activity: "2025-12-02T14:22:00Z"
status: "active"  # active | completed | archived

# Work context
project: "bash-guidance-consolidation"
project_type: "wayfinder"  # wayfinder | research | bugfix | ad-hoc

# Git context
worktree:
  path: "{WORKTREES_ROOT}/github/vbonnet/engram/feature-bash-guidance"
  repo: "github.com/vbonnet/engram"
  branch: "feature/bash-guidance-consolidation"
  base_branch: "wayfinder-prototype"

# Artifacts tracking
artifacts:
  created:
    - path: "S7-plan.md"
      size: "2.5KB"
      created: "2025-12-02T11:00:00Z"
    - path: "S8-implementation.md"
      size: "8.3KB"
      created: "2025-12-02T13:15:00Z"
  worktree_files: 12  # Count of files accessed

# Context audit (LangChain Pattern 4)
context_audit:
  tokens_consumed: 15234
  files_available: 500
  files_accessed: 12
  efficiency_ratio: 2.4%  # Good - only loaded what needed

# Tags for filtering
tags:
  - "wayfinder"
  - "engram"
  - "bash-guidance"

# Resumption info
resumption:
  cwd: "{WORKTREES_ROOT}/github/vbonnet/engram/feature-bash-guidance"
  last_phase: "S8-implementation"
  next_steps: |
    Complete S8 implementation
    Run S9 validation
    Proceed to S10 deployment
```

**Variable substitution:**
- `{WORKTREES_ROOT}` → Actual path at runtime
- Enables cross-machine portability

**Rationale:**
- Rich metadata for resumption
- Context audit for optimization
- Tags for filtering/searching
- Resumption section for quick pickup

---

## Decision 5: Lifecycle Zones

### Options Evaluated

**Option A: Simple (active / archived)**
- ✅ Easy to understand
- ✅ Clear decision point
- ⚠️ No middle ground

**Option B: Granular (active / recent / archived)**
- ⚠️ "Recent" is ambiguous (7 days? 30 days?)
- ❌ More complexity
- ❌ Multi-persona flagged as too complex

**Option C: Time-based (0-7d / 7-30d / 30d+)**
- ❌ Arbitrary time boundaries
- ❌ Doesn't match user mental model
- ❌ Over-engineering

### ✅ CHOSEN: Option A - Simple (Active / Archived)

**Two states only:**
1. **Active:** Currently working on
2. **Archived:** Completed and moved to engram-research

**Rationale:**
- Multi-persona consensus (all flagged complexity)
- User mental model: "Am I working on this? Yes/No"
- Simpler automation (no middle states)
- Matches current practice (work in progress vs completed)

**Decision point:** Session complete → archive

**No intermediate states needed**

**Implementation:**
- Active: `~/.claude/sessions/{id}/`
- Archived: `~/engram-research/session-archives/{date}/{id}/`

---

## Decision 6: Automation Level

### Options Evaluated

**Option A: Fully automatic**
- Auto-archive after N days inactive
- Auto-delete working/ on archive
- Auto-cleanup merged worktrees
- ❌ Scary (what if wrong?)
- ❌ Loss of control

**Option B: Prompted (ask before action)**
- Detect stale sessions → prompt user
- Detect merged worktrees → prompt
- User approves before action
- ✅ User stays in control
- ✅ Learn before trust

**Option C: Manual with helpers**
- Tools make it easy, but user initiates
- No automation
- ❌ Requires discipline
- ❌ Will decay

### ✅ CHOSEN: Option B - Prompted Automation

**Automation boundaries:**

**Auto (no prompt):**
- Create session directory on first use
- Update manifest after events (tool call, artifact)
- Update last_activity timestamp

**Prompted (ask first):**
- Archive completed session
- Delete working/ directory
- Remove merged worktrees
- Cleanup stale sessions (> 30 days inactive)

**Manual (user initiates):**
- Create new worktree
- Switch between sessions
- Search session archives

**Rationale:**
- Multi-persona consensus
- User preference: "I want control"
- Build trust gradually (auto-actions can be added later)
- Skeptic concern: "Over-automation breaks trust"

**Example prompt:**
```
Session abc123 completed 7 days ago.
Archive to engram-research? [Y/n]

Worktree feature-bash-guidance merged into main.
Remove worktree? [Y/n]
```

**Implementation:**
- Dashboard command shows what needs action
- Cleanup wizard walks through each item
- User can bulk-approve or skip

---

## Decision 7: Migration Strategy

### Options Evaluated

**Option A: Clean slate**
- Start from zero, ignore existing chaos
- ❌ Loses current work
- ❌ Disruptive

**Option B: Gradual transition**
- New work uses new structure
- Old work stays until naturally archived
- ✅ Non-disruptive
- ✅ Learn new structure gradually
- ⚠️ Two systems coexist temporarily

**Option C: Full migration**
- Move everything to new structure immediately
- ❌ Requires pausing work
- ❌ Risk of breaking active sessions
- ✅ Clean transition

### ✅ CHOSEN: Option C - Full Migration Upfront

**USER MODIFICATION:** Changed from gradual to full upfront migration

**Reasoning:** User assessed "not that much content, let's just do it all upfront"

**Approach:**

**Phase 1: Preparation (30 min)**
- Create ~/src/ and ~/worktrees/ hierarchies
- Install gwq tool
- Set up ~/.claude/sessions/ structure
- Backup current state

**Phase 2: Full Migration (2-3 hours)**
- Move ALL repos from /tmp/ and ~/ to ~/src/{platform}/{user}/{repo}
- Move ALL worktrees to ~/worktrees/{platform}/{user}/{repo}/{branch}
- Create manifests for ALL current active sessions
- Archive ALL completed work to engram-research/session-archives/

**Phase 3: Verification (30 min)**
- Verify all repos accessible and functional
- Verify all worktrees work with git
- Test session manifests
- Confirm archives in engram-research

**Phase 4: Cleanup (30 min)**
- Delete old locations (/tmp/ repos, old worktrees)
- Remove 400+ claude-XXXX-cwd files
- Clean up scattered files

**Total time:** 3-4 hours (one-time, complete transition)

**Rationale:**
- User preference: Clean break over gradual
- Current content is manageable (known from cleanup)
- No coexistence complexity
- Immediate benefits (all worktrees visible, all sessions tracked)

**Migration script:**
```bash
#!/bin/bash
# Full migration script

# Phase 1: Setup
mkdir -p ~/src/github/{vbonnet,[REDACTED_EMPLOYER]-src}
mkdir -p ~/worktrees/github/{vbonnet,[REDACTED_EMPLOYER]-src}
mkdir -p ~/.claude/sessions/

# Phase 2: Migrate repos
migrate-all-repos    # Moves all from /tmp/ and ~/
migrate-all-worktrees
create-all-session-manifests

# Phase 3: Verify
verify-repos
verify-worktrees
verify-sessions

# Phase 4: Cleanup (with confirmation)
cleanup-old-locations --confirm
```

**Trade-offs:**
- Requires dedicated migration time (3-4 hours)
- Can't work during migration
- Higher risk (mitigated by verification + backup)

**Benefits:**
- Clean slate immediately
- No two-system confusion
- Faster to full compliance
- Learn new structure completely

**Timeline:**
- Single migration session: 3-4 hours
- Then: Fully using new structure

---

## Decision 8: Session Working Directory (Scratch Pad)

### ✅ CHOSEN: Include in D3 (User Request) - NON-EPHEMERAL

**User feedback Round 1:** "I really like the idea of these scratch pads, and I'd like to start using them and iterate on them, so yes please!"

**USER MODIFICATION Round 2:** "I'd rather keep them non-ephemeral for now so we can study them and see if there's anything about them we can learn from. Also I'd like to make sure they are structured by session as well."

**Structure:**
```
~/.claude/sessions/{id}/
├── manifest.yaml
├── working/                    # NON-ephemeral (keep on archive for study)
│   ├── tool-results/          # grep, find, API outputs
│   ├── analysis/              # Temporary analysis docs
│   └── scratch/               # Notes and intermediate work
└── artifacts/                  # Final outputs (keep on archive)
    ├── D1-problem.md
    └── S7-plan.md
```

**Purpose (LangChain Pattern 3 - Modified):**
- Keep large tool results out of conversation
- Reduce context bloat
- Enable better session resumption
- Provide working space for intermediate steps
- **NEW:** Study intermediate steps to learn patterns

**Lifecycle:**
1. **Active session:** working/ grows with tool outputs
2. **Review artifacts:** Move valuable work from working/ to artifacts/
3. **Archive session:** Keep BOTH working/ AND artifacts/ + manifest
4. **Study phase:** Analyze working/ patterns across sessions
5. **Future optimization:** Decide what's truly ephemeral based on learnings

**Rationale for non-ephemeral:**
- User wants to iterate on working/ design
- Need data to understand what's valuable in intermediate steps
- Can analyze patterns across sessions
- Early stage → collect data, optimize later
- Session-structured → working/ stays with its session (never merged)

**Archived structure:**
```
~/engram-research/session-archives/2025-12-02/session-abc123/
├── manifest.yaml
├── working/               # ← KEPT (was going to be deleted)
│   ├── tool-results/
│   ├── analysis/
│   └── scratch/
└── artifacts/
    ├── D1-problem.md
    └── S7-plan.md
```

**Storage impact:**
- +50-200KB per session (working/ contents)
- ~3.5-15MB/year (50 sessions)
- **Verdict:** Negligible, worth the learning value

**Implementation:**
- Auto-create working/ subdirs on first use
- Tools can write to working/ instead of conversation
- Archive command: Keep working/ + artifacts/ + manifest
- Future: Can analyze working/ to determine true ephemeral patterns

---

## Decisions Summary

| Decision | Chosen Approach | Key Reason | Modified? |
|----------|-----------------|------------|-----------|
| 1. Directory structure | Hierarchical platform-mirroring | Scales, natural mental model | No |
| 2. Worktree organization | Hierarchical mirroring | Mirrors ~/src/, works with gwq | No |
| 3. Session tracking | Hybrid local + git-backed | Fast + portable | No |
| 4. Manifest format | YAML with rich schema | Human-readable, git-friendly | No |
| 5. Lifecycle zones | Simple (active / archived) | User mental model, less complexity | No |
| 6. Automation level | Prompted (ask before action) | User control, build trust | No |
| 7. Migration strategy | **Full upfront (3-4h)** | **User: "Not that much content"** | **✅ YES** |
| 8. Working directory | **Non-ephemeral** | **User: Study & learn from them** | **✅ YES** |

---

## Final Directory Structure

### Complete Layout

```
~/
├── src/                           # Main repository clones
│   ├── github/
│   │   ├── vbonnet/              # Personal
│   │   │   ├── engram/
│   │   │   ├── engram-research/
│   │   │   ├── dotfiles/
│   │   │   └── retro-tasks/
│   │   └── [REDACTED_EMPLOYER]-src/           # Work
│   │       └── project-x/
│   └── gitlab/
│       └── organization/
│           └── project-y/
│
├── worktrees/                     # Git worktrees (mirrors src/)
│   └── github/
│       ├── vbonnet/
│       │   └── engram/
│       │       ├── feature-bash-guidance/
│       │       ├── fix-telemetry/
│       │       └── review-pr-123/
│       └── [REDACTED_EMPLOYER]-src/
│           └── project-x/
│               └── feature-auth/
│
├── .claude/                       # Claude Code session state
│   └── sessions/
│       ├── active/               # Currently working (local)
│       │   └── claude-abc123/
│       │       ├── manifest.yaml
│       │       ├── working/      # Scratch pad (ephemeral)
│       │       │   ├── tool-results/
│       │       │   ├── analysis/
│       │       │   └── scratch/
│       │       └── artifacts/    # Keep on archive
│       └── recent/               # (Deprecated - use simple lifecycle)
│
└── [existing directories]
    ├── .local/share/chezmoi/     # Dotfiles source
    ├── bin/                      # Custom scripts
    ├── retro-tasks/              # Process improvements (git-backed)
    └── go/                       # Go workspace

# Git-backed archives (in engram-research)
~/src/github/vbonnet/engram-research/
└── session-archives/
    └── 2025-12-02/
        └── session-abc123/
            ├── manifest.yaml
            └── artifacts/
```

### Path Variables

For cross-machine portability, use these variables:

```bash
# Environment variables
SRC_ROOT=~/src
WORKTREES_ROOT=~/worktrees
SESSIONS_ROOT=~/.claude/sessions
ARCHIVES_ROOT=~/src/github/vbonnet/engram-research/session-archives

# In manifests, use:
{SRC_ROOT}/github/vbonnet/engram
{WORKTREES_ROOT}/github/vbonnet/engram/feature-x
```

---

## Implementation Phases

### Phase 1: Directory Setup (Week 1)
- Create ~/src/ and ~/worktrees/ hierarchies
- Install gwq tool
- Set up session tracking directories
- **Deliverable:** Empty structure ready

### Phase 2: Helper Scripts (Week 1-2)
- create-worktree script
- archive-session script
- session-dashboard command
- **Deliverable:** Basic automation working

### Phase 3: Migration Tools (Week 2)
- move-repo script
- move-worktree script
- create-session-manifest script
- **Deliverable:** Can migrate incrementally

### Phase 4: Gradual Migration (Weeks 2-4)
- Move active work to new structure
- Create manifests for current sessions
- Archive completed sessions
- **Deliverable:** Actively using new structure

### Phase 5: Cleanup & Polish (Week 4+)
- Remove old locations
- Refine automation based on usage
- Add advanced features (context audit, pattern extraction)
- **Deliverable:** Fully transitioned

---

## Quality Control Checks

### Against D1 Requirements

| D1 Problem | D3 Solution | Status |
|------------|-------------|---------|
| Directory organization | ~/src/{platform}/{user}/{repo}/ | ✅ SOLVED |
| Worktree lifecycle | ~/worktrees/ + gwq tool | ✅ SOLVED |
| Session tracking | Hybrid local + git-backed manifests | ✅ SOLVED |
| Work resumability | Rich manifest with resumption section | ✅ SOLVED |
| Persistent logging | Session manifests + archives | ✅ SOLVED |
| Completed artifacts | Lifecycle zones (active/archived) | ✅ SOLVED |

**Coverage:** 100% of D1 problems addressed with concrete solutions

### Against D1 Success Criteria

**Quantitative:**
- ✅ Worktree cleanup < 5 min/week → gwq dashboard
- ✅ Session restart < 2 min → manifest + resumption info
- ✅ Work transfer < 5 min → git-backed archives
- ✅ Zero data loss from /tmp/ → session tracking + archives
- ✅ 100% worktree visibility → gwq tool

**Qualitative:**
- ✅ "What Claude sessions exist?" → session dashboard
- ✅ Easy resumption → manifest.yaml with next steps
- ✅ Structured logs → artifacts/ directory
- ✅ Git-backed → session archives in engram-research
- ✅ Scalable structure → hierarchical mirrors online

**All success criteria met by D3 design**

### Against D2 Patterns

| D2 Pattern | D3 Implementation | Status |
|------------|-------------------|---------|
| 1. Hierarchical directory | ~/src/{platform}/{user}/{repo}/ | ✅ ADOPTED |
| 2. Dedicated worktree subdirectory | ~/worktrees/ (mirrors) | ✅ ADOPTED |
| 3. Session manifests | YAML in ~/.claude/sessions/ | ✅ ADAPTED |
| 4. Lifecycle zones | Simplified (active/archived) | ✅ ADOPTED (simplified) |
| 5. Tool split | Separate homes for different artifacts | ✅ ADOPTED |
| 6. Prefix naming | Optional for worktrees | ✅ OPTIONAL |

**All D2 patterns incorporated**

### Against LangChain Patterns

| LangChain Pattern | D3 Implementation | Status |
|-------------------|-------------------|---------|
| File-based instruction passing | manifest.yaml files | ✅ ADOPTED |
| Learned patterns feedback | Retro-tasks (already doing) | ✅ EXISTING |
| Scratch pad for tool results | working/ directory | ✅ ADOPTED (user request!) |
| Context audit framework | context_audit in manifest | ✅ DESIGNED (implement later) |

**All LangChain patterns integrated or planned**

---

## Open Questions for D4

1. **Manifest auto-update frequency**
   - After every tool call? Every N calls?
   - On timer (like tmux-continuum)?
   - Decision: Start with event-based (tool call, artifact created)

2. **gwq integration depth**
   - Wrapper scripts or direct usage?
   - Fallback if gwq unavailable?
   - Decision: Wrapper for abstraction, git commands as fallback

3. **Session dashboard UI**
   - Terminal UI (TUI)?
   - Simple list output?
   - Decision: Start simple (list), add TUI later if needed

4. **Context audit automation**
   - Track automatically or manual?
   - Decision: Manual for now (avoid overhead)

5. **Pattern extraction**
   - Automated analysis or manual?
   - Decision: Manual for now (too complex for D4)

---

## Next Steps

**Immediate:**
- Quality control check (this document)
- User review and approval
- Proceed to D4 (detailed requirements)

**D4 Focus:**
- Detailed manifest schema
- Helper script specifications
- Migration script specifications
- Session dashboard requirements
- Archive process requirements

**Estimated D4 time:** 3-4 hours

---

**Status:** ✅ READY FOR QUALITY CONTROL

**Next:** Quality control checks + user review

**Then:** D4 - Solution Requirements

---
