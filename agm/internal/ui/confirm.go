package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/vbonnet/dear-agent/agm/internal/fuzzy"
)

// ConfirmCreate asks user to confirm creating a new session
func ConfirmCreate(name, project string, cfg *Config) (bool, error) {
	var confirmed bool

	desc := fmt.Sprintf("Project: %s\nTmux session will be created and Claude will start", project)

	err := huh.NewConfirm().
		Title(fmt.Sprintf("Create new session '%s'?", name)).
		Description(desc).
		Affirmative("Yes, create").
		Negative("Cancel").
		Value(&confirmed).
		WithTheme(getTheme(cfg.UI.Theme)).
		Run()

	return confirmed, err
}

// DidYouMean shows fuzzy match suggestions when session not found
func DidYouMean(input string, matches []fuzzy.Match, cfg *Config) (string, error) {
	options := make([]huh.Option[string], 0, len(matches)+2)

	// Add fuzzy matches
	for _, m := range matches {
		label := fmt.Sprintf("%s (%.0f%% match)", m.Name, m.Similarity*100)
		options = append(options, huh.NewOption(label, m.Name))
	}

	// Add create new option
	createLabel := fmt.Sprintf("Create new session '%s'", input)
	options = append(options, huh.NewOption(createLabel, "__CREATE__"))

	// Add cancel option
	options = append(options, huh.NewOption("Cancel", "__CANCEL__"))

	var choice string
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Session '%s' not found. Did you mean:", input)).
		Options(options...).
		Value(&choice).
		WithTheme(getTheme(cfg.UI.Theme)).
		Run()

	if err != nil || choice == "__CANCEL__" {
		return "", fmt.Errorf("cancelled")
	}

	if choice == "__CREATE__" {
		return "", nil // Empty string signals "create new"
	}

	return choice, nil
}

// ConfirmCleanup confirms batch archive/delete operations
func ConfirmCleanup(toArchive, toDelete []string, cfg *Config) (bool, error) {
	if len(toArchive) == 0 && len(toDelete) == 0 {
		return false, fmt.Errorf("no sessions selected")
	}

	var confirmed bool
	err := huh.NewConfirm().
		Title("Confirm cleanup").
		Description(cleanupConfirmationDescription(toArchive, toDelete)).
		Affirmative("Yes, proceed").
		Negative("Cancel").
		Value(&confirmed).
		WithTheme(getTheme(cfg.UI.Theme)).
		Run()

	return confirmed, err
}

func cleanupConfirmationDescription(toArchive, toDelete []string) string {
	var desc strings.Builder
	writeCleanupConfirmationTargets(&desc, "Archive", toArchive)
	writeCleanupConfirmationTargets(&desc, "Delete", toDelete)
	desc.WriteString("\nThis action cannot be undone for deleted sessions.")
	return desc.String()
}

func writeCleanupConfirmationTargets(desc *strings.Builder, action string, targets []string) {
	if len(targets) == 0 {
		return
	}
	fmt.Fprintf(desc, "%s %d session", action, len(targets))
	if len(targets) != 1 {
		desc.WriteByte('s')
	}
	desc.WriteString(":\n")
	for _, target := range targets {
		fmt.Fprintf(desc, "  - %s\n", target)
	}
}
