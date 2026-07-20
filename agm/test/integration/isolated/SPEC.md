# AGM Isolated Lifecycle Integration Specification

<!-- Last audited at: 2026-07-19 -->

## Requirements

**IISO-01** When scheduled integration runs disable legacy end-to-end scenarios, the suite shall still execute the isolated source-built Codex lifecycle.

**IISO-02** When the isolated Codex lifecycle completes, the suite shall verify exact tmux, filesystem, and SQLite cleanup without reading or mutating host session state.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Isolated lifecycle tests: `agm/test/integration/isolated/*_test.go`
