# Deprecated Gemini CLI compatibility specification

<!-- Last audited at: 2026-07-17 -->

## Executable EARS requirements

**GCMP-01** When an existing `gemini-cli` manifest is loaded, the system shall route it to the CLI/tmux compatibility adapter.

**GCMP-02** When the Gemini compatibility adapter is called directly to launch or resume a session, the system shall invoke the local `gemini` CLI through tmux; the top-level AGM resume dispatcher does not restart this deprecated harness.

**GCMP-03** When active harness parity is evaluated, the system shall exclude the deprecated `gemini-cli` harness.

**GCMP-04** The system shall not require a Gemini API key or Google Gemini Go SDK for the `gemini-cli` compatibility path.

## BDD traceability

- Feature: `agm/test/bdd/features/legacy_spec_strictness_guardrails.feature`

## Source and tests

- Lifecycle registry: `agm/internal/agent/harnesses.go`
- Constructor registry: `agm/internal/agent/factory.go`
- Implementation: `agm/internal/agent/gemini_cli_adapter.go`
- Focused tests: `agm/internal/agent/gemini_cli_adapter_test.go`
