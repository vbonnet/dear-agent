// GraphQL queries, mutations, and the thread-fetching/mutating layer
// underneath the commands in main.go: everything that talks to GitHub.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
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
		args := []string{
			"-f", "owner=" + owner,
			"-f", "repo=" + repo,
			"-F", "pr=" + strconv.Itoa(pr),
			"-f", "query=" + listQuery,
		}
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		raw, err := ghGraphQL(ctx, args...)
		if err != nil {
			return nil, err
		}

		var resp threadsResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("parse reviewThreads response: %w", err)
		}

		rt := resp.Data.Repository.PullRequest.ReviewThreads
		for _, n := range rt.Nodes {
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
	IsResolved bool   `json:"isResolved"`
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
	t := thread{ID: n.ID, IsResolved: n.IsResolved, IsOutdated: n.IsOutdated, Path: n.Path, Author: "unknown"}
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
	raw, err := ghGraphQL(ctx, "-f", "id="+threadID, "-f", "query="+threadByIDQuery)
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
	return toThread(resp.Data.Node), nil
}

// readOrExit re-reads a thread and reports whether the caller should return
// immediately. code is -1 when t is fresh and safe to act on; otherwise the
// caller should `return code` as-is. A read failure fails with errMsg framing
// context the generic error lacks; an already-resolved thread means someone
// else closed it since the caller last looked, so the skip is reported here
// once rather than at every call site.
func readOrExit(ctx context.Context, threadID, errMsg string) (t thread, code int) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return thread{}, fail("%s: %v", errMsg, err)
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
func readMatchingTail(ctx context.Context, threadID, wantLastID string) (t thread, code int) {
	cur, exitCode := readOrExit(ctx, threadID, "cannot re-read thread state, nothing posted")
	if exitCode >= 0 {
		return cur, exitCode
	}
	if cur.LastID != wantLastID {
		return cur, fail("someone commented on thread %s while its history was "+
			"being read; nothing was posted.\n"+
			"read the new comment(s) and answer those:\n"+
			"  resolve-review-threads reply-resolve %s \"...\"", threadID, threadID)
	}
	return cur, -1
}

// unansweredError marks a thread that was refused on evidence, as opposed to
// a GraphQL or permission failure. The sweep continues past the former and
// stops on the latter, so a globally denied operation is not retried once per
// thread and then misreported as "nobody replied".
type unansweredError struct{ msg string }

func (e *unansweredError) Error() string { return e.msg }

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
		return "", false, err
	}
	if wantLastID != "" && cur.LastID != wantLastID {
		return "", false, &unansweredError{msg: fmt.Sprintf(
			"%s %s: the comment this resolution was based on is no longer last "+
				"(someone commented after it)", threadID, cur.Path)}
	}
	if cur.IsResolved {
		// Someone else closed it between the listing and now. Report it, but
		// do not claim it as this sweep's work: the count is an audit record.
		return fmt.Sprintf("skipped %s (already resolved)", threadID), false, nil
	}
	if !cur.Answered && !force {
		return "", false, &unansweredError{msg: fmt.Sprintf("%s %s: unanswered (last word: @%s)%s\n"+
			"reply with the reason instead: resolve-review-threads reply-resolve %s \"Fixed - <what changed>\"",
			threadID, cur.Path, cur.LastAuthor, outdatedNote(cur), threadID)}
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
	msg, gotLastID, err := mutateThread(ctx, "resolve", threadID)
	if err != nil {
		// gh api can fail on the client side (network drop, timeout) after
		// GitHub already applied the mutation server-side: this error alone
		// does not prove the resolve never happened. Re-read before trusting
		// it as a clean no-op, so a resolution that actually went through
		// still gets the same reconciliation below rather than silently
		// skipping it because the client-side signal was ambiguous.
		after, checkErr := fetchThread(ctx, threadID)
		if checkErr != nil || !after.IsResolved {
			// Genuinely failed, or genuinely unclear and we can't do better:
			// report the original error, same as before this check existed.
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
	if effectiveWantID != "" && gotLastID != effectiveWantID {
		if _, _, uErr := mutateThread(ctx, "unresolve", threadID); uErr != nil {
			return "", false, &failedReopenError{msg: fmt.Sprintf(
				"%s %s: resolved on stale or unverifiable evidence (someone "+
					"commented while the mutation was in flight, or GitHub's "+
					"response didn't confirm what it resolved against) AND "+
					"reopening it failed: %v — the thread is STILL RESOLVED. "+
					"Run this before anything else: "+
					"resolve-review-threads unresolve %s, then read the new "+
					"comment and answer it", threadID, cur.Path, uErr, threadID)}
		}
		return "", false, &unansweredError{msg: fmt.Sprintf(
			"%s %s: reopened — someone commented while the resolve mutation was "+
				"in flight, or GitHub's response didn't confirm what it resolved "+
				"against, so it is not accounted for", threadID, cur.Path)}
	}
	return msg, true, nil
}

// failedReopenError marks a resolution that went through on stale or
// unverifiable evidence AND whose automatic reopen also failed: the thread is
// left resolved on GitHub despite that. It is distinguished from
// unansweredError because a caller's generic "safe to retry" advice for that
// type is actively wrong here — reply-resolve's own readOrExit would see
// IsResolved, print "skipped ... (already resolved)", and exit 0 without ever
// reading the intervening comment or reopening the thread.
type failedReopenError struct{ msg string }

func (e *failedReopenError) Error() string { return e.msg }

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
		args := []string{"-f", "id=" + threadID, "-f", "query=" + threadCommentsQuery}
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		raw, err := ghGraphQL(ctx, args...)
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
func fetchHistoryTail(ctx context.Context, threadID string) (history []tailComment, lastID, prevID string, code int) {
	history, err := fetchAllComments(ctx, threadID)
	if err != nil {
		return nil, "", "", fail("cannot read thread history, nothing posted: %v", err)
	}
	if len(history) == 0 {
		return nil, "", "", fail("thread %s has no comments to read; nothing was posted", threadID)
	}
	lastID = history[len(history)-1].ID
	if len(history) > 1 {
		prevID = history[len(history)-2].ID
	}
	return history, lastID, prevID, -1
}

// mutateThread resolves ("resolve") or re-opens ("unresolve") one thread and
// returns a human-readable confirmation line. lastCommentID is the thread's
// last comment as of the SAME response as the mutation, when the query
// requested it (currently only resolveMutation does); it is empty for
// "unresolve", which has no caller that needs it.
func mutateThread(ctx context.Context, action, threadID string) (msg, lastCommentID string, err error) {
	query, field := resolveMutation, "resolveReviewThread"
	if action == "unresolve" {
		query, field = unresolveMutation, "unresolveReviewThread"
	}
	raw, err := ghGraphQL(ctx, "-f", "threadId="+threadID, "-f", "query="+query)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		Data map[string]struct {
			Thread struct {
				ID         string `json:"id"`
				IsResolved bool   `json:"isResolved"`
				Comments   struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"thread"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", "", fmt.Errorf("parse %s response: %w", field, err)
	}
	th := resp.Data[field].Thread
	if n := th.Comments.Nodes; len(n) > 0 {
		lastCommentID = n[len(n)-1].ID
	}
	return fmt.Sprintf("%sd %s (isResolved=%t)", action, th.ID, th.IsResolved), lastCommentID, nil
}

// ghGraphQL runs `gh api graphql <args...>` and returns stdout. Stderr is
// folded into the error so gh's diagnostics survive.
func ghGraphQL(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"api", "graphql"}, args...)
	// #nosec G702 G204 — fixed "gh" binary; args are passed as argv (no shell),
	// so owner/repo/threadId values cannot inject commands.
	cmd := exec.CommandContext(ctx, "gh", full...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(errBuf.Bytes()); len(msg) > 0 {
			return nil, fmt.Errorf("gh api graphql: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	return out.Bytes(), nil
}
