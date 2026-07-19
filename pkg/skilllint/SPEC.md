# Skill Lint Package Specification

<!-- Last audited at: 2026-07-18 -->

## Overview

`pkg/skilllint` owns discovery and deterministic validation of Dear Agent's
tracked `SKILL.md` and provider command Markdown. Skills have a harness-neutral
core; command prompts retain Claude model/effort and tool-declaration policy.
Content defects are `Violation` values, while Git and file failures are Go
errors.

## EARS Requirements

**SKILLLINT-PKG-01** When a surface file cannot be read, the system shall return an operational I/O error.

**SKILLLINT-PKG-02** When a recognized surface lacks leading YAML frontmatter, the system shall return a missing-frontmatter violation.

**SKILLLINT-PKG-03** When frontmatter is unterminated or invalid YAML, the system shall return a parse violation rather than an operational error.

**SKILLLINT-PKG-04** When a command prompt is validated, the system shall require nonempty `description` and `allowed-tools` fields.

**SKILLLINT-PKG-05** When a command prompt is validated, the system shall require `model` to be `haiku`, `sonnet`, or `opus` and `effort` to be `low`, `medium`, or `high`.

**SKILLLINT-PKG-06** When a skill is validated, the system shall require a kebab-case `name` of at most 64 characters and a nonempty description containing `Use when` or `Trigger when` case-insensitively.

**SKILLLINT-PKG-07** When a skill body lacks both a workflow heading and two ordered steps, the system shall return a missing-workflow violation.

**SKILLLINT-PKG-08** When a skill body lacks a verification or completion heading, the system shall return a missing-verification violation.

**SKILLLINT-PKG-09** When a skill exceeds 100 lines without a References, Documentation, or Resources section, the system shall return a progressive-disclosure violation.

**SKILLLINT-PKG-10** When a skill exceeds 500 lines, the system shall return a review-threshold violation.

**SKILLLINT-PKG-11** When a skill declares optional `model` or `effort` metadata, the system shall require both fields and validate their values against the command tiers.

**SKILLLINT-PKG-12** When a skill declares a provider execution extension, the system shall require explicit actionable fallback through a harness-neutral route and shall reject fallback to another invocation of the same provider tool.

**SKILLLINT-PKG-13** When a surface declares an unsupported frontmatter key, the system shall return a surface-specific unsupported-field violation.

**SKILLLINT-PKG-14** When checking a repository, the system shall use caller-cancelable, timeout-bounded, non-interactive Git tracked inventory to discover every `SKILL.md` and direct `commands/*.md` prompt while excluding README and SPEC contracts.

**SKILLLINT-PKG-15** When checking a repository, the system shall ignore untracked files and return repository-relative violation paths.

**SKILLLINT-PKG-16** When two tracked regular-file skills are byte-identical, the system shall report the later path and the first tracked owner; a tracked symlink to a canonical skill shall remain a validated discovery alias rather than count as a copy.

**SKILLLINT-PKG-17** When checking one file or a directory, the system shall route recognized surfaces through the same validators used by repository checks.

## BDD Traceability

- `agm/test/bdd/features/plugin_skill_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
