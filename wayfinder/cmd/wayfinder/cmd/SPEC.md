# Wayfinder root command requirements specification

<!-- Last audited at: 2026-07-17 -->

**Status:** Active
**Scope:** Root command registration and project-directory resolution.

## EARS requirements

**WFROOT-01** When root help is rendered, the system shall describe the canonical nine-phase model.

**WFROOT-02** When lifecycle operations are registered, the system shall expose them only through the `session` command.

**WFROOT-03** When the root command is built, the system shall omit retired direct executors and compatibility commands.

**WFROOT-04** When `--directory` is supplied, the system shall resolve session files relative to that directory.

**WFROOT-05** When `--directory` is omitted, the system shall use the current working directory or a safe dot fallback.

**WFROOT-06** When exactly one project exists beneath `wf`, the system shall discover it for session operations.

**WFROOT-07** When zero or multiple projects exist beneath `wf`, the system shall require an explicit project directory.

**WFROOT-08** When session commands are registered, the system shall include lifecycle, task, sandbox, rewind, and coordination operations implemented by the session package.

## Traceability

- Package tests: `wayfinder/cmd/wayfinder/cmd/*_test.go`
- BDD: `agm/test/bdd/features/wayfinder_v2_command_guardrails.feature`
