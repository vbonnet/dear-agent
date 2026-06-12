---
phase: DESIGN
phase_name: Design — host scheduler architecture
wayfinder_session_id: b56e8212-3f64-4bbe-97b2-44dea52da1e8
created_at: 2026-06-11
note: phase tracked manually — wayfinder CLI deliverable validator requires
  engram pipeline artifacts not generated on this host (bead filed)
---

# DESIGN: Reliable Host-Side Scheduled Automation

## Approaches compared

| | A. launchd → agm loop tick (chosen) | B. Per-job launchd plists | C. Temporal Go worker | D. New Go scheduler daemon | E. Fix-in-Cowork via MCP bridge |
|---|---|---|---|---|---|
| Host capabilities | Full | Full | Full | Full | Only what's exposed as tools |
| Run history / audit | loops.db (exists) | launchctl exit codes only | Temporal UI (rich) | Build from scratch | Gateway log (build) |
| New moving parts | 1 plist + wrapper | N plists | Colima+Docker+Temporal+tunnel+worker | Daemon + store + supervision | MCP tools + permissions |
| Reboot-safe | Yes | Yes | Only if whole chain auto-starts | Yes (launchd-supervised) | N/A |
| Dogfoods our stack | **Yes (AGM)** | No | No | Partially | Yes (agm-mcp-server) |
| Failure points before job runs | 1 | 1 | 4 (all currently dead) | 2 | 3 |
| Marginal code | ~small | none (but plist sprawl) | large | large | medium, security-critical |

**Decision: A.** One durable trigger (launchd), job definition/cadence/run
history in `agm loop` (already shipped, dogfoods AGM per principle 6),
plist sprawl avoided, smallest new surface. B is the fallback for jobs that
must not depend on the agm binary (none identified). C rejected: its own
live failure is an exhibit in the retro that spawned this project. D
violates "don't rebuild what exists." E deferred (design-gated P2): correct
pattern for Cowork tasks needing *one* host action, but none of the five
broken jobs needs to remain in Cowork.

## Architecture

```
launchd (com.dear-agent.loop-tick, StartInterval=300, lock-guarded)
  └─ agm loop tick                      # runs all due loops
       ├─ loop: src-health      (4h)  ──┐
       ├─ loop: dep-health      (Mon) ──┤  each loop cmd =
       ├─ loop: burndown-maint  (2h)  ──┤  agm-job run <name> -- <cmd>
       └─ loop: linkedin-vale   (Mon/Thu)┘  (lock + verify + escalate)
                 │
                 ├─ deterministic jobs: Go binaries / <50-line scripts
                 ├─ agentic jobs: headless `claude -p` (scoped allowedTools)
                 └─ effect verification: required --verify command per job
Run history: ~/.agm/loops.db (started/finished/exit/stdout/stderr per run)
Escalation: osascript notification + `agm send msg meta-orchestrator`
            (procwatch/dev-tools-update precedent)
```

### Components

1. **Tick trigger** — `com.dear-agent.loop-tick.plist`:
   `StartInterval=300` (finer cadence belongs to loop definitions, not
   launchd), explicit `PATH` (incl. `~/go/bin`, `/opt/homebrew/bin`),
   `HOME`, `WorkingDirectory=$HOME`, `Nice=10`, `LowPriorityIO`,
   `ThrottleInterval=30`, logs to `~/.agm/logs/loop-tick.log`.
   **Never** combined with `StartCalendarInterval` (double-fire incident).
2. **Installer** — `agm loop install-launchd` subcommand following
   `cmd/dear-agent-bumblebee/launchagent.go`: template the plist, bootout +
   bootstrap idempotently, testable `launchctlRun` seam. Plists are
   installed artifacts versioned with the code that defines the jobs (the
   Bumblebee/agm-bus precedent), **not** chezmoi-managed — answers charter
   Q3; chezmoi's strict review gate stays reserved for security boundaries.
3. **Run wrapper** — `agm-job` (Go, principle 9 atomic wrapper):
   - Per-job overlap lock: atomic `mkdir` + PID + comm-name check (no
     flock on macOS; ai-conversation-sync-wrapper precedent). Deliberate
     skip exits 0.
   - Runs the job command; then runs the **mandatory** `--verify` command;
     a job whose verify fails is a failed run regardless of exit 0 —
     this is the structural fix for "no-op runs are silent."
   - On failure/stale effect: macOS notification + `agm send msg
     meta-orchestrator`; never silent.
   - Log hygiene: loops.db already captures output; wrapper keeps its own
     log under `~/.agm/logs/` with size-capped self-rotation
     (dev-tools-update precedent). Nothing writes under `~/src`.
4. **Headless agentic jobs** — sanctioned invocation only:
   `claude -p "<prompt>" --allowedTools "<minimal set>" --output-format json`
   from the wrapper; per-job tool allowlists (e.g. dep-health summary needs
   only `Bash(govulncheck *) Read`); **no blanket
   `--dangerously-skip-permissions`** for jobs that touch git or `~/src`
   (the orchestrator-loop `bypassPermissions` lesson). Auth: subscription
   OAuth where the keychain is reachable; otherwise `ANTHROPIC_API_KEY`
   from a 0600 env file with a monthly cap (charter Q1 — needs owner
   answer before Phase B ships agentic jobs; deterministic jobs don't wait).
5. **Effect watchdog** — loop `loops-watchdog` (daily): queries loops.db
   for loops whose last success or last verified effect is older than
   2× cadence; escalates. The watchdog is itself a loop with a trivial
   verify, so a dead tick plist is caught by its silence → also add the
   inverse guard: a Cowork MCP-only canary task that alerts if the
   host-side heartbeat file (written by the watchdog into a mounted
   folder) goes stale — the two schedulers watch each other.

### Job migrations

| Broken Cowork task | Host loop | Implementation | Verify |
|---|---|---|---|
| bead-burndown-loop | `burndown-maint`, 2h | Deterministic Go: `agm session list` → count active burndown workers → `agm session new` with prompt template up to target N (owner decision Q2, default 1 until answered) | ≥N burndown sessions active |
| weekly-security-audit + weekly-dep-health-check | `dep-health`, Mon 03:00 | Script: govulncheck (Go repos), npm audit, brew outdated; report → `~/src/ai-conversation-logs/reports/` via worktree-safe write; optional `claude -p` summarization | Dated report exists, non-template |
| src-repo-health-audit | `src-health`, 4h | Go binary reusing `internal/safesrc` checks: branch=main, clean, ahead=0, behind≤5; escalate on violation | Report written; violations escalated |
| orchestrator-loop | **retired** | Backlog is Beads (`ce-6as`), not BACKLOG.md; dispatch = burndown-maint; no replacement file-writer | n/a |
| linkedin-cross-post | split: `linkedin-vale-gate`, Mon/Thu 09:30 + existing Cowork task | Host loop runs Vale + writes lint verdict into `~/src/ai-conversation-logs/` (a folder the Cowork task has mounted rw); Cowork task (10:00) reads verdict, refuses to post unlinted drafts, keeps MCP posting | Verdict file fresher than queue head |

Cowork shells for the four migrated tasks are disabled (not deleted) with a
pointer to the host loop name in their SKILL.md frontmatter.

### Placement rule (the recurrence guard)

Added to project CLAUDE.md + the schedule-creation skill path:

> Before creating any scheduled task, classify it: if the job needs host
> filesystem, host CLI tools, host MCP servers, or must spawn Claude Code
> sessions → it is a **host job**: define it as an `agm loop` entry. Only
> jobs that operate purely through cloud/proxied MCP tools may be Cowork
> scheduled tasks. A task whose prompt must explain what it *cannot* do is
> in the wrong scheduler.

## Security considerations

- The Cowork sandbox stays intact; we route around it for host jobs rather
  than weakening it.
- Scheduled host jobs run with user privileges → every job command is
  either a vetted binary/script in-repo or a scoped `claude -p`; no
  free-form shell assembled from file contents (injection path).
- Git network ops only via `safe-push` / `GIT_TERMINAL_PROMPT=0 gtimeout`;
  `~/src` writes only via `src-recovery`.
- Deferred MCP bridge (P2): verb-scoped tools only, fixed argv, gateway
  audit log, per-task `approvedPermissions`; explicitly never generic exec.

## Risks

| Risk | Mitigation |
|---|---|
| agm loop engine has latent bugs (unused in anger) | Phase A canary (src-health) runs 1 week before Phase B migrations |
| Headless claude auth/cost surprises (SDK credit pool 2026-06-15) | Deterministic-first design: only dep-health summary + burndown prompts are agentic; cap + Q1 gate |
| Tick plist silently dies | Dual watchdog (host loop + Cowork canary heartbeat) |
| Burndown spawner recreates session pile-ups | Spawn ≤1 per tick, count-before-spawn, lock; per_task_limit lesson |
| loops.db growth | Run-history retention pruning in tick (size-check, same convention as log self-rotation) |
