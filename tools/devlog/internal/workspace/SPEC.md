# Devlog Workspace Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Loaded workspace state and deterministic repository path resolution.

## EARS Requirements

**DEVLOG-WORKSPACE-01** When a workspace is loaded, the system shall discover and merge its base and local configuration.

**DEVLOG-WORKSPACE-02** When no workspace configuration can be discovered, the system shall return a configuration error.

**DEVLOG-WORKSPACE-03** When a repository path is requested, the system shall resolve it relative to the configured workspace root.

**DEVLOG-WORKSPACE-04** When a worktree path is requested, the system shall resolve it from the owning repository and worktree configuration.

**DEVLOG-WORKSPACE-05** When loaded state is returned, the system shall retain the configuration directory needed for subsequent operations.

## Test Traceability

- Package tests: `tools/devlog/internal/workspace/workspace_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
