package main

import (
	"context"
	"strings"
	"testing"
)

func TestResolveAccessDenialCannotBeRecoveredAsAppliedMutation(t *testing.T) {
	const (
		threadID = "PRRT_denied_concurrent_resolution"
		denial   = "gh: GraphQL: Resource not accessible by personal access token"
	)
	predecessor := providerComment{
		id:    "PRRC_denied_predecessor",
		login: "reviewer",
		body:  "P1: preserve access-denial attribution.",
	}
	reply := providerComment{
		id:    "PRRC_denied_reply",
		login: "author",
		body:  "The review point is addressed.",
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		providerStep{stderr: denial + "\n", exit: 1},
		// A concurrent actor resolved the same unchanged boundary. That terminal
		// state must not turn this caller's explicitly denied mutation into an
		// attributed success.
		providerStep{stdout: threadResponse(threadID, true, predecessor, reply)},
	)

	msg, mutated, err := resolveWithEvidence(
		context.Background(),
		threadID,
		false,
		resolutionEvidence{
			LastID:                reply.id,
			PredecessorID:         predecessor.id,
			PredecessorBodySHA256: exactBodySHA256([]byte(predecessor.body)),
			BodySHA256:            exactBodySHA256([]byte(reply.body)),
		},
	)
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "resolve", "thread")
	if err == nil {
		t.Fatal("typed access denial was recovered as a successful resolution")
	}
	if mutated {
		t.Fatal("typed access denial reported mutated=true after a concurrent resolution")
	}
	if !isAccessDenied(err) {
		t.Fatalf("reconciled resolution lost its typed access-denial cause: %v", err)
	}
	if strings.Contains(msg, "had already applied") || strings.Contains(msg, "resolved "+threadID) {
		t.Fatalf("denied mutation was attributed as successful: %q", msg)
	}
	assertContainsNone(t, err.Error(), predecessor.body, reply.body, denial)
}

func TestFailedCorrectiveReopenPreservesAccessDenialCause(t *testing.T) {
	const (
		threadID = "PRRT_denied_corrective_reopen"
		denial   = "gh: GraphQL: Resource not accessible by personal access token"
	)
	comment := providerComment{
		id:    "PRRC_denied_reopen_tail",
		login: "reviewer",
		body:  "private reviewer text that must not enter diagnostics",
	}
	provider := installSequencedProvider(t,
		providerStep{stderr: denial + "\n", exit: 1},
		providerStep{stdout: threadResponse(threadID, true, comment)},
	)

	err := reopenOrFail(
		context.Background(),
		threadID,
		"review.go",
		"resolved on invalidated exact reply evidence",
		answerChangedEvidence,
	)
	provider.assertExhausted()
	assertQueryKinds(t, provider, "unresolve", "thread")
	if err == nil {
		t.Fatal("access-denied corrective reopen unexpectedly succeeded")
	}
	if !matchesErrorType[*failedReopenError](err) {
		t.Fatalf("corrective reopen error type = %T, want *failedReopenError", err)
	}
	if !isAccessDenied(err) {
		t.Fatalf("corrective reopen lost its typed access-denial cause: %v", err)
	}
	assertContainsNone(t, err.Error(), comment.body, denial)

	guidance, handled := replyResolutionEvidenceFailure(
		threadID,
		err,
		continuationRecoveryGuidance(threadID, "SAFE_RECEIPT", "/tmp/reply.md"),
	)
	if !handled {
		t.Fatalf("failed corrective reopen was not mapped to recovery guidance: %v", err)
	}
	assertContainsAll(t, guidance,
		"reopening the resolved thread failed",
		"Repair `gh` credentials",
		"unchanged credentials will be denied again",
	)
	assertContainsNone(t, guidance, comment.body, denial)
}

func TestDeniedResolveWithChangedUnresolvedEvidencePreservesAccessCause(t *testing.T) {
	const (
		denial                = "gh: GraphQL: Resource not accessible by personal access token"
		movedTailBody         = "private follow-up that must not enter diagnostics"
		editedPredecessorBody = "private edited predecessor that must not enter diagnostics"
	)

	for _, tc := range []struct {
		name        string
		threadID    string
		freshBody   string
		fresh       func(string, providerComment, providerComment) string
		wantErrType func(error) bool
	}{
		{
			name:      "moved tail",
			threadID:  "PRRT_denied_moved_tail",
			freshBody: movedTailBody,
			fresh: func(threadID string, predecessor, reply providerComment) string {
				followup := providerComment{
					id:    "PRRC_denied_followup",
					login: "reviewer",
					body:  movedTailBody,
				}
				return threadResponse(threadID, false, predecessor, reply, followup)
			},
			wantErrType: matchesErrorType[*supersededEvidenceError],
		},
		{
			name:      "same ID digest change",
			threadID:  "PRRT_denied_digest_change",
			freshBody: editedPredecessorBody,
			fresh: func(threadID string, predecessor, reply providerComment) string {
				predecessor.body = editedPredecessorBody
				return threadResponse(threadID, false, predecessor, reply)
			},
			wantErrType: matchesErrorType[*invalidatedReplyEvidenceError],
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			predecessor := providerComment{
				id:    "PRRC_denied_predecessor",
				login: "reviewer",
				body:  "private original predecessor that must not enter diagnostics",
			}
			reply := providerComment{
				id:    "PRRC_denied_reply",
				login: "author",
				body:  "private reply that must not enter diagnostics",
			}
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(tc.threadID, false, predecessor, reply)},
				providerStep{stderr: denial + "\n", exit: 1},
				providerStep{stdout: tc.fresh(tc.threadID, predecessor, reply)},
			)

			msg, mutated, err := resolveWithEvidence(
				context.Background(),
				tc.threadID,
				false,
				resolutionEvidence{
					LastID:                reply.id,
					PredecessorID:         predecessor.id,
					PredecessorBodySHA256: exactBodySHA256([]byte(predecessor.body)),
					BodySHA256:            exactBodySHA256([]byte(reply.body)),
				},
			)
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "thread")
			if err == nil {
				t.Fatal("denied resolve with changed evidence unexpectedly succeeded")
			}
			if mutated {
				t.Fatal("denied resolve with changed evidence reported mutated=true")
			}
			if msg != "" {
				t.Fatalf("denied resolve with changed evidence returned success text: %q", msg)
			}
			if !tc.wantErrType(err) {
				t.Fatalf("changed-evidence error type = %T, want case-specific evidence error: %v", err, err)
			}
			if !isAccessDenied(err) {
				t.Fatalf("changed-evidence error lost its typed access-denial cause: %v", err)
			}
			assertContainsNone(t, err.Error(), predecessor.body, reply.body, tc.freshBody, denial)

			guidance, handled := replyResolutionEvidenceFailure(
				tc.threadID,
				err,
				continuationRecoveryGuidance(tc.threadID, "SAFE_RECEIPT", "/tmp/reply.md"),
			)
			if !handled {
				t.Fatalf("changed-evidence error was not mapped to recovery guidance: %v", err)
			}
			assertContainsAll(t, guidance,
				"Repair `gh` credentials",
				"unchanged credentials will be denied again",
			)
			assertContainsNone(t, guidance, predecessor.body, reply.body, tc.freshBody, denial)
		})
	}
}
