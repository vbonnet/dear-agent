# Wayfinder Root Command Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Canonical `wayfinder` command registration and project discovery.

## EARS Requirements

**WFC-ROOT-01** When the root command renders help, the system shall describe the canonical nine-phase Wayfinder V2 model.

**WFC-ROOT-02** When the root command registers lifecycle operations, the system shall expose them through the `session` command.

**WFC-ROOT-03** When the root command is built, the system shall not register retired V1 `start`, `autopilot`, `features`, or `abort` executors.

**WFC-ROOT-04** When a project directory flag is supplied, the system shall use that directory for session operations.

**WFC-ROOT-05** When no project directory flag is supplied, the system shall use the current working directory or a safe dot fallback.

**WFC-ROOT-06** When one project exists beneath `wf`, the system shall discover that project for session operations.

**WFC-ROOT-07** When zero or multiple projects exist beneath `wf`, the system shall require an explicit project directory.

**WFC-ROOT-08** When session commands are registered, the system shall include start, status, next-phase, start-phase, complete-phase, end, tasks, sandbox, rewind, lifecycle-state, coordination, and explicit migration operations.

## Test Traceability

- Package tests: `wayfinder/cmd/wayfinder/cmd/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
