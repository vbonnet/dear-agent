package main

import (
	"strings"
	"testing"
)

func TestInitiallyResolvedSupersededReplyRequiresExplicitReopen(t *testing.T) {
	const (
		threadID  = "PRRT_resolved_superseded_reply"
		replyBody = "The earlier exact answer."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P1: answer this."}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "This still misses the race."}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, true, opening, reply, followup)},
		providerStep{stdout: historyResponse(threadID, opening, reply, followup)},
		providerStep{stdout: threadResponse(threadID, true, opening, reply, followup)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("initially resolved superseded reply reported success")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("initially resolved superseded reply printed success: %q", stdout)
	}
	lower := strings.ToLower(diagnostics)
	assertContainsAll(t, lower, "newer commentary", "nothing was posted", "first reopen", "same-source revised-answer lifecycle")
	unresolveAt := strings.Index(diagnostics, "resolve-review-threads unresolve "+threadID)
	replyResolveAt := strings.Index(diagnostics, "resolve-review-threads reply-resolve "+threadID)
	if unresolveAt < 0 || replyResolveAt < 0 || unresolveAt >= replyResolveAt {
		t.Fatalf("recovery must put explicit unresolve before revised reply-resolve:\n%s", diagnostics)
	}
	assertContainsNone(t, stdout+diagnostics, opening.body, reply.body, followup.body)
}
