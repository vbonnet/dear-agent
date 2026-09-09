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

func accessDeniedReplyGuidance(threadID, bodyFile string) string {
	return fmt.Sprintf(
		"This is an access problem: run `gh auth status` and fix credentials before retrying; "+
			"re-running with the same credentials will be denied again. Retain the exact reply-body "+
			"source unchanged. After credential repair, use this unchanged-source lifecycle:\n%s",
		unchangedReplyBodyGuidance(threadID, bodyFile),
	)
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

func shellQuoteArgument(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
