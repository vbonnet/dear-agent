# Skill Lint Package Specification

<!-- Last audited at: 2026-07-18 -->

## Overview

`pkg/skilllint` owns discovery and deterministic validation of Dear Agent's
tracked `SKILL.md` and provider command Markdown. Each skill name has one
regular-file content owner; repository-local discovery aliases are contained
symlinks to that tracked canonical `SKILL.md`, never separately authored
copies. Skills have a harness-neutral procedural core; provider loader or
execution extensions require an actionable harness-neutral fallback. Command
prompts retain Claude model/effort and tool-declaration policy.
Optional `content-hash` metadata is accepted as opaque input; this package does
not attest it or use it to distinguish separately authored skill owners.
Content defects are `Violation` values, while Git and file failures are Go
errors.

## EARS Requirements

**SKILLLINT-PKG-01** When a surface file cannot be read, the system shall return an operational I/O error.

**SKILLLINT-PKG-02** When a recognized surface lacks leading YAML frontmatter, the system shall return a missing-frontmatter violation.

**SKILLLINT-PKG-03** When frontmatter is unterminated or invalid YAML, the system shall return a parse violation rather than an operational error.

**SKILLLINT-PKG-04** When a command prompt is validated, the system shall require nonempty `description` and `allowed-tools` fields.

**SKILLLINT-PKG-05** When a command prompt is validated, the system shall require `model` to be `haiku`, `sonnet`, or `opus` and `effort` to be `low`, `medium`, or `high`.

**SKILLLINT-PKG-06** When a skill is validated, the system shall require a kebab-case `name` of at most 64 characters and a nonempty description.

**SKILLLINT-PKG-07** When a skill body lacks both a nonempty procedural workflow section and two ordered steps, the system shall return a missing-workflow violation.

**SKILLLINT-PKG-08** When a skill body lacks a nonempty verification or completion section, the system shall return a missing-verification violation.

**SKILLLINT-PKG-09** When a skill exceeds 100 lines without a References, Documentation, or Resources section, the system shall return a progressive-disclosure violation.

**SKILLLINT-PKG-10** When a skill exceeds 500 lines, the system shall return a review-threshold violation.

**SKILLLINT-PKG-11** When a skill declares optional `model` or `effort` metadata, the system shall require both fields and validate their values against the command tiers.

**SKILLLINT-PKG-12** When a skill declares a provider loader or execution extension, the system shall require explicit actionable fallback through a harness-neutral route and shall reject fallback to another invocation of the same provider tool.

**SKILLLINT-PKG-13** When a surface declares an unsupported frontmatter key, the system shall return a surface-specific unsupported-field violation.

**SKILLLINT-PKG-14** When checking a repository, the system shall use caller-cancelable, timeout-bounded, non-interactive Git tracked inventory to discover every `SKILL.md` and direct `commands/*.md` prompt while excluding README and SPEC contracts.

**SKILLLINT-PKG-15** When checking a repository, the system shall ignore untracked files and return repository-relative violation paths.

**SKILLLINT-PKG-16** When more than one tracked regular-file skill declares the same skill name, the system shall report the later path and first tracked owner whether their normalized content is equivalent or divergent.

**SKILLLINT-PKG-17** When checking one file or a directory, the system shall route recognized surfaces through the same validators used by repository checks.

**SKILLLINT-PKG-18** When a tracked skill or command symlink resolves outside the validation root, the system shall report a containment violation without reading the external target.

**SKILLLINT-PKG-19** When a contained tracked skill symlink is checked, the system shall allow it as a discovery alias only when its resolved target is the one tracked regular-file canonical `SKILL.md` owner for that skill name; an untracked target, a tracked non-`SKILL.md` target, or a different owner shall produce an alias-target violation.

## BDD Traceability

- `agm/test/bdd/features/plugin_skill_package_guardrails.feature` enforces that this package keeps co-located SPEC coverage.
