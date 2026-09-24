package ui

import (
	"context"
	"fmt"
	"io"
	"os"
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
	return confirmCleanupWithIO(toArchive, toDelete, cfg, os.Stderr, os.Stdin)
}

// ConfirmCleanupContext is ConfirmCleanup that gives up when ctx is canceled.
//
// huh's RunAccessible takes no context and blocks on stdin, so SIGINT during
// the prompt cancelled the command context while the process sat waiting for
// input that was never coming. The cleanup loops downstream therefore never
// reached their own cancellation checks, and Ctrl-C left `agm admin clean`
// hanging at the confirmation instead of exiting nonzero.
//
// The reader goroutine stays parked on stdin after cancellation. That is
// deliberate rather than overlooked: there is no portable way to interrupt a
// blocking terminal read, the channel is buffered so the goroutine can finish
// and exit if input ever arrives, and the only caller is a command that is
// already on its way out.
func ConfirmCleanupContext(ctx context.Context, toArchive, toDelete []string, cfg *Config) (bool, error) {
	return confirmCleanupWithIOContext(ctx, toArchive, toDelete, cfg, os.Stderr, os.Stdin)
}

func confirmCleanupWithIOContext(ctx context.Context, toArchive, toDelete []string,
	cfg *Config, out io.Writer, in io.Reader) (bool, error) {
	type outcome struct {
		confirmed bool
		err       error
	}
	done := make(chan outcome, 1)
	go func() {
		confirmed, err := confirmCleanupWithIO(toArchive, toDelete, cfg, out, in)
		done <- outcome{confirmed, err}
	}()
	select {
	case got := <-done:
		return got.confirmed, got.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func confirmCleanupWithIO(toArchive, toDelete []string, cfg *Config, out io.Writer, in io.Reader) (bool, error) {
	if len(toArchive) == 0 && len(toDelete) == 0 {
		return false, fmt.Errorf("no sessions selected")
	}
	if _, err := fmt.Fprintln(out, cleanupConfirmationDescription(toArchive, toDelete)); err != nil {
		return false, fmt.Errorf("display cleanup targets: %w", err)
	}

	var confirmed bool
	err := huh.NewConfirm().
		Title("Confirm cleanup (deletions cannot be undone)").
		Affirmative("Yes, proceed").
		Negative("Cancel").
		Value(&confirmed).
		WithTheme(getTheme(cfg.UI.Theme)).
		RunAccessible(out, in)

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
