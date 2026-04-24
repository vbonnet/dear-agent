# agm-bus channel MCP adapter

Bridges a running Claude Code session to the `agm-bus` broker so sessions can
message each other, relay permission prompts, and reach external channels
(Discord, Matrix) through a single pipe.

## What it does

- Opens `~/.agm/bus.sock` (or `$AGM_BUS_SOCKET`) and identifies itself as the
  session named by `$AGM_SESSION_NAME`.
- Declares the `claude/channel` and `claude/channel/permission` MCP capabilities
  so Claude Code routes inbound events to us and forwards permission prompts.
- Translates between the two protocols:

  | Direction                         | Adapter behavior                                 |
  |-----------------------------------|--------------------------------------------------|
  | Broker → session (`deliver`)      | `notifications/claude/channel` event in Claude   |
  | Broker → session (`permission_verdict`) | `notifications/claude/channel/permission` verdict |
  | Session → broker (agent `send` tool)    | `FrameSend` to target peer                  |
  | Session → broker (agent `permission_verdict` tool) | `FramePermissionVerdict`         |
  | Claude Code `permission_request` notification | `FramePermissionRequest` to `$AGM_PERMISSION_RELAY_TARGET` |

## Layout

```
agm-bus/
├── package.json          # node/bun package metadata + mcp-sdk dep
├── tsconfig.json         # ES2022, strict
├── src/
│   ├── wire.ts           # Frame types + streaming reader (mirrors bus/wire.go)
│   ├── broker-client.ts  # unix-socket client with auto-reconnect
│   ├── index.ts          # MCP server: tools + notification relay
│   └── wire.test.ts      # node:test unit tests for the reader
└── dist/                 # tsc output (not checked in)
```

## Running

```bash
# One-time:
npm install && npm run build

# In a session (normally done by `agm supervisor run`):
AGM_SESSION_NAME=s1 \
AGM_PERMISSION_RELAY_TARGET=s2 \
claude --dangerously-load-development-channels server:agm-bus
```

The `server:agm-bus` syntax assumes the adapter is registered in your
`.mcp.json`:

```json
{
  "mcpServers": {
    "agm-bus": {
      "command": "node",
      "args": ["/path/to/agm/agm-plugin/channels/agm-bus/dist/index.js"]
    }
  }
}
```

During the channels research preview `--dangerously-load-development-channels`
is required because this plugin isn't on the Anthropic-maintained allowlist.
Once it ships to the marketplace we can drop that flag for supervisor sessions
and eventually enable it for workers too.

## Permission relay

When Claude Code opens a Bash/Write/Edit permission dialog, it sends
`notifications/claude/channel/permission_request` to the agm-bus MCP server.
The adapter forwards that as a `permission_request` frame to
`$AGM_PERMISSION_RELAY_TARGET` (typically the peer supervisor). The peer's
adapter surfaces it as a `<channel source="agm-bus" kind="permission_request">`
event in its session context; Claude (the peer) can then call our
`permission_verdict` tool, which sends a `permission_verdict` frame back. The
originating adapter sees the verdict, translates it to a
`notifications/claude/channel/permission` notification, and Claude Code
applies it to the open dialog. Worker never deadlocks.

## Env vars

| Variable                       | Purpose                                              |
|--------------------------------|------------------------------------------------------|
| `AGM_SESSION_NAME`             | Session id this adapter identifies as. **Required.** |
| `AGM_BUS_SOCKET`               | Override unix socket path (default `~/.agm/bus.sock`)|
| `AGM_PERMISSION_RELAY_TARGET`  | Where to forward permission prompts. No default — if unset, permission relay is inert and the local terminal dialog still blocks. |

## Not yet implemented

- `permission_relay_policy` negotiation (who can answer yes/no without human involvement).
- Automatic reconnect backoff beyond the fixed 500 ms.
- Integration tests — the `wire.test.ts` unit tests cover the protocol layer;
  end-to-end tests with a real broker happen in `agm/internal/bus/server_test.go`.
