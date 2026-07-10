# Wayfinder Lint Context Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Deterministic summary of repository lint configuration for agent context.

## EARS Requirements

**WAYFINDER-LINTCTX-01** When no recognized lint configuration exists, the system shall return an empty summary without failing.

**WAYFINDER-LINTCTX-02** When the project directory does not exist, the system shall return a filesystem error.

**WAYFINDER-LINTCTX-03** When golangci-lint configuration exists, the system shall summarize enabled, disabled, and relevant configured linters.

**WAYFINDER-LINTCTX-04** When Python lint configuration exists, the system shall summarize Ruff and Pyright settings.

**WAYFINDER-LINTCTX-05** When ESLint JSON or flat configuration exists, the system shall report the discovered configuration form.

**WAYFINDER-LINTCTX-06** When a summary is formatted, the system shall produce stable sections without invoking a model provider.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder-session/internal/lintcontext/lintcontext_test.go`
- BDD: `agm/test/bdd/features/wayfinder_internal_package_guardrails.feature`
