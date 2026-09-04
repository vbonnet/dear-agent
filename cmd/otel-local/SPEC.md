# otel-local Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/otel-local` runs a local Jaeger v2 all-in-one trace collector for
dear-agent development without requiring Docker. It resolves a local Jaeger
binary, optionally downloads a pinned release, verifies the checksum, launches
the collector, and prints the environment variable required by instrumented
binaries.

## Requirements

**OTEL-LOCAL-01** When no subcommand is provided, the command shall behave as `up`.

**OTEL-LOCAL-02** When `env` is requested, the command shall print `export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317`.

**OTEL-LOCAL-03** When `up` is requested, the command shall resolve Jaeger using `$JAEGER_BINARY`, the dear-agent cache path, and then `jaeger` on `PATH`.

**OTEL-LOCAL-04** When `up` cannot find a Jaeger binary and `--fetch` is not set, the command shall return an install hint for the current platform.

**OTEL-LOCAL-05** When `up --fetch` is used, the command shall download the pinned Jaeger release asset and the platform SHA-256 manifest named `jaeger-<version>-<goos>-<goarch>.sha256sum.txt`, extract the `jaeger` binary, verify the extracted binary against its manifest entry, install it only after that check passes, and mark it executable.

**OTEL-LOCAL-12** When the extracted binary fails checksum verification, the command shall not leave a binary at the cache path it resolves and launches.

**OTEL-LOCAL-06** When a collector is already alive on the Jaeger UI port, the command shall reuse it and print the OTLP environment export.

**OTEL-LOCAL-07** When `up --detach` succeeds, the command shall write a pidfile and return after printing the stop command.

**OTEL-LOCAL-08** When foreground `up` receives interrupt or terminate signals, the command shall signal Jaeger, wait briefly for exit, and kill the process if it does not stop.

**OTEL-LOCAL-09** When `down` is requested, the command shall read the pidfile, send `SIGTERM`, remove the pidfile, and report the signaled process ID.

**OTEL-LOCAL-10** When Jaeger health checks do not become healthy before timeout, the command shall kill the launched process and return an error.

**OTEL-LOCAL-11** When unsupported commands or flags are provided, the command shall return usage-oriented errors without starting a collector.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
