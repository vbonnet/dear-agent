---
phase: PLAN
phase_name: Plan — phased rollout
wayfinder_session_id: b56e8212-3f64-4bbe-97b2-44dea52da1e8
created_at: 2026-06-11
note: phase tracked manually — wayfinder CLI deliverable validator requires
  engram pipeline artifacts not generated on this host (bead filed)
---

# PLAN: Phased Rollout

Each phase is a separate scoped PR by a dedicated worker (principle 1).
Every task's Definition of Done includes: changes committed to the working
branch; branch pushed; tests + lint green on the committed tree.

## Phase A — Substrate + canary (P0, ~1 day)

1. `agm loop install-launchd` subcommand (Bumblebee pattern, launchctl
   seam, unit tests). Plist per DESIGN env/throttle/log conventions.
2. `agm-job` wrapper: lock, mandatory verify, dual escalation,
   self-rotating log. ≤200 lines + tests.
3. Canary loop `src-health`: Go check over the 7 `~/src` repos
   (branch/clean/ahead/behind via `git -C`), report + escalation.
4. Soak: canary runs ≥1 week (≥42 ticks) with AC1/AC2/AC3 verified before
   Phase B. Gate: loops.db shows verified runs, zero overlaps, logs bounded.

Exit criteria: AC1, AC2, AC3, AC8.

## Phase B — High-value migrations (P0/P1, ~1–2 days after soak)

5. `burndown-maint` loop: count-before-spawn worker maintenance via
   `agm session new`; spawn ≤1/tick; target N=1 until charter Q2 answered.
   Disable the Cowork bead-burndown-loop shell (frontmatter pointer).
6. `dep-health` loop: govulncheck/npm audit/brew outdated script; dated
   report; agentic summarization only after charter Q1 (cost cap) is
   answered — ship deterministic first. Disable weekly-dep-health-check
   and delete the long-dead weekly-security-audit shell.
7. Retire orchestrator-loop permanently: SKILL.md replaced by a tombstone
   pointing at Beads (`ce-6as`) + burndown-maint; note the 193h corruption
   incident.

Exit criteria: AC4, AC5; Cowork shells disabled; beads updated.

## Phase C — Quality gates + watchdog (P1, ~1 day)

8. `linkedin-vale-gate` loop + Cowork-task edit to consume the verdict
   file (the only SKILL.md edit that adds capability rather than removing
   pretense).
9. `loops-watchdog` loop + heartbeat file + Cowork canary task (the
   MCP-only kind that actually works there — eat our own placement rule).
10. Disable src-repo-health-audit Cowork shell (replaced in Phase A by the
    canary, which by now has weeks of history).

Exit criteria: AC6, AC7.

## Phase D — Doctrine + deferred bridge (P2)

11. Placement rule into project CLAUDE.md + schedule-creation skill;
    30-day waste re-audit (AC9).
12. Design gate (not implementation) for verb-scoped host MCP tools on
    agm-mcp-server — only if a concrete Cowork task needs exactly one host
    action; security review mandatory before any build.

## Dependencies & risks

- Phase B5 depends on `agm session new` reliability (known good) and Q2.
- Phase B6 agentic half blocked on Q1; deterministic half is not.
- Soak gate between A and B is non-negotiable — it is the empirical answer
  to "agm loop was never used in anger."

## Stakeholder decision points (carried from charter)

- Q1: monthly cap for headless `claude -p` spend (blocks B6 summary only).
- Q2: burndown worker target N (default 1; 3 was the old aspiration).
- Q3: **answered in DESIGN** — in-repo installer, not chezmoi.
