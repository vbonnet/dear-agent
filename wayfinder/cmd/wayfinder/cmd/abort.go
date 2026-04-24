package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/internal/project"
	"github.com/vbonnet/dear-agent/wayfinder/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/internal/tracker"
)

var abortCmd = &cobra.Command{
	Use:   "abort [project-path]",
	Short: "Abort and archive Wayfinder project",
	Long: `Abort the current Wayfinder project and archive it.

This command will:
- Mark the session as abandoned
- Archive the project to ~/.archived/

Examples:
  wayfinder abort                      # Abort project in current directory
  wayfinder abort ~/src/my-project  # Abort specific project`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAbort,
}

func init() {
	rootCmd.AddCommand(abortCmd)
}

func runAbort(cmd *cobra.Command, args []string) error {
	// Determine project directory
	var projectPath string
	if len(args) > 0 {
		projectPath = args[0]
	}

	projectDir, err := project.DetectDir(projectPath)
	if err != nil {
		return err
	}

	// Get project name
	projectName := filepath.Base(projectDir)

	// Read existing status to get session ID
	st, err := status.ReadFrom(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read session status: %v\n", err)
		// Continue anyway - archiving is more important
	} else {
		// Mark session as abandoned
		tr, err := tracker.New(st.SessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize tracker: %v\n", err)
		} else {
			defer tr.Close(context.Background())

			// Publish session.completed with abandoned status
			if err := tr.EndSession("abandoned"); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to publish session.completed event: %v\n", err)
			}
		}

		// Update status file
		st.Status = "abandoned"
		if err := st.WriteTo(projectDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update status file: %v\n", err)
		}
	}

	// Archive the project
	archiveDir := filepath.Join(os.Getenv("HOME"), "src", "ws", ".archived")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	archivedPath := filepath.Join(archiveDir, projectName)

	// Check if archived project already exists
	if _, err := os.Stat(archivedPath); err == nil {
		return fmt.Errorf("archived project already exists: %s\nPlease remove it manually or choose a different name", archivedPath)
	}

	// Move project to archive
	if err := os.Rename(projectDir, archivedPath); err != nil {
		return fmt.Errorf("failed to archive project: %w", err)
	}

	fmt.Printf("\n✅ Project aborted and archived\n")
	fmt.Printf("Project: %s\n", projectName)
	fmt.Printf("Archived to: %s\n", archivedPath)

	return nil
}
