# Stack Integrity Rules Specification

<!-- Last audited at: 2026-09-02 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `internal/stackguard`.

## Overview

`internal/stackguard` decides whether a pull request that presents itself as
part of a stack is actually wired as one. It supplies the rules for
`tools/stack-lint`, which is invoked by
`.github/workflows/stack-integrity.yml`.

The rules exist because a falsely labelled stack loses data. In a real stack
each pull request's base is the previous one's head, so merging the tip
cascades: GitHub merges the whole chain in order, atomically, and every pull
request keeps its own commits, description and review history. When the
descriptions say "stack" but every base targets the trunk, merging the tip
lands it alone and strands the siblings, which must then be closed, destroying
the descriptions and review threads they carried.

The package is pure. Callers supply the pull request, the open head and base
branches, and an ancestry oracle, and receive findings. No rule reads prose
where a structural fact is available, and GitHub's own stack registration
overrides prose wherever the two disagree.

## EARS Requirements

**STACKGUARD-01** When a description states a positional or present-tense stack claim and the base ref is the trunk, the system shall report a blocking finding naming the orphaning consequence.

**STACKGUARD-02** When a description states only an affiliation with a stack and the base ref is the trunk, the system shall report an advisory finding, because a drained stack is indistinguishable from one that was never wired.

**STACKGUARD-03** When a branch has an open pull request targeting it, the system shall treat it as a stack bottom and report no base-ref finding.

**STACKGUARD-04** When a registered stack records the pull request below the top position, the base ref is the trunk, and an entry below it is still open, the system shall report a blocking finding.

**STACKGUARD-05** When every registered entry below the pull request has merged, the system shall report no base-ref finding.

**STACKGUARD-06** When a stack is claimed and the base ref is neither the trunk nor the head of an open pull request, the system shall report a blocking finding.

**STACKGUARD-07** When the base ref is the head of an open pull request and the head does not descend from it, the system shall report a blocking stale-link finding.

**STACKGUARD-08** When the ancestry oracle fails, the system shall report a blocking finding rather than assume the link is sound.

**STACKGUARD-09** When a description declares a base branch other than the base ref, the system shall report a blocking finding.

**STACKGUARD-10** When the base ref is the head of an open pull request and the description carries no stack marker, the system shall report an advisory finding.

**STACKGUARD-11** When a pull request presents as stacked, belongs to no registered stack, and no blocking finding was produced, the system shall report an advisory finding.

**STACKGUARD-12** When a registered position or size contradicts the declared position or size, the system shall report a blocking finding.

**STACKGUARD-13** When strict evaluation is requested, the system shall report every advisory finding as blocking.

**STACKGUARD-14** When the registration could not be read, the system shall suppress the registration findings and still evaluate every structural rule.

**STACKGUARD-15** When a description shows a marker inside fenced code, an inline code span, a table row, a blockquote or double quotes, the system shall not read it as a claim, because a pull request that documents stacks is not claiming to be in one.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`

## Test Traceability

- Unit package: `internal/stackguard`
- Command package: `tools/stack-lint`
