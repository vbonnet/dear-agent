# resolve-review-threads Command Specification

<!-- Last audited at: 2026-09-09 -->

**Version:** 3.8
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

**RESOLVE-REVIEW-THREADS-09** When GitHub CLI reports a GraphQL error, the system shall include GitHub CLI diagnostics in the returned error only when neither the request variables nor the selected response fields can contain a comment body; body-bearing operations shall suppress those diagnostics as required by RESOLVE-REVIEW-THREADS-53.

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

**RESOLVE-REVIEW-THREADS-21** When `reply-resolve` finds its requested reply is already the thread's most recent comment, the system shall skip posting; when the stable initial state was already resolved with that exact reply, it shall report a non-mutating skip, and when the stable initial state was unresolved it shall follow RESOLVE-REVIEW-THREADS-77 rather than adopt the old reply.

**RESOLVE-REVIEW-THREADS-22** When `reply-resolve` has confirmed both that its reply is posted and that it directly follows the originally observed predecessor, but a transient resolution failure prevents terminal resolution, the system shall retain the actual body source unchanged, emit a bounded continuation receipt binding the original evidence, and direct the user to `continue-resolve`, which validates that receipt and resolves without a reply mutation.

**RESOLVE-REVIEW-THREADS-23** When comparing an existing comment against a requested reply, the system shall compare the bodies without whitespace collapsing or truncation, ignoring only surrounding whitespace.

**RESOLVE-REVIEW-THREADS-24** When `reply-resolve` cannot read the thread's current state, the system shall post nothing, leave the thread unchanged, and exit non-zero.

**RESOLVE-REVIEW-THREADS-25** When `reply-resolve` posts a reply and a later non-empty comment anchor confirms that the thread changed before resolution, the system shall report that the reply no longer accounts for the thread and direct the user to read and answer the later comment rather than use the resolution-only command.

**RESOLVE-REVIEW-THREADS-26** When a thread was already resolved by another actor before the pre-mutation re-read and every caller-named anchor and answer-evidence check required by that path succeeds, the system shall report it as skipped and shall not count it among the threads it resolved.

**RESOLVE-REVIEW-THREADS-27** When GitHub refuses a resolution for access reasons, the system shall report it as an access problem rather than prescribing an immediate retry of the same mutation.

**RESOLVE-REVIEW-THREADS-28** When `reply-resolve` posts or recovers a reply, the system shall preserve the predecessor ID and exact body digest from its initial exact-target read, refuse if full history or a stable pre-post read no longer matches that boundary, require that same exact predecessor boundary before an ambiguous absent-reply outcome may license unchanged retry, verify that the reply with its exact accepted body directly follows the unchanged predecessor before resolving, and refuse resolution when another comment or same-ID body edit intervened.

**RESOLVE-REVIEW-THREADS-29** When `reply-resolve` finds its requested reply already present but followed by later comments, the system shall post nothing, leave the thread unresolved, and direct the user to answer those comments.

**RESOLVE-REVIEW-THREADS-30** When `reply-resolve`'s initial state read finds the thread already resolved, the system shall perform a non-mutating stable-history classification before either reporting it as skipped or refusing mismatched, superseded, or unverifiable reply evidence, shall not post, resolve, or reopen the thread, and shall require an explicit confirmed `unresolve` before the revised same-source lifecycle when newer commentary supersedes an earlier requested reply.

**RESOLVE-REVIEW-THREADS-31** When `reply-resolve` resolves a thread, the system shall first confirm that the specific named reply it just posted or recovered within the same invocation is the thread's last comment and still directly follows the unchanged exact predecessor observed before posting.

**RESOLVE-REVIEW-THREADS-32** When the reply mutation returns no comment ID, the system shall not use that response as proof that the reply was posted and shall not resolve the thread unless a full-history re-read recovers the exact reply directly after the originally observed predecessor and verifies that recovered comment as the current tail.

**RESOLVE-REVIEW-THREADS-33** When determining whether a reply is already present, the system shall page through the thread's entire comment history rather than a bounded window.

**RESOLVE-REVIEW-THREADS-34** When `resolve-all` evaluates a candidate thread, the system shall decide refusal from a fresh per-thread read rather than from the listing snapshot.

**RESOLVE-REVIEW-THREADS-35** When `-h`, `--help`, or `help` is requested, the system shall print usage and exit zero.

**RESOLVE-REVIEW-THREADS-36** When comment pagination reports a further page with an empty or unchanged cursor, the system shall abort with an error rather than request the same page again.

**RESOLVE-REVIEW-THREADS-37** When resolving on behalf of a reply, the system shall verify at the pre-mutation read that the named reply is still the last comment.

**RESOLVE-REVIEW-THREADS-38** When the thread becomes resolved while its history is being paged, the system shall report it as skipped only when stable history is a single opening comment or shows the exact requested reply current, shall reopen and refuse when stable history ends in an unanswered reviewer hand-back, shall follow RESOLVE-REVIEW-THREADS-64 for a superseded requested reply, and shall follow RESOLVE-REVIEW-THREADS-73 for different-answer or unavailable-author evidence.

**RESOLVE-REVIEW-THREADS-39** When `reply-resolve`'s resolution is refused for access reasons, the system shall state that an immediate retry will be denied too and direct the user to fix credentials before finishing with `continue-resolve`, the unchanged body-file source, and the emitted receipt, not an unguarded resolution-only command.

**RESOLVE-REVIEW-THREADS-40** When a reply mutation succeeds but its response omits the new comment's ID, the system shall treat whether the reply is live as ambiguous, retain the original predecessor and actual body source, and permit resolution only after reconciliation under RESOLVE-REVIEW-THREADS-58 recovers the exact reply from full history, proves that it directly follows the original predecessor, and verifies its current-tail placement.

**RESOLVE-REVIEW-THREADS-41** When resolving a thread without `--force`, the system shall verify the comment its evidence read was based on against the resolution mutation's own response — using the caller-named anchor when one was given, otherwise the last comment observed by the pre-mutation evidence check — and classify an empty or changed response anchor under RESOLVE-REVIEW-THREADS-59 rather than leave the thread resolved on unverified evidence.

**RESOLVE-REVIEW-THREADS-42** When a resolution mutation succeeds but its response omits the thread's last comment, the system shall treat that outcome as unverifiable, reopen the thread, preserve the actual body source unchanged, and shall not treat the missing anchor as a confirmed later comment or superseded answer.

**RESOLVE-REVIEW-THREADS-43** When a stale or unverifiable resolution is detected and the automatic reopen also fails, the system shall report the thread as still resolved, direct the user to run `unresolve` before any other action, and require a fresh state inspection before selecting unchanged-retry or revised-answer guidance.

**RESOLVE-REVIEW-THREADS-44** When recovery depends on matching an identical reply body, the system shall direct the user to reuse the actual unchanged named path or standard-input form and any emitted continuation receipt without rendering the reply body as command-line or shell-evaluable diagnostic text.

**RESOLVE-REVIEW-THREADS-45** When a resolution mutation reports an error, the system shall re-read the thread before treating that as a clean no-op, since the client can fail after GitHub already applied the mutation server-side; if the re-read confirms it resolved, the system shall apply the same anchor and author verification and reopen-on-mismatch as a normally-reported success, but shall describe the matching state as independently confirmed rather than attribute or count the transition as this caller's mutation.

**RESOLVE-REVIEW-THREADS-46** When a reply mutation reports an error, the system shall preserve the predecessor observed before posting and re-read the thread's history before reporting the reply as failed, since the client can fail after GitHub already applied the mutation server-side; recovery shall follow RESOLVE-REVIEW-THREADS-58 rather than discard that predecessor or invite a blind, duplicating retry.

**RESOLVE-REVIEW-THREADS-47** When an automatic reopen is attempted, the system shall verify the mutation's own resolved-state postcondition rather than treat a nil error as proof the thread reopened, since another actor can race the same thread; on an access-denial failure, the system shall preserve that denial even when a fresh read independently confirms the requested safe state, shall not attribute the state transition to the denied caller, and shall require credential repair before later provider mutation.

**RESOLVE-REVIEW-THREADS-48** When a resolution's named anchor is no longer the thread's last comment AND the thread is already resolved, the system shall reopen it before reporting the evidence refusal, so a later retry does not silently no-op against a resolved thread whose intervening comment was never read.

**RESOLVE-REVIEW-THREADS-49** When a body-file source contains valid UTF-8 reply content within the declared size limits, the system shall preserve its complete byte sequence as the provider-visible reply body without trimming or normalization.

**RESOLVE-REVIEW-THREADS-50** When a body-file source is missing, duplicated, non-regular, unreadable, oversized, invalid UTF-8, empty, or whitespace-only, the system shall fail before issuing any provider mutation.

**RESOLVE-REVIEW-THREADS-51** When invoking an external process for a review-thread GraphQL operation, the system shall pass the query and variables as data input and shall not place any variable value in process arguments.

**RESOLVE-REVIEW-THREADS-52** The system shall not evaluate reply-body content as shell syntax or permit embedded command substitutions to execute.

**RESOLVE-REVIEW-THREADS-53** When the provider client fails an operation whose variables include a reply body or whose GraphQL selection can return a comment body, the system shall remove provider debug environment, shall not retain, return, log, or print raw standard output or standard error because debug output can echo the body, and shall preserve only a bounded redacted typed failure category.

**RESOLVE-REVIEW-THREADS-54** When a named body-file source is selected, the system shall open it without blocking on a substituted non-regular object on Unix, validate the opened descriptor as a regular file before reading it, and direct streaming input to the explicit standard-input source.

**RESOLVE-REVIEW-THREADS-55** When any body source emits more than 262,144 bytes or valid UTF-8 contains more than 65,536 Unicode code points, the system shall stop after reading at most 262,145 bytes and fail before issuing any provider mutation.

**RESOLVE-REVIEW-THREADS-56** When the review-thread command's generated help or recovery guidance instructs an operator to create a named reply-body file, the system shall prescribe a fresh task-owned system-temporary path outside the repository for each thread, quote that path in review-thread command invocations, direct retention and unchanged reuse through every applicable exact-body retry, and direct removal only after terminal resolution is confirmed and before proceeding to another thread or merge.

**RESOLVE-REVIEW-THREADS-57** When later reviewer comments conclusively supersede the current reply body while its thread remains unresolved, repository-generated remediation guidance shall direct the operator to revise the same actual named path or retained standard-input source rather than create or assign a replacement path, retain the revised source unchanged through every applicable exact-body retry, and conditionally remove a task-owned temporary source only after terminal resolution is confirmed.

**RESOLVE-REVIEW-THREADS-58** When a reply mutation has an ambiguous outcome or omits the new comment ID, the system shall retain the originally observed predecessor ID, exact body digest, provider update time, edit-history count, latest retained edit ID, opening author, and actual reply source; re-read full history and exact target state; recover a byte-exact matching reply only when its original predecessor is present and the mutation failure was not a classified access denial; accept delayed visibility only after exact predecessor-to-reply placement becomes observable; permit unchanged-source retry only when both recovery reads match that complete predecessor boundary and any classified access denial has first been repaired under RESOLVE-REVIEW-THREADS-70; require inspection and same-source revision when a complete boundary field is confirmed changed; and require inspection without blind retry or recovery claims when history omits the predecessor or state evidence is incomplete.

**RESOLVE-REVIEW-THREADS-59** When the resolution response supplies an empty last-comment anchor, the system shall classify the outcome as unverifiable and retain the actual body source unchanged for inspection and retry after reopening; when it supplies a non-empty anchor different from the expected anchor, the system shall classify the thread as changed and require the later comment to be read and answered with the same source before resolution.

**RESOLVE-REVIEW-THREADS-60** When reply-placement verification finds the reply buried or jumped and the thread is already resolved, the system shall reopen the thread and verify the reopen postcondition before emitting revised-answer guidance; if reopening fails, the system shall report that the thread remains resolved and require `unresolve` before reply recovery.

**RESOLVE-REVIEW-THREADS-61** When a named reply remains last but answer evidence is insufficient because an author login is missing, changes between full history and the stable exact-target read, or equals the opening author, or when recovery state cannot be read, the system shall retain the actual body source unchanged, report the precise uncertainty, and require thread inspection without posting or claiming that a reviewer follow-up occurred; a continuation lifecycle shall retain its receipt and source and forbid retry until author identity is available and distinct; a nonempty later comment ID after an earlier exact reply may establish positional supersession without its author or body and shall still trigger verified corrective reopen when concurrently resolved.

**RESOLVE-REVIEW-THREADS-62** When unchanged-retry or revised-answer guidance references a reply body source, the system shall preserve the caller's actual source form by rendering a reusable named path with lossless shell quoting or the standard-input marker `-`, shall not synthesize a replacement path, and shall preserve task-owned cleanup identity until terminal resolution is confirmed.

**RESOLVE-REVIEW-THREADS-63** When a resolve or unresolve mutation response is evaluated, the system shall require a non-empty target thread identity equal to the requested node ID and a resolved-state value equal to the requested postcondition before claiming, printing, or counting that mutation as successful; otherwise, it shall treat the response as unverified and reconcile fresh provider state before making a state claim.

**RESOLVE-REVIEW-THREADS-64** When `reply-resolve` observes a full-history tail or existing-reply predecessor different from its initial exact-target read, the system shall perform one fresh exact-target reconciliation before recovery, shall remain non-mutating when the initial thread was already resolved, and when an initially unresolved thread is now resolved on changed complete evidence it shall verify corrective reopen before emitting same-source revised-answer guidance.

**RESOLVE-REVIEW-THREADS-65** When predecessor, recovered-reply, current-tail, or mutation-anchor evidence contains an empty comment node ID, the system shall treat the evidence as incomplete and unverified, retain the actual body source, and require fresh inspection without claiming stable placement, successful resolution, or a conclusively superseding comment.

**RESOLVE-REVIEW-THREADS-66** When a resolve mutation reports a transport error, the system shall re-read the requested thread and classify the fresh result as an independently confirmed no-op only when identity, requested resolved state, expected comment anchor, and required author evidence all match, without attributing or counting that state transition as this caller's mutation; as stale or unverifiable resolution requiring verified reopen when resolved state is present but anchor evidence is empty or changed; as an absent requested postcondition when the matching thread is currently unresolved; or as unknown state requiring inspection when the re-read fails or returns mismatched identity.

**RESOLVE-REVIEW-THREADS-67** When an unresolve mutation reports a transport error or returns an unverified response, the system shall reconcile the requested thread from a fresh read before making a state claim, count success only when matching identity and unresolved state are confirmed, report the requested postcondition as absent when matching identity remains resolved, and require inspection when the fresh state is unreadable or identifies another thread.

**RESOLVE-REVIEW-THREADS-68** When `resolve-all` reports an aggregate outcome, the system shall count as resolved only mutations with verified requested postconditions, count and identify each evidence refusal exactly once, exclude transport, provider-state, and verification failures from the refusal count, and report the confirmed resolved and refused totals accumulated before any abort.

**RESOLVE-REVIEW-THREADS-69** When body-bearing provider standard error is evaluated for a redacted failure category, the system shall inspect at most 4,096 bytes from physical lines beginning exactly with the GitHub CLI diagnostic prefix `gh:` by retaining only streaming matcher state, recognize an explicit authorization-denial phrase only when it begins after that prefix and optional whitespace or after the exact provider prefix `GraphQL:`, discard all other bytes including request-envelope and response-body echoes, reject a bare HTTP 403, bare permission word, or later quoted denial phrase as authorization proof, and shall not allow access-like reply-body text to create an access-denied category.

**RESOLVE-REVIEW-THREADS-70** When a reply mutation failure is classified as access denied, the system shall preserve that category and credential-repair requirement across unchanged, moved, edited, missing-predecessor, and state-read reconciliation outcomes; if a byte-exact reply subsequently appears, the system shall treat it as independently observed, shall not attribute, adopt, rebind, freshly receipt, or resolve it through the denied lifecycle, and shall still verify corrective reopen when decisive current placement proves a stale resolution.

**RESOLVE-REVIEW-THREADS-71** When a posted reply is confirmed as directly following its original predecessor and remaining the current tail but terminal resolution is not, the system shall emit a bounded version-3 continuation receipt whose canonical payload binds the normalized effective GitHub provider host; thread ID; predecessor and reply IDs; exact body SHA-256 values; provider update times; edit-history counts and latest retained edit IDs; and opening and reply authors, without including comment text, the local source path, or signing key.

**RESOLVE-REVIEW-THREADS-72** When `continue-resolve` is requested with an authenticated receipt, the system shall validate the replayed bounded body source against the reply digest before provider access; verify full-history predecessor-to-reply adjacency and every bound body, update-time, edit-generation, and author field; require the named reply as the current tail; resolve against that exact predecessor-reply boundary; and shall not issue a reply mutation; when full history conclusively rejects the receipt and a fresh exact read proves the thread already resolved, the system shall carry that conclusive mismatch through unrelated missing fresh-read fields, reopen the thread, and confirm the safe unresolved state, while an incomplete named-ID or history read shall remain unverified and non-mutating.

**RESOLVE-REVIEW-THREADS-73** When ordinary `reply-resolve` finds no exact requested body but a stable independently authored answer with different bytes is already last, the system shall post nothing and require inspection or exact-source restoration; when an unresolved stable tail instead confirms reviewer hand-back, the system shall permit a revised answer, while an initially resolved reviewer hand-back shall be non-mutatingly refused and a concurrently resolved hand-back shall be reopened before refusal; and when the required author identity is unavailable, the system shall fail closed without posting.

**RESOLVE-REVIEW-THREADS-74** When a continuation receipt is malformed, oversized, unsupported, non-canonical, unsigned, authenticated by another key, names empty evidence, mismatches the replayed reply or any bound provider-visible evidence, lacks the named predecessor-to-reply adjacency, or names a reply that is not the current tail, the system shall issue neither reply nor resolution mutation and shall retain the source for inspection or revision.

**RESOLVE-REVIEW-THREADS-75** When `reply-resolve` or `continue-resolve` supplies exact reply evidence to resolution, the system shall verify the named predecessor and reply IDs plus every bound body, update-time, edit-generation, opening-author, and reply-author field in the immediate pre-mutation read and the resolution mutation response, and shall reopen a resolution whose response does not preserve that evidence.

**RESOLVE-REVIEW-THREADS-76** When a continuation receipt is decoded, the system shall require the exact `payload.signature` form in which both components are unpadded base64url, the payload is one canonical JSON object containing each known field exactly once with safe bounded evidence values, and the signature is HMAC-SHA-256 over the exact canonical payload bytes; it shall reject duplicate or unknown fields, control characters, shell syntax, non-canonical encoding, trailing data, payload tampering, signature tampering, or wrong-key authentication before reading the body source or contacting the provider.

**RESOLVE-REVIEW-THREADS-77** When ordinary `reply-resolve` finds an exact independently authored tail reply on a thread that was unresolved at its initial read, the system shall not treat current adjacency as proof that the reply preceded any same-ID edit, shall not resolve it or mint a fresh continuation receipt, and shall require the original retained receipt or explicit inspection and reviewer hand-back before a fresh reply lifecycle.

**RESOLVE-REVIEW-THREADS-78** When current provider evidence is compared with a named reply boundary, the system shall classify a nonempty changed tail or predecessor ID before testing unrelated body completeness, shall treat that ID difference as decisive changed placement, and shall verify corrective reopen if the thread is resolved; a conclusive mismatch established from full history shall remain decisive when a later current-state read omits unrelated body evidence; it shall classify evidence as incomplete without mutation only when a required ID is empty or bodies are omitted while the named IDs can still match.

**RESOLVE-REVIEW-THREADS-79** When direct `unresolve` is denied but a fresh exact-target read independently confirms unresolved state, the system shall report the requested safe state as independently confirmed, preserve the access-denial cause and credential-repair requirement, and shall not claim that this caller applied, corrected, or reopened anything.

**RESOLVE-REVIEW-THREADS-80** When a reply could create an outstanding continuation obligation beneath a shared application-state namespace, the system shall accept an existing real and stable namespace whose POSIX mode may grant group or other read and traverse access but grants no group or other write access, shall preserve that namespace's identity and permissions, shall establish private, durable, and stable command-owned issuer state beneath it before posting, and shall fail before provider mutation when any of those boundaries cannot be safely established or verified for later continuation.

**RESOLVE-REVIEW-THREADS-81** When `continue-resolve` receives a receipt, the system shall authenticate and canonically decode the receipt against the existing local issuer state and compare its provider host with the normalized effective ambient GitHub host before reading the body source or contacting the provider; the system shall treat that state as a same-user local issuance boundary rather than protection from an operating-system user who can access or replace it or invoke `gh` directly.

**RESOLVE-REVIEW-THREADS-82** When a structurally valid continuation cannot be authenticated because the original local issuer state is unavailable, unreadable, replaced, or changed, or when its bound provider host differs from the ambient host, the system shall retain both the exact receipt and actual source, issue no provider request or mutation, instruct restoration of the exact issuing host and issuer state, and forbid substitution or regeneration of that state; malformed, non-canonical, or bad-authenticator input may instead be described as an invalid receipt.

**RESOLVE-REVIEW-THREADS-83** While the exact currently provider-visible extant comment boundary remains unchanged, an authenticated continuation receipt may be replayed after a bare manual unresolve; when any currently extant bound content, identity, time, edit generation, host, or adjacency evidence changes, the system shall revoke that continuation boundary; because deleted comments are absent from GitHub's current comment connection, the system shall not claim that the receipt proves an irreversible event history or reviewer acceptance.

**RESOLVE-REVIEW-THREADS-84** If execution stops after GitHub accepts a reply but before the continuation receipt is emitted, the system shall leave the posted reply unresolved, shall not infer or rebind issuance authority on a later run, and shall direct human inspection followed by the existing bare `resolve` operation only when the operator accepts the exact current answer.

**RESOLVE-REVIEW-THREADS-85** When a provider comment reports an edit-history connection, the system shall bind and compare both `totalCount` and the latest retained edit node ID, using an empty latest edit ID only when the count is zero, so a new edit remains detectable after the provider's retained edit count saturates.

**RESOLVE-REVIEW-THREADS-86** When a non-force resolve mutation reports success or an independently confirmed resolved state, the system shall atomically re-prove the opening and answer author boundary from the returned or fresh provider state and shall reopen or refuse the resolution when either identity is missing, changes from the pre-read boundary, or becomes equal.

**RESOLVE-REVIEW-THREADS-87** While a non-Unix platform lacks runtime proof that private, durable local issuer state can be established, the system shall reject `reply-resolve` continuation preparation before issuing a reply mutation and shall reject `continue-resolve` receipt loading before reading the body source or contacting the provider; support for that platform shall require equivalent privacy and durability evidence.

## BDD Traceability

- Feature: `agm/test/bdd/features/review_thread_reply_safety.feature`
- Feature: `agm/test/bdd/features/workflow_tooling_guardrails.feature`

## Test Traceability

- Unit package: `cmd/resolve-review-threads`
- Guidance regressions: `cmd/pr-blockers` and `internal/safegit`
