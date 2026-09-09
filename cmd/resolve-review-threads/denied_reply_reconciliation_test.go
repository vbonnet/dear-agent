package main

import (
	"strings"
	"testing"
)

func TestDeniedReplyCannotAdoptIndependentlyObservedExactBody(t *testing.T) {
	const (
		threadID  = "PRRT_denied_reply_adoption"
		replyBody = "PRIVATE-EXACT-DENIED-REPLY-BODY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-PREDECESSOR-BODY"}
	independentReply := providerComment{id: "PRRC_independent_reply", login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "PRIVATE-FOLLOWUP-BODY"}

	tests := []struct {
		name          string
		recoverySteps []providerStep
		wantKinds     []string
		wantReopened  bool
	}{
		{
			name: "full history observes the independent reply",
			recoverySteps: []providerStep{
				{stdout: historyResponse(threadID, predecessor, independentReply)},
				{stdout: threadResponse(threadID, false, predecessor, independentReply)},
			},
			wantKinds: []string{"thread", "history", "thread", "reply", "history", "thread"},
		},
		{
			name: "delayed exact state observes the independent reply",
			recoverySteps: []providerStep{
				{stdout: historyResponse(threadID, predecessor)},
				{stdout: threadResponse(threadID, false, predecessor, independentReply)},
			},
			wantKinds: []string{"thread", "history", "thread", "reply", "history", "thread"},
		},
		{
			name: "independent reply is buried under a stale resolution",
			recoverySteps: []providerStep{
				{stdout: historyResponse(threadID, predecessor, independentReply, followup)},
				{stdout: threadResponse(threadID, true, predecessor, independentReply, followup)},
				{stdout: unresolveResponse(threadID)},
			},
			wantKinds:    []string{"thread", "history", "thread", "reply", "history", "thread", "unresolve"},
			wantReopened: true,
		},
		{
			name: "independent reply disappears before a stale resolution read",
			recoverySteps: []providerStep{
				{stdout: historyResponse(threadID, predecessor, independentReply)},
				{stdout: threadResponse(threadID, true, predecessor)},
				{stdout: unresolveResponse(threadID)},
			},
			wantKinds:    []string{"thread", "history", "thread", "reply", "history", "thread", "unresolve"},
			wantReopened: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, replyBody)
			steps := []providerStep{
				{stdout: threadResponse(threadID, false, predecessor)},
				{stdout: historyResponse(threadID, predecessor)},
				{stdout: threadResponse(threadID, false, predecessor)},
				{stderr: "gh: GraphQL: Resource not accessible by integration\n" + predecessor.body + "\n" + replyBody, exit: 1},
			}
			steps = append(steps, tc.recoverySteps...)
			provider := installSequencedProvider(t, steps...)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("denied reply adopted an independently observed exact body")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, tc.wantKinds...)
			assertNoReplyMutationAfterInitialAttempt(t, provider)
			if stdout != "" {
				t.Errorf("denied reply adoption printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower,
				"denied this caller's reply mutation",
				"independently posted byte-exact reply",
				"cannot be attributed",
				"repair `gh` credentials",
				"mint a fresh continuation receipt",
			)
			if tc.wantReopened {
				assertContainsAll(t, lower, "confirmed reopened", "automatic corrective attempt")
			}
			assertContainsNone(t, lower, "continuation_receipt=", "resolve-only continuation")
			assertContainsNone(t, stdout+diagnostics, predecessor.body, replyBody, followup.body)
		})
	}
}

func TestDeniedObservedReplyPrioritizesChangedPredecessorIDOverMissingBody(t *testing.T) {
	const (
		threadID  = "PRRT_denied_changed_predecessor_missing_body"
		replyBody = "PRIVATE-DENIED-DECISIVE-REPLY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-DENIED-PREDECESSOR"}
	intervening := providerComment{id: "PRRC_intervening", login: "reviewer", body: "PRIVATE-DENIED-INTERVENING"}
	independentReply := providerComment{id: "PRRC_independent_reply", login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, predecessor)},
		providerStep{stdout: historyResponse(threadID, predecessor)},
		providerStep{stdout: threadResponse(threadID, false, predecessor)},
		providerStep{stderr: "gh: GraphQL: Resource not accessible by integration\n" + predecessor.body + "\n" + replyBody, exit: 1},
		providerStep{stdout: historyResponse(threadID, predecessor, independentReply)},
		providerStep{stdout: threadResponseWithRecentBodyOmitted(
			threadID,
			true,
			[]providerComment{predecessor, intervening, independentReply},
			independentReply.id,
		)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("changed predecessor ID hidden by an omitted reply body was treated as terminal")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "history", "thread", "unresolve")
	assertNoReplyMutationAfterInitialAttempt(t, provider)
	if stdout != "" {
		t.Errorf("denied reply recovery printed success: %q", stdout)
	}
	lower := strings.ToLower(diagnostics)
	assertContainsAll(t, lower,
		"denied this caller's reply mutation",
		"confirmed reopened",
		"cannot be attributed",
		"repair `gh` credentials",
	)
	assertContainsNone(t, lower, "exact-state response omitted an id or body")
	assertContainsNone(t, stdout+diagnostics, predecessor.body, intervening.body, replyBody)
}

func TestDeniedObservedReplyReopensResolvedUnansweredAuthorEvidence(t *testing.T) {
	const (
		threadID  = "PRRT_denied_unanswered_author_evidence"
		replyBody = "PRIVATE-DENIED-AUTHOR-REPLY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-DENIED-AUTHOR-PREDECESSOR"}

	tests := []struct {
		name                 string
		observedOpeningLogin string
		observedReplyLogin   string
	}{
		{name: "reply author equals opener", observedOpeningLogin: "reviewer", observedReplyLogin: "reviewer"},
		{name: "reply author is missing", observedOpeningLogin: "reviewer", observedReplyLogin: ""},
		{name: "opening author is missing", observedOpeningLogin: "", observedReplyLogin: "author"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			observedPredecessor := predecessor
			observedPredecessor.login = tc.observedOpeningLogin
			observedReply := providerComment{id: "PRRC_observed_reply", login: tc.observedReplyLogin, body: replyBody}
			bodyFile := writeContinuationBody(t, replyBody)
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, predecessor)},
				providerStep{stdout: historyResponse(threadID, predecessor)},
				providerStep{stdout: threadResponse(threadID, false, predecessor)},
				providerStep{stderr: "gh: GraphQL: Resource not accessible by integration\n" + predecessor.body + "\n" + replyBody, exit: 1},
				providerStep{stdout: historyResponse(threadID, observedPredecessor, observedReply)},
				providerStep{stdout: threadResponse(threadID, true, observedPredecessor, observedReply)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("insufficient author evidence left a denied matching reply resolved")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "history", "thread", "unresolve")
			assertNoReplyMutationAfterInitialAttempt(t, provider)
			if stdout != "" {
				t.Errorf("denied reply author recovery printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower,
				"denied this caller's reply mutation",
				"confirmed reopened",
				"missing or equal provider author identity",
				"cannot prove an independent answer",
				"cannot be attributed",
				"repair `gh` credentials",
				"mint a fresh continuation receipt",
			)
			assertContainsNone(t, stdout+diagnostics, predecessor.body, replyBody)
		})
	}
}
