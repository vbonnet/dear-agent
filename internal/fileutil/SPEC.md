# Atomic File Write Specification

<!-- Last audited at: 2026-07-09 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/fileutil`.

## Overview

`internal/fileutil` provides the shared crash-resistant replacement path for
state, configuration, and plan files across dear-agent components.

## EARS Requirements

**FILEUTIL-01** When an atomic write targets a missing parent directory, the system shall create the parent with private permissions.

**FILEUTIL-02** When data is written atomically, the system shall create the temporary file in the destination directory.

**FILEUTIL-03** When temporary data is complete, the system shall synchronize it before renaming it over the destination.

**FILEUTIL-04** When an atomic replacement succeeds, the system shall expose the complete new content with the requested file permissions.

**FILEUTIL-05** When an atomic write fails before replacement, the system shall remove its temporary file and preserve the existing destination.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/fileutil`
