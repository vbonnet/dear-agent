package main

import (
	"context"
	"strings"
	"testing"
)

func TestAmbiguousReplyReconcilesDeletedPredecessorBeforeRecovery(t *testing.T) {
	const (
		threadID        = "PRRT_ambiguous_deleted_predecessor"
		predecessorID   = "PRRC_deleted_predecessor"
		predecessorBody = "P1: answer the point that is later deleted."
		replyBody       = "The deleted point was addressed by these exact bytes."
		bodyFile        = "/tmp/resolve-review-thread.DELETED"
	)
	replacement := providerComment{
		id:    "PRRC_tail_after_deletion",
		login: "reviewer",
		body:  "The original predecessor is gone; this is the live point.",
	}

	for _, resolved := range []bool{false, true} {
		name := "unresolved"
		if resolved {
			name = "stale-resolved"
		}
		t.Run(name, func(t *testing.T) {
			steps := []providerStep{
				{stderr: "transport dropped before acknowledgement", exit: 1},
				{stdout: historyResponse(threadID, replacement)},
				{stdout: threadResponse(threadID, resolved, replacement)},
			}
			if resolved {
				steps = append(steps, providerStep{stdout: unresolveResponse(threadID)})
			}
			provider := installSequencedProvider(t, steps...)

			code, stdout, diagnostics := provider.capture(func() int {
				_, postCode := postReplyOrExit(
					context.Background(),
					threadID,
					replyBody,
					testReplyIssuancePredecessor(
						providerComment{id: predecessorID, login: "reviewer", body: predecessorBody},
						replyBody,
					),
					bodyFile,
				)
				return postCode
			})
			if code == 0 {
				t.Fatal("deleted predecessor ambiguity reported success")
			}
			wantKinds := []string{"reply", "history", "thread"}
			if resolved {
				wantKinds = append(wantKinds, "unresolve")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, wantKinds...)
			assertNoReplyMutationAfterInitialAttempt(t, provider)
			if stdout != "" {
				t.Errorf("deleted-predecessor recovery printed success: %q", stdout)
			}
			lower := strings.ToLower(diagnostics)
			assertContainsAll(t, lower, "predecessor", "revised-answer lifecycle")
			assertContainsNone(t, lower, "exact-body retry remains applicable")
			if resolved {
				assertContainsAll(t, lower, "reopened")
			}
			assertContainsNone(t, diagnostics, predecessorBody, replacement.body, replyBody)
		})
	}
}

func TestAmbiguousReplyRecoversDelayedVisibleExactReply(t *testing.T) {
	const (
		threadID        = "PRRT_ambiguous_delayed_reply"
		predecessorID   = "PRRC_delayed_predecessor"
		predecessorBody = "P1: recover a reply that becomes visible after history."
		replyID         = "PRRC_delayed_visible_reply"
		replyBody       = "The mutation applied even though the first recovery read missed it."
		bodyFile        = "/tmp/resolve-review-thread.DELAYED"
	)
	predecessor := providerComment{id: predecessorID, login: "reviewer", body: predecessorBody}
	reply := providerComment{id: replyID, login: "author", body: replyBody}

	for _, resolved := range []bool{false, true} {
		name := "unresolved"
		if resolved {
			name = "resolved"
		}
		t.Run(name, func(t *testing.T) {
			steps := []providerStep{
				{stderr: "transport dropped while the provider was committing", exit: 1},
				// The full-history read races the commit and still sees only the
				// predecessor. The later exact-target read sees the applied reply.
				{stdout: historyResponse(threadID, predecessor)},
				{stdout: threadResponse(threadID, resolved, predecessor, reply)},
			}
			if resolved {
				// This response exists only to keep the buggy corrective reopen
				// deterministic. Correct recovery must leave it unused.
				steps = append(steps, providerStep{stdout: unresolveResponse(threadID)})
			}
			provider := installSequencedProvider(t, steps...)

			var gotID string
			code, stdout, diagnostics := provider.capture(func() int {
				var postCode int
				gotID, postCode = postReplyOrExit(
					context.Background(),
					threadID,
					replyBody,
					testReplyIssuancePredecessor(predecessor, replyBody),
					bodyFile,
				)
				if postCode >= 0 {
					return postCode
				}
				return 0
			})
			if code != 0 {
				t.Errorf("delayed visible reply was not recovered: code=%d\nstdout:\n%s\nstderr:\n%s", code, stdout, diagnostics)
			}
			if gotID != replyID {
				t.Errorf("recovered reply ID = %q, want %q", gotID, replyID)
			}
			assertQueryKinds(t, provider, "reply", "history", "thread")
			assertNoProviderMutationAfterReply(t, provider)
			assertContainsNone(t, strings.ToLower(diagnostics), "revised-answer lifecycle", "old body must not be replayed", "reopened")
			assertContainsNone(t, stdout+diagnostics, predecessorBody, replyBody)
		})
	}
}

func assertNoReplyMutationAfterInitialAttempt(t *testing.T, provider *sequencedProvider) {
	t.Helper()
	replies := 0
	for _, kind := range queryKinds(provider.requests()) {
		if kind == "reply" {
			replies++
		}
		if kind == "resolve" {
			t.Error("ambiguous reply recovery issued a forbidden resolve mutation")
		}
	}
	if replies != 1 {
		t.Errorf("ambiguous reply recovery issued %d reply mutations, want exactly the initial attempt", replies)
	}
}

func assertNoProviderMutationAfterReply(t *testing.T, provider *sequencedProvider) {
	t.Helper()
	for i, kind := range queryKinds(provider.requests()) {
		if i == 0 && kind == "reply" {
			continue
		}
		if kind == "reply" || kind == "resolve" || kind == "unresolve" {
			t.Errorf("delayed reply recovery issued forbidden %s mutation", kind)
		}
	}
}
