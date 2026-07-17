# Structural Health Command Specification

<!-- Last audited at: 2026-07-15 -->

## Overview

`cmd/structural-health` runs ratcheted structural scans over the repository and
fails only on regressions relative to the checked-in baseline. It makes
long-term structural drift visible without blocking on pre-existing findings.

## Requirements

**STRUCT-HEALTH-01** When structural scans run, the system shall execute the canonical dead-package, file-size, zero-test, doc-path, goroutine-recover, and raw-mem-gate scans.

**STRUCT-HEALTH-09** When the raw-mem-gate scan encounters a shell script that reads a raw macOS free-page metric and does not reference `memory_pressure` or `pressure_level`, the system shall flag it as a finding.

**STRUCT-HEALTH-02** When scanning repository files, the system shall skip generated, vendored, build, dependency, worktree, and VCS directories.

**STRUCT-HEALTH-03** When baseline update mode is requested, the system shall write the current finding keys to the baseline and exit successfully.

**STRUCT-HEALTH-04** When a current finding key is absent from the baseline, the system shall classify it as a regression.

**STRUCT-HEALTH-05** When a baseline finding key is absent from current findings, the system shall classify it as fixed.

**STRUCT-HEALTH-06** When JSON output is requested, the system shall emit a machine-readable report.

**STRUCT-HEALTH-07** When regressions are present, the system shall exit with code 1.

**STRUCT-HEALTH-08** When setup or usage fails, the system shall exit with code 2.

## BDD Traceability

- `agm/test/bdd/features/quality_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
