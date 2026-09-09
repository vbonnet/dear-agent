package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestContinueResolveTreatsMissingResolvedIdentityAsUnverified(t *testing.T) {
	const (
		threadID        = "PRRT_continuation_incomplete_reconcile"
		predecessorID   = "PRRC_incomplete_predecessor"
		predecessorBody = "P1: require complete evidence before corrective reopen."
		replyID         = "PRRC_incomplete_reply"
		replyBody       = "The receipt binds both exact comments."
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: predecessorBody}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_history_followup", login: "reviewer", body: "History reports a later comment."}

	for _, omitted := range []string{"last ID", "predecessor ID"} {
		t.Run(omitted, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, replyBody)
			receipt := continuationToken(t, threadID, predecessorID, predecessorBody, replyID, replyBody)
			provider := installSequencedProvider(t,
				providerStep{stdout: historyResponse(threadID, predecessor, reply, followup)},
				providerStep{stdout: resolvedThreadWithOmittedContinuationEvidence(
					threadID,
					predecessor,
					reply,
					omitted,
				)},
				// Correct reconciliation must not consume this mutation response.
				providerStep{stdout: unresolveResponse(threadID)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("incomplete resolved continuation state reported success")
			}
			assertQueryKinds(t, provider, "history", "thread")
			assertNoProviderMutation(t, provider)
			if stdout != "" {
				t.Errorf("incomplete reconciliation printed success output: %q", stdout)
			}
			assertContainsAll(t, strings.ToLower(diagnostics), "unverified", "no mutation")
			assertContainsNone(t, strings.ToLower(diagnostics), "automatically reopened", "stale evidence")
			assertContainsNone(t, diagnostics, predecessorBody, replyBody, followup.body)
		})
	}
}

func TestContinueResolveCarriesConclusiveHistoryContradictionAcrossOmittedCurrentEvidence(t *testing.T) {
	const (
		threadID        = "PRRT_continuation_conclusive_reconcile"
		predecessorID   = "PRRC_conclusive_predecessor"
		predecessorBody = "P1: preserve a conclusive history contradiction."
		replyID         = "PRRC_conclusive_reply"
		replyBody       = "The receipt binds the original exact boundary."
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: predecessorBody}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	intervening := providerComment{id: "PRRC_intervening", login: "reviewer", body: "An intervening comment."}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "A later follow-up."}

	tests := []struct {
		name       string
		history    []providerComment
		wantDetail string
	}{
		{
			name:       "duplicate placement",
			history:    []providerComment{predecessor, predecessor, reply},
			wantDetail: "duplicated the named predecessor",
		},
		{
			name:       "broken adjacency",
			history:    []providerComment{predecessor, intervening, reply},
			wantDetail: "directly follows",
		},
		{
			name:       "reply no longer current tail",
			history:    []providerComment{predecessor, reply, followup},
			wantDetail: "current tail",
		},
		{
			name: "predecessor body changed",
			history: []providerComment{
				{id: predecessor.id, login: predecessor.login, body: predecessor.body + " Changed."},
				reply,
			},
			wantDetail: "predecessor",
		},
		{
			name: "update timestamp changed",
			history: []providerComment{
				{id: predecessor.id, login: predecessor.login, body: predecessor.body, updatedAt: "2026-09-09T00:00:01Z"},
				reply,
			},
			wantDetail: "update timestamp changed",
		},
		{
			name: "edit revision changed",
			history: []providerComment{
				{id: predecessor.id, login: predecessor.login, body: predecessor.body, editCount: 1, lastEditID: "UCE_predecessor_edit_1"},
				reply,
			},
			wantDetail: "edit revision changed",
		},
		{
			name: "author changed",
			history: []providerComment{
				{id: predecessor.id, login: "different-reviewer", body: predecessor.body},
				reply,
			},
			wantDetail: "author evidence",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, replyBody)
			receipt := continuationToken(t, threadID, predecessorID, predecessorBody, replyID, replyBody)
			provider := installSequencedProvider(t,
				providerStep{stdout: historyResponse(threadID, tc.history...)},
				// The later exact read preserves both named IDs but omits an
				// unrelated current body. It cannot erase the complete
				// contradiction already established by full history.
				providerStep{stdout: resolvedThreadWithOmittedContinuationEvidence(
					threadID,
					predecessor,
					reply,
					"last body",
				)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("conclusive history contradiction reported success")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "history", "thread", "unresolve")
			assertNoReplyOrResolveMutation(t, provider)
			if stdout != "" {
				t.Errorf("conclusive history contradiction printed success output: %q", stdout)
			}
			assertContainsAll(t, strings.ToLower(diagnostics), strings.ToLower(tc.wantDetail), "automatically reopened")
			assertContainsNone(t, diagnostics, predecessorBody, replyBody, intervening.body, followup.body)
		})
	}
}

func TestContinueResolveLeavesResolvedThreadUnchangedWhenFullHistoryIsIncomplete(t *testing.T) {
	const (
		threadID        = "PRRT_continuation_incomplete_history"
		predecessorID   = "PRRC_incomplete_history_predecessor"
		predecessorBody = "P1: incomplete history must not authorize correction."
		replyID         = "PRRC_incomplete_history_reply"
		replyBody       = "The receipt names complete evidence."
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: predecessorBody}
	reply := providerComment{id: replyID, login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_incomplete_history_followup", login: "reviewer", body: "Current state differs."}

	tests := []struct {
		name    string
		history []providerComment
	}{
		{name: "named predecessor missing", history: []providerComment{reply}},
		{name: "named reply missing", history: []providerComment{predecessor}},
		{
			name: "named update timestamp omitted",
			history: []providerComment{
				{id: predecessor.id, login: predecessor.login, body: predecessor.body, omitUpdatedAt: true},
				reply,
			},
		},
		{
			name: "named edit revision omitted",
			history: []providerComment{
				predecessor,
				{id: reply.id, login: reply.login, body: reply.body, omitEditCount: true},
			},
		},
		{
			name: "named author omitted",
			history: []providerComment{
				{id: predecessor.id, body: predecessor.body},
				reply,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := writeContinuationBody(t, replyBody)
			receipt := continuationToken(t, threadID, predecessorID, predecessorBody, replyID, replyBody)
			provider := installSequencedProvider(t,
				providerStep{stdout: historyResponse(threadID, tc.history...)},
				// Even a complete current mismatch cannot upgrade an
				// incomplete full-history read into corrective authority.
				providerStep{stdout: threadResponse(threadID, true, predecessor, reply, followup)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatal("incomplete full history reported success")
			}
			assertQueryKinds(t, provider, "history", "thread")
			assertNoProviderMutation(t, provider)
			if stdout != "" {
				t.Errorf("incomplete full history printed success output: %q", stdout)
			}
			assertContainsAll(t, strings.ToLower(diagnostics), "unverified", "left unchanged", "no mutation")
			assertContainsNone(t, strings.ToLower(diagnostics), "automatically reopened", "stale evidence")
			assertContainsNone(t, diagnostics, predecessorBody, replyBody, followup.body)
		})
	}
}

func resolvedThreadWithOmittedContinuationEvidence(
	threadID string,
	predecessor, reply providerComment,
	omitted string,
) string {
	recent := []string{
		providerCommentWithOmittedEvidence(predecessor, omitted == "predecessor ID", omitted == "predecessor body"),
		providerCommentWithOmittedEvidence(reply, omitted == "last ID", omitted == "last body"),
	}
	opening := fmt.Sprintf(`{"author":{"login":%q},"body":%q}`, predecessor.login, predecessor.body)
	return fmt.Sprintf(
		`{"data":{"node":{"id":%q,"isResolved":true,"isOutdated":false,"path":"review.go","opening":{"totalCount":2,"nodes":[%s]},"recent":{"nodes":[%s]}}}}`,
		threadID,
		opening,
		strings.Join(recent, ","),
	)
}

func providerCommentWithOmittedEvidence(comment providerComment, omitID, omitBody bool) string {
	fields := make([]string, 0, 5)
	if !omitID {
		fields = append(fields, fmt.Sprintf(`"id":%q`, comment.id))
	}
	fields = append(fields, fmt.Sprintf(`"author":{"login":%q}`, comment.login))
	if !omitBody {
		fields = append(fields, fmt.Sprintf(`"body":%q`, comment.body))
	}
	fields = append(fields, fmt.Sprintf(`"updatedAt":%q`, comment.providerUpdatedAt()))
	if editEvidence := strings.TrimPrefix(comment.providerEditEvidence(), ","); editEvidence != "" {
		fields = append(fields, editEvidence)
	}
	return "{" + strings.Join(fields, ",") + "}"
}
