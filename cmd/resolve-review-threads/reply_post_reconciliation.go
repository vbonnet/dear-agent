// Ambiguous reply-post reconciliation and retry classification.
package main

import (
	"context"
	"fmt"
)

func reconcileReplyPostError(
	ctx context.Context,
	threadID, body string,
	predecessor replyIssuancePredecessor,
	bodyFile string,
	postErr error,
	observed *replyMutationEvidence,
) (string, int) {
	originalTailID := predecessor.ID
	originalTailBodySHA256 := predecessor.BodySHA256
	// A missing response ID and a client-side transport error are both
	// ambiguous until history is re-read. The mutation can have applied before
	// either signal reached this process. Recover only a byte-exact matching
	// body observed after the original predecessor.
	history, historyErr := fetchAllComments(ctx, threadID)
	if historyErr != nil {
		return "", fail("provider state is unverified: reply outcome is ambiguous and current history could not be re-read: %v (history read: %v)\n%s",
			postErr, historyErr, replyPostReadRecoveryGuidance(
				postErr,
				historyErr,
				inspectReplyOutcomeGuidance(threadID, bodyFile),
			))
	}
	foundID, predecessorFound := findReplyIDAfter(history, originalTailID, body)
	if !predecessorFound {
		return reconcileMissingAmbiguousPredecessor(
			ctx,
			threadID,
			bodyFile,
			originalTailID,
			originalTailBodySHA256,
			body,
			postErr,
		)
	}
	if foundID != "" {
		if detail := recoveredReplyHistoryMismatch(
			history,
			originalTailID,
			originalTailBodySHA256,
			foundID,
		); detail != "" {
			return rejectChangedRecoveredReplyHistory(
				ctx,
				threadID,
				bodyFile,
				foundID,
				detail,
				postErr,
			)
		}
		if isAccessDenied(postErr) {
			return rejectDeniedObservedReply(
				ctx,
				threadID,
				bodyFile,
				originalTailID,
				originalTailBodySHA256,
				foundID,
				body,
				postErr,
				nil,
			)
		}
		*observed = replyEvidenceObservedInHistory(history, originalTailID, foundID)
		return foundID, -1
	}

	cur, code := readAmbiguousReplyState(ctx, threadID, bodyFile, postErr)
	if code >= 0 {
		return "", code
	}
	return classifyAbsentReplyOutcome(
		ctx,
		threadID,
		bodyFile,
		predecessor,
		body,
		history,
		cur,
		postErr,
		observed,
	)
}

func readAmbiguousReplyState(
	ctx context.Context,
	threadID, bodyFile string,
	postErr error,
) (thread, int) {
	cur, stateErr := fetchThread(ctx, threadID)
	if stateErr != nil || cur.LastID == "" {
		return cur, fail("reply was not observed, but current thread state could not be read with a nonempty tail ID after the ambiguous outcome: %v (state read: %v)\n%s",
			postErr, stateErr, replyPostReadRecoveryGuidance(
				postErr,
				stateErr,
				inspectReplyOutcomeGuidance(threadID, bodyFile),
			))
	}
	return cur, -1
}

func classifyAbsentReplyOutcome(
	ctx context.Context,
	threadID, bodyFile string,
	predecessor replyIssuancePredecessor,
	replyBody string,
	history []tailComment,
	cur thread,
	postErr error,
	observed *replyMutationEvidence,
) (string, int) {
	originalTailID := predecessor.ID
	originalTailBodySHA256 := predecessor.BodySHA256
	if currentStateShowsAttemptedReply(cur, originalTailID, originalTailBodySHA256, replyBody) {
		if isAccessDenied(postErr) {
			return rejectDeniedObservedReply(
				ctx,
				threadID,
				bodyFile,
				originalTailID,
				originalTailBodySHA256,
				cur.LastID,
				replyBody,
				postErr,
				&cur,
			)
		}
		*observed = replyMutationEvidence{
			ID:                                   cur.LastID,
			Author:                               cur.LastAuthor,
			BodySHA256:                           exactBodySHA256([]byte(cur.LastBody)),
			UpdatedAt:                            cur.LastUpdatedAt,
			EditCount:                            cur.LastEditCount,
			EditCountPresent:                     cur.LastEditCountPresent,
			LastEditID:                           cur.LastEditID,
			LastEditIDPresent:                    cur.LastEditIDPresent,
			ObservedOpeningAuthor:                cur.Author,
			ObservedPredecessorTime:              cur.PrevUpdatedAt,
			ObservedPredecessorEditCount:         cur.PrevEditCount,
			ObservedPredecessorEditCountPresent:  cur.PrevEditCountPresent,
			ObservedPredecessorLastEditID:        cur.PrevEditID,
			ObservedPredecessorLastEditIDPresent: cur.PrevEditIDPresent,
			Recovered:                            true,
		}
		return cur.LastID, -1
	}
	if cur.LastID != originalTailID {
		return rejectAbsentReplyAfterMovedTail(
			ctx,
			threadID,
			bodyFile,
			originalTailID,
			cur,
			postErr,
		)
	}
	historyDetail, historyIncomplete := compareRecoveredPredecessor(
		predecessor,
		predecessorObservationFromHistory(history),
		"full-history recovery read",
	)
	currentDetail, currentIncomplete := compareRecoveredPredecessor(
		predecessor,
		predecessorObservationFromThread(cur),
		"exact-state recovery read",
	)
	if historyDetail == "" && currentDetail == "" {
		return rejectAbsentReplyAtUnchangedBoundary(threadID, bodyFile, originalTailID, cur, postErr)
	}
	changedDetail := firstConclusivePredecessorChange(
		historyDetail,
		historyIncomplete,
		currentDetail,
		currentIncomplete,
	)
	if changedDetail != "" && cur.LastID != originalTailID {
		return rejectAbsentReplyAfterMovedTail(
			ctx,
			threadID,
			bodyFile,
			originalTailID,
			cur,
			postErr,
		)
	}
	if changedDetail != "" && cur.LastID == originalTailID && cur.LastBodyPresent &&
		exactBodySHA256([]byte(cur.LastBody)) != originalTailBodySHA256 {
		return rejectAbsentReplyAfterSameIDEdit(
			ctx,
			threadID,
			bodyFile,
			originalTailID,
			cur,
			postErr,
		)
	}
	if changedDetail != "" {
		return rejectAbsentReplyAfterChangedPredecessor(
			ctx,
			threadID,
			bodyFile,
			cur,
			changedDetail,
			postErr,
		)
	}
	incompleteDetail := historyDetail
	if incompleteDetail == "" {
		incompleteDetail = currentDetail
	}
	return "", fail("provider state is unverified: reply was not observed, but %s; both recovery reads must reproduce the full pre-post predecessor snapshot before an unchanged retry is safe: %v%s\n%s",
		incompleteDetail, postErr, accessRepairNote(postErr), inspectReplyOutcomeGuidance(threadID, bodyFile))
}

type recoveredPredecessorObservation struct {
	ID                string
	BodySHA256        string
	BodyPresent       bool
	UpdatedAt         string
	EditCount         int
	EditCountPresent  bool
	LastEditID        string
	LastEditIDPresent bool
	OpeningAuthor     string
	Author            string
}

func predecessorObservationFromHistory(history []tailComment) recoveredPredecessorObservation {
	if len(history) == 0 {
		return recoveredPredecessorObservation{}
	}
	tail := history[len(history)-1]
	return recoveredPredecessorObservation{
		ID:                tail.ID,
		BodySHA256:        exactBodySHA256([]byte(tail.Body)),
		BodyPresent:       true,
		UpdatedAt:         tail.UpdatedAt,
		EditCount:         tail.EditCount,
		EditCountPresent:  tail.EditCountPresent,
		LastEditID:        tail.LastEditID,
		LastEditIDPresent: tail.LastEditIDPresent,
		OpeningAuthor:     history[0].Login,
		Author:            tail.Login,
	}
}

func predecessorObservationFromThread(cur thread) recoveredPredecessorObservation {
	openingAuthor := cur.Author
	if openingAuthor == "unknown" {
		openingAuthor = ""
	}
	tailAuthor := cur.LastAuthor
	if tailAuthor == "unknown" {
		tailAuthor = ""
	}
	return recoveredPredecessorObservation{
		ID:                cur.LastID,
		BodySHA256:        exactBodySHA256([]byte(cur.LastBody)),
		BodyPresent:       cur.LastBodyPresent,
		UpdatedAt:         cur.LastUpdatedAt,
		EditCount:         cur.LastEditCount,
		EditCountPresent:  cur.LastEditCountPresent,
		LastEditID:        cur.LastEditID,
		LastEditIDPresent: cur.LastEditIDPresent,
		OpeningAuthor:     openingAuthor,
		Author:            tailAuthor,
	}
}

// compareRecoveredPredecessor treats missing fields differently from changed
// fields. A complete change proves the retained reply source is stale; an
// omission proves only that an unchanged retry is unsafe.
func compareRecoveredPredecessor(
	want replyIssuancePredecessor,
	got recoveredPredecessorObservation,
	readName string,
) (detail string, incomplete bool) {
	switch {
	case got.ID == "":
		return readName + " omitted the predecessor ID", true
	case got.ID != want.ID:
		return fmt.Sprintf("%s observed a different tail after predecessor %s", readName, want.ID), false
	case !got.BodyPresent:
		return readName + " omitted the predecessor body", true
	case got.BodySHA256 != want.BodySHA256:
		return fmt.Sprintf("%s proved predecessor %s changed its exact body", readName, want.ID), false
	case got.UpdatedAt == "":
		return readName + " omitted the predecessor update timestamp", true
	case got.UpdatedAt != want.UpdatedAt:
		return fmt.Sprintf("%s proved predecessor %s changed its update timestamp", readName, want.ID), false
	case !got.EditCountPresent:
		return readName + " omitted the predecessor edit generation", true
	case !got.LastEditIDPresent:
		return readName + " omitted the predecessor latest-edit ID", true
	case got.EditCount != want.EditCount || got.LastEditID != want.LastEditID:
		return fmt.Sprintf("%s proved predecessor %s changed its edit revision", readName, want.ID), false
	case got.OpeningAuthor == "":
		return readName + " omitted the stable opening author", true
	case got.OpeningAuthor != want.OpeningAuthor:
		return readName + " proved the opening author changed", false
	case got.Author == "":
		return readName + " omitted the stable tail author", true
	case got.Author != want.Author:
		return readName + " proved the predecessor tail author changed", false
	default:
		return "", false
	}
}

func firstConclusivePredecessorChange(
	historyDetail string,
	historyIncomplete bool,
	currentDetail string,
	currentIncomplete bool,
) string {
	if historyDetail != "" && !historyIncomplete {
		return historyDetail
	}
	if currentDetail != "" && !currentIncomplete {
		return currentDetail
	}
	return ""
}

func replyEvidenceObservedInHistory(
	history []tailComment,
	predecessorID, replyID string,
) replyMutationEvidence {
	if len(history) == 0 {
		return replyMutationEvidence{ID: replyID, Recovered: true}
	}
	observation := replyMutationEvidence{
		ID:                    replyID,
		ObservedOpeningAuthor: history[0].Login,
		Recovered:             true,
	}
	for _, comment := range history {
		switch comment.ID {
		case predecessorID:
			observation.ObservedPredecessorTime = comment.UpdatedAt
			observation.ObservedPredecessorEditCount = comment.EditCount
			observation.ObservedPredecessorEditCountPresent = comment.EditCountPresent
			observation.ObservedPredecessorLastEditID = comment.LastEditID
			observation.ObservedPredecessorLastEditIDPresent = comment.LastEditIDPresent
		case replyID:
			observation.Author = comment.Login
			observation.BodySHA256 = exactBodySHA256([]byte(comment.Body))
			observation.UpdatedAt = comment.UpdatedAt
			observation.EditCount = comment.EditCount
			observation.EditCountPresent = comment.EditCountPresent
			observation.LastEditID = comment.LastEditID
			observation.LastEditIDPresent = comment.LastEditIDPresent
		}
	}
	return observation
}

func rejectDeniedObservedReply(
	ctx context.Context,
	threadID, bodyFile, originalTailID, originalTailBodySHA256, observedReplyID, replyBody string,
	postErr error,
	observed *thread,
) (string, int) {
	var cur thread
	if observed != nil {
		cur = *observed
	} else {
		var readErr error
		cur, readErr = fetchThread(ctx, threadID)
		if readErr != nil {
			return "", fail("GitHub denied this caller's reply mutation, and an independently posted byte-exact reply was visible in history, but exact current placement could not be read: %v\n%s",
				postErr,
				providerReadRecoveryGuidance(readErr, deniedObservedReplyGuidance(threadID, bodyFile)),
			)
		}
	}
	exactPlacement, placementVerified := deniedObservedReplyPlacement(
		cur,
		originalTailID,
		originalTailBodySHA256,
		observedReplyID,
		replyBody,
	)
	if !placementVerified {
		return "", fail("GitHub denied this caller's reply mutation, and an independently posted byte-exact reply was observed, but the exact-state response omitted an ID or body required to verify its current placement: %v\n%s",
			postErr, deniedObservedReplyGuidance(threadID, bodyFile))
	}
	authorVerified := cur.Answered
	reopened := false
	if (!exactPlacement || !authorVerified) && cur.IsResolved {
		reason := "resolved after a denied caller observed an independently posted matching reply outside the exact current predecessor-to-reply boundary"
		recovery := answerChangedEvidence
		failureGuidance := revisedAnswerRecoveryGuidance(threadID, bodyFile, false)
		if exactPlacement && !authorVerified {
			reason = "resolved after a denied caller observed matching reply bytes without provider author evidence of an independent answer"
			recovery = retryUnchangedEvidence
			failureGuidance.inspect = unavailableAuthorEvidenceGuidance(threadID, bodyFile)
		}
		if reopenErr := reopenOrFail(ctx, threadID, cur.Path, reason, recovery); reopenErr != nil {
			message, _ := replyResolutionEvidenceFailure(
				threadID,
				reopenErr,
				failureGuidance,
			)
			return "", fail("%s%s", message, accessRepairNote(postErr))
		}
		reopened = true
	}
	if exactPlacement && !authorVerified {
		return "", fail("%sGitHub denied this caller's reply mutation, and a byte-exact reply is now visible after the original predecessor, but missing or equal provider author identity cannot prove an independent answer: %v\n"+
			"the observed comment cannot be attributed to this caller, rebound as this attempt, used to mint a fresh continuation receipt, or resolved by this lifecycle.\n%s",
			confirmedReopenPrefix(reopened), postErr, deniedObservedReplyGuidance(threadID, bodyFile))
	}
	return "", fail("%sGitHub denied this caller's reply mutation, but an independently posted byte-exact reply is now visible after the original predecessor: %v\n"+
		"the observed comment cannot be attributed to this caller, rebound as this attempt, used to mint a fresh continuation receipt, or resolved by this lifecycle.\n%s",
		confirmedReopenPrefix(reopened), postErr, deniedObservedReplyGuidance(threadID, bodyFile))
}

func deniedObservedReplyPlacement(
	cur thread,
	originalTailID, originalTailBodySHA256, observedReplyID, replyBody string,
) (exact, verified bool) {
	if cur.LastID == "" {
		return false, false
	}
	if cur.LastID != observedReplyID {
		return false, true
	}
	if cur.PrevID == "" {
		return false, false
	}
	if cur.PrevID != originalTailID {
		return false, true
	}
	if !cur.LastBodyPresent {
		return false, false
	}
	if cur.LastBody != replyBody {
		return false, true
	}
	if !cur.PrevBodyPresent {
		return false, false
	}
	if exactBodySHA256([]byte(cur.PrevBody)) != originalTailBodySHA256 {
		return false, true
	}
	return true, true
}

func currentStateShowsAttemptedReply(
	cur thread,
	originalTailID, originalTailBodySHA256, replyBody string,
) bool {
	return cur.LastID != "" &&
		cur.PrevID == originalTailID &&
		cur.PrevBodyPresent &&
		cur.LastBodyPresent &&
		exactBodySHA256([]byte(cur.PrevBody)) == originalTailBodySHA256 &&
		cur.LastBody == replyBody
}

func reconcileMissingAmbiguousPredecessor(
	ctx context.Context,
	threadID, bodyFile, originalTailID, originalTailBodySHA256, replyBody string,
	postErr error,
) (string, int) {
	cur, code := readAmbiguousReplyState(ctx, threadID, bodyFile, postErr)
	if code >= 0 {
		return "", code
	}
	if currentStateShowsAttemptedReply(cur, originalTailID, originalTailBodySHA256, replyBody) {
		return "", fail("provider state is unverified: reply outcome is ambiguous and a later exact-state read observed the attempted reply, but full history omitted its original predecessor; no mutation or recovery claim is safe: %v%s\n%s",
			postErr, accessRepairNote(postErr), inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if cur.LastID != originalTailID {
		return rejectAbsentReplyAfterMovedTail(
			ctx,
			threadID,
			bodyFile,
			originalTailID,
			cur,
			postErr,
		)
	}
	if !cur.LastBodyPresent {
		return "", fail("provider state is unverified: reply outcome is ambiguous, full history omitted original predecessor %s, and the later exact-state read omitted its body; no mutation or retry classification is safe: %v%s\n%s",
			originalTailID, postErr, accessRepairNote(postErr),
			inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if exactBodySHA256([]byte(cur.LastBody)) == originalTailBodySHA256 {
		return "", fail("provider state is unverified: reply outcome is ambiguous and full history omitted original predecessor %s even though a later exact-state read observed it; no mutation or unchanged retry is safe: %v%s\n%s",
			originalTailID, postErr, accessRepairNote(postErr),
			inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	return rejectAbsentReplyAfterSameIDEdit(
		ctx,
		threadID,
		bodyFile,
		originalTailID,
		cur,
		postErr,
	)
}

func rejectAbsentReplyAtUnchangedBoundary(
	threadID, bodyFile, originalTailID string,
	cur thread,
	postErr error,
) (string, int) {
	if cur.IsResolved {
		return "", fail("reply was not observed and the original tail is unchanged, but the thread is now resolved by another actor: %v%s\n%s",
			postErr, accessRepairNote(postErr), inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if isAccessDenied(postErr) {
		return "", fail("reply was denied and was not observed; the original tail %s remains last and unresolved: %v\n%s",
			originalTailID, postErr, accessDeniedReplyGuidance(threadID, bodyFile))
	}
	return "", fail("reply was not observed after the ambiguous provider outcome, and the original tail ID and exact body are unchanged in both recovery reads: %v\n"+
		"an exact-body retry remains applicable:\n%s",
		postErr, unchangedReplyBodyGuidance(threadID, bodyFile))
}

func rejectAbsentReplyAfterSameIDEdit(
	ctx context.Context,
	threadID, bodyFile, originalTailID string,
	cur thread,
	postErr error,
) (string, int) {
	reason := fmt.Sprintf(
		"resolved after predecessor %s changed its exact body while the reply was not observed",
		originalTailID,
	)
	reopened, code := reopenChangedAmbiguousReply(ctx, threadID, bodyFile, cur, reason, postErr)
	if code >= 0 {
		return "", code
	}
	return "", fail("%sreply was not observed and predecessor %s kept its ID but changed its exact body; the old answer must not be replayed against the edited comment: %v%s\n"+
		"inspect the edited comment and use this revised-answer lifecycle:\n%s",
		confirmedReopenPrefix(reopened), originalTailID, postErr, accessRepairNote(postErr),
		revisedReplyBodyGuidance(threadID, bodyFile))
}

func rejectAbsentReplyAfterChangedPredecessor(
	ctx context.Context,
	threadID, bodyFile string,
	cur thread,
	detail string,
	postErr error,
) (string, int) {
	reason := "resolved after the full pre-post predecessor snapshot changed while the reply was not observed: " + detail
	reopened, code := reopenChangedAmbiguousReply(ctx, threadID, bodyFile, cur, reason, postErr)
	if code >= 0 {
		return "", code
	}
	return "", fail("%sreply was not observed, and the full pre-post predecessor snapshot changed during recovery (%s); an unchanged retry is no longer proven and the retained answer must not be replayed blindly: %v%s\n"+
		"inspect the changed comment and use this same-source revised-answer lifecycle:\n%s",
		confirmedReopenPrefix(reopened), detail, postErr, accessRepairNote(postErr),
		revisedReplyBodyGuidance(threadID, bodyFile))
}

func rejectAbsentReplyAfterMovedTail(
	ctx context.Context,
	threadID, bodyFile, originalTailID string,
	cur thread,
	postErr error,
) (string, int) {
	reason := fmt.Sprintf(
		"resolved after the reply predecessor changed from %s to %s while the reply was not observed",
		originalTailID,
		cur.LastID,
	)
	reopened, code := reopenChangedAmbiguousReply(ctx, threadID, bodyFile, cur, reason, postErr)
	if code >= 0 {
		return "", code
	}
	return "", fail("%sreply was not observed and the thread tail moved from %s to %s; the old body must not be replayed against the new comment: %v%s\n"+
		"read the follow-up and use this revised-answer lifecycle:\n%s",
		confirmedReopenPrefix(reopened), originalTailID, cur.LastID, postErr, accessRepairNote(postErr),
		revisedReplyBodyGuidance(threadID, bodyFile))
}

func reopenChangedAmbiguousReply(
	ctx context.Context,
	threadID, bodyFile string,
	cur thread,
	reason string,
	postErr error,
) (bool, int) {
	if !cur.IsResolved {
		return false, -1
	}
	if reopenErr := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); reopenErr != nil {
		message, _ := replyResolutionEvidenceFailure(
			threadID,
			reopenErr,
			revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
		)
		return false, fail("%s%s", message, accessRepairNote(postErr))
	}
	return true, -1
}

func confirmedReopenPrefix(reopened bool) string {
	if !reopened {
		return ""
	}
	return "the thread is now confirmed reopened after the automatic corrective attempt. "
}
