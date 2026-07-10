# Skill Lint Package Specification

<!-- Last audited at: 2026-07-08 -->

## Overview

`pkg/skilllint` validates model and effort frontmatter in Claude Code skill
markdown files. It prevents unpinned skills from silently inheriting expensive
session defaults and keeps skill cost governance machine-checkable.

## EARS Requirements

**SKILLLINT-PKG-01** When a skill file cannot be read, the system shall return an I/O error.

**SKILLLINT-PKG-02** When a skill file lacks leading YAML frontmatter, the system shall return a violation for missing frontmatter.

**SKILLLINT-PKG-03** When a frontmatter block is unterminated, the system shall return a violation for the unterminated block.

**SKILLLINT-PKG-04** When frontmatter YAML cannot be parsed, the system shall return a violation describing the parse failure.

**SKILLLINT-PKG-05** When frontmatter omits `model`, the system shall return a missing-model violation.

**SKILLLINT-PKG-06** When frontmatter model is not `haiku`, `sonnet`, or `opus`, the system shall return an invalid-model violation.

**SKILLLINT-PKG-07** When frontmatter omits `effort`, the system shall return a missing-effort violation.

**SKILLLINT-PKG-08** When frontmatter effort is not `low`, `medium`, or `high`, the system shall return an invalid-effort violation.

**SKILLLINT-PKG-09** When checking a directory, the system shall lint markdown files directly under `commands` directories.

**SKILLLINT-PKG-10** When checking a directory, the system shall lint `SKILL.md` files inside `skills/<name>` directories.

**SKILLLINT-PKG-11** When checking a directory, the system shall ignore README-style markdown files, co-located `SPEC.md` contracts, and test files.

## BDD Traceability

- `agm/test/bdd/features/plugin_skill_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
