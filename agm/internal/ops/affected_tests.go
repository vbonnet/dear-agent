package ops

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindAffectedPackages returns Go packages that transitively depend on files
// changed between baseBranch and HEAD. This enables targeted testing — only
// packages whose dependency tree includes a changed file need re-testing.
func FindAffectedPackages(repoPath string, baseBranch string) ([]string, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("repoPath must not be empty")
	}
	if baseBranch == "" {
		return nil, fmt.Errorf("baseBranch must not be empty")
	}

	// Step 1: Find changed files via git diff.
	changedFiles, err := gitChangedFiles(repoPath, baseBranch)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	if len(changedFiles) == 0 {
		return nil, nil
	}

	// Step 2: Map changed files to their Go packages.
	changedPkgs := changedPackages(repoPath, changedFiles)
	if len(changedPkgs) == 0 {
		return nil, nil
	}

	// Step 3: Build reverse dependency graph using go list -deps.
	allPkgs, err := goListAllPackages(repoPath)
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}

	reverseDeps := buildReverseDeps(repoPath, allPkgs)

	// Step 4: Walk reverse deps from changed packages to find all affected.
	affected := findTransitivelyAffected(changedPkgs, reverseDeps)

	// Step 5: Intersect with repo packages (exclude stdlib/external).
	modulePath, err := goModulePath(repoPath)
	if err != nil {
		return nil, fmt.Errorf("go module path: %w", err)
	}

	var result []string
	for _, pkg := range allPkgs {
		if affected[pkg] && strings.HasPrefix(pkg, modulePath) {
			result = append(result, pkg)
		}
	}

	return result, nil
}

// gitChangedFiles returns files changed between baseBranch...HEAD.
func gitChangedFiles(repoPath, baseBranch string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", baseBranch+"...HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running git diff: %w", err)
	}
	return splitNonEmpty(strings.TrimSpace(string(out))), nil
}

// changedPackages maps file paths to their containing Go package import paths.
func changedPackages(repoPath string, files []string) map[string]bool {
	modulePath, err := goModulePath(repoPath)
	if err != nil {
		return nil
	}

	pkgs := make(map[string]bool)
	for _, f := range files {
		if !strings.HasSuffix(f, ".go") {
			continue
		}
		dir := filepath.Dir(f)
		// Convert filesystem path to import path.
		importPath := modulePath + "/" + filepath.ToSlash(dir)
		if dir == "." {
			importPath = modulePath
		}
		pkgs[importPath] = true
	}
	return pkgs
}

// goModulePath reads the module path from go.mod.
func goModulePath(repoPath string) (string, error) {
	cmd := exec.Command("go", "list", "-m")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list -m: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// goListAllPackages returns all packages in the module.
func goListAllPackages(repoPath string) ([]string, error) {
	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list ./...: %w", err)
	}
	return splitNonEmpty(strings.TrimSpace(string(out))), nil
}

// buildReverseDeps builds a map from package → set of packages that depend on it.
func buildReverseDeps(repoPath string, pkgs []string) map[string]map[string]bool {
	reverse := make(map[string]map[string]bool)

	for _, pkg := range pkgs {
		deps, err := goListDeps(repoPath, pkg)
		if err != nil {
			// Skip packages that fail to list deps (e.g., build-constrained).
			continue
		}
		for _, dep := range deps {
			if reverse[dep] == nil {
				reverse[dep] = make(map[string]bool)
			}
			reverse[dep][pkg] = true
		}
	}

	return reverse
}

// goListDeps returns the dependencies of a single package.
func goListDeps(repoPath, pkg string) ([]string, error) {
	cmd := exec.Command("go", "list", "-deps", pkg)
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return splitNonEmpty(strings.TrimSpace(string(out))), nil
}

// findTransitivelyAffected walks the reverse dependency graph from changedPkgs,
// returning all transitively affected packages.
func findTransitivelyAffected(changedPkgs map[string]bool, reverseDeps map[string]map[string]bool) map[string]bool {
	affected := make(map[string]bool)
	queue := make([]string, 0, len(changedPkgs))

	for pkg := range changedPkgs {
		queue = append(queue, pkg)
		affected[pkg] = true
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for dependent := range reverseDeps[current] {
			if !affected[dependent] {
				affected[dependent] = true
				queue = append(queue, dependent)
			}
		}
	}

	return affected
}

// splitNonEmpty splits on newlines, filtering empty strings.
func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}
