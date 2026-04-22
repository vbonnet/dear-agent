# dear-agent

**A closed-loop agent harness implementing the DEAR protocol.**

DEAR (Define, Enforce, Audit, Resolve) is an architectural pattern for building AI agent systems that get permanently better after every failure. dear-agent is its reference implementation: a session manager, orchestration layer, and safety harness for multi-agent workflows across Claude, Gemini, Codex, and OpenCode.

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
- **Immutable audit trail** — `dear-diary` records every session event as append-only JSONL. Never deleted, always queryable.
- **Worktree isolation** — Agents operate in isolated filesystem environments. Work is recoverable even if a session dies mid-task.
- **Overseer monitor** — Runs on a 5-minute heartbeat, detects stalled sessions, false completions, and missed permission prompts.
- **Cross-harness verification** — A different model family reviews agent output before merging. Independent review catches shared blind spots.

---

## Quick Start

### Prerequisites

- Go 1.23+
- tmux
- Git

### Install

```bash
go install github.com/vbonnet/dear-agent/cmd/agm@latest
```

### First Session

```bash
# Start a new agent session
agm session new my-feature

# Check status across all sessions
agm status

# Send a message to a running session
agm session send my-feature "run the tests and report back"

# Archive when done
agm session archive my-feature
```

### Multi-Agent Workflow

```bash
# Launch an orchestrator session (Claude Code)
agm session new orchestrator --agent claude

# Launch worker sessions (different harnesses for cross-verification)
agm session new worker-1 --agent claude
agm session new worker-2 --agent gemini

# The orchestrator dispatches work; workers execute and report back
# Overseer monitors all three for stalls and false completions
agm admin overseer start
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

**Enforce:** Pre-exit hook blocks termination if any predicate fails. UUID branch names are rejected at creation time. Cleanup process now auto-commits and pushes before running.

**Audit:** `agm admin audit-branches` scans all repos for stale/unmerged work. Overseer runs this check every 5 minutes.

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

dear-agent is in active development. The Enforce layer is production-ready. The Audit layer is partially implemented. The Resolve & Refine self-improvement loop is the current focus.

| Component | Status |
|-----------|--------|
| Session lifecycle (create, resume, archive, kill) | ✅ Production |
| Multi-harness adapters (Claude, Gemini, Codex, OpenCode) | ✅ Production |
| Circuit breaker with DEARLevel | ✅ Production |
| Async message delivery with state-aware routing | ✅ Production |
| dear-diary immutable event log | ✅ Production |
| Overseer monitor (5-min heartbeat) | ✅ Production |
| Cross-harness verification gate | ✅ Production |
| Pre-merge validation hooks | ✅ Production |
| Worktree isolation and recovery | ✅ Production |
| Branch auditor daemon | 🚧 In progress |
| `session.CheckCompletion()` | 🚧 In progress |
| Prompt A/B testing framework | 📋 Planned |
| Bead analysis pipeline (self-improvement) | 📋 Planned |

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
