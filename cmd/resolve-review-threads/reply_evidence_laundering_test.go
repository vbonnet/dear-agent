package main

import (
	"strings"
	"testing"
)

func TestReplyResolveRefusesToRebindExistingExactReplyWithoutReceipt(t *testing.T) {
	const (
		threadID      = "PRRT_unreceipted_exact_reply"
		predecessorID = "PRRC_unreceipted_predecessor"
		replyID       = "PRRC_independently_authored_reply"
		originalPoint = "P1: preserve proof of the temporal reply boundary."
		replyBody     = "The provider-visible reply exactly matches the selected source."
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	reply := providerComment{id: replyID, login: "independent-author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		providerStep{stdout: historyResponse(threadID, predecessor, reply)},
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		// These responses make the old adoption path deterministic. A correct
		// implementation stops after stable classification and consumes none of
		// them because current adjacency cannot recreate the missing receipt.
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, predecessor, reply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("ordinary reply-resolve rebound unreceipted temporal evidence")
	}
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("unreceipted exact reply printed success output: %q", stdout)
	}
	lowerDiagnostics := strings.ToLower(diagnostics)
	assertContainsAll(t, lowerDiagnostics,
		"unreceipted temporal pairing",
		"cannot rebind",
		"mint a fresh receipt",
		"cannot be freshly receipted",
		"retained receipt is required",
		"if it does not exist, inspect",
	)
	assertContainsNone(t, diagnostics,
		"continuation_receipt=",
		originalPoint,
		replyBody,
	)
	assertQueryKinds(t, provider, "thread", "history", "thread")
}
