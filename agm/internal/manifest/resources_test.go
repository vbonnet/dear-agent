package manifest

import (
	"testing"
	"time"
)

func TestResourceManifest_ZeroValue(t *testing.T) {
	var m Manifest
	if m.Resources != nil {
		t.Error("Expected Resources to be nil by default")
	}
}

func TestResourceManifest_AddWorktree(t *testing.T) {
	m := &Manifest{}
	m.Resources = &ResourceManifest{
		Worktrees: []WorktreeResource{
			{
				Path:      "/home/user/worktrees/ai-tools/my-feature",
				Branch:    "my-feature",
				Repo:      "/home/user/src/ai-tools",
				CreatedAt: time.Now(),
			},
		},
	}
	if len(m.Resources.Worktrees) != 1 {
		t.Errorf("Expected 1 worktree, got %d", len(m.Resources.Worktrees))
	}
	if m.Resources.Worktrees[0].Branch != "my-feature" {
		t.Errorf("Expected branch 'my-feature', got %q", m.Resources.Worktrees[0].Branch)
	}
}

func TestResourceManifest_AddBranch(t *testing.T) {
	m := &Manifest{}
	m.Resources = &ResourceManifest{
		Branches: []BranchResource{
			{
				Name:      "feat/cleanup",
				Repo:      "~/src/ai-tools",
				CreatedAt: time.Now(),
			},
		},
	}
	if len(m.Resources.Branches) != 1 {
		t.Errorf("Expected 1 branch, got %d", len(m.Resources.Branches))
	}
	if m.Resources.Branches[0].Name != "feat/cleanup" {
		t.Errorf("Expected branch name 'feat/cleanup', got %q", m.Resources.Branches[0].Name)
	}
}

func TestResourceManifest_EmptyCollections(t *testing.T) {
	m := &Manifest{
		Resources: &ResourceManifest{},
	}
	if m.Resources == nil {
		t.Error("Expected non-nil Resources")
	}
	if len(m.Resources.Worktrees) != 0 {
		t.Error("Expected empty Worktrees")
	}
	if len(m.Resources.Branches) != 0 {
		t.Error("Expected empty Branches")
	}
}

func TestWorktreeResource_Fields(t *testing.T) {
	now := time.Now()
	wt := WorktreeResource{
		Path:      "/tmp/wt",
		Branch:    "test-branch",
		Repo:      "/repo",
		CreatedAt: now,
	}
	if wt.Path != "/tmp/wt" {
		t.Errorf("Path mismatch: got %q", wt.Path)
	}
	if wt.Branch != "test-branch" {
		t.Errorf("Branch mismatch: got %q", wt.Branch)
	}
	if wt.Repo != "/repo" {
		t.Errorf("Repo mismatch: got %q", wt.Repo)
	}
	if !wt.CreatedAt.Equal(now) {
		t.Error("CreatedAt mismatch")
	}
}

func TestBranchResource_Fields(t *testing.T) {
	now := time.Now()
	br := BranchResource{
		Name:      "my-branch",
		Repo:      "/repo",
		CreatedAt: now,
	}
	if br.Name != "my-branch" {
		t.Errorf("Name mismatch: got %q", br.Name)
	}
	if br.Repo != "/repo" {
		t.Errorf("Repo mismatch: got %q", br.Repo)
	}
}
