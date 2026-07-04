# SessionStart Chezmoi Drift Hook Specification

<!-- Last audited at: 2026-07-04 -->

## Overview

`sessionstart-chezmoi-drift` warns when Claude settings drift from the chezmoi
template, without blocking session startup.

## EARS Requirements

**SCD-01** When the hook runs, the system shall bound drift detection with a three-second timeout.

**SCD-02** When the `chezmoi` binary is unavailable, the system shall skip drift detection silently.

**SCD-03** When chezmoi diff returns no settings drift, the system shall emit no warning.

**SCD-04** When chezmoi diff reports settings drift, the system shall count additions and deletions excluding diff headers.

**SCD-05** When settings drift is detected, the system shall warn on stderr and include review and sync commands.

**SCD-06** When any drift-detection path completes, the system shall not block session start.

## BDD Traceability

- Feature: `agm/test/bdd/features/hook_parity.feature`
- Package tests: `agm/hooks/cmd/sessionstart-chezmoi-drift/main_test.go`

