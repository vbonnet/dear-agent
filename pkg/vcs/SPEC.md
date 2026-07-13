# Engram Version Control Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**VCS-01** When version control is enabled, the system shall initialize or reuse the configured repository and remote without destroying history.

**VCS-02** When an Engram memory file changes, the system shall stage its supported companion file and create a commit with a default or supplied message.

**VCS-03** When a tracked memory file is deleted, the system shall stage the deletion and commit it.

**VCS-04** When memory pairing or frontmatter validation fails, the system shall return structured pre-commit findings.

**VCS-05** When push strategy is immediate, asynchronous, batched, or manual, the system shall follow that strategy without force-pushing through the normal path.

**VCS-06** When history, diff, status, or restoration is requested, the system shall scope git operations to the configured repository.

**VCS-07** When version control is disabled, the system shall make facade operations safe no-ops where documented.

**VCS-08** While changes originate from any supported harness and model family, the system shall apply identical pairing, commit, validation, and push policy.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/vcs`
