package main

import (
	"context"
	"strings"
	"testing"
)

func editEvidence(count int, ids ...string) *userContentEditsEvidence {
	nodes := make([]userContentEditNodeEvidence, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, userContentEditNodeEvidence{ID: id})
	}
	return &userContentEditsEvidence{TotalCount: &count, Nodes: &nodes}
}

func TestProviderReadsCarryLastEditNodeIDs(t *testing.T) {
	const threadID = "PRRT_provider_edit_revisions"
	predecessor := providerComment{
		id: "PRRC_predecessor", login: "reviewer", body: "P1: bind revision evidence.",
		editCount: 100, lastEditID: "UCE_predecessor_latest",
	}
	reply := providerComment{
		id: "PRRC_reply", login: "author", body: "The revision is bound.",
		editCount: 101, lastEditID: "UCE_reply_latest",
	}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, predecessor, reply)},
		providerStep{stdout: historyResponse(threadID, predecessor, reply)},
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, predecessor, reply)},
	)

	current, err := fetchThread(context.Background(), threadID)
	if err != nil {
		t.Fatalf("fetch current thread: %v", err)
	}
	if current.PrevEditCount != 100 || current.PrevEditID != "UCE_predecessor_latest" ||
		!current.PrevEditCountPresent || !current.PrevEditIDPresent ||
		current.LastEditCount != 101 || current.LastEditID != "UCE_reply_latest" ||
		!current.LastEditCountPresent || !current.LastEditIDPresent {
		t.Fatalf("current edit revisions were not preserved: %+v", current)
	}

	history, err := fetchAllComments(context.Background(), threadID)
	if err != nil {
		t.Fatalf("fetch full history: %v", err)
	}
	if len(history) != 2 || history[0].LastEditID != "UCE_predecessor_latest" ||
		!history[0].LastEditIDPresent || history[1].LastEditID != "UCE_reply_latest" ||
		!history[1].LastEditIDPresent {
		t.Fatalf("history edit revisions were not preserved: %+v", history)
	}

	_, state, err := mutateThread(context.Background(), "resolve", threadID)
	if err != nil {
		t.Fatalf("parse resolve mutation state: %v", err)
	}
	if state.PrevEditID != "UCE_predecessor_latest" || !state.PrevEditIDPresent ||
		state.LastEditID != "UCE_reply_latest" || !state.LastEditIDPresent {
		t.Fatalf("mutation edit revisions were not preserved: %+v", state)
	}
	provider.assertExhausted()
}

func TestObservedEditRevisionRequiresCountAndLastNode(t *testing.T) {
	zero := 0
	hundred := 100
	emptyNodes := []userContentEditNodeEvidence{}
	oneNode := []userContentEditNodeEvidence{{ID: "UCE_latest"}}

	tests := []struct {
		name        string
		evidence    *userContentEditsEvidence
		wantCount   int
		wantLastID  string
		wantPresent bool
	}{
		{name: "connection omitted"},
		{name: "count omitted", evidence: &userContentEditsEvidence{Nodes: &emptyNodes}},
		{name: "nodes omitted", evidence: &userContentEditsEvidence{TotalCount: &zero}},
		{name: "zero edits", evidence: editEvidence(0), wantPresent: true},
		{name: "zero count with node", evidence: &userContentEditsEvidence{TotalCount: &zero, Nodes: &oneNode}},
		{name: "positive count without last node", evidence: &userContentEditsEvidence{TotalCount: &hundred, Nodes: &emptyNodes}},
		{name: "positive count with last node", evidence: editEvidence(100, "UCE_latest"), wantCount: 100, wantLastID: "UCE_latest", wantPresent: true},
		{name: "positive count with unsafe last node", evidence: editEvidence(100, "UCE_latest\nforged")},
		{name: "positive count with multiple last nodes", evidence: editEvidence(100, "UCE_old", "UCE_latest")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			count, lastID, present := observedEditRevision(tc.evidence)
			if count != tc.wantCount || lastID != tc.wantLastID || present != tc.wantPresent {
				t.Fatalf("observedEditRevision() = (%d, %q, %t), want (%d, %q, %t)",
					count, lastID, present, tc.wantCount, tc.wantLastID, tc.wantPresent)
			}
		})
	}
}

func TestProviderQueriesRequestLastEditNodeID(t *testing.T) {
	const selection = "userContentEdits(last:1) { totalCount nodes { id } }"
	pagedSnapshotQuery := buildPagedHistorySnapshotOperation(
		"PRRT_query_contract",
		[]string{"PRRC_query_contract"},
	).Query
	queries := map[string]string{
		"list":           listQuery,
		"thread":         threadByIDQuery,
		"history":        threadCommentsQuery,
		"paged snapshot": pagedSnapshotQuery,
		"resolve":        resolveMutation,
		"reply mutation": replyMutation,
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(query, selection) {
				t.Fatalf("query omitted the last edit node selection %q:\n%s", selection, query)
			}
		})
	}
}

func TestReplyMutationEvidenceCarriesLastEditNodeID(t *testing.T) {
	const body = "Bind the complete provider edit revision."
	evidence, err := parseReplyMutationEvidence(
		[]byte(replyResponse(providerComment{
			id:         "PRRC_reply_revision",
			login:      "author",
			body:       body,
			editCount:  100,
			lastEditID: "UCE_reply_latest",
		})),
		body,
	)
	if err != nil {
		t.Fatalf("parse reply mutation evidence: %v", err)
	}
	if evidence.EditCount != 100 || !evidence.EditCountPresent ||
		evidence.LastEditID != "UCE_reply_latest" || !evidence.LastEditIDPresent {
		t.Fatalf("reply edit revision = count %d/%t last ID %q/%t",
			evidence.EditCount,
			evidence.EditCountPresent,
			evidence.LastEditID,
			evidence.LastEditIDPresent,
		)
	}
}

func TestValidateContinuationHistoryRejectsChangedLastEditIDAtFixedCount(t *testing.T) {
	const (
		threadID          = "PRRT_edit_revision"
		predecessorID     = "PRRC_predecessor"
		replyID           = "PRRC_reply"
		predecessorBody   = "P1: preserve the exact predecessor revision."
		replyBody         = "The reply is bound to that exact revision."
		predecessorEditID = "UCE_predecessor_100"
		replyEditID       = "UCE_reply_100"
	)
	receipt, err := newContinuationReceipt(
		"github.com",
		threadID,
		predecessorID,
		replyID,
		[]byte(predecessorBody),
		[]byte(replyBody),
		providerFixtureUpdatedAt,
		providerFixtureUpdatedAt,
		100,
		100,
		predecessorEditID,
		replyEditID,
		"reviewer",
		"author",
	)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	history := []tailComment{
		{
			ID: predecessorID, Login: "reviewer", Body: predecessorBody,
			UpdatedAt: providerFixtureUpdatedAt, EditCount: 100, EditCountPresent: true,
			LastEditID: predecessorEditID, LastEditIDPresent: true,
		},
		{
			ID: replyID, Login: "author", Body: replyBody,
			UpdatedAt: providerFixtureUpdatedAt, EditCount: 100, EditCountPresent: true,
			LastEditID: "UCE_reply_changed_at_same_count", LastEditIDPresent: true,
		},
	}
	if err := validateContinuationHistory(receipt, history); err == nil ||
		!strings.Contains(err.Error(), "edit revision changed") {
		t.Fatalf("fixed-count latest-edit substitution error = %v, want edit-revision rejection", err)
	}
}

func TestRecoveredReplyRejectsChangedLastEditIDAtFixedCount(t *testing.T) {
	const (
		predecessorID     = "PRRC_predecessor"
		replyID           = "PRRC_reply"
		predecessorBody   = "P1: bind the predecessor revision."
		replyBody         = "Bind the recovered reply revision too."
		predecessorEditID = "UCE_predecessor_100"
		replyEditID       = "UCE_reply_100"
	)
	predecessor := replyIssuancePredecessor{
		ID: predecessorID, BodySHA256: exactBodySHA256([]byte(predecessorBody)),
		UpdatedAt: providerFixtureUpdatedAt, EditCount: 100, EditCountPresent: true,
		LastEditID: predecessorEditID, LastEditIDPresent: true,
		OpeningAuthor: "reviewer", Author: "reviewer",
		ReplyBodySHA256: exactBodySHA256([]byte(replyBody)),
	}
	observed := replyMutationEvidence{
		ID: replyID, Author: "author", BodySHA256: exactBodySHA256([]byte(replyBody)),
		UpdatedAt: providerFixtureUpdatedAt, EditCount: 100, EditCountPresent: true,
		LastEditID: replyEditID, LastEditIDPresent: true,
		ObservedOpeningAuthor: "reviewer", ObservedPredecessorTime: providerFixtureUpdatedAt,
		ObservedPredecessorEditCount: 100, ObservedPredecessorEditCountPresent: true,
		ObservedPredecessorLastEditID: predecessorEditID, ObservedPredecessorLastEditIDPresent: true,
		Recovered: true,
	}
	history := []tailComment{
		{
			ID: predecessorID, Login: "reviewer", Body: predecessorBody,
			UpdatedAt: providerFixtureUpdatedAt, EditCount: 100, EditCountPresent: true,
			LastEditID: predecessorEditID, LastEditIDPresent: true,
		},
		{
			ID: replyID, Login: "author", Body: replyBody,
			UpdatedAt: providerFixtureUpdatedAt, EditCount: 100, EditCountPresent: true,
			LastEditID: "UCE_reply_changed_at_same_count", LastEditIDPresent: true,
		},
	}
	if _, err := issuedReplyEvidenceFromHistory(history, predecessor, observed); err == nil {
		t.Fatal("recovered reply accepted a changed latest edit ID at the same totalCount")
	}
}

func TestContinuationBoundaryRejectsChangedLastEditIDAtFixedCount(t *testing.T) {
	const (
		predecessorBody = "P1: exact predecessor"
		replyBody       = "Exact reply"
	)
	receipt, err := newContinuationReceipt(
		"github.com", "PRRT_current_revision", "PRRC_predecessor", "PRRC_reply",
		[]byte(predecessorBody), []byte(replyBody),
		providerFixtureUpdatedAt, providerFixtureUpdatedAt,
		100, 100, "UCE_predecessor_100", "UCE_reply_100", "reviewer", "author",
	)
	if err != nil {
		t.Fatalf("create receipt: %v", err)
	}
	cur := thread{
		ID: "PRRT_current_revision", IsResolved: true, Path: "review.go",
		Author: "reviewer", LastAuthor: "author", Answered: true,
		PrevID: "PRRC_predecessor", LastID: "PRRC_reply",
		PrevBody: predecessorBody, PrevBodyPresent: true, LastBody: replyBody, LastBodyPresent: true,
		PrevUpdatedAt: providerFixtureUpdatedAt, LastUpdatedAt: providerFixtureUpdatedAt,
		PrevEditCount: 100, PrevEditCountPresent: true, LastEditCount: 100, LastEditCountPresent: true,
		PrevEditID: "UCE_predecessor_100", PrevEditIDPresent: true,
		LastEditID: "UCE_reply_changed_at_same_count", LastEditIDPresent: true,
	}
	if got := classifyContinuationThreadBoundary(receipt, cur); got != continuationBoundaryMismatch {
		t.Fatalf("boundary classification = %v, want mismatch for changed latest edit ID", got)
	}
}

func TestReplyHistoryBoundaryRejectsChangedLastEditIDAtFixedCount(t *testing.T) {
	const body = "P1: preserve this exact edit revision."
	boundary := boundaryFromHistory([]tailComment{{
		ID: "PRRC_tail", Login: "reviewer", Body: body, UpdatedAt: providerFixtureUpdatedAt,
		EditCount: 100, EditCountPresent: true,
		LastEditID: "UCE_original_latest", LastEditIDPresent: true,
	}})
	cur := thread{
		Author: "reviewer", LastAuthor: "reviewer", LastID: "PRRC_tail",
		LastBody: body, LastBodyPresent: true, LastUpdatedAt: providerFixtureUpdatedAt,
		LastEditCount: 100, LastEditCountPresent: true,
		LastEditID: "UCE_changed_at_same_count", LastEditIDPresent: true,
	}
	if detail := boundary.mismatch(cur); !strings.Contains(detail, "edit revision") {
		t.Fatalf("boundary mismatch = %q, want fixed-count latest-edit rejection", detail)
	}
}
