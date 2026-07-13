# Devlog Command Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Command-line lifecycle for declarative development workspaces.

## EARS Requirements

**DEVLOG-CMD-01** When the root command is constructed, the system shall expose shared verbosity, dry-run, and configuration controls.

**DEVLOG-CMD-02** When initialization runs in an uninitialized directory, the system shall create the workspace configuration and supporting files.

**DEVLOG-CMD-03** When initialization would overwrite existing configuration, the system shall require an explicit force request with a reason.

**DEVLOG-CMD-04** When synchronization runs in dry-run mode, the system shall describe intended repository and worktree changes without applying them.

**DEVLOG-CMD-05** When status runs with valid configuration, the system shall report repository and worktree state without mutating the workspace.

**DEVLOG-CMD-06** When a command cannot find workspace configuration, the system shall return a configuration error.

## Test Traceability

- Package tests: `tools/devlog/cmd/devlog/*_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
