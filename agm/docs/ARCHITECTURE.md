# AGM architecture

<!-- Last audited at: 2026-07-27 -->

AGM is a local-first session coordinator for interactive AI command-line
harnesses. It owns session lifecycle, persistent metadata, tmux process
coordination, queued messages, and multiple operator surfaces. Detailed runtime
behavior belongs in source and generated `agm --help`; this page maps the stable
boundaries.

## Control plane

```text
operator / automation / MCP client
                 |
       +---------+----------+
       |                    |
    AGM CLI            AGM MCP server
       |                    |
       +---------+----------+
                 |
          internal/ops
                 |
       +---------+----------+
       |                    |
  internal/agent       internal/session
 concrete adapters     tmux and state
       |                    |
       +---------+----------+
                 |
          internal/dolt
        session metadata
```

The CLI and MCP server share lifecycle operations through `internal/ops`.
Surfaces may add presentation or transport concerns, but they must not maintain
independent create, archive, kill, or direct-delivery semantics.
`internal/ops.SendMessage` resolves the recipient, selects API or tmux
transport, reloads mutable delivery identity under the stable-session lifecycle
lock, and owns the atomic readiness-plus-send transaction for CLI, MCP, and
daemon callers. The CLI retains formatting, logging, queue choice, overlay
recovery, and presentation; the daemon retains dequeue scheduling, defer/retry
accounting, acknowledgments, and durable queue state.

## Source owners

| Concern | Executable owner |
|---|---|
| Cobra command tree and generated help | `agm/cmd/agm` |
| Cross-surface lifecycle operations | `agm/internal/ops` |
| Active and deprecated harness registry | `agm/internal/agent/harnesses.go` |
| Harness constructors | `agm/internal/agent/factory.go` |
| Harness adapters | `agm/internal/agent/*_adapter.go` |
| Session resolution and runtime state | `agm/internal/session` |
| Tmux process interaction | `agm/internal/tmux` |
| Persistent session metadata | `agm/internal/dolt` |
| Direct inter-session delivery | `agm/internal/ops.SendMessage` |
| Queued inter-session scheduling | `agm/internal/messages`, `agm/internal/daemon` |
| Shared runtime thresholds | `agm/internal/contracts` |
| Sandbox selection and provisioning | `internal/sandbox`, `agm/cmd/agm/new_sandbox.go` |

## Harness model

Harness names and lifecycle status come only from
`agm/internal/agent/harnesses.go`.

| Harness | Lifecycle | Integration |
|---|---|---|
| `claude-code` | active | Claude Code CLI in tmux |
| `codex-cli` | active | Codex CLI in tmux |
| `agy` | active | AGY CLI in tmux |
| `opencode-cli` | active | OpenCode CLI in tmux, with optional SSE monitoring |
| `pi-cli` | active | Pi CLI in tmux |
| `gemini-cli` | deprecated compatibility | Gemini CLI in tmux |

Active harnesses are the parity set. A deprecated harness can remain accepted
for existing manifests without becoming a default, a new feature target, or an
active parity promise. Adapter constructors return concrete types. Heterogeneous
discovery and conformance use the metadata-only `agent.Harness` contract
(`Name`, `Version`, and `Capabilities`); operation consumers define narrower
behavioral capabilities such as context-aware pure API delivery.

## Session creation

```text
CLI or MCP request
    -> validate caller, workspace, harness, model, and name
    -> internal/ops.CreateSessionWithContext
    -> launch the selected CLI harness in tmux
    -> persist the manifest through internal/dolt
    -> perform surface-specific readiness or response handling
```

The CLI runtime hooks are in `agm/cmd/agm/new_session.go`; MCP supplies its own
runtime dependencies from `agm/cmd/agm-mcp-server/tools.go`. Both identify their
caller provenance and use the shared operation.

Fresh AGY sessions require a startup prompt because AGY persists native identity
lazily on first input. Shared creation delivers that prompt once after native
readiness and before identity discovery and registration, then marks it consumed
so surface-specific completion cannot resend it.

Sandbox provisioning occurs before the shared creation lifecycle when requested.
`agm/cmd/agm/new.go` imports platform provider packages so their `init`
registrations are available to `internal/sandbox.NewProvider`.

## Storage and discovery

`internal/dolt.Adapter` is the production session metadata boundary. Workspace
configuration selects the Dolt server and database; a SQLite implementation is
reserved for isolated test environments. Callers should use the adapter rather
than query storage directly.

Session identifiers can be names, IDs, UUID prefixes, or tmux names depending on
the operation. Resolution is centralized in `internal/session` and the storage
adapter so command surfaces do not invent different matching rules.

The message queue is separate from session metadata. It is a SQLite WAL database
at `~/.config/agm/message_queue.db`, owned by `internal/messages` and consumed by
the daemon.

## Runtime coordination

AGM coordinates interactive processes through tmux. Harness adapters construct
CLI commands and translate common operations; session and tmux packages own
process discovery, pane delivery, and readiness checks.

The daemon is a queue-delivery worker, not a general session-status HTTP server.
Its current design is documented in
[`../cmd/agm-daemon/ARCHITECTURE.md`](../cmd/agm-daemon/ARCHITECTURE.md).

The AGM MCP server is an alternate local control surface over shared operations.
Its tool and transport boundaries are documented in
[`../cmd/agm-mcp-server/ARCHITECTURE.md`](../cmd/agm-mcp-server/ARCHITECTURE.md).

## Cross-harness invariants

- Core session outcomes must be reachable from every active harness.
- Harness-specific extensions may improve an experience but cannot become the
  only path to a core outcome.
- New defaults and documentation must use active harness names from the
  registry; deprecated names must be visibly marked.
- Model aliases and command flags belong to executable registries and generated
  help, not copied architecture tables.
- Session lifecycle mutations must pass through shared operations or an
  explicitly documented compatibility boundary.

## Failure boundaries

- A missing or unreachable production workspace is an operational error, not a
  reason to silently create a second source of truth.
- A harness launch failure must not leave a manifest that appears active.
- Queue delivery checks tmux existence and pane readiness before sending; display
  state is diagnostic, not the delivery authority.
- Destructive session and sandbox cleanup must target only resources owned by
  the selected session.

## Verification

- Unit and integration tests live beside the owning packages.
- Cross-surface lifecycle parity is exercised through `internal/ops` tests and
  BDD features under `agm/test/bdd/features`.
- Documentation inventories are checked by
  `internal/instructions/subsystem_architecture_test.go`.
- Use `go test ./agm/...` for the AGM module and repository preflight before
  publishing a cross-cutting change.

## Non-goals

This page is not a command reference, model comparison, performance report, or
roadmap. Use `agm --help`, package source, tests, and accepted current ADRs for
those questions.
