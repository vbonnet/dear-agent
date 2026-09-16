package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestContinueResolveRejectsInvalidReceiptBeforeProviderAccess(t *testing.T) {
	const body = "Receipt validation must finish before provider access."
	valid := continuationReceipt{
		Version:               continuationReceiptVersion,
		ProviderHost:          "github.com",
		ThreadID:              "PRRT_strict_receipt",
		PredecessorID:         "PRRC_predecessor",
		ReplyID:               "PRRC_reply",
		PredecessorBodySHA256: exactBodySHA256([]byte("P1: predecessor")),
		BodySHA256:            exactBodySHA256([]byte(body)),
		PredecessorUpdatedAt:  providerFixtureUpdatedAt,
		ReplyUpdatedAt:        providerFixtureUpdatedAt,
		OpeningAuthor:         "reviewer",
		ReplyAuthor:           "author",
	}
	validRaw, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal valid receipt fixture: %v", err)
	}
	validToken := rawContinuationBytesToken(t, validRaw)

	unsupportedVersion := valid
	unsupportedVersion.Version++
	emptyThreadID := valid
	emptyThreadID.ThreadID = ""
	emptyPredecessorID := valid
	emptyPredecessorID.PredecessorID = ""
	emptyReplyID := valid
	emptyReplyID.ReplyID = ""
	malformedDigest := valid
	malformedDigest.BodySHA256 = "not-a-sha256-digest"
	malformedPredecessorDigest := valid
	malformedPredecessorDigest.PredecessorBodySHA256 = "not-a-sha256-digest"
	missingPredecessorLastEditID := valid
	missingPredecessorLastEditID.PredecessorEditCount = 1
	unexpectedReplyLastEditID := valid
	unexpectedReplyLastEditID.ReplyLastEditID = "UCE_impossible_at_zero"
	newlineThreadID := valid
	newlineThreadID.ThreadID = "PRRT_safe\nforged-diagnostic"
	shellThreadID := valid
	shellThreadID.ThreadID = "PRRT_safe;unsafe"
	duplicateThreadID := rawContinuationBytesToken(t, fmt.Appendf(nil,
		`{"version":%d,"provider_host":%q,"thread_id":"%s","thread_id":"PRRT_duplicate","predecessor_id":"%s","reply_id":"%s","predecessor_body_sha256":"%s","body_sha256":"%s","predecessor_updated_at":%q,"reply_updated_at":%q,"opening_author":%q,"reply_author":%q}`,
		valid.Version,
		valid.ProviderHost,
		valid.ThreadID,
		valid.PredecessorID,
		valid.ReplyID,
		valid.PredecessorBodySHA256,
		valid.BodySHA256,
		valid.PredecessorUpdatedAt,
		valid.ReplyUpdatedAt,
		valid.OpeningAuthor,
		valid.ReplyAuthor,
	))
	reorderedFields := rawContinuationBytesToken(t, fmt.Appendf(nil,
		`{"thread_id":%q,"version":%d,"provider_host":%q,"predecessor_id":%q,"reply_id":%q,"predecessor_body_sha256":%q,"body_sha256":%q,"predecessor_updated_at":%q,"reply_updated_at":%q,"opening_author":%q,"reply_author":%q}`,
		valid.ThreadID,
		valid.Version,
		valid.ProviderHost,
		valid.PredecessorID,
		valid.ReplyID,
		valid.PredecessorBodySHA256,
		valid.BodySHA256,
		valid.PredecessorUpdatedAt,
		valid.ReplyUpdatedAt,
		valid.OpeningAuthor,
		valid.ReplyAuthor,
	))
	escapedThreadID := rawContinuationBytesToken(t, fmt.Appendf(nil,
		`{"version":%d,"provider_host":%q,"thread_id":"\u0050%s","predecessor_id":%q,"reply_id":%q,"predecessor_body_sha256":%q,"body_sha256":%q,"predecessor_updated_at":%q,"reply_updated_at":%q,"opening_author":%q,"reply_author":%q}`,
		valid.Version,
		valid.ProviderHost,
		valid.ThreadID[1:],
		valid.PredecessorID,
		valid.ReplyID,
		valid.PredecessorBodySHA256,
		valid.BodySHA256,
		valid.PredecessorUpdatedAt,
		valid.ReplyUpdatedAt,
		valid.OpeningAuthor,
		valid.ReplyAuthor,
	))

	tests := []struct {
		name  string
		token string
	}{
		{name: "malformed base64url", token: "%%%not-base64url%%%"},
		{name: "padded base64url", token: validToken + "="},
		{name: "non-canonical base64url trailing bits", token: nonCanonicalBase64URLReceiptToken(t, valid)},
		{name: "oversized token", token: strings.Repeat("A", maxContinuationReceiptTokenBytes+1)},
		{name: "unsupported version", token: uncheckedContinuationToken(t, unsupportedVersion)},
		{name: "empty thread id", token: uncheckedContinuationToken(t, emptyThreadID)},
		{name: "empty predecessor id", token: uncheckedContinuationToken(t, emptyPredecessorID)},
		{name: "empty reply id", token: uncheckedContinuationToken(t, emptyReplyID)},
		{name: "newline in id", token: uncheckedContinuationToken(t, newlineThreadID)},
		{name: "shell punctuation in id", token: uncheckedContinuationToken(t, shellThreadID)},
		{name: "duplicate known json field", token: duplicateThreadID},
		{name: "json whitespace", token: rawContinuationBytesToken(t, append(append([]byte(nil), validRaw...), '\n'))},
		{name: "json key order", token: reorderedFields},
		{name: "json string escape", token: escapedThreadID},
		{
			name: "unknown json field",
			token: rawContinuationToken(t, map[string]any{
				"version":                 valid.Version,
				"provider_host":           valid.ProviderHost,
				"thread_id":               valid.ThreadID,
				"predecessor_id":          valid.PredecessorID,
				"reply_id":                valid.ReplyID,
				"predecessor_body_sha256": valid.PredecessorBodySHA256,
				"body_sha256":             valid.BodySHA256,
				"predecessor_updated_at":  valid.PredecessorUpdatedAt,
				"reply_updated_at":        valid.ReplyUpdatedAt,
				"opening_author":          valid.OpeningAuthor,
				"reply_author":            valid.ReplyAuthor,
				"unexpected":              true,
			}),
		},
		{
			name:  "trailing json",
			token: rawContinuationBytesToken(t, append(append([]byte(nil), validRaw...), []byte(` {}`)...)),
		},
		{name: "malformed digest", token: uncheckedContinuationToken(t, malformedDigest)},
		{name: "malformed predecessor digest", token: uncheckedContinuationToken(t, malformedPredecessorDigest)},
		{name: "positive predecessor edit count without last edit id", token: uncheckedContinuationToken(t, missingPredecessorLastEditID)},
		{name: "zero reply edit count with last edit id", token: uncheckedContinuationToken(t, unexpectedReplyLastEditID)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, body)
			provider := installSequencedProvider(t)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"continue-resolve", tc.token, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Error("invalid continuation receipt succeeded")
			}
			if got := provider.requestCount(); got != 0 {
				t.Errorf("invalid receipt made %d provider request(s), want zero", got)
			}
			if stdout != "" {
				t.Errorf("invalid receipt printed success output: %q", stdout)
			}
			assertContainsAll(t, strings.ToLower(diagnostics), "invalid continuation receipt", "no provider request")
			assertContainsAll(t, diagnostics,
				"Retain the selected named reply-body source",
				"do not edit or remove it",
				"clean up",
				"reply_file="+shellQuoteArgument(bodyFile),
			)
			assertContainsNone(t, diagnostics, body)
		})
	}

	t.Run("invalid receipt retains standard input", func(t *testing.T) {
		provider := installSequencedProvider(t)
		var stdout, diagnostics string
		code := runWithContinuationStdin(t, body, func() int {
			var capturedCode int
			capturedCode, stdout, diagnostics = provider.capture(func() int {
				return run([]string{"continue-resolve", "%%%not-base64url%%%", "--body-file", "-"})
			})
			return capturedCode
		})
		if code == 0 {
			t.Error("invalid receipt with standard input succeeded")
		}
		if got := provider.requestCount(); got != 0 {
			t.Errorf("invalid stdin receipt made %d provider request(s), want zero", got)
		}
		if stdout != "" {
			t.Errorf("invalid stdin receipt printed success output: %q", stdout)
		}
		assertContainsAll(t, diagnostics,
			"Retain the intended exact standard-input bytes",
			"do not discard them",
			"clean up",
		)
		assertContainsNone(t, diagnostics, body, "reply_file=", "rm -f")
	})
}

func TestValidateContinuationHistory(t *testing.T) {
	const (
		threadID    = "PRRT_history_receipt"
		predecessor = "PRRC_predecessor"
		replyID     = "PRRC_reply"
		body        = "The provider-visible body bound by the receipt."
	)
	pred := tailComment{ID: predecessor, Login: "reviewer", Body: "P1: verify continuation evidence", UpdatedAt: providerFixtureUpdatedAt, EditCountPresent: true, LastEditIDPresent: true}
	receipt, err := newContinuationReceipt(
		"github.com",
		threadID,
		predecessor,
		replyID,
		[]byte(pred.Body),
		[]byte(body),
		providerFixtureUpdatedAt,
		providerFixtureUpdatedAt,
		0,
		0,
		"",
		"",
		"reviewer",
		"author",
	)
	if err != nil {
		t.Fatalf("create continuation receipt: %v", err)
	}
	reply := tailComment{ID: replyID, Login: "author", Body: body, UpdatedAt: providerFixtureUpdatedAt, EditCountPresent: true, LastEditIDPresent: true}
	other := tailComment{ID: "PRRC_other", Login: "reviewer", Body: "Other commentary.", UpdatedAt: providerFixtureUpdatedAt, EditCountPresent: true, LastEditIDPresent: true}

	tests := []struct {
		name    string
		history []tailComment
		wantErr string
	}{
		{name: "valid", history: []tailComment{pred, reply}},
		{name: "missing predecessor", history: []tailComment{reply}, wantErr: "both named comments"},
		{name: "duplicate predecessor", history: []tailComment{pred, pred, reply}, wantErr: "duplicated the named predecessor"},
		{name: "missing reply", history: []tailComment{pred}, wantErr: "both named comments"},
		{name: "duplicate reply", history: []tailComment{pred, reply, reply}, wantErr: "duplicated the named reply"},
		{name: "non-adjacent reply", history: []tailComment{pred, other, reply}, wantErr: "directly follows"},
		{name: "reply is not tail", history: []tailComment{pred, reply, other}, wantErr: "current tail"},
		{
			name: "provider predecessor digest mismatch",
			history: []tailComment{
				{ID: predecessor, Login: "reviewer", Body: pred.Body + " changed"},
				reply,
			},
			wantErr: "provider-visible predecessor does not match",
		},
		{
			name: "provider body digest mismatch",
			history: []tailComment{
				pred,
				{ID: replyID, Login: "author", Body: body + " changed"},
			},
			wantErr: "provider-visible reply does not match",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateContinuationHistory(receipt, tc.history)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid continuation history rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("invalid continuation history succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("history error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestContinueResolveRejectsMismatchedProviderThreadIdentity(t *testing.T) {
	const (
		threadID    = "PRRT_expected_history_target"
		otherThread = "PRRT_wrong_history_target"
		predecessor = "PRRC_predecessor"
		replyID     = "PRRC_reply"
		body        = "The continuation is bound to the requested thread."
	)
	opening := providerComment{id: predecessor, login: "reviewer", body: "P1: verify the exact target"}
	reply := providerComment{id: replyID, login: "author", body: body}
	bodyFile := writeContinuationBody(t, body)
	receipt := continuationToken(t, threadID, predecessor, opening.body, replyID, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(otherThread, opening, reply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("continuation accepted history for a different provider thread")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("mismatched provider identity printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "mismatched id", strings.ToLower(otherThread))
	assertContainsNone(t, diagnostics, body)
}

func TestReplyResolveRefusesHistoryThatAdvancedPastInitialTail(t *testing.T) {
	const (
		threadID = "PRRT_history_advanced"
		body     = "This answer was prepared before the unseen follow-up."
	)
	opening := providerComment{id: "PRRC_initial_tail", login: "reviewer", body: "P1: address this state"}
	followup := providerComment{id: "PRRC_unseen_followup", login: "reviewer", body: "The state changed while history was read."}
	reply := providerComment{id: "PRRC_stale_reply", login: "author", body: body}
	bodyFile := writeContinuationBody(t, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening, followup)},
		// The old implementation adopts the paged-history tail and can post the
		// body that was prepared before the follow-up was observed.
		providerStep{stdout: threadResponse(threadID, false, opening, followup)},
		providerStep{stdout: replyResponse(reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, followup, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, followup, reply)},
		providerStep{stdout: resolveResponse(threadID, reply.id)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("history that advanced past the initial tail licensed a stale reply")
	}
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("advanced history printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "initial", "tail", "changed", "nothing was posted")
	assertContainsNone(t, diagnostics, body)
}

func TestReplyResolveRefusesSameIDPredecessorBodyEditBeforePosting(t *testing.T) {
	const (
		threadID       = "PRRT_predecessor_body_edit"
		predecessorID  = "PRRC_same_predecessor"
		originalPoint  = "P1: preserve the original review point."
		editedPoint    = "P1: the same comment ID now asks for materially different work."
		replyBody      = "This answer was prepared for the original review point."
		forbiddenReply = "PRRC_forbidden_stale_answer"
	)
	before := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	after := providerComment{id: predecessorID, login: "reviewer", body: editedPoint}
	reply := providerComment{id: forbiddenReply, login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, before)},
		providerStep{stdout: historyResponse(threadID, before)},
		providerStep{stdout: threadResponse(threadID, false, after)},
		// These forbidden mutation responses keep the old behavior deterministic.
		providerStep{stdout: replyResponse(reply)},
		providerStep{stdout: threadResponse(threadID, false, after, reply)},
		providerStep{stdout: threadResponse(threadID, false, after, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, after, reply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("same-ID predecessor body edit licensed a stale reply and resolution")
	}
	assertNoProviderMutation(t, provider)
	assertQueryKinds(t, provider, "thread", "history", "thread")
	if stdout != "" {
		t.Errorf("predecessor body edit printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "predecessor", "changed", "nothing was posted")
	assertContainsNone(t, diagnostics, originalPoint, editedPoint, replyBody)
}

func TestReplyResolveRefusesResolvedStateDriftBeforeMutation(t *testing.T) {
	const (
		threadID = "PRRT_resolved_state_drift"
		body     = "Do not answer across a resolved-state transition."
	)
	opening := providerComment{id: "PRRC_stable_tail", login: "reviewer", body: "P1: preserve the state snapshot"}
	reply := providerComment{id: "PRRC_forbidden_reply", login: "author", body: body}
	bodyFile := writeContinuationBody(t, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, true, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
		// The old implementation sees the stable ID but ignores the resolved-state
		// transition, then posts and resolves a new answer.
		providerStep{stdout: replyResponse(reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: resolveResponse(threadID, reply.id)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("resolved-to-unresolved state drift licensed a new reply")
	}
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("resolved-state drift printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "resolved", "changed")
	assertContainsNone(t, diagnostics, body)
}

func TestContinueResolveRefusesEditedBodyAtSamePreResolveAnchor(t *testing.T) {
	const (
		threadID    = "PRRT_pre_resolve_body_edit"
		predecessor = "PRRC_predecessor"
		replyID     = "PRRC_reply"
		bodyA       = "The exact body that earned the continuation receipt."
		bodyB       = "The body was edited without changing its comment ID."
	)
	opening := providerComment{id: predecessor, login: "reviewer", body: "P1: revalidate exact reply data"}
	replyA := providerComment{id: replyID, login: "author", body: bodyA}
	replyB := providerComment{id: replyID, login: "author", body: bodyB}
	bodyFile := writeContinuationBody(t, bodyA)
	receipt := continuationToken(t, threadID, predecessor, opening.body, replyID, bodyA)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, opening, replyA)},
		providerStep{stdout: threadResponse(threadID, false, opening, replyB)},
		providerStep{stdout: resolveResponse(threadID, replyID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("edited provider body with the same reply ID was resolved")
	}
	assertQueryKinds(t, provider, "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("pre-resolve body edit printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "body", "changed")
	assertContainsNone(t, diagnostics, bodyA, bodyB)
}

func TestContinueResolveReopensWhenResolveResponseBodyChangedAtSameAnchor(t *testing.T) {
	const (
		threadID    = "PRRT_mutation_body_edit"
		predecessor = "PRRC_predecessor"
		replyID     = "PRRC_reply"
		bodyA       = "The exact body that earned the continuation receipt."
		bodyB       = "The body changed while the resolve mutation was in flight."
	)
	opening := providerComment{id: predecessor, login: "reviewer", body: "P1: bind the mutation response"}
	replyA := providerComment{id: replyID, login: "author", body: bodyA}
	replyB := providerComment{id: replyID, login: "author", body: bodyB}
	bodyFile := writeContinuationBody(t, bodyA)
	receipt := continuationToken(t, threadID, predecessor, opening.body, replyID, bodyA)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, opening, replyA)},
		providerStep{stdout: threadResponse(threadID, false, opening, replyA)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, opening, replyB)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("resolve response with an edited body was accepted")
	}
	assertQueryKinds(t, provider, "history", "thread", "resolve", "unresolve")
	assertNoReplyMutation(t, provider)
	assertResolveMutationRequestsExactComments(t, provider)
	if stdout != "" {
		t.Errorf("stale resolve response printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "body", "changed", "reopened")
	assertContainsNone(t, diagnostics, bodyA, bodyB)
}

func TestContinueResolveRefusesPredecessorBodyEditAfterReceipt(t *testing.T) {
	const (
		threadID      = "PRRT_receipt_predecessor_edit"
		predecessorID = "PRRC_receipt_predecessor"
		replyID       = "PRRC_receipt_reply"
		originalPoint = "P1: bind the review point represented by this comment ID."
		editedPoint   = "P1: this same comment ID now requests different work."
		replyBody     = "The posted answer addresses only the original review point."
	)
	before := providerComment{id: predecessorID, login: "reviewer", body: originalPoint}
	after := providerComment{id: predecessorID, login: "reviewer", body: editedPoint}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		// Establish a provider-visible reply and force receipt-bearing recovery.
		providerStep{stdout: threadResponse(threadID, false, before)},
		providerStep{stdout: historyResponse(threadID, before)},
		providerStep{stdout: threadResponse(threadID, false, before)},
		providerStep{stdout: replyResponse(reply)},
		providerStep{stdout: threadResponse(threadID, false, before, reply)},
		providerStep{stdout: threadResponse(threadID, false, before, reply)},
		providerStep{stderr: "connection reset after resolve", exit: 1},
		providerStep{stdout: threadResponse(threadID, false, before, reply)},
		// The predecessor is edited in place before the continuation is replayed.
		providerStep{stdout: historyResponse(threadID, after, reply)},
		// These forbidden resolve responses keep the old behavior deterministic.
		providerStep{stdout: threadResponse(threadID, false, after, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, after, reply)},
	)

	firstCode, firstStdout, firstDiagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if firstCode == 0 {
		t.Fatal("transient resolution failure unexpectedly succeeded")
	}
	if firstStdout != "" {
		t.Fatalf("transient resolution failure printed success output: %q", firstStdout)
	}
	receiptToken := emittedContinuationReceiptToken(t, firstDiagnostics)
	requestsBeforeContinuation := provider.requestCount()

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receiptToken, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("continuation resolved after its predecessor body was edited in place")
	}
	continuationRequests := provider.requests()[requestsBeforeContinuation:]
	for _, kind := range queryKinds(continuationRequests) {
		switch kind {
		case "reply", "resolve", "unresolve":
			t.Errorf("predecessor-body refusal issued forbidden %s mutation", kind)
		}
	}
	if got := queryKinds(continuationRequests); len(got) != 2 || got[0] != "history" || got[1] != "thread" {
		t.Errorf("continuation requests after predecessor edit = %q, want [history thread]", got)
	}
	if stdout != "" {
		t.Errorf("predecessor-body refusal printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "predecessor", "no mutation")
	assertContainsNone(t, firstDiagnostics+diagnostics, originalPoint, editedPoint, replyBody)
}

func TestReplyResolveRefusesTrimEquivalentByteDifferentTail(t *testing.T) {
	const (
		threadID    = "PRRT_trim_equivalent"
		postedBody  = "The provider-visible exact answer."
		requested   = "  The provider-visible exact answer.\n"
		predecessor = "PRRC_predecessor"
		replyID     = "PRRC_reply"
	)
	opening := providerComment{id: predecessor, login: "reviewer", body: "P1: keep exact byte identity"}
	reply := providerComment{id: replyID, login: "author", body: postedBody}
	bodyFile := writeContinuationBody(t, requested)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: historyResponse(threadID, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		// The old trim-equivalent classifier adopts this different body as the
		// requested reply and reaches resolution.
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: resolveResponse(threadID, replyID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("trim-equivalent but byte-different provider answer was adopted")
	}
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("byte-different answer printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "restore", "exact original bytes", "continuation receipt")
	assertContainsNone(t, diagnostics, postedBody, requested)
}

func TestReplyResolveResolvedShortcutRefusesSameIDTrimEquivalentBodyEdit(t *testing.T) {
	const (
		threadID    = "PRRT_resolved_trim_edit"
		predecessor = "PRRC_resolved_predecessor"
		replyID     = "PRRC_resolved_reply"
		body        = "The exact answer originally observed."
		editedBody  = "  The exact answer originally observed.\n"
	)
	opening := providerComment{id: predecessor, login: "reviewer", body: "P1: retain exact resolved evidence"}
	originalReply := providerComment{id: replyID, login: "author", body: body}
	editedReply := providerComment{id: replyID, login: "author", body: editedBody}
	bodyFile := writeContinuationBody(t, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, true, opening, originalReply)},
		providerStep{stdout: historyResponse(threadID, opening, originalReply)},
		providerStep{stdout: threadResponse(threadID, true, opening, editedReply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("resolved shortcut claimed success after the same reply ID acquired byte-different content")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	assertContainsNone(t, stdout, "skipped", "resolved")
	assertContainsAll(t, strings.ToLower(diagnostics), "body", "changed")
	assertContainsNone(t, diagnostics, body, editedBody)
}

func TestReplyResolveEmitsReplayableContinuationReceipt(t *testing.T) {
	const (
		threadID      = "PRRT_emitted_continuation"
		predecessorID = "PRRC_emitted_predecessor"
		replyID       = "PRRC_emitted_reply"
		replyBody     = "EXACT-REPLAYABLE-UTF8-🧪-e\u0301"
	)
	opening := providerComment{id: predecessorID, login: "reviewer", body: "P1: make the emitted receipt replayable"}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: replyResponse(reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stderr: "connection reset after resolve", exit: 1},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: historyResponse(threadID, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, opening, reply)},
	)

	firstCode, firstStdout, firstDiagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if firstCode == 0 {
		t.Fatal("transient resolution failure unexpectedly succeeded")
	}
	if firstStdout != "" {
		t.Fatalf("transient resolution failure printed success output: %q", firstStdout)
	}
	assertContainsAll(t, firstDiagnostics, "continuation_receipt=", "continue-resolve", "NOT resolved")
	assertContainsNone(t, firstDiagnostics, replyBody)

	receiptToken := emittedContinuationReceiptToken(t, firstDiagnostics)
	receipt, err := decodeContinuationReceipt(receiptToken)
	if err != nil {
		t.Fatalf("decode emitted continuation receipt: %v", err)
	}
	if receipt.ThreadID != threadID || receipt.PredecessorID != predecessorID || receipt.ReplyID != replyID {
		t.Fatalf("emitted receipt identities = %#v, want thread=%q predecessor=%q reply=%q",
			receipt, threadID, predecessorID, replyID)
	}
	if receipt.BodySHA256 != exactBodySHA256([]byte(replyBody)) {
		t.Fatalf("emitted receipt body digest = %q, want exact source digest", receipt.BodySHA256)
	}
	if receipt.PredecessorBodySHA256 != exactBodySHA256([]byte(opening.body)) {
		t.Fatalf("emitted receipt predecessor digest = %q, want exact provider predecessor digest", receipt.PredecessorBodySHA256)
	}
	payloadToken, _, found := strings.Cut(receiptToken, ".")
	if !found {
		t.Fatal("emitted receipt omitted its authenticated signature")
	}
	rawReceipt, err := base64.RawURLEncoding.DecodeString(payloadToken)
	if err != nil {
		t.Fatalf("decode emitted receipt payload: %v", err)
	}
	assertContainsNone(t, string(rawReceipt), opening.body, replyBody, bodyFile)

	requestsBeforeContinuation := provider.requestCount()
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receiptToken, "--body-file", bodyFile})
	})
	if code != 0 {
		t.Fatalf("emitted continuation receipt could not be replayed: code=%d\nstdout:\n%s\nstderr:\n%s",
			code, stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider,
		"thread", "history", "thread", "reply", "thread", "thread", "resolve", "thread",
		"history", "thread", "resolve",
	)
	assertReplyMutationBody(t, provider, replyBody)
	replyMutations := 0
	for _, kind := range queryKinds(provider.requests()) {
		if kind == "reply" {
			replyMutations++
		}
	}
	if replyMutations != 1 {
		t.Errorf("receipt replay produced %d total reply mutations, want exactly the original one", replyMutations)
	}
	for _, request := range provider.requests()[requestsBeforeContinuation:] {
		rawVariables, err := json.Marshal(request.Variables)
		if err != nil {
			t.Fatalf("encode continuation request variables: %v", err)
		}
		assertContainsNone(t, string(rawVariables), receiptToken, bodyFile, replyBody)
	}
	assertContainsAll(t, stdout, "resolved "+threadID)
	assertContainsNone(t, diagnostics, replyBody)
}

func TestContinueResolveUsesReceiptWithoutReplyMutation(t *testing.T) {
	const (
		threadID    = "PRRT_continue"
		predecessor = "PRRC_reviewer"
		replyID     = "PRRC_answer"
		body        = "Fixed by validating the provider-backed continuation evidence."
	)
	opening := providerComment{id: predecessor, login: "reviewer", body: "P1: make retries evidence-bound"}
	reply := providerComment{id: replyID, login: "author", body: body}
	bodyFile := writeContinuationBody(t, body)
	receipt := continuationToken(t, threadID, predecessor, opening.body, replyID, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, opening, reply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code != 0 {
		t.Errorf("valid continuation failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout, diagnostics)
	}
	assertQueryKinds(t, provider, "history", "thread", "resolve")
	assertQueryTargets(t, provider, threadID)
	assertNoReplyMutation(t, provider)
	assertContainsAll(t, stdout, "resolved "+threadID)
	assertContainsNone(t, diagnostics, body)
}

func TestContinueResolveRejectsChangedNamedBodyBeforeProviderAccess(t *testing.T) {
	const (
		original = "Original reply bytes that were posted."
		changed  = "Changed reply bytes must not inherit the old receipt."
	)
	bodyFile := writeContinuationBody(t, changed)
	receipt := continuationToken(t, "PRRT_named_mismatch", "PRRC_before", "original predecessor bytes", "PRRC_reply", original)
	provider := installSequencedProvider(t)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("changed named body was accepted by the original continuation receipt")
	}
	if got := provider.requestCount(); got != 0 {
		t.Errorf("body/receipt mismatch made %d provider request(s), want zero", got)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "receipt", "body", "mismatch")
	assertContainsNone(t, stdout+diagnostics, original, changed)
}

func TestContinueResolveRejectsChangedStdinBeforeProviderAccess(t *testing.T) {
	const (
		original = "Original retained standard-input bytes."
		changed  = "Regenerated standard input must not inherit the old receipt."
	)
	receipt := continuationToken(t, "PRRT_stdin_mismatch", "PRRC_before", "original predecessor bytes", "PRRC_reply", original)
	provider := installSequencedProvider(t)

	code, stdout, diagnostics := provider.capture(func() int {
		return runWithContinuationStdin(t, changed, func() int {
			return run([]string{"continue-resolve", receipt, "--body-file", "-"})
		})
	})
	if code == 0 {
		t.Error("changed standard input was accepted by the original continuation receipt")
	}
	if got := provider.requestCount(); got != 0 {
		t.Errorf("stdin/receipt mismatch made %d provider request(s), want zero", got)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "receipt", "body", "mismatch")
	assertContainsNone(t, stdout+diagnostics, original, changed)
}

func TestReplyResolveRefusesChangedBodyAfterProviderVisibleReply(t *testing.T) {
	const (
		threadID = "PRRT_changed_retry"
		oldBody  = "The first reply is already visible."
		newBody  = "A mutable source supplied different bytes on retry."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P2: preserve retry identity"}
	oldReply := providerComment{id: "PRRC_old_reply", login: "author", body: oldBody}
	newReply := providerComment{id: "PRRC_duplicate", login: "author", body: newBody}
	bodyFile := writeContinuationBody(t, newBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply)},
		providerStep{stdout: historyResponse(threadID, opening, oldReply)},
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply)},
		// These responses keep the old implementation deterministic if it reaches
		// the forbidden second reply and resolution mutations.
		providerStep{stdout: replyResponse(newReply)},
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply, newReply)},
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply, newReply)},
		providerStep{stdout: resolveResponse(threadID, newReply.id)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("changed ordinary retry posted and resolved a second reply")
	}
	assertQueryKindsPrefix(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	assertContainsAll(t, strings.ToLower(diagnostics), "different", "answer", "nothing was posted")
	assertContainsNone(t, stdout+diagnostics, oldBody, newBody)
}

func TestReplyResolveAllowsRevisedBodyAfterReviewerHandback(t *testing.T) {
	const (
		threadID = "PRRT_reviewer_handback"
		oldBody  = "Initial answer."
		newBody  = "Revised answer after reading the reviewer follow-up."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P2: cover the race"}
	oldReply := providerComment{id: "PRRC_old_reply", login: "author", body: oldBody}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "The first answer misses stdin."}
	newReply := providerComment{id: "PRRC_new_reply", login: "author", body: newBody}
	bodyFile := writeContinuationBody(t, newBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply, followup)},
		providerStep{stdout: historyResponse(threadID, opening, oldReply, followup)},
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply, followup)},
		providerStep{stdout: replyResponse(newReply)},
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply, followup, newReply)},
		providerStep{stdout: threadResponse(threadID, false, opening, oldReply, followup, newReply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, followup, newReply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code != 0 {
		t.Errorf("revised reply after reviewer hand-back failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "thread", "thread", "resolve")
	assertReplyMutationBody(t, provider, newBody)
	assertContainsAll(t, stdout, "resolved "+threadID)
}

func TestReplyResolveRefusesMissingAuthorEvidenceBeforeMutation(t *testing.T) {
	tests := []struct {
		name         string
		openingLogin string
		answerLogin  string
	}{
		{name: "opening login missing", openingLogin: "", answerLogin: "author"},
		{name: "latest login missing", openingLogin: "reviewer", answerLogin: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const (
				threadID = "PRRT_missing_login"
				newBody  = "Do not post when answer identity is unavailable."
			)
			opening := providerComment{id: "PRRC_opening", login: tc.openingLogin, body: "P2: prove the actor"}
			answer := providerComment{id: "PRRC_answer", login: tc.answerLogin, body: "An existing answer."}
			newReply := providerComment{id: "PRRC_new_reply", login: "author", body: newBody}
			bodyFile := writeContinuationBody(t, newBody)
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, opening, answer)},
				providerStep{stdout: historyResponse(threadID, opening, answer)},
				providerStep{stdout: threadResponse(threadID, false, opening, answer)},
				providerStep{stdout: replyResponse(newReply)},
				providerStep{stdout: threadResponse(threadID, false, opening, answer, newReply)},
				providerStep{stdout: threadResponse(threadID, false, opening, answer, newReply)},
				providerStep{stdout: resolveResponse(threadID, newReply.id)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Error("missing author evidence licensed a new reply")
			}
			assertQueryKindsPrefix(t, provider, "thread", "history", "thread")
			assertNoProviderMutation(t, provider)
			assertContainsAll(t, strings.ToLower(diagnostics), "author", "inspect")
			assertContainsNone(t, stdout+diagnostics, newBody)
		})
	}
}

func TestReplyResolveDoesNotAdoptMatchingReviewerText(t *testing.T) {
	const (
		threadID = "PRRT_matching_reviewer_text"
		body     = "This exact text is the requested answer."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: body}
	reply := providerComment{id: "PRRC_reply", login: "author", body: body}
	bodyFile := writeContinuationBody(t, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: historyResponse(threadID, opening)},
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: replyResponse(reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, opening, reply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code != 0 {
		t.Errorf("reviewer-body collision prevented a legitimate first reply: code %d\nstdout:\n%s\nstderr:\n%s", code, stdout, diagnostics)
	}
	assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "thread", "thread", "resolve")
	assertReplyMutationBody(t, provider, body)
	assertContainsNone(t, diagnostics, "reply already present")
}

func TestReplyResolveResolvedDifferentAnswerDoesNotReopenOrClaimSuccess(t *testing.T) {
	const (
		threadID = "PRRT_resolved_different_answer"
		oldBody  = "The provider-visible answer."
		newBody  = "Different local bytes must not be claimed as complete."
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P2: bind the continuation"}
	answer := providerComment{id: "PRRC_answer", login: "author", body: oldBody}
	bodyFile := writeContinuationBody(t, newBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, true, opening, answer)},
		providerStep{stdout: historyResponse(threadID, opening, answer)},
		providerStep{stdout: threadResponse(threadID, true, opening, answer)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Error("resolved thread with a different provider answer claimed the requested body completed")
	}
	assertQueryKindsPrefix(t, provider, "thread", "history", "thread")
	assertNoProviderMutation(t, provider)
	assertContainsNone(t, stdout, "resolved", "skipped")
	assertContainsAll(t, strings.ToLower(diagnostics), "different", "answer", "nothing was posted")
	assertContainsNone(t, diagnostics, oldBody, newBody)
}
