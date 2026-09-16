// Continuation mismatch reconciliation and stale-resolution repair.
package main

import (
	"context"
)

// reconcileContinuationHistoryMismatch distinguishes a safely unresolved
// stale receipt from a thread another actor already resolved on that stale
// evidence. The latter is reopened before returning so a reviewer follow-up or
// edited predecessor is not left resolved away merely because it was detected
// by the full-history read one request earlier than resolveWithEvidence.
func reconcileContinuationHistoryMismatch(
	ctx context.Context,
	receipt continuationReceipt,
	historyErr error,
	guidance evidenceRecoveryGuidance,
) int {
	conclusive := isConclusiveContinuationHistoryContradiction(historyErr)
	cur, readErr := fetchThread(ctx, receipt.ThreadID)
	if readErr != nil {
		return fail("continuation evidence is not current (%v), and exact provider state could not be re-read; no mutation was attempted: %v\n%s",
			historyErr, readErr, providerReadRecoveryGuidance(readErr, guidance.inspect))
	}
	if !cur.IsResolved {
		return fail("continuation evidence is not current, so no mutation was attempted: %v\n%s",
			historyErr, guidance.inspect)
	}
	if !conclusive {
		return fail("continuation history is unverified (%v), so the resolved thread was left unchanged and no mutation was attempted.\n%s",
			historyErr, guidance.inspect)
	}
	switch classifyContinuationThreadBoundary(receipt, cur) {
	case continuationBoundaryIdentityUnverified:
		return fail("continuation evidence is not current (%v), but the resolved thread's exact-state response omitted a named ID needed to prove whether that resolution is stale; no mutation was attempted.\n%s",
			historyErr, guidance.inspect)
	case continuationBoundaryEvidenceUnverified:
		// Full history already proved a complete contradiction. A later
		// resolved response that preserves the named IDs but omits an
		// unrelated body, timestamp, edit revision, or author must not erase
		// that proof and leave a stale resolution terminal.
	case continuationBoundaryMatch:
		return fail("continuation history and the later exact-state read disagree (%v); the thread is resolved on a currently matching boundary, but this command did not mutate it. Inspect the live history before treating that resolution as terminal.\n%s",
			historyErr, guidance.inspect)
	case continuationBoundaryMismatch:
		// A complete exact-state response independently proves that the current
		// resolved boundary is not the one authorized by the receipt. Reopen it
		// below; incomplete evidence never reaches this branch.
	}

	reason := "resolved on continuation evidence that the full-history read no longer validates"
	if reopenErr := reopenOrFail(
		ctx,
		receipt.ThreadID,
		cur.Path,
		reason,
		answerChangedEvidence,
	); reopenErr != nil {
		message, _ := replyResolutionEvidenceFailure(receipt.ThreadID, reopenErr, guidance)
		return fail("continuation evidence is not current (%v). %s", historyErr, message)
	}
	return fail("continuation evidence is not current (%v); the thread was resolved on that stale evidence, so it was automatically reopened and that postcondition was confirmed. No reply or resolve mutation was attempted.\n%s",
		historyErr, guidance.inspect)
}

type continuationBoundaryState uint8

const (
	continuationBoundaryIdentityUnverified continuationBoundaryState = iota
	continuationBoundaryEvidenceUnverified
	continuationBoundaryMatch
	continuationBoundaryMismatch
)

func classifyContinuationThreadBoundary(
	receipt continuationReceipt,
	cur thread,
) continuationBoundaryState {
	if state := classifyContinuationBoundaryIDs(receipt, cur); state != continuationBoundaryMatch {
		return state
	}
	if state := classifyContinuationBoundaryBodies(receipt, cur); state != continuationBoundaryMatch {
		return state
	}
	if state := classifyContinuationBoundaryTimestamps(receipt, cur); state != continuationBoundaryMatch {
		return state
	}
	if state := classifyContinuationBoundaryEdits(receipt, cur); state != continuationBoundaryMatch {
		return state
	}
	return classifyContinuationBoundaryAuthors(receipt, cur)
}

func classifyContinuationBoundaryIDs(
	receipt continuationReceipt,
	cur thread,
) continuationBoundaryState {
	if cur.LastID == "" {
		return continuationBoundaryIdentityUnverified
	}
	if cur.LastID != receipt.ReplyID {
		return continuationBoundaryMismatch
	}
	if cur.PrevID == "" {
		return continuationBoundaryIdentityUnverified
	}
	if cur.PrevID != receipt.PredecessorID {
		return continuationBoundaryMismatch
	}
	return continuationBoundaryMatch
}

func classifyContinuationBoundaryBodies(
	receipt continuationReceipt,
	cur thread,
) continuationBoundaryState {
	if !cur.PrevBodyPresent || !cur.LastBodyPresent {
		return continuationBoundaryEvidenceUnverified
	}
	if exactBodySHA256([]byte(cur.PrevBody)) != receipt.PredecessorBodySHA256 ||
		exactBodySHA256([]byte(cur.LastBody)) != receipt.BodySHA256 {
		return continuationBoundaryMismatch
	}
	return continuationBoundaryMatch
}

func classifyContinuationBoundaryTimestamps(
	receipt continuationReceipt,
	cur thread,
) continuationBoundaryState {
	if cur.PrevUpdatedAt == "" || cur.LastUpdatedAt == "" {
		return continuationBoundaryEvidenceUnverified
	}
	if cur.PrevUpdatedAt != receipt.PredecessorUpdatedAt || cur.LastUpdatedAt != receipt.ReplyUpdatedAt {
		return continuationBoundaryMismatch
	}
	return continuationBoundaryMatch
}

func classifyContinuationBoundaryEdits(
	receipt continuationReceipt,
	cur thread,
) continuationBoundaryState {
	if !cur.PrevEditCountPresent || !cur.LastEditCountPresent ||
		!cur.PrevEditIDPresent || !cur.LastEditIDPresent {
		return continuationBoundaryEvidenceUnverified
	}
	if cur.PrevEditCount != receipt.PredecessorEditCount ||
		cur.LastEditCount != receipt.ReplyEditCount ||
		cur.PrevEditID != receipt.PredecessorLastEditID ||
		cur.LastEditID != receipt.ReplyLastEditID {
		return continuationBoundaryMismatch
	}
	return continuationBoundaryMatch
}

func classifyContinuationBoundaryAuthors(
	receipt continuationReceipt,
	cur thread,
) continuationBoundaryState {
	if cur.Author == "unknown" || cur.LastAuthor == "unknown" {
		return continuationBoundaryEvidenceUnverified
	}
	if cur.Author != receipt.OpeningAuthor || cur.LastAuthor != receipt.ReplyAuthor || !cur.Answered {
		return continuationBoundaryMismatch
	}
	return continuationBoundaryMatch
}
