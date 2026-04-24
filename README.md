# dear-agent

Writing code isn't the hard part. Software engineering is.

A personal experiment in agent harness design — a pluggable meta-harness for
AI coding agents. Manage sessions, isolate workspaces, and orchestrate
multi-agent workflows, regardless of which AI CLI you use.

## Why

Modern AI coding agents (Claude Code, Gemini CLI, Codex CLI, OpenCode) are
powerful but isolated. Switching between them means rebuilding your workflow.
Running them in parallel means managing state by hand. dear-agent wraps them
behind a unified adapter and gives you session lifecycle, sandbox isolation,
and async coordination on top.

The architecture is intentionally unsophisticated. Tmux sessions as the
execution backend. SQLite as the store. Files as the coordination primitive.
The goal isn't to build a platform — it's to understand what a minimal,
correct harness looks like, and let the tooling surface evolve from that.

## Components

| Component | Directory | Description |
|-----------|-----------|-------------|
| **AGM** | `agm/` | Agent Gateway Manager — session lifecycle, orchestration, monitoring |
| **Engram** | `engram/` | Persistent memory with cue-based retrieval for AI sessions |
| **Wayfinder** | `wayfinder/` | 9-phase SDLC workflow plugin with validation gates |

## Quick Start

### Prerequisites

- Go 1.25+
- tmux
- Git

### Install

```bash
# Install AGM (session manager)
go install github.com/vbonnet/dear-agent/agm/cmd/agm@latest

# Install Engram (persistent memory)
go install github.com/vbonnet/dear-agent/engram/cmd/engram@latest
```

### Basic Usage

```bash
# Create a new session
agm session new my-feature

# List active sessions
agm session list

# Resume a session
agm session resume my-feature

# Send a message to a session
agm session send my-feature "run the tests"

# Archive when done
agm session archive my-feature
```

### Session Lifecycle

```
create → associate → work → archive
  │         │          │        │
  │         │          │        └─ Cleanup sandbox, mark lifecycle=archived
  │         │          └─ State tracking: READY → THINKING → READY
  │         └─ Link agent UUID to AGM session
  └─ Provision sandbox, create manifest, start tmux session
```

## Architecture

```
┌──────────────────────────────────────────────────────┐
│                    AGM CLI                            │
│  session · admin · workflow · send                   │
├──────────────────────────────────────────────────────┤
│              Shared Operations Layer                  │
│  (CLI, MCP server, and Skills all route here)        │
├──────────┬───────────┬───────────┬───────────────────┤
│  Claude  │  Gemini   │  Codex    │  OpenCode         │
│  Adapter │  Adapter  │  Adapter  │  Adapter          │
├──────────┴───────────┴───────────┴───────────────────┤
│              Backend Abstraction                      │
│  Tmux (current)  ·  Temporal (planned)               │
├──────────────────────────────────────────────────────┤
│              Storage & Coordination                   │
│  SQLite · Manifests · Message Queue · Sandbox        │
└──────────────────────────────────────────────────────┘
```

All three API surfaces — CLI, MCP server, and Claude Code Skills — share a
common operations layer (`agm/internal/ops/`), ensuring consistent behavior
regardless of how you interact with AGM.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full component breakdown.

## Directory Structure

```
dear-agent/
├── agm/              # AGM (session management, orchestration)
├── engram/           # Engram (persistent memory)
├── wayfinder/        # Wayfinder (SDLC workflow)
├── tools/            # Standalone CLI tools
├── cmd/              # Additional CLI entry points
├── codegen/          # Code generation framework
├── pkg/              # Shared Go packages
├── internal/         # Private implementation packages
├── scripts/          # Build and utility scripts
└── docs/             # Documentation
```

## Build & Test

```bash
# Build individual products
go build ./agm/cmd/agm
go build ./engram/cmd/engram

# Run all tests
GOWORK=off go test ./...

# Lint
golangci-lint run ./...
```

## Tools

| Tool | Directory | Description |
|------|-----------|-------------|
| `benchmark-query` | `tools/benchmark-query/` | Query benchmark metrics from test runs |
| `devlog` | `tools/devlog/` | Development log management |
| `dod-enforcer` | `tools/dod-enforcer/` | Definition-of-done enforcement |
| `schema-registry` | `tools/schema-registry/` | Schema validation registry |
| `spec-review` | `tools/spec-review/` | Specification review tooling |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and
contribution guidelines.

## License

Apache 2.0 — see [LICENSE](LICENSE) for details.
