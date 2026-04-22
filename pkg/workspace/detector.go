package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// detector implements the 6-priority detection algorithm.
type detector struct {
	config      *Config
	configPath  string
	interactive bool // Enable interactive prompts
}

// NewDetector creates a new workspace detector.
func NewDetector(configPath string) (Detector, error) {
	return NewDetectorWithInteractive(configPath, true)
}

// NewDetectorWithInteractive creates detector with interactive mode control.
func NewDetectorWithInteractive(configPath string, interactive bool) (Detector, error) {
	// Load config
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	return &detector{
		config:      config,
		configPath:  configPath,
		interactive: interactive,
	}, nil
}

// Detect implements the 6-priority detection algorithm.
//
// Priority order (highest to lowest):
// 1. Explicit --workspace flag
// 2. WORKSPACE environment variable
// 3. Auto-detect from PWD
// 4. Default workspace from config
// 5. Interactive prompt (if TTY)
// 6. Error
func (d *detector) Detect(pwd string, explicitWorkspace string) (*Workspace, error) {
	return d.DetectWithEnv(pwd, explicitWorkspace, "WORKSPACE")
}

// DetectWithEnv allows custom environment variable name.
func (d *detector) DetectWithEnv(pwd string, explicitWorkspace string, envVar string) (*Workspace, error) {
	// 1. Explicit --workspace flag (highest priority)
	if explicitWorkspace != "" {
		ws, err := d.GetWorkspace(explicitWorkspace)
		if err != nil {
			return nil, fmt.Errorf("unknown workspace '%s': %w", explicitWorkspace, err)
		}
		return ws, nil
	}

	// 2. Environment variable
	if envWorkspace := os.Getenv(envVar); envWorkspace != "" {
		ws, err := d.GetWorkspace(envWorkspace)
		if err != nil {
			return nil, fmt.Errorf("workspace '%s' from %s env not found: %w",
				envWorkspace, envVar, err)
		}
		return ws, nil
	}

	// 3. Auto-detect from PWD (walk up directory tree)
	if ws, err := d.detectFromPath(pwd); err == nil {
		return ws, nil
	}

	// 4. Default workspace from config
	if d.config.DefaultWorkspace != "" {
		ws, err := d.GetWorkspace(d.config.DefaultWorkspace)
		if err == nil {
			return ws, nil
		}
	}

	// 5. Interactive prompt (only if TTY)
	if d.interactive && isTTY(os.Stdin.Fd()) {
		prompter := NewPrompter()
		ws, err := prompter.PromptWorkspace(d.config.Workspaces)
		if err == nil {
			return ws, nil
		}
	}

	// 6. Error - no workspace found
	return nil, ErrNoWorkspaceFound
}

// detectFromPath walks up directory tree to find matching workspace.
func (d *detector) detectFromPath(path string) (*Workspace, error) {
	// Normalize path first
	absPath, err := NormalizePath(path)
	if err != nil {
		return nil, err
	}

	// Walk up from current directory
	current := absPath
	for {
		// Check all enabled workspaces for match
		for i := range d.config.Workspaces {
			ws := &d.config.Workspaces[i]
			if !ws.Enabled {
				continue
			}
			if matchWorkspace(current, ws) {
				return ws, nil
			}
		}

		// Move to parent directory
		parent := filepath.Dir(current)
		if parent == current {
			break // Reached root
		}
		current = parent
	}

	return nil, ErrNoMatchingWorkspace
}

// matchWorkspace checks if path is within workspace root.
func matchWorkspace(path string, ws *Workspace) bool {
	return IsSubpath(ws.Root, path)
}

// ListWorkspaces returns all workspaces (including disabled ones).
func (d *detector) ListWorkspaces() []Workspace {
	// Return copy to prevent modification
	workspaces := make([]Workspace, len(d.config.Workspaces))
	copy(workspaces, d.config.Workspaces)
	return workspaces
}

// GetWorkspace returns workspace by name.
func (d *detector) GetWorkspace(name string) (*Workspace, error) {
	for i := range d.config.Workspaces {
		ws := &d.config.Workspaces[i]
		if ws.Name == name {
			if !ws.Enabled {
				return nil, fmt.Errorf("%w: %s", ErrWorkspaceNotEnabled, name)
			}
			return ws, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, name)
}

// GetConfig returns the loaded configuration.
func (d *detector) GetConfig() *Config {
	// Return copy to prevent modification
	configCopy := *d.config
	configCopy.Workspaces = make([]Workspace, len(d.config.Workspaces))
	copy(configCopy.Workspaces, d.config.Workspaces)
	return &configCopy
}
