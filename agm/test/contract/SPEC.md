# AGM Live Harness Contract Specification

<!-- Last audited at: 2026-07-21 -->

## Requirements

**CTEST-01** When the contract suite enumerates active harnesses, the suite shall require `claude-code`, `codex-cli`, `agy`, `opencode-cli`, and `pi-cli` in canonical parity order.

**CTEST-02** The active contract suite shall not treat the deprecated `gemini-cli` compatibility adapter as an active parity harness.

**CTEST-03** When an active harness is resolved through the factory, the returned adapter shall identify itself with the same canonical harness name.

**CTEST-04** If a live credential, binary, server, or quota is unavailable, then the contract suite shall skip with an explicit reason rather than consume another provider's credentials.

**CTEST-05** When a live harness contract invokes AGM, the suite shall use canonical `session` and `send msg` commands with canonical harness identifiers rather than retired root commands or agent aliases.

**CTEST-06** When the Pi contract runs, the suite shall execute the installed Pi binary and validate the managed authorization extension while skipping with an explicit installation reason when Pi is unavailable.

**CTEST-07** When the installed Pi binary loads dear-agent project resources, the suite shall exit its non-interactive model-list probe without entering the reserved legacy `hooks/` migration prompt.

**CTEST-08** When provider credentials are absent, the contract-tagged suite shall still compile and execute the credential-free active-harness registry contract.

**CTEST-09** When a test only configures a provider mock server without invoking production adapter behavior, the system shall not report it as adapter contract coverage.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Contract tests: `agm/test/contract/*_test.go`
