// Initial exact-target to full-history mismatch reconciliation.
package main

import (
	"context"
	"fmt"
	"strings"
)

func rejectInitialHistoryMismatch(
	ctx context.Context,
	threadID, bodyFile string,
	initial thread,
	boundary replyHistoryBoundary,
	detail string,
) int {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return fail("provider state is unverified after the initial and full-history evidence diverged (%s); nothing was posted or resolved: %v\n%s",
			detail, err, providerReadRecoveryGuidance(err, inspectReplyOutcomeGuidance(threadID, bodyFile)))
	}
	if initial.IsResolved {
		return fail("thread %s was already resolved before this command, and %s; the stable exact-state read was non-mutating.\n%s",
			threadID, detail, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}

	state := "the fresh exact-state read confirms the thread remains unresolved"
	if cur.IsResolved {
		reason := fmt.Sprintf("resolved after initial and full-history evidence diverged: %s", detail)
		if reopenErr := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); reopenErr != nil {
			message, _ := replyResolutionEvidenceFailure(
				threadID,
				reopenErr,
				revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
			)
			return fail("%s", message)
		}
		state = "the concurrent resolution was automatically reopened and that safe state was confirmed"
	}
	if cur.LastID == "" && !cur.IsResolved {
		return fail("provider state is unverified after the initial and full-history evidence diverged (%s); the fresh unresolved response omitted the current tail ID, so no mutation was attempted.\n%s",
			detail, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if currentDetail := boundary.mismatch(cur); !cur.IsResolved && strings.Contains(currentDetail, "omitted") {
		return fail("provider state is unverified after the initial and full-history evidence diverged (%s); the fresh unresolved response %s, so no mutation was attempted.\n%s",
			detail, currentDetail, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	return fail("thread %s changed after the initial exact-target read (%s); nothing was posted or resolved, and %s. Read the current comment and use this same-source revised-answer lifecycle:\n%s",
		threadID,
		detail,
		state,
		revisedReplyBodyGuidance(threadID, bodyFile),
	)
}
