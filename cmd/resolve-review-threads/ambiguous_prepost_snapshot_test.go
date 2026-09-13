package main

import (
	"context"
	"strings"
	"testing"
)

func TestAmbiguousAbsentReplyBindsFullPrePostPredecessorSnapshot(t *testing.T) {
	const (
		threadID      = "PRRT_ambiguous_full_prepost_snapshot"
		predecessorID = "PRRC_ambiguous_full_prepost_predecessor"
		replyBody     = "The selected source answers the pre-post predecessor."
		bodyFile      = "/tmp/resolve-review-thread.FULL-PREPOST"
		originalTime  = "2026-09-09T00:00:00Z"
		changedTime   = "2026-09-09T00:00:01Z"
	)
	opening := providerComment{
		id: "PRRC_ambiguous_full_prepost_opening", login: "reviewer", body: "P1: opening point.",
	}
	basePredecessor := providerComment{
		id: predecessorID, login: "reviewer", body: "P1: current reviewer hand-back.", updatedAt: originalTime,
	}
	editedPredecessor := basePredecessor
	editedPredecessor.editCount = 1
	editedPredecessor.lastEditID = "UCE_ambiguous_predecessor_edit_1"
	originalEditedPredecessor := editedPredecessor
	changedLatestEdit := originalEditedPredecessor
	changedLatestEdit.lastEditID = "UCE_ambiguous_predecessor_edit_1_replaced"

	tests := []struct {
		name                string
		expectedPredecessor providerComment
		observedOpening     providerComment
		observedPredecessor providerComment
		resolved            bool
		want                string
		wantReopen          bool
		incomplete          bool
	}{
		{
			name:                "same ID and body with changed update timestamp",
			expectedPredecessor: basePredecessor,
			observedOpening:     opening,
			observedPredecessor: func() providerComment {
				changed := basePredecessor
				changed.updatedAt = changedTime
				return changed
			}(),
			resolved: true, want: "changed its update timestamp", wantReopen: true,
		},
		{
			name:                "same ID body and timestamp with changed edit generation",
			expectedPredecessor: basePredecessor,
			observedOpening:     opening, observedPredecessor: editedPredecessor,
			want: "changed its edit revision",
		},
		{
			name:                "same edit count with changed latest edit ID",
			expectedPredecessor: originalEditedPredecessor,
			observedOpening:     opening, observedPredecessor: changedLatestEdit,
			resolved: true, want: "changed its edit revision", wantReopen: true,
		},
		{
			name:                "changed stable opening author",
			expectedPredecessor: basePredecessor,
			observedOpening: providerComment{
				id: opening.id, login: "replacement-reviewer", body: opening.body,
			},
			observedPredecessor: basePredecessor,
			want:                "opening author changed",
		},
		{
			name:                "changed stable tail author",
			expectedPredecessor: basePredecessor,
			observedOpening:     opening,
			observedPredecessor: providerComment{
				id: predecessorID, login: "replacement-reviewer", body: basePredecessor.body,
				updatedAt: originalTime,
			},
			resolved: true, want: "tail author changed", wantReopen: true,
		},
		{
			name:                "missing stable tail author is incomplete",
			expectedPredecessor: basePredecessor,
			observedOpening:     opening,
			observedPredecessor: providerComment{
				id: predecessorID, body: basePredecessor.body, updatedAt: originalTime,
			},
			resolved: true, want: "omitted the stable tail author", incomplete: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			predecessor := testReplyIssuancePredecessor(tc.expectedPredecessor, replyBody)
			predecessor.OpeningAuthor = opening.login
			steps := []providerStep{
				{stderr: "transport dropped before acknowledgement", exit: 1},
				{stdout: historyResponse(threadID, tc.observedOpening, tc.observedPredecessor)},
				{stdout: threadResponse(threadID, tc.resolved, tc.observedOpening, tc.observedPredecessor)},
			}
			if tc.wantReopen {
				steps = append(steps, providerStep{stdout: unresolveResponse(threadID)})
			}
			provider := installSequencedProvider(t, steps...)

			code, stdout, diagnostics := provider.capture(func() int {
				_, postCode := postReplyOrExit(
					context.Background(), threadID, replyBody, predecessor, bodyFile,
				)
				return postCode
			})
			if code == 0 {
				t.Fatal("changed or incomplete predecessor snapshot licensed an unchanged reply retry")
			}
			provider.assertExhausted()
			wantKinds := []string{"reply", "history", "thread"}
			if tc.wantReopen {
				wantKinds = append(wantKinds, "unresolve")
			}
			assertQueryKinds(t, provider, wantKinds...)
			if stdout != "" {
				t.Errorf("ambiguous predecessor recovery printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower, strings.ToLower(tc.want))
			assertContainsNone(t, diagnostics,
				opening.body,
				basePredecessor.body,
				replyBody,
				"an exact-body retry remains applicable",
			)
			if tc.incomplete {
				assertContainsAll(t, lower,
					"provider state is unverified",
					"both recovery reads must reproduce the full pre-post predecessor snapshot",
					"inspect the live thread before any retry",
				)
				assertContainsNone(t, lower, "revised-answer lifecycle", "confirmed reopened")
				return
			}
			assertContainsAll(t, lower,
				"full pre-post predecessor snapshot changed",
				"same-source revised-answer lifecycle",
			)
			if tc.wantReopen {
				assertContainsAll(t, lower, "confirmed reopened")
			}
		})
	}
}

func TestAmbiguousAbsentReplyRequiresBothRecoveryReadsToMatchSnapshot(t *testing.T) {
	const (
		threadID     = "PRRT_ambiguous_two_read_predecessor"
		replyBody    = "The retained answer is valid only for the original predecessor revision."
		bodyFile     = "/tmp/resolve-review-thread.TWO-READ-PREPOST"
		originalTime = "2026-09-09T00:00:00Z"
		changedTime  = "2026-09-09T00:00:01Z"
	)
	opening := providerComment{id: "PRRC_two_read_opening", login: "reviewer", body: "P1: opening point."}
	original := providerComment{
		id: "PRRC_two_read_predecessor", login: "reviewer", body: "P1: stable hand-back.", updatedAt: originalTime,
	}
	changedTimePredecessor := original
	changedTimePredecessor.updatedAt = changedTime
	changedEditPredecessor := original
	changedEditPredecessor.editCount = 1
	changedEditPredecessor.lastEditID = "UCE_two_read_changed_edit"
	missingAuthorPredecessor := original
	missingAuthorPredecessor.login = ""

	tests := []struct {
		name        string
		historyTail providerComment
		currentTail providerComment
		want        string
		incomplete  bool
	}{
		{
			name:        "first read changed while second read restored original",
			historyTail: changedTimePredecessor, currentTail: original,
			want: "full-history recovery read proved predecessor",
		},
		{
			name:        "first read original while second read changed",
			historyTail: original, currentTail: changedEditPredecessor,
			want: "exact-state recovery read proved predecessor",
		},
		{
			name:        "one read omits author while the other matches",
			historyTail: missingAuthorPredecessor, currentTail: original,
			want: "full-history recovery read omitted the stable tail author", incomplete: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			predecessor := testReplyIssuancePredecessor(original, replyBody)
			predecessor.OpeningAuthor = opening.login
			provider := installSequencedProvider(t,
				providerStep{stderr: "transport dropped before acknowledgement", exit: 1},
				providerStep{stdout: historyResponse(threadID, opening, tc.historyTail)},
				providerStep{stdout: threadResponse(threadID, false, opening, tc.currentTail)},
			)

			code, stdout, diagnostics := provider.capture(func() int {
				_, postCode := postReplyOrExit(
					context.Background(), threadID, replyBody, predecessor, bodyFile,
				)
				return postCode
			})
			if code == 0 {
				t.Fatal("one matching recovery read licensed an unchanged retry")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "reply", "history", "thread")
			if stdout != "" {
				t.Errorf("two-read mismatch printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower, strings.ToLower(tc.want))
			assertContainsNone(t, diagnostics,
				opening.body,
				original.body,
				replyBody,
				"an exact-body retry remains applicable",
			)
			if tc.incomplete {
				assertContainsAll(t, lower, "provider state is unverified", "inspect the live thread before any retry")
				assertContainsNone(t, lower, "revised-answer lifecycle")
				return
			}
			assertContainsAll(t, lower, "same-source revised-answer lifecycle")
		})
	}
}
