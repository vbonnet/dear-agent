# resolve-review-threads Command Specification

<!-- Last audited at: 2026-08-20 -->

**Version:** 1.8
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

**RESOLVE-REVIEW-THREADS-15** When `resolve` is requested for a single unanswered thread and `--force` is absent, the system shall refuse to resolve it and exit non-zero.

**RESOLVE-REVIEW-THREADS-16** When determining whether a thread is answered, the system shall derive the most recent comment author from the thread's last comment rather than from a bounded page of comments.

**RESOLVE-REVIEW-THREADS-17** When about to resolve a thread, the system shall re-read that thread and re-evaluate its answered state immediately before issuing the mutation.

**RESOLVE-REVIEW-THREADS-18** When a thread is already resolved, the system shall report it as skipped rather than issuing a redundant mutation.

**RESOLVE-REVIEW-THREADS-19** When either the opening or the most recent comment author login is unavailable, the system shall not mark the thread answered.

**RESOLVE-REVIEW-THREADS-20** When `resolve-all` encounters an error that is not an evidence refusal, the system shall abort the sweep and report how many threads were resolved and refused before it stopped.

**RESOLVE-REVIEW-THREADS-21** When `reply-resolve` finds its requested reply is already the thread's most recent comment, the system shall skip posting and proceed to resolution.

**RESOLVE-REVIEW-THREADS-22** When `reply-resolve` posts a reply but cannot resolve the thread, the system shall state that the reply is posted, warn against re-running the command, and name the resolution-only command to finish.

**RESOLVE-REVIEW-THREADS-23** When comparing an existing comment against a requested reply, the system shall compare the bodies without whitespace collapsing or truncation, ignoring only surrounding whitespace.

**RESOLVE-REVIEW-THREADS-24** When `reply-resolve` cannot read the thread's current state, the system shall post nothing, leave the thread unchanged, and exit non-zero.

**RESOLVE-REVIEW-THREADS-25** When `reply-resolve` posts a reply and the subsequent resolution is refused because the reviewer commented again, the system shall report that the reviewer now holds the thread and direct the user to answer the follow-up rather than to the resolution-only command.

**RESOLVE-REVIEW-THREADS-26** When a thread was already resolved by another actor before the pre-mutation re-read, the system shall report it as skipped and shall not count it among the threads it resolved.

**RESOLVE-REVIEW-THREADS-27** When GitHub refuses a resolution for access reasons, the system shall report it as an access problem rather than prescribing an immediate retry of the same mutation.

**RESOLVE-REVIEW-THREADS-28** When `reply-resolve` posts a reply, the system shall verify that the reply directly follows the comment it read before resolving, and shall leave the thread unresolved when another comment intervened.

## BDD Traceability

- Feature: `agm/test/bdd/features/workflow_tooling_guardrails.feature`

## Test Traceability

- Unit package: `cmd/resolve-review-threads`
