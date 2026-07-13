# W0 Charter Persistence Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**W0-01** When a charter is saved, the system shall create the W0 directory and atomically write canonical frontmatter plus content.

**W0-02** When an existing charter is replaced, the system shall preserve a backup before committing the new content.

**W0-03** When charter metadata is absent, the system shall apply documented created, status, and version defaults.

**W0-04** When a charter is read, the system shall parse supported frontmatter and return content and metadata separately.

**W0-05** When malformed or absent frontmatter is read, the system shall preserve charter content according to the compatibility contract.

**W0-06** When a charter is deleted, the system shall report whether the file existed and preserve filesystem errors.

**W0-07** While W0 is authored by any supported harness and model family, the system shall preserve identical format, backup, and atomic-write behavior.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/w0`
