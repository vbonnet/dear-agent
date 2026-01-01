package config

import (
	"testing"
)

func TestMerge_NilConfigs(t *testing.T) {
	base := &Config{Name: "base"}
	local := &Config{Name: "local"}

	tests := []struct {
		name   string
		base   *Config
		local  *Config
		expect *Config
	}{
		{"both nil", nil, nil, nil},
		{"base nil", nil, local, local},
		{"local nil", base, nil, base},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Merge(tt.base, tt.local)
			if result != tt.expect {
				t.Errorf("Merge() = %v, want %v", result, tt.expect)
			}
		})
	}
}

func TestMerge_MetadataOverride(t *testing.T) {
	base := &Config{
		Name:        "base-workspace",
		Description: "base description",
		Owner:       "base-owner",
	}

	local := &Config{
		Name:        "local-workspace",
		Description: "local description",
		Owner:       "local-owner",
	}

	result := Merge(base, local)

	if result.Name != "local-workspace" {
		t.Errorf("Name = %q, want %q", result.Name, "local-workspace")
	}
	if result.Description != "local description" {
		t.Errorf("Description = %q, want %q", result.Description, "local description")
	}
	if result.Owner != "local-owner" {
		t.Errorf("Owner = %q, want %q", result.Owner, "local-owner")
	}
}

func TestMerge_AddLocalWorktrees(t *testing.T) {
	base := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "ai-tools",
				URL:  "https://github.com/test/ai-tools.git",
				Worktrees: []Worktree{
					{Name: "base", Branch: "main"},
				},
			},
		},
	}

	local := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "ai-tools",
				Worktrees: []Worktree{
					{Name: "feature-x", Branch: "feature-x"},
				},
			},
		},
	}

	result := Merge(base, local)

	if len(result.Repos) != 1 {
		t.Fatalf("len(Repos) = %d, want 1", len(result.Repos))
	}

	repo := result.Repos[0]
	if len(repo.Worktrees) != 2 {
		t.Fatalf("len(Worktrees) = %d, want 2", len(repo.Worktrees))
	}

	// Check both worktrees exist
	foundBase := false
	foundFeature := false
	for _, wt := range repo.Worktrees {
		if wt.Name == "base" && wt.Branch == "main" {
			foundBase = true
		}
		if wt.Name == "feature-x" && wt.Branch == "feature-x" {
			foundFeature = true
		}
	}

	if !foundBase {
		t.Error("base worktree not found in merged config")
	}
	if !foundFeature {
		t.Error("feature-x worktree not found in merged config")
	}
}

func TestMerge_AddLocalRepo(t *testing.T) {
	base := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "repo1",
				URL:  "https://github.com/test/repo1.git",
			},
		},
	}

	local := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "repo2",
				URL:  "https://github.com/test/repo2.git",
			},
		},
	}

	result := Merge(base, local)

	if len(result.Repos) != 2 {
		t.Fatalf("len(Repos) = %d, want 2", len(result.Repos))
	}

	// Check both repos exist
	foundRepo1 := false
	foundRepo2 := false
	for _, repo := range result.Repos {
		if repo.Name == "repo1" {
			foundRepo1 = true
		}
		if repo.Name == "repo2" {
			foundRepo2 = true
		}
	}

	if !foundRepo1 {
		t.Error("repo1 not found in merged config")
	}
	if !foundRepo2 {
		t.Error("repo2 not found in merged config")
	}
}

func TestMerge_NoDuplicateWorktrees(t *testing.T) {
	base := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "ai-tools",
				URL:  "https://github.com/test/ai-tools.git",
				Worktrees: []Worktree{
					{Name: "base", Branch: "main"},
				},
			},
		},
	}

	local := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "ai-tools",
				Worktrees: []Worktree{
					{Name: "base", Branch: "different-branch"}, // Same name, different branch
					{Name: "feature", Branch: "feature"},
				},
			},
		},
	}

	result := Merge(base, local)

	repo := result.Repos[0]
	if len(repo.Worktrees) != 2 {
		t.Fatalf("len(Worktrees) = %d, want 2 (no duplicates)", len(repo.Worktrees))
	}

	// Base worktree should be preserved (not overridden)
	baseWt := repo.Worktrees[0]
	if baseWt.Name != "base" || baseWt.Branch != "main" {
		t.Errorf("base worktree = {%q, %q}, want {%q, %q}",
			baseWt.Name, baseWt.Branch, "base", "main")
	}

	// Feature worktree should be added
	featureWt := repo.Worktrees[1]
	if featureWt.Name != "feature" {
		t.Errorf("second worktree name = %q, want %q", featureWt.Name, "feature")
	}
}

func TestMerge_PreserveBaseRepos(t *testing.T) {
	base := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "important-repo",
				URL:  "https://github.com/test/important.git",
				Type: RepoTypeBare,
			},
		},
	}

	local := &Config{
		Name: "test",
		Repos: []Repo{
			{
				Name: "local-repo",
				URL:  "https://github.com/test/local.git",
			},
		},
	}

	result := Merge(base, local)

	// Check that base repo is preserved
	foundImportant := false
	for _, repo := range result.Repos {
		if repo.Name == "important-repo" {
			foundImportant = true
			if repo.URL != "https://github.com/test/important.git" {
				t.Errorf("base repo URL changed to %q", repo.URL)
			}
			if repo.Type != RepoTypeBare {
				t.Errorf("base repo type changed to %q", repo.Type)
			}
		}
	}

	if !foundImportant {
		t.Error("base repo was removed in merge (should be preserved)")
	}
}

func TestMerge_ComplexScenario(t *testing.T) {
	base := &Config{
		Name:        "workspace",
		Description: "team workspace",
		Owner:       "team",
		Repos: []Repo{
			{
				Name: "repo1",
				URL:  "https://github.com/test/repo1.git",
				Type: RepoTypeBare,
				Worktrees: []Worktree{
					{Name: "main", Branch: "main"},
					{Name: "develop", Branch: "develop"},
				},
			},
			{
				Name: "repo2",
				URL:  "https://github.com/test/repo2.git",
			},
		},
	}

	local := &Config{
		Name:  "my-workspace",
		Owner: "me",
		Repos: []Repo{
			{
				Name: "repo1",
				Worktrees: []Worktree{
					{Name: "my-feature", Branch: "feature"},
				},
			},
			{
				Name: "repo3",
				URL:  "https://github.com/test/repo3.git",
			},
		},
	}

	result := Merge(base, local)

	// Check metadata override
	if result.Name != "my-workspace" {
		t.Errorf("Name = %q, want %q", result.Name, "my-workspace")
	}
	if result.Owner != "me" {
		t.Errorf("Owner = %q, want %q", result.Owner, "me")
	}

	// Check repos count (repo1, repo2, repo3)
	if len(result.Repos) != 3 {
		t.Fatalf("len(Repos) = %d, want 3", len(result.Repos))
	}

	// Find repo1 and check worktrees
	var repo1 *Repo
	for i, repo := range result.Repos {
		if repo.Name == "repo1" {
			repo1 = &result.Repos[i]
			break
		}
	}

	if repo1 == nil {
		t.Fatal("repo1 not found")
	}

	// repo1 should have 3 worktrees (main, develop, my-feature)
	if len(repo1.Worktrees) != 3 {
		t.Errorf("repo1 worktrees = %d, want 3", len(repo1.Worktrees))
	}

	worktreeNames := make(map[string]bool)
	for _, wt := range repo1.Worktrees {
		worktreeNames[wt.Name] = true
	}

	expectedWorktrees := []string{"main", "develop", "my-feature"}
	for _, name := range expectedWorktrees {
		if !worktreeNames[name] {
			t.Errorf("worktree %q not found in repo1", name)
		}
	}
}
