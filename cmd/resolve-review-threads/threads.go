// GraphQL queries, mutations, and the thread-fetching/mutating layer
// underneath the commands in main.go: everything that talks to GitHub.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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
          recent: comments(last:20) { nodes { id author { login } body } }
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
      recent: comments(last:20) { nodes { id author { login } body } }
    }
  }
}`

// threadCommentsQuery pages forward through a thread's entire comment list.
// Deciding that no prior reply exists has to be a fact, not the result of a
// bounded look: a reply pushed out of the tail by later discussion would be
// reposted, and the repost anchors to the newest follow-up, so the placement
// check passes and everything in between gets resolved unread.
const threadCommentsQuery = `query($id:ID!, $after:String) {
  node(id:$id) {
    ... on PullRequestReviewThread {
      comments(first:100, after:$after) {
        pageInfo { hasNextPage endCursor }
        nodes { id author { login } body }
      }
    }
  }
}`

// resolveMutation asks for the thread's last comment in the same response as
// the mutation, not via a separate read afterward. A comment landing between
// the pre-mutation evidence check and this mutation actually applying is a
// real window a subsequent read cannot fully close (it has its own latency);
// having GitHub report the post-mutation last comment lets the caller detect
// that race directly off the mutation it just made.
const resolveMutation = `mutation($threadId:ID!) {
  resolveReviewThread(input:{threadId:$threadId}) {
    thread {
      id
      isResolved
      comments(last:1) { nodes { id } }
    }
  }
}`

const unresolveMutation = `mutation($threadId:ID!) {
  unresolveReviewThread(input:{threadId:$threadId}) {
    thread { id isResolved }
  }
}`

// replyMutation posts a reply onto an existing review thread. Note the input
// field here is pullRequestReviewThreadId, NOT the threadId that
// resolveReviewThread takes — the two mutations disagree, which is why the
// reply and the resolve are wrapped together rather than left to callers.
const replyMutation = `mutation($threadId:ID!, $body:String!) {
  addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$threadId, body:$body}) {
    comment { id }
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
	LastBody string `json:"-"`
	// LastID and PrevID are the node IDs of the last and second-to-last
	// comments. reply-resolve pins LastID before posting and then requires
	// PrevID to match it, which proves no comment slipped in underneath.
	LastID string `json:"-"`
	PrevID string `json:"-"`
	// Tail is the recent comments, oldest first, so a prior reply can be
	// located even when it is no longer the last comment.
	Tail []tailComment `json:"-"`
}

// tailComment is one comment from the tail of a thread.
type tailComment struct {
	ID    string
	Login string
	Body  string
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
			ID     string `json:"id"`
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
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
		t.LastBody = last.Body
		t.LastID = last.ID
		if k > 1 {
			t.PrevID = n.Recent.Nodes[k-2].ID
		}
		t.Tail = make([]tailComment, 0, k)
		for _, c := range n.Recent.Nodes {
			t.Tail = append(t.Tail, tailComment{ID: c.ID, Login: c.Author.Login, Body: c.Body})
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

// readOrExit re-reads a thread and reports whether the caller should return
// immediately. code is -1 when t is fresh and safe to act on; otherwise the
// caller should `return code` as-is. A read failure fails with errMsg framing
// context the generic error lacks; an already-resolved thread means someone
// else closed it since the caller last looked, so the skip is reported here
// once rather than at every call site.
func readOrExit(ctx context.Context, threadID, errMsg, bodyFile string) (t thread, code int) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return thread{}, fail("%s: %v\n%s",
			errMsg, err, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if cur.LastID == "" {
		return cur, fail("provider state is unverified: thread %s response omitted its last comment ID; nothing was posted\n%s",
			threadID, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if cur.IsResolved {
		fmt.Printf("skipped %s (already resolved)\n", threadID)
		return cur, 0
	}
	return cur, -1
}

// readMatchingTail re-reads a thread after its full history has been paged
// and refuses to proceed if the tail has moved since. fetchAllComments can
// finish before a new comment lands; classifyPriorReply never saw that
// comment, so treating it as accounted for (by silently adopting whatever is
// now last as the anchor) would resolve a thread with unread commentary on
// it. wantLastID is the last comment classifyPriorReply actually reasoned
// about. code is -1 when the tail still matches and t is safe to act on.
func readMatchingTail(
	ctx context.Context,
	threadID, wantLastID, bodyFile string,
	priorReply priorReplyState,
) (t thread, code int) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return thread{}, fail("provider state is unverified: cannot re-read thread state, nothing posted: %v\n%s",
			err, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if wantLastID == "" {
		return cur, fail("the history snapshot omitted the last comment ID needed for tail comparison; nothing was posted\n%s",
			inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if cur.LastID == "" {
		return cur, fail("provider state is unverified: thread %s response omitted the current tail ID needed for comparison; nothing was posted\n%s",
			threadID, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if cur.LastID != wantLastID {
		state := "the thread remains unresolved"
		if cur.IsResolved {
			reason := fmt.Sprintf(
				"resolved after the history tail changed from %s to %s before reply classification",
				wantLastID,
				cur.LastID,
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
				"%s %s: the history tail changed from %s to %s while it was being read; nothing was posted; %s",
				threadID,
				cur.Path,
				wantLastID,
				cur.LastID,
				state,
			)},
			revisedAnswerRecoveryGuidance(threadID, bodyFile, false),
		)
		return cur, fail("%s", message)
	}
	if cur.IsResolved {
		if priorReply == priorReplySuperseded {
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
		fmt.Printf("skipped %s (already resolved)\n", threadID)
		return cur, 0
	}
	return cur, -1
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
type supersededEvidenceError struct{ msg string }

func (e *supersededEvidenceError) Error() string { return e.msg }

// unavailableAnswerEvidenceError means the named reply is still last, but the
// author data cannot prove that a distinct participant answered the opener.
// Posting another body cannot repair missing/equal author identity.
type unavailableAnswerEvidenceError struct{ msg string }

func (e *unavailableAnswerEvidenceError) Error() string { return e.msg }

// unverifiableResolutionError means GitHub applied a resolution but returned
// no last-comment anchor, and the automatic reopen succeeded. No newer comment
// was proved, so the original operation remains the only valid retry.
type unverifiableResolutionError struct{ msg string }

func (e *unverifiableResolutionError) Error() string { return e.msg }

// unverifiedProviderStateError means a follow-up read failed after an
// ambiguous provider outcome. Callers must stop for inspection: neither retry,
// cleanup, nor merge is licensed by an unknown state.
type unverifiedProviderStateError struct{ msg string }

func (e *unverifiedProviderStateError) Error() string { return e.msg }

type evidenceRecoveryKind uint8

const (
	answerChangedEvidence evidenceRecoveryKind = iota
	retryUnchangedEvidence
)

// resolveWithEvidence re-reads the thread and resolves it only if it is still
// answered, so a reviewer comment arriving between the decision and the
// mutation cannot be resolved away. force skips the evidence check, never the
// re-read: an already-resolved thread is still reported as a no-op.
// wantLastID, when non-empty, names the comment that must still be the
// thread's last one. reply-resolve passes the reply it posted so that SPEC-31
// is enforced at the pre-mutation read rather than only at an earlier check:
// between the two, another process on the same login could comment, and the
// answered test alone cannot tell that apart from our own reply.
func resolveWithEvidence(ctx context.Context, threadID string, force bool, wantLastID string) (msg string, mutated bool, err error) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return "", false, &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread state could not be read before resolution; no mutation was attempted: %v",
			err,
		)}
	}
	if wantLastID != "" && cur.LastID != wantLastID {
		if cur.LastID == "" {
			return "", false, &unverifiedProviderStateError{msg: fmt.Sprintf(
				"thread %s response omitted the current tail ID needed to verify the named resolution anchor",
				threadID,
			)}
		}
		return "", false, staleAnchorError(ctx, threadID, cur)
	}
	if wantLastID != "" && !force && !cur.Answered {
		return "", false, &unavailableAnswerEvidenceError{msg: fmt.Sprintf(
			"%s %s: the named reply is still last, but missing or equal author identity cannot prove an independent answer%s",
			threadID, cur.Path, outdatedNote(cur),
		)}
	}
	if cur.IsResolved {
		// Someone else closed it between the listing and now. Report it, but
		// do not claim it as this sweep's work: the count is an audit record.
		return fmt.Sprintf("skipped %s (already resolved)", threadID), false, nil
	}
	if !force && cur.LastID == "" {
		return "", false, &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread %s response omitted the current tail ID needed as resolution evidence",
			threadID,
		)}
	}
	if !cur.Answered && !force {
		return "", false, &unansweredError{msg: fmt.Sprintf(
			"%s %s: unanswered (last word: @%s)%s",
			threadID, cur.Path, cur.LastAuthor, outdatedNote(cur),
		)}
	}
	// effectiveWantID is what the post-mutation check below verifies against.
	// reply-resolve already names its own anchor via wantLastID. resolve-all
	// and bare resolve pass "" — force means "resolve regardless of
	// evidence", so a late follow-up shouldn't block it either, but a
	// NON-forced call just proved cur.LastID answered the thread, and that
	// same comment is the evidence a mid-flight follow-up would invalidate.
	// Without this, those two paths passed no anchor at all, so the
	// reconciliation below was silently skipped for everything except
	// reply-resolve.
	effectiveWantID := wantLastID
	if effectiveWantID == "" && !force {
		effectiveWantID = cur.LastID
	}
	msg, gotLastID, _, err := mutateThread(ctx, "resolve", threadID)
	if err != nil {
		// gh api can fail on the client side (network drop, timeout) after
		// GitHub already applied the mutation server-side: this error alone
		// does not prove the resolve never happened. Re-read before trusting
		// it as a clean no-op, so a resolution that actually went through
		// still gets the same reconciliation below rather than silently
		// skipping it because the client-side signal was ambiguous.
		after, checkErr := fetchThread(ctx, threadID)
		if checkErr != nil {
			return msg, false, &unverifiedProviderStateError{msg: fmt.Sprintf(
				"resolve mutation reported %v, and the thread could not be re-read to determine whether it applied: %v",
				err, checkErr,
			)}
		}
		if !after.IsResolved {
			// A fresh unresolved state proves the mutation did not leave the
			// thread resolved, but the tail still decides which recovery is
			// safe. A nonempty changed ID is confirmed new evidence and must
			// not receive unchanged-retry guidance.
			if effectiveWantID != "" && after.LastID == "" {
				return "", false, &unverifiedProviderStateError{msg: fmt.Sprintf(
					"resolve mutation reported %v, and the fresh unresolved response for %s omitted the tail ID needed to choose unchanged retry or revision",
					err,
					threadID,
				)}
			}
			if effectiveWantID != "" && after.LastID != effectiveWantID {
				return "", false, &supersededEvidenceError{msg: fmt.Sprintf(
					"%s %s: the resolve outcome is unresolved, but last comment changed from %s to %s while the mutation was in flight",
					threadID,
					after.Path,
					effectiveWantID,
					after.LastID,
				)}
			}
			return msg, false, err
		}
		msg = fmt.Sprintf("resolved %s (isResolved=true) [recovered: the "+
			"resolve mutation reported an error, but it had already applied]", threadID)
		gotLastID = after.LastID
	}
	// The pre-mutation read above still leaves a window: a comment can land
	// while this mutation itself is in flight. resolveMutation asks GitHub
	// for the last comment as of the mutation's own response, so this check
	// is against the mutation applying, not a separate read after it — the
	// closest this client can get to atomic. A mismatch means the resolution
	// just went through on stale evidence; reopen it rather than leave a
	// thread with unread commentary silently closed. An EMPTY gotLastID
	// counts as a mismatch too: GitHub accepted the mutation but the response
	// didn't confirm what it resolved against, so it cannot be treated as
	// verified (the same reasoning as errReplyIDMissing for the reply
	// mutation).
	if gotLastID == "" {
		reason := "resolved without a response anchor confirming which comment it resolved against"
		if rErr := reopenOrFail(ctx, threadID, cur.Path, reason, retryUnchangedEvidence); rErr != nil {
			return "", false, rErr
		}
		return "", false, &unverifiableResolutionError{msg: fmt.Sprintf(
			"%s %s: reopened — the resolve response omitted its last-comment anchor",
			threadID, cur.Path)}
	}
	if effectiveWantID != "" && gotLastID != effectiveWantID {
		reason := fmt.Sprintf(
			"resolved after the evidence anchor changed from %s to %s",
			effectiveWantID,
			gotLastID,
		)
		if rErr := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); rErr != nil {
			return "", false, rErr
		}
		return "", false, &supersededEvidenceError{msg: fmt.Sprintf(
			"%s %s: reopened — last comment changed from %s to %s while resolution was in flight",
			threadID, cur.Path, effectiveWantID, gotLastID)}
	}
	return msg, true, nil
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
	if _, unverified := errors.AsType[*unverifiedProviderStateError](uErr); unverified {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"%s, and the automatic reopen outcome could not be verified: %v",
			reason,
			uErr,
		)}
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
}

func (e *failedReopenError) Error() string { return e.msg }

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

// unresolveWithEvidence accepts the mutation response only when mutateThread
// validated both target identity and state. Any other outcome is reconciled
// against a fresh exact-target read because the mutation may have applied
// before a transport error or malformed response reached the client.
func unresolveWithEvidence(ctx context.Context, threadID string) (string, error) {
	msg, _, _, mutationErr := mutateThread(ctx, "unresolve", threadID)
	if mutationErr == nil {
		return msg, nil
	}
	after, readErr := fetchThread(ctx, threadID)
	if readErr != nil {
		return "", &unverifiedProviderStateError{msg: fmt.Sprintf(
			"unresolve mutation reported %v, and the requested thread could not be re-read to determine whether it applied: %v",
			mutationErr,
			readErr,
		)}
	}
	if !after.IsResolved {
		return fmt.Sprintf(
			"unresolved %s (isResolved=false) [recovered: the mutation response was not proof, but a fresh provider read confirmed the requested state]",
			threadID,
		), nil
	}
	return "", &unresolveStillResolvedError{threadID: threadID, cause: mutationErr}
}

// isAccessDenied reports whether an error is GitHub refusing the caller rather
// than a transient failure. Retrying a denial just repeats it, so the two need
// different advice.
func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"permission", "forbidden", "unauthorized", "not accessible",
		"http 401", "http 403", "bad credentials", "requires authentication",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// checkCursorAdvances rejects a page cursor that has not moved. A response
// claiming another page while returning an empty or repeated cursor would
// otherwise loop forever, hammering the API and never letting the caller
// finish. Mirrors the guard in internal/safegit/threads.go.
func checkCursorAdvances(endCursor, current string) error {
	if endCursor == "" || endCursor == current {
		return fmt.Errorf("pagination did not advance")
	}
	return nil
}

// fetchAllComments returns every comment on a thread, oldest first, following
// cursor pagination. Used only where a bounded window would be unsound.
func fetchAllComments(ctx context.Context, threadID string) ([]tailComment, error) {
	var all []tailComment
	cursor := ""
	for {
		variables := map[string]any{"id": threadID}
		if cursor != "" {
			variables["after"] = cursor
		}
		raw, err := ghGraphQL(ctx, threadCommentsQuery, variables)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Data struct {
				Node struct {
					Comments struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID     string `json:"id"`
							Author struct {
								Login string `json:"login"`
							} `json:"author"`
							Body string `json:"body"`
						} `json:"nodes"`
					} `json:"comments"`
				} `json:"node"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("parse comments for %s: %w", threadID, err)
		}
		c := resp.Data.Node.Comments
		for _, n := range c.Nodes {
			if n.ID == "" {
				return nil, fmt.Errorf(
					"comments response for %s omitted a comment ID",
					threadID,
				)
			}
			all = append(all, tailComment{ID: n.ID, Login: n.Author.Login, Body: n.Body})
		}
		if !c.PageInfo.HasNextPage {
			return all, nil
		}
		if err := checkCursorAdvances(c.PageInfo.EndCursor, cursor); err != nil {
			return nil, fmt.Errorf("paging comments for %s: %w", threadID, err)
		}
		cursor = c.PageInfo.EndCursor
	}
}

// fetchHistoryTail fetches a thread's full comment history and returns the
// last two comment IDs alongside it: the two comments classifyPriorReply
// actually reasoned about, for callers that anchor a resolution to them.
// code is -1 when history was read and is non-empty; a thread with no
// comments at all cannot be reasoned about, so callers should stop rather
// than post blind.
func fetchHistoryTail(ctx context.Context, threadID, bodyFile string) (history []tailComment, lastID, prevID string, code int) {
	history, err := fetchAllComments(ctx, threadID)
	if err != nil {
		return nil, "", "", fail("provider state is unverified: cannot read complete thread history, nothing posted: %v\n%s",
			err, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	if len(history) == 0 {
		return nil, "", "", fail("thread %s has no comments to read; nothing was posted\n%s",
			threadID, inspectReplyOutcomeGuidance(threadID, bodyFile))
	}
	lastID = history[len(history)-1].ID
	if lastID == "" {
		return nil, "", "", fail(
			"thread %s history omitted the last comment ID needed as a predecessor; nothing was posted\n%s",
			threadID,
			inspectReplyOutcomeGuidance(threadID, bodyFile),
		)
	}
	if len(history) > 1 {
		prevID = history[len(history)-2].ID
	}
	return history, lastID, prevID, -1
}

// mutateThread resolves ("resolve") or re-opens ("unresolve") one thread and
// returns a human-readable confirmation line. lastCommentID is the thread's
// last comment as of the SAME response as the mutation, when the query
// requested it (currently only resolveMutation does); it is empty for
// "unresolve", which has no caller that needs it. isResolved is the
// mutation's own postcondition, not an assumption from its action: an
// "unresolve" whose GraphQL call succeeds is not proof the thread is now
// open — another resolver can race it — so callers reopening a thread must
// check this rather than treat a nil error as the postcondition itself.
func mutateThread(ctx context.Context, action, threadID string) (msg, lastCommentID string, isResolved bool, err error) {
	query, field := resolveMutation, "resolveReviewThread"
	wantResolved := true
	if action == "unresolve" {
		query, field = unresolveMutation, "unresolveReviewThread"
		wantResolved = false
	} else if action != "resolve" {
		return "", "", false, fmt.Errorf("unsupported thread mutation action %q", action)
	}
	raw, err := ghGraphQL(ctx, query, map[string]any{"threadId": threadID})
	if err != nil {
		return "", "", false, err
	}
	var resp struct {
		Data map[string]struct {
			Thread struct {
				ID         string `json:"id"`
				IsResolved *bool  `json:"isResolved"`
				Comments   struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"thread"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", "", false, fmt.Errorf("parse %s response: %w", field, err)
	}
	th := resp.Data[field].Thread
	if th.ID == "" {
		return "", "", false, fmt.Errorf(
			"%s response omitted the requested thread identity %s",
			field,
			threadID,
		)
	}
	if th.ID != threadID {
		return "", "", false, fmt.Errorf(
			"%s response returned mismatched thread ID %s for requested %s",
			field,
			th.ID,
			threadID,
		)
	}
	if th.IsResolved == nil {
		return "", "", false, fmt.Errorf(
			"%s response for %s omitted the requested resolved-state postcondition",
			field,
			threadID,
		)
	}
	gotResolved := *th.IsResolved
	if gotResolved != wantResolved {
		return "", "", gotResolved, fmt.Errorf(
			"%s response for %s reported isResolved=%t, want %t",
			field,
			threadID,
			gotResolved,
			wantResolved,
		)
	}
	if n := th.Comments.Nodes; len(n) > 0 {
		lastCommentID = n[len(n)-1].ID
	}
	return fmt.Sprintf("%sd %s (isResolved=%t)", action, th.ID, gotResolved), lastCommentID, gotResolved, nil
}

// ghGraphQL sends one typed GraphQL envelope through standard input and
// returns stdout. Query text and variable values never enter child argv.
// Body-free failures preserve gh's stderr. Reply failures suppress it because
// gh debug output can echo the request envelope, including the reply body.
func ghGraphQL(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	payload, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{Query: query, Variables: variables})
	if err != nil {
		return nil, fmt.Errorf("encode gh api graphql request: %w", err)
	}

	// #nosec G702 -- fixed executable and fixed argv; request data is stdin.
	cmd := exec.CommandContext(ctx, "gh", "api", "graphql", "--input", "-")
	cmd.Stdin = bytes.NewReader(payload)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if _, carriesReplyBody := variables["body"]; carriesReplyBody {
			return nil, fmt.Errorf("gh api graphql: %w (provider diagnostics suppressed because the request contains a reply body)", err)
		}
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("gh api graphql: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	return out.Bytes(), nil
}
