# Artifact Taxonomy Analysis - From engram-research Repository

**Date:** 2025-12-02

**Purpose:** Identify artifact types and patterns from engram-research to inform workspace management design

**Source:** Analysis of /tmp/engram-research structure

---

## Executive Summary

**What we're tracking:** The user wants to categorize different types of work artifacts to know what needs structured storage. After analyzing the engram-research repository (34 top-level directories, hundreds of files), I've identified **12 distinct artifact types** across **4 major categories**.

**Key insight:** The current engram-research structure has organically evolved multiple competing taxonomies (by time, by type, by project, by phase) which creates navigation confusion. This validates the need for a clear workspace management system.

---

## User's Initial Categories (To Validate)

From user request:
1. ✅ **Research** - Shareable for future projects
2. ✅ **Process artifacts** - Useful for looking back, not usually future work
3. ✅ **Retrospectives** - Learning capture
4. ✅ **Tasks from retrospectives** - Tied to overall product/repo
5. ✅ **Intermediary steps** - For agent state restoration/context compression (LangChain)
6. ❓ **Others?** - To be identified from engram-research

---

## Discovered Artifact Categories

### Category 1: **Shareable Research** (Reusable Knowledge)

**Characteristics:** High value for future projects, evergreen, reference material

**Subtypes Found:**

1. **External Research** (`agentic-design-patterns/`, `ai-library-review-2025-11-23/`)
   - Third-party tools/patterns analysis
   - Academic papers summaries
   - Competitor analysis
   - Examples:
     - `/agentic-design-patterns/protocols/mcp-specification.md`
     - `/agentic-design-patterns/integrations/cursor.md`
     - `/ai-library-review-2025-11-23/`

2. **Domain Analysis** (`analysis/`)
   - Domain-specific research
   - Format comparisons
   - Examples:
     - `analysis/gherkin-human-ai-contracts-analysis.md`
     - `analysis/human-ai-contract-formats-research.md`
     - `analysis/pathfinder-ai-safety-ethics-review.md`

3. **Architecture Decisions** (`architecture/decisions/`)
   - ADRs (Architecture Decision Records)
   - 11 ADRs found: ADR-001 through ADR-011
   - Examples:
     - `ADR-001-core-terminology.md`
     - `ADR-006-eventbus-architecture.md`
     - `ADR-010-security-sandboxing.md`

4. **Investigation Results** (`investigations/`, `architecture/investigations/`)
   - Deep-dive technical investigations
   - Feasibility studies
   - Examples:
     - `hook-feasibility-research.md`
     - `investigations/huggingface-alignment-2025-11`

**Storage needs:**
- ✅ Git-backed for history
- ✅ Searchable/referenceable
- ✅ Cross-project accessible
- ✅ Organized by topic/domain

---

### Category 2: **Process Artifacts** (Project-Specific Documentation)

**Characteristics:** Useful for historical review, tied to specific project/initiative

**Subtypes Found:**

5. **Wayfinder Project Artifacts** (`wayfinder-projects/`)
   - Complete D1-D4, S4-S11 phase documentation
   - Examples:
     - `wayfinder-projects/bash-guidance/` (22 files)
     - `wayfinder-projects/workspace-design/dotfiles/` (20+ files)
   - Pattern: Full lifecycle documentation for completed projects

6. **Session Work** (`sessions/`)
   - Ad-hoc working sessions with deliverables
   - 8 session directories found
   - Examples:
     - `sessions/pilotage-self-improvement-2025-11/` (9 files)
     - `sessions/governance-restoration-2025-11/`
     - `sessions/persona-distillation-2025-11/`
   - Pattern: README + phase docs + deliverables

7. **Wayfinder Reviews** (`wayfinder-reviews/`)
   - Comprehensive review documents
   - Quality control artifacts
   - Examples:
     - 18 review documents in root
     - `wayfinder-reviews/plugin-architecture-2025-11/`
     - `wayfinder-reviews/review-concerns-taxonomy-2025-11/`

8. **Phase Progress Tracking** (Top-level PHASE-*.md files)
   - 15+ PHASE-*.md files at root
   - Examples:
     - `PHASE-2.5-PROGRESS.md`
     - `PHASE-10-COMPLETE.md`
     - `PHASE-11-RETROSPECTION.md`
   - Pattern: Mix of completion markers, progress docs, retrospectives

9. **Course Corrections** (Top-level COURSE-CORRECTION-*.md files)
   - Mid-project trajectory adjustments
   - Examples:
     - `COURSE-CORRECTION-POST-PHASE-8.md`
     - `FINAL-COURSE-CORRECTION.md`

**Storage needs:**
- ✅ Organized by project/initiative
- ✅ Timestamped
- ✅ Git-backed for audit trail
- ⚠️ Clear active vs archived distinction

---

### Category 3: **Learning & Improvement** (Retrospective Knowledge)

**Characteristics:** Lessons learned, process improvements, future action items

**Subtypes Found:**

10. **Retrospectives** (`retrospectives/`)
    - Structured retrospective documents
    - Examples:
      - `retrospectives/2025-11/` (15 files)
      - `retrospectives/2025-11-retrospective-archive/`
    - Pattern: Organized by time period

11. **Improvement Tasks** (External: ~/retro-tasks/)
    - **Note:** Now in separate repo (github.com/vbonnet/retro-tasks)
    - Bead-structured improvement tasks
    - Examples: WF-001 through WF-010
    - Pattern: Structured, prioritized, trackable

12. **Case Studies** (`case-studies/`)
    - Specific learnings from incidents
    - Examples:
      - `case-studies/CASE-STUDY-domain-expert-early-invocation-2025-11-22.md`
    - Also embedded in sessions:
      - `sessions/pilotage-self-improvement-2025-11/CASE-STUDY-pilotage-self-improvement.md`

13. **Postmortems** (Top-level POSTMORTEM-*.md files)
    - Failure analysis documents
    - Examples:
      - `POSTMORTEM-LANGCHAIN-DUPLICATION-MISS-2025-11-23.md`
      - `POSTMORTEM-READY-SUMMARY.md`

**Storage needs:**
- ✅ Timestamped
- ✅ Cross-linkable to source projects
- ✅ Actionable items extracted to task tracking
- ✅ Searchable by topic/pattern

---

### Category 4: **Agent Context & State** (Operational Artifacts)

**Characteristics:** For agent state restoration, context compression, working memory

**Subtypes Found:**

14. **Session Archives** (`session-archives/`)
    - Session planning docs
    - Session prompts
    - Resumption instructions
    - Examples:
      - `session-archives/2025-12-01/` (9 files)
      - `project1-monday-demo-prep.md`
      - `prompt-project4-platform.md`
      - `README-session-resumption.md`

15. **Ephemeral Instructions** (`engram-ephemeral-instructions*/`)
    - Temporary agent guidance
    - 3 directories found:
      - `engram-ephemeral-instructions/`
      - `engram-ephemeral-instructions-pilotage/`
      - `engram-ephemeral-instructions-pilotage-v2/`
    - Pattern: Version-stamped temporary configs

16. **Progress Tracking** (Top-level S*-*.md files)
    - SDLC phase progress docs
    - Examples:
      - `S7-PROGRESS.md`
      - `S7-implementation-plan.md`
      - `S6-DESIGN-LANGCHAIN-INTEGRATION.md`

17. **Analysis/Brainstorm Docs** (Top-level *-ANALYSIS.md, *-BRAINSTORM.md)
    - Working documents for active thinking
    - Examples:
      - `ANALYSIS-plugin-conversion-priorities.md`
      - `IMPROVEMENT-BRAINSTORM-SESSION.md`
      - `GOVERNANCE-RESTORATION-ANALYSIS.md`

18. **Implementation Scorecards** (Top-level *-SCORECARD.md, *-AUDIT.md)
    - Quality checkpoints
    - Examples:
      - `IMPLEMENTATION-SCORECARD-PHASE-3.md`
      - `PHASE-10-TEST-AUDIT.md`
      - `PLUGIN-AUDIT-FINDINGS.md`

**Storage needs:**
- ✅ Organized by session/time
- ⚠️ Clear ephemeral vs permanent distinction
- ✅ Quick access for active work
- ⚠️ Automatic archival when session complete

---

## Organizational Problems Observed

### Problem 1: **Competing Taxonomies**

engram-research has organically evolved **4 different organizational schemes** that compete:

1. **By artifact type:**
   - `architecture/`, `analysis/`, `case-studies/`, `retrospectives/`, `designs/`

2. **By time:**
   - `archived/2025-10-core/`, `archived/2025-11-early/`
   - `session-archives/2025-12-01/`
   - `retrospectives/2025-11/`

3. **By project/initiative:**
   - `sessions/pilotage-self-improvement-2025-11/`
   - `wayfinder-projects/bash-guidance/`
   - `sessions/governance-restoration-2025-11/`

4. **By process phase:**
   - Top-level `PHASE-*.md` files
   - Top-level `S*-*.md` files

**Result:** Hard to navigate, unclear where new artifacts go

---

### Problem 2: **Top-Level Clutter**

**Found at root level:** 70+ markdown files with mixed purposes:
- PHASE-*.md (15 files)
- ANALYSIS-*.md (6 files)
- COURSE-CORRECTION-*.md (5 files)
- POSTMORTEM-*.md (2 files)
- S*-*.md (10+ files)
- Plus: README, CHANGELOG, ORGANIZATION.md, etc.

**Pattern:** Ad-hoc creation during active work, never archived

---

### Problem 3: **Unclear Active vs Archived**

**Observation:** Multiple "archive" strategies coexist:
- `archived/` directory (7 subdirectories)
- `retrospectives/2025-11-retrospective-archive/`
- Top-level completion markers: `PHASE-10-COMPLETE.md`

**Confusion:** Is `sessions/pilotage-2025-11/` active or archived?

---

### Problem 4: **Version Proliferation**

**Example:** Ephemeral instructions
- `engram-ephemeral-instructions/`
- `engram-ephemeral-instructions-pilotage/`
- `engram-ephemeral-instructions-pilotage-v2/`

**Pattern:** Versioning via directory naming instead of git

---

### Problem 5: **Wayfinder Projects Homeless**

**Recent discovery:** Complete Wayfinder projects had no clear home:
- bash-guidance: Complete D1-S11, temporarily dumped to engram-research
- dotfiles: Complete D1-S11, stuck in ~/workspace-design/

**Created:** `wayfinder-projects/` as bandaid solution

---

## Artifact Type Matrix

| Artifact Type | Reusable? | Shareable? | Ephemeral? | Current Location | Ideal Home? |
|---------------|-----------|------------|------------|------------------|-------------|
| External research | ✅ Yes | ✅ Yes | ❌ No | engram-research | Knowledge base |
| ADRs | ✅ Yes | ✅ Yes | ❌ No | engram-research/arch | Repo docs/ |
| Investigations | ✅ Yes | ✅ Yes | ❌ No | engram-research | Knowledge base |
| Wayfinder artifacts | ❌ No | ⚠️ Maybe | ❌ No | wayfinder-projects | Project archive |
| Session work | ❌ No | ❌ No | ❌ No | sessions/ | Session archive |
| Reviews | ⚠️ Maybe | ✅ Yes | ❌ No | wayfinder-reviews | Quality archive |
| Phase tracking | ❌ No | ❌ No | ✅ Yes | Root clutter | Session context |
| Course corrections | ⚠️ Maybe | ⚠️ Maybe | ❌ No | Root clutter | Session archive |
| Retrospectives | ✅ Yes | ✅ Yes | ❌ No | retrospectives/ | ✅ Good |
| Retro tasks | ✅ Yes | ✅ Yes | ❌ No | ~/retro-tasks (git) | ✅ Good |
| Case studies | ✅ Yes | ✅ Yes | ❌ No | case-studies/ | ✅ Good |
| Postmortems | ✅ Yes | ✅ Yes | ❌ No | Root clutter | Learnings archive |
| Session archives | ❌ No | ❌ No | ⚠️ After done | session-archives/ | ✅ Good pattern |
| Ephemeral instruct | ❌ No | ❌ No | ✅ Yes | Multiple dirs | /tmp or auto-clean |
| Progress tracking | ❌ No | ❌ No | ✅ Yes | Root clutter | Session state |
| Brainstorm/analysis | ⚠️ Maybe | ❌ No | ⚠️ After done | Root clutter | Session working |
| Scorecards/audits | ⚠️ Maybe | ⚠️ Maybe | ❌ No | Root clutter | Quality archive |

---

## Recommendations for Workspace Management System

### Recommendation 1: **Clear Artifact Lifecycle**

Define 4 lifecycle stages:
1. **Working** - Active creation (ephemeral, fast access)
2. **Session-complete** - Done but recent (session archive)
3. **Archived** - Historical reference (compressed/organized)
4. **Knowledge** - Extracted learnings (searchable/reusable)

### Recommendation 2: **Separation of Concerns**

Create distinct homes for:
1. **Shareable knowledge** (engram-research or knowledge repo)
   - Research, ADRs, investigations, case studies
   - Cross-project accessible
   - Topic-organized

2. **Project artifacts** (project-specific archives)
   - Wayfinder projects
   - Session work
   - Reviews/audits
   - Time-organized with project context

3. **Learning/improvement** (process improvement tracking)
   - Retrospectives
   - Tasks (retro-tasks repo works well)
   - Postmortems

4. **Active session state** (fast access, auto-cleanup)
   - Session planning docs
   - Progress tracking
   - Brainstorms/analysis
   - Ephemeral instructions

### Recommendation 3: **Automated Lifecycle Transitions**

**On session completion:**
- Archive session planning docs → session-archives/{date}/
- Extract learnings → retrospectives/
- Create improvement tasks → retro-tasks/
- Move shareable research → knowledge base
- Clean up ephemeral working docs

### Recommendation 4: **Structured Naming Conventions**

Instead of ad-hoc file naming at root:
```
# Bad (current)
PHASE-10-COMPLETE.md
ANALYSIS-plugin-priorities.md
COURSE-CORRECTION-POST-PHASE-8.md

# Good (proposed)
sessions/{session-id}/phase-10-complete.md
sessions/{session-id}/working/analysis-plugin-priorities.md
sessions/{session-id}/course-corrections/post-phase-8.md
```

### Recommendation 5: **Index/Navigation Layer**

Given the volume (hundreds of artifacts), need:
- **Index files** - Generated summaries per directory
- **Search capability** - Full-text search across archives
- **Cross-references** - Link related artifacts
- **Metadata** - Timestamps, tags, project association

---

## Artifact Types to Track in Workspace System

Based on this analysis, the workspace management system should track:

### Core Types (Must Have)

1. **Wayfinder Projects** (D1-D4, S4-S11 artifacts)
   - Location: Dedicated archive (wayfinder-projects/)
   - Lifecycle: Keep indefinitely (reference value)
   - Organization: By project name, then by phase

2. **Session Work** (Ad-hoc working sessions)
   - Location: Session archives by date
   - Lifecycle: Archive on completion
   - Organization: sessions/{name-date}/

3. **Session Planning/Resumption** (Active work coordination)
   - Location: Active session directory → archive on complete
   - Lifecycle: Ephemeral while active, archive when done
   - Organization: By session ID

4. **Retrospectives** (Learning capture)
   - Location: Dedicated retrospectives/ directory
   - Lifecycle: Keep indefinitely
   - Organization: By time period

5. **Improvement Tasks** (Actionable follow-ups)
   - Location: Separate git repo (retro-tasks pattern works)
   - Lifecycle: Track until implemented
   - Organization: By Bead structure

### Research Types (Knowledge Base)

6. **External Research** (Third-party analysis)
7. **Domain Investigations** (Deep-dives)
8. **ADRs** (Architecture decisions)
9. **Case Studies** (Specific learnings)
10. **Postmortems** (Failure analysis)

### Working Types (Ephemeral/Auto-cleanup)

11. **Progress Tracking** (Phase status)
12. **Brainstorm/Analysis Docs** (Working thoughts)
13. **Ephemeral Instructions** (Temporary agent config)
14. **Scorecards/Audits** (Quality checkpoints)

---

## Pattern: What Makes engram-research Hard to Navigate

**Symptom:** User says "engram-research repo exists but is unstructured and hard to navigate"

**Root causes identified:**

1. **No clear "working" vs "done" separation**
   - Root level has both active and historical docs
   - No way to know what's current

2. **Competing organizational schemes**
   - By type, by time, by project, by phase all coexist
   - Unclear which to use for new artifacts

3. **Ad-hoc file creation at root**
   - 70+ root-level markdown files
   - Created during active work, never moved

4. **Versioning via directory names**
   - `-pilotage`, `-pilotage-v2` instead of git
   - Hard to find "current" version

5. **No index/navigation**
   - README.md doesn't map structure
   - No ORGANIZATION.md at top-level (exists but buried)

---

## Key Insights for D2

### Insight 1: **Need Multiple "Homes" with Clear Boundaries**

Not one archive, but specialized locations:
- **Active work** (session state, fast access)
- **Completed projects** (Wayfinder artifacts, session work)
- **Knowledge base** (research, ADRs, learnings)
- **Process improvement** (retro tasks, separate repo)

### Insight 2: **Lifecycle Management is Critical**

The transition from working → complete → archived → knowledge must be:
- **Automated** (not manual cleanup)
- **Clear** (know what stage each artifact is in)
- **Reversible** (can reference archived work)

### Insight 3: **engram-research Needs Refactoring**

The current structure has organically evolved without a clear taxonomy. The workspace management system should:
- Define the taxonomy
- Provide migration tools
- Maintain navigation/index

### Insight 4: **Wayfinder Projects are First-Class Citizens**

Complete D1-D4, S4-S11 projects should have:
- Dedicated archive location
- Searchable/referenceable
- Cross-linkable to retro tasks
- Easy to browse past projects

### Insight 5: **Session Context is Undervalued**

Current: 400+ claude-XXXX-cwd files (just session IDs)

**Should track:**
- What project/work is session doing?
- What artifacts has it created?
- What worktree is it using?
- Status (active/abandoned/complete)?
- How to resume?

---

## Comparison to User's Initial List

**User proposed tracking:**
1. ✅ Research (shareable) - **Confirmed:** Types 6-10
2. ✅ Process artifacts - **Confirmed:** Types 1-2, 7, 9
3. ✅ Retrospectives - **Confirmed:** Type 4
4. ✅ Tasks from retros - **Confirmed:** Type 5 (retro-tasks pattern works)
5. ✅ Intermediary steps (LangChain) - **Confirmed:** Types 11-14
6. ✅ **PLUS discovered:**
   - Wayfinder projects (major omission!)
   - Session work (ad-hoc sessions)
   - Case studies & postmortems (learnings)
   - Quality artifacts (reviews, audits, scorecards)

---

## Next Steps for D2

Use this taxonomy to evaluate solutions for:

1. **Directory structure** - Where does each artifact type live?
2. **Lifecycle automation** - How to transition between stages?
3. **Navigation** - How to find artifacts across hundreds of files?
4. **Session tracking** - How to link sessions ↔ artifacts ↔ worktrees?
5. **Knowledge extraction** - How to surface learnings from archives?

---

**Analysis Complete:** 2025-12-02

**Artifact Types Identified:** 14 types across 4 categories

**Key Finding:** Need structured lifecycle management with multiple specialized "homes"

---
