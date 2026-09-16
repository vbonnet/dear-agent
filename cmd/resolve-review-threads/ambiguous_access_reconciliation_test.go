package main

import (
	"context"
	"strings"
	"testing"
)

func TestDeniedAmbiguousReplyRetainsCredentialRepairAcrossChangedEvidence(t *testing.T) {
	const (
		threadID        = "PRRT_denied_ambiguous_changed"
		predecessorID   = "PRRC_original"
		predecessorBody = "PRIVATE-ORIGINAL-PREDECESSOR"
		replyBody       = "PRIVATE-DENIED-REPLY"
		bodyFile        = "/tmp/resolve-review-thread.DENIED-CHANGED"
	)
	original := providerComment{id: predecessorID, login: "reviewer", body: predecessorBody}
	edited := providerComment{id: predecessorID, login: "reviewer", body: "PRIVATE-EDITED-PREDECESSOR"}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "PRIVATE-NEW-FOLLOWUP"}
	independentReply := providerComment{id: "PRRC_independent_reply", login: "author", body: replyBody}
	replacement := providerComment{id: "PRRC_replacement", login: "reviewer", body: "PRIVATE-REPLACEMENT"}

	tests := []struct {
		name    string
		history []providerComment
		current []providerComment
		want    []string
	}{
		{
			name:    "same-ID predecessor edit",
			history: []providerComment{edited},
			current: []providerComment{edited},
			want:    []string{"changed its exact body", "revised-answer lifecycle"},
		},
		{
			name:    "moved tail",
			history: []providerComment{original, followup},
			current: []providerComment{original, followup},
			want:    []string{"tail moved", "revised-answer lifecycle"},
		},
		{
			name:    "history omits predecessor while exact state still shows it",
			history: []providerComment{replacement},
			current: []providerComment{original},
			want:    []string{"full history omitted original predecessor", "no mutation or unchanged retry is safe"},
		},
		{
			name:    "history omits predecessor while exact state shows matching reply",
			history: []providerComment{replacement},
			current: []providerComment{original, independentReply},
			want:    []string{"full history omitted its original predecessor", "no mutation or recovery claim is safe"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stderr: "gh: GraphQL: Resource not accessible by integration\n" + predecessorBody + "\n" + replyBody, exit: 1},
				providerStep{stdout: historyResponse(threadID, tc.history...)},
				providerStep{stdout: threadResponse(threadID, false, tc.current...)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				_, postCode := postReplyOrExit(
					context.Background(),
					threadID,
					replyBody,
					testReplyIssuancePredecessor(original, replyBody),
					bodyFile,
				)
				return postCode
			})
			if code == 0 {
				t.Fatal("denied ambiguous reply lost its access failure across changed evidence")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "reply", "history", "thread")
			assertNoReplyMutationAfterInitialAttempt(t, provider)
			if stdout != "" {
				t.Errorf("denied changed-evidence recovery printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower, "provider access denied", "repair `gh` credentials", "denied again")
			assertContainsAll(t, lower, tc.want...)
			assertContainsNone(t, stdout+diagnostics,
				predecessorBody,
				replyBody,
				edited.body,
				followup.body,
				replacement.body,
			)
		})
	}
}
