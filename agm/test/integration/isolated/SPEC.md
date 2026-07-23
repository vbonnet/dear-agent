# AGM Isolated Lifecycle Integration Specification

<!-- Last audited at: 2026-07-21 -->

## Requirements

**IISO-01** When required integration evidence runs in CI, the suite shall execute the isolated source-built Codex lifecycle without host credentials or an installed AGM.

**IISO-02** When the isolated Codex lifecycle completes, the suite shall verify exact tmux, filesystem, and SQLite cleanup without reading or mutating host session state.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Isolated lifecycle tests: `agm/test/integration/isolated/*_test.go`
