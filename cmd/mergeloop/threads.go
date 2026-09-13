package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/mergeloop"
)

// knownBotLogins are the review-bot accounts whose unresolved threads the
// mergeloop auto-resolves before attempting a merge. dear-agent's main branch
// enforces required_conversation_resolution, so a single bot finding left as an
// open thread blocks the merge even when every CI check is green and the bot's
// only intent was advisory. Logins are stored in their normalized (no "[bot]"
// suffix) form; see normalizeBotLogin.
//
// Human-authored threads are NEVER auto-resolved: silently resolving a person's
// thread would hide unaddressed feedback, which is exactly what
// required_conversation_resolution exists to prevent.
//
// gemini-code-assist was removed 2026-06-24 (#724) in anticipation of its
// consumer tier sunsetting 2026-07-17, and chatgpt-codex-connector was never
// added. Both are still actively commenting as of 2026-07-20 (confirmed via
// live PR review threads, e.g. #960), so the map sat empty for nearly a
// month: mergeloop's auto-resolve step became a silent no-op, and every PR
// that received a bot comment stayed BLOCKED on required_conversation_
// resolution with fully green CI (#945, #947, #949, #950, #960, #961, #976).
// Restore both; re-remove a login only once its bot has actually stopped
// commenting.
var knownBotLogins = map[string]bool{
	"gemini-code-assist":      true,
	"chatgpt-codex-connector": true,
}

// normalizeBotLogin strips the "[bot]" suffix that some GitHub surfaces append
// to GitHub-App accounts. The reviews/threads GraphQL API returns the bare
// login (e.g. "gemini-code-assist") while operators commonly write
// "gemini-code-assist[bot]" in config; normalizing both ends lets either form
// match.
func normalizeBotLogin(login string) string {
	return strings.TrimSuffix(login, "[bot]")
}

// isKnownBotAuthor reports whether a review-thread author is a known bot whose
// threads may be auto-resolved.
func isKnownBotAuthor(login string) bool {
	return knownBotLogins[normalizeBotLogin(login)]
}

// allCommentsFromKnownBots reports whether every comment in a thread —
// including any human reply after the bot's opening comment — is authored by
// a known bot. A single non-bot author anywhere in the thread means the
// thread must never be auto-resolved (MLC-05). An empty slice is not a bot
// thread.
func allCommentsFromKnownBots(logins []string) bool {
	if len(logins) == 0 {
		return false
	}
	for _, login := range logins {
		if !isKnownBotAuthor(login) {
			return false
		}
	}
	return true
}

// ghThreadResolver implements mergeloop.ThreadResolver by resolving unresolved
// review threads authored by known bots via the GitHub GraphQL
// resolveReviewThread mutation. Thread resolution is GraphQL-only — there is no
// REST endpoint — so every call goes through an authenticated gh CLI.
type ghThreadResolver struct{ dryRun bool }

const threadsListQuery = `query($owner:String!,$repo:String!,$pr:Int!,$after:String){
  repository(owner:$owner,name:$repo){
    pullRequest(number:$pr){
      number
      reviewThreads(first:100,after:$after){
        pageInfo{ hasNextPage endCursor }
        nodes{
          id
          isResolved
          comments(first:100){ pageInfo{ hasNextPage } nodes{ author{ login __typename } body createdAt lastEditedAt } }
        }
      }
    }
  }
}`

// threadByIDQuery re-reads ONE thread immediately before it is resolved. The
// listing that chose it is a snapshot: a human can reply, or the bot can edit an
// advisory comment into a finding, between the read and the mutation. Resolving
// on a stale decision would silently close a person's newly posted disagreement,
// which is the MLC-05 violation this whole change exists to prevent.
const threadByIDQuery = `query($id:ID!){
  node(id:$id){
    ... on PullRequestReviewThread {
      id
      isResolved
      comments(first:100){ pageInfo{ hasNextPage } nodes{ author{ login __typename } body createdAt lastEditedAt } }
    }
  }
}`

const threadResolveMutation = `mutation($threadId:ID!){
  resolveReviewThread(input:{threadId:$threadId}){ thread{ id isResolved } }
}`

// threadComment is one comment on a review thread.
type threadComment struct {
	author string
	body   string
	// typename is the GraphQL actor type of the comment author ("User",
	// "Bot", "Organization", ...). It is verified positively: only a real
	// User counts as a human reply. Inferring "human" from absence in a
	// two-login bot allowlist let dependabot[bot], a GitHub Actions bot, or
	// any newly introduced review bot silently clear a P1 finding.
	typename string
	// createdAt is when the comment was posted and lastEditedAt when it was
	// last revised (zero if never). Engagement is judged on TIME, not on
	// position in the thread: a bot that edits an earlier advisory comment
	// into a P1 keeps its original position, so an ordering check would read a
	// human who replied before that edit as having engaged with a finding that
	// did not yet exist.
	createdAt    time.Time
	lastEditedAt time.Time
}

// effectiveAt is the instant this comment last said what it says now. A human
// can only have engaged with a finding as it reads after its latest revision.
func (c threadComment) effectiveAt() time.Time {
	if c.lastEditedAt.After(c.createdAt) {
		return c.lastEditedAt
	}
	return c.createdAt
}

// reviewThread is one PR review thread as fetched from GraphQL.
type reviewThread struct {
	id         string
	isResolved bool
	comments   []threadComment
	// truncated reports that the thread has more comments than one page
	// fetched, so its comment list cannot be trusted to be complete.
	truncated bool
}

// botThread is a single review thread the loop may auto-resolve.
type botThread struct {
	id     string
	author string
}

// logins returns the comment authors in order.
func (t reviewThread) logins() []string {
	out := make([]string, len(t.comments))
	for i, c := range t.comments {
		out[i] = c.author
	}
	return out
}

// bodies returns the comment bodies in order.
func (t reviewThread) bodies() []string {
	out := make([]string, len(t.comments))
	for i, c := range t.comments {
		out[i] = c.body
	}
	return out
}

// hasHumanComment reports whether any comment on the thread was written by
// someone other than an allowlisted bot. A human in the thread means a person
// engaged with the finding, so the merge gate treats it as addressed.
func (t reviewThread) hasHumanComment() bool {
	for _, c := range t.comments {
		if isHumanActor(c.typename, c.author) {
			return true
		}
	}
	return false
}

// isBotFindingAuthor reports whether a comment could carry a bot finding the
// independent gate owns.
//
// It is the complement of isHumanActor rather than a membership test against
// the auto-resolve allowlist, and the asymmetry is the point. Auto-resolving a
// thread is a PRIVILEGE granted to two known logins and must stay narrow.
// Refusing a merge is a PROTECTION, and withholding it from every bot this
// code has not been told about is exactly how an unread P1 slips through. An
// actor the API could not type is not provably a person, so it counts too.
func isBotFindingAuthor(typename, login string) bool {
	return !isHumanActor(typename, login)
}

// isHumanActor reports whether a comment author is a real person.
//
// It fails closed twice over. The GraphQL actor type must be exactly "User",
// so every Bot and Organization actor is excluded regardless of the login
// allowlist. An empty typename (an older cached payload, or a field the API
// declined to return) is NOT treated as human, because the whole point of the
// merge gate is that an unverifiable answer must never clear a P1 finding.
// The login allowlist is kept as a second, narrower check: a bot that somehow
// reports itself as a User still cannot pass.
func isHumanActor(typename, login string) bool {
	if typename != "User" {
		return false
	}
	return !isKnownBotAuthor(login)
}

// verifyPRIdentity confirms the GraphQL response actually describes the pull
// request that was asked for.
//
// GitHub's pullRequest(number:) field is nullable. A number that does not exist
// returns null, which unmarshals into a zero-valued response and looks exactly
// like a real PR with no review threads. Without this check `mergeloop threads`
// reported zero threads and PASS for every gate on a mistyped target, which is
// the worst possible answer to "would this merge be refused".
func verifyPRIdentity(got, want int, owner, name string) error {
	if got == want {
		return nil
	}
	if got == 0 {
		return fmt.Errorf("pull request #%d not found in %s/%s", want, owner, name)
	}
	return fmt.Errorf("asked for pull request #%d in %s/%s but the API described #%d",
		want, owner, name, got)
}

// partitionResolvable splits fetched threads into the ones the loop may
// auto-resolve and a count of bot threads deliberately withheld.
//
// A thread is resolvable only when ALL of these hold:
//   - it is currently unresolved (nothing else to do otherwise),
//   - its comment list is complete (a truncated thread might hide a human reply
//     past the page boundary, MLC-05),
//   - every comment is from an allowlisted bot (MLC-05: a human reply anywhere
//     in the thread makes it human feedback),
//   - and every comment carries a RECOGNISED ADVISORY severity marker.
//
// That last condition is ce-lr7j. Resolving by author identity alone released
// required_review_thread_resolution over 30 P1 findings. Anything blocking or
// unrecognised is withheld, which leaves the thread open and lets GitHub's own
// gate keep blocking the merge.
func partitionResolvable(threads []reviewThread) ([]botThread, int) {
	var resolvable []botThread
	withheld := 0
	for _, t := range threads {
		switch threadResolvability(t) {
		case resolvabilityEligible:
			resolvable = append(resolvable, botThread{id: t.id, author: t.comments[0].author})
		case resolvabilityWithheld:
			withheld++
		case resolvabilityNotOurs:
			// Human thread, already resolved, empty, or unreadable: never
			// touched, and never counted as withheld.
		}
	}
	return resolvable, withheld
}

// resolvability is the verdict on whether one thread may be auto-resolved.
type resolvability int

const (
	// resolvabilityNotOurs means this gate has no business touching the thread.
	resolvabilityNotOurs resolvability = iota
	// resolvabilityWithheld means it is a bot thread deliberately left open.
	resolvabilityWithheld
	// resolvabilityEligible means every comment is an allowlisted bot carrying
	// a recognised advisory marker.
	resolvabilityEligible
)

// threadResolvability is the single place the auto-resolve rule lives, so the
// decision made when listing and the decision re-made immediately before the
// mutation cannot drift apart.
func threadResolvability(t reviewThread) resolvability {
	if t.isResolved || len(t.comments) == 0 || t.truncated {
		return resolvabilityNotOurs
	}
	if !allCommentsFromKnownBots(t.logins()) {
		return resolvabilityNotOurs
	}
	if mergeloop.ThreadSeverityOf(t.bodies()).BlocksResolution() {
		return resolvabilityWithheld
	}
	return resolvabilityEligible
}

// blockingFindingsIn returns every bot finding that must stop a merge.
//
// This is the independent half of the fix and it deliberately ignores
// isResolved. A blocking finding that was auto-resolved is exactly the case
// GitHub's gate can no longer catch, so this looks at resolved threads too.
//
// A thread with any human comment is treated as addressed: a person engaged
// with the finding, and it is not this gate's job to second-guess them.
// Truncated threads are judged on what is visible, which is fail-closed: a
// visible blocking comment blocks even if the rest of the page is unseen.
func blockingFindingsIn(threads []reviewThread) []mergeloop.BlockingFinding {
	var out []mergeloop.BlockingFinding
	for _, t := range threads {
		if len(t.comments) == 0 {
			continue
		}
		// Every path below judges engagement PER COMMENT. There is deliberately
		// no thread-level "a human appears somewhere, skip it" shortcut: that
		// shortcut was ordered ahead of the fail-closed branches, so a human on
		// the visible page could carry a resolved thread past the truncation
		// refusal and past the unknown-severity refusal alike.

		// A recognised blocking bot finding that no person answered afterwards.
		if i, ok := unaddressedBlockingComment(t.comments); ok {
			out = append(out, mergeloop.BlockingFinding{
				ThreadID: t.id,
				Author:   t.comments[i].author,
				Severity: mergeloop.SeverityBlocking,
				Excerpt:  excerptFinding(t.comments[i:]),
			})
			continue
		}

		// Beyond this point only RESOLVED threads can produce a finding. While
		// a thread is unresolved GitHub's own conversation-resolution gate
		// still holds the merge, so flagging ordinary bot prose here too would
		// deadlock every PR. Once resolved, GitHub holds nothing and this gate
		// is the last reader.
		if !t.isResolved {
			continue
		}

		// A resolved thread this gate cannot read in full refuses outright. A
		// blocking marker may sit past the first page, and engagement visible
		// on the page says nothing about the part nobody fetched.
		if t.truncated {
			out = append(out, mergeloop.BlockingFinding{
				ThreadID: t.id,
				Author:   t.comments[0].author,
				Severity: mergeloop.SeverityUnknown,
				Excerpt:  "resolved thread has more comments than one page; severity cannot be established",
			})
			continue
		}

		// A resolved bot finding whose severity this parser does not recognise,
		// and which no person answered afterwards. A future badge format lands
		// here, as does the shape the old severity-blind resolver created.
		if i, ok := unaddressedUnknownBotComment(t.comments); ok {
			out = append(out, mergeloop.BlockingFinding{
				ThreadID: t.id,
				Author:   t.comments[i].author,
				Severity: mergeloop.SeverityUnknown,
				Excerpt:  excerptFinding(t.comments[i:]),
			})
		}
	}
	return out
}

// parseGraphQLTime turns a GitHub ISO-8601 timestamp into a time.Time. An
// empty or unparseable value yields the zero time, which the engagement check
// treats as "cannot prove" and therefore fails closed on.
func parseGraphQLTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return ts
}

// answeredAfter reports whether a real person commented after this finding's
// latest revision.
//
// The comparison is on TIME rather than position. A bot that edits an earlier
// advisory comment into a P1 keeps its original slot in the thread, so a
// positional check read a human who replied before that edit as having engaged
// with a finding that did not exist when they wrote it. Once such a thread is
// resolved, GitHub's conversation gate protects nothing.
//
// Missing timestamps fail CLOSED. If the API did not give this code enough to
// prove the reply came after the finding, it has not been shown to be
// addressed, and an unprovable answer must never clear a blocking finding.
func answeredAfter(comments []threadComment, finding threadComment) bool {
	at := finding.effectiveAt()
	if at.IsZero() {
		return false
	}
	for _, later := range comments {
		if !isHumanActor(later.typename, later.author) {
			continue
		}
		if later.createdAt.After(at) {
			return true
		}
	}
	return false
}

// unaddressedBlockingComment returns the index of the first comment carrying a
// recognised blocking severity that no human answered afterwards.
//
// "Afterwards" is the load-bearing word. A person can only have engaged with a
// finding they could actually see, so a human comment that PRECEDES the finding
// says nothing about it. Bot comments between the two are irrelevant: what
// matters is whether any real person spoke after the finding was posted.
func unaddressedBlockingComment(comments []threadComment) (int, bool) {
	return unaddressedBotComment(comments, mergeloop.SeverityBlocking)
}

// unaddressedUnknownBotComment is the same test for a bot finding whose
// severity this parser cannot read. It is applied only to RESOLVED threads,
// where GitHub's conversation gate no longer holds the merge.
func unaddressedUnknownBotComment(comments []threadComment) (int, bool) {
	return unaddressedBotComment(comments, mergeloop.SeverityUnknown)
}

// unaddressedBotComment returns the index of the first allowlisted-bot comment
// at the given severity that no human answered afterwards.
//
// "Afterwards" is the load-bearing word. A person can only have engaged with a
// finding they could actually see, so a human comment that PRECEDES the finding
// says nothing about it. Bot comments between the two are irrelevant.
//
// Any actor that is not a real PERSON can carry a finding this gate owns. That
// is deliberately wider than the auto-resolve allowlist: a renamed or newly
// introduced review bot posting a P1 into an already-resolved thread otherwise
// produced no finding at all, and GitHub's conversation gate was already
// satisfied. A person writing or quoting badge-shaped text is still ordinary
// human feedback, which that gate governs.
func unaddressedBotComment(comments []threadComment, want mergeloop.ThreadSeverity) (int, bool) {
	for i, c := range comments {
		if !isBotFindingAuthor(c.typename, c.author) {
			continue
		}
		if mergeloop.ClassifyCommentSeverity(c.body) != want {
			continue
		}
		if !answeredAfter(comments, c) {
			return i, true
		}
	}
	return 0, false
}

// excerptFinding pulls a short human-readable title out of a bot finding so the
// audit record says what is blocking rather than just that something is.
func excerptFinding(comments []threadComment) string {
	// Every caller passes the slice STARTING at the finding it selected, so the
	// text to quote is in these comments regardless of how they classify.
	// Filtering to SeverityBlocking here meant the resolved-unknown path, which
	// exists precisely for findings this parser cannot classify, always
	// excerpted as "(no excerpt)": the durable escalation described nothing and
	// gave an operator no way to act on the refusal.
	for _, c := range comments {
		// Codex puts the finding title in bold after the badge; Gemini starts
		// its prose on the line after. Take the first non-empty line with the
		// markdown noise stripped.
		for line := range strings.SplitSeq(c.body, "\n") {
			line = strings.TrimSpace(badgeNoise.ReplaceAllString(line, ""))
			line = strings.Trim(line, "*_ ")
			if len(line) > 8 {
				// Truncate by rune, not byte: a bot finding can be non-ASCII
				// and a byte slice would sever a multi-byte rune, putting
				// invalid UTF-8 into the audit record.
				if runes := []rune(line); len(runes) > 120 {
					line = string(runes[:120]) + "..."
				}
				return line
			}
		}
	}
	return "(no excerpt)"
}

// badgeNoise strips inline badge images so an excerpt reads as prose.
var badgeNoise = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)|</?sub>`)

// ResolveBotThreads resolves every unresolved review thread on the PR that is
// bot-authored AND carries a recognised advisory severity marker, returning
// what it did. Human threads, blocking findings, and threads whose severity
// this code does not recognise are all left alone.
func (r *ghThreadResolver) ResolveBotThreads(ctx context.Context, repo string, pr int) (mergeloop.ThreadResolution, error) {
	owner, name, ok := splitOwnerRepo(repo)
	if !ok {
		return mergeloop.ThreadResolution{}, fmt.Errorf("invalid repo %q (want owner/name)", repo)
	}
	threads, err := r.listThreads(ctx, owner, name, pr)
	if err != nil {
		return mergeloop.ThreadResolution{}, err
	}
	resolvable, withheld := partitionResolvable(threads)
	out := mergeloop.ThreadResolution{Withheld: withheld}
	for _, t := range resolvable {
		if r.dryRun {
			fmt.Printf("  [dry-run] would resolve advisory thread %s by %s on PR #%d\n", t.id, t.author, pr)
			out.Resolved++
			continue
		}
		// Re-read and re-decide immediately before mutating. The listing above
		// is a snapshot, and a thread that was eligible when it was taken may
		// not be eligible now.
		current, err := r.fetchThread(ctx, t.id)
		if err != nil {
			return out, fmt.Errorf("re-reading thread %s by %s before resolving: %w", t.id, t.author, err)
		}
		if v := threadResolvability(current); v != resolvabilityEligible {
			// Someone engaged with it, or it changed into something this gate
			// must not touch. Leave it open and record it as withheld so the
			// decision is visible rather than silent.
			out.Withheld++
			continue
		}
		if err := r.resolveThread(ctx, t.id); err != nil {
			return out, fmt.Errorf("resolving thread %s by %s: %w", t.id, t.author, err)
		}
		emitThreadResolutionEvent(pr, t.id, t.author)
		out.Resolved++
	}
	return out, nil
}

// BlockingFindings re-queries the PR and reports bot findings that must stop a
// merge. It shares no state with ResolveBotThreads: the fetch is fresh and the
// verdict is recomputed, so a resolver bug cannot suppress it.
func (r *ghThreadResolver) BlockingFindings(ctx context.Context, repo string, pr int) ([]mergeloop.BlockingFinding, error) {
	owner, name, ok := splitOwnerRepo(repo)
	if !ok {
		return nil, fmt.Errorf("invalid repo %q (want owner/name)", repo)
	}
	threads, err := r.listThreads(ctx, owner, name, pr)
	if err != nil {
		return nil, err
	}
	return blockingFindingsIn(threads), nil
}

// listThreads pages through every review thread on the PR, resolved or not,
// with each comment's author and body.
func (r *ghThreadResolver) listThreads(ctx context.Context, owner, name string, pr int) ([]reviewThread, error) {
	var out []reviewThread
	cursor := ""
	for {
		args := []string{"api", "graphql",
			"-f", "owner=" + owner,
			"-f", "repo=" + name,
			"-F", "pr=" + strconv.Itoa(pr),
			"-f", "query=" + threadsListQuery,
		}
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		raw, err := ghJSON(ctx, 30*time.Second, args)
		if err != nil {
			return nil, fmt.Errorf("listing review threads: %w", err)
		}
		var resp struct {
			Data struct {
				Repository struct {
					PullRequest struct {
						// Number is fetched purely as an identity probe:
						// pullRequest(number:) is nullable, so a PR that does
						// not exist decodes to the zero value and is otherwise
						// indistinguishable from a real PR with no threads.
						Number        int `json:"number"`
						ReviewThreads struct {
							PageInfo struct {
								HasNextPage bool   `json:"hasNextPage"`
								EndCursor   string `json:"endCursor"`
							} `json:"pageInfo"`
							Nodes []struct {
								ID         string `json:"id"`
								IsResolved bool   `json:"isResolved"`
								Comments   struct {
									PageInfo struct {
										HasNextPage bool `json:"hasNextPage"`
									} `json:"pageInfo"`
									Nodes []struct {
										Author struct {
											Login    string `json:"login"`
											Typename string `json:"__typename"`
										} `json:"author"`
										Body         string `json:"body"`
										CreatedAt    string `json:"createdAt"`
										LastEditedAt string `json:"lastEditedAt"`
									} `json:"nodes"`
								} `json:"comments"`
							} `json:"nodes"`
						} `json:"reviewThreads"`
					} `json:"pullRequest"`
				} `json:"repository"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("parsing review threads: %w", err)
		}
		if err := verifyPRIdentity(resp.Data.Repository.PullRequest.Number, pr, owner, name); err != nil {
			return nil, err
		}
		rt := resp.Data.Repository.PullRequest.ReviewThreads
		for _, n := range rt.Nodes {
			t := reviewThread{
				id:         n.ID,
				isResolved: n.IsResolved,
				truncated:  n.Comments.PageInfo.HasNextPage,
			}
			for _, c := range n.Comments.Nodes {
				t.comments = append(t.comments, threadComment{
					author:       c.Author.Login,
					body:         c.Body,
					typename:     c.Author.Typename,
					createdAt:    parseGraphQLTime(c.CreatedAt),
					lastEditedAt: parseGraphQLTime(c.LastEditedAt),
				})
			}
			out = append(out, t)
		}
		if !rt.PageInfo.HasNextPage {
			break
		}
		// The cursor must actually ADVANCE. GitHub can report hasNextPage with
		// an empty or repeated endCursor when the thread list changes underneath
		// a paginated read, and this loop would then refetch the same page
		// forever. The 30s timeout bounds each gh invocation, not the loop, so a
		// daemon tick would stop processing every later PR without ever failing.
		next := rt.PageInfo.EndCursor
		if next == "" || next == cursor {
			return nil, fmt.Errorf(
				"listing review threads for PR #%d: pagination stalled (hasNextPage with cursor %q)", pr, next)
		}
		cursor = next
	}
	return out, nil
}

// fetchThread re-reads a single review thread by node ID.
//
// A truncated comment list is reported as such rather than silently trimmed:
// threadResolvability refuses to auto-resolve a thread it cannot read in full,
// so an over-long thread fails closed here exactly as it does in the listing.
func (r *ghThreadResolver) fetchThread(ctx context.Context, threadID string) (reviewThread, error) {
	raw, err := ghJSON(ctx, 30*time.Second, []string{
		"api", "graphql", "-f", "id=" + threadID, "-f", "query=" + threadByIDQuery,
	})
	if err != nil {
		return reviewThread{}, fmt.Errorf("re-reading review thread: %w", err)
	}
	var resp struct {
		Data struct {
			Node struct {
				ID         string `json:"id"`
				IsResolved bool   `json:"isResolved"`
				Comments   struct {
					PageInfo struct {
						HasNextPage bool `json:"hasNextPage"`
					} `json:"pageInfo"`
					Nodes []struct {
						Author struct {
							Login    string `json:"login"`
							Typename string `json:"__typename"`
						} `json:"author"`
						Body         string `json:"body"`
						CreatedAt    string `json:"createdAt"`
						LastEditedAt string `json:"lastEditedAt"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return reviewThread{}, fmt.Errorf("decoding review thread: %w", err)
	}
	n := resp.Data.Node
	if n.ID != threadID {
		return reviewThread{}, fmt.Errorf("re-read returned thread %q, want %q", n.ID, threadID)
	}
	t := reviewThread{id: n.ID, isResolved: n.IsResolved, truncated: n.Comments.PageInfo.HasNextPage}
	for _, c := range n.Comments.Nodes {
		t.comments = append(t.comments, threadComment{
			author:       c.Author.Login,
			body:         c.Body,
			typename:     c.Author.Typename,
			createdAt:    parseGraphQLTime(c.CreatedAt),
			lastEditedAt: parseGraphQLTime(c.LastEditedAt),
		})
	}
	return t, nil
}

// resolveThread resolves one review thread by its node ID.
func (r *ghThreadResolver) resolveThread(ctx context.Context, threadID string) error {
	_, err := ghJSON(ctx, 30*time.Second, []string{"api", "graphql",
		"-f", "threadId=" + threadID,
		"-f", "query=" + threadResolveMutation,
	})
	return err
}

// splitOwnerRepo splits an "owner/name" repo string. The second return is false
// when the input is not in that form.
func splitOwnerRepo(repo string) (owner, name string, ok bool) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ThreadResolutionEvent is one structured audit line emitted per auto-resolved
// bot thread, written to the merge-loop audit log alongside the driver's
// aggregate "bot_threads_resolved" event so an operator can reconstruct exactly
// which thread was resolved, when, and for which bot.
type ThreadResolutionEvent struct {
	Kind      string `json:"kind"`
	Timestamp string `json:"timestamp"`
	PR        int    `json:"pr"`
	ThreadID  string `json:"thread_id"`
	BotAuthor string `json:"bot_author"`
}

// emitThreadResolutionEvent appends one ThreadResolutionEvent to the merge-loop
// audit JSONL. It is best-effort: an audit-log failure must never block a merge,
// so errors are swallowed (mirroring mergeloop's appendAudit convention).
func emitThreadResolutionEvent(pr int, threadID, botAuthor string) {
	ev := ThreadResolutionEvent{
		Kind:      "thread.auto-resolved",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		PR:        pr,
		ThreadID:  threadID,
		BotAuthor: botAuthor,
	}
	dir := mergeloop.StateDir()
	if d := os.Getenv("MERGELOOP_AUDIT_DIR"); d != "" {
		dir = d
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "mergeloop-audit.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "mergeloop: failed to close audit log: %v\n", cerr)
		}
	}()
	if data, err := json.Marshal(ev); err == nil {
		if _, werr := f.Write(append(data, '\n')); werr != nil {
			fmt.Fprintf(os.Stderr, "mergeloop: failed to write audit log entry: %v\n", werr)
		}
	}
}
