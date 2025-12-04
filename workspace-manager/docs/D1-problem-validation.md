# D1: Problem Validation - Workspace & Session Management

**Date:** 2025-12-01

**Project:** Directory Structure + Workspace Management + Session Tracking

**Phase:** D1 - Problem Validation

---

## Problem Statement

**What problem are we solving?**
Currently lacking a structured approach for:
1. **Directory organization** - Where to clone repos, where to create worktrees, how to organize work/personal
2. **Worktree lifecycle management** - No visibility into which worktrees exist, which are abandoned/merged, cleanup needed
3. **Session tracking** - No way to know which Claude sessions are working in which worktrees
4. **Work resumability** - Difficult to transfer context between machines or resume work after interruption
5. **Persistent logging** - Currently logs to /tmp/ (ephemeral), lost on reboot
6. **Completed project artifacts** - No clear home for finished Wayfinder projects

**Who experiences this problem?**
- User (vbonnet) running multiple concurrent Claude Code sessions
- Working across personal (github.com/vbonnet/*) and work (github.com/[REDACTED_EMPLOYER]-src/*) repos
- Frequent context switching between projects
- Need to transfer work between machines (local → GCP Workstation)

**Current pain points (validated during cleanup - see current-state-snapshot.md):**

1. **Critical work in ephemeral /tmp/ (❗ CRITICAL)**
   - Found: engram-install, engram-research, bash-guidance-worktree all in /tmp/
   - Risk: Lost on reboot!
   - Why: "No clear right place for active work" → defaults to /tmp/ to avoid polluting ~/

2. **Scattered files with no structure**
   - Found: 11 loose .md files in ~/ (session planning, prompts, research)
   - Found: ~/bash-guidance-consolidation/ orphaned after project completion
   - Found: ~/workspace-design/ name doesn't scale beyond first project
   - Found: ~/engrams/ experimental work with unclear home

3. **No worktree lifecycle management**
   - Found: ~/worktrees/dotfiles-test/ still existed weeks after branch merged
   - Found: Multiple worktrees in /tmp/ with no status tracking
   - No visibility into which worktrees exist per repo
   - No way to know which branches are merged/abandoned

4. **Session tracking is breadcrumbs only**
   - Found: 400+ `claude-XXXX-cwd` files in /tmp/ (just session IDs)
   - No context about what each session is working on
   - No way to resume sessions
   - No visibility into active vs abandoned sessions

5. **Completed Wayfinder projects homeless**
   - bash-guidance: Complete D1-S11, no clear archive location
   - dotfiles: Complete D1-S11, stuck in ~/workspace-design/
   - Need: Structured archive with searchability

6. **Repository clones scattered**
   - Pattern observed: /tmp/, ~/, ~/worktrees/, ~/.local/share/chezmoi/
   - No consistent strategy for where to clone
   - No clear work/ vs personal/ separation

**Evidence sources:**
- **current-state-snapshot.md** - Detailed documentation of messy state before cleanup (400+ lines)
- **untracked-work-audit.md** - Git audit documenting what was found and archived
- **Cleanup session (2025-12-02)** - Archived 3 completed Wayfinder projects, 11 scattered files, protected critical /tmp/ work

**Research reference:**
User has documented LangChain research about AI agents logging to filesystems (in /tmp/engram-research/LANGCHAIN-INSIGHTS-INTEGRATION-PLAN-2025-11-23.md) that addresses:
- File-based sub-agent instruction passing
- Learned patterns feedback loop
- Scratch pad for large tool results
- Context engineering audit framework

---

## Observed Patterns (From Cleanup)

**Pattern 1: "Don't want to pollute home" → /tmp/**
- **Symptom:** Cloning to /tmp/ to avoid deciding on structure
- **Root cause:** No clear "right place" for active work
- **Risk:** Work lost on reboot
- **Example:** engram-install, engram-research both in /tmp/

**Pattern 2: Ad-hoc Session Planning Docs**
- **Symptom:** project*.md, prompt*.md files scattered in ~/
- **Root cause:** No standard location for session artifacts
- **Result:** Clutter, lost context
- **Example:** 11 loose .md files found during cleanup

**Pattern 3: Completed Project Artifacts Homeless**
- **Symptom:** bash-guidance-consolidation/ sitting orphaned after completion
- **Root cause:** No "Wayfinder projects archive" location
- **Bandaid:** Dumping to engram-research
- **Example:** 3 complete Wayfinder projects with nowhere to live

**Pattern 4: Duplicate Docs Leak Out**
- **Symptom:** README.md, docs/ in ~/ (from chezmoi)
- **Root cause:** Tools (chezmoi) applying files, no clear boundaries
- **Example:** Dotfiles README duplicated to ~/

**Pattern 5: Local Work vs Repository Work Unclear**
- **Symptom:** ~/engrams/go/error-handling.ai.md - push or not?
- **Root cause:** No clear policy on local-only vs shared work
- **Example:** Experimental engram work with unclear destination

---

## Is This Worth Solving?

**Time Investment Estimate:**
- Design: 4-6 hours (this is complex, multiple subsystems)
- Initial implementation: 8-12 hours
- Ongoing maintenance: ~15 min/week

**Time Saved:**
- Worktree cleanup: Currently ~30 min/week manual searching
- Session context loss: ~1 hour/week recreating context
- Work transfer between machines: ~30 min each time (happens weekly)
- Finding abandoned work: ~20 min/week
- Total: ~2.5 hours/week saved

**ROI Calculation:**
- Setup investment: ~15 hours
- Weekly savings: ~2.5 hours
- Pays off in: 6 weeks
- Annual savings: ~130 hours
- **Verdict: HIGH VALUE - definitely worth solving**

**Additional strategic value:**
- This workspace management system would be a prime example of dogfooding Engram/Wayfinder
- The session tracking / logging patterns could feed back into Engram's design
- LangChain patterns can be validated in real-world usage

---

## Complexity Signals

**Detected complexity level:** COMPREHENSIVE

**Signals:**
1. Multiple interrelated subsystems (directory org, worktree lifecycle, session tracking, logging)
2. Integration with existing tools (git worktrees, Claude Code, engram-research)
3. Need for automated monitoring and cleanup
4. Cross-machine synchronization requirements
5. References to academic research (LangChain patterns)
6. User explicitly mentioned this relates to formalized research they were planning

**This warrants full D1-D4 discovery + S4-S11 SDLC**

---

## Success Criteria

**Quantitative:**
- Worktree cleanup time: < 5 min/week (down from 30 min)
- Session restart time: < 2 min (down from rebuilding context)
- Work transfer time: < 5 min (down from 30 min)
- Zero data loss from /tmp/ ephemerality
- 100% of worktrees have known status (active/merged/abandoned)

**Qualitative:**
- Clear answer to "what Claude sessions are running and where?"
- Easy to resume any session from any machine
- Structured, navigable project/session logs (not ad-hoc in /tmp/)
- Git-backed logging for portability and history
- Directory structure that scales (doesn't overwhelm with growth)
- Integration with engram-research patterns

---

## Key Questions for D2-D4

**Q1: Where to clone repos?**
- Current chaos: /tmp/, ~/, no pattern
- Options: ~/src/work/ and ~/src/personal/? ~/repos/? Something else?
- How to handle multiple clones of same repo?

**Q2: Where to create worktrees?**
- Current chaos: /tmp/, ~/worktrees/ (flat)
- Options: Flat ~/worktrees/? Nested ~/worktrees/{repo}/? Sibling to main clone?
- How to track which worktrees exist per repo?

**Q3: How to track sessions?**
- Current: 400+ claude-XXXX-cwd files with no context
- Options: File-based manifests? Database? Git-backed state?
- What metadata to capture? How to link session ↔ worktree?

**Q4: Where to archive completed work?**
- Current: Dumping to engram-research (bandaid)
- Wayfinder projects: Where? How organized?
- Session docs: Where? Active vs archived?
- Old worktrees: How long to keep?

**Q5: How to handle ephemeral work?**
- Current: Using /tmp/ (risky!)
- Options: Keep using /tmp/ with sync mechanism? ~/ephemeral/ that's safe to delete? Auto-cleanup?
- How to distinguish: permanent vs ephemeral?

**Q6: Integration with engram-research?**
- Should some of this be engram-research structure?
- Separate workspace repo?
- Local-only?
- How to make it portable across machines?

---

## Next Steps

1. **D2: Solutions Search** - Research existing tools/patterns:
   - Workspace management tools
   - Session management approaches
   - Git worktree lifecycle tools
   - Structured logging systems for AI agents
   - LangChain filesystem patterns (already documented)

2. **D3: Approach Decision** - Choose architecture:
   - Directory organization pattern
   - Session tracking mechanism
   - Logging storage and structure
   - Integration vs standalone tooling

3. **D4: Solution Requirements** - Define detailed specs for chosen approach

---

## Decision

**Status:** ✅ PROCEED TO D2

**Confidence:** VERY HIGH

**Reasoning:**
- Clear, well-articulated problem with concrete pain points
- Strong ROI (6-week payback, 130h/year savings)
- Strategic value for Engram dogfooding
- User has already done significant related research (LangChain patterns)
- Complexity warrants full discovery process
