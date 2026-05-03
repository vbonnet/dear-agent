package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// DetectWorkspace extracts workspace name using priority algorithm:
//  1. $WORKSPACE env var (explicit user/AGM setting)
//  2. AGM session query (read from current session manifest)
//  3. Path pattern detection (~/src/ws/{workspace}/wf/{project})
//
// Returns empty string if workspace cannot be determined
func DetectWorkspace(projectPath string) string {
	// Priority 1: Check $WORKSPACE environment variable
	if workspace := os.Getenv("WORKSPACE"); workspace != "" {
		if isValidWorkspaceName(workspace) {
			return workspace
		}
	}

	// Priority 2: Query AGM for current workspace
	if workspace := queryAGMWorkspace(); workspace != "" {
		if isValidWorkspaceName(workspace) {
			return workspace
		}
	}

	// Priority 3: Auto-detect from project path
	workspace := detectWorkspaceFromPath(projectPath)
	if workspace != "" {
		return workspace
	}

	return ""
}

// queryAGMWorkspace attempts to read workspace from AGM current session manifest
// Returns empty string if AGM session not found or workspace not set
func queryAGMWorkspace() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Try reading from AGM current session link
	currentSessionLink := filepath.Join(home, ".agm", "current-session")
	manifestPath, err := os.Readlink(currentSessionLink)
	if err != nil {
		// Symlink doesn't exist, try alternative paths
		// Check ~/.claude/current-session as fallback
		currentSessionLink = filepath.Join(home, ".claude", "current-session")
		manifestPath, err = os.Readlink(currentSessionLink)
		if err != nil {
			return ""
		}
	}

	// Read manifest file
	manifestFile := filepath.Join(manifestPath, "manifest.yaml")
	data, err := os.ReadFile(manifestFile)
	if err != nil {
		// Try JSON format as fallback
		manifestFile = filepath.Join(manifestPath, "manifest.json")
		data, err = os.ReadFile(manifestFile)
		if err != nil {
			return ""
		}
	}

	// Parse manifest to extract workspace field
	// AGM manifest v2 has a workspace field
	var manifest struct {
		Workspace string `json:"workspace" yaml:"workspace"`
	}

	// Try JSON first
	if err := json.Unmarshal(data, &manifest); err != nil {
		// YAML parsing would require yaml package, skip for now
		// Most AGM manifests are JSON anyway
		return ""
	}

	return manifest.Workspace
}

// detectWorkspaceFromPath extracts workspace from path pattern matching
// Expected path formats:
//   - Production: ~/src/ws/{workspace}/wf/{project}
//   - Test: /tmp/.../oss/wf/{project} or /tmp/.../acme/wf/{project}
func detectWorkspaceFromPath(projectPath string) string {
	// Normalize path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return ""
	}

	// Look for /ws/{workspace}/ pattern (production)
	parts := strings.Split(absPath, string(filepath.Separator))
	for i, part := range parts {
		if part == "ws" && i+1 < len(parts) {
			workspace := parts[i+1]
			// Validate workspace name (alphanumeric, hyphens, underscores)
			if isValidWorkspaceName(workspace) {
				return workspace
			}
		}
	}

	// Look for /{workspace}/wf/ pattern (test environments)
	// This handles paths like /tmp/.../oss/wf/ or /tmp/.../acme/wf/
	for i, part := range parts {
		if part == "wf" && i > 0 {
			workspace := parts[i-1]
			// Validate workspace name
			if isValidWorkspaceName(workspace) {
				return workspace
			}
		}
	}

	return ""
}

// isValidWorkspaceName checks if a workspace name is valid
func isValidWorkspaceName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// ProjectInfo contains metadata about a Wayfinder project
type ProjectInfo struct {
	ProjectPath  string
	SessionID    string
	Status       string
	CurrentPhase string
	Workspace    string
}

// ListProjects lists all Wayfinder projects in a workspace root
// Expected root format: ~/src/ws/{workspace}/wf/
// Returns list of projects found
func ListProjects(workspaceRoot string) ([]ProjectInfo, error) {
	var projects []ProjectInfo

	// Check if workspace root exists
	if _, err := os.Stat(workspaceRoot); err != nil {
		if os.IsNotExist(err) {
			// Empty workspace is valid, return empty list
			return projects, nil
		}
		return nil, err
	}

	// Walk the workspace root looking for WAYFINDER-STATUS.md files
	err := filepath.Walk(workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // Skip paths with errors
		}

		// Look for WAYFINDER-STATUS.md files
		if !info.IsDir() && info.Name() == status.StatusFilename {
			projectPath := filepath.Dir(path)

			// Load status to get project metadata
			st, err := status.Load(projectPath)
			if err != nil {
				return nil //nolint:nilerr // Skip invalid status files
			}

			// Detect workspace from path
			workspace := DetectWorkspace(projectPath)

			projects = append(projects, ProjectInfo{
				ProjectPath:  projectPath,
				SessionID:    st.SessionID,
				Status:       st.Status,
				CurrentPhase: st.CurrentPhase,
				Workspace:    workspace,
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return projects, nil
}

// GetWorkspaceRoot returns the workspace root directory from a project path
// Expected format: ~/src/ws/{workspace}/wf/
func GetWorkspaceRoot(projectPath string) string {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return ""
	}

	// Look for /ws/{workspace}/wf/ pattern
	parts := strings.Split(absPath, string(filepath.Separator))
	for i, part := range parts {
		if part == "ws" && i+1 < len(parts) && i+2 < len(parts) {
			workspace := parts[i+1]
			if isValidWorkspaceName(workspace) && parts[i+2] == "wf" {
				// Reconstruct path up to /wf/
				rootParts := parts[:i+3]
				return filepath.Join(rootParts...)
			}
		}
	}

	return ""
}

// ValidateWorkspaceIsolation checks if a project path is within its expected workspace
func ValidateWorkspaceIsolation(projectPath, expectedWorkspace string) bool {
	actualWorkspace := DetectWorkspace(projectPath)
	return actualWorkspace == expectedWorkspace
}
