package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
          comments(first:100){ pageInfo{ hasNextPage } nodes{ author{ login } } }
        }
      }
    }
  }
}`

const threadResolveMutation = `mutation($threadId:ID!){
  resolveReviewThread(input:{threadId:$threadId}){ thread{ id isResolved } }
}`

// botThread is a single unresolved review thread attributed to a known bot.
type botThread struct {
	id     string
	author string
}

// ResolveBotThreads resolves every unresolved review thread authored by a known
// bot on the PR and returns the number resolved. Human threads are left alone.
// In dry-run it reports what it would resolve without mutating anything.
func (r *ghThreadResolver) ResolveBotThreads(ctx context.Context, repo string, pr int) (int, error) {
	owner, name, ok := splitOwnerRepo(repo)
	if !ok {
		return 0, fmt.Errorf("invalid repo %q (want owner/name)", repo)
	}
	threads, err := r.listBotThreads(ctx, owner, name, pr)
	if err != nil {
		return 0, err
	}
	resolved := 0
	for _, t := range threads {
		if r.dryRun {
			fmt.Printf("  [dry-run] would resolve thread %s by %s on PR #%d\n", t.id, t.author, pr)
			resolved++
			continue
		}
		if err := r.resolveThread(ctx, t.id); err != nil {
			// Surface the partial count so the caller can audit progress.
			return resolved, fmt.Errorf("resolving thread %s by %s: %w", t.id, t.author, err)
		}
		emitThreadResolutionEvent(pr, t.id, t.author)
		resolved++
	}
	return resolved, nil
}

// listBotThreads pages through the PR's review threads and returns the
// unresolved ones where every comment — not just the first — is authored by
// a known bot. A bot opening a thread that a human later replies to must
// never be auto-resolved (MLC-05); checking only the first comment would miss
// that reply entirely and silently discard human feedback the moment this
// PR's own review-thread finding illustrates it does (ce-hz14 follow-up). A
// thread with more comments than a single page fetches is left unresolved
// rather than risk missing a human reply past the page boundary.
func (r *ghThreadResolver) listBotThreads(ctx context.Context, owner, name string, pr int) ([]botThread, error) {
	var out []botThread
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
			if n.IsResolved || len(n.Comments.Nodes) == 0 || n.Comments.PageInfo.HasNextPage {
				continue
			}
			logins := make([]string, len(n.Comments.Nodes))
			for i, c := range n.Comments.Nodes {
				logins[i] = c.Author.Login
			}
			if !allCommentsFromKnownBots(logins) {
				continue
			}
			out = append(out, botThread{id: n.ID, author: logins[0]})
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
