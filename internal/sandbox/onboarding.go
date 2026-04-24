package sandbox

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const defaultOnboardingTemplate = `# Sandbox Environment — Read-Only Filesystem

This session runs inside a sandboxed overlay filesystem.
The repository directories below are **READ-ONLY** (overlay lower dirs).
Any attempt to modify files directly will fail with a permission error.

## How to Make Changes: Git Worktrees

All code changes MUST use ` + "`git worktree add`" + ` to create a writable copy:

` + "```" + `bash
# Create a worktree for the repo you need to modify:
git -C ~/src/ws/oss/repos/{repo} worktree add ~/src/ws/oss/worktrees/{repo}/{branch} -b {branch}

# Then work in the worktree directory:
# ~/src/ws/oss/worktrees/{repo}/{branch}/
` + "```" + `

## Quick Reference

| Item | Path |
|------|------|
{{- range .Repos}}
| Repo (READ-ONLY) | ` + "`{{.}}`" + ` |
{{- end}}
| Worktree base | ` + "`~/src/ws/oss/worktrees/`" + ` |
| Session name | ` + "`{{.SessionName}}`" + ` |

## Rules

1. **NEVER** modify files directly in repos/ — they are read-only
2. **ALWAYS** create a git worktree before making changes
3. Use the session name (` + "`{{.SessionName}}`" + `) as the branch name
4. Use ` + "`git -C`" + ` flag instead of ` + "`cd`" + ` for git commands
5. Commit and push from the worktree directory
6. **Summarize sub-agent results**: When sub-agents (Explore, Plan) return results, condense them into 3–5 bullet points before acting on them. Never let raw sub-agent context bloat the main session.
`

// OnboardingData holds the template data for generating CLAUDE.md content.
type OnboardingData struct {
	SessionName string
	MergedPath  string
	Repos       []string
}

// GenerateOnboardingContent renders the onboarding CLAUDE.md template
// with session-specific values.
func GenerateOnboardingContent(sessionName, mergedPath string, repos []string) (string, error) {
	// Shorten repo paths for readability
	homeDir, _ := os.UserHomeDir()
	shortRepos := make([]string, len(repos))
	for i, r := range repos {
		if strings.HasPrefix(r, homeDir) {
			shortRepos[i] = "~" + r[len(homeDir):]
		} else {
			shortRepos[i] = r
		}
	}

	data := OnboardingData{
		SessionName: sessionName,
		MergedPath:  mergedPath,
		Repos:       shortRepos,
	}

	tmpl, err := template.New("onboarding").Parse(defaultOnboardingTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse onboarding template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render onboarding template: %w", err)
	}

	return buf.String(), nil
}

// GenerateOnboardingContentFromFile renders a custom template file
// with session-specific values.
func GenerateOnboardingContentFromFile(templatePath, sessionName, mergedPath string, repos []string) (string, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file %s: %w", templatePath, err)
	}

	homeDir, _ := os.UserHomeDir()
	shortRepos := make([]string, len(repos))
	for i, r := range repos {
		if strings.HasPrefix(r, homeDir) {
			shortRepos[i] = "~" + r[len(homeDir):]
		} else {
			shortRepos[i] = r
		}
	}

	data := OnboardingData{
		SessionName: sessionName,
		MergedPath:  mergedPath,
		Repos:       shortRepos,
	}

	tmpl, err := template.New("custom-onboarding").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse custom template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render custom template: %w", err)
	}

	return buf.String(), nil
}

// ClaudeProjectDir returns the Claude Code project directory path for the given
// working directory. Claude Code encodes project paths by replacing all path
// separators with hyphens: /home/user/project → -home-user-project
// The resulting directory is ~/.claude/projects/<encoded>/
func ClaudeProjectDir(workDir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Claude Code encodes: replace all "/" with "-"
	encoded := strings.ReplaceAll(workDir, string(filepath.Separator), "-")

	return filepath.Join(homeDir, ".claude", "projects", encoded), nil
}

// WriteOnboardingClaudeMd writes the onboarding instructions to the Claude Code
// project directory (~/.claude/projects/<encoded-mergedPath>/CLAUDE.md) rather
// than the repo's git-tracked CLAUDE.md. This prevents the stop hook from
// detecting uncommitted changes in the sandbox overlay.
func WriteOnboardingClaudeMd(mergedPath, content string) error {
	projectDir, err := ClaudeProjectDir(mergedPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory %s: %w", projectDir, err)
	}

	claudeMdPath := filepath.Join(projectDir, "CLAUDE.md")

	// If a project-level CLAUDE.md already exists, prepend onboarding content
	if existing, err := os.ReadFile(claudeMdPath); err == nil {
		content = content + "\n---\n\n" + string(existing)
	}

	// #nosec G306 G703 -- claudeMdPath is constructed from ~/.claude/projects/ + deterministic encoding of mergedPath
	return os.WriteFile(claudeMdPath, []byte(content), 0600)
}
