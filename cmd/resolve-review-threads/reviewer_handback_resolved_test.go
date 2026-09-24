package main

import (
	"strings"
	"testing"
)

func TestReviewerHandbackIsDistinctFromVirginReplyState(t *testing.T) {
	history := []tailComment{
		{ID: "PRRC_opening", Login: "reviewer", Body: "P1: answer this."},
		{ID: "PRRC_old_answer", Login: "author", Body: "The earlier answer."},
		{ID: "PRRC_handback", Login: "reviewer", Body: "This still needs work."},
	}
	if got := classifyPriorReply(history, "The revised answer."); got != reviewerHandbackIsLast {
		t.Fatalf("revised body over reviewer hand-back classified as %v, want reviewerHandbackIsLast", got)
	}
}

func TestReplyResolveNeverSkipsResolvedReviewerHandbackAsTerminal(t *testing.T) {
	const (
		threadID = "PRRT_resolved_reviewer_handback"
		newBody  = "The revised answer addresses the hand-back."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P1: answer this."}
	oldAnswer := providerComment{id: "PRRC_old_answer", login: "author", body: "The earlier answer."}
	handback := providerComment{id: "PRRC_handback", login: "reviewer", body: "This still needs work."}

	tests := []struct {
		name              string
		initiallyResolved bool
		wantKinds         []string
		want              []string
	}{
		{
			name:              "resolved before invocation is non-mutatingly refused",
			initiallyResolved: true,
			wantKinds:         []string{"thread", "history", "thread"},
			want:              []string{"already resolved before this command", "reviewer-side hand-back", "no mutation was attempted"},
		},
		{
			name:              "resolution during history read is reopened and refused",
			initiallyResolved: false,
			wantKinds:         []string{"thread", "history", "thread", "unresolve"},
			want:              []string{"hand-back remains unanswered", "automatically reopened", "revised-answer lifecycle"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, newBody)
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, tc.initiallyResolved, opening, oldAnswer, handback)},
				providerStep{stdout: historyResponse(threadID, opening, oldAnswer, handback)},
				providerStep{stdout: threadResponse(threadID, true, opening, oldAnswer, handback)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatalf("resolved reviewer hand-back was reported as terminal:\nstdout:\n%s\nstderr:\n%s", stdout, diagnostics)
			}
			assertQueryKinds(t, provider, tc.wantKinds...)
			if stdout != "" {
				t.Errorf("resolved reviewer hand-back printed success output: %q", stdout)
			}
			assertContainsAll(t, strings.ToLower(diagnostics), tc.want...)
			assertContainsNone(t, stdout+diagnostics,
				"skipped "+threadID,
				opening.body,
				oldAnswer.body,
				handback.body,
				newBody,
			)
			for _, kind := range queryKinds(provider.requests()) {
				if kind == "reply" || kind == "resolve" {
					t.Errorf("resolved reviewer hand-back issued forbidden %s mutation", kind)
				}
			}
		})
	}
}
