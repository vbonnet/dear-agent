package prreviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func (f *fakeRunner) posted() []call {
	var out []call
	for _, c := range f.calls {
		if c.name == "gh" && len(c.args) >= 2 && c.args[0] == "pr" && c.args[1] == "review" {
			out = append(out, c)
		}
	}
	return out
}

func listKey(repo string) string {
	return "gh pr list --repo " + repo + " --state open --limit 50 --json number,title,headRefOid,updatedAt,isDraft"
}

func listResponse(t *testing.T, prs ...PR) string {
	t.Helper()
	raw, err := json.Marshal(prs)
	if err != nil {
		t.Fatalf("marshal PR list: %v", err)
	}
	return string(raw)
}

func viewKey(repo string, number int) string {
	return fmt.Sprintf("gh pr view %d --repo %s --json headRefOid", number, repo)
}

func viewResponse(sha string) string {
	return fmt.Sprintf(`{"headRefOid":%q}`, sha)
}

func fixedClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC) }
}

func TestRunOnceReviewsOnlyNewHeadAndPostsComment(t *testing.T) {
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/repo"):            listResponse(t, PR{Number: 7, Title: "Fix nil panic", HeadRefOID: "abc123", UpdatedAt: time.Now()}),
			"gh pr diff 7 --repo owner/repo": "diff --git a/x.go b/x.go",
			viewKey("owner/repo", 7):         viewResponse("abc123"),
			"codex exec -":                   "Codex finding",
			"agy run -":                      "Gemini finding",
		},
		errors: map[string]error{},
	}
	dir := t.TempDir()
	var out strings.Builder
	results, err := RunOnce(context.Background(), Config{
		Repos:     []string{"owner/repo"},
		StatePath: dir + "/state.json",
		Now:       fixedClock(),
	}, r, &out)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(results) != 1 || !results[0].Posted {
		t.Fatalf("expected one posted result, got %#v", results)
	}
	review := r.posted()
	if len(review) != 1 || !containsArg(review[0].args, "--comment") {
		t.Fatalf("expected one gh pr review --comment, got %#v", r.calls)
	}
	body := review[0].args[slices.Index(review[0].args, "-b")+1]
	for _, want := range []string{"owner/repo", "PR #7", "2026-08-09T00:00:00Z", "## Codex", "Codex finding", "## Gemini", "Gemini finding"} {
		if !strings.Contains(body, want) {
			t.Fatalf("review body %q missing %q", body, want)
		}
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

func TestRunOnceToleratesGeminiFailureWithoutLeakingProviderError(t *testing.T) {
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/repo"):            listResponse(t, PR{Number: 9, Title: "Change API", HeadRefOID: "def456"}),
			"gh pr diff 9 --repo owner/repo": "diff",
			viewKey("owner/repo", 9):         viewResponse("def456"),
			"codex exec -":                   "Codex ok",
		},
		errors: map[string]error{"agy run -": fmt.Errorf("agy failed: token ghp_secret in /home/op/.config")},
	}
	var out strings.Builder
	results, err := RunOnce(context.Background(), Config{
		Repos:     []string{"owner/repo"},
		StatePath: t.TempDir() + "/state.json",
		Now:       fixedClock(),
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
	review := r.posted()
	if len(review) != 1 {
		t.Fatalf("expected one posted review, got %#v", r.calls)
	}
	body := review[0].args[slices.Index(review[0].args, "-b")+1]
	if strings.Contains(body, "ghp_secret") || strings.Contains(body, "agy failed") {
		t.Fatalf("review body leaked provider error text: %q", body)
	}
	if !strings.Contains(body, "Gemini review unavailable after 2 attempt(s)") {
		t.Fatalf("review body missing unavailability note: %q", body)
	}
	if !strings.Contains(out.String(), "ghp_secret") {
		t.Fatalf("provider failure detail should reach the operator log, got %q", out.String())
	}
}

func TestRunOnceDryRunDoesNotPostOrPersist(t *testing.T) {
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/repo"):            listResponse(t, PR{Number: 3, Title: "Docs", HeadRefOID: "abc"}),
			"gh pr diff 3 --repo owner/repo": "diff",
			"codex exec -":                   "Codex ok",
			"agy run -":                      "Gemini ok",
		},
		errors: map[string]error{},
	}
	state := filepath.Join(t.TempDir(), "state.json")
	var out strings.Builder
	_, err := RunOnce(context.Background(), Config{Repos: []string{"owner/repo"}, DryRun: true, StatePath: state}, r, &out)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(r.posted()) != 0 {
		t.Fatalf("dry-run must not post review: %#v", r.calls)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not persist state: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run would post COMMENT review") {
		t.Fatalf("missing dry-run output: %q", out.String())
	}
}

func TestRunOnceSkipsDraftPullRequests(t *testing.T) {
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/repo"): listResponse(t, PR{Number: 11, Title: "WIP", HeadRefOID: "draft1", IsDraft: true}),
		},
		errors: map[string]error{},
	}
	var out strings.Builder
	results, err := RunOnce(context.Background(), Config{Repos: []string{"owner/repo"}, StatePath: t.TempDir() + "/state.json"}, r, &out)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(results) != 1 || !results[0].Skipped || results[0].Reason != "draft" {
		t.Fatalf("expected draft skip, got %#v", results)
	}
	if len(r.calls) != 1 {
		t.Fatalf("draft PR must not be diffed or reviewed: %#v", r.calls)
	}
}

func TestRunOnceSkipsWhenHeadMovedDuringReview(t *testing.T) {
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/repo"):             listResponse(t, PR{Number: 17, Title: "Moving target", HeadRefOID: "old"}),
			"gh pr diff 17 --repo owner/repo": "diff",
			viewKey("owner/repo", 17):         viewResponse("new"),
			"codex exec -":                    "Codex ok",
			"agy run -":                       "Gemini ok",
		},
		errors: map[string]error{},
	}
	state := filepath.Join(t.TempDir(), "state.json")
	var out strings.Builder
	results, err := RunOnce(context.Background(), Config{Repos: []string{"owner/repo"}, StatePath: state}, r, &out)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(results) != 1 || !results[0].Skipped || results[0].Reason != "head-changed" {
		t.Fatalf("expected head-changed skip, got %#v", results)
	}
	if len(r.posted()) != 0 {
		t.Fatalf("must not review a revision the PR no longer points at: %#v", r.calls)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("a skipped pull request must not record a reviewed head: %v", err)
	}
}

func TestRunOnceRejectsInvalidConfig(t *testing.T) {
	tests := map[string]Config{
		"no repo":       {StatePath: t.TempDir() + "/state.json"},
		"unknown event": {Repos: []string{"owner/repo"}, ReviewEvent: ReviewEvent("MERGE"), StatePath: t.TempDir() + "/state.json"},
	}
	for name, cfg := range tests {
		r := &fakeRunner{responses: map[string]string{}, errors: map[string]error{}}
		var out strings.Builder
		if _, err := RunOnce(context.Background(), cfg, r, &out); err == nil {
			t.Fatalf("%s: RunOnce() error = nil, want error", name)
		}
		if len(r.calls) != 0 {
			t.Fatalf("%s: invalid config must not run commands: %#v", name, r.calls)
		}
	}
}

func TestRunOnceDoesNotPostWhenPrimaryProviderFails(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/repo"):             listResponse(t, PR{Number: 13, Title: "Risky", HeadRefOID: "sha13"}),
			"gh pr diff 13 --repo owner/repo": "diff",
		},
		errors: map[string]error{"codex exec -": fmt.Errorf("codex unavailable")},
	}
	var out strings.Builder
	if _, err := RunOnce(context.Background(), Config{Repos: []string{"owner/repo"}, StatePath: state}, r, &out); err == nil {
		t.Fatal("RunOnce() error = nil, want primary provider error")
	}
	if len(r.posted()) != 0 {
		t.Fatalf("must not post a review without a primary review: %#v", r.calls)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state should not be written when nothing was posted: %v", err)
	}
}

func TestRunOncePersistsPostedReviewsWhenALaterRepoFails(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/first"):            listResponse(t, PR{Number: 1, Title: "First", HeadRefOID: "sha1"}),
			"gh pr diff 1 --repo owner/first": "diff",
			viewKey("owner/first", 1):         viewResponse("sha1"),
			"codex exec -":                    "Codex ok",
			"agy run -":                       "Gemini ok",
		},
		errors: map[string]error{listKey("owner/second"): fmt.Errorf("gh rate limited")},
	}
	var out strings.Builder
	results, err := RunOnce(context.Background(), Config{
		Repos:     []string{"owner/first", "owner/second"},
		StatePath: state,
	}, r, &out)
	if err == nil {
		t.Fatal("RunOnce() error = nil, want listing error for the second repo")
	}
	if len(results) != 1 || !results[0].Posted {
		t.Fatalf("expected the first review to be posted, got %#v", results)
	}
	raw, readErr := os.ReadFile(state)
	if readErr != nil {
		t.Fatalf("a posted review must survive a later failure: %v", readErr)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parse state: %v", err)
	}
	if got["owner/first#1"] != "sha1" {
		t.Fatalf("state = %#v, want reviewed head sha1", got)
	}
}

func TestRunOncePersistsStateWithOwnerOnlyPermissions(t *testing.T) {
	r := &fakeRunner{
		responses: map[string]string{
			listKey("owner/repo"):             listResponse(t, PR{Number: 21, Title: "Ship", HeadRefOID: "sha21"}),
			"gh pr diff 21 --repo owner/repo": "diff",
			viewKey("owner/repo", 21):         viewResponse("sha21"),
			"codex exec -":                    "Codex ok",
			"agy run -":                       "Gemini ok",
		},
		errors: map[string]error{},
	}
	state := filepath.Join(t.TempDir(), "nested", "state.json")
	var out strings.Builder
	if _, err := RunOnce(context.Background(), Config{Repos: []string{"owner/repo"}, StatePath: state}, r, &out); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	info, err := os.Stat(state)
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state permissions = %o, want 600", perm)
	}
	dirInfo, err := os.Stat(filepath.Dir(state))
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("state dir permissions = %o, want 700", perm)
	}
	entries, err := os.ReadDir(filepath.Dir(state))
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("atomic write left temporary files behind: %#v", entries)
	}
}

func TestSaveStateReplacesExistingFileAtomically(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state.json")
	if err := saveState(state, map[string]string{"owner/repo#1": "old"}); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	if err := saveState(state, map[string]string{"owner/repo#1": "new"}); err != nil {
		t.Fatalf("second saveState() error = %v", err)
	}
	got, err := loadState(state)
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	if got["owner/repo#1"] != "new" {
		t.Fatalf("state = %#v, want the replacement value", got)
	}
}

func TestLoadStateTreatsMissingFileAsEmpty(t *testing.T) {
	st, err := loadState(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	if len(st) != 0 {
		t.Fatalf("loadState() = %#v, want empty state", st)
	}
}

func containsArg(args []string, want string) bool {
	return slices.Contains(args, want)
}
