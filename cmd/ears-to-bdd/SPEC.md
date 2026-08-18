# EARS to BDD Command Specification

<!-- Last audited at: 2026-07-17 -->

## Overview

`cmd/ears-to-bdd` converts EARS requirements from `SPEC.md` files into Gherkin
feature stubs. It is the bridge from validated requirements to executable BDD
coverage.

## Requirements

**EARS-BDD-01** When no path arguments are provided, the system shall search the current directory for `SPEC.md` files.

**EARS-BDD-02** When a directory path is provided, the system shall discover `SPEC.md` files through the shared repository inventory so Git-ignored, VCS, nested-worktree, dependency, generated-output, and test-fixture paths are excluded consistently.

**EARS-BDD-03** When an input path cannot be read, the system shall return a command failure.

**EARS-BDD-04** When stdout mode is used, the system shall print generated feature content for each SPEC.

**EARS-BDD-05** When an output directory is provided, the system shall write generated feature files named from the SPEC parent directory.

**EARS-BDD-06** When dry-run mode is requested, the system shall report planned writes without creating feature files.

**EARS-BDD-07** When a SPEC has no extractable EARS requirements, the system shall skip generation for that SPEC and continue processing.

**EARS-BDD-08** When feature files are written, the system shall create parent directories and write owner-only files.

## BDD Traceability

- `agm/test/bdd/features/quality_command_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
