package workspace

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvManager handles environment variable isolation per workspace.
type EnvManager struct {
	registryPath string
}

// NewEnvManager creates a new environment manager.
func NewEnvManager(registryPath string) *EnvManager {
	if registryPath == "" {
		registryPath = GetDefaultRegistryPath()
	}
	return &EnvManager{
		registryPath: registryPath,
	}
}

// GetEnvFilePath returns the path to a workspace's env file.
func (e *EnvManager) GetEnvFilePath(workspaceName string) string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "~"
	}
	return filepath.Join(home, ".workspace", "envs", workspaceName+".env")
}

// LoadEnvFile loads environment variables from a workspace's .env file.
func (e *EnvManager) LoadEnvFile(workspaceName string) (map[string]string, error) {
	envPath := e.GetEnvFilePath(workspaceName)
	expandedPath := ExpandHome(envPath)

	// Check if file exists
	if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
		// Return empty map if file doesn't exist (not an error)
		return make(map[string]string), nil
	}

	// Read file
	file, err := os.Open(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open env file: %w", err)
	}
	defer file.Close()

	// Parse env file
	envVars := make(map[string]string)
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse line (support both "export KEY=value" and "KEY=value")
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid env file format at line %d: %s", lineNum, line)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = strings.Trim(value, `"'`)

		envVars[key] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading env file: %w", err)
	}

	return envVars, nil
}

// SaveEnvFile saves environment variables to a workspace's .env file.
func (e *EnvManager) SaveEnvFile(workspaceName string, envVars map[string]string) error {
	envPath := e.GetEnvFilePath(workspaceName)
	expandedPath := ExpandHome(envPath)

	// Ensure directory exists
	dir := filepath.Dir(expandedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create envs directory: %w", err)
	}

	// Create file
	file, err := os.Create(expandedPath)
	if err != nil {
		return fmt.Errorf("failed to create env file: %w", err)
	}
	defer file.Close()

	// Write header
	fmt.Fprintf(file, "# Environment variables for workspace: %s\n", workspaceName)
	fmt.Fprintf(file, "# This file contains workspace-specific credentials and settings\n\n")

	// Write env vars
	for key, value := range envVars {
		fmt.Fprintf(file, "export %s=%q\n", key, value)
	}

	// Set restrictive permissions (user-only read/write)
	if err := os.Chmod(expandedPath, 0600); err != nil {
		return fmt.Errorf("failed to set env file permissions: %w", err)
	}

	return nil
}

// GenerateActivationScript generates a shell script to activate a workspace.
func (e *EnvManager) GenerateActivationScript(workspaceName string) (string, error) {
	// Load registry to get workspace
	registry, err := LoadRegistry(e.registryPath)
	if err != nil {
		return "", err
	}

	ws, err := registry.GetWorkspaceByName(workspaceName)
	if err != nil {
		return "", err
	}

	// Load env vars
	envVars, err := e.LoadEnvFile(workspaceName)
	if err != nil {
		return "", err
	}

	// Generate script
	var script strings.Builder

	script.WriteString(fmt.Sprintf("# Workspace activation script for: %s\n\n", workspaceName))

	// Core workspace vars
	script.WriteString("# Core workspace variables\n")
	script.WriteString(fmt.Sprintf("export WORKSPACE_ROOT=%q\n", ws.Root))
	script.WriteString(fmt.Sprintf("export WORKSPACE_NAME=%q\n", ws.Name))
	script.WriteString(fmt.Sprintf("export WORKSPACE_ENABLED=%t\n", ws.Enabled))

	// Protocol settings
	if ws.OutputDir != "" {
		script.WriteString(fmt.Sprintf("export WORKSPACE_OUTPUT_DIR=%q\n", ws.OutputDir))
	}

	// Default cache dir
	cacheDir := filepath.Join(ws.Root, ".cache")
	script.WriteString(fmt.Sprintf("export WORKSPACE_CACHE_DIR=%q\n", cacheDir))

	// Load settings from registry
	if registry.DefaultSettings != nil {
		script.WriteString("\n# Default settings\n")
		for key, value := range registry.DefaultSettings {
			envKey := "WORKSPACE_" + strings.ToUpper(key)
			script.WriteString(fmt.Sprintf("export %s=%q\n", envKey, fmt.Sprint(value)))
		}
	}

	// Workspace-specific settings
	if ws.Settings != nil {
		script.WriteString("\n# Workspace settings\n")
		for key, value := range ws.Settings {
			envKey := "WORKSPACE_" + strings.ToUpper(key)
			script.WriteString(fmt.Sprintf("export %s=%q\n", envKey, fmt.Sprint(value)))
		}
	}

	// Custom env vars from .env file
	if len(envVars) > 0 {
		script.WriteString("\n# Workspace-specific environment\n")
		for key, value := range envVars {
			// Skip variables we already set
			if strings.HasPrefix(key, "WORKSPACE_") {
				continue
			}
			script.WriteString(fmt.Sprintf("export %s=%q\n", key, value))
		}
	}

	// Verification message
	script.WriteString(fmt.Sprintf("\necho \"Activated workspace: %s\"\n", workspaceName))
	script.WriteString(fmt.Sprintf("echo \"  Root: %s\"\n", ws.Root))

	return script.String(), nil
}

// MaskSensitiveValue masks sensitive environment variable values.
func MaskSensitiveValue(key, value string) string {
	// List of sensitive key patterns
	sensitivePatterns := []string{
		"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIALS",
		"API_KEY", "AUTH", "PRIVATE",
	}

	keyUpper := strings.ToUpper(key)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(keyUpper, pattern) {
			if len(value) > 7 {
				return value[:3] + "***"
			}
			return "***"
		}
	}

	return value
}
