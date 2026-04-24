package ops

import (
	"testing"
)

func TestBatchSpawnRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     *BatchSpawnRequest
		wantLen int
	}{
		{
			name:    "nil request has zero workers",
			req:     nil,
			wantLen: 0,
		},
		{
			name:    "empty workers list",
			req:     &BatchSpawnRequest{Workers: []WorkerSpec{}},
			wantLen: 0,
		},
		{
			name: "single worker",
			req: &BatchSpawnRequest{
				Workers: []WorkerSpec{
					{Name: "fix-lint", PromptFile: "/tmp/fix-lint.txt", Model: "opus"},
				},
			},
			wantLen: 1,
		},
		{
			name: "multiple workers",
			req: &BatchSpawnRequest{
				Workers: []WorkerSpec{
					{Name: "fix-lint", PromptFile: "/tmp/fix-lint.txt", Model: "opus"},
					{Name: "add-docs", PromptFile: "/tmp/add-docs.txt", Model: "sonnet"},
				},
			},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.req == nil {
				return
			}
			if len(tt.req.Workers) != tt.wantLen {
				t.Errorf("got %d workers, want %d", len(tt.req.Workers), tt.wantLen)
			}
		})
	}
}

func TestBatchSpawnResult_Summary(t *testing.T) {
	result := &BatchSpawnResult{
		Operation: "batch_spawn",
		Spawned: []SpawnedWorker{
			{Name: "w1", Model: "opus"},
			{Name: "w2", Model: "sonnet"},
		},
		Failed: []FailedWorker{
			{Name: "w3", Error: "tmux error"},
		},
		Summary: BatchSpawnSummary{
			Total:   3,
			Success: 2,
			Failed:  1,
		},
	}

	if result.Summary.Total != 3 {
		t.Errorf("total: got %d, want 3", result.Summary.Total)
	}
	if result.Summary.Success != 2 {
		t.Errorf("success: got %d, want 2", result.Summary.Success)
	}
	if result.Summary.Failed != 1 {
		t.Errorf("failed: got %d, want 1", result.Summary.Failed)
	}
	if len(result.Spawned) != 2 {
		t.Errorf("spawned: got %d, want 2", len(result.Spawned))
	}
	if len(result.Failed) != 1 {
		t.Errorf("failed: got %d, want 1", len(result.Failed))
	}
}

func TestBatchStatusResult_Structure(t *testing.T) {
	result := &BatchStatusResult{
		Operation: "batch_status",
		Workers: []WorkerStatus{
			{Name: "w1", State: "DONE", Duration: "1h0m0s", Commits: 3, Branch: "agm/abc123"},
			{Name: "w2", State: "WORKING", Duration: "30m0s", Commits: 1, Branch: "agm/def456"},
		},
		Total: 2,
	}

	if result.Operation != "batch_status" {
		t.Errorf("operation: got %q, want %q", result.Operation, "batch_status")
	}
	if result.Total != 2 {
		t.Errorf("total: got %d, want 2", result.Total)
	}

	w1 := result.Workers[0]
	if w1.Name != "w1" || w1.State != "DONE" || w1.Commits != 3 {
		t.Errorf("worker 1: got name=%s state=%s commits=%d", w1.Name, w1.State, w1.Commits)
	}
}

func TestBatchMergeResult_Structure(t *testing.T) {
	result := &BatchMergeResult{
		Operation: "batch_merge",
		Merged: []MergedWorker{
			{Name: "w1", Branch: "agm/abc", Commits: []string{"abc123", "abc456"}},
		},
		Skipped: []SkippedWorker{
			{Name: "w2", Reason: "no commits to merge"},
		},
		Summary: BatchMergeSummary{
			Total:   2,
			Merged:  1,
			Skipped: 1,
		},
	}

	if result.Summary.Merged != 1 {
		t.Errorf("merged: got %d, want 1", result.Summary.Merged)
	}
	if result.Summary.Skipped != 1 {
		t.Errorf("skipped: got %d, want 1", result.Summary.Skipped)
	}
	if len(result.Merged[0].Commits) != 2 {
		t.Errorf("commits: got %d, want 2", len(result.Merged[0].Commits))
	}
}

func TestCountBranchCommits_NoRepo(t *testing.T) {
	// When repo dir is empty, should return 0
	count := countBranchCommits("", "some-branch")
	if count != 0 {
		t.Errorf("expected 0 for empty repo dir, got %d", count)
	}
}

func TestCountBranchCommits_NonexistentDir(t *testing.T) {
	count := countBranchCommits("/nonexistent/repo", "some-branch")
	if count != 0 {
		t.Errorf("expected 0 for nonexistent dir, got %d", count)
	}
}

func TestGetBranchCommits_InvalidRepo(t *testing.T) {
	_, err := getBranchCommits("/nonexistent/repo", "branch", "main")
	if err == nil {
		t.Error("expected error for nonexistent repo")
	}
}

func TestGetBranchCommits_DefaultTarget(t *testing.T) {
	// When target is empty, should default to HEAD
	_, err := getBranchCommits("/nonexistent/repo", "branch", "")
	if err == nil {
		t.Error("expected error for nonexistent repo")
	}
}

func TestWorkerSpec_Fields(t *testing.T) {
	w := WorkerSpec{
		Name:       "fix-lint",
		PromptFile: "/tmp/fix-lint.txt",
		Model:      "opus",
	}
	if w.Name != "fix-lint" {
		t.Errorf("name: got %q, want %q", w.Name, "fix-lint")
	}
	if w.PromptFile != "/tmp/fix-lint.txt" {
		t.Errorf("prompt-file: got %q, want %q", w.PromptFile, "/tmp/fix-lint.txt")
	}
	if w.Model != "opus" {
		t.Errorf("model: got %q, want %q", w.Model, "opus")
	}
}
