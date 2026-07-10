# Session Association Repair Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/fix` diagnoses and repairs missing or broken conversation UUID
associations using history detection and AGM storage.

## Requirements

**FIX-01** When scanning unassociated sessions, the system shall produce suggestions from available history evidence without mutating storage.

**FIX-02** When an association is accepted, the system shall validate the session and persist the selected UUID through the storage adapter.

**FIX-03** When an association is cleared, the system shall remove the stored UUID without deleting the AGM session.

**FIX-04** When scanning broken associations, the system shall report UUIDs that no longer resolve to valid conversation evidence.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/fix/*_test.go`
