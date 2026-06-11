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
//	resolve-review-threads list        <owner> <repo> <pr>           # unresolved threads (JSON lines)
//	resolve-review-threads list-all    <owner> <repo> <pr>           # every thread
//	resolve-review-threads resolve     <threadId>                    # one thread by ID
//	resolve-review-threads resolve-all <owner> <repo> <pr> [author]  # all unresolved, optional author filter
//	resolve-review-threads unresolve   <threadId>                    # re-open a thread
//
// All GitHub calls go through `gh api graphql`, so authentication uses the gh
// CLI's token (no git push, no keychain prompt). Requires gh (authenticated).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
          comments(first:1) { nodes { author { login } body } }
        }
      }
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

// thread is the flattened view of a PullRequestReviewThread that we emit.
type thread struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	Author     string `json:"author"`
	Body       string `json:"body"`
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
func cmdMutate(ctx context.Context, cmd string, rest []string) int {
	if len(rest) != 1 {
		return fail("usage: %s <threadId>", cmd)
	}
	msg, err := mutateThread(ctx, cmd, rest[0])
	if err != nil {
		return fail("%v", err)
	}
	fmt.Println(msg)
	return 0
}

// cmdResolveAll resolves every unresolved thread on a PR, optionally limited to
// a single comment author.
func cmdResolveAll(ctx context.Context, rest []string) int {
	if len(rest) < 3 || len(rest) > 4 {
		return fail("usage: resolve-all <owner> <repo> <pr> [author]")
	}
	pr, err := strconv.Atoi(rest[2])
	if err != nil {
		return fail("pr must be an integer, got %q", rest[2])
	}
	author := ""
	if len(rest) == 4 {
		author = rest[3]
	}
	threads, err := listThreads(ctx, rest[0], rest[1], pr)
	if err != nil {
		return fail("%v", err)
	}
	n := 0
	for _, t := range filterThreads(threads, author) {
		msg, err := mutateThread(ctx, "resolve", t.ID)
		if err != nil {
			return fail("%v", err)
		}
		fmt.Println(msg)
		n++
	}
	fmt.Printf("resolved %d thread(s)\n", n)
	return 0
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
								IsOutdated bool   `json:"isOutdated"`
								Path       string `json:"path"`
								Comments   struct {
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
			return nil, fmt.Errorf("parse reviewThreads response: %w", err)
		}

		rt := resp.Data.Repository.PullRequest.ReviewThreads
		for _, n := range rt.Nodes {
			t := thread{ID: n.ID, IsResolved: n.IsResolved, IsOutdated: n.IsOutdated, Path: n.Path, Author: "unknown"}
			if len(n.Comments.Nodes) > 0 {
				c := n.Comments.Nodes[0]
				if c.Author.Login != "" {
					t.Author = c.Author.Login
				}
				t.Body = cleanBody(c.Body)
			}
			all = append(all, t)
		}
		if !rt.PageInfo.HasNextPage {
			break
		}
		cursor = rt.PageInfo.EndCursor
	}
	return all, nil
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

func fail(format string, a ...any) int {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	return 1
}

func usage() {
	fmt.Fprint(os.Stderr, `resolve-review-threads — list and resolve GitHub PR review threads

usage:
  resolve-review-threads list        <owner> <repo> <pr>           unresolved threads (JSON lines)
  resolve-review-threads list-all    <owner> <repo> <pr>           every thread
  resolve-review-threads resolve     <threadId>                    resolve one thread by ID
  resolve-review-threads unresolve   <threadId>                    re-open one thread by ID
  resolve-review-threads resolve-all <owner> <repo> <pr> [author]  resolve all unresolved
                                                                   (optional author filter,
                                                                    e.g. gemini-code-assist)

Resolution is GraphQL-only; all calls go through an authenticated gh CLI.
`)
}
