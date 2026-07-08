# Skill Lint Command Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`tools/skill-lint` is the command-line entry point for `pkg/skilllint`. It
checks one skill file or one or more directories and exits nonzero when skill
frontmatter violates model or effort policy.

## Requirements

**SKILL-LINT-CMD-01** When neither `-file` nor directory arguments are provided, the system shall print usage and exit with code 2.

**SKILL-LINT-CMD-02** When `-file <path>` is provided, the system shall lint only that file.

**SKILL-LINT-CMD-03** When directory arguments are provided, the system shall lint each directory recursively with `pkg/skilllint`.

**SKILL-LINT-CMD-04** When file or directory analysis returns an operational error, the system shall print the error to stderr and exit with code 2.

**SKILL-LINT-CMD-05** When violations are found, the system shall print each violation to stderr and exit with code 1.

**SKILL-LINT-CMD-06** When no violations are found, the system shall exit with code 0.

## BDD Traceability

- `agm/test/bdd/features/plugin_skill_package_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
