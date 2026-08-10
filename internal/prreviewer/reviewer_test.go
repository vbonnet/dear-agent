package prreviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

type call struct {
	name  string
	args  []string
	stdin string
}

type fakeRunner struct {
	calls     []call
	responses map[string]string
	errors    map[string]error
}

func (f *fakeRunner) Run(ctx context.Context, name string, args []string, stdin string) (string, error) {
	f.calls = append(f.calls, call{name: name, args: append([]string(nil), args...), stdin: stdin})
	key := name + " " + strings.Join(args, " ")
	if err := f.errors[key]; err != nil {
		return "", err
	}
	return f.responses[key], nil
}

func TestRunOnceReviewsOnlyNewHeadAndPostsComment(t *testing.T) {
	prs := []PR{{Number: 7, Title: "Fix nil panic", HeadRefOID: "abc123", UpdatedAt: time.Now()}}
	prJSON, _ := json.Marshal(prs)
	r := &fakeRunner{
		responses: map[string]string{
			"gh pr list --repo owner/repo --state open --limit 50 --json number,title,headRefOid,updatedAt,isDraft": string(prJSON),
			"gh pr diff 7 --repo owner/repo": "diff --git a/x.go b/x.go",
			"codex exec -":                   "Codex finding",
			"agy run -":                      "Gemini finding",
			"gh pr review 7 --repo owner/repo -b Automated external review for `owner/repo` PR #7 at 2026-08-09T00:00:00Z.\n\n## Codex\n\nCodex finding\n\n## Gemini\n\nGemini finding\n --comment": "",
		},
		errors: map[string]error{},
	}
	dir := t.TempDir()
	var out strings.Builder
	results, err := RunOnce(context.Background(), Config{
		Repos:     []string{"owner/repo"},
		StatePath: dir + "/state.json",
		Now:       func() time.Time { return time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC) },
	}, r, &out)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(results) != 1 || !results[0].Posted {
		t.Fatalf("expected one posted result, got %#v", results)
	}
	if got := r.calls[len(r.calls)-1]; got.name != "gh" || !containsArg(got.args, "--comment") {
		t.Fatalf("expected gh pr review --comment, got %#v", got)
	}

	r.calls = nil
	results, err = RunOnce(context.Background(), Config{Repos: []string{"owner/repo"}, StatePath: dir + "/state.json"}, r, &out)
	if err != nil {
		t.Fatalf("second RunOnce() error = %v", err)
	}
	if len(results) != 1 || !results[0].Skipped || results[0].Reason != "already-reviewed" {
		t.Fatalf("expected already-reviewed skip, got %#v", results)
	}
	for _, c := range r.calls {
		if c.name == "codex" || c.name == "agy" {
			t.Fatalf("provider should not run for unchanged head: %#v", r.calls)
		}
	}
}

func TestRunOnceToleratesGeminiFailure(t *testing.T) {
	prs := []PR{{Number: 9, Title: "Change API", HeadRefOID: "def456"}}
	prJSON, _ := json.Marshal(prs)
	r := &fakeRunner{
		responses: map[string]string{
			"gh pr list --repo owner/repo --state open --limit 50 --json number,title,headRefOid,updatedAt,isDraft": string(prJSON),
			"gh pr diff 9 --repo owner/repo": "diff",
			"codex exec -":                   "Codex ok",
			"gh pr review 9 --repo owner/repo -b Automated external review for `owner/repo` PR #9 at 2026-08-09T00:00:00Z.\n\n## Codex\n\nCodex ok\n\n## Gemini\n\nGemini review unavailable after retry; skipped best-effort secondary review. Last error: `agy failed`.\n --comment": "",
		},
		errors: map[string]error{"agy run -": fmt.Errorf("agy failed")},
	}
	var out strings.Builder
	results, err := RunOnce(context.Background(), Config{
		Repos:     []string{"owner/repo"},
		StatePath: t.TempDir() + "/state.json",
		Now:       func() time.Time { return time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC) },
	}, r, &out)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(results) != 1 || !results[0].Posted {
		t.Fatalf("expected posted result despite Gemini error, got %#v", results)
	}
	var geminiCalls int
	for _, c := range r.calls {
		if c.name == "agy" {
			geminiCalls++
		}
	}
	if geminiCalls != 2 {
		t.Fatalf("expected 2 Gemini attempts, got %d calls: %#v", geminiCalls, r.calls)
	}
}

func TestRunOnceDryRunDoesNotPostOrPersist(t *testing.T) {
	prs := []PR{{Number: 3, Title: "Docs", HeadRefOID: "abc"}}
	prJSON, _ := json.Marshal(prs)
	r := &fakeRunner{
		responses: map[string]string{
			"gh pr list --repo owner/repo --state open --limit 50 --json number,title,headRefOid,updatedAt,isDraft": string(prJSON),
			"gh pr diff 3 --repo owner/repo": "diff",
			"codex exec -":                   "Codex ok",
			"agy run -":                      "Gemini ok",
		},
		errors: map[string]error{},
	}
	var out strings.Builder
	_, err := RunOnce(context.Background(), Config{Repos: []string{"owner/repo"}, DryRun: true, StatePath: t.TempDir() + "/state.json"}, r, &out)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	for _, c := range r.calls {
		if c.name == "gh" && len(c.args) >= 2 && c.args[0] == "pr" && c.args[1] == "review" {
			t.Fatalf("dry-run must not post review: %#v", r.calls)
		}
	}
	if !strings.Contains(out.String(), "dry-run would post COMMENT review") {
		t.Fatalf("missing dry-run output: %q", out.String())
	}
}

func containsArg(args []string, want string) bool {
	return slices.Contains(args, want)
}
