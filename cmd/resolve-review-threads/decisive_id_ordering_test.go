package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestContinuationMismatchPrioritizesMovedLastIDOverMissingBody(t *testing.T) {
	const (
		threadID  = "PRRT_continuation_moved_id_missing_body"
		replyBody = "PRIVATE-CONTINUATION-REPLY"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-ORIGINAL-POINT"}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "PRIVATE-NEW-FOLLOWUP"}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessor.id, predecessor.body, reply.id, reply.body)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, predecessor, reply, followup)},
		providerStep{stdout: threadResponseWithRecentBodyOmitted(threadID, true, []providerComment{predecessor, reply, followup}, followup.id)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("moved continuation tail with an omitted body was treated as terminal")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread", "unresolve")
	assertNoReplyOrResolveMutation(t, provider)
	if stdout != "" {
		t.Errorf("stale continuation printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "continuation evidence is not current", "reopened")
	assertContainsNone(t, stdout+diagnostics, predecessor.body, reply.body, followup.body)
}

func TestContinueResolvePreReadPrioritizesChangedPredecessorIDOverMissingBody(t *testing.T) {
	const (
		threadID  = "PRRT_pre_read_changed_predecessor_missing_body"
		replyBody = "PRIVATE-PRE-READ-REPLY"
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "PRIVATE-OPENING"}
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-PREDECESSOR"}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessor.id, predecessor.body, reply.id, reply.body)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, opening, predecessor, reply)},
		providerStep{stdout: threadResponseWithRecentBodyOmitted(threadID, true, []providerComment{opening, reply}, reply.id)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("changed predecessor ID hidden by an omitted body was treated as terminal")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread", "unresolve")
	assertNoReplyOrResolveMutation(t, provider)
	if stdout != "" {
		t.Errorf("invalid continuation pre-read printed success output: %q", stdout)
	}
	lower := strings.ToLower(diagnostics)
	assertContainsAll(t, lower, "exact reply evidence changed", "predecessor", "fresh reply-resolve lifecycle")
	assertContainsNone(t, stdout+diagnostics, opening.body, predecessor.body, reply.body)
}

func TestStableHistoryPrioritizesChangedPredecessorIDOverMissingLastBody(t *testing.T) {
	const (
		threadID  = "PRRT_stable_history_changed_predecessor_missing_body"
		replyBody = "PRIVATE-STABLE-HISTORY-REPLY"
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "PRIVATE-OPENING"}
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-PREDECESSOR"}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening, predecessor, reply)},
		providerStep{stdout: historyResponse(threadID, opening, predecessor, reply)},
		providerStep{stdout: threadResponseWithRecentBodyOmitted(threadID, true, []providerComment{opening, reply}, reply.id)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("stable-history predecessor change hidden by an omitted body was treated as terminal")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
	assertNoReplyOrResolveMutation(t, provider)
	if stdout != "" {
		t.Errorf("stale stable-history boundary printed success output: %q", stdout)
	}
	lower := strings.ToLower(diagnostics)
	assertContainsAll(t, lower, "predecessor id changed", "automatically reopened")
	assertContainsNone(t, stdout+diagnostics, opening.body, predecessor.body, reply.body)
}

func TestReplyPlacementPrioritizesChangedLastIDOverMissingPredecessorID(t *testing.T) {
	const (
		threadID = "PRRT_reply_placement_changed_last_empty_prev"
		bodyFile = "/tmp/resolve-review-thread.DECISIVE-PLACEMENT"
	)
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-PREDECESSOR"}
	reply := providerComment{id: "PRRC_reply", login: "author", body: "PRIVATE-REPLY"}
	followup := providerComment{id: "PRRC_followup", login: "reviewer", body: "PRIVATE-FOLLOWUP"}
	evidence := resolutionEvidence{
		LastID:                reply.id,
		PredecessorID:         predecessor.id,
		PredecessorBodySHA256: exactBodySHA256([]byte(predecessor.body)),
		BodySHA256:            exactBodySHA256([]byte(reply.body)),
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, true, followup)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return verifyExactReplyPlacement(context.Background(), threadID, evidence, bodyFile)
	})
	if code == 0 {
		t.Fatal("changed last ID hidden by an empty predecessor ID passed reply placement")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "unresolve")
	if stdout != "" {
		t.Errorf("stale reply placement printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "not last", "automatically reopened")
	assertContainsNone(t, stdout+diagnostics, predecessor.body, reply.body, followup.body)
}

func TestResolveMutationEvidencePrioritizesChangedPredecessorIDOverMissingBody(t *testing.T) {
	const (
		threadID  = "PRRT_mutation_changed_predecessor_missing_body"
		replyBody = "PRIVATE-MUTATION-REPLY"
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "PRIVATE-OPENING"}
	predecessor := providerComment{id: "PRRC_predecessor", login: "reviewer", body: "PRIVATE-PREDECESSOR"}
	reply := providerComment{id: "PRRC_reply", login: "author", body: replyBody}
	bodyFile := writeContinuationBody(t, replyBody)
	receipt := continuationToken(t, threadID, predecessor.id, predecessor.body, reply.id, reply.body)
	provider := installSequencedProvider(t,
		providerStep{stdout: historyResponse(threadID, opening, predecessor, reply)},
		providerStep{stdout: threadResponse(threadID, false, opening, predecessor, reply)},
		providerStep{stdout: resolveResponseWithBodyOmitted(threadID, true, []providerComment{opening, reply}, reply.id)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"continue-resolve", receipt, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("resolve response predecessor change hidden by an omitted body reported success")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "thread", "resolve", "unresolve")
	if stdout != "" {
		t.Errorf("invalid resolve mutation evidence printed success output: %q", stdout)
	}
	lower := strings.ToLower(diagnostics)
	assertContainsAll(t, lower, "exact reply evidence changed", "predecessor", "reopened", "fresh reply-resolve lifecycle")
	assertContainsNone(t, lower, "resolve response omitted exact reply evidence", "do not revise the reply source")
	assertContainsNone(t, stdout+diagnostics, opening.body, predecessor.body, reply.body)
}

func TestInitialHistoryMismatchPrioritizesChangedLastIDOverMissingBody(t *testing.T) {
	const (
		threadID  = "PRRT_initial_history_changed_id_missing_body"
		replyBody = "PRIVATE-INITIAL-HISTORY-REPLY"
	)
	initial := providerComment{id: "PRRC_initial", login: "reviewer", body: "PRIVATE-INITIAL"}
	historyTail := providerComment{id: "PRRC_history_tail", login: "reviewer", body: "PRIVATE-HISTORY-TAIL"}
	currentTail := providerComment{id: "PRRC_current_tail", login: "reviewer", body: "PRIVATE-CURRENT-TAIL"}
	bodyFile := writeContinuationBody(t, replyBody)
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, initial)},
		providerStep{stdout: historyResponse(threadID, initial, historyTail)},
		providerStep{stdout: threadResponseWithRecentBodyOmitted(threadID, true, []providerComment{currentTail}, currentTail.id)},
		providerStep{stdout: unresolveResponse(threadID)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("fresh changed ID hidden by an omitted body left the initial-history mismatch terminal")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "thread", "unresolve")
	assertNoReplyOrResolveMutation(t, provider)
	if stdout != "" {
		t.Errorf("initial-history mismatch printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "changed after the initial", "automatically reopened")
	assertContainsNone(t, stdout+diagnostics, initial.body, historyTail.body, currentTail.body, replyBody)
}

func threadResponseWithRecentBodyOmitted(
	threadID string,
	resolved bool,
	comments []providerComment,
	omitBodyForIDs ...string,
) string {
	omitted := make(map[string]struct{}, len(omitBodyForIDs))
	for _, id := range omitBodyForIDs {
		omitted[id] = struct{}{}
	}
	opening := ""
	if len(comments) > 0 {
		opening = fmt.Sprintf(`{"author":{"login":%q},"body":%q}`, comments[0].login, comments[0].body)
	}
	recent := make([]string, 0, len(comments))
	for _, comment := range comments {
		_, omit := omitted[comment.id]
		recent = append(recent, providerCommentWithOmittedEvidence(comment, false, omit))
	}
	return fmt.Sprintf(`{"data":{"node":{"id":%q,"isResolved":%t,"isOutdated":false,"path":"review.go","opening":{"totalCount":%d,"nodes":[%s]},"recent":{"nodes":[%s]}}}}`,
		threadID, resolved, len(comments), opening, strings.Join(recent, ","))
}

func resolveResponseWithBodyOmitted(
	threadID string,
	resolved bool,
	comments []providerComment,
	omitBodyForIDs ...string,
) string {
	omitted := make(map[string]struct{}, len(omitBodyForIDs))
	for _, id := range omitBodyForIDs {
		omitted[id] = struct{}{}
	}
	opening := ""
	if len(comments) > 0 {
		opening = fmt.Sprintf(`{"author":{"login":%q}}`, comments[0].login)
	}
	start := 0
	if len(comments) > 2 {
		start = len(comments) - 2
	}
	recent := make([]string, 0, len(comments)-start)
	for _, comment := range comments[start:] {
		_, omit := omitted[comment.id]
		recent = append(recent, providerCommentWithOmittedEvidence(comment, false, omit))
	}
	return fmt.Sprintf(`{"data":{"resolveReviewThread":{"thread":{"id":%q,"isResolved":%t,"opening":{"nodes":[%s]},"recent":{"nodes":[%s]}}}}}`,
		threadID, resolved, opening, strings.Join(recent, ","))
}
