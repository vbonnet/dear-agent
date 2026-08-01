# Structural Health Command Specification

<!-- Last audited at: 2026-08-01 -->

## Overview

`cmd/structural-health` runs ratcheted structural scans over the repository and
fails only on regressions relative to the checked-in baseline. It makes
long-term structural drift visible without blocking on accepted findings.

The baseline is the single authority for accepted finding keys. Its maintenance
path is a guarded state transition: ordinary updates may tighten the ratchet,
while an expansion requires explicit review provenance in append-only history.
Schema v1 remains readable for one-way migration; all writes use schema v2.

## Requirements

**STRUCT-HEALTH-01** When structural scans run, the system shall execute the canonical dead-package, file-size, zero-test, doc-path, goroutine-recover, and raw-mem-gate scans.

**STRUCT-HEALTH-09** When the raw-mem-gate scan encounters a shell script that reads a raw macOS free-page metric and does not reference `memory_pressure` or `pressure_level`, the system shall flag it as a finding.

**STRUCT-HEALTH-02** When scanning repository files, the system shall skip generated, vendored, build, dependency, worktree, and VCS directories.

**STRUCT-HEALTH-03** When baseline update mode is requested without expansion authorization and no current finding key is absent from the prior baseline, the system shall write only current finding keys already present in the prior baseline and shall exit successfully.

**STRUCT-HEALTH-04** When a current finding key is absent from the baseline, the system shall classify it as a regression.

**STRUCT-HEALTH-05** When a baseline finding key is absent from current findings, the system shall classify it as fixed.

**STRUCT-HEALTH-06** When JSON output is requested, the system shall emit a machine-readable report.

**STRUCT-HEALTH-07** When regressions are present, the system shall exit with code 1.

**STRUCT-HEALTH-08** When setup or usage fails, the system shall exit with code 2.

**STRUCT-HEALTH-10** When a baseline is read, the system shall reject malformed JSON, duplicate JSON object-member names and noncanonical case-folded aliases of schema member names at any nesting level before decoding, unsupported schema or scanner-key versions, incomplete canonical scan coverage, unknown scans, null lists, unsorted lists, empty keys, and duplicate keys.

**STRUCT-HEALTH-11** When a valid schema-v1 baseline is updated, the system shall preserve its finding-key semantics and shall write schema v2.

**STRUCT-HEALTH-12** When an update is rejected, the system shall leave the destination baseline byte-identical.

**STRUCT-HEALTH-13** When a prior key is replaced by a different current key, the system shall classify the transition as one addition and one removal regardless of total count.

**STRUCT-HEALTH-14** When the caller uses `--accept-new`, the system shall require baseline-update mode, at least one added key, a non-blank reason, and a non-blank durable reference.

**STRUCT-HEALTH-15** When a baseline transition is written, the system shall append the scanner-key version, the literal prior-file SHA-256 for a v1 migration or absent-file bootstrap and otherwise the SHA-256 of the deterministic canonical v2 predecessor, exact added keys, exact removed keys, and required admission metadata to transition history.

**STRUCT-HEALTH-16** When schema v2 is written, the system shall emit deterministic JSON with every canonical scan represented by a sorted non-null key list.

**STRUCT-HEALTH-17** When a schema-v2 update has no key change, the system shall leave the file byte-identical and shall not append transition history.

**STRUCT-HEALTH-18** When persistence fails before atomic rename, the system shall remove temporary output and shall preserve the prior destination bytes and mode.

**STRUCT-HEALTH-19** When admission provenance is supplied, the system shall require its reference to contain a tracker ID, pull-request reference, HTTPS URL, or commit identifier.

**STRUCT-HEALTH-20** When JSON output and baseline-update mode are requested together, the system shall reject the incompatible modes before scanning or writing.

**STRUCT-HEALTH-21** When ordinary scan mode reads a checked-in baseline that uses an older supported scanner-key version, the system shall reject it until baseline-update mode records an explicitly authorized scanner-version transition.

**STRUCT-HEALTH-22** When baseline update mode is requested without expansion authorization and any current finding key is absent from the prior baseline, the system shall reject the entire update without removing fixed keys or otherwise modifying the baseline.

**STRUCT-HEALTH-23** When a schema-v2 baseline with transition history is read, the system shall reject it if any reconstructable transition's `previous_baseline_sha256` differs from the SHA-256 of the predecessor baseline's deterministic canonical serialization.

## BDD Traceability

- `agm/test/bdd/features/quality_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.

## Test Traceability

- Baseline transition and persistence behavior:
  `cmd/structural-health/baseline_test.go`.
- Scan, diff, and report behavior: `cmd/structural-health/main_test.go`.
- Repository-level signal: `go run ./cmd/structural-health`.
