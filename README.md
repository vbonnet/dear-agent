# dear-agent

Writing code isn't the hard part. Software engineering is.

A personal experiment in agent harness design — a pluggable meta-harness for
AI coding agents. The core idea: keep the harness thin, keep the substrate
queryable, and let loops do the recurring work.

## Why

Modern AI coding agents (Claude Code, Gemini CLI, Codex CLI) are powerful but
isolated. dear-agent wraps them with:

- **Loops** — named, recurring bash commands that run on a cadence and store
  every run in SQLite. `agm loop new babysit-prs --cadence 5m --cmd "gh pr list"`.
- **Sessions** — isolated tmux workspaces with lifecycle management.
- **Workflows** — YAML DAG engine for multi-step, multi-agent orchestration.
- **Engram** — persistent memory with cue-based retrieval across sessions.

The architecture stays deliberately unsophisticated: tmux sessions, SQLite
stores, files as coordination primitives. The substrate is queryable — when
something breaks you open a terminal, not a dashboard.

## Components

| Component | Directory | Description |
|-----------|-----------|-------------|
| **AGM** | `agm/` | Agent Gateway Manager — sessions, loops, orchestration |
| **Engram** | `engram/` | Persistent memory with cue-based retrieval |
| **Wayfinder** | `wayfinder/` | 9-phase SDLC workflow plugin with validation gates |

## Quick Start

### Prerequisites

- Go 1.25+
- tmux
- Git

### Install

```bash
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest
go install github.com/vbonnet/dear-agent/engram/cmd/engram@latest
```

## Loops — the primary UX

Loops are the simplest way to run recurring background work. A loop is a named
bash command that fires on a cadence and stores every run in SQLite.

```bash
# Create loops for things you want to keep an eye on.
agm loop new babysit-prs  --cadence 5m  --cmd "gh pr list --author @me"
agm loop new watch-ci     --cadence 2m  --cmd "gh run list --limit 5"
agm loop new dep-freshness --cadence 24h --cmd "./scripts/check-outdated.sh"

# See what's defined and when it last ran.
agm loop list

# Trigger one immediately (outside its cadence).
agm loop run babysit-prs

# Wire tick to cron so loops fire automatically.
# crontab: * * * * * agm loop tick

# Inspect run history — stdout, stderr, exit codes, durations.
agm loop logs babysit-prs

# Pause when you don't need it; resume when you do.
agm loop pause dep-freshness
agm loop resume dep-freshness
```

Every run is persisted to `~/.agm/loops.db` (WAL-mode SQLite). Query it
directly when you need to understand what happened:

```sql
SELECT l.name, r.started_at, r.success, r.exit_code
FROM loop_runs r JOIN loops l USING (loop_id)
ORDER BY r.started_at DESC LIMIT 20;
```

## Sessions

```bash
# Create and resume named sessions.
agm session new my-feature
agm session resume my-feature
agm session list
agm session archive my-feature
```

```
create → associate → work → archive
  │         │          │        │
  │         │          │        └─ Cleanup sandbox, mark lifecycle=archived
  │         │          └─ State tracking: READY → THINKING → READY
  │         └─ Link agent UUID to AGM session
  └─ Provision sandbox, create manifest, start tmux session
```

## Workflows — multi-step DAGs

For tasks that need more than one step, the workflow engine provides a YAML DAG
with bash and AI nodes, retry policies, gate conditions, and full audit
logging.

```bash
agm workflow create build-pipeline -f pipeline.yaml
agm workflow run build-pipeline
```

```yaml
# pipeline.yaml
name: build-and-test
tasks:
  - id: lint
    command: golangci-lint run ./...
  - id: build
    command: go build ./...
    depends_on: [lint]
  - id: test
    command: go test ./...
    depends_on: [build]
```

Every node state transition is logged to SQLite with actor, from-state,
to-state, and reason. When a run fails at 3am you query `audit_events` —
not a dashboard.

## Architecture

```
┌──────────────────────────────────────────────────────┐
│              AGM CLI  ·  MCP Server  ·  Skills        │
├──────────────────────────────────────────────────────┤
│              Shared Operations Layer                  │
│  (all three API surfaces route through agm/internal/ops/) │
├──────────┬───────────┬───────────┬───────────────────┤
│  Claude  │  Gemini   │  Codex    │  OpenCode         │
│  Adapter │  Adapter  │  Adapter  │  Adapter          │
├──────────┴───────────┴───────────┴───────────────────┤
│              Backend Abstraction                      │
│  Tmux (current)  ·  Temporal (planned)               │
├──────────────────────────────────────────────────────┤
│              Storage & Coordination                   │
│  loops.db  ·  runs.db  ·  Manifests  ·  Sandbox     │
└──────────────────────────────────────────────────────┘
```

Three API surfaces — CLI, MCP server, Claude Code Skills — share a common
operations layer (`agm/internal/ops/`). An operation implemented once is
available everywhere.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full component breakdown.

## Directory Structure

```
dear-agent/
├── agm/              # AGM (session management, loops, orchestration)
├── engram/           # Engram (persistent memory)
├── wayfinder/        # Wayfinder (SDLC workflow)
├── tools/            # Standalone CLI tools
├── cmd/              # Additional CLI entry points
├── codegen/          # Code generation framework
├── pkg/              # Shared Go packages
│   └── workflow/     # Workflow engine (YAML DAG, SQLite substrate)
├── internal/         # Private implementation packages
├── scripts/          # Build and utility scripts
└── docs/             # ADRs and design documents
```

## Build & Test

```bash
go build ./agm/cmd/agm
go build ./engram/cmd/engram

GOWORK=off go test ./...

golangci-lint run ./...
```

## Tools

| Tool | Directory | Description |
|------|-----------|-------------|
| `benchmark-query` | `tools/benchmark-query/` | Query benchmark metrics |
| `devlog` | `tools/devlog/` | Development log management |
| `dod-enforcer` | `tools/dod-enforcer/` | Definition-of-done enforcement |
| `schema-registry` | `tools/schema-registry/` | Schema validation registry |
| `spec-review` | `tools/spec-review/` | Specification review tooling |

## Design Philosophy

- **Loops over dashboards.** Recurring, purpose-specific background work beats
  manual polling. If you're checking something more than once a day, it should
  be a loop.
- **Substrate quality.** Every state transition is written to SQLite. Debugging
  is `SELECT * FROM audit_events WHERE run_id = ?`, not log-grep.
- **Role-based model abstraction.** Workflows declare roles (`analyst`,
  `implementer`), not model IDs. One edit to `roles.yaml` moves the whole
  system to a new model.
- **Pre-committed to deletion.** The harness should thin out as models improve.
  Nothing here is designed to be permanent.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and
contribution guidelines.

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.
