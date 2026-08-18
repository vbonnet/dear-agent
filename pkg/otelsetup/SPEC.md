# OpenTelemetry Setup Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/otelsetup` bootstraps OpenTelemetry tracing for dear-agent binaries. It
keeps tracing opt-in, normalizes OTLP endpoint handling, and can mirror spans
to local Engram JSONL files when a session ID is present.

## Requirements

**OTELSETUP-01** When `OTEL_EXPORTER_OTLP_ENDPOINT` is unset, the system shall install a no-op tracer provider and return a no-op shutdown function.

**OTELSETUP-02** When an OTLP endpoint is configured without a scheme, the system shall treat it as a plaintext gRPC target rather than relying on environment parsing.

**OTELSETUP-03** When an OTLP endpoint starts with `http://`, the system shall strip the scheme and configure plaintext gRPC export.

**OTELSETUP-04** When an OTLP endpoint starts with `https://`, the system shall strip the scheme and configure TLS-backed gRPC export.

**OTELSETUP-05** When a service name is configured in `OTEL_SERVICE_NAME`, the system shall prefer it over the caller-provided service name.

**OTELSETUP-06** When no service name is configured or provided, the system shall derive the service name from the current binary name.

**OTELSETUP-07** When build metadata includes a VCS revision, the system shall use the first seven characters as the service version or the full revision when the revision is shorter than seven characters.

**OTELSETUP-08** When OTLP exporter creation fails, the system shall fall back to a no-op tracer provider instead of failing the host binary.

**OTELSETUP-09** When `ENGRAM_SESSION_ID` is set and JSONL exporter creation succeeds, the system shall register a JSONL span exporter in addition to OTLP export.

**OTELSETUP-10** When JSONL spans are exported, the system shall write append-only records under `~/.engram/traces/<session-id>/spans.jsonl` with trace IDs, span IDs, parent IDs, service name, span metadata, attributes, events, status, and duration.

**OTELSETUP-11** When JSONL export shuts down, the system shall flush and close the file and make repeated shutdowns harmless.

**OTELSETUP-12** When the configured collector is unreachable or unresponsive, the system shall bound each OTLP export — the initial attempt and every retry together — so shutdown returns within the export budget even when the caller supplies an unbounded context.

**OTELSETUP-13** When an export fails with a retryable error, the system shall retry within the export budget rather than dropping the batch on first failure.

## BDD Traceability

- `agm/test/bdd/features/observability_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
