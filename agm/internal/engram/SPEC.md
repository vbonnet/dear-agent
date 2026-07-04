# AGM Engram Integration Specification

<!-- Last audited at: 2026-07-04 -->

## Purpose

`agm/internal/engram` adapts shared Engram retrieval into AGM prompt/context
injection. The package is part of harness and model-family parity because
retrieved context, scores, tags, hashes, limits, and formatting must remain
harness-neutral before being delivered to Claude Code, Codex CLI, AGY, OpenCode,
or future model-family-backed surfaces.

## EARS Requirements

**ENGRAM-01** When AGM creates an Engram client, the system shall use the in-process retrieval service rather than requiring an external Engram binary.

**ENGRAM-02** When Engram configuration is loaded, the system shall use defaults for result limit, score threshold, and query timeout unless valid environment overrides are present.

**ENGRAM-03** When an Engram environment override is invalid, the system shall keep the default value and log a warning.

**ENGRAM-04** When AGM queries Engram, the system shall apply the configured timeout, query text, tags, result limit, and Engram path to the retrieval request.

**ENGRAM-05** When Engram retrieval returns results, the system shall preserve path, title, score, tags, content, and a `sha256:` content hash in the harness-neutral result model.

**ENGRAM-06** When Engram results are below the configured score threshold, the system shall omit those results from the returned context.

**ENGRAM-07** When Engram results are formatted for injection, the system shall emit a bounded system message with per-result identifiers, scores, tags, title, and content.

**ENGRAM-08** When no Engram results are available, the system shall return an empty injection message.

**ENGRAM-09** When formatted Engram content exceeds the maximum content length, the system shall truncate the content and mark it as truncated.

## BDD Traceability

- `agm/test/bdd/features/engram_parity.feature`

## Package Test Traceability

- `agm/internal/engram/client_test.go`
- `agm/internal/engram/config_test.go`
- `agm/internal/engram/formatter_test.go`
- `agm/internal/engram/standalone_test.go`
