# OpenCode SSE monitoring

<!-- Last audited at: 2026-07-18 -->

AGM can adapt an OpenCode server's `/event` stream into harness-neutral session
state events. This optional monitor does not launch OpenCode, persist sessions,
or own fallback selection.

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
      enabled: true
      initial_delay: 5s
      max_delay: 5m
      backoff_multiplier: 2
    fallback_tmux: true
```

`fallback_tmux: true` changes the error context returned after a failed startup
probe. It does not start Astrocyte or any other tmux monitor. The caller must
select and start a fallback explicitly.

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
backoff. A positive maximum retry count stops retries after that many failures.

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
