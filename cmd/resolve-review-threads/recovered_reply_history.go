// Full-history validation for replies observed after an ambiguous post.
package main

import (
	"context"
	"fmt"
)

func recoveredReplyHistoryMismatch(
	history []tailComment,
	predecessorID, predecessorBodySHA256, replyID string,
) string {
	predecessorIndex, replyIndex := -1, -1
	for i, comment := range history {
		if predecessorIndex < 0 && comment.ID == predecessorID {
			predecessorIndex = i
		}
		if comment.ID == replyID {
			replyIndex = i
		}
	}
	if predecessorIndex < 0 || replyIndex < 0 {
		return "full history omitted a named predecessor or recovered reply"
	}
	if exactBodySHA256([]byte(history[predecessorIndex].Body)) != predecessorBodySHA256 {
		return fmt.Sprintf("predecessor %s changed its exact body before the recovered reply", predecessorID)
	}
	if replyIndex != predecessorIndex+1 && replyIndex != len(history)-1 {
		return fmt.Sprintf("recovered reply %s jumped over another comment and is followed by newer commentary", replyID)
	}
	if replyIndex != predecessorIndex+1 {
		return fmt.Sprintf("recovered reply %s jumped over a comment that followed predecessor %s", replyID, predecessorID)
	}
	if replyIndex != len(history)-1 {
		return fmt.Sprintf("recovered reply %s is followed by newer commentary", replyID)
	}
	return ""
}

func rejectChangedRecoveredReplyHistory(
	ctx context.Context,
	threadID, bodyFile, replyID, detail string,
	postErr error,
) (string, int) {
	cur, readErr := fetchThread(ctx, threadID)
	if readErr != nil {
		return "", fail("reply outcome remains unverified: full history found byte-exact reply %s but proved invalid placement (%s), and exact current state could not be read with a nonempty tail ID: %v (state read: %v)%s\n%s",
			replyID,
			detail,
			postErr,
			readErr,
			accessRepairNote(postErr),
			replyPostReadRecoveryGuidance(postErr, readErr, inspectReplyOutcomeGuidance(threadID, bodyFile)),
		)
	}
	reopened := false
	if cur.IsResolved {
		reason := "resolved after full history proved invalid recovered-reply placement: " + detail
		if reopenErr := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); reopenErr != nil {
			message, _ := replyResolutionEvidenceFailure(
				threadID,
				reopenErr,
				revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
			)
			return "", fail("%s%s", message, accessRepairNote(postErr))
		}
		reopened = true
	}
	if cur.LastID == "" && !reopened {
		return "", fail("reply outcome remains unverified: full history found byte-exact reply %s but proved invalid placement (%s), and the fresh unresolved state omitted its current tail ID: %v%s\n%s",
			replyID,
			detail,
			postErr,
			accessRepairNote(postErr),
			replyPostReadRecoveryGuidance(postErr, nil, inspectReplyOutcomeGuidance(threadID, bodyFile)),
		)
	}
	attribution := ""
	if isAccessDenied(postErr) {
		attribution = "GitHub denied this caller's reply mutation; the independently posted byte-exact reply cannot be attributed to this caller or used to mint a fresh continuation receipt. "
	}
	return "", fail("%s%sfull history found byte-exact reply %s, but it cannot be recovered because %s; no reply was adopted, receipted, or resolved: %v%s\nRead the changed thread and use this same-source revised-answer lifecycle:\n%s",
		confirmedReopenPrefix(reopened),
		attribution,
		replyID,
		detail,
		postErr,
		accessRepairNote(postErr),
		revisedReplyBodyGuidance(threadID, bodyFile),
	)
}
