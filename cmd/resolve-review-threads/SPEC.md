# resolve-review-threads Command Specification

<!-- Last audited at: 2026-08-20 -->

**Version:** 1.1
**Status:** Baseline
**Scope:** `cmd/resolve-review-threads`.

## Overview

`resolve-review-threads` is the sanctioned helper for listing, resolving, and
reopening GitHub pull-request review threads. It exists because GitHub review
thread resolution is GraphQL-only and required conversation resolution blocks
safe merges until unresolved threads are handled explicitly.

## EARS Requirements

**RESOLVE-REVIEW-THREADS-01** When no subcommand or an unknown subcommand is provided, the system shall print usage and exit with a usage failure.

**RESOLVE-REVIEW-THREADS-02** When listing review threads, the system shall require owner, repository, and integer pull-request number arguments.

**RESOLVE-REVIEW-THREADS-03** When listing review threads, the system shall page through GitHub GraphQL reviewThreads results until no next page remains.

**RESOLVE-REVIEW-THREADS-04** When `list` is requested, the system shall emit only unresolved threads as compact JSON lines.

**RESOLVE-REVIEW-THREADS-05** When `list-all` is requested, the system shall emit resolved and unresolved threads as compact JSON lines.

**RESOLVE-REVIEW-THREADS-06** When resolving or unresolving one thread, the system shall require a review-thread GraphQL node ID and call the matching GraphQL mutation.

**RESOLVE-REVIEW-THREADS-07** When `resolve-all` is requested with an author filter, the system shall resolve only unresolved threads whose first comment author matches that filter.

**RESOLVE-REVIEW-THREADS-08** When comment bodies are printed, the system shall collapse whitespace and truncate previews on rune boundaries.

**RESOLVE-REVIEW-THREADS-09** When GitHub CLI reports a GraphQL error, the system shall include GitHub CLI diagnostics in the returned error.

**RESOLVE-REVIEW-THREADS-10** When flattening a review thread, the system shall mark it answered if and only if it holds more than one comment and its last comment author differs from its first comment author.

**RESOLVE-REVIEW-THREADS-11** When `resolve-all` encounters an unanswered thread and `--force` is absent, the system shall refuse to resolve that thread, report it by node ID and path with the login holding it open, and exit non-zero.

**RESOLVE-REVIEW-THREADS-12** When `resolve-all` is given `--force`, the system shall resolve unanswered threads.

**RESOLVE-REVIEW-THREADS-13** When a thread is outdated, the system shall report that fact and shall not treat it as evidence the thread may be resolved.

**RESOLVE-REVIEW-THREADS-14** When `reply-resolve` is requested, the system shall require a thread node ID and a non-empty body, post the reply, and resolve the thread only after the reply succeeds.

## BDD Traceability

- Feature: `agm/test/bdd/features/workflow_tooling_guardrails.feature`

## Test Traceability

- Unit package: `cmd/resolve-review-threads`
