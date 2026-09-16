package main

import (
	"context"
	"strings"
	"testing"
)

func TestAmbiguousReplyAccessDeniedStateReadRequiresCredentialRepair(t *testing.T) {
	const (
		threadID        = "PRRT_ambiguous_state_access_denied"
		predecessorID   = "PRRC_ambiguous_access_predecessor"
		predecessorBody = "P1: retain this private predecessor body."
		replyBody       = "PRIVATE-AMBIGUOUS-REPLY-BODY"
		bodyFile        = "/tmp/resolve-review-thread.ACCESS"
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: predecessorBody}
	provider := installSequencedProvider(t,
		providerStep{stderr: "transport dropped before acknowledgement\n" + replyBody, exit: 1},
		providerStep{stdout: historyResponse(threadID, predecessor)},
		providerStep{stderr: "gh: Resource not accessible by integration (HTTP 403)\n" + predecessorBody + "\n" + replyBody, exit: 1},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		_, postCode := postReplyOrExit(
			context.Background(),
			threadID,
			replyBody,
			testReplyIssuancePredecessor(predecessor, replyBody),
			bodyFile,
		)
		return postCode
	})
	if code == 0 {
		t.Fatal("access-denied ambiguous-state read reported success")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "reply", "history", "thread")
	assertNoReplyMutationAfterInitialAttempt(t, provider)
	if stdout != "" {
		t.Errorf("access-denied state read printed success output: %q", stdout)
	}
	lower := strings.ToLower(diagnostics)
	assertContainsAll(t, lower, "provider access denied", "repair `gh` credentials", "denied again")
	assertContainsNone(t, diagnostics, predecessorBody, replyBody)
}

func TestContinuationMismatchAccessDeniedStateReadRequiresCredentialRepair(t *testing.T) {
	const (
		threadID        = "PRRT_continuation_state_access_denied"
		predecessorID   = "PRRC_continuation_access_predecessor"
		predecessorBody = "P1: continuation predecessor must stay private."
		replyID         = "PRRC_continuation_access_reply"
		replyBody       = "PRIVATE-CONTINUATION-REPLY-BODY"
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: predecessorBody}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_continuation_access_followup", login: "reviewer", body: "PRIVATE-FOLLOWUP-BODY"}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessorID, predecessorBody, replyID, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, predecessor, reply, followup)},
		providerStep{stderr: "gh: GraphQL: Resource not accessible by personal access token\n" + predecessorBody + "\n" + replyBody + "\n" + followup.body, exit: 1},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("access-denied continuation state read reported success")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("access-denied continuation state read printed success output: %q", stdout)
	}
	lower := strings.ToLower(diagnostics)
	assertContainsAll(t, lower, "provider access denied", "repair `gh` credentials", "denied again")
	assertContainsNone(t, diagnostics, predecessorBody, replyBody, followup.body)
}
