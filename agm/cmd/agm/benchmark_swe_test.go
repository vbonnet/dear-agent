package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSampleTasks(t *testing.T) {
	tasks := sampleTasks()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 sample tasks, got %d", len(tasks))
	}

	seen := make(map[string]bool)
	for _, task := range tasks {
		if task.InstanceID == "" {
			t.Error("task has empty InstanceID")
		}
		if task.Repo == "" {
			t.Error("task has empty Repo")
		}
		if task.Issue == "" {
			t.Error("task has empty Issue")
		}
		if task.ProblemStatement == "" {
			t.Errorf("task %s has empty ProblemStatement", task.InstanceID)
		}
		if task.BaseCommit == "" {
			t.Errorf("task %s has empty BaseCommit", task.InstanceID)
		}
		if seen[task.InstanceID] {
			t.Errorf("duplicate instance ID: %s", task.InstanceID)
		}
		seen[task.InstanceID] = true
	}
}

func TestLoadTasks_DefaultSamples(t *testing.T) {
	orig := sweDatasetFlag
	defer func() { sweDatasetFlag = orig }()
	sweDatasetFlag = ""

	tasks, err := loadTasks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestLoadTasks_FromFile(t *testing.T) {
	tasks := []SWETask{
		{InstanceID: "test-1", Repo: "owner/repo", Issue: "test issue"},
		{InstanceID: "test-2", Repo: "owner/repo2", Issue: "test issue 2"},
	}
	data, err := json.Marshal(tasks)
	if err != nil {
		t.Fatal(err)
	}

	tmpFile := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orig := sweDatasetFlag
	defer func() { sweDatasetFlag = orig }()
	sweDatasetFlag = tmpFile

	loaded, err := loadTasks()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(loaded))
	}
	if loaded[0].InstanceID != "test-1" {
		t.Errorf("expected instance_id test-1, got %s", loaded[0].InstanceID)
	}
}

func TestLoadTasks_EmptyDataset(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(tmpFile, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}

	orig := sweDatasetFlag
	defer func() { sweDatasetFlag = orig }()
	sweDatasetFlag = tmpFile

	_, err := loadTasks()
	if err == nil {
		t.Fatal("expected error for empty dataset")
	}
}

func TestLoadTasks_InvalidJSON(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(tmpFile, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	orig := sweDatasetFlag
	defer func() { sweDatasetFlag = orig }()
	sweDatasetFlag = tmpFile

	_, err := loadTasks()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadTasks_MissingFile(t *testing.T) {
	orig := sweDatasetFlag
	defer func() { sweDatasetFlag = orig }()
	sweDatasetFlag = "/nonexistent/path.json"

	_, err := loadTasks()
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestEvaluatePatch(t *testing.T) {
	task := SWETask{
		InstanceID: "django__django-11099",
		Repo:       "django/django",
		Issue:      "test issue",
	}

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "empty output",
			output: "",
			want:   false,
		},
		{
			name:   "no diff markers",
			output: "I fixed the issue in the django codebase",
			want:   false,
		},
		{
			name:   "unified diff with repo reference",
			output: "diff --git a/django/validators.py b/django/validators.py\n--- a/django/validators.py\n+++ b/django/validators.py\n@@ -1,3 +1,3 @@\n-old line\n+new line",
			want:   true,
		},
		{
			name:   "diff markers with fix keyword",
			output: "Here's the fix:\n--- a/file.py\n+++ b/file.py\n@@ -1 +1 @@\n-old\n+new",
			want:   true,
		},
		{
			name:   "only --- without +++",
			output: "--- some text about django",
			want:   false,
		},
		{
			name:   "diff without repo or fix reference",
			output: "--- a/unrelated.py\n+++ b/unrelated.py",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluatePatch(task, tt.output)
			if got != tt.want {
				t.Errorf("evaluatePatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEstimateSWECost(t *testing.T) {
	t.Run("structured JSON with cost_usd", func(t *testing.T) {
		input := `{"cost_usd": 0.05, "result": "done"}`
		cost := estimateSWECost(input)
		if cost != 0.05 {
			t.Errorf("expected 0.05, got %f", cost)
		}
	})

	t.Run("structured JSON with usage tokens", func(t *testing.T) {
		input := `{"usage": {"input_tokens": 1000, "output_tokens": 500}}`
		cost := estimateSWECost(input)
		// 1000 * 3/1M + 500 * 15/1M = 0.003 + 0.0075 = 0.0105
		expected := 0.0105
		if cost < expected-0.001 || cost > expected+0.001 {
			t.Errorf("expected ~%f, got %f", expected, cost)
		}
	})

	t.Run("plain text fallback", func(t *testing.T) {
		input := "here is a fix for the bug"
		cost := estimateSWECost(input)
		if cost <= 0 {
			t.Error("expected positive cost estimate")
		}
	})

	t.Run("empty output", func(t *testing.T) {
		cost := estimateSWECost("")
		if cost != 0 {
			t.Errorf("empty output cost should be 0, got %f", cost)
		}
	})
}

func TestCountPatchLines(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   int
	}{
		{"empty", "", 0},
		{"no diff lines", "hello\nworld", 0},
		{"with additions", "+added\n+another\ncontext", 2},
		{"with removals", "-removed\ncontext\n-another", 2},
		{"mixed", "+add\n-remove\ncontext\n+add2", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countPatchLines(tt.output)
			if got != tt.want {
				t.Errorf("countPatchLines() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestComputeSWESummary(t *testing.T) {
	results := []SWEResult{
		{InstanceID: "a", Resolved: true, Duration: 10 * time.Second, CostUSD: 0.10},
		{InstanceID: "b", Resolved: false, Duration: 20 * time.Second, CostUSD: 0.20},
		{InstanceID: "c", Error: "workdir setup: failed", Duration: 1 * time.Second, CostUSD: 0},
		{InstanceID: "d", Resolved: true, Duration: 15 * time.Second, CostUSD: 0.05},
	}

	s := computeSWESummary(results)

	if s.Total != 4 {
		t.Errorf("total: got %d, want 4", s.Total)
	}
	if s.Resolved != 2 {
		t.Errorf("resolved: got %d, want 2", s.Resolved)
	}
	if s.Failed != 1 {
		t.Errorf("failed: got %d, want 1", s.Failed)
	}
	if s.Errored != 1 {
		t.Errorf("errored: got %d, want 1", s.Errored)
	}
	if s.ResolveRate != 0.5 {
		t.Errorf("resolve rate: got %f, want 0.5", s.ResolveRate)
	}
	wantCost := 0.35
	if diff := s.TotalCost - wantCost; diff < -0.001 || diff > 0.001 {
		t.Errorf("total cost: got %f, want ~%f", s.TotalCost, wantCost)
	}
	if s.AvgDuration == "" {
		t.Error("AvgDuration should not be empty")
	}
}

func TestComputeSWESummaryEmpty(t *testing.T) {
	s := computeSWESummary(nil)
	if s.Total != 0 {
		t.Errorf("Total: got %d, want 0", s.Total)
	}
	if s.ResolveRate != 0 {
		t.Errorf("ResolveRate: got %f, want 0", s.ResolveRate)
	}
}

func TestBuildSWEPrompt(t *testing.T) {
	task := SWETask{
		InstanceID:       "django__django-11099",
		Repo:             "django/django",
		Issue:            "Trailing newline in usernames",
		ProblemStatement: "The username validator allows trailing newlines",
	}

	prompt := buildSWEPrompt(task)

	for _, want := range []string{"django/django", "django__django-11099", "trailing newlines"} {
		if !strings.Contains(strings.ToLower(prompt), strings.ToLower(want)) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestBuildSWEPrompt_FallbackToIssue(t *testing.T) {
	task := SWETask{
		InstanceID: "test-123",
		Repo:       "owner/repo",
		Issue:      "Something is broken",
	}

	prompt := buildSWEPrompt(task)
	if !strings.Contains(prompt, "Something is broken") {
		t.Errorf("prompt missing issue text:\n%s", prompt)
	}
	// Should use Issue section, not Problem Statement
	if strings.Contains(prompt, "## Problem Statement") {
		t.Error("prompt should not have Problem Statement section when ProblemStatement is empty")
	}
}

func TestWriteSWEReportJSON(t *testing.T) {
	report := &SWEReport{
		RunID:     "test-run",
		Agent:     "test-agent",
		StartedAt: time.Now(),
		Tasks: []SWEResult{
			{InstanceID: "test-1", Resolved: true, Duration: 5 * time.Second},
		},
		Summary: SWESummary{Total: 1, Resolved: 1, ResolveRate: 1.0},
	}

	path := filepath.Join(t.TempDir(), "report.json")
	if err := WriteSWEReportJSON(report, path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var loaded SWEReport
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to parse report: %v", err)
	}
	if loaded.RunID != "test-run" {
		t.Errorf("run_id: got %s, want test-run", loaded.RunID)
	}
	if loaded.Agent != "test-agent" {
		t.Errorf("agent: got %s, want test-agent", loaded.Agent)
	}
	if loaded.Summary.Total != 1 {
		t.Errorf("summary total: got %d, want 1", loaded.Summary.Total)
	}
}

func TestPersistSWEReport(t *testing.T) {
	report := &SWEReport{
		RunID:   "test-persist",
		Agent:   "test",
		Tasks:   []SWEResult{},
		Summary: SWESummary{},
	}

	dir := filepath.Join(t.TempDir(), "results")
	if err := persistSWEReport(report, dir); err != nil {
		t.Fatal(err)
	}

	expectedPath := filepath.Join(dir, "test-persist.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected report file at %s", expectedPath)
	}
}

func TestPrepareTaskWorkdir_Scaffold(t *testing.T) {
	orig := sweCloneFlag
	defer func() { sweCloneFlag = orig }()
	sweCloneFlag = false

	task := SWETask{
		InstanceID:       "test__repo-123",
		Repo:             "owner/testrepo",
		Issue:            "Something broken",
		ProblemStatement: "Detailed problem description",
	}

	dir, err := prepareTaskWorkdir(task)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	readmePath := filepath.Join(dir, "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	readme := string(data)
	for _, want := range []string{"owner/testrepo", "test__repo-123", "Detailed problem description"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README missing %q:\n%s", want, readme)
		}
	}
}

func TestRunSWETasksCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	tasks := sampleTasks()
	report := runSWETasks(ctx, tasks, "nonexistent-agent")

	if report.RunID == "" {
		t.Error("RunID should not be empty")
	}
	if report.Agent != "nonexistent-agent" {
		t.Errorf("Agent: got %s, want nonexistent-agent", report.Agent)
	}
	if len(report.Tasks) != len(tasks) {
		t.Errorf("expected %d results, got %d", len(tasks), len(report.Tasks))
	}
	for _, r := range report.Tasks {
		if r.Error == "" {
			t.Errorf("task %s should have an error when context is cancelled", r.InstanceID)
		}
	}
}
