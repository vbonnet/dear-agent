# Atomic File Write Specification

<!-- Last audited at: 2026-08-24 -->

**Version:** 1.1
**Status:** Baseline
**Scope:** `internal/fileutil`.

## Overview

`internal/fileutil` provides the shared crash-resistant replacement path for
state, configuration, and plan files across dear-agent components.

## EARS Requirements

**FILEUTIL-01** When an atomic write targets a missing parent directory, the system shall create the parent with private permissions.

**FILEUTIL-02** When data is written atomically, the system shall create the temporary file in the destination directory.

**FILEUTIL-03** When temporary data is complete, the system shall apply the requested permissions and then synchronize the file data and metadata before renaming it over the destination.

**FILEUTIL-04** When a temporary file is renamed over the destination, the system shall open and synchronize the resolved physical parent directory before reporting success.

**FILEUTIL-05** When an atomic write fails before replacement, the system shall remove its temporary file, preserve the existing destination, and propagate both the primary and cleanup errors.

**FILEUTIL-06** When an atomic write fails after replacement, the system shall propagate the durability error without removing or rewriting the published destination.

**FILEUTIL-07** When an atomic replacement succeeds, the system shall expose the complete new content with the requested file permissions.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`

## Test Traceability

- Unit package: `internal/fileutil`
