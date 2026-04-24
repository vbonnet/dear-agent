// Package config provides configuration management.
package config

// Merge combines two configs additively.
// The local config extends the base config by:
// - Adding new repos from local
// - Adding new worktrees to existing repos
// - Preserving all base config repos and worktrees
//
// This ensures local configs cannot accidentally remove shared team repos.
func Merge(base, local *Config) *Config {
	if base == nil {
		return local
	}
	if local == nil {
		return base
	}

	result := &Config{
		Name:        base.Name,
		Description: base.Description,
		Owner:       base.Owner,
		Repos:       make([]Repo, 0, len(base.Repos)),
	}

	// Override metadata if local has values
	if local.Name != "" {
		result.Name = local.Name
	}
	if local.Description != "" {
		result.Description = local.Description
	}
	if local.Owner != "" {
		result.Owner = local.Owner
	}

	// Create map of base repos for quick lookup
	baseRepos := make(map[string]*Repo)
	for i := range base.Repos {
		baseRepos[base.Repos[i].Name] = &base.Repos[i]
	}

	// Create map of local repos for quick lookup
	localRepos := make(map[string]*Repo)
	for i := range local.Repos {
		localRepos[local.Repos[i].Name] = &local.Repos[i]
	}

	// Process all base repos
	for _, baseRepo := range base.Repos {
		merged := Repo{
			Name:      baseRepo.Name,
			URL:       baseRepo.URL,
			Type:      baseRepo.Type,
			Worktrees: make([]Worktree, 0),
		}

		// Copy base worktrees
		merged.Worktrees = append(merged.Worktrees, baseRepo.Worktrees...)

		// Add local worktrees if repo exists in local config
		if localRepo, exists := localRepos[baseRepo.Name]; exists {
			// Build set of existing worktree names
			existingWorktrees := make(map[string]bool)
			for _, wt := range merged.Worktrees {
				existingWorktrees[wt.Name] = true
			}

			// Add new worktrees from local (skip duplicates)
			for _, localWt := range localRepo.Worktrees {
				if !existingWorktrees[localWt.Name] {
					merged.Worktrees = append(merged.Worktrees, localWt)
				}
			}
		}

		result.Repos = append(result.Repos, merged)
	}

	// Add repos that only exist in local config
	for _, localRepo := range local.Repos {
		if _, exists := baseRepos[localRepo.Name]; !exists {
			result.Repos = append(result.Repos, localRepo)
		}
	}

	return result
}
