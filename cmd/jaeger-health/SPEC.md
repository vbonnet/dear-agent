# jaeger-health Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`cmd/jaeger-health` probes a Jaeger HTTP API for local trace collector health.
It distinguishes liveness from readiness by checking service discovery and
recent trace availability, returning stable exit codes for automation.

## Requirements

**JAEGER-HEALTH-01** When flag parsing fails, the command shall exit with code `3`.

**JAEGER-HEALTH-02** When Jaeger `/api/services` is unreachable, invalid, or times out, the command shall report `down` and exit with code `2`.

**JAEGER-HEALTH-03** When Jaeger is alive but no traces are found in the lookback window, the command shall report `degraded` and exit with code `1`.

**JAEGER-HEALTH-04** When trace queries are canceled before all services are checked, the command shall report `degraded`, include the partial trace count, and exit with code `1`.

**JAEGER-HEALTH-05** When Jaeger is alive and at least one trace exists in the lookback window, the command shall report `healthy` and exit with code `0`.

**JAEGER-HEALTH-06** When `--json` is set, the command shall emit an indented JSON report with checked time, Jaeger URL, status, liveness, services, trace count, lookback, and error when present.

**JAEGER-HEALTH-07** When `--json` is not set, the command shall emit a single human-readable status summary.

**JAEGER-HEALTH-08** When a per-service trace query fails for a non-cancellation reason, the command shall warn on stderr and continue checking other services.

**JAEGER-HEALTH-09** When querying traces, the command shall cap each service query at 100 traces and pass through the configured lookback value.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
