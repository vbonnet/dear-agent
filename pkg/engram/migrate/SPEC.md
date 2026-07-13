# Engram Format Migration Specification

<!-- Last audited at: 2026-07-09 -->

## EARS Requirements

**MIGRATE-01** When migration scans a directory, the system shall process only `.ai.md` files recursively and count skipped or failed files.

**MIGRATE-02** When content already contains complete tier markers, the system shall skip redundant migration.

**MIGRATE-03** When tier markers are inserted, the system shall preserve frontmatter and emit T0, T1, and T2 in canonical order.

**MIGRATE-04** When migration is not a dry run, the system shall preserve an existing rationale companion and create one only when absent.

**MIGRATE-05** When migration changes non-tier content beyond the integrity tolerance, the system shall report validation failure.

**MIGRATE-06** When dry-run mode is enabled, the system shall report planned changes without modifying files.

**MIGRATE-07** When no eligible files exist, the system shall return zeroed migration statistics without failure.

**MIGRATE-08** While migration is invoked by any supported harness and model family, the system shall produce identical tiered content.

## BDD Traceability

- Feature: `agm/test/bdd/features/evaluation_control_parity.feature`

## Test Traceability

- Unit package: `pkg/engram/migrate`
