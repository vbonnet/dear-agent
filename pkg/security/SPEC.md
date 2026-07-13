# Filesystem Security Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**SECURITY-01** When a path is validated, the system shall resolve the allowed base and reject paths that escape its directory boundary.

**SECURITY-02** When symlink following is disabled, the system shall reject a user path that is itself a symbolic link.

**SECURITY-03** When diagram or output paths are validated, the system shall require the corresponding allowed extension.

**SECURITY-04** When a file exceeds the configured read limit, the system shall return the typed file-size error before reading unbounded content.

**SECURITY-05** When a directory is created safely, the system shall validate its resolved path and requested mode within the allowed base.

**SECURITY-06** When an allowed base is missing or not a directory, the system shall return contextual validation failure.

**SECURITY-07** While file operations originate from any supported harness and model family, the system shall apply identical traversal, symlink, size, and extension policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/security`
