---
phase: PROBLEM
phase_name: Problem Validation
wayfinder_session_id: b56e8212-3f64-4bbe-97b2-44dea52da1e8
created_at: 2026-06-11
phase_engram_hash: ""
phase_engram_path: ""
---

# D1: Problem Validation

## Is the problem real?

Yes — validated by direct evidence, not report. Full analysis in
`docs/retros/2026-06-11-scheduled-task-sandbox.md`. Summary of evidence:

1. **Primary source:** `scheduled-tasks.json` (Claude Desktop
   local-agent-mode-sessions) shows 5 of 13 tasks broken/disabled:
   `orchestrator-loop` (disabled), `weekly-security-audit` (disabled),
   `bead-burndown-loop` (enabled but a self-described no-op, 30 recorded
   `per_task_limit` skips), `weekly-dep-health-check` (calls a tool that
   doesn't exist in its environment), `src-repo-health-audit` (outputs
   unverifiable templates 6×/day).
2. **The SKILL.md files confess the failure in their own prompts** — e.g.
   bead-burndown-loop: "scheduled tasks run in a Cowork sandbox and CANNOT
   start code tasks or access the host filesystem."
3. **Sandbox mechanics confirmed** from host logs: gvisor user-mode
   networking, virtiofs subpath mounts only for `userSelectedFolders`,
   host MCP servers proxied via the Desktop app's session bridge (which is
   why MCP-backed tasks are the only working ones).
4. **Quantified impact:** ~460+ wasted runs/month; 193+ hours of backlog
   corruption; ~0 automated bead closes against a 110-item epic target.

## Stakeholders

- Valentin (owner): loses the autonomy layer he built these tasks for.
- Every agent session: inherits corrupted backlog state, unverified repo
  health, and a dark security-audit layer.
- The dogfooding flywheel (principle 6): scheduled automation routed
  around AGM/VROOM entirely, so our own tooling got zero data points.

## Existing-solution search (in-repo)

The codebase already contains most of the solution — which sharpens the
problem from "build a scheduler" to "wire what exists and add a placement
rule":

- `agm loop` (`agm/cmd/agm/loop.go`, `agm/internal/ops/loop.go`): named
  recurring commands, cadence, SQLite run history, `agm loop tick`
  designed for cron/launchd. Unused for any of the broken jobs.
- Bumblebee launchd installer (`cmd/dear-agent-bumblebee/launchagent.go`):
  idempotent plist install/reload pattern with a testable launchctl seam.
- `dev-tools-update.sh` (chezmoi-managed): working launchd → headless
  `claude -p` invocation with structured prompt + notification fallback.
- `cmd/src-recovery`, `cmd/safe-push`: sanctioned wrappers for the exact
  git operations scheduled jobs will need.

## Decision

**Real problem.** Not a misunderstanding, not a transient bug: an
architectural mismatch that was warned about, documented, and propagated
three times. Proceed to RESEARCH.
