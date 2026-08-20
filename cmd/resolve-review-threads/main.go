// Command resolve-review-threads lists and resolves GitHub PR review threads.
//
// # Why this exists
//
// Gemini / bot reviewers open review threads on PRs. dear-agent's main branch
// has required_conversation_resolution=true, so a PR cannot merge while any
// thread is unresolved. Replying to a comment does NOT resolve its thread —
// resolution is a distinct GraphQL mutation. This tool resolves bot threads
// without a human clicking "Resolve conversation".
//
// # Key facts (verified against the live GitHub GraphQL schema, 2026-06-09)
//
//   - Thread resolution lives ONLY in GraphQL. There is NO REST endpoint.
//   - The mutation is resolveReviewThread(input:{threadId: ID!}). The input
//     field is threadId (NOT pullRequestReviewThreadId).
//   - Thread IDs come from repository.pullRequest.reviewThreads[].id and look
//     like "PRRT_kwDO...". They are NOT the review-comment IDs from REST.
//
// # Usage
//
//	resolve-review-threads list          <owner> <repo> <pr>           # unresolved threads (JSON lines)
//	resolve-review-threads list-all      <owner> <repo> <pr>           # every thread
//	resolve-review-threads resolve       <threadId> [--force]           # one thread by ID
//	resolve-review-threads reply-resolve <threadId> <body>             # reply, then resolve
//	resolve-review-threads resolve-all   <owner> <repo> <pr> [author]  # answered threads only
//	resolve-review-threads unresolve     <threadId>                    # re-open a thread
//
// # Why resolve-all refuses unanswered threads
//
// Resolution is a claim that the reviewer's point was handled. Two opposite
// failure modes have both shipped here: bulk-resolving every thread without
// addressing any of them (silently discarding real findings), and replying to
// every thread while resolving none (leaving the PR blocked forever). Both are
// invisible after the fact, because a resolved thread looks identical whether
// or not anyone read it.
//
// So resolution requires evidence, and the only evidence GitHub records
// deterministically is a reply on the thread. A thread is ANSWERED when its
// last comment comes from someone other than the reviewer who opened it. That
// is a weak proof of correctness but a strong proof of engagement, and it is
// checkable without judgment. `resolve-all` resolves ANSWERED threads and
// refuses the rest by name; `reply-resolve` makes answer-then-resolve one
// atomic step so the natural path closes the thread.
//
// isOutdated is deliberately NOT a licence to resolve. Outdated means the diff
// hunk moved, not that the point was addressed: on dear-agent#1242, three
// outdated threads were unaddressed P1 findings. It is reported, never acted
// on.
//
// All GitHub calls go through `gh api graphql`, so authentication uses the gh
// CLI's token (no git push, no keychain prompt). Requires gh (authenticated).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// bodyPreviewLen caps the comment-body preview surfaced in list output.
const bodyPreviewLen = 120

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
          recent: comments(last:1) { nodes { author { login } body } }
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
      recent: comments(last:1) { nodes { author { login } body } }
    }
  }
}`

const resolveMutation = `mutation($threadId:ID!) {
  resolveReviewThread(input:{threadId:$threadId}) {
    thread { id isResolved }
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
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 1
	}
	ctx := context.Background()
	cmd, rest := args[0], args[1:]

	switch cmd {
	case "list", "list-all":
		return cmdList(ctx, cmd, rest)
	case "resolve", "unresolve":
		return cmdMutate(ctx, cmd, rest)
	case "reply-resolve":
		return cmdReplyResolve(ctx, rest)
	case "resolve-all":
		return cmdResolveAll(ctx, rest)
	default:
		usage()
		return 1
	}
}

// cmdList handles "list" (unresolved only) and "list-all" (every thread).
func cmdList(ctx context.Context, cmd string, rest []string) int {
	if len(rest) != 3 {
		return fail("usage: %s <owner> <repo> <pr>", cmd)
	}
	pr, err := strconv.Atoi(rest[2])
	if err != nil {
		return fail("pr must be an integer, got %q", rest[2])
	}
	threads, err := listThreads(ctx, rest[0], rest[1], pr)
	if err != nil {
		return fail("%v", err)
	}
	if cmd == "list" {
		threads = filterThreads(threads, "")
	}
	if err := printThreads(threads); err != nil {
		return fail("%v", err)
	}
	return 0
}

// cmdMutate handles "resolve" and "unresolve" of a single thread by ID.
// "resolve" enforces the same evidence rule as resolve-all: a single-thread
// path that skipped it would be a documented way around the gate. "unresolve"
// re-opens a thread, which is never the unsafe direction, so it goes straight
// through.
func cmdMutate(ctx context.Context, cmd string, rest []string) int {
	force := false
	args := make([]string, 0, len(rest))
	for _, a := range rest {
		if a == "--force" {
			force = true
			continue
		}
		args = append(args, a)
	}
	if len(args) != 1 {
		return fail("usage: %s <threadId> [--force]", cmd)
	}
	if cmd == "unresolve" {
		msg, err := mutateThread(ctx, cmd, args[0])
		if err != nil {
			return fail("%v", err)
		}
		fmt.Println(msg)
		return 0
	}
	msg, err := resolveWithEvidence(ctx, args[0], force)
	if err != nil {
		return fail("%v", err)
	}
	fmt.Println(msg)
	return 0
}

// cmdReplyResolve posts a reply on one thread and then resolves it, so the
// public justification and the resolution cannot drift apart. Resolving is
// skipped when the reply fails, leaving the thread open rather than silently
// closed with no explanation.
func cmdReplyResolve(ctx context.Context, rest []string) int {
	if len(rest) != 2 {
		return fail("usage: reply-resolve <threadId> <body>")
	}
	threadID, body := rest[0], rest[1]
	if len(bytes.TrimSpace([]byte(body))) == 0 {
		return fail("reply body must not be empty: resolution needs a stated reason")
	}
	// Retry safety: if this exact reply is already the thread's last comment,
	// a previous run posted it and only the resolve failed. Posting again
	// would leave a duplicate public comment on every retry.
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		// Without the current state we cannot tell whether a previous run
		// already posted this reply, and posting blind is the duplicate this
		// guard exists to prevent. Leave the thread untouched.
		return fail("cannot read thread state, nothing posted: %v", err)
	}
	if cur.LastAuthor != cur.Author && sameReplyBody(cur.LastBody, body) {
		fmt.Fprintln(os.Stderr, "reply already present; resolving only")
	} else if _, err := ghGraphQL(ctx, "-f", "threadId="+threadID, "-f", "body="+body,
		"-f", "query="+replyMutation); err != nil {
		return fail("reply failed, thread left unresolved: %v", err)
	}
	msg, rErr := resolveWithEvidence(ctx, threadID, false)
	if rErr != nil {
		var unanswered *unansweredError
		if errors.As(rErr, &unanswered) {
			// The reviewer commented again in the gap, so they now hold the
			// thread. Prescribing the resolve-only path here would just fail
			// the same evidence check; the honest next step is to read the
			// follow-up and answer it.
			return fail("your reply is posted, but the reviewer has since "+
				"commented again and now holds this thread:\n%v\n"+
				"read the follow-up and answer it; reply-resolve is the right "+
				"command once you have", rErr)
		}
		return fail("the reply is posted but the thread is NOT resolved: %v\n"+
			"do NOT re-run reply-resolve (it would repost); finish with:\n"+
			"  resolve-review-threads resolve %s", rErr, threadID)
	}
	fmt.Println(msg)
	return 0
}

// cmdResolveAll resolves the unresolved threads on a PR that carry evidence of
// a response, optionally limited to a single opening author. Unanswered threads
// are refused by name: see the package comment for why resolution requires
// evidence. Exits non-zero when anything was refused, so a caller driving a PR
// to mergeable never mistakes a partial pass for reaching zero.
func cmdResolveAll(ctx context.Context, rest []string) int {
	force := false
	args := make([]string, 0, len(rest))
	for _, a := range rest {
		if a == "--force" {
			force = true
			continue
		}
		args = append(args, a)
	}
	if len(args) < 3 || len(args) > 4 {
		return fail("usage: resolve-all <owner> <repo> <pr> [author] [--force]")
	}
	pr, err := strconv.Atoi(args[2])
	if err != nil {
		return fail("pr must be an integer, got %q", args[2])
	}
	author := ""
	if len(args) == 4 {
		author = args[3]
	}
	threads, err := listThreads(ctx, args[0], args[1], pr)
	if err != nil {
		return fail("%v", err)
	}

	resolved, refused := 0, 0
	for _, t := range filterThreads(threads, author) {
		if !t.Answered && !force {
			refused++
			fmt.Fprintf(os.Stderr, "REFUSED %s %s: unanswered (last word: @%s)%s\n",
				t.ID, t.Path, t.LastAuthor, outdatedNote(t))
			continue
		}
		msg, err := resolveWithEvidence(ctx, t.ID, force)
		if err != nil {
			var unanswered *unansweredError
			if !errors.As(err, &unanswered) {
				// Auth, permissions, or a GraphQL outage. Retrying it once per
				// remaining thread would just repeat a denied call and then
				// report the pile as "nobody replied".
				return fail("aborting sweep after %d resolved, %d refused: %v", resolved, refused, err)
			}
			// A thread that went unanswered between the listing and now is a
			// refusal, not a fatal error: keep sweeping the rest.
			refused++
			fmt.Fprintf(os.Stderr, "REFUSED %v\n", err)
			continue
		}
		fmt.Println(msg)
		resolved++
	}

	fmt.Printf("resolved %d thread(s), refused %d\n", resolved, refused)
	if refused == 0 {
		return 0
	}
	fmt.Fprintf(os.Stderr, `
%d thread(s) were refused because nobody has replied to them. Resolving a
thread asserts its point was handled; without a reply there is no record that
it was even read. Address each one in code, then close it with its reason:

  resolve-review-threads reply-resolve <threadId> "Fixed — <what changed>"

--force overrides this and resolves unanswered threads. Reserve it for threads
you are deliberately dismissing, and say so in a PR comment.
`, refused)
	return 1
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
	Recent struct {
		Nodes []struct {
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
	if len(n.Recent.Nodes) > 0 {
		lastLogin = n.Recent.Nodes[0].Author.Login
		t.LastBody = n.Recent.Nodes[0].Body
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
func resolveWithEvidence(ctx context.Context, threadID string, force bool) (string, error) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return "", err
	}
	if cur.IsResolved {
		return fmt.Sprintf("skipped %s (already resolved)", threadID), nil
	}
	if !cur.Answered && !force {
		return "", &unansweredError{msg: fmt.Sprintf("%s %s: unanswered (last word: @%s)%s\n"+
			"reply with the reason instead: resolve-review-threads reply-resolve %s \"Fixed - <what changed>\"",
			threadID, cur.Path, cur.LastAuthor, outdatedNote(cur), threadID)}
	}
	return mutateThread(ctx, "resolve", threadID)
}

// mutateThread resolves ("resolve") or re-opens ("unresolve") one thread and
// returns a human-readable confirmation line.
func mutateThread(ctx context.Context, action, threadID string) (string, error) {
	query, field := resolveMutation, "resolveReviewThread"
	if action == "unresolve" {
		query, field = unresolveMutation, "unresolveReviewThread"
	}
	raw, err := ghGraphQL(ctx, "-f", "threadId="+threadID, "-f", "query="+query)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data map[string]struct {
			Thread struct {
				ID         string `json:"id"`
				IsResolved bool   `json:"isResolved"`
			} `json:"thread"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse %s response: %w", field, err)
	}
	th := resp.Data[field].Thread
	return fmt.Sprintf("%sd %s (isResolved=%t)", action, th.ID, th.IsResolved), nil
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

// printThreads emits one compact JSON object per line, matching the original
// shell wrapper's output contract.
func printThreads(ts []thread) error {
	enc := json.NewEncoder(os.Stdout)
	for _, t := range ts {
		if err := enc.Encode(t); err != nil {
			return err
		}
	}
	return nil
}

// cleanBody collapses runs of whitespace to single spaces and truncates to
// bodyPreviewLen runes (rune-safe so multibyte bodies aren't split mid-char).
func cleanBody(s string) string {
	fields := bytes.Fields([]byte(s))
	collapsed := string(bytes.Join(fields, []byte(" ")))
	r := []rune(collapsed)
	if len(r) > bodyPreviewLen {
		return string(r[:bodyPreviewLen])
	}
	return collapsed
}

// sameReplyBody reports whether a thread's most recent comment is the reply we
// are about to post. It compares the bodies losslessly apart from surrounding
// whitespace: cleanBody collapses runs and truncates to a preview length, so
// using it here would fail to match any multi-line or long reply and repost it.
func sameReplyBody(lastBody, want string) bool {
	return strings.TrimSpace(lastBody) == strings.TrimSpace(want)
}

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	return 1
}

func usage() {
	fmt.Fprint(os.Stderr, `resolve-review-threads — list and resolve GitHub PR review threads

usage:
  resolve-review-threads list          <owner> <repo> <pr>          unresolved threads (JSON lines)
  resolve-review-threads list-all      <owner> <repo> <pr>          every thread
  resolve-review-threads resolve       <threadId> [--force]          resolve one thread by ID
                                                                    (same evidence rule as resolve-all)
  resolve-review-threads unresolve     <threadId>                   re-open one thread by ID
  resolve-review-threads reply-resolve <threadId> <body>            reply with the reason, then
                                                                    resolve (the normal path)
  resolve-review-threads resolve-all   <owner> <repo> <pr> [author] [--force]
                                                                    resolve ANSWERED threads only;
                                                                    refuses unanswered ones by name
                                                                    and exits non-zero

A thread counts as answered when someone other than its opening author had the
last word. Resolving asserts the point was handled, so it needs that evidence;
--force overrides it for threads you are deliberately dismissing. Outdated is
reported but never sufficient: the hunk moving is not the point being fixed.

Resolution is GraphQL-only; all calls go through an authenticated gh CLI.
`)
}
