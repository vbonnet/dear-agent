package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// ResolveIdentifier finds a manifest by tmux name, workspace ID, or Claude UUID
func ResolveIdentifier(identifier string, sessionsDir string) (*manifest.Manifest, string, error) {
	// Try to validate as UUID first
	if err := manifest.ValidateUUID(identifier); err == nil {
		// It's a UUID, search by claude.session_id
		manifests, err := manifest.List(sessionsDir)
		if err != nil {
			return nil, "", fmt.Errorf("failed to list manifests: %w", err)
		}

		for _, m := range manifests {
			if m.Claude.SessionID == identifier {
				manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
				return m, manifestPath, nil
			}
		}
		return nil, "", fmt.Errorf("session not found: %s", identifier)
	}

	// Try as session ID (workspace ID)
	manifestPath := filepath.Join(sessionsDir, identifier, "manifest.yaml")
	if _, err := os.Stat(manifestPath); err == nil {
		m, err := manifest.Read(manifestPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read manifest: %w", err)
		}
		return m, manifestPath, nil
	}

	// Try as tmux name
	manifests, err := manifest.List(sessionsDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list manifests: %w", err)
	}

	for _, m := range manifests {
		if m.Tmux.SessionName == identifier {
			manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
			return m, manifestPath, nil
		}
	}

	return nil, "", fmt.Errorf("session not found: %s", identifier)
}

// HealthReport contains health check results
type HealthReport struct {
	WorktreeExists    bool
	SessionEnvExists  bool
	FileHistoryExists bool
	Issues            []string
}

// CheckHealth validates that all paths in manifest exist
func CheckHealth(m *manifest.Manifest) (*HealthReport, error) {
	report := &HealthReport{
		Issues: []string{},
	}

	// Check worktree path
	if _, err := os.Stat(m.Worktree.Path); err != nil {
		report.WorktreeExists = false
		report.Issues = append(report.Issues, fmt.Sprintf("Worktree path does not exist: %s", m.Worktree.Path))
	} else {
		report.WorktreeExists = true
	}

	// Check session env path
	if _, err := os.Stat(m.Claude.SessionEnvPath); err != nil {
		report.SessionEnvExists = false
		report.Issues = append(report.Issues, fmt.Sprintf("Session env path does not exist: %s", m.Claude.SessionEnvPath))
	} else {
		report.SessionEnvExists = true
	}

	// Check file history path
	if _, err := os.Stat(m.Claude.FileHistoryPath); err != nil {
		report.FileHistoryExists = false
		report.Issues = append(report.Issues, fmt.Sprintf("File history path does not exist: %s", m.Claude.FileHistoryPath))
	} else {
		report.FileHistoryExists = true
	}

	return report, nil
}

// IsHealthy returns true if all health checks pass
func (r *HealthReport) IsHealthy() bool {
	return r.WorktreeExists && r.SessionEnvExists && r.FileHistoryExists
}

// Summary returns a human-readable summary of health issues
func (r *HealthReport) Summary() string {
	if r.IsHealthy() {
		return "All health checks passed"
	}
	return strings.Join(r.Issues, "\n")
}
