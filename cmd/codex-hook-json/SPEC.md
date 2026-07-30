# Codex Hook JSON Helper Specification

<!-- Last audited at: 2026-07-29 -->

## Requirements

**CHJSON-01** When a trusted Codex hook requests an approved input field, the system shall decode one JSON document and return only the supported scalar or combined edit/patch text.

**CHJSON-02** When a trusted Codex hook constructs an approved response, the system shall emit the exact supported PreToolUse or Stop response structure using JSON encoding rather than shell interpolation.

**CHJSON-03** When a filter, argument shape, input document, or trailing JSON value is unsupported, the system shall fail without evaluating a general-purpose expression language.

## Verification

- Unit tests: `cmd/codex-hook-json/main_test.go`
- Deployment contract: `internal/deploy/codex_hook_json_install_test.go`
