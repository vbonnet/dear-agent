package main

import (
	"strings"
	"testing"
)

func TestResolveAllRefusalSummaryIsTruthful(t *testing.T) {
	const (
		unansweredID = "PRRT_unanswered"
		supersededID = "PRRT_superseded"
		answerID     = "PRRC_answer"
	)
	unansweredOpening := providerComment{
		id: "PRRC_unanswered_opening", login: "reviewer", body: "P1: answer this",
	}
	supersededOpening := providerComment{
		id: "PRRC_superseded_opening", login: "reviewer", body: "P2: prove this",
	}
	answer := providerComment{id: answerID, login: "author", body: "Initially proved."}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "The proof is stale."}

	provider := installSequencedProvider(t,
		providerStep{stdout: listResponse(
			threadNodeResponse(unansweredID, false, unansweredOpening),
			threadNodeResponse(supersededID, false, supersededOpening, answer),
		)},
		providerStep{stdout: threadResponse(unansweredID, false, unansweredOpening)},
		providerStep{stdout: threadResponse(supersededID, false, supersededOpening, answer)},
		providerStep{stdout: resolveResponse(supersededID, followup.id)},
		providerStep{stdout: unresolveResponse(supersededID)},
	)
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"resolve-all", "owner", "repo", "42"})
	})
	if code == 0 {
		t.Fatalf("resolve-all succeeded despite two evidence refusals:\nstdout:\n%s\nstderr:\n%s",
			stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "list", "thread", "thread", "resolve", "unresolve")
	assertContainsAll(t, stdout, "resolved 0 thread(s), refused 2, skipped 0")
	assertContainsAll(t, diagnostics,
		"REFUSED",
		unansweredID,
		supersededID,
		"may have no independent reply yet",
		"superseded by newer commentary",
	)
	assertContainsNone(t, diagnostics,
		"nobody replied",
		"all refused threads are unanswered",
	)
}

func TestResolveAllAbortsOnAccessDeniedSupersessionCause(t *testing.T) {
	const (
		firstThread  = "PRRT_denied_then_moved"
		secondThread = "PRRT_must_not_be_retried"
	)
	firstOpening := providerComment{id: "PRRC_first_opening", login: "reviewer", body: "P1: verify access handling"}
	firstAnswer := providerComment{id: "PRRC_first_answer", login: "author", body: "The first point was answered."}
	followup := providerComment{id: "PRRC_first_followup", login: "reviewer", body: "The evidence changed during resolution."}
	secondOpening := providerComment{id: "PRRC_second_opening", login: "reviewer", body: "P1: do not retry denied credentials"}
	secondAnswer := providerComment{id: "PRRC_second_answer", login: "author", body: "This mutation must not be attempted."}
	provider := installSequencedProvider(t,
		providerStep{stdout: listResponse(
			threadNodeResponse(firstThread, false, firstOpening, firstAnswer),
			threadNodeResponse(secondThread, false, secondOpening, secondAnswer),
		)},
		providerStep{stdout: threadResponse(firstThread, false, firstOpening, firstAnswer)},
		providerStep{stderr: "gh: Resource not accessible by integration (HTTP 403)", exit: 1},
		providerStep{stdout: threadResponse(firstThread, false, firstOpening, firstAnswer, followup)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"resolve-all", "owner", "repo", "42"})
	})
	if code == 0 {
		t.Fatalf("resolve-all continued after denied mutation with superseded evidence:\nstdout:\n%s\nstderr:\n%s", stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "list", "thread", "resolve", "thread")
	for i, request := range provider.requests()[1:] {
		key := "id"
		if strings.Contains(request.Query, "mutation(") {
			key = "threadId"
		}
		if got := string(request.Variables[key]); got != `"`+firstThread+`"` {
			t.Errorf("post-list provider request %d target = %s, want %q", i, got, firstThread)
		}
	}
	assertContainsAll(t, strings.ToLower(diagnostics),
		"aborting sweep after 0 resolved, 0 refused",
		"denied",
		"repair `gh` credentials",
	)
	assertContainsNone(t, stdout+diagnostics,
		secondThread,
		firstOpening.body,
		firstAnswer.body,
		followup.body,
		secondOpening.body,
		secondAnswer.body,
	)
}
