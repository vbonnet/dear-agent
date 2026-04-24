# Architecture

## High-Level Overview

AI Tools is a Go monorepo organized around four products that share common
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
│                   Shared Operations Layer                        │
│                     (internal/ops/)                              │
│                                                                 │
│  ListSessions · GetSession · SearchSessions · GetStatus · ...  │
│                                                                 │
│  OpContext: dependency injection container for storage, tmux,   │
│  config, and output formatting preferences                      │
├─────────────────────────────────────────────────────────────────┤
│                    Harness Adapter Registry                      │
│                    (internal/agent/)                             │
│                                                                 │
│  ┌───────────┐ ┌───────────┐ ┌───────────┐ ┌───────────────┐  │
│  │  Claude   │ │  Gemini   │ │  Codex    │ │   OpenCode    │  │
│  │  Adapter  │ │  Adapter  │ │  Adapter  │ │   Adapter     │  │
│  └───────────┘ └───────────┘ └───────────┘ └───────────────┘  │
│                                                                 │
│  Each adapter implements the Agent interface:                   │
│  - Start/stop agent CLI                                        │
│  - Translate AGM commands to agent-specific actions             │
│  - Detect agent state (UUID, history, session files)            │
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

### Shared Operations Layer (`internal/ops/`)

The abstraction that makes AGM accessible from three API surfaces:

```
CLI (Cobra)    →  internal/ops  →  Dolt Storage
MCP (JSON-RPC) →  internal/ops  →  Dolt Storage
Skills (.md)   →  CLI --json    →  internal/ops  →  Dolt Storage
```

- `OpContext` provides dependency injection (storage, tmux client, config)
- RFC 7807 structured errors with stable error codes (AGM-001 through AGM-100)
- Field masks via `--fields` for token-efficient output
- JSON output mode for programmatic consumers

### Harness Adapters (`internal/agent/`)

The adapter pattern is central to AGM's multi-harness support. Each adapter
implements the `Agent` interface, encapsulating all harness-specific logic:

| Adapter | Harness | Key Capabilities |
|---------|---------|-----------------|
| Claude | Claude Code | UUID detection, slash commands, history.jsonl parsing |
| Gemini | Gemini CLI | API integration, session file management |
| Codex | Codex CLI | Thread management, OpenAI API integration |
| OpenCode | OpenCode CLI | SSE event streams, server port management |

Adding a new harness requires implementing the `Agent` interface — no changes
to the core operations layer.

### Session Management (`internal/session/`)

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
│ OverlayFS  │   APFS     │ ClaudeCode   │
│ (Linux)    │  (macOS)   │ (Worktree)   │
└────────────┴────────────┴──────────────┘
```

- **OverlayFS** — Linux: upper/lower/work/merged directory structure
- **APFS** — macOS: cloned volumes with instant snapshots
- **ClaudeCode** — Git worktree-based isolation (lightweight fallback)
- **Presets** — `ReadOnlySpec()`, `FullAccessSpec()`, `CodeOnlySpec()`

Sandbox lifecycle is tied to session lifecycle: provisioned on `new`, cleaned
up on `archive`.

### Multi-Agent Orchestration

AGM supports coordinated parallel agent work through several mechanisms:

- **Coordination Daemon** (`internal/daemon/`) — Background process polling
  every 30s for pending messages, delivering when target sessions are READY
- **Pending Messages** (`internal/messages/`) — File-based inter-agent
  messaging via `~/.agm/pending/{session}/` directories
- **Advisory File Reservations** (`internal/reservation/`) — Glob-pattern
  based file locks (advisory, not enforced) to prevent destructive concurrent
  edits
- **A2A Agent Cards** (`internal/a2a/`) — A2A Protocol agent discovery via
  generated Agent Cards
- **VROOM Architecture** — Five-role supervisory model: Verifier, Requester,
  Orchestrator, Overseer, Meta-Orchestrator

### State Monitor — Astrocyte (`internal/monitor/`)

Real-time agent state detection with harness-specific strategies:

- Hook-based detection (Claude Code PreToolUse/PostToolUse/Stop hooks)
- Tmux pane content inspection
- SSE event streams (OpenCode)
- Health check caching to avoid probe storms

### Identifier Resolution (`internal/session/`)

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
| New AI harness | Implement the `Agent` interface in `internal/agent/` |
| New backend | Implement the `Backend` interface in `internal/backend/` |
| New sandbox provider | Implement the `Provider` interface in `internal/sandbox/` |
| New storage backend | Implement the storage interface in `internal/dolt/` |
| New workflow | Add workflow definition in `internal/workflow/` |
| Custom state detection | Add monitor strategy in `internal/monitor/` |

## Monorepo Structure

```
ai-tools/
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
├── research/            # Research: deep research & feeds
│   ├── cmd/             #   CLI entry points
│   └── autonomous/      #   Autonomous research engine
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

1. **Adapter pattern for extensibility** — All harness-specific logic isolated
   in adapters. Adding a new AI CLI never touches core operations.
2. **Shared operations layer** — CLI, MCP, and Skills all route through the
   same business logic. No behavior divergence between API surfaces.
3. **Configuration cascade** — CLI flags → environment variables → config file
   → smart defaults.
4. **Advisory over enforced** — File reservations warn rather than block,
   avoiding deadlocks in multi-agent scenarios.
5. **Dependency injection** — External dependencies (tmux, filesystem) injected
   via `OpContext` for testability.
6. **Fail-fast test isolation** — Tests are blocked from touching production
   workspaces at the infrastructure level.
