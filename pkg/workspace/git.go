package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitConfigManager handles git configuration integration.
type GitConfigManager struct{}

// NewGitConfigManager creates a new git config manager.
func NewGitConfigManager() *GitConfigManager {
	return &GitConfigManager{}
}

// GitConfig represents git configuration for a workspace.
type GitConfig struct {
	UserName   string
	UserEmail  string
	SigningKey string
	CommitSign bool
	SSHCommand string
}

// GetGitConfigForPath returns the effective git config for a given path.
func (g *GitConfigManager) GetGitConfigForPath(path string) (*GitConfig, error) {
	config := &GitConfig{}

	// Get git config values
	if userName, err := g.getGitConfig(path, "user.name"); err == nil {
		config.UserName = userName
	}

	if userEmail, err := g.getGitConfig(path, "user.email"); err == nil {
		config.UserEmail = userEmail
	}

	if signingKey, err := g.getGitConfig(path, "user.signingkey"); err == nil {
		config.SigningKey = signingKey
	}

	if commitSign, err := g.getGitConfig(path, "commit.gpgsign"); err == nil {
		config.CommitSign = strings.ToLower(commitSign) == "true"
	}

	if sshCommand, err := g.getGitConfig(path, "core.sshCommand"); err == nil {
		config.SSHCommand = sshCommand
	}

	return config, nil
}

// getGitConfig runs git config command for a specific path.
func (g *GitConfigManager) getGitConfig(path, key string) (string, error) {
	cmd := exec.Command("git", "config", "--get", key)
	cmd.Dir = path

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// GenerateGitConfigFile generates a workspace-specific git config file.
func (g *GitConfigManager) GenerateGitConfigFile(workspaceName string, config GitConfig) (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		home = "~"
	}

	configPath := filepath.Join(home, fmt.Sprintf(".gitconfig-%s", workspaceName))
	expandedPath := ExpandHome(configPath)

	// Generate config content
	var content strings.Builder

	fmt.Fprintf(&content, "# Git configuration for workspace: %s\n\n", workspaceName)

	if config.UserName != "" || config.UserEmail != "" {
		content.WriteString("[user]\n")
		if config.UserName != "" {
			fmt.Fprintf(&content, "    name = %s\n", config.UserName)
		}
		if config.UserEmail != "" {
			fmt.Fprintf(&content, "    email = %s\n", config.UserEmail)
		}
		if config.SigningKey != "" {
			fmt.Fprintf(&content, "    signingkey = %s\n", config.SigningKey)
		}
		content.WriteString("\n")
	}

	if config.CommitSign {
		content.WriteString("[commit]\n")
		content.WriteString("    gpgsign = true\n\n")
	}

	if config.SSHCommand != "" {
		content.WriteString("[core]\n")
		fmt.Fprintf(&content, "    sshCommand = %s\n\n", config.SSHCommand)
	}

	// Write file
	if err := os.WriteFile(expandedPath, []byte(content.String()), 0o600); err != nil {
		return "", fmt.Errorf("failed to write git config file: %w", err)
	}

	return expandedPath, nil
}

// AddGitIncludeIf adds an includeIf directive to the global git config.
func (g *GitConfigManager) AddGitIncludeIf(workspaceRoot, configPath string) error {
	// Get global git config path
	home := os.Getenv("HOME")
	if home == "" {
		return fmt.Errorf("HOME environment variable not set")
	}

	globalConfigPath := filepath.Join(home, ".gitconfig")

	// Read existing config
	content, err := os.ReadFile(globalConfigPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read global git config: %w", err)
	}

	existingContent := string(content)

	// Check if includeIf already exists
	includeIfLine := fmt.Sprintf("[includeIf \"gitdir:%s/\"]", workspaceRoot)
	if strings.Contains(existingContent, includeIfLine) {
		// Already exists, nothing to do
		return nil
	}

	// Append includeIf directive
	var newContent strings.Builder
	newContent.WriteString(existingContent)

	if !strings.HasSuffix(existingContent, "\n") && existingContent != "" {
		newContent.WriteString("\n")
	}

	newContent.WriteString("\n")
	newContent.WriteString(includeIfLine + "\n")
	fmt.Fprintf(&newContent, "    path = %s\n", configPath)

	// Write updated config
	if err := os.WriteFile(globalConfigPath, []byte(newContent.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write global git config: %w", err)
	}

	return nil
}

// ValidateGitConfig checks if git config is correctly set for a workspace.
type GitConfigValidation struct {
	WorkspaceName string
	ExpectedEmail string
	ActualEmail   string
	Match         bool
	Issues        []string
}

// ValidateGitConfigForWorkspace validates git config for a workspace directory.
func (g *GitConfigManager) ValidateGitConfigForWorkspace(workspaceRoot, expectedEmail string) (*GitConfigValidation, error) {
	validation := &GitConfigValidation{
		ExpectedEmail: expectedEmail,
		Match:         true,
		Issues:        []string{},
	}

	// Get actual git config
	config, err := g.GetGitConfigForPath(workspaceRoot)
	if err != nil {
		validation.Match = false
		validation.Issues = append(validation.Issues, fmt.Sprintf("Failed to get git config: %v", err))
		return validation, nil
	}

	validation.ActualEmail = config.UserEmail

	// Check email match
	if config.UserEmail != expectedEmail {
		validation.Match = false
		validation.Issues = append(validation.Issues,
			fmt.Sprintf("Email mismatch: expected '%s', got '%s'", expectedEmail, config.UserEmail))
	}

	return validation, nil
}

// GitConfigDoctor performs health checks on git configuration.
type GitConfigDoctorResult struct {
	Checks   []GitConfigCheck
	Passed   int
	Warnings int
	Errors   int
}

// GitConfigCheck represents a single git config health check.
type GitConfigCheck struct {
	Name    string
	Status  string // "passed", "warning", "error"
	Message string
	Details string
}

// Doctor performs comprehensive git configuration health checks.
func (g *GitConfigManager) Doctor(registry *Registry) (*GitConfigDoctorResult, error) {
	result := &GitConfigDoctorResult{
		Checks: []GitConfigCheck{},
	}

	// Check 1: Global .gitconfig exists
	home := os.Getenv("HOME")
	globalConfigPath := filepath.Join(home, ".gitconfig")

	if _, err := os.Stat(globalConfigPath); os.IsNotExist(err) {
		result.Checks = append(result.Checks, GitConfigCheck{
			Name:    "global-gitconfig-exists",
			Status:  "error",
			Message: "Global .gitconfig not found",
			Details: fmt.Sprintf("Expected: %s", globalConfigPath),
		})
		result.Errors++
	} else {
		result.Checks = append(result.Checks, GitConfigCheck{
			Name:    "global-gitconfig-exists",
			Status:  "passed",
			Message: "Global .gitconfig exists",
		})
		result.Passed++
	}

	// Check 2: Workspace-specific configs exist
	for _, ws := range registry.Workspaces {
		if !ws.Enabled {
			continue
		}

		wsConfigPath := filepath.Join(home, fmt.Sprintf(".gitconfig-%s", ws.Name))
		if _, err := os.Stat(wsConfigPath); os.IsNotExist(err) {
			result.Checks = append(result.Checks, GitConfigCheck{
				Name:    fmt.Sprintf("workspace-gitconfig-%s", ws.Name),
				Status:  "warning",
				Message: fmt.Sprintf("Workspace config for '%s' not found", ws.Name),
				Details: fmt.Sprintf("Expected: %s", wsConfigPath),
			})
			result.Warnings++
		} else {
			result.Checks = append(result.Checks, GitConfigCheck{
				Name:    fmt.Sprintf("workspace-gitconfig-%s", ws.Name),
				Status:  "passed",
				Message: fmt.Sprintf("Workspace config for '%s' exists", ws.Name),
			})
			result.Passed++
		}
	}

	return result, nil
}
