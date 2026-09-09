package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostReplyRecoveryMatrixPreservesOriginalTail(t *testing.T) {
	const (
		threadID    = "PRRT_recovery"
		originalID  = "PRRC_original"
		recoveredID = "PRRC_recovered"
		body        = "Addressed with a descriptor-bound read."
		customPath  = "/private/tmp/custom reply's body"
		canonical   = "/tmp/resolve-review-thread.ABC123"
	)
	original := providerComment{id: originalID, login: "reviewer", body: "P2: close the race"}
	recovered := providerComment{id: recoveredID, login: "author", body: body}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "This still races."}

	t.Run("missing id recovers exact reply and verifies against original tail", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: replyResponse(providerComment{})},
			providerStep{stdout: historyResponse(threadID, original, recovered)},
			providerStep{stdout: threadResponse(threadID, false, original, recovered)},
		)
		var gotID string
		code, _, diagnostics := provider.capture(func() int {
			var postCode int
			gotID, postCode = postReplyOrExit(
				context.Background(), threadID, body,
				testReplyIssuancePredecessor(original, body), customPath,
			)
			if postCode >= 0 {
				return postCode
			}
			return verifyExactReplyPlacement(
				context.Background(),
				threadID,
				resolutionEvidence{
					LastID:                gotID,
					PredecessorID:         originalID,
					PredecessorBodySHA256: exactBodySHA256([]byte(original.body)),
					BodySHA256:            exactBodySHA256([]byte(recovered.body)),
				},
				customPath,
			)
		})
		if code != 0 {
			t.Fatalf("recovered reply placement failed with code %d:\n%s", code, diagnostics)
		}
		if gotID != recoveredID {
			t.Fatalf("recovered reply ID = %q, want %q", gotID, recoveredID)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history", "thread")
		assertContainsNone(t, diagnostics, "Retry with:", "revise the same named body source", "safe-merge")
	})

	t.Run("moved tail without exact reply revises same custom source", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: "transport dropped after write", exit: 1},
			providerStep{stdout: historyResponse(threadID, original, followup)},
			providerStep{stdout: threadResponse(threadID, false, original, followup)},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body,
				testReplyIssuancePredecessor(original, body), customPath,
			)
			return postCode
		})
		if code == 0 {
			t.Fatalf("moved-tail ambiguity succeeded:\n%s", diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history", "thread")
		assertContainsAll(t, diagnostics,
			"revise the same named body source in place",
			`reply_file='/private/tmp/custom reply'"'"'s body'`,
			`--body-file "$reply_file"`,
		)
		assertContainsNone(t, diagnostics,
			"Keep the exact same named reply-body source and do not edit",
			"create a task-owned reply file",
		)
	})

	t.Run("unchanged tail selects source-aware exact retry", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: "transport dropped before acknowledgement", exit: 1},
			providerStep{stdout: historyResponse(threadID, original)},
			providerStep{stdout: threadResponse(threadID, false, original)},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body,
				testReplyIssuancePredecessor(original, body), canonical,
			)
			return postCode
		})
		if code == 0 {
			t.Fatalf("unconfirmed post succeeded:\n%s", diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history", "thread")
		assertContainsAll(t, diagnostics,
			"Keep the exact same named reply-body source and do not edit or replace it",
			"reply_file='/tmp/resolve-review-thread.ABC123'",
			"Retain that file unchanged",
		)
		assertContainsNone(t, diagnostics,
			"revise the same named body source in place",
			"create a task-owned reply file",
		)
	})

	t.Run("unreadable history requires inspection and retains stdin", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stderr: "transport dropped after write", exit: 1},
			providerStep{stderr: "history unavailable", exit: 1},
		)
		code, _, diagnostics := provider.capture(func() int {
			_, postCode := postReplyOrExit(
				context.Background(), threadID, body,
				testReplyIssuancePredecessor(original, body), "-",
			)
			return postCode
		})
		if code == 0 {
			t.Fatalf("unreadable provider state succeeded:\n%s", diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "reply", "history")
		assertContainsAll(t, diagnostics,
			"Retain the exact standard-input bytes while provider state is unverified",
			"Do not revise or discard them",
			"do not continue to another thread or safe-merge",
			"Inspect the live thread before any retry",
		)
		assertContainsNone(t, diagnostics,
			"rm -f",
			"Replay the exact same retained standard-input bytes without revising them",
			"Replay the revised retained bytes",
		)
	})
}

func TestVerifyExactReplyPlacementRejectsIDOnlyOrIncompleteEvidenceBeforeProviderAccess(t *testing.T) {
	const (
		threadID    = "PRRT_incomplete_placement_evidence"
		predecessor = "PRRC_predecessor"
		reply       = "PRRC_reply"
		bodyFile    = "/tmp/resolve-review-thread.INCOMPLETE"
		openingBody = "P1: prove exact placement."
		replyBody   = "Exact placement is proved."
	)
	predecessorBodySHA256 := exactBodySHA256([]byte(openingBody))
	replyBodySHA256 := exactBodySHA256([]byte(replyBody))
	tests := []struct {
		name     string
		evidence resolutionEvidence
	}{
		{
			name:     "reply id only",
			evidence: resolutionEvidence{LastID: reply},
		},
		{
			name: "both ids only",
			evidence: resolutionEvidence{
				LastID:        reply,
				PredecessorID: predecessor,
			},
		},
		{
			name: "predecessor digest missing",
			evidence: resolutionEvidence{
				LastID:        reply,
				PredecessorID: predecessor,
				BodySHA256:    replyBodySHA256,
			},
		},
		{
			name: "reply digest missing",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         predecessor,
				PredecessorBodySHA256: predecessorBodySHA256,
			},
		},
		{
			name: "predecessor id missing",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorBodySHA256: predecessorBodySHA256,
				BodySHA256:            replyBodySHA256,
			},
		},
		{
			name: "predecessor and reply IDs equal",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         reply,
				PredecessorBodySHA256: predecessorBodySHA256,
				BodySHA256:            replyBodySHA256,
			},
		},
		{
			name: "predecessor digest invalid",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         predecessor,
				PredecessorBodySHA256: "not-a-digest",
				BodySHA256:            replyBodySHA256,
			},
		},
		{
			name: "reply digest invalid",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         predecessor,
				PredecessorBodySHA256: predecessorBodySHA256,
				BodySHA256:            "not-a-digest",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t)
			code, stdout, diagnostics := provider.capture(func() int {
				_, code := verifyReplyPlacementAgainst(
					context.Background(), threadID, tc.evidence, bodyFile,
				)
				return code
			})
			if code == 0 {
				t.Fatalf("incomplete exact placement evidence succeeded:\n%s", diagnostics)
			}
			if stdout != "" {
				t.Errorf("incomplete exact placement evidence printed success output: %q", stdout)
			}
			provider.assertExhausted()
			assertContainsAll(t, diagnostics, "exact placement evidence", "resolution was not attempted")
		})
	}
}

func TestResolvedPlacementMismatchReopensBeforeRevisedGuidance(t *testing.T) {
	const (
		threadID   = "PRRT_resolved_race"
		originalID = "PRRC_original"
		replyID    = "PRRC_reply"
		bodyFile   = "/private/tmp/custom-recovery-body"
	)
	original := providerComment{id: originalID, login: "reviewer", body: "P2: close the race"}
	reply := providerComment{id: replyID, login: "author", body: "Fixed."}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "Not yet."}

	tests := []struct {
		name     string
		bodyFile string
		comments []providerComment
		marker   string
	}{
		{
			name:     "buried",
			bodyFile: bodyFile,
			comments: []providerComment{original, reply, followup},
			marker:   "posted reply is not last",
		},
		{
			name:     "jumped",
			bodyFile: "-",
			comments: []providerComment{original, followup, reply},
			marker:   "arrived between the original read",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, true, tc.comments...)},
				providerStep{stdout: unresolveResponse(threadID)},
			)
			code, _, diagnostics := provider.capture(func() int {
				return verifyExactReplyPlacement(
					context.Background(),
					threadID,
					resolutionEvidence{
						LastID:                replyID,
						PredecessorID:         originalID,
						PredecessorBodySHA256: exactBodySHA256([]byte(original.body)),
						BodySHA256:            exactBodySHA256([]byte(reply.body)),
					},
					tc.bodyFile,
				)
			})
			if code == 0 {
				t.Fatalf("resolved %s placement mismatch succeeded:\n%s", tc.name, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "unresolve")
			assertContainsAll(t, diagnostics, tc.marker, "revised")
			if tc.bodyFile == "-" {
				assertContainsAll(t, diagnostics,
					"revise the retained standard-input bytes",
					"--body-file -",
				)
			} else {
				assertContainsAll(t, diagnostics,
					"revise the same named body source in place",
					"reply_file='/private/tmp/custom-recovery-body'",
				)
			}
		})
	}
}

func TestResolutionMutationRecoveryDistinguishesUnverifiableFromSuperseded(t *testing.T) {
	const (
		threadID = "PRRT_resolution_recovery"
		anchorID = "PRRC_reply"
		bodyFile = "/tmp/resolve-review-thread.RESOLVE"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P2: prove it"}
	reply := providerComment{id: anchorID, login: "author", body: "Proved."}

	tests := []struct {
		name         string
		mutationTail string
		want         []string
		forbidden    []string
	}{
		{
			name:         "empty mutation anchor keeps unchanged source",
			mutationTail: "",
			want: []string{
				"did not confirm its evidence anchor",
				"do not revise the reply source",
				"Keep the exact same named reply-body source",
				"reply_file='/tmp/resolve-review-thread.RESOLVE'",
			},
			forbidden: []string{"revise the same named body source in place"},
		},
		{
			name:         "changed nonempty mutation anchor needs new answer",
			mutationTail: "PRRC_newer",
			want: []string{
				"newer comment",
				"revised-answer lifecycle",
				"revise the same named body source in place",
				"reply_file='/tmp/resolve-review-thread.RESOLVE'",
			},
			forbidden: []string{"do not revise the reply source"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, reply)},
				providerStep{stdout: resolveResponse(threadID, tc.mutationTail)},
				providerStep{stdout: unresolveResponse(threadID)},
			)
			var recovery string
			code, _, diagnostics := provider.capture(func() int {
				_, mutated, err := resolveWithEvidence(
					context.Background(),
					threadID,
					false,
					resolutionEvidence{LastID: anchorID},
				)
				if err == nil || mutated {
					t.Fatalf("resolveWithEvidence = (mutated=%t, err=%v), want reopened failure", mutated, err)
				}
				var handled bool
				recovery, handled = replyResolutionEvidenceFailure(
					threadID,
					err,
					revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
				)
				if !handled {
					t.Fatalf("recovery error was not classified: %T: %v", err, err)
				}
				return 0
			})
			if code != 0 || diagnostics != "" {
				t.Fatalf("recovery classification emitted unexpected command diagnostics (code=%d): %s", code, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "unresolve")
			assertContainsAll(t, recovery, tc.want...)
			assertContainsNone(t, recovery, tc.forbidden...)
		})
	}
}

func TestUnavailableOrEqualAuthorEvidenceInspectsAndRetains(t *testing.T) {
	const (
		threadID = "PRRT_author_evidence"
		anchorID = "PRRC_reply"
		bodyFile = "/tmp/resolve-review-thread.AUTHOR"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P2: prove it"}
	tests := []struct {
		name      string
		lastLogin string
	}{
		{name: "unavailable latest author", lastLogin: ""},
		{name: "latest author equals opener", lastLogin: "reviewer"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			last := providerComment{id: anchorID, login: tc.lastLogin, body: "Attempted reply."}
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, last)},
			)
			var recovery string
			code, _, diagnostics := provider.capture(func() int {
				_, mutated, err := resolveWithEvidence(
					context.Background(),
					threadID,
					false,
					resolutionEvidence{LastID: anchorID},
				)
				if err == nil || mutated {
					t.Fatalf("resolveWithEvidence = (mutated=%t, err=%v), want evidence refusal", mutated, err)
				}
				var handled bool
				recovery, handled = replyResolutionEvidenceFailure(
					threadID,
					err,
					revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
				)
				if !handled {
					t.Fatalf("author-evidence error was not classified: %T: %v", err, err)
				}
				return 0
			})
			if code != 0 || diagnostics != "" {
				t.Fatalf("author-evidence classification emitted unexpected diagnostics (code=%d): %s", code, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread")
			assertContainsAll(t, recovery,
				"author evidence is insufficient",
				"Retain the exact named reply-body source unchanged",
				"reply_file='/tmp/resolve-review-thread.AUTHOR'",
				"Inspect the live thread before any retry",
			)
			assertContainsNone(t, recovery,
				"revise the same named body source in place",
				"Replay the revised retained bytes",
			)
		})
	}

	t.Run("externally resolved exact tail still requires independent answer evidence", func(t *testing.T) {
		const body = "The same opening identity supplied this attempted answer."
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		reply := providerComment{id: anchorID, login: original.login, body: body}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: historyResponse(threadID, original)},
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: replyResponse(reply)},
			providerStep{stdout: threadResponse(threadID, false, original, reply)},
			providerStep{stdout: threadResponse(threadID, true, original, reply)},
			providerStep{stdout: unresolveResponse(threadID)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("resolved exact tail bypassed independent-answer evidence:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("evidence refusal printed terminal success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread", "reply", "thread", "thread", "unresolve")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"continuation issuance boundary could not be proved",
			"requires safe distinct opening and reply authors",
			"no receipt was minted and no resolution was attempted",
			"Retain the exact named reply-body source unchanged",
			"Inspect the live thread before any retry",
		)
		assertContainsNone(t, diagnostics,
			"skipped "+threadID+" (already resolved)",
			"Only then continue",
			"rm -f",
		)
	})
}

func TestMutationResponsesRequireRequestedIdentityAndState(t *testing.T) {
	const (
		threadID = "PRRT_postcondition"
		anchorID = "PRRC_answer"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: prove the state"}
	answer := providerComment{id: anchorID, login: "author", body: "The state is proved."}

	t.Run("nominal resolve response remains unresolved", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original, answer)},
			providerStep{stdout: resolveResponseWithState(threadID, anchorID, false)},
			providerStep{stdout: threadResponse(threadID, false, original, answer)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"resolve", threadID})
		})
		if code == 0 {
			t.Fatalf("resolve accepted isResolved=false:\nstdout:\n%s\nstderr:\n%s", stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("failed resolve printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "resolve", "thread")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, strings.ToLower(diagnostics), "resolved")
		assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
	})

	t.Run("nominal unresolve response remains resolved", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: unresolveResponseWithState(threadID, true)},
			providerStep{stdout: threadResponse(threadID, true, original, answer)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"unresolve", threadID})
		})
		if code == 0 {
			t.Fatalf("unresolve accepted isResolved=true:\nstdout:\n%s\nstderr:\n%s", stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("failed unresolve printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "unresolve", "thread")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, strings.ToLower(diagnostics), "resolved")
		assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
	})

	t.Run("forced resolve still requires a nonempty mutation anchor", func(t *testing.T) {
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: resolveResponse(threadID, "")},
			providerStep{stdout: unresolveResponse(threadID)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"resolve", threadID, "--force"})
		})
		if code == 0 {
			t.Fatalf("forced resolve accepted an empty mutation anchor:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("unverifiable forced resolve printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "resolve", "unresolve")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"did not confirm its evidence anchor",
			"automatic reopen restored a safe retry state",
			"Retry the unchanged resolution operation",
			"resolve-review-threads resolve "+threadID+" --force",
		)
	})

	t.Run("identity payloads", testMutationThreadIdentityRequiresExactTargetOrRereadProof)
}

func testMutationThreadIdentityRequiresExactTargetOrRereadProof(t *testing.T) {
	const (
		threadID = "PRRT_identity"
		anchorID = "PRRC_answer"
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: verify identity"}
	answer := providerComment{id: anchorID, login: "author", body: "Identity verified."}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "This changed."}

	for _, variant := range []string{"missing", "null", "mismatched", "state-missing", "state-null"} {
		t.Run("resolve "+variant+" identity does not prove resolution", func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, answer)},
				providerStep{stdout: malformedMutationResponse(
					"resolveReviewThread", variant, threadID, anchorID, true,
				)},
				providerStep{stdout: threadResponse(threadID, false, original, answer)},
			)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"resolve", threadID})
			})
			if code == 0 {
				t.Fatalf("resolve accepted %s identity without a proving reread:\nstdout:\n%s\nstderr:\n%s",
					variant, stdout, diagnostics)
			}
			if stdout != "" {
				t.Errorf("failed resolve printed success output:\n%s", stdout)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
		})

		t.Run("unresolve "+variant+" identity does not prove reopening", func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: malformedMutationResponse(
					"unresolveReviewThread", variant, threadID, "", false,
				)},
				providerStep{stdout: threadResponse(threadID, true, original, answer)},
			)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"unresolve", threadID})
			})
			if code == 0 {
				t.Fatalf("unresolve accepted %s identity while target stayed resolved:\nstdout:\n%s\nstderr:\n%s",
					variant, stdout, diagnostics)
			}
			if stdout != "" {
				t.Errorf("failed unresolve printed success output:\n%s", stdout)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "unresolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
		})

		t.Run("automatic reopen "+variant+" identity is reconciled before guidance", func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, original, answer)},
				providerStep{stdout: resolveResponse(threadID, followup.id)},
				providerStep{stdout: malformedMutationResponse(
					"unresolveReviewThread", variant, threadID, "", false,
				)},
				providerStep{stdout: threadResponse(threadID, false, original, answer, followup)},
			)
			code, _, diagnostics := provider.capture(func() int {
				return run([]string{"resolve", threadID})
			})
			if code == 0 {
				t.Fatalf("stale resolution unexpectedly succeeded after %s reopen identity:\n%s",
					variant, diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "unresolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, diagnostics, "reopened", "requires a new answer")
			assertContainsNone(t, diagnostics, "STILL RESOLVED")
		})
	}
}

func TestIncompleteCommentIdentityRequiresInspection(t *testing.T) {
	const (
		threadID   = "PRRT_missing_ids"
		originalID = "PRRC_original"
		body       = "A descriptor-bound read now closes the race."
		bodyFile   = "/tmp/resolve-review-thread.MISSINGIDS"
	)
	original := providerComment{id: originalID, login: "reviewer", body: "P2: close the race"}

	tests := []struct {
		name  string
		steps []providerStep
		want  []string
	}{
		{
			name: "history predecessor id missing",
			steps: []providerStep{
				{stdout: replyResponse(providerComment{})},
				{stdout: historyResponse(threadID, providerComment{id: "", login: original.login, body: original.body})},
			},
			want: []string{"provider state is unverified", "Inspect the live thread before any retry"},
		},
		{
			name: "matching recovered reply id missing",
			steps: []providerStep{
				{stderr: "connection closed after write", exit: 1},
				{stdout: historyResponse(threadID, original, providerComment{id: "", login: "author", body: body})},
			},
			want: []string{"provider state is unverified", "Inspect the live thread before any retry"},
		},
		{
			name: "current tail id missing",
			steps: []providerStep{
				{stderr: "connection closed before acknowledgement", exit: 1},
				{stdout: historyResponse(threadID, original)},
				{stdout: threadResponse(threadID, false, providerComment{id: "", login: original.login, body: original.body})},
			},
			want: []string{"current thread state could not be read with a nonempty tail ID", "Inspect the live thread before any retry"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t, tc.steps...)
			code, _, diagnostics := provider.capture(func() int {
				_, postCode := postReplyOrExit(
					context.Background(), threadID, body,
					testReplyIssuancePredecessor(original, body), bodyFile,
				)
				return postCode
			})
			if code == 0 {
				t.Fatalf("missing comment identity selected a recovery branch:\n%s", diagnostics)
			}
			provider.assertExhausted()
			wantKinds := []string{"reply", "history"}
			if len(tc.steps) == 3 {
				wantKinds = append(wantKinds, "thread")
			}
			assertQueryKinds(t, provider, wantKinds...)
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, diagnostics, tc.want...)
			assertContainsAll(t, diagnostics,
				"Retain the exact named reply-body source unchanged",
				"do not continue to another thread or safe-merge",
			)
			assertContainsNone(t, diagnostics,
				"an exact-body retry remains applicable",
				"use this revised-answer lifecycle",
				"rm -f",
			)
		})
	}
}

func TestResolveErrorRecoveryClassifiesFreshState(t *testing.T) {
	const (
		threadID = "PRRT_resolve_transport"
		anchorID = "PRRC_reply"
		body     = "The descriptor now supplies the verified bytes."
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P2: prove it"}
	reply := providerComment{id: anchorID, login: "author", body: body}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "The evidence changed."}

	tests := []struct {
		name      string
		after     string
		want      []string
		forbidden []string
		inspect   bool
	}{
		{
			name:  "unchanged nonempty tail permits exact-body retry",
			after: threadResponse(threadID, false, original, reply),
			want: []string{
				"Keep the exact same named reply-body source and continuation receipt",
				"Retain that file unchanged",
			},
			forbidden: []string{"revise the same named body source in place"},
		},
		{
			name:  "changed nonempty tail requires revision",
			after: threadResponse(threadID, false, original, reply, followup),
			want: []string{
				"newer comment",
				"revise the same named body source in place",
			},
			forbidden: []string{"Keep the exact same named reply-body source"},
		},
		{
			name: "empty tail id requires inspection",
			after: threadResponse(threadID, false,
				original,
				providerComment{id: "", login: "author", body: body},
			),
			want: []string{
				"provider state could not be verified",
				"Inspect the live thread before any retry",
				"do not continue to another thread or safe-merge",
			},
			forbidden: []string{
				"Keep the exact same named reply-body source",
				"revise the same named body source in place",
				"rm -f",
			},
			inspect: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bodyFile := filepath.Join(t.TempDir(), "reply body")
			if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
				t.Fatalf("write reply body: %v", err)
			}
			receipt := continuationToken(
				t,
				threadID,
				original.id,
				original.body,
				reply.id,
				reply.body,
			)
			provider := installSequencedProvider(t,
				providerStep{stdout: historyResponse(threadID, original, reply)},
				providerStep{stdout: threadResponse(threadID, false, original, reply)},
				providerStep{stderr: "connection reset after resolve", exit: 1},
				providerStep{stdout: tc.after},
			)
			code, _, diagnostics := provider.capture(func() int {
				return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
			})
			if code == 0 {
				t.Fatalf("ambiguous resolve with unresolved reread succeeded:\n%s", diagnostics)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "history", "thread", "resolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, diagnostics, tc.want...)
			assertContainsNone(t, diagnostics, tc.forbidden...)
			if tc.inspect {
				assertContainsNone(t, diagnostics, "Only then continue")
			}
		})
	}
}

func TestUnresolveErrorRecoveryReconcilesFreshState(t *testing.T) {
	const threadID = "PRRT_unresolve_transport"
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reopen this"}

	tests := []struct {
		name       string
		reread     providerStep
		wantCode   int
		wantStdout []string
		wantStderr []string
		unverified bool
	}{
		{
			name:       "transport failed after reopen applied",
			reread:     providerStep{stdout: threadResponse(threadID, false, original)},
			wantCode:   0,
			wantStdout: []string{"unresolved", "isResolved=false", "recovered"},
		},
		{
			name:       "transport failed and reopen did not apply",
			reread:     providerStep{stdout: threadResponse(threadID, true, original)},
			wantCode:   1,
			wantStderr: []string{"still resolved"},
		},
		{
			name:       "transport failed and reread failed",
			reread:     providerStep{stderr: "thread state unavailable", exit: 1},
			wantCode:   1,
			wantStderr: []string{"could not be re-read"},
			unverified: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stderr: "connection reset after unresolve", exit: 1},
				tc.reread,
			)
			code, stdout, diagnostics := provider.capture(func() int {
				return run([]string{"unresolve", threadID})
			})
			if tc.wantCode == 0 && code != 0 {
				t.Fatalf("applied unresolve was not recovered (code=%d):\n%s", code, diagnostics)
			}
			if tc.wantCode != 0 && code == 0 {
				t.Fatalf("unverified/unapplied unresolve succeeded:\n%s", stdout)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "unresolve", "thread")
			assertQueryTargets(t, provider, threadID)
			assertContainsAll(t, stdout, tc.wantStdout...)
			assertContainsAll(t, strings.ToLower(diagnostics), tc.wantStderr...)
			if tc.wantCode != 0 && stdout != "" {
				t.Errorf("failed unresolve printed success output:\n%s", stdout)
			}
			if tc.unverified {
				assertContainsNone(t, diagnostics, "rm -f", "Only then continue")
			}
		})
	}
}

func TestMovedResolvedTailReopensBeforeGuidance(t *testing.T) {
	t.Run("buried reply with unavailable latest author uses neutral supersession wording", func(t *testing.T) {
		const (
			threadID = "PRRT_buried_unknown_author"
			body     = "The earlier answer addressed only the original state."
		)
		original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
		oldReply := providerComment{id: "PRRC_reply", login: "author", body: body}
		followup := providerComment{id: "PRRC_followup", login: "", body: "There is newer commentary."}
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original, oldReply, followup)},
			providerStep{stdout: historyResponse(threadID, original, oldReply, followup)},
			providerStep{stdout: threadResponse(threadID, false, original, oldReply, followup)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("buried reply with unknown latest author succeeded:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread")
		assertContainsAll(t, diagnostics,
			"newer commentary follows it",
			"read the comment(s) after your reply",
			"revise the same named body source in place",
		)
		assertContainsNone(t, diagnostics,
			"the reviewer has commented since",
			"reviewer follow-up",
		)
	})

	t.Run("buried prior reply is classified before resolved skip", func(t *testing.T) {
		const (
			threadID = "PRRT_buried_before_skip"
			body     = "The earlier answer addressed only the original review state."
		)
		original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
		oldReply := providerComment{id: "PRRC_reply", login: "author", body: body}
		followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "That answer is incomplete."}
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original, oldReply, followup)},
			providerStep{stdout: historyResponse(threadID, original, oldReply, followup)},
			providerStep{stdout: threadResponse(threadID, true, original, oldReply, followup)},
			providerStep{stdout: unresolveResponse(threadID)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("buried prior reply was skipped as terminal:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("buried-reply refusal printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"earlier matching reply is followed by newer commentary",
			"automatically reopened",
			"revise the same named body source in place",
		)
		assertContainsNone(t, diagnostics,
			"skipped "+threadID+" (already resolved)",
			"Keep the exact same named reply-body source",
		)
	})

	t.Run("history reread compares the moved tail before resolved skip", func(t *testing.T) {
		const (
			threadID = "PRRT_history_tail_race"
			body     = "The reply was prepared for the original review state."
		)
		original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
		followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "The state changed."}
		bodyFile := filepath.Join(t.TempDir(), "reply source")
		if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
			t.Fatalf("write reply body: %v", err)
		}
		provider := installSequencedProvider(t,
			providerStep{stdout: threadResponse(threadID, false, original)},
			providerStep{stdout: historyResponse(threadID, original)},
			providerStep{stdout: threadResponse(threadID, true, original, followup)},
			providerStep{stdout: unresolveResponse(threadID)},
		)
		code, stdout, diagnostics := provider.capture(func() int {
			return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
		})
		if code == 0 {
			t.Fatalf("moved resolved history tail was skipped as terminal:\nstdout:\n%s\nstderr:\n%s",
				stdout, diagnostics)
		}
		if stdout != "" {
			t.Errorf("history-tail refusal printed success output:\n%s", stdout)
		}
		provider.assertExhausted()
		assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
		assertQueryTargets(t, provider, threadID)
		assertContainsAll(t, diagnostics,
			"history tail changed",
			"automatically reopened",
			"revised-answer lifecycle",
			"revise the same named body source in place",
		)
		assertContainsNone(t, diagnostics,
			"skipped "+threadID+" (already resolved)",
			"Keep the exact same named reply-body source",
		)
	})

	t.Run("ambiguous reply observes a moved resolved tail", testResolvedMovedAmbiguousReplyOutcome)
}

func testResolvedMovedAmbiguousReplyOutcome(t *testing.T) {
	const (
		threadID = "PRRT_full_ambiguous_reply"
		body     = "The reply addresses the state observed before posting."
	)
	original := providerComment{id: "PRRC_original", login: "reviewer", body: "P1: reconcile this"}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "Provider state moved."}
	bodyFile := filepath.Join(t.TempDir(), "reply source")
	if err := os.WriteFile(bodyFile, []byte(body), 0o600); err != nil {
		t.Fatalf("write reply body: %v", err)
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, original)},
		providerStep{stdout: historyResponse(threadID, original)},
		providerStep{stdout: threadResponse(threadID, false, original)},
		providerStep{stderr: "connection closed after reply write", exit: 1},
		providerStep{stdout: historyResponse(threadID, original, followup)},
		providerStep{stdout: threadResponse(threadID, true, original, followup)},
		providerStep{stdout: unresolveResponse(threadID)},
	)
	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatalf("resolved moved ambiguous reply outcome succeeded:\nstdout:\n%s\nstderr:\n%s",
			stdout, diagnostics)
	}
	if stdout != "" {
		t.Errorf("ambiguous reply path printed success output:\n%s", stdout)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider,
		"thread", "history", "thread", "reply", "history", "thread", "unresolve",
	)
	assertQueryTargets(t, provider, threadID)
	assertContainsAll(t, diagnostics,
		"thread tail moved",
		"revised-answer lifecycle",
		"revise the same named body source in place",
	)
	assertContainsNone(t, diagnostics,
		"Keep the exact same named reply-body source",
		"provider state is unverified",
	)
}
