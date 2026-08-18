package manifest

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateSandboxOwnership verifies that sandbox metadata is a complete,
// self-consistent ownership record for one stable AGM session. It validates
// attribution and structural boundaries only; host-specific cleanup also
// verifies the sandbox is under the current user's configured sandbox base.
func ValidateSandboxOwnership(sessionID string, sandbox *SandboxConfig) error {
	if sessionID == "" {
		return fmt.Errorf("session ID is required")
	}
	if sandbox == nil {
		return fmt.Errorf("sandbox metadata is required")
	}
	if !sandbox.Enabled {
		return fmt.Errorf("sandbox metadata is not enabled")
	}
	if sandbox.ID != sessionID {
		return fmt.Errorf("sandbox ID %q does not match session ID %q", sandbox.ID, sessionID)
	}
	if strings.TrimSpace(sandbox.Provider) == "" {
		return fmt.Errorf("sandbox provider is required")
	}
	if sandbox.CreatedAt.IsZero() {
		return fmt.Errorf("sandbox creation time is required")
	}
	if err := validateCleanAbsolutePath("merged path", sandbox.MergedPath); err != nil {
		return err
	}
	if err := validateCleanAbsolutePath("working directory", sandbox.WorkingDir); err != nil {
		return err
	}

	sandboxDir := filepath.Dir(sandbox.MergedPath)
	if filepath.Base(sandbox.MergedPath) != "merged" || filepath.Base(sandboxDir) != sandbox.ID {
		return fmt.Errorf("sandbox merged path %q is not the identified sandbox cleanup boundary", sandbox.MergedPath)
	}
	rel, err := filepath.Rel(sandbox.MergedPath, sandbox.WorkingDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("sandbox working directory %q is outside merged path %q", sandbox.WorkingDir, sandbox.MergedPath)
	}
	return nil
}

func validateCleanAbsolutePath(field, value string) error {
	if !filepath.IsAbs(value) {
		return fmt.Errorf("sandbox %s %q is not absolute", field, value)
	}
	if filepath.Clean(value) != value {
		return fmt.Errorf("sandbox %s %q is not clean", field, value)
	}
	return nil
}
