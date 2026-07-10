# Garbage Collection Log Specification

<!-- Last audited at: 2026-07-09 -->

## Overview

`agm/internal/gclog` records append-only JSONL evidence for session garbage
collection and estimates archived directory sizes.

## Requirements

**GCL-01** When a GC logger is created, the system shall create the parent directory and open the configured append-only log.

**GCL-02** When a GC entry is logged, the system shall assign a timestamp when absent and append one valid JSON object without replacing prior evidence.

**GCL-03** When a dry-run operation is recorded, the system shall preserve its dry-run state in the log entry.

**GCL-04** When directory size is requested, the system shall sum accessible file sizes and return zero for a missing directory.

## BDD Traceability

- Feature: `agm/test/bdd/features/agm_diagnostics_package_guardrails.feature`
- Package tests: `agm/internal/gclog/*_test.go`
