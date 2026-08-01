# Versioned Install Canary Specification

<!-- Last audited at: 2026-07-31 -->

## EARS Requirements

**VERSIONED-INSTALL-01** When the root `go.mod` contains a `replace` or `exclude` directive, the canary shall fail before constructing the module proxy.

**VERSIONED-INSTALL-02** When the canary constructs the root module archive, it shall include tracked and non-ignored untracked regular files while excluding vendor content, symbolic links, and nested modules.

**VERSIONED-INSTALL-03** When the canary installs commands, the canary shall request the exact synthetic version from a local module proxy with workspace discovery disabled and a fresh module cache.

**VERSIONED-INSTALL-04** When validating documented command installation, the canary shall install `agm`, `engram`, and `wayfinder` from their root-module package paths.

**VERSIONED-INSTALL-05** When every documented command installs as an executable, the canary shall report success.

**VERSIONED-INSTALL-06** If proxy construction, dependency resolution, compilation, or executable validation fails, the canary shall exit non-zero with the failing operation in its diagnostic.

**VERSIONED-INSTALL-07** While executing external commands, the canary shall enforce a bounded deadline and bounded diagnostic capture while continuing to drain discarded output.

**VERSIONED-INSTALL-08** When reading the root module definition or packaging root-module files, the canary shall reject content beyond the Go module size limits before unbounded allocation.

## BDD Traceability

- Feature: `agm/test/bdd/features/developer_tool_package_guardrails.feature`
