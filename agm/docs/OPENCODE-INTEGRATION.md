# OpenCode SSE monitoring

<!-- Last audited at: 2026-07-18 -->

AGM can adapt an OpenCode server's `/event` stream into harness-neutral session
state events. This optional monitor does not launch OpenCode, persist sessions,
or activate another monitor when startup fails.

## Configure the monitor

Start the server separately:

```sh
opencode serve --port 4096
```

The adapter configuration uses the same server URL:

```yaml
adapters:
  opencode:
    enabled: true
    server_url: http://localhost:4096
    reconnect:
      initial_delay: 5s
      max_delay: 5m
      multiplier: 2
```

If the startup health probe fails, the adapter stays inactive and the daemon
reports that OpenCode sessions are not being monitored. AGM does not currently
implement automatic tmux failover for OpenCode.

## State mapping

| OpenCode event | AGM state |
|---|---|
| `permission.asked` | `AWAITING_PERMISSION` |
| `tool.execute.before` | `WORKING` |
| `tool.execute.after` | `IDLE` |
| `session.created` | `DONE` |
| `session.closed` | `TERMINATED` |
| unknown type | `WORKING` |

Unknown events stay working rather than being interpreted as idle. Available
permission, tool, session, and original event-type fields are retained as
metadata.

## Connection behavior

`Start` probes the configured health endpoint and then connects to
`GET <server_url>/event`. The SSE reader reconnects with configured exponential
backoff whenever the OpenCode adapter is enabled. The daemon currently retries
without a configured attempt limit; there is no separate reconnect toggle.

Health reports connection state plus the last event and heartbeat. After the
first heartbeat, more than five minutes without another is unhealthy. A stream
that has never emitted a heartbeat is evaluated from connection state alone.

Events are broadcast in process, non-blocking, and best effort. The adapter
does not provide durable delivery acknowledgement.

## Operate and verify

Use the canonical session interface for AGM status:

```sh
agm session status
```

Run the focused package verification after changing event mappings or
lifecycle behavior:

```sh
go test -race ./agm/internal/monitor/opencode -count=1
```

For package boundaries and observable requirements, see
[`../internal/monitor/opencode/ARCHITECTURE.md`](../internal/monitor/opencode/ARCHITECTURE.md)
and [`../internal/monitor/opencode/SPEC.md`](../internal/monitor/opencode/SPEC.md).
