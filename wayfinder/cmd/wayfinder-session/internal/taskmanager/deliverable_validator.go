package taskmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateDeliverables checks that all deliverable file paths exist relative
// to repoRoot. Paths may be absolute or relative (resolved against repoRoot).
// Returns a non-nil error listing every missing file if any are absent.
func ValidateDeliverables(repoRoot string, deliverables []string) error {
	if len(deliverables) == 0 {
		return nil
	}

	var missing []string
	for _, d := range deliverables {
		resolved := resolvePath(repoRoot, d)
		if _, err := os.Stat(resolved); err != nil {
			missing = append(missing, d)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("deliverable files not found in repo: %s", strings.Join(missing, ", "))
	}
	return nil
}

// resolvePath resolves a deliverable path against a repo root.
// Absolute paths are returned as-is; relative paths are joined with repoRoot.
func resolvePath(repoRoot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}
