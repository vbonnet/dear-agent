# Agentic Review Gate Command Specification

<!-- Last audited at: 2026-09-03 -->

## Overview

`cmd/agentic-review-gate` adapts GitHub to the per-family review policy in
`internal/agenticreview` and publishes the verdict as the `agentic-review/gate`
commit status that the `main` ruleset requires.

The command exists to keep the merge-blocking path cheap. The reviewers are the
expensive, quota-limited, occasionally-down part of the system, so nothing that
blocks a merge is allowed to depend on one of them answering a second time. This
command reads three GitHub endpoints and a policy file, and calls no model.

## EARS Requirements

**AGC-01** When evaluating a live pull request, the command shall read only the pull request view, its issue timeline, and its head commit, and shall invoke no model.

**AGC-02** When the verdict permits a merge, the command shall exit zero; otherwise it shall exit non-zero.

**AGC-03** When the policy file is missing, unreadable, or invalid, the command shall exit with a usage error rather than evaluating against a built-in default.

**AGC-04** When neither an input file nor a repository and pull request are supplied, or when both are, the command shall exit with a usage error.

**AGC-05** When `--quorum` is supplied, the command shall evaluate against that threshold in place of the configured one, and shall report it in the verdict.

**AGC-06** When `--post-status` is requested without a repository slug and head SHA, the command shall exit with a usage error.

**AGC-07** When `--post-status` is requested, the command shall publish the `agentic-review/gate` context against the supplied head as `success` for a pass, `pending` for an unresolved lifecycle, and `failure` for a block.

**AGC-08** When the status publication fails, the command shall exit non-zero even for a passing verdict, because the ruleset reads the status rather than the exit code.

**AGC-09** When a status description exceeds the provider limit, the command shall truncate it rather than failing the publication.

**AGC-10** When emitting a text summary, the command shall report the decision, every configured family's state and reason, the approval count, the down count, and the quorum.

**AGC-11** When a family published labels but is not configured, the command shall report it as a warning rather than counting it toward the quorum.

## Test Traceability

- Package tests: `cmd/agentic-review-gate/main_test.go`
- Scenario fixtures: `cmd/agentic-review-gate/testdata/*.json`
- Workflow wiring: `tests/bats/agentic-review-gate.bats`

## BDD Consequence

No new BDD feature is required. The command's observable protocol is its exit
code and the published commit status, both driven end to end from recorded
fixtures and a replayed GitHub transcript in the package tests above.
