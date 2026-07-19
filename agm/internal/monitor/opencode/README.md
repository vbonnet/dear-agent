# OpenCode SSE monitor

<!-- Last audited at: 2026-07-18 -->

This package converts OpenCode server-sent events into AGM session-state
events. Use it when an OpenCode server exposes `/health` and `/event`; use the
package architecture page for boundaries and invariants.

## Construct and run

```go
cfg := opencode.DefaultConfig()
cfg.Enabled = true
cfg.ServerURL = "http://localhost:4096"
cfg.SessionID = "agm-session-id"

adapter, err := opencode.NewAdapter(bus, cfg)
if err != nil {
    return err
}
defer adapter.Stop(shutdownCtx)
if err := adapter.Start(ctx); err != nil {
    return err
}
```

`NewAdapter` requires a non-nil EventBus publisher, server URL, and AGM session
ID. `Start` probes the configured health endpoint before opening the SSE
stream. `Stop` cancels the stream and waits for its goroutines within the
caller's context.

## What downstream consumers receive

Published `SessionStateChangeEvent` values contain:

- the configured AGM session ID;
- the mapped AGM state;
- the OpenCode timestamp and available metadata;
- a process-local monotonic sequence;
- source `opencode-sse` and harness `opencode-cli`.

Broadcast is non-blocking and best effort. This package does not persist
events or prove that every subscriber accepted them.

## Health and fallback

`Health` reports connection state, last event, last heartbeat, and the server
and session identifiers. Heartbeats are tracked separately so an idle session
does not look disconnected merely because it emitted no work event.

`FallbackTmux` does not start a tmux monitor. When the health probe fails it
annotates the returned error so the caller can choose its fallback explicitly.

## Current limitation

The internal `incrementMetric` helper emits diagnostic log records only. It is
not a metrics backend. Do not describe Prometheus counters, throughput, or
latency results without implementing them and recording a dated reproducible
measurement.

## Verify

```sh
go test -race ./agm/internal/monitor/opencode -count=1
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for component seams and [SPEC.md](SPEC.md)
for observable requirements.
