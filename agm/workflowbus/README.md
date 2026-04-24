# workflowbus — workflow ↔ agm-bus gate bridge

Connects a `pkg/workflow` Runner to the agm-bus broker so Gate nodes can
be unblocked by external messages (Discord DMs, Matrix room posts,
`agm send` from any session) routed via the Layer 3 bus.

## How it works

1. A workflow declares a Gate node whose `gate.name` is, say, `"approve"`.
2. A Bridge instance registered as pseudo-session `wf-<id>` on the bus
   receives `FrameDeliver` frames addressed to it.
3. Any delivered frame with either shape:
   - `Text` starts with `gate:<name>` (e.g. `"gate:approve"`), or
   - `Extra["kind"] == "gate"` AND `Extra["gate"] == "<name>"`

   becomes a `runner.Signal(name)` call, unblocking the Gate.

## Typical wiring

```go
import (
    "github.com/vbonnet/dear-agent/agm/workflowbus"
    "github.com/vbonnet/dear-agent/pkg/workflow"
)

runner := workflow.NewRunner(ai)
bridge := workflowbus.New("wf-research-pipeline", runner)

go func() { _ = bridge.Start(ctx) }()

// The Runner runs normally; any Gate whose signal name matches an
// inbound gate-shaped bus message will unblock.
_, err := runner.Run(ctx, w, inputs)
```

## Triggering a gate from an external source

From any shell with the `agm` CLI installed (or a Discord DM routed
through the DiscordAdapter):

```sh
agm send wf-research-pipeline gate:approve
```

Or via the Matrix adapter, from a Matrix room the bot is in:

```
to:wf-research-pipeline gate:approve
```

## Pseudo-session naming

The bridge's SessionID must be unique within the broker's Registry.
Convention: `wf-<workflow-name>` or `wf-<uuid>` for multi-tenant setups.
Re-Starting a bridge with the same id after a crash is safe — the
broker unregisters the stale delivery when the old socket closed.

## Reconnect behavior

The bridge auto-reconnects if the broker socket drops. `ReconnectDelay`
throttles the loop (default 2s). A duplicate-session-id rejection on
reconnect backs off and retries; the broker drops the stale entry when
the old socket eventually closes.

## Not in this package

- Sending outbound from a workflow to an external source. Use an
  `ai`/`bash` node to invoke `agm send` or the broker's Go client
  directly.
- Transforming gate names (e.g. namespacing per-workflow). Use your own
  Signaler wrapper if you need renaming.
