<!-- Last audited at: 2026-06-16 -->

# otelsetup — local OpenTelemetry tracing for dear-agent

`pkg/otelsetup` is the shared OpenTelemetry bootstrap. `InitTracer(serviceName)`
installs a global `TracerProvider` and returns a shutdown func. Tracing is
**opt-in**:

- **`OTEL_EXPORTER_OTLP_ENDPOINT` unset** → a no-op provider. Instrumented code
  runs unchanged and spans are discarded. This is the default everywhere.
- **`OTEL_EXPORTER_OTLP_ENDPOINT` set** → an OTLP **gRPC** exporter (batched) to
  that endpoint, plus a JSONL file exporter when `ENGRAM_SESSION_ID` is set
  (`~/.engram/traces/<id>/spans.jsonl`, see `jsonl_exporter.go`).

## Quick start (no Docker)

```sh
# 1. Build + launch a local Jaeger collector (fetches the pinned release once).
make otel-up                       # or: otel-local up --fetch

# 2. In the shell where you run an instrumented binary, point it at the collector.
eval "$(otel-local env)"           # export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# 3. Run something instrumented; spans flow to Jaeger.
wayfinder session complete-phase PROBLEM --outcome success
safe-merge --pr 42 --dry-run

# 4. View traces.
open http://localhost:16686
```

`otel-local` (see `cmd/otel-local`) runs the native **Jaeger v2 all-in-one**
binary — OTLP gRPC on `:4317`, OTLP HTTP on `:4318`, UI/query on `:16686` — with
no Docker or VM. `otel-local up` runs it in the foreground (Ctrl-C to stop);
`--detach` backgrounds it and `otel-local down` stops it. `jaeger-health`
(`cmd/jaeger-health`) is a liveness/readiness probe for the same instance.

## Endpoint scheme matters

The endpoint may be given with or without a scheme — `localhost:4317`,
`http://localhost:4317`, and `https://tempo:4317` all work (`parseOTLPEndpoint`
splits it into a gRPC dial target + insecure flag). This guards a real footgun:
`otlptracegrpc`'s own env parser maps a scheme-less `localhost:4317` to an empty
target and then **silently exports nothing**. When in doubt, use the
scheme-qualified `http://localhost:4317` that `otel-local env` prints.

## What is instrumented

`InitTracer` is called in these binaries; each emits spans only when a collector
is configured:

| Binary           | Tracer name       | Key spans |
|------------------|-------------------|-----------|
| `agm`            | `agm`             | `agm.session.register` / `kill` / `state_set` / `list`, `session.lifecycle` |
| `safe-pr`        | `safe-pr`         | `safepr.<verb>` (create/close) |
| `safe-merge`     | `safe-merge`      | `safemerge.attempt`/`watch` → `safemerge.gate.ci`/`threads`/`soak`, `safemerge.merge` |
| `vroom-dispatch` | `vroom-dispatch`  | `vroom.ensure_sessions`, `vroom.session.spawn` → `create`/`boot`/`loop`, `vroom.session.recreate` |
| `wayfinder`      | `wayfinder`       | `wayfinder.phase.start`, `wayfinder.phase.complete` |
| `engram`         | `engram`          | (engram service spans) |

## Notes

- The repo standardizes on **gRPC** OTLP (`:4317`); do not add
  `otlptracehttp`/`otlpmetrichttp`.
- Metrics live separately in `internal/telemetry` (`InitMeter`); this package is
  traces only. Don't add a second tracer — reuse `InitTracer`.
- `otel-local --fetch` pins Jaeger `v2.19.0` and verifies the download against
  the release's published `*.sha256sum.txt` over TLS. That integrity check
  guards against corrupt/truncated downloads; it is not a substitute for
  verifying the release itself (no GPG `.asc` check is performed).
- The Jaeger binary is a checksum-verified **download**, not a `go install`
  target, so it must live outside `~/go/bin`. The launch agent
  (`make install-jaeger-launchagent`) runs it from the dear-agent cache, because
  a `GOBIN` sweep removes anything under `~/go/bin` and a download cannot be
  restored by rebuilding from source (`DECL-LAUNCHD-05`, bead ce-24f1).
- Every collector listener is loopback-only. `deploy/jaeger/SPEC.md` contracts
  the receiver, query, and storage bounds; `cmd/otel-local/config_contract_test.go`
  enforces them against the deployed `deploy/jaeger/config.yaml`.
- Both the plist and the collector config are registered in
  `deploy/manifest.yaml`, so `dear-deploy status` reports drift and
  `dear-deploy sync` is the only supported way to stage them. A restaged plist
  needs `launchctl bootout` then `bootstrap`; `kickstart` restarts only the job
  definition launchd already holds in memory.

## Verifying that spans are actually arriving

A collector can be up, reachable, and receiving nothing. Process liveness and a
port check both pass in that state, so neither is sufficient.

`cmd/jaeger-health` is the check that distinguishes them: it reports `degraded`
and exits `1` when Jaeger is alive but no traces appear in the lookback window
(`JAEGER-HEALTH-03`), and `down` with exit `2` when it is unreachable. Run it
directly, or schedule it through `absence-alarm`, which owns running absence
probes on an interval and routing their exit codes to a notifier.
