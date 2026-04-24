package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
)

// SessionPicker displays an interactive session picker with fuzzy search
func SessionPicker(sessions []*Session, cfg *Config) (*Session, error) {
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions available")
	}

	// Build options
	options := make([]huh.Option[string], len(sessions))
	sessionMap := make(map[string]*Session)

	for i, s := range sessions {
		label := formatSessionOption(s, cfg)
		options[i] = huh.NewOption(label, s.Name)
		sessionMap[s.Name] = s
	}

	// Create form with picker
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select session to resume").
				Description("Type to filter • ↑/↓ navigate • Enter to resume • Ctrl-C to cancel").
				Options(options...).
				Value(&selected).
				Height(cfg.UI.PickerHeight),
		),
	).WithTheme(getTheme(cfg.UI.Theme))

	if err := form.Run(); err != nil {
		return nil, err
	}

	// Return selected session
	return sessionMap[selected], nil
}

// formatSessionOption formats a session for display in the picker
func formatSessionOption(s *Session, cfg *Config) string {
	// Format: "name (status) [project] updated: 2h ago"
	parts := []string{s.Name}

	// Add status with color indicator
	status := s.Status
	if status == "" {
		status = "unknown"
	}
	parts = append(parts, fmt.Sprintf("(%s)", status))

	// Add project path if enabled
	if cfg.UI.ShowProjectPaths && s.Context.Project != "" {
		project := s.Context.Project
		if len(project) > 40 {
			project = "..." + project[len(project)-37:]
		}
		parts = append(parts, fmt.Sprintf("[%s]", project))
	}

	// Add relative time
	relTime := formatRelativeTime(s.UpdatedAt)
	parts = append(parts, fmt.Sprintf("updated: %s", relTime))

	return strings.Join(parts, " ")
}

// formatRelativeTime formats a time as relative (e.g., "2h ago")
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	dur := time.Since(t)

	if dur < time.Minute {
		return "just now"
	}
	if dur < time.Hour {
		mins := int(dur.Minutes())
		return fmt.Sprintf("%dm ago", mins)
	}
	if dur < 24*time.Hour {
		hours := int(dur.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	if dur < 7*24*time.Hour {
		days := int(dur.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}
	if dur < 30*24*time.Hour {
		weeks := int(dur.Hours() / 24 / 7)
		return fmt.Sprintf("%dw ago", weeks)
	}

	months := int(dur.Hours() / 24 / 30)
	return fmt.Sprintf("%dmo ago", months)
}

// getTheme returns the Huh theme based on config (internal)
func getTheme(themeName string) *huh.Theme {
	switch themeName {
	case "agm":
		return AGMTheme() // High-contrast custom theme (default)
	case "agm-light":
		return AGMThemeLight() // High-contrast theme for light terminals
	case "dracula":
		return huh.ThemeDracula()
	case "catppuccin":
		return huh.ThemeCatppuccin()
	case "charm":
		return huh.ThemeCharm()
	case "base":
		return huh.ThemeBase()
	default:
		return AGMTheme() // Use AGM theme as default (high contrast)
	}
}

// GetTheme returns the appropriate Huh theme based on global config (exported for cmd/agm)
func GetTheme() *huh.Theme {
	cfg := GetGlobalConfig()
	return getTheme(cfg.UI.Theme)
}

// ArchivedSessionInfo represents minimal info for archived session selection
type ArchivedSessionInfo struct {
	SessionID  string
	Name       string
	ArchivedAt string
	Tags       []string
	Project    string
}

// ArchivedSessionPicker displays selection UI for archived sessions
func ArchivedSessionPicker(sessions []ArchivedSessionInfo) (string, error) {
	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions available")
	}

	// Build options
	options := make([]huh.Option[string], len(sessions))
	for i, s := range sessions {
		label := formatArchivedSessionOption(s)
		options[i] = huh.NewOption(label, s.SessionID)
	}

	// Create form with picker (using default values)
	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select archived session to restore").
				Description("Type to filter • ↑/↓ navigate • Enter to restore • Ctrl-C to cancel").
				Options(options...).
				Value(&selected).
				Height(10), // Default height
		),
	).WithTheme(huh.ThemeBase()) // Default theme

	if err := form.Run(); err != nil {
		return "", err
	}

	return selected, nil
}

// formatArchivedSessionOption formats an archived session for display
func formatArchivedSessionOption(s ArchivedSessionInfo) string {
	// Format: "name [project] archived: 2025-12-01 tags: [tag1, tag2]"
	parts := []string{s.Name}

	// Add project (truncated)
	if s.Project != "" {
		project := s.Project
		if len(project) > 40 {
			project = "..." + project[len(project)-37:]
		}
		parts = append(parts, fmt.Sprintf("[%s]", project))
	}

	// Add archived date
	if s.ArchivedAt != "" && s.ArchivedAt != "unknown" {
		parts = append(parts, fmt.Sprintf("archived: %s", s.ArchivedAt))
	}

	// Add tags if present
	if len(s.Tags) > 0 {
		tagStr := strings.Join(s.Tags, ", ")
		if len(tagStr) > 30 {
			tagStr = tagStr[:27] + "..."
		}
		parts = append(parts, fmt.Sprintf("tags: [%s]", tagStr))
	}

	return strings.Join(parts, " ")
}
