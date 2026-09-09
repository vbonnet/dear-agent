package main

import (
	"context"
	"strings"
	"testing"
)

func TestAmbiguousReplyRejectsSameIDPredecessorEditBeforeUnchangedRetry(t *testing.T) {
	const (
		threadID      = "PRRT_ambiguous_predecessor_edit"
		predecessorID = "PRRC_same_id_edited_after_post"
		originalBody  = "P1: answer the original review point."
		editedBody    = "P1: this edited point requires a different answer."
		replyBody     = "The original point was addressed."
		bodyFile      = "/tmp/resolve-review-thread.AMBIGUOUS"
	)
	original := providerComment{id: predecessorID, login: "reviewer", body: originalBody}
	edited := providerComment{id: predecessorID, login: "reviewer", body: editedBody}

	for _, resolved := range []bool{false, true} {
		name := "unresolved"
		if resolved {
			name = "stale-resolved"
		}
		t.Run(name, func(t *testing.T) {
			steps := []providerStep{
				{stderr: "transport dropped before acknowledgement", exit: 1},
				{stdout: historyResponse(threadID, edited)},
				{stdout: threadResponse(threadID, resolved, edited)},
			}
			if resolved {
				steps = append(steps, providerStep{stdout: unresolveResponse(threadID)})
			}
			provider := installSequencedProvider(t, steps...)

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
				t.Fatal("same-ID predecessor edit licensed an unchanged reply retry")
			}
			wantKinds := []string{"reply", "history", "thread"}
			if resolved {
				wantKinds = append(wantKinds, "unresolve")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, wantKinds...)
			if stdout != "" {
				t.Errorf("ambiguous edited-predecessor recovery printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower, "kept its id", "changed its exact body", "revised-answer lifecycle")
			assertContainsNone(t, diagnostics,
				"an exact-body retry remains applicable",
				originalBody,
				editedBody,
				replyBody,
			)
		})
	}
}
