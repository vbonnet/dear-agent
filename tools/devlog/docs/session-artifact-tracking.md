# Session Artifact Tracking Guide

Instructions for AI assistants to automatically preserve valuable session artifacts instead of leaving them in /tmp.

## Quick Reference

When creating artifacts during sessions, save to persistent locations:

| Artifact Type | Save Location | Pattern Match |
|--------------|---------------|---------------|
| Learnings/Patterns | `{{DEVLOG_ROOT}}/ws/oss/retrospectives/sessions/YYYY-MM-DD-{topic}.md` | `*-learnings*.md`, `*-patterns*.md`, `RETROSPECTIVE*.md` |
| Coverage Reports | `{{DEVLOG_ROOT}}/repos/{project}/.coverage/YYYY-MM-DD-coverage.{html,json}` | `*coverage*.{html,json,xml}` |
| Baselines/Metrics | `{{DEVLOG_ROOT}}/ws/oss/projects/{project}/baselines/YYYY-MM-DD-{metric}.txt` | `*baseline*.txt`, `*wordcount*.txt`, `*metrics*.{csv,json}` |
| Reusable Scripts | `~/bin/{name}` or `{{DEVLOG_ROOT}}/repos/{project}/tools/{name}` | `*.{sh,py}` (>20 lines or with docs) |
| Debug Scripts | `{{DEVLOG_ROOT}}/ws/sessions/.debug-scripts/{name}` | `debug-*.sh`, `*-debug*.{py,sh}` |
| Project Closures | `{{DEVLOG_ROOT}}/ws/oss/projects/{project}/closure-YYYY-MM-DD.md` | `close-*.md`, `*-complete*.md`, `*-followups*.md` |
| Task Snapshots | `{{DEVLOG_ROOT}}/ws/oss/.beads/snapshots/YYYY-MM-DD-{desc}.jsonl` | `*-beads*.jsonl`, `*-tasks*.jsonl` |

---

## Detailed Guidelines

### 1. Retrospectives & Learnings

**When:** Documenting patterns, techniques, multi-agent learnings, optimization findings

**Location:** `{{DEVLOG_ROOT}}/ws/oss/retrospectives/sessions/YYYY-MM-DD-{topic}.md`

**Examples:**
- `/tmp/eng-swarm-update.txt` → `{{DEVLOG_ROOT}}/ws/oss/retrospectives/sessions/2025-12-08-swarm-patterns.md`
- Session learning notes → `{{DEVLOG_ROOT}}/ws/oss/retrospectives/sessions/2025-12-08-git-cleanup-learnings.md`

**Why:** Cross-session pattern recognition, compound learning over time

---

### 2. Metrics & Baselines

**When:** Creating coverage reports, wordcounts, performance metrics, quality baselines

**Locations:**
- Coverage: `{{DEVLOG_ROOT}}/repos/{project}/.coverage/YYYY-MM-DD-coverage.{html,json}`
- Baselines: `{{DEVLOG_ROOT}}/ws/oss/projects/{project}/baselines/YYYY-MM-DD-{metric}.txt`

**Examples:**
- `/tmp/coverage.html` → `{{DEVLOG_ROOT}}/repos/engram/base/.coverage/2025-12-08-coverage.html`
- `/tmp/baseline_wordcount.txt` → `{{DEVLOG_ROOT}}/ws/oss/projects/engram-optimization/baselines/2025-12-08-wordcount.txt`

**Why:** Track quality trends, detect regressions, verify improvements

---

### 3. Tools & Scripts

**When:** Creating utility scripts during session

**Decision Tree:**
- Script > 20 lines OR has usage docs → Permanent location
  - General utility → `~/bin/{name}`
  - Project-specific → `{{DEVLOG_ROOT}}/repos/{project}/tools/{name}`
- Script < 20 lines AND one-off debug → `{{DEVLOG_ROOT}}/ws/sessions/.debug-scripts/YYYY-MM-DD-{name}`

**Examples:**
- `/tmp/audit-engrams.py` → `{{DEVLOG_ROOT}}/repos/engram/base/tools/audit-engrams.py` (reusable)
- `/tmp/check-sessions.sh` → `{{DEVLOG_ROOT}}/ws/sessions/check-sessions.sh` (session utility)
- `/tmp/debug-hook.sh` → `{{DEVLOG_ROOT}}/ws/sessions/.debug-scripts/2025-12-08-debug-hook.sh` (one-off)

**Why:** Tool discovery, avoid re-inventing, debug reproducibility

---

### 4. Project Closures

**When:** Completing wayfinder projects, creating closure docs, generating followup beads

**Location:** `{{DEVLOG_ROOT}}/ws/oss/projects/{project}/closure-YYYY-MM-DD.md`

**Examples:**
- `/tmp/close-wf-003-and-create-followups.md` → `{{DEVLOG_ROOT}}/ws/oss/projects/wf-003-pre-implementation-validation/closure-2025-12-08.md`

**Why:** Project archival, follow-up tracking, decision records

---

### 5. Task Snapshots

**When:** Exporting beads, creating task snapshots

**Location:** `{{DEVLOG_ROOT}}/ws/oss/.beads/snapshots/YYYY-MM-DD-{desc}.jsonl`

**Examples:**
- `/tmp/beads-only-open.jsonl` → `{{DEVLOG_ROOT}}/ws/oss/.beads/snapshots/2025-12-08-open-tasks.jsonl`
- `/tmp/core-6ed-closed.jsonl` → `{{DEVLOG_ROOT}}/ws/oss/.beads/snapshots/2025-12-08-core-6ed-closed.jsonl`

**Why:** Historical workload view, burndown tracking, priority evolution

---

## Session End Protocol

Before ending a session, AI assistants should:

1. **Scan /tmp** for valuable artifacts matching patterns above
2. **Categorize** found artifacts by type
3. **Prompt user:** "Found X artifacts in /tmp. Archive to persistent storage?"
4. **Show list** with suggested destinations
5. **Execute moves** with user confirmation

---

## Implementation Status

- ✅ **Phase 1:** Manual guidelines (this document)
- 🚧 **Phase 2:** `~/bin/engram-session-archive-hook.sh` (planned)
- 📋 **Phase 3:** Devlog plugin with search (future)

---

## Related Documentation

- Devlog recommendations: `{{DEVLOG_ROOT}}/ws/sessions/devlog-recommendations-2025-12-08.md`
- Devlog design project: `{{DEVLOG_ROOT}}/ws/oss/projects/devlog-design-v3/`
- Wayfinder learnings: `{{DEVLOG_ROOT}}/repos/engram/base/plugins/wayfinder/docs/case-studies/wayfinder-improvements/LEARNINGS.md`
