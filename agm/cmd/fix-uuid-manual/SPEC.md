# Manual UUID Repair Command Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/cmd/fix-uuid-manual` is an operator command for explicitly repairing a
session's saved Claude conversation UUID in Dolt.

## Requirements

**FUM-01** When the command is invoked without the required session and UUID arguments, the system shall print usage and exit without mutating storage.

**FUM-02** When the target session does not exist, the system shall report the lookup failure and avoid creating a replacement session.

**FUM-03** When valid arguments identify a session, the system shall update only that manifest's Claude UUID and persist it through the Dolt adapter.

**FUM-04** When database initialization or update fails, the system shall return a non-zero command result with the failure context.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_product_surface_guardrails.feature`
- Package tests: `agm/cmd/fix-uuid-manual/*_test.go`
