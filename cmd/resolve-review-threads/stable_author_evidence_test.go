package main

import (
	"strings"
	"testing"
)

func TestReplyResolveRefusesChangedOrMissingStableAuthorEvidenceBeforePosting(t *testing.T) {
	const (
		threadID = "PRRT_stable_author_evidence"
		body     = "The revised answer selected for this exact source."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P1: answer this."}
	oldAnswer := providerComment{id: "PRRC_old_answer", login: "author", body: "The earlier answer."}
	handback := providerComment{id: "PRRC_handback", login: "reviewer", body: "The race is still open."}

	tests := []struct {
		name   string
		stable []providerComment
	}{
		{
			name: "opening author omitted",
			stable: []providerComment{
				{id: opening.id, login: "", body: opening.body},
				oldAnswer,
				handback,
			},
		},
		{
			name: "tail author omitted",
			stable: []providerComment{
				opening,
				oldAnswer,
				{id: handback.id, login: "", body: handback.body},
			},
		},
		{
			name: "opening author changed",
			stable: []providerComment{
				{id: opening.id, login: "different-reviewer", body: opening.body},
				oldAnswer,
				handback,
			},
		},
		{
			name: "tail author changed",
			stable: []providerComment{
				opening,
				oldAnswer,
				{id: handback.id, login: "different-reviewer", body: handback.body},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, body)
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, opening, oldAnswer, handback)},
				providerStep{stdout: historyResponse(threadID, opening, oldAnswer, handback)},
				providerStep{stdout: threadResponse(threadID, false, tc.stable...)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("changed or missing stable author evidence permitted posting")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "history", "thread")
			assertNoProviderMutation(t, provider)
			if stdout != "" {
				t.Errorf("unstable author evidence printed success: %q", stdout)
			}
			assertContainsAll(t, strings.ToLower(diagnostics), "author evidence", "nothing was posted or resolved", "retain")
			assertContainsNone(t, stdout+diagnostics, opening.body, oldAnswer.body, handback.body, body)
		})
	}
}
