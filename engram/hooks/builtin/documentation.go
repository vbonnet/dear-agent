package builtin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/engram/hooks"
)

// DocumentationChecker detects documentation gaps
type DocumentationChecker struct {
	projectRoot string
}

// NewDocumentationChecker creates a new documentation checker
func NewDocumentationChecker(projectRoot string) *DocumentationChecker {
	return &DocumentationChecker{
		projectRoot: projectRoot,
	}
}

// CheckDocumentation detects stale or missing documentation
func (dc *DocumentationChecker) CheckDocumentation(ctx context.Context) (*hooks.VerificationResult, error) {
	var violations []hooks.Violation

	// Check for key documentation files
	requiredDocs := []string{
		"README.md",
		"SPEC.md",
		"ARCHITECTURE.md",
	}

	missingDocs := []string{}
	for _, doc := range requiredDocs {
		docPath := filepath.Join(dc.projectRoot, doc)
		if _, err := os.Stat(docPath); os.IsNotExist(err) {
			missingDocs = append(missingDocs, doc)
		}
	}

	if len(missingDocs) > 0 {
		violations = append(violations, hooks.Violation{
			Severity:   "medium",
			Message:    fmt.Sprintf("Missing documentation files: %s", strings.Join(missingDocs, ", ")),
			Suggestion: "Create missing documentation to improve project understanding",
		})
	}

	// Check for stale documentation using git
	staleDocs, err := dc.detectStaleDocs(ctx)
	if err != nil {
		// Not a git repo or git not available - skip stale check
	} else if len(staleDocs) > 0 {
		violations = append(violations, hooks.Violation{
			Severity:   "medium",
			Message:    fmt.Sprintf("Documentation may be stale: %s", strings.Join(staleDocs, ", ")),
			Files:      staleDocs,
			Suggestion: "Update documentation to reflect recent code changes",
		})
	}

	status := hooks.VerificationStatusPass
	if len(violations) > 0 {
		status = hooks.VerificationStatusWarning
	}

	return &hooks.VerificationResult{
		Status:     status,
		Violations: violations,
	}, nil
}

// detectStaleDocs finds documentation files that haven't been updated recently
func (dc *DocumentationChecker) detectStaleDocs(ctx context.Context) ([]string, error) {
	// Get files modified in last commit
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "HEAD~1..HEAD")
	cmd.Dir = dc.projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	modifiedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Check if code files were modified but documentation wasn't
	codeModified := false
	docsModified := false

	for _, file := range modifiedFiles {
		ext := filepath.Ext(file)
		if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".rs" {
			codeModified = true
		}
		if ext == ".md" || strings.HasSuffix(file, "README") {
			docsModified = true
		}
	}

	// If code was modified but docs weren't, report potential stale docs
	var staleDocs []string
	if codeModified && !docsModified {
		// Check which doc files exist
		docFiles := []string{"README.md", "SPEC.md", "ARCHITECTURE.md"}
		for _, doc := range docFiles {
			docPath := filepath.Join(dc.projectRoot, doc)
			if _, err := os.Stat(docPath); err == nil {
				staleDocs = append(staleDocs, doc)
			}
		}
	}

	return staleDocs, nil
}
