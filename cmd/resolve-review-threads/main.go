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
//
// The GraphQL queries/mutations and the thread-fetching/mutating layer live in
// threads.go; this file is the CLI: argument parsing, command dispatch, and
// the reply-then-resolve safety argument.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// bodyPreviewLen caps the comment-body preview surfaced in list output.
const bodyPreviewLen = 120

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
	case "-h", "--help", "help":
		usage()
		return 0
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
		msg, _, err := mutateThread(ctx, cmd, args[0])
		if err != nil {
			return fail("%v", err)
		}
		fmt.Println(msg)
		return 0
	}
	msg, _, err := resolveWithEvidence(ctx, args[0], force, "")
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
	// Read first: to skip an already-resolved thread, and to notice a reply a
	// previous run already posted so a retry does not duplicate it. Without
	// the current state we cannot tell whether a previous run already posted
	// this reply, and posting blind is the duplicate this guard exists to
	// prevent, so any failure here leaves the thread untouched.
	_, code := readOrExit(ctx, threadID, "cannot read thread state, nothing posted")
	if code >= 0 {
		return code
	}

	// Decide against the FULL history, not the tail: a prior reply buried by
	// later discussion must still be found, or it gets reposted and anchored
	// to the newest follow-up, which resolves everything in between unread.
	// historyLastID/historyPrevID are the two comments classifyPriorReply
	// actually reasoned about (the tail of history, oldest-first). Anchoring
	// to these below, rather than to a later read's LastID/PrevID, keeps the
	// anchor and the classification talking about the same comment.
	history, historyLastID, historyPrevID, code := fetchHistoryTail(ctx, threadID)
	if code >= 0 {
		return code
	}

	// Paging a long thread takes time, so re-read state before acting on it:
	// another actor may have resolved it meanwhile, and replying then would
	// comment on a settled conversation. If the tail has moved since history
	// was read, a comment landed in that gap that classifyPriorReply never
	// saw; refuse rather than silently adopt it as the anchor, which would
	// treat that unread comment as accounted for.
	_, code = readMatchingTail(ctx, threadID, historyLastID)
	if code >= 0 {
		return code
	}

	// anchorID must be the thread's last comment, and anchorPrevID must be the
	// comment before it, for resolution to be allowed. Both are required: see
	// checkReplyPlacement.
	var anchorID, anchorPrevID string
	switch classifyPriorReply(history, body) {
	case priorReplySuperseded:
		// A previous run posted this reply and the reviewer has spoken since.
		// Reposting would duplicate the comment AND put our copy last, making
		// the thread look answered while the follow-up went unread.
		return fail("your earlier reply is already on thread %s and the reviewer "+
			"has commented since; nothing was posted.\n"+
			"read the comment(s) after your reply and answer those:\n"+
			"  resolve-review-threads reply-resolve %s \"...\"", threadID, threadID)
	case priorReplyIsLast:
		// A previous run posted this and nothing has been said since, so the
		// comment we matched (confirmed above to still be the true tail) is
		// the anchor.
		fmt.Fprintln(os.Stderr, "reply already present; resolving only")
		anchorID, anchorPrevID = historyLastID, historyPrevID
	case noPriorReply:
		// Our reply must land directly after the comment we just read.
		anchorPrevID = historyLastID
		id, code := postReplyOrExit(ctx, threadID, body)
		if code >= 0 {
			return code
		}
		anchorID = id
	}
	if code := verifyReplyPlacement(ctx, threadID, anchorPrevID, anchorID, body); code != 0 {
		return code
	}
	msg, _, rErr := resolveWithEvidence(ctx, threadID, false, anchorID)
	if rErr != nil {
		var reopenFailed *failedReopenError
		if errors.As(rErr, &reopenFailed) {
			// The thread is STILL marked resolved on GitHub. Every other
			// branch here treats "not resolved" as ground truth and tells the
			// operator retrying reply-resolve is safe — but retrying now
			// would hit readOrExit's IsResolved check first, print "skipped
			// (already resolved)", and exit 0 without ever reopening the
			// thread or reading what landed on it. unresolve must run first.
			return fail("your reply is posted, but resolving it failed AND "+
				"reopening it also failed: %v\n"+
				"do NOT just retry reply-resolve — it would see the thread as "+
				"already resolved and silently no-op. Reopen it first:\n"+
				"  resolve-review-threads unresolve %s\n"+
				"then read what landed on it and answer it", rErr, threadID)
		}
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
		if isAccessDenied(rErr) {
			// Retrying immediately would just repeat the denied mutation; the
			// credential problem has to be fixed first. Once it is, prefer
			// reply-resolve over bare resolve for the same reason as the
			// generic case below: it preserves the anchor check. The body is
			// spelled out, not "...", because retry safety depends on
			// text equality: a DIFFERENT body is a new reply that lands after
			// whatever showed up while this was failing, not a safe retry.
			return fail("your reply is posted, but GitHub refused the resolution: %v\n"+
				"this is an access problem, not a transient one: retrying immediately "+
				"will be denied too.\n"+
				"check `gh auth status` and that the token can resolve threads on this "+
				"repo, then finish with the EXACT SAME body:\n"+
				"  resolve-review-threads reply-resolve %s %q", rErr, threadID, body)
		}
		return fail("the reply is posted but the thread is NOT resolved (likely "+
			"transient): %v\n"+
			"this is safe to retry with the EXACT SAME body below (not a "+
			"reworded one — retry safety depends on text equality): "+
			"reply-resolve will see your reply is already last and resolve "+
			"without reposting it. Bare resolve would work too, but it drops "+
			"the anchor check this thread is relying on, so prefer:\n"+
			"  resolve-review-threads reply-resolve %s %q", rErr, threadID, body)
	}
	fmt.Println(msg)
	return 0
}

// postReply posts a reply and returns the new comment's node ID. That ID is
// the anchor for the whole safety argument: resolution is permitted only when
// this exact comment is the thread's last one.
func postReply(ctx context.Context, threadID, body string) (string, error) {
	raw, err := ghGraphQL(ctx, "-f", "threadId="+threadID, "-f", "body="+body,
		"-f", "query="+replyMutation)
	if err != nil {
		return "", err
	}
	return parseReplyCommentID(raw)
}

// postReplyOrExit posts a reply and reports whether the caller should return
// immediately. code is -1 when id is a fresh anchor safe to resolve against.
// errReplyIDMissing gets distinct advice from every other failure: the reply
// text is already live, so telling the operator to just retry — as a generic
// failure would — risks a real duplicate if they reword it while retrying.
func postReplyOrExit(ctx context.Context, threadID, body string) (id string, code int) {
	id, err := postReply(ctx, threadID, body)
	if err == nil {
		return id, -1
	}
	if errors.Is(err, errReplyIDMissing) {
		return "", fail("your reply was posted, but GitHub's response did not "+
			"confirm its ID, so it cannot be used as the resolution anchor: %v\n"+
			"the thread was left UNRESOLVED on purpose. Do NOT reword and repost: "+
			"re-run with the EXACT SAME body below and reply-resolve will find "+
			"your reply by its text and resolve without duplicating it:\n"+
			"  resolve-review-threads reply-resolve %s %q", err, threadID, body)
	}
	return "", fail("reply failed, thread left unresolved: %v", err)
}

// parseReplyCommentID extracts the new comment's node ID from the reply
// mutation response. An absent ID is an error rather than an empty anchor:
// without it there is nothing to prove our reply is the last comment, and
// resolving on a blank anchor would defeat the check entirely.
func parseReplyCommentID(raw []byte) (string, error) {
	var resp struct {
		Data struct {
			AddPullRequestReviewThreadReply struct {
				Comment struct {
					ID string `json:"id"`
				} `json:"comment"`
			} `json:"addPullRequestReviewThreadReply"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("parse reply response: %w", err)
	}
	id := resp.Data.AddPullRequestReviewThreadReply.Comment.ID
	if id == "" {
		return "", errReplyIDMissing
	}
	return id, nil
}

// errReplyIDMissing marks a reply mutation GitHub accepted — the comment is
// live on the thread — whose response simply omitted the new comment's ID.
// It is distinguished from a genuine post failure because the two need
// opposite advice: retrying with a DIFFERENT body would create a real
// duplicate, but retrying reply-resolve with the SAME body stays safe, since
// classifyPriorReply finds the posted comment by its text and resolves
// without reposting it.
var errReplyIDMissing = errors.New("reply posted but GitHub returned no comment ID")

// replyPlacement describes where our reply ended up relative to what we read.
type replyPlacement int

const (
	// replyExact means our reply is the last comment AND it directly follows
	// the comment we had read: nobody spoke on either side of it.
	replyExact replyPlacement = iota
	// replyBuried means something was said after our reply.
	replyBuried
	// replyJumped means something was said between our read and our reply, so
	// our reply sits on top of a comment we never saw.
	replyJumped
)

// checkReplyPlacement is the whole safety argument, as a pure function.
//
// Two conditions are needed and neither implies the other. wantLast proves
// nothing came AFTER our reply; wantPrev proves nothing arrived between our
// read and our post, which would leave our reply sitting on top of an unread
// comment while still being last. An earlier revision replaced the second
// check with the first and silently reopened that race, which is why this
// lives in a tested pure function instead of inline in the command.
func checkReplyPlacement(gotPrev, gotLast, wantPrev, wantLast string) replyPlacement {
	if gotLast != wantLast {
		return replyBuried
	}
	if gotPrev != wantPrev {
		return replyJumped
	}
	return replyExact
}

// verifyReplyPlacement re-reads the thread and applies checkReplyPlacement.
// Returns 0 when the reply is exactly where we expect it.
func verifyReplyPlacement(ctx context.Context, threadID, wantPrevID, wantLastID, body string) int {
	after, err := fetchThread(ctx, threadID)
	if err != nil {
		return fail("your reply is posted but the thread state could not be "+
			"re-read, so it was NOT resolved (likely transient): %v\n"+
			"this is safe to retry with the EXACT SAME body below (not a "+
			"reworded one — retry safety depends on text equality): "+
			"reply-resolve will see your reply is already last and resolve "+
			"without reposting it. Bare resolve would work too, but it drops "+
			"the anchor check this thread is relying on, so prefer:\n"+
			"  resolve-review-threads reply-resolve %s %q", err, threadID, body)
	}
	switch checkReplyPlacement(after.PrevID, after.LastID, wantPrevID, wantLastID) {
	case replyBuried:
		return fail("your reply is posted, but it is not the last comment on "+
			"thread %s: someone spoke after it, and your reply does not answer them.\n"+
			"the thread was left UNRESOLVED on purpose. Read what came after "+
			"your reply, then answer it with:\n"+
			"  resolve-review-threads reply-resolve %s \"...\"", threadID, threadID)
	case replyJumped:
		return fail("your reply is posted, but someone commented between the "+
			"moment this command read thread %s and the moment it replied, so "+
			"your reply sits on top of a comment it does not answer.\n"+
			"the thread was left UNRESOLVED on purpose. Read the comment above "+
			"your reply, then answer it with:\n"+
			"  resolve-review-threads reply-resolve %s \"...\"", threadID, threadID)
	case replyExact:
		return 0
	}
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

	resolved, refused, skipped := 0, 0, 0
	for _, t := range filterThreads(threads, author) {
		// No pre-check on the listing snapshot: a reply may have arrived
		// since, and refusing on stale state would report "nobody replied"
		// about a thread that is currently answered. resolveWithEvidence
		// re-reads and is the single place that decides.
		msg, mutated, err := resolveWithEvidence(ctx, t.ID, force, "")
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
		if mutated {
			resolved++
		} else {
			skipped++
		}
	}

	fmt.Printf("resolved %d thread(s), refused %d, skipped %d\n", resolved, refused, skipped)
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

// priorReplyState describes what a previous reply-resolve run left behind.
type priorReplyState int

const (
	// noPriorReply means this body is not present in the thread tail.
	noPriorReply priorReplyState = iota
	// priorReplyIsLast means a previous run already posted it and nothing has
	// been said since, so only the resolve remains.
	priorReplyIsLast
	// priorReplySuperseded means a previous run posted it and someone has
	// commented afterwards, so the prewritten body no longer answers the
	// thread.
	priorReplySuperseded
)

// classifyPriorReply reports whether this exact reply already sits in the
// thread and, if so, whether anyone has spoken after it. Reposting a
// superseded reply would both duplicate the comment and, because our copy
// would then be last, make the thread look answered while the follow-up went
// unread.
func classifyPriorReply(tail []tailComment, body string) priorReplyState {
	idx := -1
	for i, c := range tail {
		if sameReplyBody(c.Body, body) {
			idx = i
		}
	}
	switch {
	case idx < 0:
		return noPriorReply
	case idx == len(tail)-1:
		return priorReplyIsLast
	default:
		return priorReplySuperseded
	}
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
