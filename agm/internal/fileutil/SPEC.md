# Atomic File Utility Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/fileutil` provides crash-resistant atomic file replacement.

## Requirements

**AFU-01** When atomically writing a file, the system shall create missing parent directories and write through a temporary file in the destination directory.

**AFU-02** When replacing an existing file, the system shall rename the complete temporary file over the destination without exposing partial contents.

**AFU-03** When a write succeeds, the system shall apply the requested permissions and remove temporary artifacts.

**AFU-04** When any write stage fails, the system shall return the failure and clean up the temporary file.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/fileutil/*_test.go`
