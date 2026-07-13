# Devlog Configuration Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Discovery, validation, and deterministic merging of workspace configuration.

## EARS Requirements

**DEVLOG-CONFIG-01** When configuration is loaded, the system shall reject missing, oversized, unreadable, or malformed YAML files.

**DEVLOG-CONFIG-02** When configuration is validated, the system shall require a workspace name and reject duplicate repository or worktree names.

**DEVLOG-CONFIG-03** When repository URLs are validated, the system shall accept supported Git URL forms and reject unsupported schemes.

**DEVLOG-CONFIG-04** When worktree paths contain traversal components, the system shall reject the configuration.

**DEVLOG-CONFIG-05** When local configuration exists, the system shall merge local metadata and additions over the base without discarding unrelated base entries.

**DEVLOG-CONFIG-06** When configuration discovery starts below a workspace, the system shall search parent directories for the workspace configuration.

## Test Traceability

- Package tests: `tools/devlog/internal/config/*_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
