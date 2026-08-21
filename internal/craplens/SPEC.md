# CRAP Lens Library Specification

<!-- Last audited at: 2026-08-20 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/craplens`.

## Overview

`craplens` scores the functions a pull request changed by joining cyclomatic
complexity with test coverage, using the Crap4j formula
`complexity^2 * (1 - coverage)^3 + complexity`. It exists to report the one
thing the repository's existing deterministic gates cannot see: complexity that
no test exercises.

It owns that join and nothing else. Discarded error returns belong to
`errcheck` and raw complexity belongs to `gocyclo`, both already enabled in
`.golangci.yml` with `new-from-merge-base`, so both are already diff-scoped and
already hard-gating. Packages that ship no test files belong to the `zero-test`
scan in `cmd/structural-health`. This library deliberately duplicates none of
them.

The library is advisory by construction. It reports and never blocks, and it
distinguishes coverage it could not measure from coverage it measured as zero,
because several packages in this module require a live tmux socket, a Dolt
server, or a container runtime.

## EARS Requirements

**CRAPLENS-01** When a base or head revision is absent, the system shall reject the request rather than analyze an empty diff.

**CRAPLENS-02** When the changed set is computed, the system shall consider only non-test Go source, excluding `_test.go`, vendored, generated, and testdata files.

**CRAPLENS-03** When function declarations are resolved, the system shall read source from the head revision rather than the working tree, so head-side diff line numbers cannot be attributed to functions in a different revision.

**CRAPLENS-04** When a changed function is identified, the system shall require its declaration span to overlap a head-side line the diff wrote.

**CRAPLENS-05** When cyclomatic complexity is computed, the system shall count one for the function entry plus one for each branch point, matching the counting the repository's configured `gocyclo` linter performs.

**CRAPLENS-06** When function coverage is derived, the system shall intersect coverage-profile blocks with the function's declaration span, and shall report a function containing no counted statements as fully covered.

**CRAPLENS-07** When a touched package's coverage cannot be collected, the system shall report that package as unknown and shall exclude its functions from scoring rather than score them as untested.

**CRAPLENS-08** When the working tree is not at the head revision, the system shall skip coverage collection entirely, report the mismatch, and flag nothing.

**CRAPLENS-09** When a touched package is measured at zero coverage, the system shall report it, and shall distinguish a package whose every changed file is newly added from an existing package that was edited.

**CRAPLENS-10** When changed functions are scored, the system shall report individually those above the configured threshold, ordered worst first, and shall report the proportion of scored functions at or under the agent-written target.

**CRAPLENS-11** When a report contains no functions above the threshold and no zero-coverage package, the system shall render nothing.

**CRAPLENS-12** When a report is rendered, the system shall bound each list, disclose any truncation, state that the signal cannot fail a check, and name the gates that own discarded error returns and raw complexity.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`

## Test Traceability

- Unit package: `internal/craplens`
- Command wrapper: `tools/crap-lint`
- Consumer: `.github/workflows/pr-size-scope.yml`
