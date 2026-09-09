package main

import (
	"strings"
	"testing"
)

func TestReplyResolveReopensResolvedThreadWhenPredecessorEditedBeforePlacementVerification(t *testing.T) {
	const (
		threadID      = "PRRT_predecessor_edit_after_post"
		predecessorID = "PRRC_stable_predecessor_id"
		originalPoint = "P1: answer the originally reviewed behavior."
		editedPoint   = "P1: this same comment ID now asks for different behavior."
		replyID       = "PRRC_reply_to_original_point"
		replyBody     = "The original review point is addressed."
	)
	original := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	edited := providerComment{id: predecessorID, login: "reviewer", body: editedPoint}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, original)},
		providerStep{stdout: historyResponse(threadID, original)},
		providerStep{stdout: threadResponse(threadID, false, original)},
		providerStep{stdout: replyResponse(reply)},
		// The predecessor is edited in place and another actor resolves the
		// thread after the reply post but before placement verification.
		providerStep{stdout: threadResponse(threadID, true, edited, reply)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("same-ID predecessor edit after posting was accepted")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "thread", "unresolve")
	assertReplyMutationBody(t, provider, replyBody)
	if stdout != "" {
		t.Errorf("stale placement printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "predecessor", "changed")
	assertContainsNone(t, diagnostics, originalPoint, editedPoint, replyBody)
}

func TestContinueResolveRefusesSameIDPredecessorEditAtImmediatePreResolveRead(t *testing.T) {
	const (
		threadID      = "PRRT_predecessor_edit_before_resolve"
		predecessorID = "PRRC_predecessor_before_resolve"
		replyID       = "PRRC_receipt_bound_reply"
		originalPoint = "P1: preserve the review point through continuation."
		editedPoint   = "P1: this edited point requires a different answer."
		replyBody     = "The original point was addressed."
	)
	original := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	edited := providerComment{id: predecessorID, login: "reviewer", body: editedPoint}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessorID, originalPoint, replyID, replyBody)
	provider := installSequencedProvider(t,
		// Full-history validation sees the receipt-bound predecessor.
		providerStep{stdout: historyResponse(threadID, original, reply)},
		// The immediate pre-resolve read sees an edit under the same ID.
		providerStep{stdout: threadResponse(threadID, false, edited, reply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("same-ID predecessor edit at the pre-resolve read was accepted")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("pre-resolve predecessor edit printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "predecessor", "bytes", "changed")
	assertContainsNone(t, diagnostics, originalPoint, editedPoint, replyBody)
}

func TestContinueResolveReopensWhenResolveResponsePredecessorEditedAtSameID(t *testing.T) {
	const (
		threadID      = "PRRT_predecessor_edit_in_resolve"
		predecessorID = "PRRC_predecessor_in_resolve"
		replyID       = "PRRC_reply_in_resolve"
		originalPoint = "P1: bind the predecessor in the resolve response."
		editedPoint   = "P1: the predecessor changed while resolution was in flight."
		replyBody     = "The original predecessor was addressed."
	)
	original := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	edited := providerComment{id: predecessorID, login: "reviewer", body: editedPoint}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessorID, originalPoint, replyID, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, original, reply)},
		providerStep{stdout: threadResponse(threadID, false, original, reply)},
		// Resolution applies, but its own response observes the predecessor
		// edited under the same ID. The stale resolution must be reopened.
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, edited, reply)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("resolve response accepted a same-ID predecessor edit")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread", "resolve", "unresolve")
	assertNoReplyMutation(t, provider)
	assertResolveMutationRequestsExactComments(t, provider)
	if stdout != "" {
		t.Errorf("stale resolution printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "predecessor", "changed", "reopened")
	assertContainsNone(t, diagnostics, originalPoint, editedPoint, replyBody)
}

func TestReplyResolveInitiallyResolvedExactReplyStopsAfterStableClassification(t *testing.T) {
	const (
		threadID      = "PRRT_initially_resolved_exact_reply"
		predecessorID = "PRRC_initially_resolved_predecessor"
		replyID       = "PRRC_initially_resolved_reply"
		originalPoint = "P1: preserve the initial resolved no-mutation boundary."
		editedPoint   = "P1: this same-ID edit lands after the stable classification."
		replyBody     = "The exact answer was already present before this command."
	)
	original := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	edited := providerComment{id: predecessorID, login: "reviewer", body: editedPoint}
	reply := providerComment{id: replyID, login: "author", body: replyBody}

	tests := []struct {
		name  string
		later []providerStep
	}{
		{
			name: "placement read sees a same-ID predecessor edit on the resolved thread",
			later: []providerStep{
				{stdout: threadResponse(threadID, true, edited, reply)},
				{stdout: unresolveResponse(threadID)},
			},
		},
		{
			name: "pre-resolve read sees another actor reopen the thread",
			later: []providerStep{
				{stdout: threadResponse(threadID, true, original, reply)},
				{stdout: threadResponse(threadID, false, original, reply)},
				{stdout: resolveResponseWithExactComments(threadID, true, original, reply)},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, replyBody)
			steps := []providerStep{
				{stdout: threadResponse(threadID, true, original, reply)},
				{stdout: historyResponse(threadID, original, reply)},
				{stdout: threadResponse(threadID, true, original, reply)},
			}
			steps = append(steps, tc.later...)
			provider := installSequencedProvider(t, steps...)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code != 0 {
				t.Errorf("initially resolved exact reply failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout, diagnostics)
			}
			assertQueryKinds(t, provider, "thread", "history", "thread")
			assertNoProviderMutation(t, provider)
			assertContainsAll(t, strings.ToLower(stdout), "skipped "+strings.ToLower(threadID), "already resolved")
			assertContainsNone(t, stdout+diagnostics, originalPoint, editedPoint, replyBody)
		})
	}
}
