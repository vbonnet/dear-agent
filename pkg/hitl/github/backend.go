// Package github implements a HITL backend that surfaces approval requests as
// GitHub PR comments and accepts reply comments as decisions.
//
// Design:
//   - Request renders the approval as a PR comment embedding the approvalID as
//     a machine-readable marker, then records the comment-id for correlation.
//   - Wait polls the PR's issue comments on a configurable interval until a
//     comment after the request comment is found that begins with "approve" or
//     "reject" (same vocabulary as the Discord backend).
//
// The backend is wire-compatible with workflow.HITLBackend so callers can
// substitute it for the Discord or SQLite defaults without code changes.
//
// The narrow Commenter interface allows tests to stub the HTTP layer.
package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// DefaultPollInterval is how often Wait polls for a new comment.
const DefaultPollInterval = 15 * time.Second

// Commenter abstracts the two GitHub API calls the backend needs.
// Production code wraps an http.Client; tests substitute an in-memory fake.
type Commenter interface {
	// PostComment creates a comment on the given PR (issue_number) and
	// returns the new comment's ID.
	PostComment(ctx context.Context, owner, repo string, prNumber int, body string) (commentID int64, err error)

	// ListComments returns the comments on the given PR posted after sinceID.
	// sinceID == 0 means "all comments".
	ListComments(ctx context.Context, owner, repo string, prNumber int, sinceID int64) ([]Comment, error)
}

// Comment is a minimal GitHub PR comment representation.
type Comment struct {
	ID        int64
	Body      string
	UserLogin string
	CreatedAt time.Time
}

// Backend is the GitHub PR HITL backend. It is safe for concurrent use.
type Backend struct {
	client   Commenter
	owner    string
	repo     string
	prNumber int

	// PollInterval controls how often Wait polls. Zero uses DefaultPollInterval.
	PollInterval time.Duration

	mu      sync.Mutex
	pending map[string]*pendingApproval // keyed by approvalID
}

type pendingApproval struct {
	commentID int64               // id of our Request comment
	ch        chan workflow.HITLResolution
}

// NewBackend constructs a Backend against the given PR.
func NewBackend(client Commenter, owner, repo string, prNumber int) *Backend {
	return &Backend{
		client:   client,
		owner:    owner,
		repo:     repo,
		prNumber: prNumber,
		pending:  map[string]*pendingApproval{},
	}
}

// Request posts an approval request as a PR comment.
func (b *Backend) Request(ctx context.Context, req workflow.HITLRequest) error {
	if b.client == nil {
		return errors.New("github HITL backend: client not configured")
	}
	body := renderRequest(req)
	commentID, err := b.client.PostComment(ctx, b.owner, b.repo, b.prNumber, body)
	if err != nil {
		return fmt.Errorf("github HITL: post comment: %w", err)
	}
	b.mu.Lock()
	if _, ok := b.pending[req.ApprovalID]; !ok {
		b.pending[req.ApprovalID] = &pendingApproval{
			commentID: commentID,
			ch:        make(chan workflow.HITLResolution, 1),
		}
	} else {
		b.pending[req.ApprovalID].commentID = commentID
	}
	b.mu.Unlock()
	return nil
}

// Wait polls for a reply comment that contains an approve/reject decision.
func (b *Backend) Wait(ctx context.Context, approvalID string) (workflow.HITLResolution, error) {
	b.mu.Lock()
	pa, ok := b.pending[approvalID]
	if !ok {
		pa = &pendingApproval{ch: make(chan workflow.HITLResolution, 1)}
		b.pending[approvalID] = pa
	}
	b.mu.Unlock()

	// If another goroutine already resolved it (e.g. Request returned late
	// and the reply came in immediately), drain the buffered channel.
	select {
	case res := <-pa.ch:
		return res, nil
	default:
	}

	interval := b.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Track the highest comment-id we've seen to avoid re-processing.
	var sinceID int64
	b.mu.Lock()
	sinceID = pa.commentID // start scanning after our own comment
	b.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return workflow.HITLResolution{}, ctx.Err()
		case res := <-pa.ch:
			return res, nil
		case <-ticker.C:
			comments, err := b.client.ListComments(ctx, b.owner, b.repo, b.prNumber, sinceID)
			if err != nil {
				continue // transient; keep polling
			}
			for _, c := range comments {
				if c.ID <= sinceID {
					continue
				}
				sinceID = c.ID
				dec, ok := parseDecision(c.Body)
				if !ok {
					continue
				}
				res := workflow.HITLResolution{
					ApprovalID: approvalID,
					Decision:   dec,
					Approver:   c.UserLogin,
					Reason:     c.Body,
					ResolvedAt: c.CreatedAt,
				}
				select {
				case pa.ch <- res:
				default:
				}
			}
		}
	}
}

// renderRequest formats the human-facing PR comment.
// The approvalID marker is embedded so automated tooling can correlate it.
func renderRequest(r workflow.HITLRequest) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**HITL approval needed** (approval-id: `%s`)\n\n", r.ApprovalID)
	fmt.Fprintf(&sb, "**Workflow:** `%s` — node `%s`\n", r.WorkflowName, r.NodeID)
	if r.ApproverRole != "" {
		fmt.Fprintf(&sb, "**Required role:** `%s`\n", r.ApproverRole)
	}
	if r.Reason != "" {
		fmt.Fprintf(&sb, "**Reason:** %s\n", r.Reason)
	}
	if r.Confidence > 0 {
		fmt.Fprintf(&sb, "**Reported confidence:** %.2f\n", r.Confidence)
	}
	if r.NodeOutput != "" {
		preview := r.NodeOutput
		if len(preview) > 500 {
			preview = preview[:500] + "…"
		}
		fmt.Fprintf(&sb, "\n<details><summary>Output preview</summary>\n\n```\n%s\n```\n\n</details>\n", preview)
	}
	sb.WriteString("\nReply `approve` (or `lgtm`) to proceed, or `reject <reason>` (or `deny`) to block.")
	return sb.String()
}

// parseDecision returns the HITLDecision encoded in the first word of body.
func parseDecision(body string) (workflow.HITLDecision, bool) {
	first := strings.ToLower(strings.TrimSpace(body))
	if i := strings.IndexAny(first, " \n\t"); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSuffix(first, "!")
	switch first {
	case "approve", "approved", "lgtm", "ok", "yes", "y":
		return workflow.HITLDecisionApprove, true
	case "reject", "rejected", "deny", "denied", "no", "n":
		return workflow.HITLDecisionReject, true
	}
	return "", false
}

// HTTPCommenter is the production Commenter backed by the GitHub REST API.
// It authenticates with a Bearer token.
type HTTPCommenter struct {
	Token      string
	HTTPClient *http.Client
}

// NewHTTPCommenter returns a Commenter that calls api.github.com.
// If token is empty the commenter is unauthenticated (rate-limited to 60 req/h).
func NewHTTPCommenter(token string) *HTTPCommenter {
	return &HTTPCommenter{Token: token, HTTPClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *HTTPCommenter) do(ctx context.Context, method, url string, body any) (*http.Response, error) {
	var reqBody strings.Builder
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody.Write(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(reqBody.String()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTPClient.Do(req)
}

// PostComment implements Commenter.
func (c *HTTPCommenter) PostComment(ctx context.Context, owner, repo string, prNumber int, body string) (int64, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, prNumber)
	resp, err := c.do(ctx, http.MethodPost, url, map[string]string{"body": body})
	if err != nil {
		return 0, fmt.Errorf("github: post comment: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("github: post comment: HTTP %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("github: decode comment response: %w", err)
	}
	return out.ID, nil
}

// ListComments implements Commenter.
func (c *HTTPCommenter) ListComments(ctx context.Context, owner, repo string, prNumber int, sinceID int64) ([]Comment, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments?per_page=100", owner, repo, prNumber)
	resp, err := c.do(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("github: list comments: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github: list comments: HTTP %d", resp.StatusCode)
	}
	var raw []struct {
		ID        int64     `json:"id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("github: decode comments: %w", err)
	}
	var out []Comment
	for _, r := range raw {
		if r.ID <= sinceID {
			continue
		}
		out = append(out, Comment{
			ID:        r.ID,
			Body:      r.Body,
			UserLogin: r.User.Login,
			CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}
