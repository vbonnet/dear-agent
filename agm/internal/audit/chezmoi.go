package audit

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// chezmoiInstalled reports whether the chezmoi binary is on PATH.
func chezmoiInstalled() bool {
	_, err := exec.LookPath("chezmoi")
	return err == nil
}

// checkChezmoiDrift reports drift between the user's home-directory dotfiles
// and the chezmoi source. Drift means a managed file was edited directly,
// bypassing chezmoi — running `chezmoi apply` would silently overwrite the
// local edits. Returns nil issues (and nil error) when chezmoi is not
// installed.
func checkChezmoiDrift() ([]*AuditIssue, error) {
	if !chezmoiInstalled() {
		return nil, nil
	}

	cmd := exec.Command("chezmoi", "diff")
	stdout, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		stderr := ""
		if errors.As(err, &exitErr) {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("chezmoi diff failed: %w (%s)", err, stderr)
	}

	paths := parseChezmoiDiffPaths(string(stdout))
	if len(paths) == 0 {
		return nil, nil
	}

	now := time.Now()
	issues := make([]*AuditIssue, 0, len(paths))
	for _, p := range paths {
		issues = append(issues, &AuditIssue{
			Type:     IssueChezmoiDrift,
			Severity: SeverityWarning,
			Path:     p,
			Message:  fmt.Sprintf("Chezmoi drift: %s", p),
			Details: "Managed dotfile edited outside chezmoi. " +
				"`chezmoi apply` would overwrite the local changes.",
			Recommendation: fmt.Sprintf(
				"Capture local edits: `chezmoi re-add ~%s`. "+
					"For templated source files (.tmpl), edit the source manually.",
				p,
			),
			DetectedAt: now,
		})
	}
	return issues, nil
}

// parseChezmoiDiffPaths extracts the relative path of each drifted file from
// `chezmoi diff` unified-diff output. Each diff hunk begins with a line like
// `diff --git a/<path> b/<path>`; we keep the `a/<path>` form (without the
// leading `a/`).
func parseChezmoiDiffPaths(out string) []string {
	const prefix = "diff --git a/"
	seen := map[string]struct{}{}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := strings.TrimPrefix(line, prefix)
		i := strings.Index(rest, " b/")
		if i < 0 {
			continue
		}
		path := "/" + rest[:i]
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}
