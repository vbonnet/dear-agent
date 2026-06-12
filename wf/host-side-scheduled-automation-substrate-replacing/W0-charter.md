---
phase: W0
title: Charter — Host-side scheduled automation substrate
date: 2026-06-11
status: complete
project: host-side-scheduled-automation-substrate-replacing
session_id: b56e8212-3f64-4bbe-97b2-44dea52da1e8
---

# W0 Charter: Host-Side Scheduled Automation Substrate

## Problem statement

5 of 13 Claude Desktop (Cowork) scheduled tasks are broken or disabled
because they require host capabilities (host filesystem, git/bd/vale/
govulncheck/brew, spawning Claude Code tasks, host MCP servers) while
executing inside an isolated Linux VM that withholds all of them by design.
Full evidence and root-cause analysis:
`docs/retros/2026-06-11-scheduled-task-sandbox.md`.

Cost to date: ~460+ wasted runs/month, 193+ hours of backlog data
corruption, the burndown/security/repo-health automation layer delivering
zero value while appearing scheduled and "running."

## Goal

Every recurring automation job runs in an environment that actually has the
capabilities the job needs, with run history, effect verification, and a
placement rule that prevents recurrence.

## Scope

1. **Placement rule** — a documented decision procedure: host-capability
   jobs → host scheduler; cloud/MCP-only jobs → Cowork scheduled tasks.
2. **Host scheduling substrate** — launchd-driven `agm loop tick` (reusing
   the existing `agm loop` system) + an idempotent installer + conventions
   (locks, log rotation, env) distilled from the existing LaunchAgent fleet.
3. **Headless agentic jobs** — a sanctioned wrapper for invoking
   `claude -p` from scheduled context (modeled on `dev-tools-update.sh`).
4. **Migration of the 5 broken tasks** — burndown maintenance,
   dep/security audit, src-repo-health, orchestrator (subsumed), and the
   host-side half of linkedin-cross-post; disable the Cowork no-op shells.
5. **Effect verification** — each migrated job asserts its intended effect;
   a watchdog flags stale effects.

## Non-goals

- Replacing or weakening the Cowork sandbox (it is correct; we stop abusing
  it). MCP-only Cowork tasks (inbox-triage, close-chrome-tabs, calendar)
  stay where they are.
- A generic "run any host command" MCP bridge (security non-starter;
  verb-scoped tools are a later, design-gated phase — P2 bead).
- Adopting Temporal for dear-agent scheduling (4-link dependency chain,
  currently dead on this host, Python worker; rejected in D3).
- Building a new scheduler engine (agm loop exists; we wire it, not rewrite it).
- Fixing brain-v2's crash-looping host-worker (separate repo; filed as a bead).

## Constraints

- Go only (principle 4); wrappers <50-line shell or <200-line Go (principle 9).
- `~/src/**` read-only; scheduled jobs use worktrees or the sanctioned
  `src-recovery` path; no logs written under `~/src` (two prior incidents).
- launchd plists are chezmoi-managed home-dir artifacts → deployment goes
  through the chezmoi source + review flow, or through an in-repo installer
  binary (Bumblebee precedent) — decided in DESIGN.
- Headless claude auth: no keychain prompts in launchd context;
  `GIT_TERMINAL_PROMPT=0` + safe-push for any git network ops.
- Track all work in Beads (`--tags scheduled-tasks`).

## Success criteria

1. The 5 broken tasks are either running successfully on the host substrate
   (verified effects, run history in `~/.agm/loops.db`) or explicitly
   retired with rationale.
2. Zero scheduled runs that "complete" without their effect being verified.
3. Placement rule documented in project CLAUDE.md + the task-creation path.
4. No new unbounded logs, no overlapping runs, reboot-safe.
5. A month after rollout: wasted-run count ≈ 0 (vs ~460 baseline).

## Stakeholder alignment questions

- Q1: Is metered API spend for headless `claude -p` jobs acceptable, and at
  what monthly cap? (Agent SDK credit pool lands 2026-06-15.)
- Q2: Should the burndown target remain "3 concurrent workers" or be
  rescoped given the per_task_limit pile-ups?
- Q3: launchd plist deployment: chezmoi-managed (strict review) vs in-repo
  installer binary (Bumblebee pattern)? Recommendation in DESIGN.
