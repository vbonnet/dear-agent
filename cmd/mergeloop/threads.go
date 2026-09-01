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
      reviewThreads(first:100,after:$after){
        pageInfo{ hasNextPage endCursor }
        nodes{
          id
          isResolved
          comments(first:100){ pageInfo{ hasNextPage } nodes{ author{ login } body } }
        }
      }
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
		if !isKnownBotAuthor(c.author) {
			return true
		}
	}
	return false
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
		if t.isResolved || len(t.comments) == 0 || t.truncated {
			continue
		}
		if !allCommentsFromKnownBots(t.logins()) {
			continue // human thread: never touched, and never counted as withheld
		}
		if mergeloop.ThreadSeverityOf(t.bodies()).BlocksResolution() {
			withheld++
			continue
		}
		resolvable = append(resolvable, botThread{id: t.id, author: t.comments[0].author})
	}
	return resolvable, withheld
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
		if len(t.comments) == 0 || t.hasHumanComment() {
			continue
		}
		sev := mergeloop.ThreadSeverityOf(t.bodies())
		if sev != mergeloop.SeverityBlocking {
			// Unknown severity on an unresolved thread is already blocked by
			// GitHub. Flagging it here too would deadlock every PR carrying
			// ordinary bot prose.
			continue
		}
		out = append(out, mergeloop.BlockingFinding{
			ThreadID: t.id,
			Author:   t.comments[0].author,
			Severity: sev,
			Excerpt:  excerptFinding(t.comments),
		})
	}
	return out
}

// excerptFinding pulls a short human-readable title out of a bot finding so the
// audit record says what is blocking rather than just that something is.
func excerptFinding(comments []threadComment) string {
	for _, c := range comments {
		if mergeloop.ClassifyCommentSeverity(c.body) != mergeloop.SeverityBlocking {
			continue
		}
		// Codex puts the finding title in bold after the badge; Gemini starts
		// its prose on the line after. Take the first non-empty line with the
		// markdown noise stripped.
		for line := range strings.SplitSeq(c.body, "\n") {
			line = strings.TrimSpace(badgeNoise.ReplaceAllString(line, ""))
			line = strings.Trim(line, "*_ ")
			if len(line) > 8 {
				if len(line) > 120 {
					line = line[:120] + "..."
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
											Login string `json:"login"`
										} `json:"author"`
										Body string `json:"body"`
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
		rt := resp.Data.Repository.PullRequest.ReviewThreads
		for _, n := range rt.Nodes {
			t := reviewThread{
				id:         n.ID,
				isResolved: n.IsResolved,
				truncated:  n.Comments.PageInfo.HasNextPage,
			}
			for _, c := range n.Comments.Nodes {
				t.comments = append(t.comments, threadComment{author: c.Author.Login, body: c.Body})
			}
			out = append(out, t)
		}
		if !rt.PageInfo.HasNextPage {
			break
		}
		cursor = rt.PageInfo.EndCursor
	}
	return out, nil
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
