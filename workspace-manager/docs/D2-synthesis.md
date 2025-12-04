# D2: Synthesis - Patterns & Key Insights

**Date:** 2025-12-02

**Project:** Workspace & Session Management System

**Phase:** D2 - Synthesis

**Purpose:** Extract patterns from research, validate against D1 requirements

---

## Executive Summary

**Research completed:** 4 areas (directory structure, git worktrees, session management, knowledge management)

**Key insight:** The community has already solved many of our problems - we need **integration** more than **invention**

**Top findings:**
1. **Hierarchical directory structure** (`~/src/github/username/`) is proven and scalable
2. **gwq tool** solves worktree discovery/cleanup problem
3. **Session manifest pattern** (from tmuxp) applicable to Claude Code sessions
4. **Git-backed Markdown** aligns with current engram-research approach
5. **Tool split by use case** - don't force one structure for all artifact types

---

## Patterns Discovered

### Pattern 1: Hierarchical Platform-Mirroring Directory Structure

**Description:**
Organize repos by mirroring their hosting platform structure on disk.

**Format:**
```
~/src/
├── github/
│   ├── username1/
│   │   ├── repo1/
│   │   └── repo2/
│   └── username2/
│       └── repo3/
└── gitlab/
    └── organization/
        └── repo4/
```

**Used by:**
- GoLang community (GOPATH convention)
- Many developers in 2025 (per research)
- Scales to hundreds of repos across multiple platforms

**Pros:**
- ✅ Clear work/personal separation (different usernames)
- ✅ Scales as projects grow
- ✅ Mirrors familiar online structure
- ✅ Easy to script (`~/src/github/vbonnet/*` vs `~/src/github/[REDACTED_EMPLOYER]-src/*`)
- ✅ No conflicts between repos with same name from different users

**Cons:**
- ⚠️ Deeper directory nesting
- ⚠️ Requires typing more path segments

**Applicability:** HIGH - Directly addresses D1 Problem: "Where to clone repos?"

**Recommendation:** ADOPT

---

### Pattern 2: Dedicated Worktree Subdirectory

**Description:**
Keep worktrees separate from main clones in dedicated subdirectory, not as siblings.

**Format:**
```
~/src/github/vbonnet/engram/          # Main clone
~/worktrees/github/vbonnet/engram/    # Worktrees root
├── feature-bash-guidance/            # Worktree 1
├── fix-telemetry/                    # Worktree 2
└── review-pr-123/                    # Worktree 3
```

**Used by:**
- Git worktree best practices community
- gwq tool (expects organized worktrees)

**Pros:**
- ✅ Main repo stays clean
- ✅ Easy to see all worktrees for a repo
- ✅ Clear separation: permanent (~/src) vs temporary (~/worktrees)
- ✅ Can clean up entire worktree directory when needed

**Cons:**
- ⚠️ Worktrees not siblings to main clone (some devs prefer siblings)
- ⚠️ Requires mirroring directory structure

**Applicability:** HIGH - Addresses D1 Problem: "Where to create worktrees?"

**Recommendation:** ADOPT (with hierarchical mirroring)

---

### Pattern 3: Session Manifest Files

**Description:**
Store session metadata in structured files (JSON/YAML) for resumption.

**Inspired by:** tmuxp (tmux session configs)

**Example manifest:**
```yaml
session_id: "claude-abc123"
created: "2025-12-02T10:30:00Z"
project: "bash-guidance-consolidation"
worktree: "/home/user/worktrees/github/vbonnet/engram/feature-bash-guidance"
branch: "feature/bash-guidance-consolidation"
status: "active"
last_activity: "2025-12-02T14:22:00Z"
artifacts:
  - "S7-plan.md"
  - "S8-implementation.md"
tags:
  - "wayfinder"
  - "engram"
```

**Used by:**
- tmuxp, tmuxinator (terminal multiplexer session managers)
- IDE workspace configs (VS Code .code-workspace files)

**Pros:**
- ✅ Human-readable and editable
- ✅ Git-backed for portability
- ✅ Easy to script against
- ✅ Can store rich metadata (not just session ID)
- ✅ Restorability across machines

**Cons:**
- ⚠️ Requires active maintenance (could go stale)
- ⚠️ Manual creation vs automatic

**Applicability:** HIGH - Addresses D1 Problem: "Session tracking" & "Work resumability"

**Recommendation:** ADAPT for Claude Code sessions

---

### Pattern 4: Lifecycle-Based Storage Zones

**Description:**
Different "homes" for artifacts based on lifecycle stage, not by type alone.

**Zones:**
```
~/.claude/sessions/active/          # Currently running sessions
~/.claude/sessions/recent/          # Completed < 7 days ago
~/.claude/sessions/archived/        # Completed > 7 days ago

~/projects/wayfinder/active/        # Active Wayfinder projects
~/projects/wayfinder/complete/      # Completed projects

~/knowledge/                        # Shareable research (git-backed)
~/ephemeral/                        # Safe to delete anytime
```

**Inspired by:**
- Knowledge management systems (active vs archived)
- Email clients (inbox, sent, archive)
- Document retention policies

**Pros:**
- ✅ Clear expectations for each zone
- ✅ Easy cleanup (delete ~/ephemeral/* anytime)
- ✅ Automated transitions (move active → recent → archived)
- ✅ Addresses "where does this go?" question

**Cons:**
- ⚠️ Requires automation to transition between zones
- ⚠️ Multiple places to search

**Applicability:** HIGH - Addresses D1 Problem: "Artifact lifecycle management"

**Recommendation:** ADOPT with automation

---

### Pattern 5: Tool Split by Use Case

**Description:**
Don't force all artifact types into one tool/structure - use specialized tools.

**Example split:**
- **Git repos** - Version-controlled code
- **Git-backed Markdown** - Documentation, research, notes (engram-research)
- **Task tracker** - Improvement tasks (retro-tasks repo)
- **Session state** - Active work metadata (manifest files)

**Inspired by:**
- Obsidian/Notion/Logseq split (dev notes / team docs / daily logs)
- Unix philosophy (do one thing well)

**Pros:**
- ✅ Each tool optimized for its purpose
- ✅ No forcing square pegs into round holes
- ✅ Can evolve each independently

**Cons:**
- ⚠️ More tools to learn
- ⚠️ Cross-tool integration complexity

**Applicability:** MEDIUM - Validates having engram-research separate from retro-tasks

**Recommendation:** ADOPT (already doing this)

---

### Pattern 6: Prefix-Based Naming Conventions

**Description:**
Use consistent prefixes to indicate purpose/status at a glance.

**Examples:**
- Worktrees: `feature-`, `fix-`, `review-`, `experiment-`
- Sessions: `wayfinder-`, `research-`, `bugfix-`
- Artifacts: `D1-`, `S4-`, `PHASE-`, `RETRO-`

**Used by:**
- Git branch naming conventions
- File naming in large projects
- Git worktree best practices

**Pros:**
- ✅ Instant visual categorization
- ✅ Easy to filter/search (`ls feature-*`)
- ✅ Self-documenting

**Cons:**
- ⚠️ Requires discipline to maintain
- ⚠️ Can feel bureaucratic

**Applicability:** MEDIUM - Nice-to-have for organization

**Recommendation:** ADOPT for worktrees and key artifacts

---

## Key Insights

### Insight 1: The /tmp/ Problem Needs Multiple Solutions

**Problem:** Valuable work in ephemeral /tmp/ at risk of loss

**Why /tmp/?** "No clear right place for active work" → defaults to /tmp/

**Solutions discovered:**
1. **Dedicated ephemeral zone** - `~/ephemeral/` that's safe to delete but persistent across reboots
2. **Session manifests** - Track what's in /tmp/ so it can be recovered/archived
3. **Auto-archive on reboot** - Cron job to archive /tmp/ work before it's lost

**Insight:** Not one solution, but a system of safeguards

---

### Insight 2: Worktree Management is Mostly Solved

**Problem:** No visibility into worktrees, which are merged/stale, cleanup needed

**Solution discovered:** **gwq tool** + git built-ins solve this

**What gwq provides:**
- Fuzzy finder for all worktrees
- Status dashboard (branch merged? stale?)
- Cleanup automation
- "Perfect for AI coding workflows with parallel branches"

**Insight:** Don't build custom tool - use gwq + wrapper scripts

---

### Insight 3: Session Tracking ≠ Session Management

**Current:** 400+ claude-XXXX-cwd files (just session IDs)

**Research reveals two separate needs:**

1. **Session Management** (tmux-like)
   - Create, attach, detach sessions
   - Persist across restarts
   - **Not applicable:** Claude Code manages its own sessions

2. **Session Tracking** (metadata)
   - What is each session working on?
   - Where are its artifacts?
   - How to resume context?
   - **Very applicable:** We need this!

**Insight:** We need session *tracking* (manifest files), not session *management* (that's Claude's job)

---

### Insight 4: Git-Backed Everything (Almost)

**Pattern observed:** Successful knowledge systems are git-backed

**What should be git-backed:**
- ✅ Research & learnings (engram-research)
- ✅ Improvement tasks (retro-tasks)
- ✅ Wayfinder project artifacts (could be)
- ✅ Session manifests (portability)
- ⚠️ Active session state (maybe - could change frequently)

**What shouldn't:**
- ❌ Binary artifacts
- ❌ Truly ephemeral scratchpads
- ❌ Sensitive data (unless encrypted)

**Insight:** Default to git-backed Markdown unless there's a reason not to

---

### Insight 5: Hierarchical Structure Mirrors Mental Model

**Why ~/src/github/vbonnet/ works:**
- Mirrors where code lives online
- Matches how developers think ("my GitHub repos")
- Provides natural categorization

**Application to worktrees:**
- `~/worktrees/github/vbonnet/engram/feature-x` mirrors main clone location
- Mental model: "Worktrees for this repo live in the same relative path"

**Insight:** Good directory structure reflects how users already think

---

### Insight 6: Automation Prevents Decay

**Observation from research:**
- tmux-continuum: Auto-saves every 15 min
- Git worktree: Auto-prunes stale info
- Knowledge tools: Auto-sync, auto-index

**Application:**
- Session manifests should auto-update (not manual)
- Worktree status should auto-refresh
- Lifecycle transitions should auto-trigger (active → archived)

**Insight:** Manual processes decay - automate the critical paths

---

## Validation Against D1 Requirements

### D1 Problem 1: Directory Organization ✅

**D1 stated:** "Where to clone repos, where to create worktrees, how to organize work/personal"

**D2 solutions found:**
- ✅ Clone location: `~/src/github/username/repo` (hierarchical pattern)
- ✅ Worktree location: `~/worktrees/github/username/repo/branch` (dedicated subdirectory)
- ✅ Work/personal separation: Different username paths

**Status:** SOLVED by Pattern 1 & Pattern 2

---

### D1 Problem 2: Worktree Lifecycle Management ✅

**D1 stated:** "No visibility into which worktrees exist, which are abandoned/merged, cleanup needed"

**D2 solutions found:**
- ✅ Discovery: `gwq` tool provides fuzzy finder + status dashboard
- ✅ Merged detection: `git branch --merged` built-in
- ✅ Cleanup: `gwq` automation + `git worktree remove` best practices

**Status:** SOLVED by gwq tool + Pattern 2

---

### D1 Problem 3: Session Tracking ✅

**D1 stated:** "No way to know which Claude sessions are working in which worktrees"

**D2 solutions found:**
- ✅ Session manifest pattern (from tmuxp)
- ✅ Metadata format: YAML/JSON files
- ✅ Storage: Git-backed for portability

**Status:** APPROACH IDENTIFIED (Pattern 3) - needs D3 design

---

### D1 Problem 4: Work Resumability ✅

**D1 stated:** "Difficult to transfer context between machines or resume work after interruption"

**D2 solutions found:**
- ✅ Git-backed session manifests (portable)
- ✅ Documented worktree locations (deterministic paths)
- ✅ Artifact tracking in manifests

**Status:** APPROACH IDENTIFIED (Pattern 3 + git-backed artifacts)

---

### D1 Problem 5: Persistent Logging ⚠️

**D1 stated:** "Currently logs to /tmp/ (ephemeral), lost on reboot"

**D2 solutions found:**
- ⚠️ No direct solution found in research
- 💡 Insight: Combine ephemeral zone + session manifests + auto-archive

**Status:** APPROACH IDENTIFIED (Pattern 4 lifecycle zones) - needs D3 design

---

### D1 Problem 6: Completed Project Artifacts ✅

**D1 stated:** "No clear home for finished Wayfinder projects"

**D2 solutions found:**
- ✅ Lifecycle-based storage (Pattern 4)
- ✅ Git-backed project archives
- ✅ engram-research/wayfinder-projects/ already working

**Status:** PARTIALLY SOLVED (current structure works, could formalize)

---

## Coverage Assessment

**D1 Problems:** 6 total

**D2 Solutions:**
- ✅ Fully solved: 3 (directory org, worktree lifecycle, completed artifacts)
- ✅ Approach identified: 3 (session tracking, resumability, logging)

**Coverage:** 100% of problems have viable approaches

**Confidence:** HIGH - research validates feasibility

---

## Open Questions for D3

### Question 1: Session Manifest Details

**Decide:**
- What metadata to capture?
- Update frequency (auto-save every 15min like tmux-continuum)?
- Where to store? (`~/.claude/sessions/` or git-backed repo?)
- How to link session ↔ worktree ↔ artifacts?

### Question 2: Ephemeral Zone Strategy

**Decide:**
- Create `~/ephemeral/` as safer /tmp/ alternative?
- Or keep using /tmp/ with auto-archive safeguards?
- What triggers archival (time-based? session-end? reboot hook?)

### Question 3: Integration vs Standalone

**Decide:**
- Build as Engram plugin?
- Standalone shell scripts?
- Combination (scripts + optional Engram integration)?

### Question 4: Automation Level

**Decide:**
- Fully automatic (no user intervention)?
- Prompted (ask before cleanup/archive)?
- Manual with helpers (tools to make it easy)?

### Question 5: Migration Path

**Decide:**
- How to migrate current /tmp/ mess to new structure?
- Clean slate or gradual transition?
- Tooling to help migration?

---

## Recommended Patterns for D3

Based on research and validation, recommend adopting:

1. ✅ **Pattern 1:** Hierarchical directory structure (`~/src/github/username/`)
2. ✅ **Pattern 2:** Dedicated worktree subdirectory (mirrors hierarchy)
3. ✅ **Pattern 3:** Session manifest files (YAML, git-backed)
4. ✅ **Pattern 4:** Lifecycle-based storage zones
5. ✅ **Pattern 5:** Tool split by use case (already doing)
6. ⚠️ **Pattern 6:** Prefix-based naming (optional, for organization)

**Tools to adopt:**
1. ✅ **gwq** - Git worktree manager
2. ✅ **Git-backed Markdown** - Documentation (already doing)
3. ⚠️ **Custom scripts** - Session manifest management (to be designed in D3)

---

## Next Steps for D3

**D3 Focus:** Make specific design decisions based on these patterns

**Key decisions:**
1. Exact directory structure (paths, naming)
2. Session manifest schema (fields, format)
3. Automation strategy (what, when, how)
4. Integration approach (Engram plugin vs standalone)
5. Migration plan (move from current chaos to new structure)

**Deliverable:** Detailed design spec ready for D4 requirements

---

**Synthesis Status:** ✅ COMPLETE

**Ready for:** Multi-persona review + D3 Approach Decision

---
