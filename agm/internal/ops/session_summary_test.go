package ops

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// mockGitRunner is a test double for gitRunner.
type mockGitRunner struct {
	responses map[string]mockGitResponse
}

type mockGitResponse struct {
	output string
	err    error
}

func newMockGitRunner() *mockGitRunner {
	return &mockGitRunner{responses: make(map[string]mockGitResponse)}
}

func (m *mockGitRunner) on(args string, output string, err error) {
	m.responses[args] = mockGitResponse{output: output, err: err}
}

func (m *mockGitRunner) run(args ...string) (string, error) {
	key := strings.Join(args, " ")
	if resp, ok := m.responses[key]; ok {
		return resp.output, resp.err
	}
	return "", fmt.Errorf("unexpected git args: %s", key)
}

// --- parseFileStats tests ---

func TestParseFileStats_FullSummary(t *testing.T) {
	input := ` file1.go | 10 ++++------
 file2.go | 20 ++++++++----------
 3 files changed, 120 insertions(+), 15 deletions(-)`

	stats := parseFileStats(input)
	if stats.TotalFiles != 3 {
		t.Errorf("expected 3 files, got %d", stats.TotalFiles)
	}
	if stats.LinesAdded != 120 {
		t.Errorf("expected 120 added, got %d", stats.LinesAdded)
	}
	if stats.LinesRemoved != 15 {
		t.Errorf("expected 15 removed, got %d", stats.LinesRemoved)
	}
}

func TestParseFileStats_InsertionsOnly(t *testing.T) {
	input := ` 1 file changed, 42 insertions(+)`
	stats := parseFileStats(input)
	if stats.TotalFiles != 1 {
		t.Errorf("expected 1 file, got %d", stats.TotalFiles)
	}
	if stats.LinesAdded != 42 {
		t.Errorf("expected 42 added, got %d", stats.LinesAdded)
	}
	if stats.LinesRemoved != 0 {
		t.Errorf("expected 0 removed, got %d", stats.LinesRemoved)
	}
}

func TestParseFileStats_DeletionsOnly(t *testing.T) {
	input := ` 2 files changed, 7 deletions(-)`
	stats := parseFileStats(input)
	if stats.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", stats.TotalFiles)
	}
	if stats.LinesAdded != 0 {
		t.Errorf("expected 0 added, got %d", stats.LinesAdded)
	}
	if stats.LinesRemoved != 7 {
		t.Errorf("expected 7 removed, got %d", stats.LinesRemoved)
	}
}

func TestParseFileStats_Empty(t *testing.T) {
	stats := parseFileStats("")
	if stats.TotalFiles != 0 || stats.LinesAdded != 0 || stats.LinesRemoved != 0 {
		t.Errorf("expected all zeros for empty input, got %+v", stats)
	}
}

// --- getCommits tests ---

func TestGetCommits_Success(t *testing.T) {
	git := newMockGitRunner()
	git.on("log main..feature --format=%H\t%s\t%an\t%aI --reverse",
		"abcdef123456789\tAdd feature X\tAlice\t2026-04-13T10:00:00Z\n"+
			"1234567890ab\tFix bug Y\tBob\t2026-04-13T11:00:00Z",
		nil)

	commits, err := getCommits(git, "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits, got %d", len(commits))
	}

	if commits[0].Hash != "abcdef123456" {
		t.Errorf("expected truncated hash 'abcdef123456', got %q", commits[0].Hash)
	}
	if commits[0].Subject != "Add feature X" {
		t.Errorf("expected subject 'Add feature X', got %q", commits[0].Subject)
	}
	if commits[0].Author != "Alice" {
		t.Errorf("expected author 'Alice', got %q", commits[0].Author)
	}
	if commits[1].Hash != "1234567890ab" {
		t.Errorf("expected hash '1234567890ab', got %q", commits[1].Hash)
	}
}

func TestGetCommits_FallbackToOriginMain(t *testing.T) {
	git := newMockGitRunner()
	git.on("log main..feature --format=%H\t%s\t%an\t%aI --reverse",
		"", fmt.Errorf("unknown revision"))
	git.on("log origin/main..feature --format=%H\t%s\t%an\t%aI --reverse",
		"abc123def456\tInit\tDev\t2026-04-13T09:00:00Z",
		nil)

	commits, err := getCommits(git, "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
}

func TestGetCommits_Empty(t *testing.T) {
	git := newMockGitRunner()
	git.on("log main..feature --format=%H\t%s\t%an\t%aI --reverse", "", nil)

	commits, err := getCommits(git, "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
}

func TestGetCommits_BothFail(t *testing.T) {
	git := newMockGitRunner()
	git.on("log main..feature --format=%H\t%s\t%an\t%aI --reverse",
		"", fmt.Errorf("fail1"))
	git.on("log origin/main..feature --format=%H\t%s\t%an\t%aI --reverse",
		"", fmt.Errorf("fail2"))

	_, err := getCommits(git, "feature")
	if err == nil {
		t.Fatal("expected error when both main and origin/main fail")
	}
}

// --- getFileStats tests ---

func TestGetFileStats_Success(t *testing.T) {
	git := newMockGitRunner()
	git.on("diff --stat main...feature",
		" file.go | 5 +++--\n 2 files changed, 10 insertions(+), 3 deletions(-)", nil)

	stats, err := getFileStats(git, "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", stats.TotalFiles)
	}
}

func TestGetFileStats_FallbackToOrigin(t *testing.T) {
	git := newMockGitRunner()
	git.on("diff --stat main...feature", "", fmt.Errorf("no main"))
	git.on("diff --stat origin/main...feature",
		" 1 file changed, 5 insertions(+)", nil)

	stats, err := getFileStats(git, "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TotalFiles != 1 {
		t.Errorf("expected 1 file, got %d", stats.TotalFiles)
	}
}

// --- computeDuration tests ---

func TestComputeDuration_Normal(t *testing.T) {
	start := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	commits := []CommitInfo{
		{Timestamp: "2026-04-13T10:05:00Z"},
		{Timestamp: "2026-04-13T11:30:00Z"},
	}

	dur := computeDuration(start, commits)
	if dur == nil {
		t.Fatal("expected non-nil duration")
	}
	if dur.StartTime != "2026-04-13T10:00:00Z" {
		t.Errorf("expected start 2026-04-13T10:00:00Z, got %s", dur.StartTime)
	}
	if dur.EndTime != "2026-04-13T11:30:00Z" {
		t.Errorf("expected end 2026-04-13T11:30:00Z, got %s", dur.EndTime)
	}
	if dur.Elapsed != "1h30m" {
		t.Errorf("expected elapsed '1h30m', got %q", dur.Elapsed)
	}
}

func TestComputeDuration_FirstCommitBeforeSessionStart(t *testing.T) {
	start := time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC)
	commits := []CommitInfo{
		{Timestamp: "2026-04-13T10:00:00Z"},
		{Timestamp: "2026-04-13T12:00:00Z"},
	}

	dur := computeDuration(start, commits)
	if dur == nil {
		t.Fatal("expected non-nil duration")
	}
	if dur.StartTime != "2026-04-13T10:00:00Z" {
		t.Errorf("expected start adjusted to first commit, got %s", dur.StartTime)
	}
}

func TestComputeDuration_Empty(t *testing.T) {
	start := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	dur := computeDuration(start, nil)
	if dur != nil {
		t.Error("expected nil for empty commits")
	}
}

func TestComputeDuration_InvalidTimestamp(t *testing.T) {
	start := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	commits := []CommitInfo{
		{Timestamp: "not-a-time"},
	}
	dur := computeDuration(start, commits)
	if dur != nil {
		t.Error("expected nil for invalid timestamp")
	}
}

// --- formatElapsed tests ---

func TestFormatElapsed_Seconds(t *testing.T) {
	if got := formatElapsed(30 * time.Second); got != "30s" {
		t.Errorf("expected '30s', got %q", got)
	}
}

func TestFormatElapsed_Minutes(t *testing.T) {
	if got := formatElapsed(45 * time.Minute); got != "45m" {
		t.Errorf("expected '45m', got %q", got)
	}
}

func TestFormatElapsed_Hours(t *testing.T) {
	if got := formatElapsed(2*time.Hour + 15*time.Minute); got != "2h15m" {
		t.Errorf("expected '2h15m', got %q", got)
	}
}

// --- buildSessionSummary tests ---

func TestBuildSessionSummary_WithCommits(t *testing.T) {
	git := newMockGitRunner()
	git.on("log main..test-branch --format=%H\t%s\t%an\t%aI --reverse",
		"aabbccddee11\tFirst commit\tDev\t2026-04-13T10:10:00Z\n"+
			"112233445566\tSecond commit\tDev\t2026-04-13T10:30:00Z",
		nil)
	git.on("diff --stat main...test-branch",
		" 2 files changed, 50 insertions(+), 10 deletions(-)", nil)

	ctx := testCtx(nil)
	createdAt := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

	result, err := buildSessionSummary(ctx, "test-session", "test-branch", git, createdAt, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Operation != "session_summary" {
		t.Errorf("expected operation 'session_summary', got %q", result.Operation)
	}
	if result.CommitCount != 2 {
		t.Errorf("expected 2 commits, got %d", result.CommitCount)
	}
	if result.Files.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", result.Files.TotalFiles)
	}
	if result.Files.LinesAdded != 50 {
		t.Errorf("expected 50 lines added, got %d", result.Files.LinesAdded)
	}
	if result.Duration == nil {
		t.Fatal("expected non-nil duration")
	}
	if result.Duration.Elapsed != "30m" {
		t.Errorf("expected elapsed '30m', got %q", result.Duration.Elapsed)
	}
}

func TestBuildSessionSummary_NoCommits(t *testing.T) {
	git := newMockGitRunner()
	git.on("log main..empty-branch --format=%H\t%s\t%an\t%aI --reverse", "", nil)
	git.on("diff --stat main...empty-branch", "", nil)

	ctx := testCtx(nil)
	createdAt := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)

	result, err := buildSessionSummary(ctx, "empty-session", "empty-branch", git, createdAt, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CommitCount != 0 {
		t.Errorf("expected 0 commits, got %d", result.CommitCount)
	}
	if result.Duration != nil {
		t.Error("expected nil duration for no commits")
	}
}

func TestBuildSessionSummary_WithCost(t *testing.T) {
	git := newMockGitRunner()
	git.on("log main..cost-branch --format=%H\t%s\t%an\t%aI --reverse", "", nil)
	git.on("diff --stat main...cost-branch", "", nil)

	ctx := testCtx(nil)
	createdAt := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	cost := &CostSummary{
		TokensIn:      1_000_000,
		TokensOut:     500_000,
		EstimatedCost: 52.5,
	}

	result, err := buildSessionSummary(ctx, "cost-session", "cost-branch", git, createdAt, cost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Cost == nil {
		t.Fatal("expected non-nil cost")
	}
	if result.Cost.TokensIn != 1_000_000 {
		t.Errorf("expected 1M tokens in, got %d", result.Cost.TokensIn)
	}
	if result.Cost.EstimatedCost != 52.5 {
		t.Errorf("expected estimated cost 52.5, got %f", result.Cost.EstimatedCost)
	}
}

// --- estimateCostFromTokens tests ---

func TestEstimateCostFromTokens(t *testing.T) {
	// 1M input tokens at $15/M = $15
	// 1M output tokens at $75/M = $75
	cost := estimateCostFromTokens(1_000_000, 1_000_000)
	if cost != 90.0 {
		t.Errorf("expected $90.0, got $%.2f", cost)
	}
}

func TestEstimateCostFromTokens_Zero(t *testing.T) {
	cost := estimateCostFromTokens(0, 0)
	if cost != 0.0 {
		t.Errorf("expected $0.0, got $%.2f", cost)
	}
}

// --- GenerateSessionSummary input validation tests ---

func TestGenerateSessionSummary_NilRequest(t *testing.T) {
	ctx := testCtx(nil)
	_, err := GenerateSessionSummary(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}
}

func TestGenerateSessionSummary_EmptyName(t *testing.T) {
	ctx := testCtx(nil)
	_, err := GenerateSessionSummary(ctx, &GenerateSessionSummaryRequest{SessionName: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestGenerateSessionSummary_SessionNotFound(t *testing.T) {
	ctx := testCtx(nil)
	_, err := GenerateSessionSummary(ctx, &GenerateSessionSummaryRequest{SessionName: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}
