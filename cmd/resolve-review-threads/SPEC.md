# resolve-review-threads Command Specification

<!-- Last audited at: 2026-09-08 -->

**Version:** 3.3
**Status:** Baseline
**Scope:** `cmd/resolve-review-threads` and repository-generated remediation guidance for it.

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

**RESOLVE-REVIEW-THREADS-09** When GitHub CLI reports a GraphQL error, the system shall include GitHub CLI diagnostics in the returned error unless the request variables include a reply body, in which case it shall suppress those diagnostics as required by RESOLVE-REVIEW-THREADS-53.

**RESOLVE-REVIEW-THREADS-10** When flattening a review thread, the system shall mark it answered if and only if it holds more than one comment and its last comment author differs from its first comment author.

**RESOLVE-REVIEW-THREADS-11** When `resolve-all` encounters an unanswered thread and `--force` is absent, the system shall refuse to resolve that thread, report it by node ID and path with the login holding it open, and exit non-zero.

**RESOLVE-REVIEW-THREADS-12** When `resolve-all` is given `--force`, the system shall resolve unanswered threads.

**RESOLVE-REVIEW-THREADS-13** When a thread is outdated, the system shall report that fact and shall not treat it as evidence the thread may be resolved.

**RESOLVE-REVIEW-THREADS-14** When `reply-resolve` is requested, the system shall require a thread node ID and an explicit body-file source, post the non-empty body from that source, and resolve the thread only after the reply succeeds.

**RESOLVE-REVIEW-THREADS-15** When `resolve` is requested for a single unanswered thread and `--force` is absent, the system shall refuse to resolve it and exit non-zero.

**RESOLVE-REVIEW-THREADS-16** When determining whether a thread is answered, the system shall derive the most recent comment author from the thread's last comment rather than from a bounded page of comments.

**RESOLVE-REVIEW-THREADS-17** When about to resolve a thread, the system shall re-read that thread and re-evaluate its answered state immediately before issuing the mutation.

**RESOLVE-REVIEW-THREADS-18** When a pre-mutation read finds a thread already resolved and every caller-named anchor and answer-evidence check required by that path succeeds, the system shall report it as skipped rather than issue a redundant resolution mutation.

**RESOLVE-REVIEW-THREADS-19** When either the opening or the most recent comment author login is unavailable, the system shall not mark the thread answered.

**RESOLVE-REVIEW-THREADS-20** When `resolve-all` encounters an error that is not an evidence refusal, the system shall abort the sweep and report how many threads were resolved and refused before it stopped.

**RESOLVE-REVIEW-THREADS-21** When `reply-resolve` finds its requested reply is already the thread's most recent comment, the system shall skip posting and proceed to resolution.

**RESOLVE-REVIEW-THREADS-22** When `reply-resolve` has confirmed both that its reply is posted and that it directly follows the originally observed predecessor, but a transient resolution failure prevents terminal resolution, the system shall retain the actual body source unchanged and direct the user to retry `reply-resolve` with that source, which finds the posted reply by its text and resolves without reposting it.

**RESOLVE-REVIEW-THREADS-23** When comparing an existing comment against a requested reply, the system shall compare the bodies without whitespace collapsing or truncation, ignoring only surrounding whitespace.

**RESOLVE-REVIEW-THREADS-24** When `reply-resolve` cannot read the thread's current state, the system shall post nothing, leave the thread unchanged, and exit non-zero.

**RESOLVE-REVIEW-THREADS-25** When `reply-resolve` posts a reply and a later non-empty comment anchor confirms that the thread changed before resolution, the system shall report that the reply no longer accounts for the thread and direct the user to read and answer the later comment rather than use the resolution-only command.

**RESOLVE-REVIEW-THREADS-26** When a thread was already resolved by another actor before the pre-mutation re-read and every caller-named anchor and answer-evidence check required by that path succeeds, the system shall report it as skipped and shall not count it among the threads it resolved.

**RESOLVE-REVIEW-THREADS-27** When GitHub refuses a resolution for access reasons, the system shall report it as an access problem rather than prescribing an immediate retry of the same mutation.

**RESOLVE-REVIEW-THREADS-28** When `reply-resolve` posts or recovers a reply, the system shall preserve the originally observed predecessor, verify that the reply directly follows that predecessor before resolving, and refuse resolution when another comment intervened.

**RESOLVE-REVIEW-THREADS-29** When `reply-resolve` finds its requested reply already present but followed by later comments, the system shall post nothing, leave the thread unresolved, and direct the user to answer those comments.

**RESOLVE-REVIEW-THREADS-30** When `reply-resolve`'s initial state read finds the thread already resolved before the command posts or adopts a reply, the system shall report it as skipped and post no reply.

**RESOLVE-REVIEW-THREADS-31** When `reply-resolve` resolves a thread, the system shall first confirm that a specific named comment, either the reply it just posted or a matching reply already present, is the thread's last comment.

**RESOLVE-REVIEW-THREADS-32** When the reply mutation returns no comment ID, the system shall not use that response as proof that the reply was posted and shall not resolve the thread unless a full-history re-read recovers the exact reply directly after the originally observed predecessor and verifies that recovered comment as the current tail.

**RESOLVE-REVIEW-THREADS-33** When determining whether a reply is already present, the system shall page through the thread's entire comment history rather than a bounded window.

**RESOLVE-REVIEW-THREADS-34** When `resolve-all` evaluates a candidate thread, the system shall decide refusal from a fresh per-thread read rather than from the listing snapshot.

**RESOLVE-REVIEW-THREADS-35** When `-h`, `--help`, or `help` is requested, the system shall print usage and exit zero.

**RESOLVE-REVIEW-THREADS-36** When comment pagination reports a further page with an empty or unchanged cursor, the system shall abort with an error rather than request the same page again.

**RESOLVE-REVIEW-THREADS-37** When resolving on behalf of a reply, the system shall verify at the pre-mutation read that the named reply is still the last comment.

**RESOLVE-REVIEW-THREADS-38** When the thread becomes resolved while its history is being paged and the complete history does not show a requested reply superseded by newer commentary, the system shall report it as skipped and post no reply; a superseded requested reply shall instead follow RESOLVE-REVIEW-THREADS-64.

**RESOLVE-REVIEW-THREADS-39** When `reply-resolve`'s resolution is refused for access reasons, the system shall state that an immediate retry will be denied too and direct the user to fix credentials before finishing with `reply-resolve` and the unchanged body-file source, not an unguarded resolution-only command.

**RESOLVE-REVIEW-THREADS-40** When a reply mutation succeeds but its response omits the new comment's ID, the system shall treat whether the reply is live as ambiguous, retain the original predecessor and actual body source, and permit resolution only after reconciliation under RESOLVE-REVIEW-THREADS-58 recovers the exact reply from full history, proves that it directly follows the original predecessor, and verifies its current-tail placement.

**RESOLVE-REVIEW-THREADS-41** When resolving a thread without `--force`, the system shall verify the comment its evidence read was based on against the resolution mutation's own response — using the caller-named anchor when one was given, otherwise the last comment observed by the pre-mutation evidence check — and classify an empty or changed response anchor under RESOLVE-REVIEW-THREADS-59 rather than leave the thread resolved on unverified evidence.

**RESOLVE-REVIEW-THREADS-42** When a resolution mutation succeeds but its response omits the thread's last comment, the system shall treat that outcome as unverifiable, reopen the thread, preserve the actual body source unchanged, and shall not treat the missing anchor as a confirmed later comment or superseded answer.

**RESOLVE-REVIEW-THREADS-43** When a stale or unverifiable resolution is detected and the automatic reopen also fails, the system shall report the thread as still resolved, direct the user to run `unresolve` before any other action, and require a fresh state inspection before selecting unchanged-retry or revised-answer guidance.

**RESOLVE-REVIEW-THREADS-44** When recovery depends on matching an identical reply body, the system shall direct the user to reuse the actual unchanged named path or standard-input form without rendering the reply body as command-line or shell-evaluable diagnostic text.

**RESOLVE-REVIEW-THREADS-45** When a resolution mutation reports an error, the system shall re-read the thread before treating that as a clean no-op, since the client can fail after GitHub already applied the mutation server-side; if the re-read confirms it resolved, the system shall apply the same anchor verification and reopen-on-mismatch as a normally-reported success.

**RESOLVE-REVIEW-THREADS-46** When a reply mutation reports an error, the system shall preserve the predecessor observed before posting and re-read the thread's history before reporting the reply as failed, since the client can fail after GitHub already applied the mutation server-side; recovery shall follow RESOLVE-REVIEW-THREADS-58 rather than discard that predecessor or invite a blind, duplicating retry.

**RESOLVE-REVIEW-THREADS-47** When an automatic reopen is attempted, the system shall verify the mutation's own resolved-state postcondition rather than treat a nil error as proof the thread reopened, since another actor can race the same thread; on an access-denial failure, the system shall say so distinctly, since retrying the same reopen with the same credentials repeats the denial.

**RESOLVE-REVIEW-THREADS-48** When a resolution's named anchor is no longer the thread's last comment AND the thread is already resolved, the system shall reopen it before reporting the evidence refusal, so a later retry does not silently no-op against a resolved thread whose intervening comment was never read.

**RESOLVE-REVIEW-THREADS-49** When a body-file source contains valid UTF-8 reply content within the declared size limits, the system shall preserve its complete byte sequence as the provider-visible reply body without trimming or normalization.

**RESOLVE-REVIEW-THREADS-50** When a body-file source is missing, duplicated, non-regular, unreadable, oversized, invalid UTF-8, empty, or whitespace-only, the system shall fail before issuing any provider mutation.

**RESOLVE-REVIEW-THREADS-51** When invoking an external process for a review-thread GraphQL operation, the system shall pass the query and variables as data input and shall not place any variable value in process arguments.

**RESOLVE-REVIEW-THREADS-52** The system shall not evaluate reply-body content as shell syntax or permit embedded command substitutions to execute.

**RESOLVE-REVIEW-THREADS-53** When the provider client fails an operation whose variables include a reply body, the system shall not retain, return, log, or print the client's raw standard error because debug output can echo the request body; only a redacted typed failure category may survive.

**RESOLVE-REVIEW-THREADS-54** When a named body-file source is selected, the system shall open it without blocking on a substituted non-regular object on Unix, validate the opened descriptor as a regular file before reading it, and direct streaming input to the explicit standard-input source.

**RESOLVE-REVIEW-THREADS-55** When any body source emits more than 262,144 bytes or valid UTF-8 contains more than 65,536 Unicode code points, the system shall stop after reading at most 262,145 bytes and fail before issuing any provider mutation.

**RESOLVE-REVIEW-THREADS-56** When repository-generated remediation guidance instructs an operator to create a named reply-body file, the system shall prescribe a fresh task-owned system-temporary path outside the repository for each thread, quote that path in review-thread command invocations, direct retention and unchanged reuse through every applicable exact-body retry, and direct removal only after terminal resolution is confirmed and before proceeding to another thread or merge.

**RESOLVE-REVIEW-THREADS-57** When later reviewer comments conclusively supersede the current reply body while its thread remains unresolved, repository-generated remediation guidance shall direct the operator to revise the same actual named path or retained standard-input source rather than create or assign a replacement path, retain the revised source unchanged through every applicable exact-body retry, and conditionally remove a task-owned temporary source only after terminal resolution is confirmed.

**RESOLVE-REVIEW-THREADS-58** When a reply mutation has an ambiguous outcome or omits the new comment ID, the system shall retain the originally observed predecessor and actual body source, re-read the full thread history, recover a matching reply and verify it against that predecessor when found, permit unchanged-source retry only when no match is found and the tail is confirmed unchanged and any classified access denial has first been repaired under RESOLVE-REVIEW-THREADS-70, require inspection and same-source revision when the tail is confirmed moved, and require inspection without blind retry when the state cannot be read.

**RESOLVE-REVIEW-THREADS-59** When the resolution response supplies an empty last-comment anchor, the system shall classify the outcome as unverifiable and retain the actual body source unchanged for inspection and retry after reopening; when it supplies a non-empty anchor different from the expected anchor, the system shall classify the thread as changed and require the later comment to be read and answered with the same source before resolution.

**RESOLVE-REVIEW-THREADS-60** When reply-placement verification finds the reply buried or jumped and the thread is already resolved, the system shall reopen the thread and verify the reopen postcondition before emitting revised-answer guidance; if reopening fails, the system shall report that the thread remains resolved and require `unresolve` before reply recovery.

**RESOLVE-REVIEW-THREADS-61** When a named reply remains last but answer evidence is insufficient because an author login is missing or the opening and latest authors are equal, or when recovery state cannot be read, the system shall retain the actual body source unchanged, report the precise uncertainty, and require thread inspection without claiming that a reviewer follow-up occurred or that the source should be revised; a nonempty later comment may establish positional supersession without establishing that comment's author.

**RESOLVE-REVIEW-THREADS-62** When unchanged-retry or revised-answer guidance references a reply body source, the system shall preserve the caller's actual source form by rendering a reusable named path with lossless shell quoting or the standard-input marker `-`, shall not synthesize a replacement path, and shall preserve task-owned cleanup identity until terminal resolution is confirmed.

**RESOLVE-REVIEW-THREADS-63** When a resolve or unresolve mutation response is evaluated, the system shall require a non-empty target thread identity equal to the requested node ID and a resolved-state value equal to the requested postcondition before claiming, printing, or counting that mutation as successful; otherwise, it shall treat the response as unverified and reconcile fresh provider state before making a state claim.

**RESOLVE-REVIEW-THREADS-64** When `reply-resolve` observes a full-history tail different from the originally observed predecessor or named reply, the system shall classify the moved tail and verify reply placement before applying an already-resolved shortcut, and when the moved thread is resolved without verified reply placement it shall reopen the thread before emitting recovery guidance.

**RESOLVE-REVIEW-THREADS-65** When predecessor, recovered-reply, current-tail, or mutation-anchor evidence contains an empty comment node ID, the system shall treat the evidence as incomplete and unverified, retain the actual body source, and require fresh inspection without claiming stable placement, successful resolution, or a conclusively superseding comment.

**RESOLVE-REVIEW-THREADS-66** When a resolve mutation reports a transport error, the system shall re-read the requested thread and classify the fresh result as confirmed success only when identity, requested resolved state, and expected comment anchor all match; as stale or unverifiable resolution requiring verified reopen when resolved state is present but anchor evidence is empty or changed; as an absent requested postcondition when the matching thread is currently unresolved; or as unknown state requiring inspection when the re-read fails or returns mismatched identity.

**RESOLVE-REVIEW-THREADS-67** When an unresolve mutation reports a transport error or returns an unverified response, the system shall reconcile the requested thread from a fresh read before making a state claim, count success only when matching identity and unresolved state are confirmed, report the requested postcondition as absent when matching identity remains resolved, and require inspection when the fresh state is unreadable or identifies another thread.

**RESOLVE-REVIEW-THREADS-68** When `resolve-all` reports an aggregate outcome, the system shall count as resolved only mutations with verified requested postconditions, count and identify each evidence refusal exactly once, exclude transport, provider-state, and verification failures from the refusal count, and report the confirmed resolved and refused totals accumulated before any abort.

**RESOLVE-REVIEW-THREADS-69** When body-bearing provider standard error is evaluated for a redacted failure category, the system shall inspect at most 4,096 bytes from physical lines beginning exactly with the GitHub CLI diagnostic prefix `gh:` by retaining only streaming matcher state, recognize an explicit authorization-denial phrase only when it begins after that prefix and optional whitespace or after the exact provider prefix `GraphQL:`, discard all other bytes including request-envelope and response-body echoes, reject a bare HTTP 403, bare permission word, or later quoted denial phrase as authorization proof, and shall not allow access-like reply-body text to create an access-denied category.

**RESOLVE-REVIEW-THREADS-70** When a reply mutation failure is classified as access denied and full-history plus exact-target reconciliation confirms that the attempted reply is absent while the original tail remains last and unresolved, the system shall retain the actual body source, state that the same credentials will be denied again, direct the operator to repair `gh` credentials before retrying, and shall not present a generic immediate exact-body retry as recovery.

## BDD Traceability

- Feature: `agm/test/bdd/features/review_thread_reply_safety.feature`
- Feature: `agm/test/bdd/features/workflow_tooling_guardrails.feature`

## Test Traceability

- Unit package: `cmd/resolve-review-threads`
- Guidance regressions: `cmd/pr-blockers` and `internal/safegit`
