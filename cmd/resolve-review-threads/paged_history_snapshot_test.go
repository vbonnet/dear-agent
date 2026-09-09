package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func pagedHistoryPageResponse(
	threadID string,
	hasNextPage bool,
	endCursor string,
	comments ...providerComment,
) string {
	nodes := make([]string, 0, len(comments))
	for _, comment := range comments {
		nodes = append(nodes, fmt.Sprintf(
			`{"id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}`,
			comment.id,
			comment.login,
			comment.body,
			comment.providerUpdatedAt(),
			comment.providerEditEvidence(),
		))
	}
	return fmt.Sprintf(
		`{"data":{"node":{"id":%q,"comments":{"pageInfo":{"hasNextPage":%t,"endCursor":%q},"nodes":[%s]}}}}`,
		threadID,
		hasNextPage,
		endCursor,
		strings.Join(nodes, ","),
	)
}

func pagedHistoryCommentJSON(comment providerComment) string {
	return fmt.Sprintf(
		`{"__typename":"PullRequestReviewComment","id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}`,
		comment.id,
		comment.login,
		comment.body,
		comment.providerUpdatedAt(),
		comment.providerEditEvidence(),
	)
}

func pagedSnapshotRaw(
	threadID string,
	totalCount int,
	snapshotBatches [][]string,
	openingNodes, recentNodes []string,
) string {
	fields := []string{fmt.Sprintf(
		`"target":{"id":%q,"isResolved":false,"path":"review.go","opening":{"totalCount":%d,"nodes":[%s]},"recent":{"nodes":[%s]}}`,
		threadID,
		totalCount,
		strings.Join(openingNodes, ","),
		strings.Join(recentNodes, ","),
	)}
	for i, batch := range snapshotBatches {
		fields = append(fields, fmt.Sprintf(`"snapshot%d":[%s]`, i, strings.Join(batch, ",")))
	}
	return fmt.Sprintf(
		`{"data":{%s}}`,
		strings.Join(fields, ","),
	)
}

func pagedSnapshotResponse(
	threadID string,
	snapshot []providerComment,
	opening providerComment,
	recent ...providerComment,
) string {
	snapshotNodes := make([]string, 0, len(snapshot))
	for _, comment := range snapshot {
		snapshotNodes = append(snapshotNodes, pagedHistoryCommentJSON(comment))
	}
	recentNodes := make([]string, 0, len(recent))
	for _, comment := range recent {
		recentNodes = append(recentNodes, pagedHistoryCommentJSON(comment))
	}
	batches := make([][]string, 0, (len(snapshotNodes)+maxPagedHistorySnapshotIDsPerField-1)/maxPagedHistorySnapshotIDsPerField)
	for start := 0; start < len(snapshotNodes); start += maxPagedHistorySnapshotIDsPerField {
		end := min(start+maxPagedHistorySnapshotIDsPerField, len(snapshotNodes))
		batches = append(batches, snapshotNodes[start:end])
	}
	return pagedSnapshotRaw(
		threadID,
		len(snapshot),
		batches,
		[]string{pagedHistoryCommentJSON(opening)},
		recentNodes,
	)
}

func pagedThreadResponse(
	threadID string,
	resolved bool,
	totalCount int,
	opening providerComment,
	recent ...providerComment,
) string {
	recentNodes := make([]string, 0, len(recent))
	for _, comment := range recent {
		recentNodes = append(recentNodes, fmt.Sprintf(
			`{"id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}`,
			comment.id,
			comment.login,
			comment.body,
			comment.providerUpdatedAt(),
			comment.providerEditEvidence(),
		))
	}
	return fmt.Sprintf(
		`{"data":{"node":{"id":%q,"isResolved":%t,"isOutdated":false,"path":"review.go","opening":{"totalCount":%d,"nodes":[{"author":{"login":%q},"body":%q}]},"recent":{"nodes":[%s]}}}}`,
		threadID,
		resolved,
		totalCount,
		opening.login,
		opening.body,
		strings.Join(recentNodes, ","),
	)
}

func providerCommentRange(prefix string, count int) []providerComment {
	comments := make([]providerComment, count)
	for i := range comments {
		comments[i] = providerComment{
			id:    fmt.Sprintf("PRRC_%s_%03d", prefix, i),
			login: "reviewer",
			body:  fmt.Sprintf("PRIVATE-%s-%03d", prefix, i),
		}
	}
	return comments
}

func observedPagedComments(comments ...providerComment) []tailComment {
	observed := make([]tailComment, 0, len(comments))
	for _, comment := range comments {
		observed = append(observed, tailComment{ID: comment.id})
	}
	return observed
}

func TestPagedHistorySnapshotReclassifiesOlderEditedMatchBeforeMutation(t *testing.T) {
	const (
		threadID = "PRRT_paged_edited_older"
		body     = "PRIVATE-REQUESTED-PAGED-ANSWER"
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "PRIVATE-OPENING"}
	older := providerComment{id: "PRRC_older", login: "author", body: "PRIVATE-OLDER-ANSWER"}
	handback := providerComment{id: "PRRC_handback", login: "reviewer", body: "PRIVATE-HANDBACK"}
	editedOlder := providerComment{
		id: "PRRC_older", login: "author", body: body,
		updatedAt: "2026-09-09T00:00:01Z", editCount: 1, lastEditID: "UCE_older_latest",
	}
	comments := providerCommentRange("PAGED-EDITED", 101)
	comments[0] = opening
	comments[50] = older
	comments[100] = handback
	refreshed := append([]providerComment(nil), comments...)
	refreshed[50] = editedOlder
	bodyFile := writeContinuationBody(t, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: pagedThreadResponse(threadID, false, len(comments), opening, comments[81:]...)},
		providerStep{stdout: pagedHistoryPageResponse(threadID, true, "cursor-1", comments[:100]...)},
		providerStep{stdout: pagedHistoryPageResponse(threadID, false, "", comments[100:]...)},
		providerStep{stdout: pagedSnapshotResponse(
			threadID,
			refreshed,
			opening,
			refreshed[99],
			handback,
		)},
		providerStep{stdout: pagedThreadResponse(threadID, false, len(refreshed), opening, refreshed[81:]...)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code == 0 {
		t.Fatal("an older matching reply refreshed after page retrieval licensed a duplicate reply")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "history", "history", "history-snapshot", "thread")
	assertNoProviderMutation(t, provider)
	if stdout != "" {
		t.Errorf("superseded paged reply printed success output: %q", stdout)
	}
	assertContainsAll(t, strings.ToLower(diagnostics), "earlier reply", "newer commentary", "nothing was posted")
	assertContainsNone(t, stdout+diagnostics, opening.body, older.body, handback.body, body)
}

func TestPagedHistorySnapshotStableControlProceeds(t *testing.T) {
	const (
		threadID = "PRRT_paged_stable_control"
		body     = "PRIVATE-REVISED-PAGED-ANSWER"
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "PRIVATE-OPENING"}
	oldReply := providerComment{id: "PRRC_old_reply", login: "author", body: "PRIVATE-OLD-ANSWER"}
	handback := providerComment{id: "PRRC_handback", login: "reviewer", body: "PRIVATE-HANDBACK"}
	newReply := providerComment{id: "PRRC_new_reply", login: "author", body: body}
	comments := providerCommentRange("PAGED-STABLE", 101)
	comments[0] = opening
	comments[99] = oldReply
	comments[100] = handback
	bodyFile := writeContinuationBody(t, body)
	provider := installSequencedProvider(t,
		providerStep{stdout: pagedThreadResponse(threadID, false, len(comments), opening, comments[81:]...)},
		providerStep{stdout: pagedHistoryPageResponse(threadID, true, "cursor-1", comments[:100]...)},
		providerStep{stdout: pagedHistoryPageResponse(threadID, false, "", comments[100:]...)},
		providerStep{stdout: pagedSnapshotResponse(
			threadID,
			comments,
			opening,
			oldReply,
			handback,
		)},
		providerStep{stdout: pagedThreadResponse(threadID, false, len(comments), opening, comments[81:]...)},
		providerStep{stdout: replyResponse(newReply)},
		providerStep{stdout: pagedThreadResponse(threadID, false, len(comments)+1, opening, handback, newReply)},
		providerStep{stdout: pagedThreadResponse(threadID, false, len(comments)+1, opening, handback, newReply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, handback, newReply)},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		return run([]string{"reply-resolve", threadID, "--body-file", bodyFile})
	})
	if code != 0 {
		t.Fatalf("stable paged history failed with code %d:\nstdout:\n%s\nstderr:\n%s", code, stdout, diagnostics)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider,
		"thread", "history", "history", "history-snapshot", "thread",
		"reply", "thread", "thread", "resolve",
	)
	assertReplyMutationBody(t, provider, body)
	assertContainsAll(t, stdout, "resolved "+threadID)
}

func TestPagedHistorySnapshotRejectsIncompleteOrInconsistentEvidence(t *testing.T) {
	const threadID = "PRRT_paged_invalid_snapshot"
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "PRIVATE-OPENING"}
	middle := providerComment{id: "PRRC_middle", login: "author", body: "PRIVATE-MIDDLE"}
	last := providerComment{id: "PRRC_last", login: "reviewer", body: "PRIVATE-LAST"}
	unknown := providerComment{id: "PRRC_unknown", login: "author", body: "PRIVATE-UNKNOWN"}

	openingNode := pagedHistoryCommentJSON(opening)
	middleNode := pagedHistoryCommentJSON(middle)
	lastNode := pagedHistoryCommentJSON(last)
	unknownNode := pagedHistoryCommentJSON(unknown)
	validSnapshot := []string{openingNode, middleNode, lastNode}
	validRecent := []string{middleNode, lastNode}
	valid := pagedSnapshotRaw(threadID, 3, [][]string{validSnapshot}, []string{openingNode}, validRecent)

	missingBody := strings.Replace(
		openingNode,
		fmt.Sprintf(`,"body":%q`, opening.body),
		"",
		1,
	)
	missingEditEvidence := pagedHistoryCommentJSON(providerComment{
		id: opening.id, login: opening.login, body: opening.body, omitEditCount: true,
	})
	badTimestamp := pagedHistoryCommentJSON(providerComment{
		id: opening.id, login: opening.login, body: opening.body, omitUpdatedAt: true,
	})
	wrongType := strings.Replace(openingNode, "PullRequestReviewComment", "IssueComment", 1)

	tests := []struct {
		name     string
		response string
	}{
		{name: "provider errors", response: strings.Replace(valid, `{"data":`, `{"errors":[{"message":"redacted"}],"data":`, 1)},
		{name: "partial data", response: `{"data":{"target":null,"snapshot":[]}}`},
		{name: "resolved state omitted", response: strings.Replace(valid, `"isResolved":false,`, "", 1)},
		{name: "path omitted", response: strings.Replace(valid, `"path":"review.go",`, "", 1)},
		{name: "count drift", response: pagedSnapshotRaw(threadID, 4, [][]string{validSnapshot}, []string{openingNode}, validRecent)},
		{name: "null snapshot node", response: pagedSnapshotRaw(threadID, 3, [][]string{{openingNode, "null", lastNode}}, []string{openingNode}, validRecent)},
		{name: "wrong node type", response: pagedSnapshotRaw(threadID, 3, [][]string{{wrongType, middleNode, lastNode}}, []string{openingNode}, validRecent)},
		{name: "duplicate snapshot node", response: pagedSnapshotRaw(threadID, 3, [][]string{{openingNode, openingNode, lastNode}}, []string{openingNode}, validRecent)},
		{name: "unknown snapshot node", response: pagedSnapshotRaw(threadID, 3, [][]string{{openingNode, unknownNode, lastNode}}, []string{openingNode}, validRecent)},
		{name: "body omitted", response: pagedSnapshotRaw(threadID, 3, [][]string{{missingBody, middleNode, lastNode}}, []string{openingNode}, validRecent)},
		{name: "timestamp omitted", response: pagedSnapshotRaw(threadID, 3, [][]string{{badTimestamp, middleNode, lastNode}}, []string{openingNode}, validRecent)},
		{name: "edit evidence omitted", response: pagedSnapshotRaw(threadID, 3, [][]string{{missingEditEvidence, middleNode, lastNode}}, []string{openingNode}, validRecent)},
		{name: "opening mismatch", response: pagedSnapshotRaw(threadID, 3, [][]string{validSnapshot}, []string{middleNode}, validRecent)},
		{name: "tail mismatch", response: pagedSnapshotRaw(threadID, 3, [][]string{validSnapshot}, []string{openingNode}, []string{lastNode, middleNode})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t, providerStep{stdout: tc.response})
			_, err := revalidatePagedHistory(
				context.Background(),
				threadID,
				observedPagedComments(opening, middle, last),
			)
			if err == nil {
				t.Fatal("incomplete or inconsistent snapshot was accepted")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "history-snapshot")
			assertNoProviderMutation(t, provider)
			assertContainsNone(t, err.Error(), opening.body, middle.body, last.body, unknown.body)
		})
	}

	t.Run("duplicate observed ID fails before provider access", func(t *testing.T) {
		provider := installSequencedProvider(t)
		_, err := revalidatePagedHistory(context.Background(), threadID, []tailComment{
			{ID: opening.id},
			{ID: opening.id},
		})
		if err == nil {
			t.Fatal("duplicate observed IDs were accepted")
		}
		if got := provider.requestCount(); got != 0 {
			t.Fatalf("duplicate observed IDs reached the provider with %d requests", got)
		}
	})

	t.Run("multiple aliases are complete and globally unique", func(t *testing.T) {
		comments := providerCommentRange("INVALID-ALIASES", 101)
		nodes := make([]string, 0, len(comments))
		for _, comment := range comments {
			nodes = append(nodes, pagedHistoryCommentJSON(comment))
		}
		responses := map[string]string{
			"missing second alias": pagedSnapshotRaw(
				threadID, len(comments), [][]string{nodes[:100]},
				[]string{nodes[0]}, nodes[99:],
			),
			"null node in second alias": pagedSnapshotRaw(
				threadID, len(comments), [][]string{nodes[:100], {"null"}},
				[]string{nodes[0]}, nodes[99:],
			),
			"duplicate across aliases": pagedSnapshotRaw(
				threadID, len(comments), [][]string{nodes[:100], {nodes[0]}},
				[]string{nodes[0]}, nodes[99:],
			),
			"extra alias": pagedSnapshotRaw(
				threadID, len(comments), [][]string{nodes[:100], nodes[100:], {}},
				[]string{nodes[0]}, nodes[99:],
			),
		}
		for name, response := range responses {
			t.Run(name, func(t *testing.T) {
				provider := installSequencedProvider(t, providerStep{stdout: response})
				_, err := revalidatePagedHistory(
					context.Background(),
					threadID,
					observedPagedComments(comments...),
				)
				if err == nil {
					t.Fatal("invalid multi-alias snapshot was accepted")
				}
				provider.assertExhausted()
				assertQueryKinds(t, provider, "history-snapshot")
				assertNoProviderMutation(t, provider)
			})
		}

		t.Run("equal-sized aliases cannot be swapped", func(t *testing.T) {
			comments := providerCommentRange("SWAPPED-ALIASES", 200)
			nodes := make([]string, 0, len(comments))
			for _, comment := range comments {
				nodes = append(nodes, pagedHistoryCommentJSON(comment))
			}
			provider := installSequencedProvider(t, providerStep{stdout: pagedSnapshotRaw(
				threadID,
				len(comments),
				[][]string{nodes[100:], nodes[:100]},
				[]string{nodes[0]},
				nodes[len(nodes)-2:],
			)})
			_, err := revalidatePagedHistory(
				context.Background(),
				threadID,
				observedPagedComments(comments...),
			)
			if err == nil {
				t.Fatal("equal-sized swapped alias payloads were accepted")
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "history-snapshot")
			assertNoProviderMutation(t, provider)
			assertContainsAll(t, strings.ToLower(err.Error()), "another alias")
		})
	})
}

func TestPagedHistorySnapshotProviderFailureIsSingleBodySafeRequest(t *testing.T) {
	const (
		threadID = "PRRT_paged_provider_failure"
		secret   = "PRIVATE-PAGED-PROVIDER-BODY"
	)
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: secret}
	last := providerComment{id: "PRRC_last", login: "reviewer", body: "PRIVATE-LAST"}
	comments := providerCommentRange("PAGED-FAILURE", 101)
	comments[0] = opening
	comments[100] = last
	provider := installSequencedProvider(t,
		providerStep{stdout: pagedHistoryPageResponse(threadID, true, "cursor-1", comments[:100]...)},
		providerStep{stdout: pagedHistoryPageResponse(threadID, false, "", comments[100:]...)},
		providerStep{
			stdout: fmt.Sprintf(`{"data":{"snapshot":[{"body":%q}]}}`, secret),
			stderr: "gh: GraphQL: Resource not accessible by personal access token\nprovider echoed " + secret,
			exit:   1,
		},
	)

	code, stdout, diagnostics := provider.capture(func() int {
		_, _, code := fetchHistoryTail(context.Background(), threadID, "reply.md")
		return code
	})
	if code == 0 {
		t.Fatal("paged snapshot provider failure was accepted")
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "history", "history", "history-snapshot")
	assertNoProviderMutation(t, provider)
	assertContainsAll(t, strings.ToLower(diagnostics), "provider state is unverified", "provider access denied", "diagnostics suppressed")
	assertContainsNone(t, stdout+diagnostics, secret)
}

func TestPagedHistorySnapshotQueryContract(t *testing.T) {
	const threadID = "PRRT_paged_query_contract"
	comments := providerCommentRange("PAGED-QUERY", 101)
	operation := buildPagedHistorySnapshotOperation(
		threadID,
		func() []string {
			ids := make([]string, 0, len(comments))
			for _, comment := range comments {
				ids = append(ids, comment.id)
			}
			return ids
		}(),
	)
	for _, selection := range []string{
		"target: node(id:$thread)",
		"snapshot0: nodes(ids:$commentIDs0)",
		"snapshot1: nodes(ids:$commentIDs1)",
		"opening: comments(first:1)",
		"recent: comments(last:2)",
		"totalCount",
		"id",
		"isResolved",
		"path",
		"author { login }",
		"body",
		"updatedAt",
		"userContentEdits(last:1) { totalCount nodes { id } }",
	} {
		if !strings.Contains(operation.Query, selection) {
			t.Errorf("paged history snapshot query omitted %q:\n%s", selection, operation.Query)
		}
	}
	if got := strings.Count(operation.Query, "nodes(ids:$commentIDs"); got != 2 {
		t.Errorf("bulk comment lookup field count = %d, want two 100-bounded aliases", got)
	}
	opening := comments[0]
	last := comments[len(comments)-1]
	provider := installSequencedProvider(t, providerStep{stdout: pagedSnapshotResponse(
		threadID,
		comments,
		opening,
		comments[len(comments)-2],
		last,
	)})
	if _, err := revalidatePagedHistory(
		context.Background(),
		threadID,
		observedPagedComments(comments...),
	); err != nil {
		t.Fatalf("valid paged snapshot: %v", err)
	}
	provider.assertExhausted()
	request := provider.requests()[0]
	var firstIDs, secondIDs []string
	if err := json.Unmarshal(request.Variables["commentIDs0"], &firstIDs); err != nil {
		t.Fatalf("decode first bulk comment-ID field: %v", err)
	}
	if err := json.Unmarshal(request.Variables["commentIDs1"], &secondIDs); err != nil {
		t.Fatalf("decode second bulk comment-ID field: %v", err)
	}
	if len(firstIDs) != 100 || len(secondIDs) != 1 {
		t.Errorf("bulk field sizes = %d and %d, want 100 and 1", len(firstIDs), len(secondIDs))
	}
	gotIDs := append(append([]string(nil), firstIDs...), secondIDs...)
	wantIDs := make([]string, 0, len(comments))
	for _, comment := range comments {
		wantIDs = append(wantIDs, comment.id)
	}
	if strings.Join(gotIDs, ",") != strings.Join(wantIDs, ",") {
		t.Errorf("bulk comment IDs did not preserve the original complete order")
	}
	var gotThreadID string
	if err := json.Unmarshal(request.Variables["thread"], &gotThreadID); err != nil {
		t.Fatalf("decode snapshot thread ID: %v", err)
	}
	if gotThreadID != threadID {
		t.Errorf("snapshot thread ID = %q, want %q", gotThreadID, threadID)
	}
}
