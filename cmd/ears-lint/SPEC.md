# EARS Lint Command Specification

<!-- Last audited at: 2026-07-17 -->

## Overview

`cmd/ears-lint` validates EARS requirements in `SPEC.md` files. It is the
strict quality gate that keeps SPEC requirements machine-checkable before they
are linked to BDD coverage.

## Requirements

**EARS-LINT-01** When no path arguments are provided, the system shall lint `SPEC.md` in the current directory.

**EARS-LINT-02** When a directory path is provided, the system shall discover `SPEC.md` files through the shared repository inventory so Git-ignored, VCS, nested-worktree, dependency, generated-output, and test-fixture paths are excluded consistently.

**EARS-LINT-03** When an EARS config path is provided, the system shall load that config before constructing the linter.

**EARS-LINT-04** When JSON output is requested, the system shall emit indented machine-readable lint results.

**EARS-LINT-05** When strict mode is requested, the system shall fail if any linted file has a non-conforming requirement.

**EARS-LINT-06** When no `SPEC.md` files are found, the system shall return a usage or input error.

**EARS-LINT-07** When all linted files pass, the system shall exit with code 0.

**EARS-LINT-08** When one or more linted files fail, the system shall exit with code 1.

**EARS-LINT-09** If an explicit EARS YAML configuration is missing or malformed, then the command shall return an explicit configuration diagnostic without constructing the linter.

## BDD Traceability

- `agm/test/bdd/features/quality_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
