package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HarnessType represents a supported coding agent harness
type HarnessType string

// Recognized HarnessType values.
const (
	HarnessCodex    HarnessType = "codex"
	HarnessGemini   HarnessType = "gemini"
	HarnessOpenCode HarnessType = "opencode"
)

// ValidateHarness checks if the harness type is supported
func ValidateHarness(harness string) (HarnessType, error) {
	switch strings.ToLower(harness) {
	case "codex":
		return HarnessCodex, nil
	case "gemini":
		return HarnessGemini, nil
	case "opencode":
		return HarnessOpenCode, nil
	default:
		return "", fmt.Errorf("unsupported harness: %s (supported: codex, gemini, opencode)", harness)
	}
}

// HarnessInstallResult represents the result of an installation attempt
type HarnessInstallResult struct {
	Success       bool   `json:"success"`
	Harness       string `json:"harness"`
	Message       string `json:"message"`
	Version       string `json:"version,omitempty"`
	Path          string `json:"path,omitempty"`
	SkipReason    string `json:"skip_reason,omitempty"`
	ErrorDetails  string `json:"error_details,omitempty"`
}

// IsInstalled checks if a harness is available in the system PATH
func IsInstalled(ctx context.Context, harness HarnessType) (bool, string, error) {
	var cmd string
	switch harness {
	case HarnessCodex:
		cmd = "codex"
	case HarnessGemini:
		cmd = "gemini"
	case HarnessOpenCode:
		cmd = "opencode"
	default:
		return false, "", fmt.Errorf("unknown harness: %s", harness)
	}

	path, err := exec.LookPath(cmd)
	if err != nil {
		return false, "", nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}
	return true, path, nil
}

// GetVersion retrieves the version of an installed harness
func GetVersion(ctx context.Context, harness HarnessType) (string, error) {
	var cmd string
	var args []string

	switch harness {
	case HarnessCodex:
		cmd = "codex"
		args = []string{"--version"}
	case HarnessGemini:
		cmd = "gemini"
		args = []string{"--version"}
	case HarnessOpenCode:
		cmd = "opencode"
		args = []string{"--version"}
	default:
		return "", fmt.Errorf("unknown harness: %s", harness)
	}

	cmdExec := exec.CommandContext(ctx, cmd, args...)
	output, err := cmdExec.CombinedOutput()
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(string(output))
	return version, nil
}

// InstallCodex installs the Codex CLI binary
func InstallCodex(ctx context.Context) *HarnessInstallResult {
	result := &HarnessInstallResult{
		Harness: string(HarnessCodex),
	}

	// Check if already installed
	installed, path, err := IsInstalled(ctx, HarnessCodex)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Error checking installation: %v", err)
		result.ErrorDetails = err.Error()
		return result
	}

	if installed {
		result.Success = true
		result.Message = "Codex CLI is already installed"
		result.Path = path

		// Try to get version
		if version, err := GetVersion(ctx, HarnessCodex); err == nil {
			result.Version = version
		}
		result.SkipReason = "already_installed"
		return result
	}

	// Attempt installation via npm
	installCmd := exec.CommandContext(ctx, "npm", "install", "-g", "@openai/codex")
	var stdOut, stdErr bytes.Buffer
	installCmd.Stdout = &stdOut
	installCmd.Stderr = &stdErr

	if err := installCmd.Run(); err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to install Codex CLI via npm: %v", err)
		result.ErrorDetails = stdErr.String()
		return result
	}

	// Verify installation
	installed, path, verifyErr := IsInstalled(ctx, HarnessCodex)
	if verifyErr != nil || !installed {
		result.Success = false
		result.Message = "Installation completed but codex binary not found in PATH"
		if verifyErr != nil {
			result.ErrorDetails = verifyErr.Error()
		}
		return result
	}

	result.Success = true
	result.Message = "Codex CLI installed successfully via npm"
	result.Path = path

	// Try to get version
	if version, err := GetVersion(ctx, HarnessCodex); err == nil {
		result.Version = version
	}

	return result
}

// InstallGemini installs the Gemini CLI binary (via linuxbrew)
func InstallGemini(ctx context.Context) *HarnessInstallResult {
	result := &HarnessInstallResult{
		Harness: string(HarnessGemini),
	}

	// Check if already installed
	installed, path, err := IsInstalled(ctx, HarnessGemini)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Error checking installation: %v", err)
		result.ErrorDetails = err.Error()
		return result
	}

	if installed {
		result.Success = true
		result.Message = "Gemini CLI is already installed"
		result.Path = path

		// Try to get version
		if version, err := GetVersion(ctx, HarnessGemini); err == nil {
			result.Version = version
		}
		result.SkipReason = "already_installed"
		return result
	}

	// Attempt installation via linuxbrew
	brewCmd := findBrewCommand()
	if brewCmd == "" {
		result.Success = false
		result.Message = "Linuxbrew not found; cannot install Gemini"
		return result
	}

	installCmd := exec.CommandContext(ctx, brewCmd, "install", "gemini")
	var stdOut, stdErr bytes.Buffer
	installCmd.Stdout = &stdOut
	installCmd.Stderr = &stdErr

	if err := installCmd.Run(); err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to install Gemini via linuxbrew: %v", err)
		result.ErrorDetails = stdErr.String()
		return result
	}

	// Verify installation
	installed, path, verifyErr := IsInstalled(ctx, HarnessGemini)
	if verifyErr != nil || !installed {
		result.Success = false
		result.Message = "Installation completed but gemini binary not found in PATH"
		if verifyErr != nil {
			result.ErrorDetails = verifyErr.Error()
		}
		return result
	}

	result.Success = true
	result.Message = "Gemini CLI installed successfully via linuxbrew"
	result.Path = path

	// Try to get version
	if version, err := GetVersion(ctx, HarnessGemini); err == nil {
		result.Version = version
	}

	return result
}

// InstallOpenCode installs the OpenCode CLI binary (via linuxbrew)
func InstallOpenCode(ctx context.Context) *HarnessInstallResult {
	result := &HarnessInstallResult{
		Harness: string(HarnessOpenCode),
	}

	// Check if already installed
	installed, path, err := IsInstalled(ctx, HarnessOpenCode)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Error checking installation: %v", err)
		result.ErrorDetails = err.Error()
		return result
	}

	if installed {
		result.Success = true
		result.Message = "OpenCode CLI is already installed"
		result.Path = path

		// Try to get version
		if version, err := GetVersion(ctx, HarnessOpenCode); err == nil {
			result.Version = version
		}
		result.SkipReason = "already_installed"
		return result
	}

	// Attempt installation via linuxbrew
	brewCmd := findBrewCommand()
	if brewCmd == "" {
		result.Success = false
		result.Message = "Linuxbrew not found; cannot install OpenCode"
		return result
	}

	installCmd := exec.CommandContext(ctx, brewCmd, "install", "opencode")
	var stdOut, stdErr bytes.Buffer
	installCmd.Stdout = &stdOut
	installCmd.Stderr = &stdErr

	if err := installCmd.Run(); err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to install OpenCode via linuxbrew: %v", err)
		result.ErrorDetails = stdErr.String()
		return result
	}

	// Verify installation
	installed, path, verifyErr := IsInstalled(ctx, HarnessOpenCode)
	if verifyErr != nil || !installed {
		result.Success = false
		result.Message = "Installation completed but opencode binary not found in PATH"
		if verifyErr != nil {
			result.ErrorDetails = verifyErr.Error()
		}
		return result
	}

	result.Success = true
	result.Message = "OpenCode CLI installed successfully via linuxbrew"
	result.Path = path

	// Try to get version
	if version, err := GetVersion(ctx, HarnessOpenCode); err == nil {
		result.Version = version
	}

	return result
}

// Install installs a harness and returns the result as JSON
func Install(ctx context.Context, harness HarnessType) (*HarnessInstallResult, error) {
	switch harness {
	case HarnessCodex:
		return InstallCodex(ctx), nil
	case HarnessGemini:
		return InstallGemini(ctx), nil
	case HarnessOpenCode:
		return InstallOpenCode(ctx), nil
	default:
		return nil, fmt.Errorf("unknown harness: %s", harness)
	}
}

// findBrewCommand locates the brew executable in common linuxbrew paths
func findBrewCommand() string {
	candidates := []string{
		"brew",
		"/home/linuxbrew/.linuxbrew/bin/brew",
		filepath.Join(os.Getenv("HOME"), ".linuxbrew/bin/brew"),
		"/usr/local/bin/brew",
	}

	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}

	return ""
}

// ResultToJSON converts a result to JSON
func ResultToJSON(result *HarnessInstallResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
