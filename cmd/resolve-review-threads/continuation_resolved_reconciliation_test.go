package main

import (
	"strings"
	"testing"
)

func TestContinueResolveReopensAlreadyResolvedBuriedReceiptEvidence(t *testing.T) {
	const (
		threadID      = "PRRT_resolved_buried_continuation"
		predecessorID = "PRRC_buried_predecessor"
		replyID       = "PRRC_buried_reply"
		originalPoint = "P1: preserve the receipt-bound tail."
		replyBody     = "The original point was addressed."
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_followup_after_receipt", login: "reviewer", body: "This still needs work."}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessorID, originalPoint, replyID, replyBody)
	provider := installSequencedProvider(t,
		// Full history proves the receipt-bound reply is now buried.
		providerStep{stdout: historyResponse(threadID, predecessor, reply, followup)},
		// Exact-state reconciliation proves another actor left the stale
		// evidence resolved, so continuation must reopen it.
		providerStep{stdout: threadResponse(threadID, true, predecessor, reply, followup)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("resolved thread with buried continuation evidence reported success")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread", "unresolve")
	assertQueryTargets(t, provider, threadID)
	assertNoReplyOrResolveMutation(t, provider)
	if stdout != "" {
		t.Errorf("stale resolved continuation printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "reply", "current tail", "reopened")
	assertContainsNone(t, diagnostics, originalPoint, replyBody, followup.body)
}

func TestContinueResolveReopensAlreadyResolvedSameIDPredecessorEdit(t *testing.T) {
	const (
		threadID      = "PRRT_resolved_edited_predecessor"
		predecessorID = "PRRC_same_predecessor_id"
		replyID       = "PRRC_reply_after_edited_predecessor"
		originalPoint = "P1: answer this original review point."
		editedPoint   = "P1: the same comment ID now requests different work."
		replyBody     = "The original review point was addressed."
	)
	original := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	edited := providerComment{id: predecessorID, login: "reviewer", body: editedPoint}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessorID, original.body, replyID, replyBody)
	provider := installSequencedProvider(t,
		// The predecessor body changed without changing the receipt-bound ID.
		providerStep{stdout: historyResponse(threadID, edited, reply)},
		// Reconciliation confirms that exact stale state is already resolved.
		providerStep{stdout: threadResponse(threadID, true, edited, reply)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("resolved thread with an edited predecessor reported continuation success")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread", "unresolve")
	assertQueryTargets(t, provider, threadID)
	assertNoReplyOrResolveMutation(t, provider)
	if stdout != "" {
		t.Errorf("edited resolved predecessor printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "predecessor", "digest", "reopened")
	assertContainsNone(t, diagnostics, originalPoint, editedPoint, replyBody)
}

func assertNoReplyOrResolveMutation(t *testing.T, provider *sequencedProvider) {
	t.Helper()
	for _, kind := range queryKinds(provider.requests()) {
		if kind == "reply" || kind == "resolve" {
			t.Errorf("stale continuation issued forbidden %s mutation", kind)
		}
	}
}
