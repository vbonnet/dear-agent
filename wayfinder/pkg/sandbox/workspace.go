package sandbox

import (
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/pkg/workspace"
)

// GetWorkspaceProjectsDir returns the workspace-specific projects directory
// using workspace detection. Falls back to ~/.wayfinder if detection fails.
func GetWorkspaceProjectsDir(explicitWorkspace string) string {
	// Try workspace detection
	configPath := workspace.GetDefaultConfigPath("wayfinder")
	detector, err := workspace.NewDetector(configPath)
	if err != nil {
		// No workspace config - use legacy path
		return filepath.Join(os.Getenv("HOME"), ".wayfinder")
	}

	// Detect workspace from current directory
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".wayfinder")
	}

	ws, err := detector.Detect(cwd, explicitWorkspace)
	if err != nil {
		// Detection failed - use legacy path
		return filepath.Join(os.Getenv("HOME"), ".wayfinder")
	}

	// Workspace detected! Use workspace-specific projects dir
	// Config should have projects_dir in settings, or use <root>/wf as default
	if projectsDir, ok := ws.Settings["projects_dir"].(string); ok {
		return workspace.ExpandHome(projectsDir)
	}

	// Fallback: workspace root + /wf
	return filepath.Join(ws.Root, "wf")
}
