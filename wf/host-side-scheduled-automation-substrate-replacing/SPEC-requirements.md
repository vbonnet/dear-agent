---
phase: SPEC
phase_name: Spec — functional and non-functional requirements
wayfinder_session_id: b56e8212-3f64-4bbe-97b2-44dea52da1e8
created_at: 2026-06-11
note: phase tracked manually — wayfinder CLI deliverable validator requires
  engram pipeline artifacts not generated on this host (bead filed)
---

# SPEC: Host Scheduler Requirements

## Functional requirements

- **FR1** `agm loop install-launchd` installs/refreshes the tick plist
  idempotently (bootout + bootstrap); `--uninstall` removes it. Running it
  twice is safe and converges.
- **FR2** `agm loop tick` (triggered every 300s) runs all due loops; a loop
  already running is skipped (exit 0, recorded as skip, never queued).
- **FR3** `agm-job run <name> --verify <cmd> -- <cmd>` executes the job
  command, then the verify command; the run is recorded successful **only
  if both** exit 0. Verify is mandatory — `agm-job` refuses to run without
  one.
- **FR4** On job failure or verify failure: macOS user notification AND
  `agm send msg meta-orchestrator` with job name, exit code, log tail.
- **FR5** The five migrations behave per the DESIGN table (burndown-maint,
  dep-health, src-health, linkedin-vale-gate; orchestrator-loop retired).
- **FR6** Migrated Cowork tasks are disabled with frontmatter pointing to
  their replacement loop name.
- **FR7** `loops-watchdog` (daily) escalates any loop whose last verified
  success is older than 2× its cadence, and writes a heartbeat file into a
  Cowork-mounted folder; a Cowork canary task alerts if that heartbeat is
  >48h stale.
- **FR8** Placement rule text lands in project CLAUDE.md and in the
  schedule-creation path.

## Non-functional requirements

- **NFR1 No overlap:** at most one instance of any loop runs at a time
  (atomic mkdir lock + PID + comm-name check).
- **NFR2 Bounded logs:** all logs under `~/.agm/logs/` with size-capped
  rotation; loops.db run-history pruned by retention policy. Nothing is
  ever written under `~/src/**` except via `src-recovery`.
- **NFR3 Reboot-safe:** jobs resume after reboot with no manual step
  (launchd-native).
- **NFR4 No interactive prompts:** no keychain/GUI prompt can block a run
  (`GIT_TERMINAL_PROMPT=0`, safe-push, pre-resolved auth for `claude -p`).
- **NFR5 Auditable:** every run (including skips) queryable from loops.db:
  start, end, exit, verify result, output.
- **NFR6 Least privilege:** agentic jobs declare per-job `--allowedTools`;
  no `--dangerously-skip-permissions` for jobs touching git or `~/src`.
- **NFR7 Go + wrappers:** all new code Go (or <50-line shell), wrapper
  ≤200 lines (principles 4 and 9); `make preflight` green.
- **NFR8 Cost cap:** agentic jobs subject to a monthly spend cap (value =
  charter Q1); deterministic jobs have zero model cost.

## Acceptance criteria (testable)

- **AC1** Reboot test: after `sudo reboot`, src-health runs at its next
  cadence with no manual intervention; run visible in loops.db.
- **AC2** Overlap test: a job sleeping past the tick interval yields
  exactly one running instance and a recorded skip.
- **AC3** Verify test: a job that exits 0 but whose verify exits 1 is
  recorded failed and produces both escalation channels (FR4).
- **AC4** Burndown test: with 0 active workers and target N=1, one tick
  spawns exactly one worker; the next tick spawns none (count-before-spawn).
- **AC5** Dep-health test: Monday run produces a dated, non-template report
  containing real govulncheck output for dear-agent.
- **AC6** Vale-gate test: a queued draft failing Vale blocks the Cowork
  posting task (verdict file says fail → task refuses, notifies).
- **AC7** Watchdog test: pausing src-health for >8h triggers escalation;
  unloading the tick plist for >48h triggers the Cowork canary.
- **AC8** Unit + integration tests pass in CI (`make preflight-tests`);
  installer tested via the launchctl seam without touching real launchd.
- **AC9** 30 days post-rollout: scheduled-task waste audit shows ≈0 no-op
  runs (baseline: ~460/month).
