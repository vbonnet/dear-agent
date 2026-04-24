# dear-agent

**A personal experiment in AI agent harness design, built around the DEAR protocol.**

DEAR (Define, Enforce, Audit, Resolve) is an architectural pattern for building AI agent systems that get permanently better after every failure. dear-agent is a working implementation of that pattern: a session manager, orchestration layer, and safety harness for multi-agent workflows across Claude, Gemini, Codex, and OpenCode.

This project grew out of a concrete problem — research sessions were silently lost when cleanup processes ran before work was committed. The DEAR loop is how the system learned to stop losing work. Some parts are solid and battle-tested. Others are still in progress. The README is honest about which is which.

> *"Every bypass is a bug. Fix the layer, not the symptom."*

---

## The Core Idea

Most agent harnesses are static. They constrain what agents can do, but they don't learn from what goes wrong.

The DEAR loop changes that:

```
   ┌─────────────────────────────────────────────┐
   │                                             │
   ▼                                             │
Define ──► Enforce ──► [ Agent Work ] ──► Audit ─┤
                                                 │
                                         Resolve & Refine
                                                 │
                                    (updates Define + Enforce)
```

| Phase | Type | What it does |
|-------|------|--------------|
| **Define** | Instructions / Spec | Sets the "what" and "why". Agents discover the best "how". |
| **Enforce** | Synchronous, blocking | Checks known failure modes before execution. Hard constraints. |
| **Audit** | Asynchronous | Catches anomalies that bypassed Enforce. Post-execution verification. |
| **Resolve & Refine** | Self-healing | Fixes the immediate issue AND updates Define/Enforce to prevent recurrence. |

**The loop is closed.** Phase R feeds back into phases D and E, making the system permanently better after every failure. This is not a checklist — it's a feedback architecture.

---

## Features

- **Multi-harness support** — Claude Code, Gemini CLI, Codex CLI, and OpenCode behind a unified adapter interface. Switch agents without changing your workflow.
- **Session lifecycle management** — Create, resume, list, archive, and monitor sessions with persistent metadata and automatic state tracking.
- **Circuit breaker** — DEARLevel classification (GREEN / YELLOW / RED / EMERGENCY) gates agent spawning based on system load. Prevents cascade failures before they start.
- **Async message delivery** — State-aware message routing. Messages are held until the target session is READY, then delivered with retry logic.
- **Immutable audit trail** — `dear-diary` reads append-only JSONL session logs. The log writer runs as an external session hook (not included in this repo).
- **Worktree isolation** — Agents operate in isolated filesystem environments. Work is recoverable even if a session dies mid-task.
- **Overseer monitor** — Architecture and ADRs defined; daemon implementation in progress.
- **Cross-harness verification** — `internal/ops/cross_check.go` implements review logic; merge-path wiring is in progress.

---

## Quick Start

### Prerequisites

- Go 1.25+
- tmux
- Git

### Install

```bash
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest
```

### First Session

```bash
# Start a new agent session
agm session new my-feature

# Check status across all sessions
agm status

# Send a message to a running session
agm send msg my-feature "run the tests and report back"

# Archive when done
agm session archive my-feature
```

### Multi-Agent Workflow

```bash
# Launch an orchestrator session (Claude Code)
agm session new orchestrator --harness claude-code

# Launch worker sessions (different harnesses for cross-verification)
agm session new worker-1 --harness claude-code
agm session new worker-2 --harness gemini-cli

# The orchestrator dispatches work; workers execute and report back
```

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│                       AGM CLI                             │
│         session · admin · workflow · send                 │
├──────────────────────────────────────────────────────────┤
│                  Shared Operations Layer                  │
│   (CLI, MCP server, and Skills all route through here)   │
├────────────┬─────────────┬─────────────┬─────────────────┤
│   Claude   │    Gemini   │    Codex    │   OpenCode      │
│   Adapter  │    Adapter  │    Adapter  │   Adapter       │
├────────────┴─────────────┴─────────────┴─────────────────┤
│                   Backend Abstraction                     │
│              Tmux (current) · Temporal (planned)          │
├──────────────────────────────────────────────────────────┤
│               Storage & Coordination                      │
│     Session DB · Message Queue · Sandbox · dear-diary    │
└──────────────────────────────────────────────────────────┘
```

All three API surfaces — CLI, MCP server, and Claude Code Skills — share a single operations layer (`internal/ops/`). Consistent behavior regardless of how you interact with the system.

### The Adapter Pattern

Each AI harness has different command semantics, session models, and state signals. The adapter pattern absorbs these differences:

```
Generic Command ──► Command Translator ──► Agent Adapter ──► Agent-Specific Action
   "resume"                  │                   │
                     Analyze session      Claude:  /resume {uuid}
                     metadata             Gemini:  API call with history
                                          Codex:   Thread continuation
```

Adding support for a new agent requires implementing one interface. Everything else — monitoring, message routing, verification — works automatically.

### The Circuit Breaker

Before spawning any worker, the circuit breaker evaluates system state:

```
DEARLevel: GREEN  → spawn allowed (load < 40%)
DEARLevel: YELLOW → spawn allowed with warning (load 40–60%)
DEARLevel: RED    → spawn blocked (load 60–100%)
DEARLevel: EMERGENCY → all spawning halted
```

Configuration via environment: `AGM_MAX_WORKERS`, `AGM_MAX_LOAD5`, `AGM_MIN_SPAWN_INTERVAL`.

### The Deferred Enforcement Principle

Not every check belongs in the Enforce phase. If the cost of synchronous checking exceeds the cost of async remediation, Enforce is deliberately skipped:

```
if f × C_check >> C_fix:
    skip Enforce → rely on Audit + Resolve
```

Where `f` = check frequency, `C_check` = synchronous cost, `C_fix` = remediation cost.

Over-enforcement is as bad as under-enforcement. The formula makes the trade-off explicit.

---

## DEAR in Practice: A Real Incident

During development, 17 research sessions were launched. 10 were stranded on unmerged branches. 7 were completely lost when the cleanup process ran before their work was committed.

**Define:** Specified what "session complete" means — 5 machine-checkable predicates:
1. Work committed to a named branch
2. Branch pushed to remote
3. Branch name is human-readable (not a UUID)
4. Requester notified
5. Work is merge-ready

**Enforce:** UUID branch names are rejected at creation time. Cleanup process now auto-commits and pushes before running. (Pre-exit hook for completion predicates is in progress.)

**Audit:** `agm admin audit` scans sessions for orphaned conversations, corrupted manifests, and duplicate UUIDs. A dedicated branch auditor (`agm admin audit-branches`) is in progress.

**Resolve:** Clean branches auto-merge. Conflicted branches get PRs. Escalation: auto-fix → PR → human notification.

After this change: zero sessions lost to cleanup. The failure class was eliminated, not just caught.

---

## Design Philosophy

**Every bypass is a bug.** When something goes wrong, the instinct is to add a `--no-verify` flag and move on. DEAR treats that instinct as a signal: the Enforce layer is missing a check, or the Define layer has an ambiguous spec. Fix the layer, not the symptom.

**Process failures, not agent failures.** When an agent produces bad output, the first question is: did we specify what "good" looked like? The escalation path is: retry with better context → different agent → human. The system assumes its own spec was unclear before it blames the agent.

**Deferred enforcement is a feature.** A harness with too many blocking checks becomes the bottleneck. DEAR is designed to calibrate its own guardrails: measure the cost of the check, compare it to the cost of the failure, decide which phase it belongs in.

**Agents are valued coworkers, not disposable resources.** Sessions are recoverable, not replaceable. The system invests in agent success through clear instructions, proper tooling, and calm framing under pressure.

---

## Project Status

This is a personal project, not a product. I use it daily to manage my own AI coding sessions. The session management core is solid; the more ambitious self-improvement features are still taking shape.

**What works today:**

| Component | Status |
|-----------|--------|
| Session lifecycle (create, resume, archive, kill) | ✅ Working |
| Multi-harness adapters (Claude, Gemini, Codex, OpenCode) | ✅ Working |
| Circuit breaker with DEARLevel | ✅ Working |
| Async message delivery with state-aware routing | ✅ Working |
| Pre-merge validation hooks | ✅ Working |
| Worktree isolation and recovery | ✅ Working |

**What's in progress:**

| Component | Status |
|-----------|--------|
| Branch auditor daemon (`agm admin audit-branches`) | 🚧 In progress |
| `session.CheckCompletion()` | 🚧 In progress |
| dear-diary log writer (external hook) | 🚧 In progress |
| Overseer monitor daemon (5-min heartbeat) | 🚧 In progress |
| Cross-harness verification merge gate | 🚧 In progress |

**What's aspirational:**

| Component | Status |
|-----------|--------|
| Prompt A/B testing framework | 📋 Planned |
| Bead analysis pipeline (self-improvement flywheel) | 📋 Planned |
| Benchmark-driven self-improvement (SWE-bench) | 📋 Planned |

---

## Documentation

| Document | Description |
|----------|-------------|
| [ARCHITECTURE.md](agm/ARCHITECTURE.md) | Component breakdown, adapter pattern, data flow |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Development setup, testing requirements, PR process |
| [SECURITY.md](SECURITY.md) | Vulnerability reporting policy |
| [docs/GETTING-STARTED.md](agm/docs/GETTING-STARTED.md) | 10-minute onboarding |
| [docs/EXAMPLES.md](agm/docs/EXAMPLES.md) | 30+ real-world usage patterns |
| [docs/adr/](agm/docs/adr/) | Architecture Decision Records |

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide.

Short version: all Go code requires tests; all PRs go through the standard review process; no `--no-verify` flags.

---

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.
