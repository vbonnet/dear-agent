package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveErrorRecoveryReopensChangedAuthorEvidence(t *testing.T) {
	const (
		threadID        = "PRRT_resolve_author_recovery"
		predecessorBody = "PRIVATE-AUTHOR-PREDECESSOR"
		replyBody       = "PRIVATE-AUTHOR-REPLY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: predecessorBody}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	evidence := resolutionEvidence{
		LastID:                reply.id,
		PredecessorID:         predecessor.id,
		PredecessorBodySHA256: exactBodySHA256([]byte(predecessor.body)),
		BodySHA256:            exactBodySHA256([]byte(reply.body)),
	}

	for _, changedLogin := range []string{"reviewer", ""} {
		name := "tail author becomes equal to opener"
		if changedLogin == "" {
			name = "tail author becomes unavailable"
		}
		t.Run(name, func(t *testing.T) {
			changedReply := providerComment{id: reply.id, login: changedLogin, body: reply.body}
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
				providerStep{stderr: "transport dropped after resolve request", exit: 1},
				providerStep{stdout: threadResponse(threadID, true, predecessor, changedReply)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			msg, mutated, err := resolveWithEvidence(context.Background(), threadID, false, evidence)
			if err == nil {
				t.Fatal("author contradiction after ambiguous resolve reported success")
			}
			if mutated {
				t.Fatal("author contradiction after ambiguous resolve was attributed as this caller's mutation")
			}
			if msg != "" {
				t.Errorf("author contradiction returned success message: %q", msg)
			}
			var unavailable *unavailableAnswerEvidenceError
			if !errors.As(err, &unavailable) {
				t.Fatalf("author contradiction error = %T %v, want unavailableAnswerEvidenceError", err, err)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "thread", "unresolve")
			assertContainsAll(t, strings.ToLower(err.Error()), "reopened", "fresh author evidence", "independent answer")
			assertContainsNone(t, err.Error(), predecessorBody, replyBody)
		})
	}
}

func TestResolveErrorRecoveryPrioritizesChangedAnchorOverAuthorEvidence(t *testing.T) {
	const (
		threadID        = "PRRT_resolve_anchor_before_author"
		predecessorBody = "PRIVATE-ANCHOR-PREDECESSOR"
		replyBody       = "PRIVATE-ANCHOR-REPLY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: predecessorBody}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "PRIVATE-ANCHOR-FOLLOWUP"}
	evidence := resolutionEvidence{
		LastID:                reply.id,
		PredecessorID:         predecessor.id,
		PredecessorBodySHA256: exactBodySHA256([]byte(predecessor.body)),
		BodySHA256:            exactBodySHA256([]byte(reply.body)),
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		providerStep{stderr: "transport dropped after resolve request", exit: 1},
		providerStep{stdout: threadResponse(threadID, true, predecessor, reply, followup)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	msg, mutated, err := resolveWithEvidence(context.Background(), threadID, false, evidence)
	if err == nil {
		t.Fatal("changed anchor plus author contradiction reported success")
	}
	if mutated || msg != "" {
		t.Fatalf("changed anchor returned (msg=%q, mutated=%t), want no success attribution", msg, mutated)
	}
	var superseded *supersededEvidenceError
	if !errors.As(err, &superseded) {
		t.Fatalf("changed anchor error = %T %v, want supersededEvidenceError", err, err)
	}
	var unavailable *unavailableAnswerEvidenceError
	if errors.As(err, &unavailable) {
		t.Fatalf("changed anchor was masked as unavailable author evidence: %v", err)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "resolve", "thread", "unresolve")
	assertContainsAll(t, strings.ToLower(err.Error()), "reopened", "last comment changed")
	assertContainsNone(t, err.Error(), predecessorBody, replyBody, followup.body)
}

func TestContinueResolveAuthorEvidenceRequiresIdentityRepair(t *testing.T) {
	const (
		threadID        = "PRRT_continuation_author_repair"
		predecessorBody = "PRIVATE-CONTINUATION-AUTHOR-PREDECESSOR"
		replyBody       = "PRIVATE-CONTINUATION-AUTHOR-REPLY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: predecessorBody}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}

	for _, changedLogin := range []string{"reviewer", ""} {
		name := "reply author equals opener"
		if changedLogin == "" {
			name = "reply author unavailable"
		}
		t.Run(name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, replyBody)
			receipt := continuationToken(t, threadID, predecessor.id, predecessor.body, reply.id, reply.body)
			changedReply := providerComment{id: reply.id, login: changedLogin, body: reply.body}
			provider := installSequencedProvider(t,
				providerStep{stdout: historyResponse(threadID, predecessor, reply)},
				providerStep{stdout: threadResponse(threadID, false, predecessor, changedReply)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("continuation accepted insufficient author evidence")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "history", "thread")
			assertNoProviderMutation(t, provider)
			if stdout != "" {
				t.Errorf("author-invalid continuation printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower,
				"author evidence is insufficient",
				"retain the continuation receipt",
				"do not retry continue-resolve",
				"missing, equal, or inconsistent",
			)
			assertContainsNone(t, lower, "retry the resolve-only continuation with:")
			assertContainsNone(t, stdout+diagnostics, predecessorBody, replyBody)
		})
	}
}
