# resolve-review-threads Command Specification

<!-- Last audited at: 2026-08-20 -->

**Version:** 2.6
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

**RESOLVE-REVIEW-THREADS-22** When `reply-resolve` posts a reply but a transient failure (the resolution itself, or the placement re-read that precedes it) prevents resolution, the system shall state that the reply is posted and direct the user to retry `reply-resolve`, which finds the posted reply by its text and resolves without reposting it.

**RESOLVE-REVIEW-THREADS-23** When comparing an existing comment against a requested reply, the system shall compare the bodies without whitespace collapsing or truncation, ignoring only surrounding whitespace.

**RESOLVE-REVIEW-THREADS-24** When `reply-resolve` cannot read the thread's current state, the system shall post nothing, leave the thread unchanged, and exit non-zero.

**RESOLVE-REVIEW-THREADS-25** When `reply-resolve` posts a reply and the subsequent resolution is refused because the reviewer commented again, the system shall report that the reviewer now holds the thread and direct the user to answer the follow-up rather than to the resolution-only command.

**RESOLVE-REVIEW-THREADS-26** When a thread was already resolved by another actor before the pre-mutation re-read, the system shall report it as skipped and shall not count it among the threads it resolved.

**RESOLVE-REVIEW-THREADS-27** When GitHub refuses a resolution for access reasons, the system shall report it as an access problem rather than prescribing an immediate retry of the same mutation.

**RESOLVE-REVIEW-THREADS-28** When `reply-resolve` posts a reply, the system shall verify that the reply directly follows the comment it read before resolving, and shall leave the thread unresolved when another comment intervened.

**RESOLVE-REVIEW-THREADS-29** When `reply-resolve` finds its requested reply already present but followed by later comments, the system shall post nothing, leave the thread unresolved, and direct the user to answer those comments.

**RESOLVE-REVIEW-THREADS-30** When `reply-resolve` finds the thread already resolved, the system shall report it as skipped and post no reply.

**RESOLVE-REVIEW-THREADS-31** When `reply-resolve` resolves a thread, the system shall first confirm that a specific named comment, either the reply it just posted or a matching reply already present, is the thread's last comment.

**RESOLVE-REVIEW-THREADS-32** When the reply mutation returns no comment ID, the system shall treat it as an error and shall not resolve the thread.

**RESOLVE-REVIEW-THREADS-33** When determining whether a reply is already present, the system shall page through the thread's entire comment history rather than a bounded window.

**RESOLVE-REVIEW-THREADS-34** When `resolve-all` evaluates a candidate thread, the system shall decide refusal from a fresh per-thread read rather than from the listing snapshot.

**RESOLVE-REVIEW-THREADS-35** When `-h`, `--help`, or `help` is requested, the system shall print usage and exit zero.

**RESOLVE-REVIEW-THREADS-36** When comment pagination reports a further page with an empty or unchanged cursor, the system shall abort with an error rather than request the same page again.

**RESOLVE-REVIEW-THREADS-37** When resolving on behalf of a reply, the system shall verify at the pre-mutation read that the named reply is still the last comment.

**RESOLVE-REVIEW-THREADS-38** When the thread becomes resolved while its history is being paged, the system shall report it as skipped and post no reply.

**RESOLVE-REVIEW-THREADS-39** When `reply-resolve`'s resolution is refused for access reasons, the system shall state that an immediate retry will be denied too and direct the user to fix credentials before finishing with `reply-resolve`, not an unguarded resolution-only command.

**RESOLVE-REVIEW-THREADS-40** When a reply mutation succeeds but its response omits the new comment's ID, the system shall leave the thread unresolved, state that the reply may already be live, and direct the user to re-run the same command rather than reword and repost.

**RESOLVE-REVIEW-THREADS-41** When resolving a thread without `--force`, the system shall verify the comment its evidence read was based on against the resolution mutation's own response — using the caller-named anchor when one was given, otherwise the last comment observed by the pre-mutation evidence check — and, on a mismatch, reopen the thread rather than leave it resolved on stale evidence.

**RESOLVE-REVIEW-THREADS-42** When a resolution mutation succeeds but its response omits the thread's last comment, the system shall treat that as unverifiable and reopen the thread, the same as an actual mismatch, rather than treat a missing anchor as confirmed.

**RESOLVE-REVIEW-THREADS-43** When a stale-evidence resolution is detected and the automatic reopen also fails, the system shall report the thread as still resolved and direct the user to run `unresolve` before any other action, distinctly from the advice given when the reviewer has simply commented again.

**RESOLVE-REVIEW-THREADS-44** When advising a retry that depends on posting the identical reply body, the system shall include that exact body, POSIX-shell-quoted so a literal copy-paste reproduces it unchanged, rather than a generic placeholder or a representation (such as a Go string literal) that a shell would not round-trip.

**RESOLVE-REVIEW-THREADS-45** When a resolution mutation reports an error, the system shall re-read the thread before treating that as a clean no-op, since the client can fail after GitHub already applied the mutation server-side; if the re-read confirms it resolved, the system shall apply the same anchor verification and reopen-on-mismatch as a normally-reported success.

**RESOLVE-REVIEW-THREADS-46** When a reply mutation reports an error, the system shall re-read the thread's history before reporting the reply as failed, since the client can fail after GitHub already applied the mutation server-side; if a comment matching the attempted body is found, the system shall use it as the resolution anchor rather than invite a reworded, duplicating retry.

**RESOLVE-REVIEW-THREADS-47** When an automatic reopen is attempted, the system shall verify the mutation's own resolved-state postcondition rather than treat a nil error as proof the thread reopened, since another actor can race the same thread; on an access-denial failure, the system shall say so distinctly, since retrying the same reopen with the same credentials repeats the denial.

**RESOLVE-REVIEW-THREADS-48** When a resolution's named anchor is no longer the thread's last comment AND the thread is already resolved, the system shall reopen it before reporting the evidence refusal, so a later retry does not silently no-op against a resolved thread whose intervening comment was never read.

## BDD Traceability

- Feature: `agm/test/bdd/features/workflow_tooling_guardrails.feature`

## Test Traceability

- Unit package: `cmd/resolve-review-threads`
