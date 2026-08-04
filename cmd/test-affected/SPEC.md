# Affected Tests Command Specification

<!-- Last audited at: 2026-07-31 -->

## Overview

`cmd/test-affected` selects Go test packages affected by a pull request. It
diffs changed files, maps them onto Go packages, walks dependency edges, and
prints or runs the affected test-bearing packages.

## Requirements

**TEST-AFFECTED-01** When no repo root is provided, the system shall resolve the root with `git rev-parse --show-toplevel`.

**TEST-AFFECTED-02** When the base or head ref is empty, the system shall return a usage error.

**TEST-AFFECTED-03** When force-full paths such as module, workflow, or selector files change, the system shall select all test-bearing packages.

**TEST-AFFECTED-04** When a Go package changes, the system shall select test-bearing packages that transitively depend on that package.

**TEST-AFFECTED-05** When a testdata file changes, the system shall map the change to the nearest owning package directory.

**TEST-AFFECTED-06** When no affected packages are selected, the system shall treat that as a clean pass.

**TEST-AFFECTED-07** When `--all` is provided, the system shall include affected non-test-bearing packages in output.

**TEST-AFFECTED-08** When `--run` is provided, the system shall execute `go test -race -count=1` for the selected packages, pass every test binary a native timeout that matches required CI and local preflight, bound package discovery and aggregate test execution, and when the test command's own bound expires report exit code 124.

## BDD Traceability

- `agm/test/bdd/features/quality_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
- `agm/test/bdd/features/local_development_guardrails.feature` enforces timeout parity across affected integration tests, required CI, and local preflight.
