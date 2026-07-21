# dear-agent

> A local agent harness substrate for long-running software engineering work.

# Goal

## Product Direction

dear-agent is a local meta-harness for AI-assisted software engineering. It
keeps coding agents, workflow runners, memory, and verification attached to
queryable records instead of ephemeral chat context.

The current product surface is the code that exists today:

- **AGM (`agm`)** — session lifecycle, tmux-backed harness management, loops,
  message delivery, sandbox setup, health checks, and PR/process guardrails.
- **Engram (`engram`)** — persistent memory and retrieval for agent work.
- **Wayfinder (`wayfinder session` and the companion agent skill)** — structured SDLC
  phases, validation gates, task tracking, review, and retrospectives.
- **Workflow and LLM packages (`pkg/workflow`, `pkg/llm`)** — reusable DAG,
  audit, budget, human-in-the-loop, provider, routing, and authentication
  primitives used by higher-level tools.

This is not a promise of a separate top-level CLI. The living CLI direction is
to make the existing `agm`, `engram`, `wayfinder session`, and repository
scripts coherent, scriptable, and cross-harness compatible.

## Principles

### Local-first, inspectable substrate

The default architecture is deliberately plain: tmux sessions, files, SQLite
or Dolt-backed state, Git worktrees, shell scripts where they are sufficient,
and Go for maintained product code. When something breaks, a developer should
be able to inspect state from a terminal without a hosted dashboard.

### Records before conversation

Agents need durable state outside the context window: stable IDs, explicit
ownership, lifecycle state, structural verbs, and queryable history. Session
manifests, Beads, PRs, Git history, Wayfinder artifacts, loop runs, and memory
records are the substrate. Chat is an interface to that substrate, not the
source of truth.

### Harness parity without harness denial

AGM supports Claude Code, Codex CLI, AGY, OpenCode, and Pi as active harnesses;
Gemini CLI remains deprecated compatibility. Harness-specific behavior exists
and must be made explicit where it leaks. The goal is parity through shared
contracts, adapters, tests, and honest fallback paths, not documentation that
pretends every command already shares one perfect abstraction.

### Staff+ engineering quality bar

Code and documentation must state the system that exists. That means clear
abstractions, focused tests, structured errors, privacy-preserving metadata,
and architecture decisions that are updated when the implementation changes.
Broken or obsolete policy text is treated as product debt.

### Security and privacy first

Agents operate with sandboxing, permission profiles, guardrails, and routing
policy because agent execution is high-trust automation. Secrets and PII do
not belong in session metadata or retrospectives. Workspaces and production
state must be protected by infrastructure-level checks where possible.

## What We're Building

- **Session lifecycle management** — create, resume, archive, monitor, and
  recover AI agent sessions with persistent metadata and state tracking.
- **Loops** — recurring local commands with stored run history for lightweight
  automation and monitoring.
- **Sandbox isolation** — copy-on-write and worktree-based environments so
  agents can work without trampling each other or the host workspace.
- **Multi-agent coordination** — async messages, state-aware delivery,
  advisory file reservations, task ownership, and PR guardrails.
- **Persistent memory** — cue-based retrieval and research diary integration
  so useful context survives individual sessions.
- **Structured workflows** — YAML-driven DAG execution plus Wayfinder phase
  gates, reviews, build loops, retrospectives, and audit sinks.
- **LLM/provider plumbing** — authentication, provider interfaces, routing,
  delegation, and budget primitives for tools that need direct model calls.
- **Cross-harness policy** — AGENTS.md as the shared instruction surface, with
  harness-specific shims only where a harness requires them.

## What We're Not Building

- **A replacement for AI CLIs** — the harnesses still do the coding work. AGM
  starts, tracks, resumes, and coordinates them.
- **A hosted cloud platform** — dear-agent is local-first. There is no hosted
  account, billing, or remote control plane in this repo.
- **A prompt library product** — prompts may exist where a workflow needs them,
  but the product goal is durable execution, not prompt marketplace curation.
- **A pretend single abstraction** — shared packages and operations are useful,
  but some CLI lifecycle paths still contain harness-specific tmux and command
  logic. Documentation should expose that reality until the code changes.

## Harness Parity Roadmap

dear-agent should make supported harnesses viable for the same core workflows:
session startup, association, message delivery, mode/model control where the
harness supports it, Wayfinder phases, code review, and autonomous worker
loops.

Current direction:

1. Keep `AGENTS.md` as the canonical cross-harness instruction surface.
2. Keep per-harness shims thin and tested for import drift.
3. Record harness capability gaps explicitly instead of routing everything
   through Claude-only features.
4. Prefer shared Go contracts and tests for behavior that must be portable.
5. Preserve non-Claude fallback paths for new supervisor, bus, event, and
   workflow features.

## Product Flywheel

The repository improves itself through the same substrate it provides:

- **Bead analysis** — compare task outcomes, prompts, elapsed time, and failure
  modes across worker sessions.
- **Session cost tracking** — record tokens, API calls, wall time, commits, and
  cost-per-useful-output signals.
- **Automated verification** — check acceptance criteria, commits, tests, and
  review findings before work is considered done.
- **DEAR retrospectives** — capture Define/Execute/Audit/Retro lessons in
  engram-research and turn repeated failures into scoped follow-up work.
- **Benchmark-driven improvement** — use external benchmarks and internal
  workflow metrics to find bottlenecks, fix them, and measure again.

## Maintenance Capacity

Reserve capacity for the substrate itself:

- dependency and toolchain updates;
- stale branch, stale doc, and stale policy cleanup;
- test flake investigation;
- instruction/import drift checks;
- guardrail and permission wrapper hardening.

The goal is a small system that stays accurate under use, not an expanding set
of aspirational documents.
