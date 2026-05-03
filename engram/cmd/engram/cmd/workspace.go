package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/pkg/workspace"
)

// loadConfigWithWorkspace loads engram config and detects workspace.
// It uses the following priority for workspace detection:
// 1. Explicit --workspace flag (highest priority)
// 2. WORKSPACE environment variable
// 3. Auto-detect from current working directory
// 4. Default workspace from config
// 5. Interactive prompt (if TTY)
// 6. Fall back to ~/.engram (backward compatible)
func loadConfigWithWorkspace() (*config.Config, error) {
	// Load engram config first
	loader := config.NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Attempt workspace detection (optional - don't fail if it doesn't work)
	workspaceConfigPath := cfg.WorkspaceConfigPath
	if workspaceConfigPath == "" {
		workspaceConfigPath = workspace.GetDefaultConfigPath("engram")
	}

	detector, err := workspace.NewDetector(workspaceConfigPath)
	if err != nil {
		// Workspace config not found or invalid - use legacy behavior
		return cfg, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Detect workspace using current working directory
	cwd, err := os.Getwd()
	if err != nil {
		// Can't get cwd - use legacy behavior
		return cfg, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	ws, err := detector.Detect(cwd, workspaceFlag)
	if err != nil {
		// Workspace detection failed - use legacy behavior
		return cfg, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Workspace detected successfully!
	cfg.Workspace = ws.Name

	// Override EngramPath with workspace-specific path
	if ws.OutputDir != "" {
		cfg.Platform.EngramPath = ws.OutputDir
	} else {
		// Fallback: use workspace root + .engram
		cfg.Platform.EngramPath = filepath.Join(ws.Root, ".engram")
	}

	return cfg, nil
}

// getEngramBasePath returns the base path for engram storage.
// It uses workspace detection if available, otherwise falls back to:
// 1. ENGRAM_HOME environment variable
// 2. ~/.engram (default)
//
// This function is intended for backward compatibility with existing
// commands that don't yet use the full config system.
func getEngramBasePath() (string, error) {
	// Try workspace detection first
	cfg, err := loadConfigWithWorkspace()
	if err == nil && cfg.Platform.EngramPath != "" {
		return cfg.Platform.EngramPath, nil
	}

	// Fallback: ENGRAM_HOME or ~/.engram
	basePath := os.Getenv("ENGRAM_HOME")
	if basePath != "" {
		return basePath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(home, ".engram"), nil
}
