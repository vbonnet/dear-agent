# AGM Lifecycle Integration Specification

<!-- Last audited at: 2026-07-10 -->

## Requirements

**LIFEI-01** When lifecycle scenarios create, resume, archive, kill, or list sessions, the suite shall verify manifest, storage, and tmux state remain consistent.

**LIFEI-02** When hooks execute around lifecycle transitions, the suite shall verify ordering, environment propagation, timeout handling, and failure behavior.

**LIFEI-03** When multiple lifecycle tests run concurrently, the suite shall prove cleanup leaves no test sessions or processes behind.

**LIFEI-04** If a session fixture is missing, corrupt, or already archived, then the suite shall verify a deterministic error outcome without damaging other sessions.

**LIFEI-05** When the integration test binary or a child AGM command opens Dolt storage, the suite shall select explicit test mode, test workspace, and test database values before adapter construction.

**LIFEI-06** When Codex lifecycle behavior is exercised through the real CLI, the suite shall use a compiled fake Codex process inside a fully isolated source-built AGM environment, shall exercise create, list, send, kill, resume, and archive through that environment, and shall verify tmux and SQLite postconditions before exact cleanup.

**LIFEI-07** When a required process-table or tmux prerequisite cannot start, the isolated Codex lifecycle shall skip only for a missing executable or an explicit permission denial and shall fail for every other setup error.

## BDD Traceability

- Feature: `agm/test/bdd/features/test_support_package_guardrails.feature`
- Lifecycle tests: `agm/test/integration/lifecycle/*_test.go`
