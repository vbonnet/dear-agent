// GraphQL queries, mutations, and the thread-fetching/mutating layer
// underneath the commands in main.go: everything that talks to GitHub.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// listQuery pages through review threads 100 at a time. $after is nil on the
// first page (gh omits the unset variable → GraphQL treats it as null).
const listQuery = `query($owner:String!, $repo:String!, $pr:Int!, $after:String) {
  repository(owner:$owner, name:$repo) {
    pullRequest(number:$pr) {
      reviewThreads(first:100, after:$after) {
        totalCount
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          isResolved
          isOutdated
          path
          opening: comments(first:1) { totalCount nodes { author { login } body } }
          recent: comments(last:20) { nodes { id author { login } body updatedAt userContentEdits(last:1) { totalCount nodes { id } } } }
        }
      }
    }
  }
}`

// threadByIDQuery re-reads one thread. Resolution decisions are re-derived
// from this immediately before each mutation, so a reviewer comment landing
// mid-sweep cannot be resolved away on stale state.
const threadByIDQuery = `query($id:ID!) {
  node(id:$id) {
    ... on PullRequestReviewThread {
      id
      isResolved
      isOutdated
      path
      opening: comments(first:1) { totalCount nodes { author { login } body } }
      recent: comments(last:20) { nodes { id author { login } body updatedAt userContentEdits(last:1) { totalCount nodes { id } } } }
    }
  }
}`

// thread is the flattened view of a PullRequestReviewThread that we emit.
type thread struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	// Answered reports whether anyone replied after the thread's opening
	// author had the last word. It is the evidence gate for resolve-all.
	Answered bool `json:"answered"`
	// LastAuthor is the login of the most recent commenter, so a refusal can
	// say who is holding the thread open.
	LastAuthor string `json:"lastAuthor"`
	// LastBody is the most recent comment, used to recognise a reply this
	// tool already posted so a retry does not duplicate it.
	LastBody        string `json:"-"`
	LastBodyPresent bool   `json:"-"`
	// PrevBody is the exact second-to-last comment. Exact reply evidence binds
	// it because a provider-side edit can change what a reply answers without
	// changing the predecessor's node ID.
	PrevBody        string `json:"-"`
	PrevBodyPresent bool   `json:"-"`
	// UpdatedAt is non-null provider evidence that changes across same-ID
	// comment edits, including edit-then-restore cycles that body digests alone
	// cannot distinguish from the originally issued boundary.
	LastUpdatedAt        string `json:"-"`
	PrevUpdatedAt        string `json:"-"`
	LastEditCount        int    `json:"-"`
	PrevEditCount        int    `json:"-"`
	LastEditCountPresent bool   `json:"-"`
	PrevEditCountPresent bool   `json:"-"`
	LastEditID           string `json:"-"`
	PrevEditID           string `json:"-"`
	LastEditIDPresent    bool   `json:"-"`
	PrevEditIDPresent    bool   `json:"-"`
	// LastID and PrevID are the node IDs of the last and second-to-last
	// comments. reply-resolve pins LastID before posting and then requires
	// PrevID to match it, which proves no comment slipped in underneath.
	LastID string `json:"-"`
	PrevID string `json:"-"`
	// Tail is the recent comments, oldest first, so a prior reply can be
	// located even when it is no longer the last comment.
	Tail []tailComment `json:"-"`
}

// outdatedNote flags an outdated thread in refusal output. It is informational
// only: outdated means the diff hunk moved, not that the point was addressed.
func outdatedNote(t thread) string {
	if t.IsOutdated {
		return " [outdated: hunk moved, which is NOT evidence it was addressed]"
	}
	return ""
}

// filterThreads keeps only unresolved threads, optionally restricted to a
// single comment author (e.g. "gemini-code-assist"). An empty author matches
// every author — but never a resolved thread.
func filterThreads(ts []thread, author string) []thread {
	out := make([]thread, 0, len(ts))
	for _, t := range ts {
		if t.IsResolved {
			continue
		}
		if author != "" && t.Author != author {
			continue
		}
		out = append(out, t)
	}
	return out
}

// listThreads returns every review thread on a PR, following cursor pagination
// so PRs with more than 100 threads are handled correctly.
func listThreads(ctx context.Context, owner, repo string, pr int) ([]thread, error) {
	var all []thread
	cursor := ""
	for {
		variables := map[string]any{
			"owner": owner,
			"repo":  repo,
			"pr":    pr,
		}
		if cursor != "" {
			variables["after"] = cursor
		}
		raw, err := ghGraphQL(ctx, listQuery, variables)
		if err != nil {
			return nil, err
		}

		var resp threadsResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("parse reviewThreads response: %w", err)
		}

		rt := resp.Data.Repository.PullRequest.ReviewThreads
		for _, n := range rt.Nodes {
			if n.ID == "" || n.IsResolved == nil {
				return nil, fmt.Errorf(
					"reviewThreads response omitted required thread identity or resolved state",
				)
			}
			all = append(all, toThread(n))
		}
		if !rt.PageInfo.HasNextPage {
			break
		}
		cursor = rt.PageInfo.EndCursor
	}
	return all, nil
}

// threadsResponse mirrors the reviewThreads GraphQL payload. It is a named
// type so the flattening in toThread can be unit-tested without a network.
type threadsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []threadNode `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type userContentEditNodeEvidence struct {
	ID string `json:"id"`
}

type userContentEditsEvidence struct {
	TotalCount *int                           `json:"totalCount"`
	Nodes      *[]userContentEditNodeEvidence `json:"nodes"`
}

// observedEditRevision validates the two-field provider edit revision as one
// indivisible value. A zero-count comment has no last edit node; every
// positive count must carry exactly the requested last edit node ID.
func observedEditRevision(evidence *userContentEditsEvidence) (count int, lastID string, present bool) {
	if evidence == nil || evidence.TotalCount == nil || evidence.Nodes == nil || *evidence.TotalCount < 0 {
		return 0, "", false
	}
	count = *evidence.TotalCount
	nodes := *evidence.Nodes
	if count == 0 {
		return 0, "", len(nodes) == 0
	}
	if len(nodes) != 1 || !validContinuationReceiptID(nodes[0].ID) {
		return 0, "", false
	}
	return count, nodes[0].ID, true
}

type threadNode struct {
	ID         string `json:"id"`
	IsResolved *bool  `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	// Opening is the first comment (the reviewer's point) plus the thread's
	// total comment count. Recent is the single most recent comment. Asking
	// GitHub for each end directly keeps the answered check exact on threads
	// of any length, instead of paging to find the end.
	Opening struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
		} `json:"nodes"`
	} `json:"opening"`
	// Recent holds the tail of the thread, newest last. More than two because
	// reply-resolve must be able to find a reply a previous run already posted
	// even when the reviewer has spoken since.
	Recent struct {
		Nodes []struct {
			ID               string                    `json:"id"`
			UpdatedAt        string                    `json:"updatedAt"`
			UserContentEdits *userContentEditsEvidence `json:"userContentEdits"`
			Author           struct {
				Login string `json:"login"`
			} `json:"author"`
			Body *string `json:"body"`
		} `json:"nodes"`
	} `json:"recent"`
}

// toThread flattens one GraphQL node, deriving the Answered evidence flag.
//
// A thread is answered when it has more than one comment and the last one
// comes from someone other than the author who opened it. A reviewer who
// comments again after our reply flips it back to unanswered, which is the
// reading we want: the ball is in our court again.
func toThread(n threadNode) thread {
	isResolved := false
	if n.IsResolved != nil {
		isResolved = *n.IsResolved
	}
	t := thread{ID: n.ID, IsResolved: isResolved, IsOutdated: n.IsOutdated, Path: n.Path, Author: "unknown"}
	if len(n.Opening.Nodes) == 0 {
		return t
	}
	if login := n.Opening.Nodes[0].Author.Login; login != "" {
		t.Author = login
	}
	t.Body = cleanBody(n.Opening.Nodes[0].Body)
	openLogin := n.Opening.Nodes[0].Author.Login
	lastLogin := ""
	if k := len(n.Recent.Nodes); k > 0 {
		// Newest is last in the connection, so index from the end.
		last := n.Recent.Nodes[k-1]
		lastLogin = last.Author.Login
		if last.Body != nil {
			t.LastBody = *last.Body
			t.LastBodyPresent = true
		}
		t.LastID = last.ID
		t.LastUpdatedAt = last.UpdatedAt
		if editCount, lastEditID, present := observedEditRevision(last.UserContentEdits); present {
			t.LastEditCount = editCount
			t.LastEditCountPresent = true
			t.LastEditID = lastEditID
			t.LastEditIDPresent = true
		}
		if k > 1 {
			previous := n.Recent.Nodes[k-2]
			t.PrevID = previous.ID
			t.PrevUpdatedAt = previous.UpdatedAt
			if editCount, lastEditID, present := observedEditRevision(previous.UserContentEdits); present {
				t.PrevEditCount = editCount
				t.PrevEditCountPresent = true
				t.PrevEditID = lastEditID
				t.PrevEditIDPresent = true
			}
			if previous.Body != nil {
				t.PrevBody = *previous.Body
				t.PrevBodyPresent = true
			}
		}
		t.Tail = make([]tailComment, 0, k)
		for _, c := range n.Recent.Nodes {
			body := ""
			if c.Body != nil {
				body = *c.Body
			}
			comment := tailComment{
				ID: c.ID, Login: c.Author.Login, Body: body, UpdatedAt: c.UpdatedAt,
			}
			if editCount, lastEditID, present := observedEditRevision(c.UserContentEdits); present {
				comment.EditCount = editCount
				comment.EditCountPresent = true
				comment.LastEditID = lastEditID
				comment.LastEditIDPresent = true
			}
			t.Tail = append(t.Tail, comment)
		}
	}
	t.LastAuthor = "unknown"
	if lastLogin != "" {
		t.LastAuthor = lastLogin
	}
	// Both logins must be observed. A deleted or hidden account leaves an
	// empty login, and comparing it against the "unknown" placeholder would
	// manufacture a second participant that was never seen. Answered is a
	// licence to close someone's finding, so absence of evidence is treated
	// as evidence of absence in the safe direction only.
	t.Answered = n.Opening.TotalCount > 1 && openLogin != "" && lastLogin != "" && lastLogin != openLogin
	return t
}

// fetchThread re-reads a single thread by node ID.
func fetchThread(ctx context.Context, threadID string) (thread, error) {
	raw, err := ghGraphQL(ctx, threadByIDQuery, map[string]any{"id": threadID})
	if err != nil {
		return thread{}, err
	}
	var resp struct {
		Data struct {
			Node threadNode `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return thread{}, fmt.Errorf("parse thread %s: %w", threadID, err)
	}
	if resp.Data.Node.ID == "" {
		return thread{}, fmt.Errorf("no review thread with ID %s", threadID)
	}
	if resp.Data.Node.ID != threadID {
		return thread{}, fmt.Errorf(
			"thread query for %s returned mismatched ID %s",
			threadID,
			resp.Data.Node.ID,
		)
	}
	if resp.Data.Node.IsResolved == nil {
		return thread{}, fmt.Errorf("thread %s response omitted its resolved state", threadID)
	}
	return toThread(resp.Data.Node), nil
}

// readInitialReplyState establishes the exact target and initial resolved
// state without acting on it. Even an already-resolved thread needs a stable
// history classification before reply-resolve can distinguish a true skip
// from a changed source that must not be claimed as the resolved answer.
func readInitialReplyState(ctx context.Context, threadID, errMsg, bodyFile string) (t thread, code int) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return thread{}, fail("%s: %v\n%s",
			errMsg, err, providerReadRecoveryGuidance(err, inspectReplyOutcomeGuidance(threadID, bodyFile)))
	}
	if cur.LastID == "" {
		return cur, fail("provider state is unverified: thread %s response omitted its last comment ID; nothing was posted\n%s",
			threadID, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if !cur.LastBodyPresent {
		return cur, fail("provider state is unverified: thread %s response omitted its last comment body; nothing was posted\n%s",
			threadID, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	return cur, -1
}

// readMatchingTail re-reads a thread after its full history has been paged
// and refuses to proceed if the tail pair's IDs or exact bodies moved since.
// code is -1 when the boundary still matches and t is safe to act on.
func readMatchingTail(
	ctx context.Context,
	threadID, bodyFile string,
	boundary replyHistoryBoundary,
	priorReply priorReplyState,
	initiallyResolved bool,
) (t thread, code int) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return thread{}, fail("provider state is unverified: cannot re-read thread state, nothing posted: %v\n%s",
			err, providerReadRecoveryGuidance(err, inspectReplyOutcomeGuidance(threadID, bodyFile)))
	}
	if boundary.LastID == "" || !validExactBodySHA256(boundary.LastBodySHA256) {
		return cur, fail("the history snapshot omitted the last comment ID needed for tail comparison; nothing was posted\n%s",
			inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if !initiallyResolved && cur.IsResolved {
		if classified, classifiedCode, handled := rejectConcurrentlyResolvedHistory(
			ctx,
			threadID,
			bodyFile,
			cur,
			priorReply,
		); handled {
			return classified, classifiedCode
		}
	}
	if cur.LastID == "" {
		return cur, fail("provider state is unverified: thread %s response omitted the current tail ID needed for comparison; nothing was posted\n%s",
			threadID, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if priorReply == priorReplySuperseded && cur.LastID == boundary.LastID {
		// Full history already proved that this named tail follows the earlier
		// exact reply. Missing body or author fields cannot erase that positional
		// supersession or hide a concurrent stale resolution behind an
		// "unverified" return; this path never licenses posting.
		if initiallyResolved {
			return classifyInitiallyResolvedStableTail(threadID, bodyFile, cur, priorReply)
		}
		return classifyConcurrentlyResolvedStableTail(ctx, threadID, bodyFile, cur, priorReply)
	}
	if detail := boundary.mismatch(cur); detail != "" {
		return rejectChangedReplyHistoryBoundary(
			ctx,
			threadID,
			bodyFile,
			cur,
			detail,
			initiallyResolved,
		)
	}
	if priorReply != priorReplySuperseded {
		if detail := boundary.authorMismatch(cur); detail != "" {
			return cur, fail("provider author evidence is unverified: %s; nothing was posted or resolved.\n%s",
				detail,
				unavailableAuthorEvidenceGuidance(threadID, bodyFile),
			)
		}
	}
	if initiallyResolved {
		return classifyInitiallyResolvedStableTail(threadID, bodyFile, cur, priorReply)
	}
	return classifyConcurrentlyResolvedStableTail(ctx, threadID, bodyFile, cur, priorReply)
}

func rejectConcurrentlyResolvedHistory(
	ctx context.Context,
	threadID, bodyFile string,
	cur thread,
	priorReply priorReplyState,
) (t thread, code int, handled bool) {
	switch priorReply {
	case priorReplySuperseded:
		t, code = rejectConcurrentlyResolvedSupersededReply(ctx, threadID, bodyFile, cur)
		return t, code, true
	case reviewerHandbackIsLast:
		t, code = rejectConcurrentlyResolvedReviewerHandback(ctx, threadID, bodyFile, cur)
		return t, code, true
	case noPriorReply, priorReplyIsLast, differentAnswerIsLast, unavailableReplyIntent:
		return cur, -1, false
	}
	return cur, -1, false
}

// unansweredError marks a thread that was refused on evidence, as opposed to
// a GraphQL or permission failure. The sweep continues past the former and
// stops on the latter, so a globally denied operation is not retried once per
// thread and then misreported as "nobody replied".
type unansweredError struct{ msg string }

func (e *unansweredError) Error() string { return e.msg }

// supersededEvidenceError means a nonempty, newer comment ID proved that the
// evidence used for a reply or resolution is no longer current. Recovery must
// answer that comment; it must not replay an old body unchanged.
type supersededEvidenceError struct {
	msg   string
	cause error
}

func (e *supersededEvidenceError) Error() string { return e.msg }

func (e *supersededEvidenceError) Unwrap() error { return e.cause }

// invalidatedReplyEvidenceError means the named reply ID remains observable,
// but its predecessor or exact provider-visible body no longer matches the
// evidence authorized by reply-resolve or its continuation receipt.
type invalidatedReplyEvidenceError struct {
	msg   string
	cause error
}

func (e *invalidatedReplyEvidenceError) Error() string { return e.msg }

func (e *invalidatedReplyEvidenceError) Unwrap() error { return e.cause }

// unavailableAnswerEvidenceError means the named reply is still last, but the
// author data cannot prove that a distinct participant answered the opener.
// Posting another body cannot repair missing/equal author identity.
type unavailableAnswerEvidenceError struct {
	msg   string
	cause error
}

func (e *unavailableAnswerEvidenceError) Error() string { return e.msg }

func (e *unavailableAnswerEvidenceError) Unwrap() error { return e.cause }

// unverifiableResolutionError means GitHub applied a resolution but returned
// no last-comment anchor, and the automatic reopen succeeded. No newer comment
// was proved, so the original operation remains the only valid retry.
type unverifiableResolutionError struct {
	msg   string
	cause error
}

func (e *unverifiableResolutionError) Error() string { return e.msg }

func (e *unverifiableResolutionError) Unwrap() error { return e.cause }

// unverifiedProviderStateError means a follow-up read failed after an
// ambiguous provider outcome. Callers must stop for inspection: neither retry,
// cleanup, nor merge is licensed by an unknown state.
type unverifiedProviderStateError struct {
	msg   string
	cause error
}

func (e *unverifiedProviderStateError) Error() string { return e.msg }

func (e *unverifiedProviderStateError) Unwrap() error { return e.cause }

// resolutionEvidence is the named boundary between callers and resolution.
// A zero value requests the ordinary answered-thread gate. LastID alone is an
// ID anchor used by existing resolution checks. All three fields bind an exact
// reply to its predecessor and provider-visible bytes.
type resolutionEvidence struct {
	LastID                       string
	PredecessorID                string
	PredecessorBodySHA256        string
	BodySHA256                   string
	PredecessorUpdatedAt         string
	ReplyUpdatedAt               string
	PredecessorEditCount         int
	ReplyEditCount               int
	PredecessorEditCountPresent  bool
	ReplyEditCountPresent        bool
	PredecessorLastEditID        string
	ReplyLastEditID              string
	PredecessorLastEditIDPresent bool
	ReplyLastEditIDPresent       bool
	OpeningAuthor                string
	ReplyAuthor                  string
}

func (e resolutionEvidence) exactReply() bool {
	return e.LastID != "" && e.PredecessorID != "" &&
		e.PredecessorBodySHA256 != "" && e.BodySHA256 != ""
}

func (e resolutionEvidence) issuedReply() bool {
	return e.exactReply() && e.PredecessorUpdatedAt != "" && e.ReplyUpdatedAt != "" &&
		e.PredecessorEditCountPresent && e.ReplyEditCountPresent &&
		e.PredecessorLastEditIDPresent && e.ReplyLastEditIDPresent &&
		e.OpeningAuthor != "" && e.ReplyAuthor != ""
}

func (e resolutionEvidence) validate() error {
	if e.LastID == "" {
		if e.hasExactReplyField() || e.hasIssuedReplyField() {
			return errors.New("resolution evidence names a predecessor or body digest without a reply ID")
		}
		return nil
	}
	if !e.hasExactReplyField() {
		return nil
	}
	if err := e.validateExactReplyFields(); err != nil {
		return err
	}
	return e.validateIssuedReplyFields()
}

func (e resolutionEvidence) hasExactReplyField() bool {
	return e.PredecessorID != "" || e.PredecessorBodySHA256 != "" || e.BodySHA256 != ""
}

func (e resolutionEvidence) hasIssuedReplyField() bool {
	return e.hasIssuedTimestampField() || e.hasIssuedEditCountField() ||
		e.hasIssuedEditIDField() || e.hasIssuedAuthorField()
}

func (e resolutionEvidence) hasIssuedTimestampField() bool {
	return e.PredecessorUpdatedAt != "" || e.ReplyUpdatedAt != ""
}

func (e resolutionEvidence) hasIssuedEditCountField() bool {
	return e.PredecessorEditCount != 0 || e.ReplyEditCount != 0 ||
		e.PredecessorEditCountPresent || e.ReplyEditCountPresent
}

func (e resolutionEvidence) hasIssuedEditIDField() bool {
	return e.PredecessorLastEditID != "" || e.ReplyLastEditID != "" ||
		e.PredecessorLastEditIDPresent || e.ReplyLastEditIDPresent
}

func (e resolutionEvidence) hasIssuedAuthorField() bool {
	return e.OpeningAuthor != "" || e.ReplyAuthor != ""
}

func (e resolutionEvidence) validateExactReplyFields() error {
	if e.PredecessorID == "" || e.PredecessorBodySHA256 == "" || e.BodySHA256 == "" {
		return errors.New("exact reply evidence requires predecessor, reply, and both body digests together")
	}
	if e.PredecessorID == e.LastID {
		return errors.New("exact reply evidence cannot use the reply as its own predecessor")
	}
	if !validExactBodySHA256(e.PredecessorBodySHA256) || !validExactBodySHA256(e.BodySHA256) {
		return errors.New("exact reply evidence has an invalid predecessor or reply body digest")
	}
	return nil
}

func (e resolutionEvidence) validateIssuedReplyFields() error {
	if !e.hasIssuedReplyField() {
		return nil
	}
	if !e.issuedReply() {
		return errors.New("issued reply evidence requires both update timestamps and both authors together")
	}
	if !validContinuationTimestamp(e.PredecessorUpdatedAt) || !validContinuationTimestamp(e.ReplyUpdatedAt) {
		return errors.New("issued reply evidence has an invalid update timestamp")
	}
	if !validContinuationEditRevision(e.PredecessorEditCount, e.PredecessorLastEditID) ||
		!validContinuationEditRevision(e.ReplyEditCount, e.ReplyLastEditID) {
		return errors.New("issued reply evidence has an invalid edit revision")
	}
	if !validContinuationAuthor(e.OpeningAuthor) || !validContinuationAuthor(e.ReplyAuthor) ||
		e.OpeningAuthor == e.ReplyAuthor {
		return errors.New("issued reply evidence requires safe distinct opening and reply authors")
	}
	return nil
}

type evidenceRecoveryKind uint8

const (
	answerChangedEvidence evidenceRecoveryKind = iota
	retryUnchangedEvidence
)

// resolveWithEvidence re-reads the thread and resolves it only if it is still
// answered, so a reviewer comment arriving between the decision and the
// mutation cannot be resolved away. force skips the evidence check, never the
// re-read: an already-resolved thread is still reported as a no-op.
// evidence, when non-zero, names the comment that must still be last. Exact
// reply evidence additionally binds its predecessor and body digest at both
// the pre-mutation read and the mutation response.
func resolveWithEvidence(
	ctx context.Context,
	threadID string,
	force bool,
	evidence resolutionEvidence,
) (msg string, mutated bool, err error) {
	if err := evidence.validate(); err != nil {
		return "", false, &unverifiedProviderStateError{msg: fmt.Sprintf(
			"invalid caller-supplied resolution evidence: %v; no mutation was attempted",
			err,
		)}
	}
	cur, err := readResolutionCandidate(ctx, threadID, force, evidence)
	if err != nil {
		return "", false, err
	}
	if cur.IsResolved {
		return fmt.Sprintf("skipped %s (already resolved)", threadID), false, nil
	}

	effectiveEvidence := evidence
	if effectiveEvidence.LastID == "" && !force {
		effectiveEvidence.LastID = cur.LastID
	}
	msg, mutationState, mutationErr := mutateThread(ctx, "resolve", threadID)
	if mutationErr != nil {
		msg, mutationState, err = reconcileResolveMutationError(
			ctx,
			threadID,
			effectiveEvidence,
			mutationErr,
		)
		if err != nil {
			return msg, false, err
		}
	}
	if err := validateResolveMutationEvidence(
		ctx,
		threadID,
		cur.Path,
		effectiveEvidence,
		mutationState,
		mutationErr,
	); err != nil {
		return "", false, err
	}
	if !force {
		if err := validateResolveMutationAuthorBoundary(
			ctx,
			threadID,
			cur,
			mutationState,
			mutationErr,
		); err != nil {
			return "", false, err
		}
	}
	if mutationErr != nil && isAccessDenied(mutationErr) {
		return msg, false, mutationErr
	}
	if mutationErr != nil {
		return independentlyConfirmedResolution(msg)
	}
	return msg, true, nil
}

// independentlyConfirmedResolution reports the reconciled provider state as a
// no-op. A transport failure means this caller cannot claim or count the
// transition even when a fresh read proves the requested postcondition.
func independentlyConfirmedResolution(msg string) (string, bool, error) {
	return msg, false, nil
}

func invalidatedReplyAnchorError(ctx context.Context, threadID string, cur thread, detail string) error {
	if cur.IsResolved {
		if rErr := reopenOrFail(ctx, threadID, cur.Path, "resolved on invalidated exact reply evidence", answerChangedEvidence); rErr != nil {
			return rErr
		}
	}
	return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
		"%s %s: %s",
		threadID,
		cur.Path,
		detail,
	)}
}

// reopenOrFail attempts to reopen a thread whose resolution is being
// corrected, and reports a failedReopenError if that doesn't work.
// It checks the mutation's own IsResolved postcondition, not just a nil
// error: another resolver can race the unresolve and re-resolve it before
// this response lands, or the mutation can fail without a transport error
// surfacing at all. An access-denial failure gets a distinct hint, since
// retrying the same unresolve command with the same credentials just
// repeats the denial rather than fixing anything.
// staleAnchorError builds the refusal for an anchor that is no longer last,
// reopening the thread first if it was resolved on that stale evidence. A
// reviewer follow-up and another resolver can both act before the read that
// found the mismatch, leaving the thread simultaneously resolved and
// anchor-stale; reporting the refusal without reopening would leave that
// follow-up resolved away unread, and a later retry would just see
// IsResolved at readOrExit and silently no-op instead of reopening it.
func staleAnchorError(ctx context.Context, threadID string, cur thread) error {
	if cur.IsResolved {
		if rErr := reopenOrFail(ctx, threadID, cur.Path, "resolved on stale evidence", answerChangedEvidence); rErr != nil {
			return rErr
		}
	}
	return &supersededEvidenceError{msg: fmt.Sprintf(
		"%s %s: the comment this resolution was based on is no longer last "+
			"(someone commented after it)", threadID, cur.Path)}
}

func reopenOrFail(
	ctx context.Context,
	threadID, path, reason string,
	recovery evidenceRecoveryKind,
) error {
	_, uErr := unresolveWithEvidence(ctx, threadID)
	if uErr == nil {
		return nil
	}
	if matchesErrorType[*accessDeniedMutationWithConfirmedStateError](uErr) {
		// The requested safe state is independently confirmed, but the denied
		// caller must carry that access warning into the next recovery decision.
		return uErr
	}
	if matchesErrorType[*unverifiedProviderStateError](uErr) {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"%s, and the automatic reopen outcome could not be verified: %v",
			reason,
			uErr,
		), cause: uErr}
	}
	detail := fmt.Sprintf("%s AND a fresh provider read confirmed that the thread is still resolved", reason)
	access := ""
	if isAccessDenied(uErr) {
		access = " This is an access problem: check `gh auth status` and " +
			"fix credentials before retrying unresolve — re-running it " +
			"unchanged will be denied again."
	}
	return &failedReopenError{
		msg: fmt.Sprintf(
			"%s %s: %s — the thread is STILL RESOLVED.%s",
			threadID, path, detail, access,
		),
		recovery: recovery,
		cause:    uErr,
	}
}

// failedReopenError marks a resolution that went through on stale or
// unverifiable evidence AND whose automatic reopen also failed: the thread is
// left resolved on GitHub despite that. It is distinguished from
// unansweredError because a caller's generic "safe to retry" advice for that
// type is actively wrong here — reply-resolve's own readOrExit would see
// IsResolved, print "skipped ... (already resolved)", and exit 0 without ever
// reading the intervening comment or reopening the thread.
type failedReopenError struct {
	msg      string
	recovery evidenceRecoveryKind
	cause    error
}

func (e *failedReopenError) Error() string { return e.msg }

func (e *failedReopenError) Unwrap() error { return e.cause }

// unresolveStillResolvedError records a failed or structurally invalid
// unresolve response whose exact-target follow-up read confirmed that the
// requested thread remains resolved. Keeping the original cause in the text
// preserves access-denial classification without mistaking the mutation's
// client-side signal for proof of provider state.
type unresolveStillResolvedError struct {
	threadID string
	cause    error
}

func (e *unresolveStillResolvedError) Error() string {
	return fmt.Sprintf(
		"unresolve %s was not confirmed by its mutation response (%v), and a fresh provider read confirms the thread is STILL RESOLVED",
		e.threadID,
		e.cause,
	)
}

func (e *unresolveStillResolvedError) Unwrap() error { return e.cause }

// unresolveWithEvidence accepts the mutation response only when mutateThread
// validated both target identity and state. Any other outcome is reconciled
// against a fresh exact-target read because the mutation may have applied
// before a transport error or malformed response reached the client.
func unresolveWithEvidence(ctx context.Context, threadID string) (string, error) {
	msg, _, mutationErr := mutateThread(ctx, "unresolve", threadID)
	if mutationErr == nil {
		return msg, nil
	}
	after, readErr := fetchThread(ctx, threadID)
	if readErr != nil {
		return "", &unverifiedProviderStateError{msg: fmt.Sprintf(
			"unresolve mutation reported %v, and the requested thread could not be re-read to determine whether it applied: %v",
			mutationErr,
			readErr,
		), cause: errors.Join(mutationErr, readErr)}
	}
	if !after.IsResolved {
		msg := fmt.Sprintf(
			"unresolved %s (isResolved=false) [recovered: the mutation response was not proof, but a fresh provider read confirmed the requested state]",
			threadID,
		)
		if isAccessDenied(mutationErr) {
			return msg, &accessDeniedMutationWithConfirmedStateError{
				operation: "unresolve",
				threadID:  threadID,
				cause:     mutationErr,
			}
		}
		return msg, nil
	}
	return "", &unresolveStillResolvedError{threadID: threadID, cause: mutationErr}
}

// accessDeniedMutationWithConfirmedStateError records the narrow case where
// GitHub denied this caller's mutation but a subsequent exact-target read found
// the requested state already present. The state is safe, yet it must not be
// attributed to the denied caller or used to invite another mutation with the
// same credentials.
type accessDeniedMutationWithConfirmedStateError struct {
	operation string
	threadID  string
	cause     error
}

func (e *accessDeniedMutationWithConfirmedStateError) Error() string {
	return fmt.Sprintf(
		"GitHub denied this caller's %s mutation for %s; a fresh read independently confirmed the requested state, likely after another actor changed it",
		e.operation,
		e.threadID,
	)
}

func (e *accessDeniedMutationWithConfirmedStateError) Unwrap() error { return e.cause }

// isAccessDenied reports whether an error is GitHub refusing the caller rather
// than a transient failure. Retrying a denial just repeats it, so the two need
// different advice.
func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	return matchesErrorType[*providerAccessDeniedError](err)
}

// matchesErrorType consumes the typed match even when callers only need the
// boolean. The repository's pinned errcheck version treats a blank first
// errors.AsType result as an unchecked error.
func matchesErrorType[T error](err error) bool {
	matched, ok := errors.AsType[T](err)
	_ = matched
	return ok
}
