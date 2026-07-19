# Skill Lint Command Specification

<!-- Last audited at: 2026-07-18 -->

## Overview

`tools/skill-lint` is the thin command adapter for `pkg/skilllint`. It checks a
tracked repository, one recognized file, or one or more directories and keeps
usage, content, and operational outcomes distinct.

## EARS Requirements

**SKILL-LINT-CMD-01** When no validation mode is selected, the system shall print usage and exit with code 2.

**SKILL-LINT-CMD-02** When more than one of repository, file, or directory mode is selected, the system shall print usage and exit with code 2.

**SKILL-LINT-CMD-03** When `-repo <root>` is provided, the system shall call `pkg/skilllint.CheckRepository` with the signal-aware command context for that root.

**SKILL-LINT-CMD-04** When `-file <path>` is provided, the system shall call `pkg/skilllint.CheckFile` only for that path.

**SKILL-LINT-CMD-05** When directory arguments are provided, the system shall call `pkg/skilllint.CheckDir` for each directory.

**SKILL-LINT-CMD-06** When validation returns an operational error, the system shall print the error to stderr and exit with code 2.

**SKILL-LINT-CMD-07** When validation returns content violations, the system shall print each violation to stderr and exit with code 1.

**SKILL-LINT-CMD-08** When validation returns no violations, the system shall exit with code 0.

## BDD Traceability

- `agm/test/bdd/features/plugin_skill_package_guardrails.feature` enforces that this command keeps co-located SPEC coverage.
