// Package workspace provides workspace detection and configuration management.
// It allows tools to auto-detect workspace from current directory and load
// workspace-specific configurations.
package workspace

import "errors"

// Workspace represents a configured workspace.
type Workspace struct {
	Name      string         `yaml:"name"`       // User-friendly name (e.g., "oss", "acme")
	Root      string         `yaml:"root"`       // Absolute path to workspace root
	OutputDir string         `yaml:"output_dir"` // Default output directory
	Enabled   bool           `yaml:"enabled"`    // Can be disabled without deleting
	Settings  map[string]any `yaml:",inline"`    // Tool-specific settings
}

// Config represents the workspace configuration file.
type Config struct {
	Version          int            `yaml:"version"`                 // Schema version (currently 1)
	DefaultWorkspace string         `yaml:"default_workspace"`       // Fallback workspace name
	Workspaces       []Workspace    `yaml:"workspaces"`              // List of configured workspaces
	ToolSettings     map[string]any `yaml:"tool_settings,omitempty"` // Global tool settings
}

// Detector is the main interface for workspace detection.
type Detector interface {
	Detect(pwd string, explicitWorkspace string) (*Workspace, error)
	DetectWithEnv(pwd string, explicitWorkspace string, envVar string) (*Workspace, error)
	ListWorkspaces() []Workspace
	GetWorkspace(name string) (*Workspace, error)
	GetConfig() *Config
}

// DetectionMethod indicates how workspace was determined.
type DetectionMethod int

// DetectionMethod values describing how a workspace was selected.
const (
	MethodFlag DetectionMethod = iota
	MethodEnvVar
	MethodAutoDetect
	MethodDefault
	MethodInteractive
	MethodError
)

func (m DetectionMethod) String() string {
	switch m {
	case MethodFlag:
		return "flag"
	case MethodEnvVar:
		return "env_var"
	case MethodAutoDetect:
		return "auto_detect"
	case MethodDefault:
		return "default"
	case MethodInteractive:
		return "interactive"
	case MethodError:
		return "error"
	default:
		return "unknown"
	}
}

// Common errors
var (
	ErrNoWorkspaceFound    = errors.New("no workspace detected")
	ErrNoMatchingWorkspace = errors.New("no workspace matches current directory")
	ErrInvalidConfig       = errors.New("invalid workspace configuration")
	ErrWorkspaceNotEnabled = errors.New("workspace is disabled")
	ErrConfigNotFound      = errors.New("config file not found")
	ErrWorkspaceNotFound   = errors.New("workspace not found")
	ErrInvalidVersion      = errors.New("unsupported config version")
	ErrNoEnabledWorkspaces = errors.New("no enabled workspaces configured")
)
