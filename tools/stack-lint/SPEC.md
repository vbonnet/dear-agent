# Stack Lint Command Specification

<!-- Last audited at: 2026-09-02 -->

**Version:** 1.0
**Status:** Baseline
**Scope:** `tools/stack-lint`.

## Overview

`stack-lint` refutes a pull request that presents itself as part of a stack but
is not wired as one. It is the command wrapper over `internal/stackguard` and
is invoked by `.github/workflows/stack-integrity.yml`.

The gate exists because a falsely labelled stack loses data. In a real stack
each base is the previous head, so merging the tip cascades and every pull
request keeps its own commits, description and review history. When the
descriptions say "stack" but every base targets the trunk, merging the tip lands
it alone and strands the siblings, which must then be closed, destroying the
descriptions and review threads they carried.

The command reads the pull request, the repository's open head branches and the
registered GitHub stack from `gh`, decides ancestry from the checkout's
remote-tracking refs, and applies the rules in `internal/stackguard`. Structural
findings block; the two hygiene findings are advisory unless `-strict` is given.

Registration is read through the `stack` and `stackEntry` preview fields. When
that query fails the command retries without them and suppresses the
registration rules, so an unavailable preview surface never turns into a false
"unregistered" verdict.

## EARS Requirements

**STACKLINT-01** When no pull request number is supplied, the command shall report the missing flag and exit with the usage status.

**STACKLINT-02** When the pull request cannot be read from GitHub, the command shall report the failure and exit with the usage status.

**STACKLINT-03** When the registered-stack fields cannot be queried, the command shall retry without them and mark the pull request registration-unread.

**STACKLINT-04** When a base or head branch is absent from the checkout, the command shall treat the stack link as undecidable and fail closed.

**STACKLINT-09** When open pull requests are listed, the command shall record both their head branches and their base branches, so a genuine stack bottom is distinguishable from a lone claim on the trunk.

**STACKLINT-10** When a registered stack is read, the command shall record whether any entry below this pull request is still open, so a drained stack's tip is not reported as unstacked.

**STACKLINT-05** When any finding is blocking, the command shall exit with the violation status.

**STACKLINT-06** When every finding is advisory, the command shall print each finding and exit successfully.

**STACKLINT-07** When no finding is produced, the command shall report the pull request as consistent and exit successfully.

**STACKLINT-08** When JSON output is requested, the command shall emit the pull request number, the blocking verdict, and a findings array that is present even when empty.

## Rules

| Code | Meaning | Default |
| --- | --- | --- |
| STACK-01 | A stack claim the base ref cannot support, so a tip merge would orphan the siblings | blocking, advisory for an affiliation claim |
| STACK-02 | A claimed parent branch with no open pull request | blocking |
| STACK-03 | A base ref that is no longer an ancestor of the head | blocking |
| STACK-04 | A body naming a base other than the one targeted | blocking |
| STACK-05 | A chain whose description never says it is stacked | advisory |
| STACK-06 | A chain GitHub has not been told about | advisory |
| STACK-07 | A body whose position or size contradicts the registration | blocking |
| STACK-08 | Ancestry that could not be decided | blocking |

STACK-01 separates three kinds of claim. A **positional** claim ("Stack 2/5")
and a **present-tense dependency** claim ("Stacked on #1379") assert where the
pull request sits right now, so a trunk base refutes them and blocks. An
**affiliation** claim ("part of the queue-privacy stack", "4/4 of the #1133
stack") reads identically whether the stack has drained or was never wired, so
it is advisory only. GitHub's own registration overrides all prose: a pull
request recorded mid-stack whose base is the trunk blocks regardless of wording,
but only while an entry below it is still open. Once every lower entry has
merged the stack has drained, nothing is left to orphan, and the trunk base is
correct (#1218 is the fixture for that).

A branch that has an open pull request targeting it is a genuine stack bottom.
Targeting the trunk is correct there, so STACK-01 never fires on it.

STACK-06 is suppressed whenever a blocking finding already fired: telling an
author to register a stack that is not yet a stack buries the finding that
needs acting on.

## BDD Traceability

- Feature: `agm/test/bdd/features/root_lifecycle_command_guardrails.feature`

## Test Traceability

- Unit package: `internal/stackguard`
- Command package: `tools/stack-lint`
