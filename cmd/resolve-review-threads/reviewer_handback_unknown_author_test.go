package main

import (
	"strings"
	"testing"
)

func TestStableReviewerHandbackWithUnknownAuthorNeverLeavesStaleResolution(t *testing.T) {
	const (
		threadID = "PRRT_handback_unknown_stable_author"
		body     = "The revised answer for the reviewer hand-back."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P1: answer this."}
	oldAnswer := providerComment{id: "PRRC_old_answer", login: "author", body: "The old answer."}
	handback := providerComment{id: "PRRC_handback", login: "reviewer", body: "This still needs work."}
	stableOpening := providerComment{id: opening.id, login: "", body: opening.body}
	stableHandback := providerComment{id: handback.id, login: "", body: handback.body}

	for _, resolved := range []bool{false, true} {
		name := "unresolved refuses on identity uncertainty"
		if resolved {
			name = "concurrent stale resolution is reopened"
		}
		t.Run(name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, body)
			steps := []providerStep{
				{stdout: threadResponse(threadID, false, opening, oldAnswer, handback)},
				{stdout: historyResponse(threadID, opening, oldAnswer, handback)},
				{stdout: threadResponse(threadID, resolved, stableOpening, oldAnswer, stableHandback)},
			}
			if resolved {
				steps = append(steps, providerStep{stdout: unresolveResponse(threadID)})
			}
			provider := installSequencedProvider(t, steps...)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("unknown stable hand-back identity reported terminal success")
			}
			provider.assertExhausted()
			wantKinds := []string{"thread", "history", "thread"}
			if resolved {
				wantKinds = append(wantKinds, "unresolve")
			}
			assertQueryKinds(t, provider, wantKinds...)
			assertNoReplyOrResolveMutation(t, provider)
			lower := strings.ToLower(diagnostics)
			if resolved {
				assertContainsAll(t, lower, "hand-back remains unanswered", "automatically reopened", "revised-answer lifecycle")
			} else {
				assertContainsAll(t, lower, "author evidence", "nothing was posted or resolved")
			}
			assertContainsNone(t, stdout+diagnostics, opening.body, oldAnswer.body, handback.body, body)
		})
	}
}
