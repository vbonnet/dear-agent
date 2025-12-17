package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// ResolveIdentifier finds a manifest by tmux name, workspace ID, or session ID
func ResolveIdentifier(identifier string, sessionsDir string) (*manifest.Manifest, string, error) {
	// Try as session ID (workspace ID) first
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

	// Try as manifest Name (v2 field)
	for _, m := range manifests {
		if m.Name == identifier {
			manifestPath := filepath.Join(sessionsDir, m.SessionID, "manifest.yaml")
			return m, manifestPath, nil
		}
	}

	return nil, "", fmt.Errorf("session not found: %s", identifier)
}

// HealthReport contains health check results
type HealthReport struct {
	WorktreeExists bool
	Issues         []string
}

// CheckHealth validates that all paths in manifest exist
func CheckHealth(m *manifest.Manifest) (*HealthReport, error) {
	report := &HealthReport{
		Issues: []string{},
	}

	// Check working directory (v2: Context.Project)
	if _, err := os.Stat(m.Context.Project); err != nil {
		report.WorktreeExists = false
		report.Issues = append(report.Issues, fmt.Sprintf("Working directory does not exist: %s", m.Context.Project))
	} else {
		report.WorktreeExists = true
	}

	return report, nil
}

// IsHealthy returns true if all health checks pass
func (r *HealthReport) IsHealthy() bool {
	return r.WorktreeExists
}

// Summary returns a human-readable summary of health issues
func (r *HealthReport) Summary() string {
	if r.IsHealthy() {
		return "All health checks passed"
	}
	return strings.Join(r.Issues, "\n")
}
