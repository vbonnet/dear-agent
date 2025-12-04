# Current State Snapshot - Before Workspace Management Implementation

**Date:** 2025-12-02

**Purpose:** Document current messy state as input for D2 Solutions Search

**Status:** Cleaned up, but documenting what was found

---

## TL;DR - The Mess We Found

**Problem:** Work scattered across ~/, /tmp/, multiple git repos, no clear structure

**Critical finding:** Valuable work living in ephemeral /tmp/ that would be lost on reboot!

**Quick fix applied:** Archived everything to engram-research, but this is a bandaid - need proper structure

---

## What We Found (Before Cleanup)

### 1. Ephemeral Work in /tmp/ (❗ CRITICAL)

**Risk:** Lost on reboot!

**Found:**
- `/tmp/engram-install/` - Main engram repo clone (branch: wayfinder-prototype)
- `/tmp/engram-research/` - Research repo with extensive docs
- `/tmp/bash-guidance-worktree/` - Git worktree from completed Wayfinder project
- `/tmp/engram-install-fix-init/` - Another git clone/worktree
- `/tmp/engram-demo-test/` - Demo testing directory
- 400+ `claude-XXXX-cwd` files - Session tracking breadcrumbs

**Why /tmp/?**
- Quick clones to avoid polluting home directory
- No clear "right place" for active work
- Avoiding premature structure decisions

### 2. Scattered Markdown Files in ~/ (11 files)

**Found:**
- `project1-monday-demo-prep.md` - Session planning doc
- `project2-pilotage-self-improvement.md` - Session planning
- `project3-repository-cleanup.md` - Session planning
- `project4-core-platform-development.md` - Session planning
- `prompt-project{1-4}.md` - 4 session prompts
- `README-session-resumption.md` - Meta-doc for resuming work
- `README.md` - Dotfiles README (leaked from chezmoi)
- `bash-tool-guidance-analysis.md` - Research for bash-guidance project

**Why scattered?**
- No clear "session archives" location
- Some leaked from other directories (dotfiles README)
- Ad-hoc creation during active sessions

### 3. Half-Organized Directories

**~/workspace-design/** (partially working)
```
workspace-design/
├── dotfiles/              # ✅ Complete Wayfinder project (D1-S11)
└── workspace-management/  # ⚠️ Incomplete (only D1 exists)
```

**Issue:** Name "workspace-design" made sense for first project, but not general

**~/bash-guidance-consolidation/** (orphaned)
- Complete Wayfinder project (D1-D4, S4-S11)
- Implementation already merged into engram repo
- Artifacts had no clear "home" after completion

**~/retro-tasks/** (good idea, incomplete)
- Wayfinder process improvements
- ✅ Fixed: Now git-backed at github.com/vbonnet/retro-tasks

**~/engrams/** (local work)
- `go/error-handling.ai.md` - Local engram development
- Not clear if this should be pushed or stay local

### 4. Git Repository Locations

**Clone locations observed:**
- `/tmp/engram-install/` - Main repo
- `/tmp/engram-research/` - Research repo
- `/tmp/bash-guidance-worktree/` - Worktree from engram repo
- `~/vpaste-wayfinder-autonomous/` - Different project
- `~/.local/share/chezmoi/` - Dotfiles source (managed by chezmoi)

**Worktree locations:**
- `/tmp/bash-guidance-worktree/`
- `~/worktrees/dotfiles-test/`

**Pattern:** No consistent clone/worktree strategy

### 5. Session Tracking Files

**Found in /tmp/:**
- 400+ `claude-XXXX-cwd` files (just session IDs)
- No actual session context stored
- No way to see "what sessions exist" or "what are they working on"

**Missing:**
- Which session is working on which worktree?
- What's the status of each session?
- How to resume a session?

---

## Cleanup Actions Taken

### Immediate (2025-12-02)

**Protected critical data:**
1. ✅ Initialized ~/retro-tasks/ as git repo → github.com/vbonnet/retro-tasks (private)
2. ✅ Archived bash-guidance Wayfinder project to engram-research
3. ✅ Archived session planning docs to engram-research/session-archives/2025-12-01/
4. ✅ Removed duplicate dotfiles docs (README.md, docs/TROUBLESHOOTING.md)
5. ✅ Cleaned up ~/bash-guidance-consolidation/ (now in engram-research)

**Result:**
- All work pushed to remote (safe from loss)
- Home directory much cleaner
- But: Still no long-term structure!

---

## Problems This Workspace Management System Needs to Solve

### P1: Clone/Worktree Organization

**Current:** Random locations (/tmp/, ~/, ~/worktrees/)

**Need:**
- Consistent place to clone repos
- Consistent place for worktrees
- Clear work/ vs personal/ separation
- Doesn't overwhelm any single directory

### P2: Completed Wayfinder Project Artifacts

**Current:** No clear home (bash-guidance dumped to engram-research as bandaid)

**Need:**
- Structured archive for completed Wayfinder projects
- Searchable/referenceable
- Git-backed
- Organized by project/date

### P3: Session Planning/Resumption Docs

**Current:** Loose files in ~/, now archived to engram-research/session-archives/

**Need:**
- Active vs archived session docs
- Clear resumption workflow
- Associated with specific worktrees/sessions

### P4: Session Tracking

**Current:** 400+ `claude-XXXX-cwd` files with no context

**Need:**
- Know what sessions exist
- Know what each is working on
- Know status (active/abandoned/completed)
- Ability to resume or clean up

### P5: Ephemeral /tmp/ Usage

**Current:** Critical work in /tmp/ (lost on reboot!)

**Need:**
- Safer temporary work location
- Easy cleanup of truly temporary files
- Clear distinction: permanent vs ephemeral

### P6: Worktree Lifecycle

**Current:** No visibility into which worktrees are merged/abandoned

**Need:**
- List all worktrees per repo
- See which branches are merged
- Easy cleanup of abandoned worktrees
- Status tracking

---

## Patterns Observed

### Pattern 1: "Don't want to pollute home" → /tmp/

**Symptom:** Cloning to /tmp/ to avoid deciding on structure

**Root cause:** No clear "right place" for active work

**Risk:** Work lost on reboot

### Pattern 2: Ad-hoc Session Planning Docs

**Symptom:** project*.md, prompt*.md files in ~/

**Root cause:** No standard location for session artifacts

**Result:** Clutter, lost context

### Pattern 3: Completed Project Artifacts Homeless

**Symptom:** bash-guidance-consolidation/ sitting orphaned

**Root cause:** No "Wayfinder projects archive" location

**Bandaid:** Dumping to engram-research

### Pattern 4: Duplicate Docs Leak Out

**Symptom:** README.md, docs/ in ~/ (from chezmoi)

**Root cause:** Tools (chezmoi) applying files, no clear boundaries

### Pattern 5: Local Work vs Repository Work Unclear

**Symptom:** ~/engrams/go/error-handling.ai.md - push or not?

**Root cause:** No clear policy on local-only vs shared work

---

## Key Insights for D2

### Insight 1: Need Multiple Workspace Zones

Not one structure, but zones:
- **Active work:** Current worktrees, session docs
- **Permanent repos:** Long-lived clones
- **Archives:** Completed projects, old sessions
- **Ephemeral:** True temporary files (safe to delete)
- **Reference:** Docs, notes, research

### Insight 2: Session ↔ Worktree Linkage Critical

**Need to know:**
- Which Claude session is in which worktree?
- What's the session working on?
- How to resume that exact context?

### Insight 3: Wayfinder Projects Need Structure

**Observed:**
- dotfiles: Complete project with all artifacts
- bash-guidance: Complete project, needed archiving
- workspace-management: In progress (only D1)

**Need:**
- Active Wayfinder projects location
- Completed Wayfinder projects archive
- Easy navigation and reference

### Insight 4: Git Worktrees Need Lifecycle Management

**Problems:**
- Don't know which worktrees exist
- Don't know which branches are merged
- No easy cleanup mechanism
- Worktrees can outlive their purpose

### Insight 5: Cross-Machine Context

**Questions:**
- How to transfer session context to different machine?
- How to see "what's active" across machines?
- How to avoid conflicts (same work on two machines)?

---

## Current Clean State (After Cleanup)

**Home directory (~/):**
```
~/
├── .claude/                  # Claude Code config & sessions
├── .local/share/chezmoi/     # Dotfiles source (managed)
├── bin/                      # Custom scripts (dotfiles-managed)
├── engrams/                  # Local engram work
│   └── go/error-handling.ai.md
├── retro-tasks/              # ✅ Git-backed process improvements
│   └── wayfinder/
├── src/                      # ✅ Organized clones
│   ├── personal/
│   └── work/
├── vpaste-wayfinder-autonomous/  # ✅ Other project
├── workspace-design/         # ⚠️ Name needs rethinking
│   ├── dotfiles/
│   └── workspace-management/
├── worktrees/                # ✅ Worktree location
│   └── dotfiles-test/
└── [dotfiles managed configs]
```

**/tmp/ (Ephemeral - acceptable for ephemeral work):**
```
/tmp/
├── engram-install/           # Main engram repo clone
├── engram-research/          # Research repo
├── bash-guidance-worktree/   # Completed project worktree
└── claude-XXXX-cwd           # Session tracking files (400+)
```

**Remote (Safe):**
- github.com/vbonnet/dotfiles (private)
- github.com/vbonnet/retro-tasks (private)
- github.com/vbonnet/engram
- github.com/vbonnet/engram-research

---

## Real-World Examples for D2

These are concrete examples to inform solutions search:

**Example 1: Bash Guidance Wayfinder Project**
- Started as worktree in /tmp/bash-guidance-worktree/
- Generated 21 artifact files (D1-D4, S4-S11)
- Stored temporarily in ~/bash-guidance-consolidation/
- Implementation merged into engram repo
- Artifacts had no clear permanent home
- **Solution needed:** Wayfinder project archive structure

**Example 2: Session Planning Docs**
- 4 projects with planning docs (project1-4.md, prompt1-4.md)
- Created ad-hoc in ~/
- No standard format or location
- **Solution needed:** Session work directory structure

**Example 3: Dotfiles Project**
- Created in ~/workspace-design/dotfiles/
- Complete D1-S11 artifacts
- Implementation in ~/.local/share/chezmoi/ (managed by tool)
- **Works well,** but "workspace-design" name doesn't scale

**Example 4: Worktree Cleanup**
- ~/worktrees/dotfiles-test/ created for testing
- Branch merged weeks ago
- Worktree still exists (forgotten)
- **Solution needed:** Worktree lifecycle tracking

---

## Questions for D2 to Answer

1. **Where to clone repos?**
   - ~/src/work/ and ~/src/personal/?
   - Something else?
   - How to handle multiple clones of same repo?

2. **Where to create worktrees?**
   - ~/worktrees/ flat?
   - ~/worktrees/{repo}/ nested?
   - Sibling to main clone?
   - Separate location entirely?

3. **How to track sessions?**
   - File-based (PID files, session manifests)?
   - Database?
   - Git-backed state?

4. **Where to archive completed work?**
   - Wayfinder projects: Where?
   - Session docs: Where?
   - Old worktrees: How long to keep?

5. **How to handle ephemeral work?**
   - Keep using /tmp/ (with sync mechanism)?
   - Create ~/ephemeral/ that's safe to delete?
   - Auto-cleanup old work?

6. **Integration with engram-research?**
   - Should some of this be engram-research structure?
   - Separate workspace repo?
   - Local-only?

---

## Success Criteria (From D1)

Reminder of what we're trying to achieve:

**Quantitative:**
- Worktree cleanup: < 5 min/week (down from 30 min)
- Session restart: < 2 min
- Work transfer: < 5 min (down from 30 min)
- Zero data loss from /tmp/
- 100% worktree status visibility

**Qualitative:**
- Clear answer: "What Claude sessions exist and where?"
- Easy session resumption from any machine
- Structured, navigable logs (not ad-hoc)
- Git-backed for portability
- Directory structure that scales

---

## Ready for D2

**This snapshot provides:**
- Real examples of the mess
- Concrete problems to solve
- Patterns that emerged
- Current (cleaned) state as baseline

**Next:** D2 Solutions Search - research tools and patterns for workspace/session management

---

**Snapshot Date:** 2025-12-02

**Cleanup Status:** ✅ Critical data protected

**Ready for:** D2 Solutions Search

---
