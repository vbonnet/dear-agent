package trigger

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ScopeResolver resolves project identity and filters engrams by scope.
type ScopeResolver struct{}

// NewScopeResolver creates a new ScopeResolver.
func NewScopeResolver() *ScopeResolver { return &ScopeResolver{} }

// ResolveProjectID determines the project identity from the given directory.
// Priority: WAYFINDER-STATUS.md project_name -> git repo root basename -> directory basename
func (sr *ScopeResolver) ResolveProjectID(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Priority 1: Check WAYFINDER-STATUS.md for project_name
	if name := sr.projectNameFromWayfinder(absDir); name != "" {
		return name
	}

	// Priority 2: Walk up to find .git/ directory, use repo root basename
	if root := sr.findGitRoot(absDir); root != "" {
		return filepath.Base(root)
	}

	// Priority 3: Fallback to directory basename
	return filepath.Base(absDir)
}

// projectNameFromWayfinder parses WAYFINDER-STATUS.md YAML frontmatter for project_name.
func (sr *ScopeResolver) projectNameFromWayfinder(dir string) string {
	path := filepath.Join(dir, "WAYFINDER-STATUS.md")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			// End of frontmatter
			break
		}

		if inFrontmatter {
			if strings.HasPrefix(trimmed, "project_name:") {
				val := strings.TrimSpace(strings.TrimPrefix(trimmed, "project_name:"))
				// Strip surrounding quotes if present
				val = strings.Trim(val, "\"'")
				if val != "" {
					return val
				}
			}
		}
	}

	return ""
}

// findGitRoot walks up directories looking for .git/ to find the repo root.
func (sr *ScopeResolver) findGitRoot(dir string) string {
	current := dir
	for {
		gitPath := filepath.Join(current, ".git")
		if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return ""
}

// FindProjectEngramDir returns the .engram/engrams/ directory for a project, or empty string.
func (sr *ScopeResolver) FindProjectEngramDir(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	engramDir := filepath.Join(absDir, ".engram", "engrams")
	if info, err := os.Stat(engramDir); err == nil && info.IsDir() {
		return engramDir
	}
	return ""
}

// IsInScope checks whether a trigger's scope matches the current context.
// scope values: "global" (always matches), "project" (matches by projectID),
// "session" (matches by sessionID)
func (sr *ScopeResolver) IsInScope(scope string, triggerProjectID string, currentProjectID string, triggerSessionID string, currentSessionID string) bool {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "global", "":
		return true
	case "project":
		if triggerProjectID == "" {
			return true
		}
		return triggerProjectID == currentProjectID
	case "session":
		return triggerSessionID == currentSessionID
	default:
		return false
	}
}
