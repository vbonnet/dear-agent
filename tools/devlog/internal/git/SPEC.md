# Devlog Git Adapter Requirements Specification (EARS)

<!-- Last audited at: 2026-07-10 -->

**Version**: 1.0
**Status**: Active
**Scope**: Local bare-repository and worktree operations behind the devlog Git contract.

## EARS Requirements

**DEVLOG-GIT-01** When a local repository adapter is created, the system shall retain its configured filesystem path.

**DEVLOG-GIT-02** When repository existence is checked, the system shall recognize only a valid bare Git repository structure.

**DEVLOG-GIT-03** When porcelain worktree output is parsed, the system shall preserve path, branch, head, and detached state for each entry.

**DEVLOG-GIT-04** When a repository operation targets a missing repository or worktree, the system shall return an error instead of reporting success.

**DEVLOG-GIT-05** When Git behavior is consumed by workspace logic, the system shall expose it through the provider-neutral `Repository` interface.

## Test Traceability

- Package tests: `tools/devlog/internal/git/local_test.go`
- BDD: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
