# AGM Live Harness Contract Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**CTEST-01** When the contract suite enumerates active harnesses, the suite shall require `claude-code`, `codex-cli`, `agy`, and `opencode-cli` in canonical parity order.

**CTEST-02** The active contract suite shall not treat the deprecated `gemini-cli` compatibility adapter as an active parity harness.

**CTEST-03** When an active harness is resolved through the factory, the returned adapter shall identify itself with the same canonical harness name.

**CTEST-04** If a live credential, binary, server, or quota is unavailable, then the contract suite shall skip with an explicit reason rather than consume another provider's credentials.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Contract tests: `agm/test/contract/*_test.go`
