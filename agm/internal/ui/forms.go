package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
)

// NewSessionForm shows multi-step form for creating a new session
func NewSessionForm(existingSessions []*Session, cfg *Config) (*NewSessionFormData, error) {
	var data NewSessionFormData

	// Build map of existing session names for validation
	existingNames := make(map[string]bool)
	for _, s := range existingSessions {
		existingNames[s.Name] = true
	}

	// Step 1: Session name with validation
	nameGroup := huh.NewGroup(
		huh.NewInput().
			Title("Session Name").
			Description("Unique identifier for this session (alphanumeric, hyphens, underscores)").
			Placeholder("my-feature-work").
			Value(&data.Name).
			Validate(func(s string) error {
				return validateSessionName(s, existingNames)
			}),
	)

	// Step 2: Project path with smart defaults
	cwd, _ := os.Getwd()
	data.Project = cwd // Default to current directory

	projectGroup := huh.NewGroup(
		huh.NewInput().
			Title("Project Directory").
			Description("Path to your project (absolute or relative)").
			Placeholder(cwd).
			Value(&data.Project).
			Validate(validateProjectPath),
	)

	// Step 3: Purpose/description (optional)
	purposeGroup := huh.NewGroup(
		huh.NewText().
			Title("Session Purpose (Optional)").
			Description("Brief description of what you're working on").
			Placeholder("Implementing user authentication feature").
			Value(&data.Purpose).
			CharLimit(200),
	)

	// Create multi-step form with navigation
	form := huh.NewForm(
		nameGroup,
		projectGroup,
		purposeGroup,
	).WithTheme(getTheme(cfg.UI.Theme))

	if err := form.Run(); err != nil {
		return nil, err
	}

	// Clean up inputs
	data.Name = strings.TrimSpace(data.Name)
	data.Project = expandPath(data.Project)
	data.Purpose = strings.TrimSpace(data.Purpose)

	return &data, nil
}

// validateSessionName ensures name follows rules and doesn't conflict
func validateSessionName(name string, existing map[string]bool) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}

	// Must be alphanumeric with hyphens/underscores
	validName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("name must contain only letters, numbers, hyphens, and underscores")
	}

	// Check length constraints
	if len(name) < 2 {
		return fmt.Errorf("name must be at least 2 characters")
	}
	if len(name) > 64 {
		return fmt.Errorf("name must be 64 characters or less")
	}

	// Check for conflicts
	if existing[name] {
		return fmt.Errorf("session '%s' already exists", name)
	}

	return nil
}

// validateProjectPath ensures path is valid
func validateProjectPath(path string) error {
	path = strings.TrimSpace(path)

	if path == "" {
		return fmt.Errorf("project path cannot be empty")
	}

	// Expand path for validation
	expanded := expandPath(path)

	// Check if path exists
	info, err := os.Stat(expanded)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", expanded)
		}
		return fmt.Errorf("cannot access path: %w", err)
	}

	// Must be a directory
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory, not a file")
	}

	return nil
}

// expandPath handles ~ and relative paths
func expandPath(path string) string {
	path = strings.TrimSpace(path)

	// Handle empty
	if path == "" {
		return path
	}

	// Expand tilde
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	// Convert to absolute
	absPath, err := filepath.Abs(path)
	if err == nil {
		return absPath
	}

	return path
}
