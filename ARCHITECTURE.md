# Architecture

<!-- Last audited at: 2026-07-27 -->

## High-Level Overview

dear-agent is a Go monorepo organized around four products that share common
infrastructure. AGM (Agent Gateway Manager) is the core product — it manages
the lifecycle of AI coding agent sessions across multiple harnesses.

```
┌─────────────────────────────────────────────────────────────────┐
│                         User / Automation                       │
│                                                                 │
│   CLI (agm ...)     MCP Server (JSON-RPC)     Skills (.md)     │
└────────┬──────────────────┬────────────────────────┬────────────┘
         │                  │                        │
         v                  v                        v
┌─────────────────────────────────────────────────────────────────┐
│              Operations + CLI Lifecycle Logic                    │
│                                                                 │
│  internal/ops covers many reusable API operations, while some   │
│  CLI lifecycle paths still own harness/tmux control directly.   │
│                                                                 │
│  Examples in ops: list/get/search/status, health, tag, retry,   │
│  compact helpers, GC, install, and selected send/create paths.  │
│                                                                 │
│  Examples still leaky in cmd/agm: session new, resume/attach,   │
│  send msg, and mode/model command dispatch.                     │
├─────────────────────────────────────────────────────────────────┤
│                    Concrete Harness Adapters                     │
│                    (internal/agent/)                             │
│                                                                 │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────────┐  │
│  │  Claude   │ │  Gemini   │ │  Codex    │ │   OpenCode    │  │
│  │  Adapter  │ │  Adapter  │ │  Adapter  │ │   Adapter     │  │
│  └───────────┘ └───────────┘ └───────────┘ └───────────────┘  │
│                                                                 │
│  Concrete adapters expose harness-specific mechanisms.          │
│  Heterogeneous discovery sees only Harness metadata:            │
│  - Canonical name, adapter version, and capabilities            │
│  - Lifecycle ordering belongs to operation-specific consumers   │
│  - No universal adapter lifecycle facade                        │
├─────────────────────────────────────────────────────────────────┤
│                    Backend Abstraction                           │
│                    (internal/backend/)                           │
│                                                                 │
│  ┌──────────────────────┐  ┌──────────────────────────────┐    │
│  │   Tmux Backend       │  │   Temporal Backend (planned) │    │
│  │   Session mgmt,      │  │   Durable execution,         │    │
│  │   pane control,      │  │   workflow orchestration      │    │
│  │   key sending        │  │                               │    │
│  └──────────────────────┘  └──────────────────────────────┘    │
├─────────────────────────────────────────────────────────────────┤
│                    Storage & Coordination                        │
│                                                                 │
│  ┌─────────┐ ┌───────────┐ ┌──────────┐ ┌────────────────┐    │
│  │  Dolt   │ │ Manifests │ │ Message  │ │   Sandbox      │    │
│  │  DB     │ │  (YAML)   │ │  Queue   │ │  (OverlayFS /  │    │
│  │         │ │           │ │          │ │   APFS)        │    │
│  └─────────┘ └───────────┘ └──────────┘ └────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## Key Components

### AGM CLI (`agm/cmd/agm/`)

The primary user interface. Cobra-based command tree with these groups:

- **Session commands** — `new`, `resume`, `list`, `archive`, `kill`, `associate`
- **Admin commands** — `doctor`, `fix-uuid`, `clean`, `unlock`, `migrate`,
  `check-worktrees`, `cleanup-worktrees`
- **Workflow commands** — `deep-research`, `code-review`, `architect`
- **Communication** — `send`, `compact`

### Operations Layer and CLI Lifecycle Split

`agm/internal/ops/` is a reusable operations layer for many command and API
paths, but it is not yet the single business-logic funnel for every AGM
surface. The real split is:

```
MCP / JSON-friendly paths  →  internal/ops  → storage / tmux / backend
Many CLI commands          →  internal/ops  → storage / tmux / backend
Skills (.md)               →  CLI commands  → whichever path that command uses
CLI lifecycle paths        →  cmd/agm helpers + harness/tmux switches
```

- `OpContext` provides dependency injection for storage, tmux, manager backend,
  config, and output preferences where a command uses ops.
- RFC 7807 structured errors with stable error codes (AGM-001 through AGM-100)
- Field masks via `--fields` for token-efficient output
- JSON output mode for programmatic consumers
- `agm session new` still runs through `agm/cmd/agm/new*.go`, including
  sandbox setup, tmux creation, harness command construction, post-create
  hooks, and attach/detach handling.
- `agm session resume` still runs through `agm/cmd/agm/resume.go`, including
  identifier resolution, health checks, tmux recreation, and harness-specific
  resume command delivery.
- `agm send msg` still runs through `agm/cmd/agm/send_msg.go`, including
  queueing, safety checks, pending-file writes, tmux delivery, and a legacy
  API-adapter branch for API-based harness names.

This leaky split is intentional documentation, not a re-architecture plan. The
ops/adapter boundary cleanup is tabled separately.

### Harness Adapters (`agm/internal/agent/`)

The adapter pattern is central to AGM's multi-harness support. Concrete
adapters encapsulate harness-specific mechanisms. Heterogeneous discovery and
conformance see only the metadata-sized `Harness` contract; operation owners
define any behavioral capability interfaces they consume:

| Adapter | Harness | Key Capabilities |
|---------|---------|-----------------|
| Claude | Claude Code | UUID detection, slash commands, history.jsonl parsing |
| Gemini | Gemini CLI | API integration, session file management |
| Codex | Codex CLI | CLI launch/resume, composer readiness detection, model alias resolution |
| OpenCode | OpenCode CLI | SSE event streams, server port management |

Adding a new harness starts with a concrete adapter, the metadata contract, and
the finite constructor/model catalogs. Each operation that needs new behavior
must define or extend a capability-sized consumer boundary; any still-leaky CLI
lifecycle switches must be audited as well.

### Session Management (`agm/internal/session/`)

Sessions are the primary resource. Each session has:

- **Manifest** (YAML, v3 schema) — metadata, lifecycle state, harness type,
  model, sandbox config, context usage
- **Dolt record** — queryable session metadata with Git-like versioned SQL
- **State** — READY, THINKING, PERMISSION_PROMPT, COMPACTING, OFFLINE

State is detected via a priority chain: hook → tmux → manual.

### Sandbox Isolation (`internal/sandbox/`)

Copy-on-write filesystem isolation so agents work in contained environments:

```
┌────────────────────────────────────────┐
│          Provider Interface            │
├────────────┬────────────┬──────────────┤
│ Bubblewrap │ OverlayFS  │     APFS     │
│ (Linux)    │ (Linux)    │   (macOS)    │
└────────────┴────────────┴──────────────┘
```

- **Bubblewrap** — Linux: materialized worktree with namespace validation
- **OverlayFS** — Linux: upper/lower/work/merged directory structure
- **APFS** — macOS: cloned directories with isolated project-path mapping

Sandbox lifecycle is tied to session lifecycle: provisioned on `new`, cleaned
up on `archive`.

### Multi-Agent Orchestration

AGM supports coordinated parallel agent work through several mechanisms:

- **Coordination Daemon** (`agm/internal/daemon/`) — Background process polling
  every 30s for pending messages and translating shared direct-delivery
  outcomes into defer, retry, acknowledgment, and queue bookkeeping
- **Message Delivery Operation** (`agm/internal/ops/`) — Resolves a recipient,
  selects API or tmux transport, and couples harness readiness to direct
  delivery under the stable-session lifecycle lock; CLI, MCP, and daemon
  callers retain only their distinct policy
- **Pending Messages** (`agm/internal/messages/`) — File-based inter-agent
  messaging via `~/.agm/pending/{session}/` directories
- **Advisory File Reservations** (`agm/internal/reservation/`) — Glob-pattern
  based file locks (advisory, not enforced) to prevent destructive concurrent
  edits
- **A2A Agent Cards** (`agm/internal/a2a/`) — A2A Protocol agent discovery via
  generated Agent Cards

> **VROOM is not an AGM-internal component.** VROOM is the supervisory
> **execution framework** that sits *above* AGM and drives it as a tool —
> three supervisors (Meta-Orchestrator / Orchestrator / Overseer) plus per-task
> Primary/Secondary/Tertiary ownership, Workers, Auditors, and SRE agents. It
> is intentionally *not* an `internal/` package here. See
> [CONTEXT.md](CONTEXT.md) and
> [docs/adr/ADR-002](docs/adr/ADR-002-vroom-execution-architecture.md). (The
> earlier "five-role Verifier/Requester/…" description was inaccurate and is
> superseded.)

### State Monitor — Astrocyte (`agm/internal/monitor/`)

Real-time agent state detection with harness-specific strategies:

- Hook-based detection (Claude Code PreToolUse/PostToolUse/Stop hooks)
- Tmux pane content inspection
- SSE event streams (OpenCode)
- Health check caching to avoid probe storms

### Identifier Resolution (`agm/internal/session/`)

Multi-strategy session lookup: exact match → UUID prefix → fuzzy match →
interactive picker. Users never need to type exact session names.

## Data Flow: Session Lifecycle

### 1. Creation (`agm session new my-feature`)

```
User → CLI validates flags
     → SessionManager generates UUID
     → Adapter selected (--harness flag or default)
     → Sandbox provisioned (OverlayFS/APFS/worktree)
     → Manifest written (YAML v3)
     → Dolt record inserted
     → Tmux session started with agent CLI
     → User attached to tmux session
```

### 2. Association (`agm session associate`)

```
Agent starts → Hook fires (or manual association)
            → Claude UUID detected from history.jsonl
            → Manifest updated with UUID binding
            → State tracking begins (hook → tmux → manual)
```

### 3. Active Work

```
Agent idle (READY) → Message arrives
                   → Daemon delivers via tmux send-keys
                   → State transitions to THINKING
                   → Agent processes, calls tools
                   → Hook updates state on each tool call
                   → Returns to READY
```

### 4. Archival (`agm session archive my-feature`)

```
User → Completion verified (no pending work)
     → MCP processes cleaned up
     → Sandbox destroyed (unmount/remove)
     → Manifest marked lifecycle=archived
     → Dolt record updated
     → Tmux session killed
```

## Extension Points

| Extension | How |
|-----------|-----|
| New AI harness | Add a concrete adapter plus metadata and finite-catalog entries in `agm/internal/agent/` |
| New backend | Implement the `Backend` interface in `agm/internal/backend/` |
| New sandbox provider | Implement the `Provider` interface in `internal/sandbox/` |
| New storage backend | Implement the storage interface in `agm/internal/dolt/` |
| New workflow | Add workflow definition in `agm/internal/workflow/` |
| Custom state detection | Add monitor strategy in `agm/internal/monitor/` |

## Monorepo Structure

```
dear-agent/
├── agm/                 # AGM: session management & orchestration
│   ├── cmd/agm/         #   CLI entry point
│   ├── internal/        #   Core logic (ops, agent, session, backend, ...)
│   └── docs/            #   ADRs, specs, capability matrix
├── engram/              # Engram: persistent memory
│   ├── cmd/engram/      #   CLI entry point
│   ├── ecphory/         #   Cue-based retrieval engine
│   └── retrieval/       #   Memory retrieval strategies
├── wayfinder/           # Wayfinder: SDLC workflow
│   ├── cmd/             #   CLI entry point
│   └── review/          #   Phase review tooling
├── pkg/                 # Shared packages
│   ├── cliframe/        #   CLI framework utilities
│   ├── llm/             #   Unified LLM provider interface
│   ├── monitoring/      #   Observability helpers
│   ├── table/           #   ASCII table rendering
│   └── telemetry/       #   Telemetry collection
├── internal/            # Private shared packages
│   └── sandbox/         #   Sandbox provider implementations
├── tools/               # Standalone CLI tools
└── scripts/             # Build and utility scripts
```

## Design Principles

1. **Adapter pattern for extensibility** — Harness-specific logic should live
   in adapters where the current code supports it, but session creation,
   resume, send, and mode/model dispatch still have command-layer harness
   switches that must be kept honest.
2. **Shared operations layer** — `agm/internal/ops` is the target home for
   reusable API behavior and already backs many surfaces, but not all CLI
   lifecycle behavior routes through it yet.
3. **Configuration cascade** — CLI flags → environment variables → config file
   → smart defaults.
4. **Advisory over enforced** — File reservations warn rather than block,
   avoiding deadlocks in multi-agent scenarios.
5. **Dependency injection** — External dependencies (tmux, filesystem) injected
   via `OpContext` for testability.
6. **Fail-fast test isolation** — Tests are blocked from touching production
   workspaces at the infrastructure level.
