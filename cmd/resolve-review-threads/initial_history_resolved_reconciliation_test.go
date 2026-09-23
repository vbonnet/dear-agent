package main

import (
	"strings"
	"testing"
)

func TestInitialHistoryMismatchReopensConcurrentResolution(t *testing.T) {
	const (
		threadID = "PRRT_initial_history_concurrent_resolution"
		body     = "The exact reply bytes prepared before evidence moved."
	)

	tests := []struct {
		name    string
		initial []providerComment
		history []providerComment
	}{
		{
			name: "tail advances before full history completes",
			initial: []providerComment{
				{id: "PRRC_opening", login: "reviewer", body: "P1: address the initial point."},
			},
			history: []providerComment{
				{id: "PRRC_opening", login: "reviewer", body: "P1: address the initial point."},
				{id: "PRRC_followup", login: "reviewer", body: "A newer point arrived."},
			},
		},
		{
			name: "existing reply predecessor is edited under the same ID",
			initial: []providerComment{
				{id: "PRRC_same_predecessor", login: "reviewer", body: "P1: original point."},
				{id: "PRRC_existing_reply", login: "author", body: body},
			},
			history: []providerComment{
				{id: "PRRC_same_predecessor", login: "reviewer", body: "P1: edited point."},
				{id: "PRRC_existing_reply", login: "author", body: body},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, body)
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, tc.initial...)},
				providerStep{stdout: historyResponse(threadID, tc.history...)},
				providerStep{stdout: threadResponse(threadID, true, tc.history...)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("initial-to-history mismatch left a concurrent resolution terminal")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
			assertNoReplyOrResolveMutation(t, provider)
			if stdout != "" {
				t.Errorf("initial-history mismatch printed success: %q", stdout)
			}
			assertContainsAll(t, strings.ToLower(diagnostics), "initial", "changed", "automatically reopened", "same-source revised-answer lifecycle")
			for _, comment := range append(append([]providerComment{}, tc.initial...), tc.history...) {
				assertContainsNone(t, stdout+diagnostics, comment.body)
			}
			assertContainsNone(t, stdout+diagnostics, body)
		})
	}
}
