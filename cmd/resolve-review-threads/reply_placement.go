// Exact reply placement validation and recovery classification.
package main

import (
	"context"
	"fmt"
)

// replyPlacement describes where our reply ended up relative to what we read.
type replyPlacement int

const (
	// replyExact means our reply is the last comment AND it directly follows
	// the comment we had read: nobody spoke on either side of it.
	replyExact replyPlacement = iota
	// replyBuried means something was said after our reply.
	replyBuried
	// replyJumped means something was said between our read and our reply, so
	// our reply sits on top of a comment we never saw.
	replyJumped
)

// checkReplyPlacement is the ID-level safety argument as a pure function.
// wantLast proves nothing came after our reply; wantPrev proves nothing
// arrived between our read and post. Exact production placement additionally
// binds both bodies in verifyExactReplyPlacement.
func checkReplyPlacement(gotPrev, gotLast, wantPrev, wantLast string) replyPlacement {
	if gotLast != wantLast {
		return replyBuried
	}
	if gotPrev != wantPrev {
		return replyJumped
	}
	return replyExact
}

// verifyReplyPlacement retains the ID-only seam used by focused placement
// tests. Production reply-resolve uses verifyExactReplyPlacement.
func verifyReplyPlacement(ctx context.Context, threadID, wantPrevID, wantLastID, bodyFile string) int {
	_, code := verifyReplyPlacementAgainst(ctx, threadID, resolutionEvidence{
		LastID:        wantLastID,
		PredecessorID: wantPrevID,
	}, bodyFile)
	return code
}

// verifyExactReplyPlacement binds both adjacent comment IDs and both exact
// bodies. GitHub permits review-comment edits without changing node IDs, so
// an ID-only placement check cannot prove what the reply actually answered.
func verifyExactReplyPlacement(
	ctx context.Context,
	threadID string,
	evidence resolutionEvidence,
	bodyFile string,
) int {
	_, code := verifyExactReplyPlacementState(ctx, threadID, evidence, bodyFile)
	return code
}

func verifyExactReplyPlacementState(
	ctx context.Context,
	threadID string,
	evidence resolutionEvidence,
	bodyFile string,
) (thread, int) {
	if err := evidence.validate(); err != nil {
		return thread{}, fail("the reply is posted, but its exact placement evidence is invalid; resolution was not attempted: %v\n%s",
			err, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if !evidence.exactReply() {
		return thread{}, fail("the reply is posted, but its exact placement evidence is incomplete; resolution was not attempted\n%s",
			inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	return verifyReplyPlacementAgainst(ctx, threadID, evidence, bodyFile)
}

func verifyReplyPlacementAgainst(
	ctx context.Context,
	threadID string,
	evidence resolutionEvidence,
	bodyFile string,
) (thread, int) {
	after, err := fetchThread(ctx, threadID)
	if err != nil {
		return thread{}, fail("your reply may be posted, but placement and current resolved state could not be re-read; this command did not attempt resolution: %v\n%s",
			err, providerReadRecoveryGuidance(err, inspectReplyOutcomeGuidance(threadID, bodyFile)))
	}
	placement := checkReplyPlacement(after.PrevID, after.LastID, evidence.PredecessorID, evidence.LastID)
	if after.LastID == "" || (after.LastID == evidence.LastID && after.PrevID == "") {
		message, _ := replyResolutionEvidenceFailure(
			threadID,
			&unverifiedProviderStateError{msg: "reply placement response omitted the comment IDs needed to distinguish a moved tail from incomplete evidence"},
			revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
		)
		return after, fail("%s", message)
	}
	switch placement {
	case replyBuried:
		err := replyPlacementEvidenceError(
			ctx,
			threadID,
			after,
			"the posted reply is not last because comment "+after.LastID+" follows it",
		)
		message, _ := replyResolutionEvidenceFailure(
			threadID,
			err,
			revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
		)
		return after, fail("%s", message)
	case replyJumped:
		err := replyPlacementEvidenceError(
			ctx,
			threadID,
			after,
			"comment "+after.PrevID+" arrived between the original read and the posted reply",
		)
		message, _ := replyResolutionEvidenceFailure(
			threadID,
			err,
			revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
		)
		return after, fail("%s", message)
	case replyExact:
		if !evidence.exactReply() {
			return after, 0
		}
		if !after.PrevBodyPresent || !after.LastBodyPresent {
			message, _ := replyResolutionEvidenceFailure(
				threadID,
				&unverifiedProviderStateError{msg: "reply placement response omitted the predecessor or reply body needed for exact evidence"},
				revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
			)
			return after, fail("%s", message)
		}
		if exactBodySHA256([]byte(after.PrevBody)) != evidence.PredecessorBodySHA256 ||
			exactBodySHA256([]byte(after.LastBody)) != evidence.BodySHA256 {
			err := invalidatedReplyAnchorError(
				ctx,
				threadID,
				after,
				"the predecessor or reply body changed without changing its comment ID during reply placement",
			)
			message, _ := replyResolutionEvidenceFailure(
				threadID,
				err,
				revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
			)
			return after, fail("%s", message)
		}
		return after, 0
	}
	return after, 0
}

func replyPlacementEvidenceError(
	ctx context.Context,
	threadID string,
	after thread,
	detail string,
) error {
	state := "the thread remains unresolved"
	if after.IsResolved {
		reason := "resolved despite stale reply placement: " + detail
		if err := reopenOrFail(ctx, threadID, after.Path, reason, answerChangedEvidence); err != nil {
			return err
		}
		state = "the concurrent resolution was automatically reopened and that postcondition was confirmed"
	}
	return &supersededEvidenceError{msg: fmt.Sprintf(
		"%s %s: %s; %s",
		threadID,
		after.Path,
		detail,
		state,
	)}
}
