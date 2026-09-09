package main

import (
	"fmt"
	"strings"
)

const newReplyBodyGuidanceTemplate = `For this thread, create a task-owned reply file in the system temporary directory outside the repository or merge worktree:
  reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"
Write a thread-specific reason to "$reply_file", then run:
  resolve-review-threads reply-resolve <threadId> --body-file "$reply_file"
Whenever exact-body retry remains valid, retain this same file unchanged through the retry.
After terminal resolution is confirmed, remove it:
  rm -f -- "$reply_file"
Only then continue to the next thread or safe-merge.`

func newReplyBodyGuidance(threadID string) string {
	return strings.ReplaceAll(newReplyBodyGuidanceTemplate, "<threadId>", threadID)
}

const revisedNamedReplyBodyGuidanceTemplate = `For this thread, revise the same named body source in place to answer the new comments; do not create or choose another path.
Keep the original source identity:
  reply_file=<bodyFile>
Replace only its contents, then run:
  resolve-review-threads reply-resolve <threadId> --body-file "$reply_file"
Whenever exact-body retry remains valid, retain that same source unchanged through the retry.
After terminal resolution is confirmed, remove the source only if it is the task-owned temporary file created by generated guidance:
  rm -f -- "$reply_file"
Do not remove a user-owned source.
Only then continue to the next thread or safe-merge.`

const revisedStdinReplyBodyGuidanceTemplate = `For this thread, revise the retained standard-input bytes to answer the new comments; do not create or choose a named path merely for recovery.
Replay the revised retained bytes with:
  resolve-review-threads reply-resolve <threadId> --body-file -
Whenever exact-body retry remains valid, retain those same revised bytes unchanged through the retry.
Standard input has no named source to clean up.
Only after terminal resolution is confirmed may you continue to the next thread or safe-merge.`

func revisedReplyBodyGuidance(threadID, bodyFile string) string {
	template := revisedStdinReplyBodyGuidanceTemplate
	if bodyFile != "-" {
		template = strings.ReplaceAll(
			revisedNamedReplyBodyGuidanceTemplate,
			"<bodyFile>",
			shellQuoteArgument(bodyFile),
		)
	}
	return strings.ReplaceAll(template, "<threadId>", threadID)
}

const unchangedNamedReplyBodyGuidanceTemplate = `Keep the exact same named reply-body source and do not edit or replace it:
  reply_file=<bodyFile>
Retry with:
  resolve-review-threads reply-resolve <threadId> --body-file "$reply_file"
Retain that file unchanged through every applicable exact-body retry.
After terminal resolution is confirmed, remove it only if it is the task-owned temporary file created by generated guidance:
  rm -f -- "$reply_file"
Do not remove a user-owned source.
Only then continue to another thread or safe-merge.`

const unchangedStdinReplyBodyGuidanceTemplate = `Replay the exact same retained standard-input bytes without revising them:
  resolve-review-threads reply-resolve <threadId> --body-file -
Retain those bytes unchanged through every applicable exact-body retry.
Standard input has no named source to clean up.
Only after terminal resolution is confirmed may you continue to another thread or safe-merge.`

func unchangedReplyBodyGuidance(threadID, bodyFile string) string {
	template := unchangedStdinReplyBodyGuidanceTemplate
	if bodyFile != "-" {
		template = strings.ReplaceAll(
			unchangedNamedReplyBodyGuidanceTemplate,
			"<bodyFile>",
			shellQuoteArgument(bodyFile),
		)
	}
	return strings.ReplaceAll(template, "<threadId>", threadID)
}

const continuationNamedReplyBodyGuidanceTemplate = `Keep the exact same named reply-body source and continuation receipt; do not edit or replace either:
  continuation_receipt=<receipt>
  reply_file=<bodyFile>
Retry the resolve-only continuation with:
  resolve-review-threads continue-resolve "$continuation_receipt" --body-file "$reply_file"
This continuation validates the original predecessor ID and body, reply ID and exact body bytes, and cannot post another reply.
Retain that file unchanged together with the receipt through every applicable retry.
After terminal resolution is confirmed, remove the file only if it is the task-owned temporary file created by generated guidance:
  rm -f -- "$reply_file"
Do not remove a user-owned source.
Only then continue to another thread or safe-merge.`

const continuationStdinReplyBodyGuidanceTemplate = `Keep the exact same retained standard-input bytes and continuation receipt; do not revise either:
  continuation_receipt=<receipt>
Replay the exact retained bytes with the resolve-only continuation:
  resolve-review-threads continue-resolve "$continuation_receipt" --body-file -
This continuation validates the original predecessor ID and body, reply ID and exact body bytes, and cannot post another reply.
Retain the receipt and bytes through every applicable retry.
Standard input has no named source to clean up.
Only after terminal resolution is confirmed may you continue to another thread or safe-merge.`

func continuationReplyBodyGuidance(receiptToken, bodyFile string) string {
	template := strings.ReplaceAll(
		continuationStdinReplyBodyGuidanceTemplate,
		"<receipt>",
		shellQuoteArgument(receiptToken),
	)
	if bodyFile != "-" {
		template = strings.ReplaceAll(
			continuationNamedReplyBodyGuidanceTemplate,
			"<bodyFile>",
			shellQuoteArgument(bodyFile),
		)
		template = strings.ReplaceAll(template, "<receipt>", shellQuoteArgument(receiptToken))
	}
	return template
}

func inspectContinuationGuidance(threadID, receiptToken, bodyFile string) string {
	if bodyFile == "-" {
		return fmt.Sprintf(
			"Retain the exact standard-input bytes and continuation receipt unchanged while provider state is unverified:\n"+
				"  continuation_receipt=%s\n"+
				"Do not revise or discard the bytes, and do not continue to another thread or safe-merge.\n"+
				"Inspect the live thread before any retry. If the receipt's unchanged predecessor and reply bodies remain directly adjacent, "+
				"and the reply is still last, replay these bytes with continue-resolve. If a reviewer "+
				"has taken thread %s back, revise the retained bytes and start a fresh reply-resolve lifecycle.",
			shellQuoteArgument(receiptToken),
			threadID,
		)
	}
	return fmt.Sprintf(
		"Retain the exact named reply-body source unchanged and retain the continuation receipt while provider state is unverified:\n"+
			"  continuation_receipt=%s\n"+
			"  reply_file=%s\n"+
			"Do not edit or remove that source, and do not continue to another thread or safe-merge.\n"+
			"Inspect the live thread before any retry. If the receipt's unchanged predecessor and reply bodies remain directly adjacent, "+
			"and the reply is still last, retry with continue-resolve. If a reviewer has taken "+
			"thread %s back, revise this same source and start a fresh reply-resolve lifecycle.",
		shellQuoteArgument(receiptToken),
		shellQuoteArgument(bodyFile),
		threadID,
	)
}

func inspectContinuationAuthorGuidance(threadID, receiptToken, bodyFile string) string {
	source := "Retain the exact standard-input bytes unchanged."
	if bodyFile != "-" {
		source = fmt.Sprintf(
			"Retain the exact named reply-body source unchanged:\n  reply_file=%s",
			shellQuoteArgument(bodyFile),
		)
	}
	return fmt.Sprintf(
		"%s\nRetain the continuation receipt:\n  continuation_receipt=%s\n"+
			"Do not retry continue-resolve while the opening and current reply authors for thread %s are missing, equal, or inconsistent across fresh reads; unchanged IDs and bodies do not repair that identity boundary. "+
			"Inspect the live thread before any retry and repair provider-visible author evidence before any resolution attempt. Do not post another reply, discard the source or receipt, clean up, or safe-merge on this evidence.",
		source,
		shellQuoteArgument(receiptToken),
		threadID,
	)
}

func inspectContinuationMismatchGuidance(threadID, receiptToken, bodyFile string) string {
	source := "Retain the current standard-input bytes for inspection; do not discard them while the mismatch is unresolved."
	if bodyFile != "-" {
		source = fmt.Sprintf(
			"Retain the current named source for inspection; do not remove it while the mismatch is unresolved:\n  reply_file=%s",
			shellQuoteArgument(bodyFile),
		)
	}
	return fmt.Sprintf(
		"%s\nRetain the continuation receipt:\n  continuation_receipt=%s\n"+
			"Do not mutate provider state, clean up, or safe-merge on this evidence. Inspect thread %s. "+
			"Restore the exact original bytes only if this receipt is still the intended continuation; "+
			"otherwise keep or revise the source for a fresh reply-resolve after confirmed reviewer hand-back.",
		source,
		shellQuoteArgument(receiptToken),
		threadID,
	)
}

func invalidContinuationReceiptGuidance(bodyFile string) string {
	if bodyFile == "-" {
		return "Retain the intended exact standard-input bytes for inspection or revision; " +
			"do not discard them, mutate provider state, clean up, or safe-merge on an invalid receipt."
	}
	return fmt.Sprintf(
		"Retain the selected named reply-body source for inspection or revision; do not edit or remove it, "+
			"mutate provider state, clean up, or safe-merge on an invalid receipt:\n  reply_file=%s",
		shellQuoteArgument(bodyFile),
	)
}

func continuationIssuerStateRecoveryGuidance(receiptToken, bodyFile string) string {
	source := "Retain the exact intended standard-input bytes; do not discard or revise them while issuer state is being restored."
	if bodyFile != "-" {
		source = fmt.Sprintf(
			"Retain the exact selected named reply-body source; do not edit or remove it while issuer state is being restored:\n  reply_file=%s",
			shellQuoteArgument(bodyFile),
		)
	}
	return fmt.Sprintf(
		"%s\nRetain the continuation receipt exactly as issued:\n  continuation_receipt=%s\n"+
			"Restore the exact issuing GH_HOST, XDG_STATE_HOME, and signing key on a supported Unix environment before retrying continue-resolve. "+
			"Do not regenerate or overwrite the key: a replacement cannot recreate issuance authority and can strand every outstanding receipt. "+
			"If the exact issuer state and token still do not authenticate, inspect the live thread and treat the token as suspect. "+
			"Do not read the source into a new command, mutate provider state, clean up, or safe-merge until this boundary is restored or inspected.",
		source,
		shellQuoteArgument(receiptToken),
	)
}

func stableDifferentAnswerGuidance(threadID, bodyFile string) string {
	source := "Retain the current standard-input bytes while you inspect the provider-visible answer."
	if bodyFile != "-" {
		source = fmt.Sprintf(
			"Retain the current named reply-body source while you inspect the provider-visible answer:\n  reply_file=%s",
			shellQuoteArgument(bodyFile),
		)
	}
	return fmt.Sprintf(
		"%s\nDo not retry ordinary reply-resolve or clean up yet. Inspect thread %s. "+
			"If this source was changed from the already-posted answer, restore the exact original bytes and "+
			"use their retained continuation receipt. Current adjacency cannot recreate that temporal evidence "+
			"or license a fresh receipt. If no receipt was retained, keep the source and inspect the live thread; "+
			"do not ask ordinary reply-resolve to adopt the existing answer. If a reviewer has "+
			"taken the thread back, revise this same source to answer that hand-back and start a fresh "+
			"reply-resolve lifecycle. Otherwise, do not stack a second independent answer or safe-merge.",
		source,
		threadID,
	)
}

func unreceiptedExistingReplyGuidance(threadID, bodyFile string) string {
	source := "Retain the exact standard-input bytes and any receipt emitted by the original posting attempt."
	if bodyFile != "-" {
		source = fmt.Sprintf(
			"Retain the exact named reply-body source and any receipt emitted by the original posting attempt:\n  reply_file=%s",
			shellQuoteArgument(bodyFile),
		)
	}
	return fmt.Sprintf(
		"%s\nA provider-visible reply and predecessor observed only now do not prove their temporal pairing, cannot be rebound, and cannot be freshly receipted. "+
			"The retained receipt is required for continue-resolve with these exact bytes. If it does not exist, inspect thread %s; "+
			"do not rerun ordinary reply-resolve, resolve directly, clean up, or safe-merge on this evidence. "+
			"Only a confirmed reviewer hand-back permits revising this same source and beginning a fresh reply-resolve lifecycle.",
		source,
		threadID,
	)
}

func resolvedReviewerHandbackGuidance(threadID, bodyFile string) string {
	return fmt.Sprintf(
		"Inspect the live reviewer hand-back before treating the existing resolution as terminal. If it still needs this answer, first reopen the thread with:\n"+
			"  resolve-review-threads unresolve %s\n"+
			"After that reopen is confirmed, use this same-source revised-answer lifecycle:\n%s",
		threadID,
		revisedReplyBodyGuidance(threadID, bodyFile),
	)
}

func resolvedSupersededReplyGuidance(threadID, bodyFile string) string {
	return fmt.Sprintf(
		"The newer commentary is still hidden behind an existing resolution. First reopen the thread with:\n"+
			"  resolve-review-threads unresolve %s\n"+
			"After that reopen is confirmed, use this same-source revised-answer lifecycle:\n%s",
		threadID,
		revisedReplyBodyGuidance(threadID, bodyFile),
	)
}

func unavailableAuthorEvidenceGuidance(threadID, bodyFile string) string {
	source := "Retain the selected standard-input bytes unchanged while you inspect the live thread."
	if bodyFile != "-" {
		source = fmt.Sprintf(
			"Retain the selected named reply-body source unchanged while you inspect the live thread:\n  reply_file=%s",
			shellQuoteArgument(bodyFile),
		)
	}
	return fmt.Sprintf(
		"%s\nThread %s has provider author identity that is missing or inconsistent across stable reads. Do not post, resolve, or use an "+
			"exact-body retry while that identity boundary is unavailable; identical text cannot prove who "+
			"answered. Inspect or repair the provider-visible identity evidence, then begin a fresh "+
			"reply-resolve decision from current history. Do not clean up or safe-merge on this evidence.",
		source,
		threadID,
	)
}

func accessDeniedReplyGuidance(threadID, bodyFile string) string {
	return fmt.Sprintf(
		"This is an access problem: run `gh auth status` and fix credentials before retrying; "+
			"re-running with the same credentials will be denied again. Retain the exact reply-body "+
			"source unchanged. After credential repair, use this unchanged-source lifecycle:\n%s",
		unchangedReplyBodyGuidance(threadID, bodyFile),
	)
}

func deniedObservedReplyGuidance(threadID, bodyFile string) string {
	source := "Retain the exact standard-input bytes while the independently observed reply is inspected."
	if bodyFile != "-" {
		source = fmt.Sprintf(
			"Retain the exact named reply-body source while the independently observed reply is inspected:\n  reply_file=%s",
			shellQuoteArgument(bodyFile),
		)
	}
	return fmt.Sprintf(
		"%s\nRepair `gh` credentials before any later provider mutation; unchanged credentials will be denied again. "+
			"Inspect thread %s and determine which actor or retained receipt owns the visible reply. Do not rerun ordinary reply-resolve, "+
			"mint a receipt from current adjacency, resolve directly, clean up, or safe-merge on this evidence.",
		source,
		threadID,
	)
}

func providerReadRecoveryGuidance(err error, inspect string) string {
	if !isAccessDenied(err) {
		return inspect
	}
	return "GitHub denied the provider-state read. Repair `gh` credentials before inspection or retry; " +
		"unchanged credentials will be denied again.\n" + inspect
}

func replyPostReadRecoveryGuidance(postErr, readErr error, inspect string) string {
	guidance := providerReadRecoveryGuidance(readErr, inspect)
	if !isAccessDenied(postErr) {
		return guidance
	}
	return "GitHub denied the original reply mutation. Repair `gh` credentials before inspection or retry; " +
		"unchanged credentials will be denied again.\n" + guidance
}

const inspectNamedReplyOutcomeGuidanceTemplate = `Retain the exact named reply-body source unchanged while provider state is unverified:
  reply_file=<bodyFile>
Do not edit or remove it, and do not continue to another thread or safe-merge.
Inspect the live thread before any retry. If the attempted reply is present, verify that it directly follows the comment originally read and that no newer comment follows it. If the tail moved without that reply, revise this same source in place. If the original tail is still last and the reply is absent, an exact-body retry may be applicable. If the thread is resolved on stale or unverifiable evidence, unresolve it before answering or retrying.`

const inspectStdinReplyOutcomeGuidanceTemplate = `Retain the exact standard-input bytes while provider state is unverified.
Do not revise or discard them, and do not continue to another thread or safe-merge.
Inspect the live thread before any retry. If the attempted reply is present, verify that it directly follows the comment originally read and that no newer comment follows it. If the tail moved without that reply, prepare revised bytes for the same standard-input form. If the original tail is still last and the reply is absent, an exact-body retry may be applicable. If the thread is resolved on stale or unverifiable evidence, unresolve it before answering or retrying.`

func inspectReplyOutcomeGuidance(threadID, bodyFile string) string {
	template := inspectStdinReplyOutcomeGuidanceTemplate
	if bodyFile != "-" {
		template = strings.ReplaceAll(
			inspectNamedReplyOutcomeGuidanceTemplate,
			"<bodyFile>",
			shellQuoteArgument(bodyFile),
		)
	}
	return strings.ReplaceAll(template, "<threadId>", threadID)
}

func bareResolutionRetryGuidance(threadID string, force bool) string {
	forceArg := ""
	if force {
		forceArg = " --force"
	}
	return fmt.Sprintf(
		"Retry the unchanged resolution operation with:\n  resolve-review-threads resolve %s%s",
		threadID,
		forceArg,
	)
}

func bareResolutionInspectionGuidance(threadID string) string {
	return fmt.Sprintf("Inspect thread %s before retrying, continuing a sweep, or merging; do not treat unverified provider state as a completed resolution", threadID)
}

type evidenceRecoveryGuidance struct {
	answerKind     string
	answer         string
	unchanged      string
	inspect        string
	author         string
	replyWasPosted bool
}

func newAnswerRecoveryGuidance(threadID string, force bool) evidenceRecoveryGuidance {
	return evidenceRecoveryGuidance{
		answerKind: "first-answer",
		answer:     newReplyBodyGuidance(threadID),
		unchanged:  bareResolutionRetryGuidance(threadID, force),
		inspect:    bareResolutionInspectionGuidance(threadID),
	}
}

func revisedAnswerRecoveryGuidance(threadID, bodyFile string, replyWasPosted bool) evidenceRecoveryGuidance {
	return evidenceRecoveryGuidance{
		answerKind:     "revised-answer",
		answer:         revisedReplyBodyGuidance(threadID, bodyFile),
		unchanged:      unchangedReplyBodyGuidance(threadID, bodyFile),
		inspect:        inspectReplyOutcomeGuidance(threadID, bodyFile),
		replyWasPosted: replyWasPosted,
	}
}

func continuationRecoveryGuidance(threadID, receiptToken, bodyFile string) evidenceRecoveryGuidance {
	return evidenceRecoveryGuidance{
		answerKind:     "revised-answer",
		answer:         revisedReplyBodyGuidance(threadID, bodyFile),
		unchanged:      continuationReplyBodyGuidance(receiptToken, bodyFile),
		inspect:        inspectContinuationGuidance(threadID, receiptToken, bodyFile),
		author:         inspectContinuationAuthorGuidance(threadID, receiptToken, bodyFile),
		replyWasPosted: true,
	}
}

func shellQuoteArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
