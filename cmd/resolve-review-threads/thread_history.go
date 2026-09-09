// Full-history retrieval and exact tail-boundary evidence for review threads.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// threadCommentsQuery pages forward through a thread's entire comment list.
// Deciding that no prior reply exists has to be a fact, not the result of a
// bounded look: a reply pushed out of the tail by later discussion would be
// reposted, and the repost anchors to the newest follow-up.
const threadCommentsQuery = `query($id:ID!, $after:String) {
  node(id:$id) {
    id
    ... on PullRequestReviewThread {
      comments(first:100, after:$after) {
        pageInfo { hasNextPage endCursor }
        nodes { id author { login } body updatedAt userContentEdits(last:1) { totalCount nodes { id } } }
      }
    }
  }
}`

const maxPagedHistorySnapshotIDsPerField = 100

const pagedHistorySnapshotTargetSelection = `  target: node(id:$thread) {
    ... on PullRequestReviewThread {
      id
      isResolved
      path
      opening: comments(first:1) {
        totalCount
        nodes { __typename id author { login } body updatedAt userContentEdits(last:1) { totalCount nodes { id } } }
      }
      recent: comments(last:2) {
        nodes { __typename id author { login } body updatedAt userContentEdits(last:1) { totalCount nodes { id } } }
      }
    }
  }`

const pagedHistorySnapshotNodeSelection = `{
    __typename
    ... on PullRequestReviewComment {
      id
      author { login }
      body
      updatedAt
      userContentEdits(last:1) { totalCount nodes { id } }
    }
  }`

type pagedHistorySnapshotOperation struct {
	Query     string
	Variables map[string]any
	Aliases   []string
	BatchIDs  [][]string
}

// buildPagedHistorySnapshotOperation refreshes every comment that a
// multi-request history walk observed and binds that refreshed set to the same
// thread's count and end points in one provider operation. GitHub rejects more
// than 100 IDs in one nodes(ids:) field, so deterministic aliases partition the
// IDs within the single GraphQL document and response. The lookup is never
// split across independently timed provider operations.
func buildPagedHistorySnapshotOperation(
	threadID string,
	commentIDs []string,
) pagedHistorySnapshotOperation {
	definitions := []string{"$thread:ID!"}
	selections := []string{pagedHistorySnapshotTargetSelection}
	variables := map[string]any{"thread": threadID}
	aliases := make([]string, 0, (len(commentIDs)+maxPagedHistorySnapshotIDsPerField-1)/maxPagedHistorySnapshotIDsPerField)
	batchIDs := make([][]string, 0, cap(aliases))
	for start, batch := 0, 0; start < len(commentIDs); start, batch = start+maxPagedHistorySnapshotIDsPerField, batch+1 {
		end := start + maxPagedHistorySnapshotIDsPerField
		if end > len(commentIDs) {
			end = len(commentIDs)
		}
		variableName := fmt.Sprintf("commentIDs%d", batch)
		alias := fmt.Sprintf("snapshot%d", batch)
		definitions = append(definitions, fmt.Sprintf("$%s:[ID!]!", variableName))
		selections = append(selections, fmt.Sprintf("  %s: nodes(ids:$%s) %s", alias, variableName, pagedHistorySnapshotNodeSelection))
		ids := append([]string(nil), commentIDs[start:end]...)
		variables[variableName] = ids
		aliases = append(aliases, alias)
		batchIDs = append(batchIDs, ids)
	}
	return pagedHistorySnapshotOperation{
		Query:     fmt.Sprintf("query(%s) {\n%s\n}", strings.Join(definitions, ","), strings.Join(selections, "\n")),
		Variables: variables,
		Aliases:   aliases,
		BatchIDs:  batchIDs,
	}
}

// tailComment is one provider-visible review-thread comment.
type tailComment struct {
	ID                string
	Login             string
	Body              string
	UpdatedAt         string
	EditCount         int
	EditCountPresent  bool
	LastEditID        string
	LastEditIDPresent bool
}

// stableReplyHistory is the only full-history type reply-resolve may classify.
// A one-page connection is one provider observation; a multi-page connection
// enters this type only after the generated paged snapshot operation validates
// and refreshes every decision-bearing field.
type stableReplyHistory struct {
	comments []tailComment
}

func (h stableReplyHistory) classify(body string) priorReplyState {
	return classifyPriorReply(h.comments, body)
}

func (h stableReplyHistory) last() tailComment {
	return h.comments[len(h.comments)-1]
}

type pagedHistoryCommentNode struct {
	TypeName         string                    `json:"__typename"`
	ID               string                    `json:"id"`
	UpdatedAt        string                    `json:"updatedAt"`
	UserContentEdits *userContentEditsEvidence `json:"userContentEdits"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
	Body *string `json:"body"`
}

type pagedHistoryTarget struct {
	ID         string `json:"id"`
	IsResolved *bool  `json:"isResolved"`
	Path       string `json:"path"`
	Opening    struct {
		TotalCount *int                        `json:"totalCount"`
		Nodes      *[]*pagedHistoryCommentNode `json:"nodes"`
	} `json:"opening"`
	Recent struct {
		Nodes *[]*pagedHistoryCommentNode `json:"nodes"`
	} `json:"recent"`
}

type pagedHistorySnapshotResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []json.RawMessage          `json:"errors"`
}

// replyHistoryBoundary binds the final pair of comments used to classify a
// reply. Comment node IDs alone are insufficient because GitHub permits body
// edits without changing those IDs.
type replyHistoryBoundary struct {
	LastID                       string
	LastBodySHA256               string
	LastAuthor                   string
	LastUpdatedAt                string
	LastEditCount                int
	LastEditCountPresent         bool
	LastEditID                   string
	LastEditIDPresent            bool
	PredecessorID                string
	PredecessorBodySHA256        string
	PredecessorUpdatedAt         string
	PredecessorEditCount         int
	PredecessorEditCountPresent  bool
	PredecessorLastEditID        string
	PredecessorLastEditIDPresent bool
	OpeningAuthor                string
}

func boundaryFromHistory(history []tailComment) replyHistoryBoundary {
	if len(history) == 0 {
		return replyHistoryBoundary{}
	}
	last := history[len(history)-1]
	boundary := replyHistoryBoundary{
		LastID:               last.ID,
		LastBodySHA256:       exactBodySHA256([]byte(last.Body)),
		LastAuthor:           last.Login,
		LastUpdatedAt:        last.UpdatedAt,
		LastEditCount:        last.EditCount,
		LastEditCountPresent: last.EditCountPresent,
		LastEditID:           last.LastEditID,
		LastEditIDPresent:    last.LastEditIDPresent,
		OpeningAuthor:        history[0].Login,
	}
	if len(history) > 1 {
		previous := history[len(history)-2]
		boundary.PredecessorID = previous.ID
		boundary.PredecessorBodySHA256 = exactBodySHA256([]byte(previous.Body))
		boundary.PredecessorUpdatedAt = previous.UpdatedAt
		boundary.PredecessorEditCount = previous.EditCount
		boundary.PredecessorEditCountPresent = previous.EditCountPresent
		boundary.PredecessorLastEditID = previous.LastEditID
		boundary.PredecessorLastEditIDPresent = previous.LastEditIDPresent
	}
	return boundary
}

// authorMismatch keeps reply intent bound to the same provider identities as
// the full-history classification. IDs and bodies alone cannot prove whether
// the live tail is a reviewer hand-back or an independently authored answer.
func (b replyHistoryBoundary) authorMismatch(cur thread) string {
	switch {
	case b.OpeningAuthor == "" || b.LastAuthor == "":
		return "full history omitted the opening or current-tail author"
	case cur.Author == "unknown" || cur.LastAuthor == "unknown":
		return "the stable exact-target read omitted the opening or current-tail author"
	case cur.Author != b.OpeningAuthor:
		return "the opening-comment author changed between full history and the stable exact-target read"
	case cur.LastAuthor != b.LastAuthor:
		return "the current-tail author changed between full history and the stable exact-target read"
	default:
		return ""
	}
}

// mismatch describes a changed or incomplete current boundary without
// rendering either comment body.
func (b replyHistoryBoundary) mismatch(cur thread) string {
	if detail := b.identityMismatch(cur); detail != "" {
		return detail
	}
	if detail := b.tailBodyMismatch(cur); detail != "" {
		return detail
	}
	if detail := b.tailRevisionMismatch(cur); detail != "" {
		return detail
	}
	if detail := b.predecessorBodyMismatch(cur); detail != "" {
		return detail
	}
	return b.predecessorRevisionMismatch(cur)
}

func (b replyHistoryBoundary) identityMismatch(cur thread) string {
	switch {
	case cur.LastID != b.LastID:
		return fmt.Sprintf("tail ID changed from %s to %s", b.LastID, cur.LastID)
	case b.PredecessorID != "" && cur.PrevID != "" && cur.PrevID != b.PredecessorID:
		return fmt.Sprintf("predecessor ID changed from %s to %s", b.PredecessorID, cur.PrevID)
	default:
		return ""
	}
}

func (b replyHistoryBoundary) tailBodyMismatch(cur thread) string {
	switch {
	case !cur.LastBodyPresent:
		return "current tail body was omitted"
	case exactBodySHA256([]byte(cur.LastBody)) != b.LastBodySHA256:
		return fmt.Sprintf("body of tail %s changed without changing its ID", b.LastID)
	default:
		return ""
	}
}

func (b replyHistoryBoundary) tailRevisionMismatch(cur thread) string {
	switch {
	case b.LastUpdatedAt == "":
		return "full history omitted the current tail update timestamp"
	case cur.LastUpdatedAt == "":
		return "current tail update timestamp was omitted"
	case cur.LastUpdatedAt != b.LastUpdatedAt:
		return fmt.Sprintf("update timestamp of tail %s changed without changing its ID", b.LastID)
	case !b.LastEditCountPresent:
		return "full history omitted the current tail edit generation"
	case !cur.LastEditCountPresent:
		return "current tail edit generation was omitted"
	case !b.LastEditIDPresent:
		return "full history omitted the current tail last-edit ID"
	case !cur.LastEditIDPresent:
		return "current tail last-edit ID was omitted"
	case cur.LastEditCount != b.LastEditCount || cur.LastEditID != b.LastEditID:
		return fmt.Sprintf("edit revision of tail %s changed without changing its comment ID", b.LastID)
	default:
		return ""
	}
}

func (b replyHistoryBoundary) predecessorBodyMismatch(cur thread) string {
	if b.PredecessorID == "" {
		return ""
	}
	switch {
	case cur.PrevID == "":
		return "current predecessor ID was omitted"
	case !cur.PrevBodyPresent:
		return "current predecessor body was omitted"
	case exactBodySHA256([]byte(cur.PrevBody)) != b.PredecessorBodySHA256:
		return fmt.Sprintf("body of predecessor %s changed without changing its ID", b.PredecessorID)
	default:
		return ""
	}
}

func (b replyHistoryBoundary) predecessorRevisionMismatch(cur thread) string {
	if b.PredecessorID == "" {
		return ""
	}
	switch {
	case b.PredecessorUpdatedAt == "":
		return "full history omitted the predecessor update timestamp"
	case cur.PrevUpdatedAt == "":
		return "current predecessor update timestamp was omitted"
	case cur.PrevUpdatedAt != b.PredecessorUpdatedAt:
		return fmt.Sprintf("update timestamp of predecessor %s changed without changing its ID", b.PredecessorID)
	case !b.PredecessorEditCountPresent:
		return "full history omitted the predecessor edit generation"
	case !cur.PrevEditCountPresent:
		return "current predecessor edit generation was omitted"
	case !b.PredecessorLastEditIDPresent:
		return "full history omitted the predecessor last-edit ID"
	case !cur.PrevEditIDPresent:
		return "current predecessor last-edit ID was omitted"
	case cur.PrevEditCount != b.PredecessorEditCount || cur.PrevEditID != b.PredecessorLastEditID:
		return fmt.Sprintf("edit revision of predecessor %s changed without changing its comment ID", b.PredecessorID)
	default:
		return ""
	}
}

func rejectChangedReplyHistoryBoundary(
	ctx context.Context,
	threadID, bodyFile string,
	cur thread,
	detail string,
	initiallyResolved bool,
) (thread, int) {
	if strings.Contains(detail, "omitted") {
		return cur, fail(
			"provider state is unverified: thread %s cannot be compared with the stable history boundary because its %s; nothing was posted\n%s",
			threadID,
			detail,
			inspectReplyOutcomeGuidance(threadID, bodyFile),
		)
	}
	if initiallyResolved {
		return cur, fail(
			"thread %s was already resolved before this command, and its exact history boundary changed while history was read (%s); no mutation was attempted\n%s",
			threadID,
			detail,
			inspectReplyOutcomeGuidance(threadID, bodyFile),
		)
	}
	state := "the thread remains unresolved"
	if cur.IsResolved {
		reason := fmt.Sprintf(
			"resolved after the exact history boundary changed before reply classification: %s",
			detail,
		)
		if reopenErr := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); reopenErr != nil {
			message, _ := replyResolutionEvidenceFailure(
				threadID,
				reopenErr,
				revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
			)
			return cur, fail("%s", message)
		}
		state = "the stale resolution was automatically reopened and that state was confirmed"
	}
	message, _ := replyResolutionEvidenceFailure(
		threadID,
		&supersededEvidenceError{msg: fmt.Sprintf(
			"%s %s: the history tail changed at its exact boundary while it was being read (%s); nothing was posted; %s",
			threadID,
			cur.Path,
			detail,
			state,
		)},
		revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
	)
	return cur, fail("%s", message)
}

func classifyInitiallyResolvedStableTail(
	threadID, bodyFile string,
	cur thread,
	priorReply priorReplyState,
) (thread, int) {
	if !cur.IsResolved {
		return cur, fail("thread %s was resolved on the initial read but became unresolved before "+
			"the stable-history decision; provider state changed, so no mutation was attempted\n%s",
			threadID,
			inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	switch priorReply {
	case priorReplySuperseded, differentAnswerIsLast, unavailableReplyIntent, priorReplyIsLast:
		// The command did not create the initial resolved state. Carry the
		// stable classification back to cmdReplyResolve, which will either make
		// the exact non-mutating skip or explain the mismatch.
		return cur, -1
	case reviewerHandbackIsLast:
		return cur, fail("thread %s was already resolved before this command, but stable history ends in a reviewer-side hand-back that the selected reply bytes do not yet answer; no mutation was attempted.\n%s",
			threadID, resolvedReviewerHandbackGuidance(threadID, bodyFile))
	case noPriorReply:
		fmt.Printf("skipped %s (already resolved before this command)\n", threadID)
		return cur, 0
	}
	return cur, -1
}

func classifyConcurrentlyResolvedStableTail(
	ctx context.Context,
	threadID, bodyFile string,
	cur thread,
	priorReply priorReplyState,
) (thread, int) {
	if !cur.IsResolved {
		return cur, -1
	}
	if priorReply == priorReplySuperseded {
		return rejectConcurrentlyResolvedSupersededReply(ctx, threadID, bodyFile, cur)
	}
	if priorReply == reviewerHandbackIsLast {
		return rejectConcurrentlyResolvedReviewerHandback(ctx, threadID, bodyFile, cur)
	}
	if priorReply == differentAnswerIsLast ||
		priorReply == unavailableReplyIntent ||
		priorReply == priorReplyIsLast {
		return cur, -1
	}
	fmt.Printf("skipped %s (already resolved)\n", threadID)
	return cur, 0
}

func rejectConcurrentlyResolvedReviewerHandback(
	ctx context.Context,
	threadID, bodyFile string,
	cur thread,
) (thread, int) {
	reason := "resolved while stable history still ended in an unanswered reviewer-side hand-back"
	if reopenErr := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); reopenErr != nil {
		message, _ := replyResolutionEvidenceFailure(
			threadID,
			reopenErr,
			revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
		)
		return cur, fail("%s", message)
	}
	message, _ := replyResolutionEvidenceFailure(
		threadID,
		&supersededEvidenceError{msg: fmt.Sprintf(
			"%s %s: the stable reviewer-side hand-back remains unanswered; the concurrent stale resolution was automatically reopened",
			threadID,
			cur.Path,
		)},
		revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
	)
	return cur, fail("%s", message)
}

func rejectConcurrentlyResolvedSupersededReply(
	ctx context.Context,
	threadID, bodyFile string,
	cur thread,
) (thread, int) {
	reason := "resolved while an earlier matching reply was already followed by newer commentary"
	if reopenErr := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); reopenErr != nil {
		message, _ := replyResolutionEvidenceFailure(
			threadID,
			reopenErr,
			revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
		)
		return cur, fail("%s", message)
	}
	message, _ := replyResolutionEvidenceFailure(
		threadID,
		&supersededEvidenceError{msg: fmt.Sprintf(
			"%s %s: the earlier matching reply is followed by newer commentary; the stale resolution was automatically reopened",
			threadID,
			cur.Path,
		)},
		revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
	)
	return cur, fail("%s", message)
}

// checkCursorAdvances rejects a page cursor that has not moved. A response
// claiming another page while returning an empty or repeated cursor would
// otherwise loop forever and hammer the API.
func checkCursorAdvances(endCursor, current string) error {
	if endCursor == "" || endCursor == current {
		return fmt.Errorf("pagination did not advance")
	}
	return nil
}

// fetchAllComments returns every comment on a thread, oldest first, following
// cursor pagination. It verifies the exact requested thread and requires IDs
// and bodies because continuation evidence binds both.
func fetchAllComments(ctx context.Context, threadID string) ([]tailComment, error) {
	return fetchAllCommentsObserved(ctx, threadID, nil)
}

// fetchAllCommentsObserved optionally reports whether the complete walk
// required more than one provider request. Callers that make classification
// decisions use that fact to require a coherent one-operation refresh.
func fetchAllCommentsObserved(ctx context.Context, threadID string, paged *bool) ([]tailComment, error) {
	var all []tailComment
	cursor := ""
	pages := 0
	for {
		variables := map[string]any{"id": threadID}
		if cursor != "" {
			variables["after"] = cursor
		}
		raw, err := ghGraphQL(ctx, threadCommentsQuery, variables)
		if err != nil {
			return nil, err
		}
		pages++
		if paged != nil {
			*paged = pages > 1
		}
		var resp struct {
			Data struct {
				Node struct {
					ID       string `json:"id"`
					Comments struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID               string                    `json:"id"`
							UpdatedAt        string                    `json:"updatedAt"`
							UserContentEdits *userContentEditsEvidence `json:"userContentEdits"`
							Author           struct {
								Login string `json:"login"`
							} `json:"author"`
							Body *string `json:"body"`
						} `json:"nodes"`
					} `json:"comments"`
				} `json:"node"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("parse comments for %s: %w", threadID, err)
		}
		if resp.Data.Node.ID == "" {
			return nil, fmt.Errorf("comments response omitted the requested thread ID %s", threadID)
		}
		if resp.Data.Node.ID != threadID {
			return nil, fmt.Errorf(
				"comments query for %s returned mismatched ID %s",
				threadID,
				resp.Data.Node.ID,
			)
		}
		comments := resp.Data.Node.Comments
		for _, comment := range comments.Nodes {
			if comment.ID == "" {
				return nil, fmt.Errorf("comments response for %s omitted a comment ID", threadID)
			}
			if comment.Body == nil {
				return nil, fmt.Errorf(
					"comments response for %s omitted body for comment %s",
					threadID,
					comment.ID,
				)
			}
			observed := tailComment{
				ID:        comment.ID,
				Login:     comment.Author.Login,
				Body:      *comment.Body,
				UpdatedAt: comment.UpdatedAt,
			}
			if editCount, lastEditID, present := observedEditRevision(comment.UserContentEdits); present {
				observed.EditCount = editCount
				observed.EditCountPresent = true
				observed.LastEditID = lastEditID
				observed.LastEditIDPresent = true
			}
			all = append(all, observed)
		}
		if !comments.PageInfo.HasNextPage {
			return all, nil
		}
		if err := checkCursorAdvances(comments.PageInfo.EndCursor, cursor); err != nil {
			return nil, fmt.Errorf("paging comments for %s: %w", threadID, err)
		}
		cursor = comments.PageInfo.EndCursor
	}
}

func decodePagedHistoryComment(
	threadID, location string,
	node *pagedHistoryCommentNode,
) (tailComment, error) {
	if node == nil {
		return tailComment{}, fmt.Errorf("paged history snapshot for %s returned a null %s", threadID, location)
	}
	if node.TypeName != "PullRequestReviewComment" {
		return tailComment{}, fmt.Errorf(
			"paged history snapshot for %s returned non-review-comment %s evidence",
			threadID,
			location,
		)
	}
	if !validContinuationReceiptID(node.ID) {
		return tailComment{}, fmt.Errorf("paged history snapshot for %s omitted a safe %s ID", threadID, location)
	}
	if !validContinuationAuthor(node.Author.Login) {
		return tailComment{}, fmt.Errorf("paged history snapshot for %s omitted a safe %s author", threadID, location)
	}
	if node.Body == nil {
		return tailComment{}, fmt.Errorf("paged history snapshot for %s omitted the %s body", threadID, location)
	}
	if !validContinuationTimestamp(node.UpdatedAt) {
		return tailComment{}, fmt.Errorf("paged history snapshot for %s omitted a valid %s update timestamp", threadID, location)
	}
	editCount, lastEditID, present := observedEditRevision(node.UserContentEdits)
	if !present {
		return tailComment{}, fmt.Errorf("paged history snapshot for %s omitted complete %s edit evidence", threadID, location)
	}
	return tailComment{
		ID:                node.ID,
		Login:             node.Author.Login,
		Body:              *node.Body,
		UpdatedAt:         node.UpdatedAt,
		EditCount:         editCount,
		EditCountPresent:  true,
		LastEditID:        lastEditID,
		LastEditIDPresent: true,
	}, nil
}

func samePagedHistoryComment(left, right tailComment) bool {
	return left == right
}

// revalidatePagedHistory turns a mixed-request history walk into the stable
// classification input. The observed IDs define the set and order; one bulk
// provider operation refreshes every field and binds it to the target's count,
// opening comment, and final pair. It intentionally makes no claim about a
// provider transaction spanning this read and a later mutation.
func revalidatePagedHistory(
	ctx context.Context,
	threadID string,
	observed []tailComment,
) (stableReplyHistory, error) {
	if len(observed) == 0 {
		return stableReplyHistory{}, fmt.Errorf("paged history for %s was empty", threadID)
	}
	commentIDs := make([]string, 0, len(observed))
	seenObserved := make(map[string]struct{}, len(observed))
	for _, comment := range observed {
		if !validContinuationReceiptID(comment.ID) {
			return stableReplyHistory{}, fmt.Errorf("paged history for %s omitted a safe comment ID", threadID)
		}
		if _, duplicate := seenObserved[comment.ID]; duplicate {
			return stableReplyHistory{}, fmt.Errorf("paged history for %s repeated comment ID %s", threadID, comment.ID)
		}
		seenObserved[comment.ID] = struct{}{}
		commentIDs = append(commentIDs, comment.ID)
	}

	operation := buildPagedHistorySnapshotOperation(threadID, commentIDs)
	raw, err := ghGraphQL(ctx, operation.Query, operation.Variables)
	if err != nil {
		return stableReplyHistory{}, fmt.Errorf("refresh paged history for %s: %w", threadID, err)
	}
	var resp pagedHistorySnapshotResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return stableReplyHistory{}, fmt.Errorf("parse paged history snapshot for %s: %w", threadID, err)
	}
	if len(resp.Errors) != 0 {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s contained provider errors", threadID)
	}
	if resp.Data == nil {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s was partial", threadID)
	}
	if len(resp.Data) != len(operation.Aliases)+1 {
		return stableReplyHistory{}, fmt.Errorf(
			"paged history snapshot for %s returned an unexpected number of fields",
			threadID,
		)
	}
	targetRaw, present := resp.Data["target"]
	if !present {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s omitted its target", threadID)
	}
	var target *pagedHistoryTarget
	if err := json.Unmarshal(targetRaw, &target); err != nil {
		return stableReplyHistory{}, fmt.Errorf("parse paged history target for %s: %w", threadID, err)
	}
	if target == nil {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s returned a null target", threadID)
	}
	if target.ID == "" {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot omitted the requested thread ID %s", threadID)
	}
	if target.ID != threadID {
		return stableReplyHistory{}, fmt.Errorf(
			"paged history snapshot for %s returned mismatched thread ID %s",
			threadID,
			target.ID,
		)
	}
	if target.IsResolved == nil {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s omitted its resolved state", threadID)
	}
	if target.Path == "" {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s omitted its path", threadID)
	}
	if target.Opening.TotalCount == nil || target.Opening.Nodes == nil || target.Recent.Nodes == nil {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s omitted its count, opening, or tail", threadID)
	}
	if *target.Opening.TotalCount != len(observed) {
		return stableReplyHistory{}, fmt.Errorf(
			"paged history snapshot for %s changed comment count from %d to %d",
			threadID,
			len(observed),
			*target.Opening.TotalCount,
		)
	}

	snapshot := make([]tailComment, 0, len(observed))
	for i, alias := range operation.Aliases {
		batchRaw, ok := resp.Data[alias]
		if !ok {
			return stableReplyHistory{}, fmt.Errorf(
				"paged history snapshot for %s omitted alias %s",
				threadID,
				alias,
			)
		}
		var batch []*pagedHistoryCommentNode
		if err := json.Unmarshal(batchRaw, &batch); err != nil {
			return stableReplyHistory{}, fmt.Errorf(
				"parse paged history snapshot alias %s for %s: %w",
				alias,
				threadID,
				err,
			)
		}
		if len(batch) != len(operation.BatchIDs[i]) {
			return stableReplyHistory{}, fmt.Errorf(
				"paged history snapshot alias %s for %s returned %d comments, want %d",
				alias,
				threadID,
				len(batch),
				len(operation.BatchIDs[i]),
			)
		}
		expectedBatchIDs := make(map[string]struct{}, len(operation.BatchIDs[i]))
		for _, id := range operation.BatchIDs[i] {
			expectedBatchIDs[id] = struct{}{}
		}
		seenBatchIDs := make(map[string]struct{}, len(batch))
		for j, node := range batch {
			comment, err := decodePagedHistoryComment(
				threadID,
				fmt.Sprintf("alias %s comment %d", alias, j+1),
				node,
			)
			if err != nil {
				return stableReplyHistory{}, err
			}
			if _, requested := expectedBatchIDs[comment.ID]; !requested {
				return stableReplyHistory{}, fmt.Errorf(
					"paged history snapshot alias %s for %s returned an ID requested through another alias",
					alias,
					threadID,
				)
			}
			if _, duplicate := seenBatchIDs[comment.ID]; duplicate {
				return stableReplyHistory{}, fmt.Errorf(
					"paged history snapshot alias %s for %s repeated a comment ID",
					alias,
					threadID,
				)
			}
			seenBatchIDs[comment.ID] = struct{}{}
			snapshot = append(snapshot, comment)
		}
	}
	refreshedByID := make(map[string]tailComment, len(snapshot))
	for _, comment := range snapshot {
		if _, known := seenObserved[comment.ID]; !known {
			return stableReplyHistory{}, fmt.Errorf(
				"paged history snapshot for %s returned unrequested comment ID %s",
				threadID,
				comment.ID,
			)
		}
		if _, duplicate := refreshedByID[comment.ID]; duplicate {
			return stableReplyHistory{}, fmt.Errorf(
				"paged history snapshot for %s repeated refreshed comment ID %s",
				threadID,
				comment.ID,
			)
		}
		refreshedByID[comment.ID] = comment
	}
	refreshed := make([]tailComment, 0, len(observed))
	for _, id := range commentIDs {
		comment, ok := refreshedByID[id]
		if !ok {
			return stableReplyHistory{}, fmt.Errorf(
				"paged history snapshot for %s omitted observed comment ID %s",
				threadID,
				id,
			)
		}
		refreshed = append(refreshed, comment)
	}

	openingNodes := *target.Opening.Nodes
	if len(openingNodes) != 1 {
		return stableReplyHistory{}, fmt.Errorf(
			"paged history snapshot for %s returned %d opening comments",
			threadID,
			len(openingNodes),
		)
	}
	opening, err := decodePagedHistoryComment(threadID, "opening comment", openingNodes[0])
	if err != nil {
		return stableReplyHistory{}, err
	}
	if !samePagedHistoryComment(opening, refreshed[0]) {
		return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s returned an inconsistent opening comment", threadID)
	}

	recentNodes := *target.Recent.Nodes
	recentStart := len(refreshed) - 2
	if recentStart < 0 {
		recentStart = 0
	}
	expectedRecent := refreshed[recentStart:]
	if len(recentNodes) != len(expectedRecent) {
		return stableReplyHistory{}, fmt.Errorf(
			"paged history snapshot for %s returned %d tail comments, want %d",
			threadID,
			len(recentNodes),
			len(expectedRecent),
		)
	}
	for i, node := range recentNodes {
		comment, err := decodePagedHistoryComment(threadID, fmt.Sprintf("tail comment %d", i+1), node)
		if err != nil {
			return stableReplyHistory{}, err
		}
		if !samePagedHistoryComment(comment, expectedRecent[i]) {
			return stableReplyHistory{}, fmt.Errorf("paged history snapshot for %s returned an inconsistent tail", threadID)
		}
	}

	return stableReplyHistory{comments: refreshed}, nil
}

// fetchHistoryTail returns full history and the exact last-two-comment
// boundary. A thread with no comments cannot be reasoned about safely.
func fetchHistoryTail(
	ctx context.Context,
	threadID, bodyFile string,
) (history stableReplyHistory, boundary replyHistoryBoundary, code int) {
	paged := false
	observed, err := fetchAllCommentsObserved(ctx, threadID, &paged)
	if err != nil {
		return stableReplyHistory{}, replyHistoryBoundary{}, fail(
			"provider state is unverified: cannot read complete thread history, nothing posted: %v\n%s",
			err,
			providerReadRecoveryGuidance(err, inspectReplyOutcomeGuidance(threadID, bodyFile)),
		)
	}
	if len(observed) == 0 {
		return stableReplyHistory{}, replyHistoryBoundary{}, fail(
			"thread %s has no comments to read; nothing was posted\n%s",
			threadID,
			inspectReplyOutcomeGuidance(threadID, bodyFile),
		)
	}
	history = stableReplyHistory{comments: observed}
	if paged {
		history, err = revalidatePagedHistory(ctx, threadID, observed)
		if err != nil {
			return stableReplyHistory{}, replyHistoryBoundary{}, fail(
				"provider state is unverified: cannot establish a coherent paged thread history, nothing posted: %v\n%s",
				err,
				providerReadRecoveryGuidance(err, inspectReplyOutcomeGuidance(threadID, bodyFile)),
			)
		}
	}
	boundary = boundaryFromHistory(history.comments)
	if boundary.LastID == "" {
		return stableReplyHistory{}, replyHistoryBoundary{}, fail(
			"thread %s history omitted the last comment ID needed as a predecessor; nothing was posted\n%s",
			threadID,
			inspectReplyOutcomeGuidance(threadID, bodyFile),
		)
	}
	if boundary.PredecessorID == "" && len(history.comments) > 1 {
		return stableReplyHistory{}, replyHistoryBoundary{}, fail(
			"thread %s history omitted the predecessor comment ID needed for exact evidence; nothing was posted\n%s",
			threadID,
			inspectReplyOutcomeGuidance(threadID, bodyFile),
		)
	}
	return history, boundary, -1
}
