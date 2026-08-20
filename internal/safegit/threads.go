// Package safegit threads.go owns PR review-thread listing and the
// unresolved-thread gate shared by safe-merge and the pr-blockers classifier.
// Unresolved threads block regardless of isOutdated: GitHub's required
// conversation resolution counts outdated threads even though the UI
// collapses them (retro 2026-08-18-pr-merge-blocker-guessing).
package safegit

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const reviewThreadsQuery = `
query($owner: String!, $name: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        nodes {
          id
          isResolved
          isOutdated
          path
          comments(first: 1) {
            nodes {
              author { login }
              body
            }
          }
        }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

type gqlReviewThread struct {
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
}

type gqlReviewResult struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes    []gqlReviewThread `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// ReviewThread is one PR review thread in the shape the merge-blocker
// classifier consumes. IsOutdated matters: GitHub's required conversation
// resolution counts unresolved outdated threads even though the UI collapses
// them and casual queries omit them.
type ReviewThread struct {
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	Author     string `json:"author"`
	Body       string `json:"body"`
}

// ListReviewThreads pages through every review thread on the PR, outdated
// threads included.
func ListReviewThreads(ctx context.Context, prNum int, repo string) ([]ReviewThread, error) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo %q", repo)
	}
	owner, name := parts[0], parts[1]

	var threads []ReviewThread
	cursor := ""
	for {
		args := []string{
			"api", "graphql",
			"--field", fmt.Sprintf("owner=%s", owner),
			"--field", fmt.Sprintf("name=%s", name),
			"--field", fmt.Sprintf("number=%d", prNum),
			"--field", fmt.Sprintf("query=%s", reviewThreadsQuery),
		}
		if cursor != "" {
			args = append(args, "--field", fmt.Sprintf("cursor=%s", cursor))
		}
		out, err := runCommand(exec.CommandContext(ctx, "gh", args...))
		if err != nil {
			return nil, fmt.Errorf("GraphQL query failed: %w", err)
		}
		var result gqlReviewResult
		if err := json.Unmarshal(out, &result); err != nil {
			return nil, fmt.Errorf("parsing GraphQL response: %w", err)
		}
		page := result.Data.Repository.PullRequest.ReviewThreads
		threads = append(threads, flattenThreads(page.Nodes)...)
		if !page.PageInfo.HasNextPage {
			break
		}
		if page.PageInfo.EndCursor == "" || page.PageInfo.EndCursor == cursor {
			return nil, fmt.Errorf("GraphQL review-thread pagination did not advance")
		}
		cursor = page.PageInfo.EndCursor
	}
	return threads, nil
}

func flattenThreads(nodes []gqlReviewThread) []ReviewThread {
	out := make([]ReviewThread, 0, len(nodes))
	for _, n := range nodes {
		t := ReviewThread{ID: n.ID, IsResolved: n.IsResolved, IsOutdated: n.IsOutdated, Path: n.Path}
		if len(n.Comments.Nodes) > 0 {
			t.Author = n.Comments.Nodes[0].Author.Login
			t.Body = n.Comments.Nodes[0].Body
		}
		out = append(out, t)
	}
	return out
}

func checkReviewThreads(ctx context.Context, prNum int, repo string) error {
	threads, err := ListReviewThreads(ctx, prNum, repo)
	if err != nil {
		return err
	}
	return reviewThreadError(threads)
}

// parseReviewThreads returns an error if any unresolved review threads exist
// in the GraphQL response. Exported for testing.
func parseReviewThreads(data []byte) error {
	var result gqlReviewResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parsing GraphQL response: %w", err)
	}

	return reviewThreadError(flattenThreads(result.Data.Repository.PullRequest.ReviewThreads.Nodes))
}

// reviewThreadError blocks on every unresolved thread, outdated or not.
// It used to skip isOutdated threads, but GitHub's required conversation
// resolution counts them: the gate reported "no unresolved review threads"
// while the provider kept the PR BLOCKED, and agents went hunting for phantom
// code problems (retro 2026-08-18-pr-merge-blocker-guessing). Outdated
// threads are labeled so the fix is obvious.
func reviewThreadError(threads []ReviewThread) error {
	var unresolved []string
	outdated := 0
	for i, t := range threads {
		if t.IsResolved {
			continue
		}
		tag := ""
		if t.IsOutdated {
			outdated++
			tag = " (outdated)"
		}
		label := fmt.Sprintf("thread #%d%s", i+1, tag)
		if t.Author != "" || t.Body != "" {
			author := t.Author
			if author == "" {
				author = "unknown"
			}
			body := t.Body
			if len([]rune(body)) > 80 {
				body = string([]rune(body)[:80]) + "…"
			}
			label = fmt.Sprintf("  • @%s%s: %q", author, tag, body)
		}
		unresolved = append(unresolved, label)
	}

	if len(unresolved) > 0 {
		note := ""
		if outdated > 0 {
			note = fmt.Sprintf(" (%d outdated: collapsed in the GitHub UI but still counted by required conversation resolution)", outdated)
		}
		return fmt.Errorf("%d unresolved review thread(s)%s:\n%s",
			len(unresolved), note, strings.Join(unresolved, "\n"))
	}
	return nil
}

// threadRemediationGuidance is safe-merge's advice when the review-thread gate
// rejects a PR. It leads with reply-resolve because resolving a thread asserts
// its point was handled, and a reply is the only record that it was read.
func threadRemediationGuidance(repo string, pr int) string {
	owner, name := repo, repo
	if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 {
		owner, name = parts[0], parts[1]
	}
	return fmt.Sprintf(
		"safe-merge: guidance: address each thread, then close it with its reason:\n"+
			"  resolve-review-threads reply-resolve <threadId> \"Fixed - <what changed>\"\n"+
			"then sweep the answered ones and re-run safe-merge:\n"+
			"  resolve-review-threads resolve-all %s %s %d [author]\n",
		owner, name, pr)
}
