package deepresearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/workflow"
)

// ---------------------------------------------------------------------------
// gemini.go tests
// ---------------------------------------------------------------------------

func TestExtractURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single URL",
			input: "check out https://example.com/page for details",
			want:  []string{"https://example.com/page"},
		},
		{
			name:  "no URLs",
			input: "no links here at all",
			want:  nil,
		},
		{
			name:  "multiple URLs",
			input: "see https://a.com and http://b.com/path for info",
			want:  []string{"https://a.com", "http://b.com/path"},
		},
		{
			name:  "URLs with trailing punctuation",
			input: "visit https://example.com/page. Also see https://other.com/path, and https://third.com!",
			want:  []string{"https://example.com/page", "https://other.com/path", "https://third.com"},
		},
		{
			name:  "URL with query params",
			input: "https://example.com/search?q=test&page=1",
			want:  []string{"https://example.com/search?q=test&page=1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractURLs(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("extractURLs(%q) returned %d URLs, want %d: got %v", tt.input, len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractURLs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseReportPath(t *testing.T) {
	w := &GeminiDeepResearch{}

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "report saved to",
			output: "Deep Research completed. Report saved to: ~/src/research/report.md",
			want:   "~/src/research/report.md",
		},
		{
			name:   "research already exists",
			output: "Research already exists at: ~/cache/deep-research/report.md",
			want:   "~/cache/deep-research/report.md",
		},
		{
			name:   "no match returns empty",
			output: "some random output with no path info",
			want:   "",
		},
		{
			name:   "fallback to report.md in line",
			output: "output line\nfound at /tmp/data/report.md here\nmore lines",
			want:   "/tmp/data/report.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.parseReportPath(tt.output)
			if got != tt.want {
				t.Errorf("parseReportPath(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestNewGeminiDeepResearch(t *testing.T) {
	w := NewGeminiDeepResearch()
	if w == nil {
		t.Fatal("NewGeminiDeepResearch() returned nil")
	}
	if w.cliPath == "" {
		t.Error("NewGeminiDeepResearch() produced empty cliPath")
	}
}

func TestGeminiDeepResearchGetters(t *testing.T) {
	w := NewGeminiDeepResearch()

	if got := w.Name(); got != "deep-research" {
		t.Errorf("Name() = %q, want %q", got, "deep-research")
	}

	if got := w.Description(); got == "" {
		t.Error("Description() returned empty string")
	}

	harnesses := w.SupportedHarnesses()
	if len(harnesses) == 0 {
		t.Fatal("SupportedHarnesses() returned empty slice")
	}
	if harnesses[0] != "gemini-cli" {
		t.Errorf("SupportedHarnesses()[0] = %q, want %q", harnesses[0], "gemini-cli")
	}
}

// ---------------------------------------------------------------------------
// applicator.go tests
// ---------------------------------------------------------------------------

func TestNewResearchApplicator(t *testing.T) {
	t.Run("empty repos defaults to engram and ai-tools", func(t *testing.T) {
		a, err := NewResearchApplicator(nil)
		if err != nil {
			t.Fatalf("NewResearchApplicator(nil) error: %v", err)
		}
		if len(a.repos) != 2 {
			t.Fatalf("expected 2 default repos, got %d", len(a.repos))
		}
		if a.repos[0] != "engram" || a.repos[1] != "ai-tools" {
			t.Errorf("default repos = %v, want [engram ai-tools]", a.repos)
		}
	})

	t.Run("custom repos", func(t *testing.T) {
		a, err := NewResearchApplicator([]string{"myrepo"})
		if err != nil {
			t.Fatalf("NewResearchApplicator error: %v", err)
		}
		if len(a.repos) != 1 || a.repos[0] != "myrepo" {
			t.Errorf("repos = %v, want [myrepo]", a.repos)
		}
	})
}

func TestResearchApplicatorApply(t *testing.T) {
	tmpDir := t.TempDir()

	// Create research report files with keyword content.
	report1 := filepath.Join(tmpDir, "report1.md")
	report2 := filepath.Join(tmpDir, "report2.md")

	if err := os.WriteFile(report1, []byte("# Report 1\n\nThis discusses architecture patterns and performance optimization strategies.\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(report2, []byte("# Report 2\n\nFocus on testing best practices and automation pipelines.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	artifacts := []workflow.Artifact{
		{Type: "research-report", Path: report1},
		{Type: "research-report", Path: report2},
	}

	a, err := NewResearchApplicator([]string{"engram", "ai-tools"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := a.Apply(context.Background(), artifacts)
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}

	// Should have proposals for both repos.
	if len(result.Proposals) != 2 {
		t.Fatalf("expected proposals for 2 repos, got %d", len(result.Proposals))
	}

	// Each repo should have proposals generated from keywords found in the reports.
	for repo, proposals := range result.Proposals {
		if len(proposals) == 0 {
			t.Errorf("repo %q has zero proposals", repo)
		}
	}

	if result.Summary == "" {
		t.Error("Apply() returned empty summary")
	}

	if len(result.CrossCuttingIdeas) == 0 {
		t.Error("Apply() returned no cross-cutting ideas")
	}
}

func TestApplyNoReports(t *testing.T) {
	a, err := NewResearchApplicator(nil)
	if err != nil {
		t.Fatal(err)
	}

	// Artifacts with no research-report type should fail.
	_, err = a.Apply(context.Background(), []workflow.Artifact{
		{Type: "other", Path: "/nonexistent"},
	})
	if err == nil {
		t.Error("Apply() with no research reports should return error")
	}
}

func TestWriteProposalsToMarkdown(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "subdir", "proposals.md")

	result := ApplicationResult{
		Summary: "Generated 2 proposals across 1 repository",
		Proposals: map[string][]Proposal{
			"myrepo": {
				{
					Title:         "Improve caching",
					Description:   "Add LRU cache layer",
					Category:      "performance",
					Priority:      "high",
					TestableIdeas: []string{"Benchmark before and after"},
				},
			},
		},
		CrossCuttingIdeas: []string{"Shared logging framework"},
	}

	if err := WriteProposalsToMarkdown(result, outPath); err != nil {
		t.Fatalf("WriteProposalsToMarkdown() error: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	s := string(content)
	for _, want := range []string{
		"# Research-Based Improvement Proposals",
		"Generated 2 proposals across 1 repository",
		"Improve caching",
		"**Category**: performance",
		"**Priority**: high",
		"Add LRU cache layer",
		"Benchmark before and after",
		"Shared logging framework",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing expected string %q", want)
		}
	}
}

func TestGetApplicationSummary(t *testing.T) {
	if got := getApplicationSummary(nil); got != "" {
		t.Errorf("getApplicationSummary(nil) = %q, want empty", got)
	}

	r := &ApplicationResult{Summary: "test summary"}
	if got := getApplicationSummary(r); got != "test summary" {
		t.Errorf("getApplicationSummary() = %q, want %q", got, "test summary")
	}
}

// ---------------------------------------------------------------------------
// logger.go tests
// ---------------------------------------------------------------------------

func TestNewResearchLogger(t *testing.T) {
	tmpDir := t.TempDir()
	urls := []string{"https://example.com", "https://other.com"}

	logger, err := NewResearchLogger("test-session", urls, tmpDir)
	if err != nil {
		t.Fatalf("NewResearchLogger() error: %v", err)
	}

	logPath := logger.GetLogPath()
	if logPath == "" {
		t.Fatal("GetLogPath() returned empty")
	}

	// Log file should exist on disk.
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file does not exist: %v", err)
	}

	// Should contain session ID in content.
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "test-session") {
		t.Error("log file missing session ID")
	}
}

func TestResearchLoggerStateTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	urls := []string{"https://a.com", "https://b.com", "https://c.com"}

	logger, err := NewResearchLogger("transitions", urls, tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Initially all pending.
	pending := logger.GetPendingURLs()
	if len(pending) != 3 {
		t.Fatalf("expected 3 pending, got %d", len(pending))
	}
	completed := logger.GetCompletedURLs()
	if len(completed) != 0 {
		t.Fatalf("expected 0 completed, got %d", len(completed))
	}

	// Mark one started then completed.
	logger.MarkStarted("https://a.com")
	logger.MarkCompleted("https://a.com", "/tmp/report-a.md")

	completed = logger.GetCompletedURLs()
	if len(completed) != 1 || completed[0] != "https://a.com" {
		t.Errorf("completed = %v, want [https://a.com]", completed)
	}

	pending = logger.GetPendingURLs()
	if len(pending) != 2 {
		t.Errorf("expected 2 pending, got %d", len(pending))
	}

	// Mark one failed.
	logger.MarkStarted("https://b.com")
	logger.MarkFailed("https://b.com", fmt.Errorf("network error"))

	// b.com should not be in pending or completed.
	pending = logger.GetPendingURLs()
	if len(pending) != 1 || pending[0] != "https://c.com" {
		t.Errorf("after failure, pending = %v, want [https://c.com]", pending)
	}
	completed = logger.GetCompletedURLs()
	if len(completed) != 1 {
		t.Errorf("after failure, completed count = %d, want 1", len(completed))
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 3*time.Minute + 15*time.Second, "3m0s"},
		{"hours", 2*time.Hour + 10*time.Minute, "2h10m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
