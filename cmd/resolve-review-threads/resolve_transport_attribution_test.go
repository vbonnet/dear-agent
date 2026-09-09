package main

import (
	"context"
	"strings"
	"testing"
)

func TestResolveTransportRecoveryNeverAttributesConcurrentMatchingState(t *testing.T) {
	const (
		threadID        = "PRRT_resolve_transport_attribution"
		predecessorBody = "PRIVATE-TRANSPORT-PREDECESSOR"
		replyBody       = "PRIVATE-TRANSPORT-REPLY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: predecessorBody}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	evidence := resolutionEvidence{
		LastID:                reply.id,
		PredecessorID:         predecessor.id,
		PredecessorBodySHA256: exactBodySHA256([]byte(predecessor.body)),
		BodySHA256:            exactBodySHA256([]byte(reply.body)),
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		providerStep{stderr: "connection reset after resolve request", exit: 1},
		providerStep{stdout: threadResponse(threadID, true, predecessor, reply)},
	)

	msg, mutated, err := resolveWithEvidence(context.Background(), threadID, false, evidence)
	if err != nil {
		t.Fatalf("independently confirmed matching resolved state returned error: %v", err)
	}
	if mutated {
		t.Fatal("ambiguous resolve transport outcome was attributed to this caller")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "resolve", "thread")
	lower := strings.ToLower(msg)
	assertContainsAll(t, lower, "skipped", "independently found", "not attributed")
	assertContainsNone(t, lower, "had already applied")
	assertContainsNone(t, msg, predecessorBody, replyBody)
}
