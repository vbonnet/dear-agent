# Retrospective Guard Linter Specification

<!-- Last audited at: 2026-09-05 -->
<!-- Audit scope: pkg/retrolint (domain) + cmd/retro-lint (CLI), both
     present at this revision. cmd/retro-lint/SPEC.owner delegates here.

     Domain (pkg/retrolint/*_test.go):
       RLINT-01 through RLINT-07, RLINT-12.

     CLI (cmd/retro-lint/main_test.go):
       RLINT-08 through RLINT-11. -->

**Status:** Production-ready
**Scope:** Machine-enforced deterministic guard verification for DEAR retrospectives

## Purpose

`cmd/retro-lint` and `pkg/retrolint` implement the M3 mechanism of the
absence-blindness architecture (ce-23lf7, ce-8v9d3): a deterministic linter
and absence-alarm probe that prevents incident retrospectives from landing
as prose-only recommendations without machine-verifiable guards.

Historically, incident-class retrospectives frequently documented systemic
lessons that recurred 2 to 4 times because recommendations remained in prose
rather than shipping automated verification artifacts (such as test paths,
launchd services, CI workflows, or lint rules).

The retrospective guard linter solves this by parsing retrospectives for a
structured `Guards:` block, verifying that declared artifacts exist in the
target codebase, enforcing bead tracking for deferred items, and maintaining
a grandfathered baseline with ratchet enforcement.

## Applicability

The linter evaluates retrospective markdown documents in `engram-research`
or standalone repositories, functioning both as a CI gate on changed files
and as an absence-alarm probe.

## EARS Requirements

**RLINT-01** When a retrospective file is evaluated, the system shall require at least one declared guard or deferred guard.

**RLINT-02** When a guard specifies a repository file path, the system shall verify that the path exists in the target repository.

**RLINT-03** When a guard specifies a launchd label, the system shall verify that the launchd label is non-empty.

**RLINT-04** When a guard specifies a deferred action, the system shall require both a non-empty bead identifier and a non-empty rationale.

**RLINT-05** If a retrospective path is present in the grandfathered baseline store, then the system shall waive missing guard requirements for that file.

**RLINT-06** While ratchet mode is enabled, the system shall reject baseline entries that declare valid guards or reference removed files.

**RLINT-07** When evaluated under absence-alarm mode, the system shall classify retrospectives added within the lookback window lacking valid guards as ABSENT.

**RLINT-08** When all evaluated retrospectives satisfy guard requirements, the CLI shall exit 0.

**RLINT-09** When at least one evaluated retrospective fails guard requirements or names missing artifacts, the CLI shall exit 1.

**RLINT-10** When configuration loading fails, invalid flags are supplied, or target directories cannot be read, the CLI shall exit 2 with a usage error.

**RLINT-11** When JSON mode is enabled, the CLI shall output structured results on stdout.

**RLINT-12** When a retrospective is evaluated, the system shall bound execution with a timeout so file reads cannot hang indefinitely.

## BDD Traceability

- Feature: `agm/test/bdd/features/audit_package_guardrails.feature`
- Package tests: `pkg/retrolint/retrolint_test.go` (RLINT-01..RLINT-07, RLINT-12)
- CLI tests: `cmd/retro-lint/main_test.go` (RLINT-08..RLINT-11)
