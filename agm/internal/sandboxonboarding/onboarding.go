package sandboxonboarding

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/vbonnet/dear-agent/agm/internal/config"
)

const outputName = "CLAUDE.md"

const defaultTemplate = `# Sandbox Environment — Read-Only Filesystem

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

// Request is the complete input for one sandbox onboarding installation.
type Request struct {
	Home         config.HomeRoot
	SessionID    string
	MergedPath   string
	WorkingDir   string
	Repos        []string
	TemplatePath string
}

type templateData struct {
	SessionName string
	MergedPath  string
	Repos       []string
}

type installHooks struct {
	beforeCommit func()
}

// Install renders and atomically installs one project-scoped Claude onboarding
// file beneath the retained HOME capability.
func Install(request Request) error {
	return installWithHooks(request, installHooks{})
}

func installWithHooks(request Request, hooks installHooks) error {
	if err := requireSupportedPlatform(); err != nil {
		return err
	}
	if !filepath.IsAbs(request.WorkingDir) || filepath.Clean(request.WorkingDir) != request.WorkingDir {
		return fmt.Errorf("sandbox working directory %q must be a clean absolute path", request.WorkingDir)
	}
	if !filepath.IsAbs(request.MergedPath) || filepath.Clean(request.MergedPath) != request.MergedPath {
		return fmt.Errorf("sandbox merged path %q must be a clean absolute path", request.MergedPath)
	}
	relativeWorkingDir, err := filepath.Rel(request.MergedPath, request.WorkingDir)
	if err != nil || filepath.IsAbs(relativeWorkingDir) || relativeWorkingDir == ".." ||
		strings.HasPrefix(relativeWorkingDir, ".."+string(filepath.Separator)) {
		return fmt.Errorf(
			"sandbox working directory %q must be the merged path %q or its descendant",
			request.WorkingDir,
			request.MergedPath,
		)
	}

	homePath, err := request.Home.Path()
	if err != nil {
		return fmt.Errorf("resolve retained HOME: %w", err)
	}
	content, err := render(request, homePath)
	if err != nil {
		return err
	}
	return installOutput(homePath, encodedProjectName(request.WorkingDir), []byte(content), hooks)
}

func render(request Request, homePath string) (string, error) {
	templateText := defaultTemplate
	templateName := "sandbox-onboarding"
	if request.TemplatePath != "" {
		// #nosec G304 -- TemplatePath is the operator-selected AGM configuration value.
		content, err := os.ReadFile(request.TemplatePath)
		if err != nil {
			return "", fmt.Errorf("read onboarding template %q: %w", request.TemplatePath, err)
		}
		templateText = string(content)
		templateName = "custom-sandbox-onboarding"
	}

	tmpl, err := template.New(templateName).Parse(templateText)
	if err != nil {
		return "", fmt.Errorf("parse onboarding template: %w", err)
	}
	data := templateData{
		SessionName: request.SessionID,
		MergedPath:  request.MergedPath,
		Repos:       shortenRepos(homePath, request.Repos),
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render onboarding template: %w", err)
	}
	return rendered.String(), nil
}

func shortenRepos(homePath string, repos []string) []string {
	shortened := make([]string, len(repos))
	for i, repo := range repos {
		shortened[i] = shortenBelowHome(homePath, repo)
	}
	return shortened
}

func shortenBelowHome(homePath, candidate string) string {
	relative, err := filepath.Rel(homePath, candidate)
	if err != nil || filepath.IsAbs(relative) || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return candidate
	}
	if relative == "." {
		return "~"
	}
	return filepath.Join("~", relative)
}

func encodedProjectName(workingDir string) string {
	return strings.ReplaceAll(workingDir, string(filepath.Separator), "-")
}
