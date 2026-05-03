package vcs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidationError represents a pre-commit validation failure
type ValidationError struct {
	File    string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

// ValidateMemoryPair checks that staged .ai.md files have .why.md companions.
// repoDir is the root of the memory git repo.
// stagedFiles are relative paths to staged files.
func ValidateMemoryPair(repoDir string, stagedFiles []string) []ValidationError {
	var errors []ValidationError

	for _, f := range stagedFiles {
		if !strings.HasSuffix(f, ".ai.md") {
			continue
		}

		whyPath := strings.TrimSuffix(f, ".ai.md") + ".why.md"
		absWhyPath := filepath.Join(repoDir, whyPath)

		if _, err := os.Stat(absWhyPath); os.IsNotExist(err) {
			errors = append(errors, ValidationError{
				File:    f,
				Message: fmt.Sprintf("missing companion .why.md file (expected %s)", whyPath),
			})
		}
	}

	return errors
}

// ValidateEngramFrontmatter does a basic check that .ai.md files have valid frontmatter.
// This is a lightweight check — full validation uses the engram validator package.
func ValidateEngramFrontmatter(repoDir string, stagedFiles []string) []ValidationError {
	var errors []ValidationError

	for _, f := range stagedFiles {
		if !strings.HasSuffix(f, ".ai.md") {
			continue
		}

		absPath := filepath.Join(repoDir, f)
		data, err := os.ReadFile(absPath)
		if err != nil {
			errors = append(errors, ValidationError{
				File:    f,
				Message: fmt.Sprintf("cannot read file: %v", err),
			})
			continue
		}

		content := string(data)

		// Check for frontmatter delimiters
		if !strings.HasPrefix(content, "---\n") {
			errors = append(errors, ValidationError{
				File:    f,
				Message: "missing YAML frontmatter (must start with ---)",
			})
			continue
		}

		// Find closing delimiter
		closeIdx := strings.Index(content[4:], "\n---\n")
		if closeIdx == -1 {
			// Also check for file ending with ---
			if !strings.HasSuffix(strings.TrimSpace(content[4:]), "---") {
				errors = append(errors, ValidationError{
					File:    f,
					Message: "unclosed YAML frontmatter (missing closing ---)",
				})
			}
		}
	}

	return errors
}

// preCommitHookScript is the git pre-commit hook that validates memory files
const preCommitHookScript = `#!/bin/sh
# Engram memory VCS pre-commit hook
# Validates .ai.md / .why.md pairing and frontmatter

# Get staged files
STAGED=$(git diff --cached --name-only --diff-filter=ACM)

# Check each .ai.md has a .why.md companion
for f in $STAGED; do
    case "$f" in
        *.ai.md)
            why="${f%.ai.md}.why.md"
            if [ ! -f "$why" ]; then
                echo "ERROR: $f is missing companion file: $why"
                echo "Every .ai.md must have a corresponding .why.md"
                exit 1
            fi
            ;;
    esac
done

# Validate frontmatter exists
for f in $STAGED; do
    case "$f" in
        *.ai.md)
            if ! head -1 "$f" | grep -q "^---$"; then
                echo "ERROR: $f is missing YAML frontmatter"
                exit 1
            fi
            ;;
    esac
done

exit 0
`

// InstallPreCommitHook writes the pre-commit hook to the repo's .git/hooks/
func InstallPreCommitHook(repoDir string) error {
	// Find .git directory
	gitDir := filepath.Join(repoDir, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		// Could be a worktree with .git as a file
		return fmt.Errorf("not a git repository: %s", repoDir)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o700); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-commit")

	// Write to temp file then rename (atomic write pattern)
	tmpPath := hookPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(preCommitHookScript), 0o700); err != nil { //#nosec G306 -- executable hook
		return fmt.Errorf("write hook: %w", err)
	}

	if err := os.Rename(tmpPath, hookPath); err != nil {
		os.Remove(tmpPath) // cleanup
		return fmt.Errorf("install hook: %w", err)
	}

	return nil
}
