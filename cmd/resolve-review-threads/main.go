// Command resolve-review-threads lists and resolves GitHub PR review threads.
//
// # Why this exists
//
// Gemini / bot reviewers open review threads on PRs. dear-agent's main branch
// has required_conversation_resolution=true, so a PR cannot merge while any
// thread is unresolved. Replying to a comment does NOT resolve its thread —
// resolution is a distinct GraphQL mutation. This tool resolves bot threads
// without a human clicking "Resolve conversation".
//
// # Key facts (verified against the live GitHub GraphQL schema, 2026-06-09)
//
//   - Thread resolution lives ONLY in GraphQL. There is NO REST endpoint.
//   - The mutation is resolveReviewThread(input:{threadId: ID!}). The input
//     field is threadId (NOT pullRequestReviewThreadId).
//   - Thread IDs come from repository.pullRequest.reviewThreads[].id and look
//     like "PRRT_kwDO...". They are NOT the review-comment IDs from REST.
//
// # Usage
//
//	resolve-review-threads list          <owner> <repo> <pr>           # unresolved threads (JSON lines)
//	resolve-review-threads list-all      <owner> <repo> <pr>           # every thread
//	resolve-review-threads resolve       <threadId> [--force]           # one thread by ID
//	resolve-review-threads reply-resolve <threadId> --body-file <path|-> # reply, then resolve
//	resolve-review-threads continue-resolve <receipt> --body-file <path|-> # resolve posted reply
//	resolve-review-threads resolve-all   <owner> <repo> <pr> [author]  # answered threads only
//	resolve-review-threads unresolve     <threadId>                    # re-open a thread
//
// # Why resolve-all refuses unanswered threads
//
// Resolution is a claim that the reviewer's point was handled. Two opposite
// failure modes have both shipped here: bulk-resolving every thread without
// addressing any of them (silently discarding real findings), and replying to
// every thread while resolving none (leaving the PR blocked forever). Both are
// invisible after the fact, because a resolved thread looks identical whether
// or not anyone read it.
//
// So resolution requires evidence, and the only evidence GitHub records
// deterministically is a reply on the thread. A thread is ANSWERED when its
// last comment comes from someone other than the reviewer who opened it. That
// is a weak proof of correctness but a strong proof of engagement, and it is
// checkable without judgment. `resolve-all` resolves ANSWERED threads and
// refuses the rest by name; `reply-resolve` makes answer-then-resolve one
// atomic step so the natural path closes the thread.
//
// isOutdated is deliberately NOT a licence to resolve. Outdated means the diff
// hunk moved, not that the point was addressed: on dear-agent#1242, three
// outdated threads were unaddressed P1 findings. It is reported, never acted
// on.
//
// All GitHub calls go through `gh api graphql`, so authentication uses the gh
// CLI's token (no git push, no keychain prompt). Requires gh (authenticated).
// Continuation receipts currently require Unix: non-Unix platforms fail before
// reply mutation or receipt loading because POSIX modes and cross-compilation
// cannot prove owner-private key state and directory-entry durability there.
//
// The GraphQL queries/mutations and the thread-fetching/mutating layer live in
// threads.go; this file is the CLI: argument parsing, command dispatch, and
// the reply-then-resolve safety argument.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// bodyPreviewLen caps the comment-body preview surfaced in list output.
	bodyPreviewLen = 120
	// GitHub-owned review tooling caps comment bodies at 65,536 Unicode code
	// points. The byte ceiling keeps reads finite while allowing UTF-8's
	// largest encoding for every accepted code point.
	maxReplyBodyCharacters = 65_536
	maxReplyBodyBytes      = maxReplyBodyCharacters * utf8.UTFMax
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 1
	}
	ctx := context.Background()
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "-h", "--help", "help":
		usage()
		return 0
	case "list", "list-all":
		return cmdList(ctx, cmd, rest)
	case "resolve", "unresolve":
		return cmdMutate(ctx, cmd, rest)
	case "reply-resolve":
		return cmdReplyResolve(ctx, rest)
	case "continue-resolve":
		return cmdContinueResolve(ctx, rest)
	case "resolve-all":
		return cmdResolveAll(ctx, rest)
	default:
		usage()
		return 1
	}
}

// cmdList handles "list" (unresolved only) and "list-all" (every thread).
func cmdList(ctx context.Context, cmd string, rest []string) int {
	if len(rest) != 3 {
		return fail("usage: %s <owner> <repo> <pr>", cmd)
	}
	pr, err := strconv.Atoi(rest[2])
	if err != nil {
		return fail("pr must be an integer, got %q", rest[2])
	}
	threads, err := listThreads(ctx, rest[0], rest[1], pr)
	if err != nil {
		return fail("%v", err)
	}
	if cmd == "list" {
		threads = filterThreads(threads, "")
	}
	if err := printThreads(threads); err != nil {
		return fail("%v", err)
	}
	return 0
}

// cmdMutate handles "resolve" and "unresolve" of a single thread by ID.
// "resolve" enforces the same evidence rule as resolve-all: a single-thread
// path that skipped it would be a documented way around the gate. "unresolve"
// re-opens a thread, which is never the unsafe direction, so it goes straight
// through.
func cmdMutate(ctx context.Context, cmd string, rest []string) int {
	force := false
	args := make([]string, 0, len(rest))
	for _, a := range rest {
		if a == "--force" {
			force = true
			continue
		}
		args = append(args, a)
	}
	if len(args) != 1 {
		return fail("usage: %s <threadId> [--force]", cmd)
	}
	if cmd == "unresolve" {
		msg, err := unresolveWithEvidence(ctx, args[0])
		if err != nil {
			if recovered, ok := errors.AsType[*accessDeniedMutationWithConfirmedStateError](err); ok {
				return fail("the requested unresolved state is independently confirmed, but this caller cannot claim the mutation: %v\n"+
					"repair `gh` credentials before any later provider mutation; unchanged credentials will be denied again.\n"+
					"Inspect the fresh thread before selecting the next lifecycle:\n%s",
					recovered, bareResolutionInspectionGuidance(args[0]))
			}
			if recovery, handled := replyResolutionEvidenceFailure(
				args[0],
				err,
				newAnswerRecoveryGuidance(args[0], false),
			); handled {
				return fail("%s", recovery)
			}
			return fail("%v", err)
		}
		fmt.Println(msg)
		return 0
	}
	msg, _, err := resolveWithEvidence(ctx, args[0], force, resolutionEvidence{})
	if err != nil {
		if recovery, handled := replyResolutionEvidenceFailure(
			args[0],
			err,
			newAnswerRecoveryGuidance(args[0], force),
		); handled {
			return fail("%s", recovery)
		}
		return fail("%v", err)
	}
	fmt.Println(msg)
	return 0
}

func parseReplyResolveArgs(args []string) (threadID, bodyFile string, err error) {
	const usage = "usage: reply-resolve <threadId> --body-file <path|->"
	if len(args) != 3 || args[1] != "--body-file" || strings.TrimSpace(args[0]) == "" || args[2] == "" {
		return "", "", errors.New(usage)
	}
	return args[0], args[2], nil
}

// loadReplyBody reads the caller-selected data source once, within a fixed
// memory bound, and returns accepted bytes unchanged. UTF-8 is validated before
// conversion because encoding/json replaces invalid string bytes with U+FFFD,
// which would silently violate the exact-body contract.
func loadReplyBody(path string, stdin io.Reader) (string, error) {
	var reader io.Reader
	if path == "-" {
		if stdin == nil {
			return "", errors.New("read reply body from stdin: input is unavailable")
		}
		reader = stdin
	} else {
		file, err := openReplyBodyFile(path)
		if err != nil {
			return "", fmt.Errorf("read reply body from %q: %w", path, err)
		}
		defer func() {
			_ = file.Close()
		}()

		openedInfo, err := file.Stat()
		if err != nil {
			return "", fmt.Errorf("inspect reply body source %q: %w", path, err)
		}
		if !openedInfo.Mode().IsRegular() {
			return "", fmt.Errorf(
				"reply body source %q must be a regular file; use --body-file - for standard input",
				path,
			)
		}
		if openedInfo.Size() > maxReplyBodyBytes {
			return "", replyBodyTooLargeError(path)
		}
		reader = file
	}

	body, err := io.ReadAll(io.LimitReader(reader, maxReplyBodyBytes+1))
	if err != nil {
		return "", fmt.Errorf("read reply body from %q: %w", path, err)
	}
	if len(body) > maxReplyBodyBytes {
		return "", replyBodyTooLargeError(path)
	}
	if !utf8.Valid(body) {
		return "", fmt.Errorf("reply body from %q must be valid UTF-8", path)
	}
	if utf8.RuneCount(body) > maxReplyBodyCharacters {
		return "", replyBodyTooLargeError(path)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "", errors.New("reply body must not be empty: resolution needs a stated reason")
	}
	return string(body), nil
}

func replyBodyTooLargeError(path string) error {
	return fmt.Errorf(
		"reply body from %q exceeds the limit of %d Unicode characters or %d UTF-8 bytes",
		path,
		maxReplyBodyCharacters,
		maxReplyBodyBytes,
	)
}

// cmdReplyResolve posts a reply on one thread and then resolves it, so the
// public justification and the resolution cannot drift apart. Resolving is
// skipped when the reply fails, leaving the thread open rather than silently
// closed with no explanation.
func cmdReplyResolve(ctx context.Context, rest []string) int {
	threadID, bodyFile, err := parseReplyResolveArgs(rest)
	if err != nil {
		return fail("%v", err)
	}
	body, err := loadReplyBody(bodyFile, os.Stdin)
	if err != nil {
		return fail("%v", err)
	}
	// Read first: to skip an already-resolved thread, and to notice a reply a
	// previous run already posted so a retry does not duplicate it. Without
	// the current state we cannot tell whether a previous run already posted
	// this reply, and posting blind is the duplicate this guard exists to
	// prevent, so any failure here leaves the thread untouched.
	initial, code := readInitialReplyState(
		ctx,
		threadID,
		"cannot read thread state, nothing posted",
		bodyFile,
	)
	if code >= 0 {
		return code
	}

	// Decide against the FULL history, not the tail: a prior reply buried by
	// later discussion must still be found, or it gets reposted and anchored
	// to the newest follow-up, which resolves everything in between unread.
	// boundary is the exact two-comment tail classifyPriorReply reasoned about.
	// It binds bodies as well as IDs because provider-side edits retain IDs.
	history, boundary, code := fetchHistoryTail(ctx, threadID, bodyFile)
	if code >= 0 {
		return code
	}
	if boundary.LastID != initial.LastID ||
		boundary.LastBodySHA256 != exactBodySHA256([]byte(initial.LastBody)) {
		return rejectInitialHistoryMismatch(
			ctx,
			threadID,
			bodyFile,
			initial,
			boundary,
			fmt.Sprintf("initially observed tail %s changed before full history reached tail %s", initial.LastID, boundary.LastID),
		)
	}
	priorReply := history.classify(body)
	if priorReply == priorReplyIsLast {
		if initial.PrevID == "" || !initial.PrevBodyPresent {
			return rejectInitialHistoryMismatch(
				ctx,
				threadID,
				bodyFile,
				initial,
				boundary,
				"the initial exact-target read omitted the predecessor needed to classify the existing reply",
			)
		}
		if initial.PrevID != boundary.PredecessorID ||
			exactBodySHA256([]byte(initial.PrevBody)) != boundary.PredecessorBodySHA256 {
			return rejectInitialHistoryMismatch(
				ctx,
				threadID,
				bodyFile,
				initial,
				boundary,
				"the existing reply's predecessor ID or exact body changed between the initial read and full history",
			)
		}
	}

	// Paging a long thread takes time, so re-read state before acting on it:
	// another actor may have resolved it meanwhile, and replying then would
	// comment on a settled conversation. If the tail has moved since history
	// was read, a comment landed in that gap that classifyPriorReply never
	// saw; refuse rather than silently adopt it as the anchor, which would
	// treat that unread comment as accounted for.
	stablePrePost, code := readMatchingTail(
		ctx,
		threadID,
		bodyFile,
		boundary,
		priorReply,
		initial.IsResolved,
	)
	if code >= 0 {
		return code
	}
	if initial.IsResolved && priorReply == priorReplyIsLast {
		// The stable three-read boundary has now confirmed that the exact
		// requested reply was already last on a thread that was resolved before
		// this command began. Do not carry that foreign resolved state into the
		// placement or resolution seams: a later edit or reopen must be handled by
		// a fresh invocation, never "corrected" by this non-mutating observation.
		if history.last().Body != body {
			return fail("thread %s was already resolved with a trim-equivalent reply whose exact bytes differ from the selected source; no mutation was attempted.\n%s",
				threadID, stableDifferentAnswerGuidance(threadID, bodyFile))
		}
		fmt.Printf("skipped %s (already resolved before this command)\n", threadID)
		return 0
	}

	// anchorID must be the thread's last comment, and anchorPrevID must be the
	// comment before it, for resolution to be allowed. Both are required: see
	// checkReplyPlacement.
	var anchorID, anchorPrevID, anchorPrevBody string
	var issuer continuationIssuer
	var postEvidence replyMutationEvidence
	var predecessorEvidence replyIssuancePredecessor
	switch priorReply {
	case priorReplySuperseded:
		// A previous run posted this reply and newer commentary follows it.
		// Reposting would duplicate the comment AND put our copy last, making
		// the thread look answered while the follow-up went unread.
		guidance := revisedReplyBodyGuidance(threadID, bodyFile)
		if initial.IsResolved {
			guidance = resolvedSupersededReplyGuidance(threadID, bodyFile)
		}
		return fail("your earlier reply is already on thread %s and newer "+
			"commentary follows it; nothing was posted.\n"+
			"read the comment(s) after your reply and answer those:\n%s", threadID,
			guidance)
	case differentAnswerIsLast:
		return fail("thread %s already has a different independently authored answer as its current tail; "+
			"nothing was posted or resolved. Ordinary reply-resolve must not stack a second answer merely "+
			"because the requested bytes differ.\n%s", threadID,
			stableDifferentAnswerGuidance(threadID, bodyFile))
	case unavailableReplyIntent:
		return fail("thread %s has incomplete author evidence, so whether another answer may be posted "+
			"cannot be proved; nothing was posted or resolved.\n%s", threadID,
			unavailableAuthorEvidenceGuidance(threadID, bodyFile))
	case priorReplyIsLast:
		if exactBodySHA256([]byte(history.last().Body)) != exactBodySHA256([]byte(body)) {
			return fail("thread %s has a matching independently authored answer at the current tail, but "+
				"its exact bytes differ from the selected source; nothing was posted or resolved.\n%s",
				threadID, stableDifferentAnswerGuidance(threadID, bodyFile))
		}
		// Current adjacency cannot prove when either body last changed. Only the
		// receipt emitted by the original post attempt preserves that temporal
		// boundary. Refuse rather than laundering an old reply into newly minted
		// evidence from today's predecessor bytes.
		return fail("thread %s already has the exact requested reply as its current tail, but ordinary reply-resolve cannot rebind that unreceipted temporal pairing or mint a fresh receipt; nothing was posted or resolved.\n%s",
			threadID, unreceiptedExistingReplyGuidance(threadID, bodyFile))
	case noPriorReply, reviewerHandbackIsLast:
		issuer, err = prepareContinuationIssuer()
		if err != nil {
			return fail("continuation signing state could not be established, so nothing was posted: %v\n%s",
				err, inspectReplyOutcomeGuidance(threadID, bodyFile))
		}
		// Our reply must land directly after the comment we just read.
		anchorPrevID = boundary.LastID
		anchorPrevBody = history.last().Body
		predecessorEvidence = replyIssuancePredecessor{
			ID:                anchorPrevID,
			BodySHA256:        exactBodySHA256([]byte(anchorPrevBody)),
			UpdatedAt:         boundary.LastUpdatedAt,
			EditCount:         boundary.LastEditCount,
			EditCountPresent:  boundary.LastEditCountPresent,
			LastEditID:        boundary.LastEditID,
			LastEditIDPresent: boundary.LastEditIDPresent,
			OpeningAuthor:     boundary.OpeningAuthor,
			Author:            boundary.LastAuthor,
			ReplyBodySHA256:   exactBodySHA256([]byte(body)),
		}
		if stablePrePost.LastUpdatedAt != predecessorEvidence.UpdatedAt ||
			!stablePrePost.LastEditCountPresent || !predecessorEvidence.EditCountPresent ||
			stablePrePost.LastEditCount != predecessorEvidence.EditCount ||
			!stablePrePost.LastEditIDPresent || !predecessorEvidence.LastEditIDPresent ||
			stablePrePost.LastEditID != predecessorEvidence.LastEditID ||
			stablePrePost.Author != predecessorEvidence.OpeningAuthor ||
			stablePrePost.LastAuthor != predecessorEvidence.Author {
			return fail("provider state changed at the continuation issuance boundary; nothing was posted or resolved.\n%s",
				unavailableAuthorEvidenceGuidance(threadID, bodyFile))
		}
		if err := issuer.preflightKnownFields(
			threadID,
			predecessorEvidence.ID,
			[]byte(anchorPrevBody),
			[]byte(body),
			predecessorEvidence.UpdatedAt,
			predecessorEvidence.EditCount,
			predecessorEvidence.LastEditID,
			predecessorEvidence.OpeningAuthor,
		); err != nil {
			return fail("continuation receipt fields could not be validated, so nothing was posted: %v\n%s",
				err, inspectReplyOutcomeGuidance(threadID, bodyFile))
		}
		var postCode int
		postEvidence, postCode = postReplyEvidenceOrExit(
			ctx,
			threadID,
			body,
			predecessorEvidence,
			bodyFile,
		)
		if postCode >= 0 {
			return postCode
		}
		anchorID = postEvidence.ID
		if postEvidence.Recovered {
			postEvidence, err = recoverIssuedReplyEvidence(
				ctx,
				threadID,
				predecessorEvidence,
				postEvidence,
			)
			if err != nil {
				return rejectUnissuablePostedReply(ctx, threadID, bodyFile, err.Error())
			}
		}
	}
	replyEvidence := resolutionEvidence{
		LastID:                anchorID,
		PredecessorID:         anchorPrevID,
		PredecessorBodySHA256: exactBodySHA256([]byte(anchorPrevBody)),
		BodySHA256:            exactBodySHA256([]byte(body)),
	}
	placementState, code := verifyExactReplyPlacementState(ctx, threadID, replyEvidence, bodyFile)
	if code != 0 {
		return code
	}
	issuedEvidence := replyEvidence
	issuedEvidence.PredecessorUpdatedAt = predecessorEvidence.UpdatedAt
	issuedEvidence.ReplyUpdatedAt = postEvidence.UpdatedAt
	issuedEvidence.PredecessorEditCount = predecessorEvidence.EditCount
	issuedEvidence.ReplyEditCount = postEvidence.EditCount
	issuedEvidence.PredecessorEditCountPresent = predecessorEvidence.EditCountPresent
	issuedEvidence.ReplyEditCountPresent = postEvidence.EditCountPresent
	issuedEvidence.PredecessorLastEditID = predecessorEvidence.LastEditID
	issuedEvidence.ReplyLastEditID = postEvidence.LastEditID
	issuedEvidence.PredecessorLastEditIDPresent = predecessorEvidence.LastEditIDPresent
	issuedEvidence.ReplyLastEditIDPresent = postEvidence.LastEditIDPresent
	issuedEvidence.OpeningAuthor = predecessorEvidence.OpeningAuthor
	issuedEvidence.ReplyAuthor = postEvidence.Author
	if err := issuedEvidence.validate(); err != nil {
		return rejectUnissuablePostedReply(ctx, threadID, bodyFile, err.Error())
	}
	if err := validateExactResolutionEvidence(ctx, threadID, placementState, issuedEvidence); err != nil {
		message, _ := replyResolutionEvidenceFailure(
			threadID,
			err,
			revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
		)
		return fail("%s", message)
	}
	receiptToken, err := issuer.issue(
		threadID,
		anchorPrevID,
		anchorID,
		[]byte(anchorPrevBody),
		[]byte(body),
		issuedEvidence.PredecessorUpdatedAt,
		issuedEvidence.ReplyUpdatedAt,
		issuedEvidence.PredecessorEditCount,
		issuedEvidence.ReplyEditCount,
		issuedEvidence.PredecessorLastEditID,
		issuedEvidence.ReplyLastEditID,
		issuedEvidence.OpeningAuthor,
		issuedEvidence.ReplyAuthor,
	)
	if err != nil {
		return fail("the reply is posted, but a resolve-only continuation receipt could not be created; "+
			"the command did not attempt resolution: %v\n%s", err,
			inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	continuationGuidance := continuationRecoveryGuidance(threadID, receiptToken, bodyFile)
	msg, _, rErr := resolveWithEvidence(ctx, threadID, false, issuedEvidence)
	if rErr != nil {
		if message, handled := replyResolutionEvidenceFailure(
			threadID,
			rErr,
			continuationGuidance,
		); handled {
			return fail("%s", message)
		}
		if isAccessDenied(rErr) {
			// Retrying immediately would just repeat the denied mutation; the
			// credential problem has to be fixed first. Once it is, prefer
			// continue-resolve over bare resolve: it preserves the original
			// predecessor, reply, and exact body-byte checks without another
			// path to the reply mutation.
			return fail("your reply is posted, but GitHub refused the resolution: %v\n"+
				"this is an access problem, not a transient one: retrying immediately "+
				"will be denied too.\n"+
				"check `gh auth status` and that the token can resolve threads on this "+
				"repo, then follow this resolve-only continuation lifecycle:\n%s", rErr,
				continuationGuidance.unchanged)
		}
		return fail("the reply is posted but the thread is NOT resolved (likely "+
			"transient): %v\n"+
			"retry only with this resolve-only continuation; it revalidates the "+
			"original predecessor, reply ID, and exact body bytes without reposting:\n%s",
			rErr, continuationGuidance.unchanged)
	}
	fmt.Println(msg)
	return 0
}

func replyResolutionEvidenceFailure(
	threadID string,
	err error,
	guidance evidenceRecoveryGuidance,
) (string, bool) {
	if recovered, ok := errors.AsType[*accessDeniedMutationWithConfirmedStateError](err); ok {
		return fmt.Sprintf("the stale resolution is confirmed reopened, but this caller cannot claim the corrective mutation: %v\n"+
			"repair `gh` credentials before any later provider mutation; unchanged credentials will be denied again.\n"+
			"Inspect the fresh unresolved thread before selecting the next lifecycle:\n%s",
			recovered, guidance.inspect), true
	}
	if failedReopen, ok := errors.AsType[*failedReopenError](err); ok {
		if failedReopen.recovery == retryUnchangedEvidence {
			return fmt.Sprintf("resolution was applied without a confirmed evidence anchor, and reopening it failed: %v\n"+
				"%s"+
				"do NOT retry while the thread is still resolved. Reopen it first:\n"+
				"  resolve-review-threads unresolve %s\n"+
				"after the explicit reopen succeeds, inspect fresh provider state before selecting unchanged retry or revision:\n%s",
				err, accessRepairNote(err), threadID, guidance.inspect), true
		}
		prefix := "resolution was applied after the evidence changed, and reopening it failed"
		if guidance.replyWasPosted {
			prefix = "your reply is posted, but its exact resolution evidence changed and reopening the resolved thread failed"
		}
		return fmt.Sprintf("%s: %v%s\n"+
			"do NOT retry while the thread is still resolved. Reopen it first:\n"+
			"  resolve-review-threads unresolve %s\n"+
			"then inspect fresh provider state and use this single %s lifecycle:\n%s",
			prefix, err, accessRepairNote(err), threadID, guidance.answerKind, guidance.answer), true
	}

	if matchesErrorType[*supersededEvidenceError](err) {
		prefix := "the evidence changed and the thread now requires a new answer"
		if guidance.replyWasPosted {
			prefix = "your reply is posted, but a newer comment is confirmed and it does not answer that comment"
		}
		return fmt.Sprintf("%s:\n%v%s\nread the follow-up and use this %s lifecycle:\n%s",
			prefix, err, accessRepairNote(err), guidance.answerKind, guidance.answer), true
	}

	if matchesErrorType[*unverifiableResolutionError](err) {
		return fmt.Sprintf("the resolution response did not confirm its evidence anchor, so the automatic reopen restored a safe retry state:\n%v%s\n"+
			"no newer comment was confirmed; do not revise the reply source. Use this unchanged-operation lifecycle:\n%s",
			err, accessRepairNote(err), guidance.unchanged), true
	}

	if matchesErrorType[*unavailableAnswerEvidenceError](err) {
		authorGuidance := guidance.author
		if authorGuidance == "" {
			authorGuidance = guidance.inspect
		}
		return fmt.Sprintf("the thread's author evidence is insufficient to prove an independent answer; another reply would not repair that evidence:\n%v%s\n%s",
			err, accessRepairNote(err), authorGuidance), true
	}

	if matchesErrorType[*invalidatedReplyEvidenceError](err) {
		return fmt.Sprintf("the exact reply evidence changed, so no further resolution may rely on its old receipt:\n%v%s\n%s",
			err, accessRepairNote(err), guidance.inspect), true
	}

	if matchesErrorType[*unverifiedProviderStateError](err) {
		if isAccessDenied(err) {
			return fmt.Sprintf("provider state could not be verified because GitHub denied access:\n%v\n"+
				"repair `gh` credentials before inspecting or retrying; the same credentials will be denied again.\n%s",
				err, guidance.inspect), true
		}
		return fmt.Sprintf("provider state could not be verified, so no retry or cleanup is yet safe:\n%v\n%s",
			err, guidance.inspect), true
	}

	if matchesErrorType[*unansweredError](err) {
		if guidance.replyWasPosted {
			return fmt.Sprintf("the posted reply is still last, but the thread is not provably answered:\n%v\n%s",
				err, guidance.inspect), true
		}
		return fmt.Sprintf("resolution was refused because the thread still needs an answer:\n%v\n"+
			"use this %s lifecycle:\n%s", err, guidance.answerKind, guidance.answer), true
	}
	return "", false
}

func accessRepairNote(err error) string {
	if !isAccessDenied(err) {
		return ""
	}
	return "\nGitHub also denied the attempted provider mutation. Repair `gh` credentials before any later provider mutation; unchanged credentials will be denied again."
}

// replyMutationEvidence is the provider state returned atomically with a
// successful reply mutation. Later reads may validate it, but must never
// replace it with newly observed author, body, or edit-time evidence.
type replyMutationEvidence struct {
	ID                                   string
	Author                               string
	BodySHA256                           string
	UpdatedAt                            string
	EditCount                            int
	EditCountPresent                     bool
	LastEditID                           string
	LastEditIDPresent                    bool
	ObservedOpeningAuthor                string
	ObservedPredecessorTime              string
	ObservedPredecessorEditCount         int
	ObservedPredecessorEditCountPresent  bool
	ObservedPredecessorLastEditID        string
	ObservedPredecessorLastEditIDPresent bool
	Recovered                            bool
}

// postReply posts a reply and returns the new comment's exact mutation-time
// evidence. Its ID is the anchor for the whole safety argument; author, body,
// and update time prevent a later read from laundering changed evidence into
// a continuation receipt.
func postReply(ctx context.Context, threadID, body string) (replyMutationEvidence, error) {
	raw, err := ghGraphQL(ctx, replyMutation, map[string]any{
		"threadId": threadID,
		"body":     body,
	})
	if err != nil {
		return replyMutationEvidence{}, err
	}
	return parseReplyMutationEvidence(raw, body)
}

// postReplyOrExit posts a reply and reports whether the caller should return
// immediately. code is -1 only when id names a reply observed after the exact
// predecessor the caller read. Any ambiguous outcome is reconciled against
// that predecessor before recovery can call an unchanged retry safe.
func postReplyOrExit(
	ctx context.Context,
	threadID, body string,
	predecessor replyIssuancePredecessor,
	bodyFile string,
) (id string, code int) {
	evidence, code := postReplyEvidenceOrExit(
		ctx,
		threadID,
		body,
		predecessor,
		bodyFile,
	)
	return evidence.ID, code
}

func postReplyEvidenceOrExit(
	ctx context.Context,
	threadID, body string,
	predecessor replyIssuancePredecessor,
	bodyFile string,
) (evidence replyMutationEvidence, code int) {
	if err := predecessor.validateForReplyPost(body); err != nil {
		return replyMutationEvidence{}, fail("reply placement evidence for predecessor %s is incomplete: %v; no provider mutation was attempted\n%s",
			predecessor.ID, err, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	evidence, err := postReply(ctx, threadID, body)
	if err == nil {
		return evidence, -1
	}
	recovered := replyMutationEvidence{}
	id, code := reconcileReplyPostError(
		ctx,
		threadID,
		body,
		predecessor,
		bodyFile,
		err,
		&recovered,
	)
	if code >= 0 {
		return replyMutationEvidence{}, code
	}
	recovered.ID = id
	recovered.Recovered = true
	return recovered, -1
}

// parseReplyCommentID extracts the new comment's node ID from the reply
// mutation response. An absent ID is an error rather than an empty anchor:
// without it there is nothing to prove our reply is the last comment, and
// resolving on a blank anchor would defeat the check entirely.
func parseReplyCommentID(raw []byte) (string, error) {
	var resp struct {
		Data struct {
			AddPullRequestReviewThreadReply struct {
				Comment struct {
					ID string `json:"id"`
				} `json:"comment"`
			} `json:"addPullRequestReviewThreadReply"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse reply response: %w", err)
	}
	id := resp.Data.AddPullRequestReviewThreadReply.Comment.ID
	if id == "" {
		return "", errReplyIDMissing
	}
	return id, nil
}

func parseReplyMutationEvidence(raw []byte, expectedBody string) (replyMutationEvidence, error) {
	var resp struct {
		Data struct {
			AddPullRequestReviewThreadReply struct {
				Comment struct {
					ID               string                    `json:"id"`
					Body             *string                   `json:"body"`
					UpdatedAt        string                    `json:"updatedAt"`
					UserContentEdits *userContentEditsEvidence `json:"userContentEdits"`
					Author           struct {
						Login string `json:"login"`
					} `json:"author"`
				} `json:"comment"`
			} `json:"addPullRequestReviewThreadReply"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return replyMutationEvidence{}, fmt.Errorf("parse reply response: %w", err)
	}
	comment := resp.Data.AddPullRequestReviewThreadReply.Comment
	if comment.ID == "" {
		return replyMutationEvidence{}, errReplyIDMissing
	}
	if !validContinuationReceiptID(comment.ID) {
		return replyMutationEvidence{}, errors.New("reply response returned an invalid comment ID")
	}
	if comment.Body == nil {
		return replyMutationEvidence{}, errors.New("reply response omitted the exact comment body")
	}
	if *comment.Body != expectedBody {
		return replyMutationEvidence{}, errors.New("reply response returned comment bytes different from the submitted body")
	}
	if !validContinuationAuthor(comment.Author.Login) {
		return replyMutationEvidence{}, errors.New("reply response omitted a safe comment author")
	}
	if !validContinuationTimestamp(comment.UpdatedAt) {
		return replyMutationEvidence{}, errors.New("reply response omitted a valid comment update timestamp")
	}
	editCount, lastEditID, editRevisionPresent := observedEditRevision(comment.UserContentEdits)
	if !editRevisionPresent {
		return replyMutationEvidence{}, errors.New("reply response omitted a valid comment edit revision")
	}
	return replyMutationEvidence{
		ID:                comment.ID,
		Author:            comment.Author.Login,
		BodySHA256:        exactBodySHA256([]byte(*comment.Body)),
		UpdatedAt:         comment.UpdatedAt,
		EditCount:         editCount,
		EditCountPresent:  true,
		LastEditID:        lastEditID,
		LastEditIDPresent: true,
	}, nil
}

// errReplyIDMissing marks a nominally successful reply response that omitted
// the new comment's ID. It does not by itself prove placement or even that the
// comment is observable; postReplyOrExit must reconcile history against the
// original predecessor before selecting recovery.
var errReplyIDMissing = errors.New("reply response omitted the comment ID")

// cmdResolveAll resolves the unresolved threads on a PR that carry evidence of
// a response, optionally limited to a single opening author. Unanswered threads
// are refused by name: see the package comment for why resolution requires
// evidence. Exits non-zero when anything was refused, so a caller driving a PR
// to mergeable never mistakes a partial pass for reaching zero.
func cmdResolveAll(ctx context.Context, rest []string) int {
	force := false
	args := make([]string, 0, len(rest))
	for _, a := range rest {
		if a == "--force" {
			force = true
			continue
		}
		args = append(args, a)
	}
	if len(args) < 3 || len(args) > 4 {
		return fail("usage: resolve-all <owner> <repo> <pr> [author] [--force]")
	}
	pr, err := strconv.Atoi(args[2])
	if err != nil {
		return fail("pr must be an integer, got %q", args[2])
	}
	author := ""
	if len(args) == 4 {
		author = args[3]
	}
	threads, err := listThreads(ctx, args[0], args[1], pr)
	if err != nil {
		return fail("%v", err)
	}

	resolved, refused, skipped := 0, 0, 0
	for _, t := range filterThreads(threads, author) {
		// No pre-check on the listing snapshot: a reply may have arrived
		// since, and refusing on stale state would report "nobody replied"
		// about a thread that is currently answered. resolveWithEvidence
		// re-reads and is the single place that decides.
		msg, mutated, err := resolveWithEvidence(ctx, t.ID, force, resolutionEvidence{})
		if err != nil {
			var unanswered *unansweredError
			var superseded *supersededEvidenceError
			pureEvidenceRefusal := !isAccessDenied(err) &&
				(errors.As(err, &unanswered) ||
					(errors.As(err, &superseded) && superseded.cause == nil))
			if !pureEvidenceRefusal {
				if recovery, handled := replyResolutionEvidenceFailure(
					t.ID,
					err,
					newAnswerRecoveryGuidance(t.ID, force),
				); handled {
					return fail("aborting sweep after %d resolved, %d refused: %s", resolved, refused, recovery)
				}
				// Auth, permissions, or a GraphQL outage. Retrying it once per
				// remaining thread would just repeat a denied call and then
				// report the pile as "nobody replied".
				return fail("aborting sweep after %d resolved, %d refused: %v", resolved, refused, err)
			}
			// A thread that is unanswered or whose evidence was superseded
			// before any provider mutation failed is a refusal, not a fatal
			// provider error: keep sweeping the rest. A supersession carrying a
			// mutation/access cause must abort above so denied credentials are not
			// retried and transport failures are not counted as review refusals.
			refused++
			recovery, _ := replyResolutionEvidenceFailure(
				t.ID,
				err,
				newAnswerRecoveryGuidance(t.ID, force),
			)
			fmt.Fprintf(os.Stderr, "REFUSED %s\n", recovery)
			continue
		}
		fmt.Println(msg)
		if mutated {
			resolved++
		} else {
			skipped++
		}
	}

	fmt.Printf("resolved %d thread(s), refused %d, skipped %d\n", resolved, refused, skipped)
	if refused == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, `
%d thread(s) were refused because they still need a current answer. A thread
may have no independent reply yet, or its earlier answer may have been
superseded by newer commentary. Resolving asserts the current point was
handled; each REFUSED diagnostic above identifies the exact evidence failure
and includes its thread-specific reply-file lifecycle.

--force overrides the current-answer evidence requirement. Reserve it for
threads you are deliberately dismissing, and say so in a PR comment.
`, refused)
	return 1
}
