package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Registry represents the unified workspace registry.
type Registry struct {
	Version          int                    `yaml:"version"`                     // Registry format version (1)
	ProtocolVersion  string                 `yaml:"protocol_version"`            // Workspace protocol version
	DefaultWorkspace string                 `yaml:"default_workspace,omitempty"` // Default workspace name
	DefaultSettings  map[string]interface{} `yaml:"default_settings,omitempty"`  // Global defaults
	Workspaces       []Workspace            `yaml:"workspaces"`                  // Workspace definitions
}

// GetDefaultRegistryPath returns the default registry location.
func GetDefaultRegistryPath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "~"
	}
	return filepath.Join(home, ".workspace", "registry.yaml")
}

// LoadRegistry loads the workspace registry from the default or specified path.
func LoadRegistry(path string) (*Registry, error) {
	// Use default path if empty
	if path == "" {
		path = GetDefaultRegistryPath()
	}

	// Expand path
	expandedPath := ExpandHome(path)

	// Check if file exists
	if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrConfigNotFound, expandedPath)
	}

	// Read file
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry file: %w", err)
	}

	// Parse YAML
	var registry Registry
	if err := yaml.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("failed to parse registry YAML: %w", err)
	}

	// Validate registry
	if err := ValidateRegistry(&registry); err != nil {
		return nil, err
	}

	// Expand paths in registry
	if err := ExpandRegistryPaths(&registry); err != nil {
		return nil, err
	}

	return &registry, nil
}

// SaveRegistry writes registry to YAML file.
func SaveRegistry(path string, registry *Registry) error {
	// Validate before saving
	if err := ValidateRegistry(registry); err != nil {
		return err
	}

	// Use default path if empty
	if path == "" {
		path = GetDefaultRegistryPath()
	}

	// Expand path
	expandedPath := ExpandHome(path)

	// Ensure directory exists
	dir := filepath.Dir(expandedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(registry)
	if err != nil {
		return fmt.Errorf("failed to marshal registry to YAML: %w", err)
	}

	// Write file with user-only permissions
	if err := os.WriteFile(expandedPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write registry file: %w", err)
	}

	return nil
}

// ValidateRegistry checks registry for errors.
func ValidateRegistry(registry *Registry) error {
	if registry == nil {
		return fmt.Errorf("%w: registry is nil", ErrInvalidConfig)
	}

	// Check version
	if registry.Version != 1 {
		return fmt.Errorf("%w: expected version 1, got %d", ErrInvalidVersion, registry.Version)
	}

	// Allow empty workspaces only if this is a newly initialized registry
	// Check at least one enabled workspace if workspaces exist
	hasEnabled := false
	workspaceNames := make(map[string]bool)

	for i, ws := range registry.Workspaces {
		// Check required fields
		if ws.Name == "" {
			return fmt.Errorf("%w: workspace %d has empty name", ErrInvalidConfig, i)
		}
		if ws.Root == "" {
			return fmt.Errorf("%w: workspace '%s' has empty root", ErrInvalidConfig, ws.Name)
		}

		// Check for duplicate names
		if workspaceNames[ws.Name] {
			return fmt.Errorf("%w: duplicate workspace name '%s'", ErrInvalidConfig, ws.Name)
		}
		workspaceNames[ws.Name] = true

		// Check if enabled
		if ws.Enabled {
			hasEnabled = true
		}
	}

	// Only require enabled workspaces if there are workspaces defined
	if len(registry.Workspaces) > 0 && !hasEnabled {
		return fmt.Errorf("%w", ErrNoEnabledWorkspaces)
	}

	// Check default workspace exists (if specified)
	if registry.DefaultWorkspace != "" {
		if !workspaceNames[registry.DefaultWorkspace] {
			return fmt.Errorf("%w: default workspace '%s' not found in workspaces list",
				ErrInvalidConfig, registry.DefaultWorkspace)
		}
	}

	return nil
}

// ExpandRegistryPaths expands ~ and environment variables in all paths.
func ExpandRegistryPaths(registry *Registry) error {
	for i := range registry.Workspaces {
		ws := &registry.Workspaces[i]

		// Expand root path
		expandedRoot, err := NormalizePath(ws.Root)
		if err != nil {
			return fmt.Errorf("invalid root path for workspace '%s': %w", ws.Name, err)
		}
		ws.Root = expandedRoot

		// Validate root is absolute
		if err := ValidateAbsolutePath(ws.Root); err != nil {
			return fmt.Errorf("workspace '%s' root: %w", ws.Name, err)
		}

		// Expand output_dir if specified
		if ws.OutputDir != "" {
			expandedOutput, err := NormalizePath(ws.OutputDir)
			if err != nil {
				return fmt.Errorf("invalid output_dir for workspace '%s': %w", ws.Name, err)
			}
			ws.OutputDir = expandedOutput
		} else {
			// Default to {root}/output
			ws.OutputDir = filepath.Join(ws.Root, "output")
		}
	}

	return nil
}

// InitializeRegistry creates a new empty registry.
func InitializeRegistry(path string) error {
	registry := &Registry{
		Version:         1,
		ProtocolVersion: "1.0.0",
		DefaultSettings: map[string]interface{}{
			"log_level":  "info",
			"log_format": "text",
		},
		Workspaces: []Workspace{},
	}

	// Use default path if empty
	if path == "" {
		path = GetDefaultRegistryPath()
	}

	// Expand path
	expandedPath := ExpandHome(path)

	// Ensure directory exists
	dir := filepath.Dir(expandedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create registry directory: %w", err)
	}

	// Marshal to YAML (skip validation for empty registry)
	data, err := yaml.Marshal(registry)
	if err != nil {
		return fmt.Errorf("failed to marshal registry to YAML: %w", err)
	}

	// Write file with user-only permissions
	if err := os.WriteFile(expandedPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write registry file: %w", err)
	}

	return nil
}

// AddWorkspace adds a workspace to the registry.
func (r *Registry) AddWorkspace(ws Workspace) error {
	// Check for duplicate name
	for _, existing := range r.Workspaces {
		if existing.Name == ws.Name {
			return fmt.Errorf("workspace '%s' already exists", ws.Name)
		}
	}

	// Add workspace
	r.Workspaces = append(r.Workspaces, ws)

	return nil
}

// RemoveWorkspace removes a workspace from the registry.
func (r *Registry) RemoveWorkspace(name string) error {
	for i, ws := range r.Workspaces {
		if ws.Name == name {
			r.Workspaces = append(r.Workspaces[:i], r.Workspaces[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("workspace '%s' not found", name)
}

// GetWorkspaceByName returns a workspace by name.
func (r *Registry) GetWorkspaceByName(name string) (*Workspace, error) {
	for i := range r.Workspaces {
		if r.Workspaces[i].Name == name {
			if !r.Workspaces[i].Enabled {
				return nil, fmt.Errorf("%w: %s", ErrWorkspaceNotEnabled, name)
			}
			return &r.Workspaces[i], nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrWorkspaceNotFound, name)
}
