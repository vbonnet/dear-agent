package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var (
	lastNameOnly bool
	lastResume   bool
)

var lastCmd = &cobra.Command{
	Use:   "last",
	Short: "Show or resume the most recently active session",
	Long: `Show the most recently active session by last activity timestamp.

By default, prints session details. Use flags for scripting or quick resume.

Examples:
  agm last              # Show most recently active session
  agm last --name       # Print just the session name (for scripting)
  agm last --resume     # Resume the most recently active session`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opCtx, cleanup, err := newOpContextWithStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to storage: %w", err)
		}
		defer cleanup()

		result, err := ops.LastSession(opCtx, &ops.LastSessionRequest{
			NameOnly: lastNameOnly,
		})
		if err != nil {
			return handleError(err)
		}

		// --name: just print the name and exit (for scripting)
		if lastNameOnly {
			fmt.Println(result.Session.Name)
			return nil
		}

		// --resume: resolve and resume the session
		if lastResume {
			adapter, err := getStorage()
			if err != nil {
				return fmt.Errorf("failed to connect to storage: %w", err)
			}
			defer adapter.Close()

			// Use the resume flow with the session name as identifier
			sessionID, manifestPath, err := resolveSessionIdentifier(adapter, result.Session.Name)
			if err != nil {
				return fmt.Errorf("failed to resolve session: %w", err)
			}

			m, err := adapter.GetSession(sessionID)
			if err != nil {
				return fmt.Errorf("failed to get session: %w", err)
			}

			harnessName := m.Harness
			if harnessName == "" {
				harnessName = "claude-code"
			}

			health, err := checkSessionHealth(adapter, sessionID, manifestPath)
			if err != nil {
				return fmt.Errorf("session health check failed: %w", err)
			}

			if !health.CanResume {
				return fmt.Errorf("session cannot be resumed - critical health issues detected")
			}

			return resumeSession(adapter, sessionID, manifestPath, harnessName, health)
		}

		// Default: print session details
		if isJSONOutput() {
			return printJSON(result)
		}

		s := result.Session
		fmt.Fprintf(os.Stderr, "Most recently active session:\n\n")
		fmt.Printf("  Name:    %s\n", s.Name)
		fmt.Printf("  ID:      %s\n", s.ID)
		fmt.Printf("  Status:  %s\n", s.Status)
		fmt.Printf("  Harness: %s\n", s.Harness)
		fmt.Printf("  Project: %s\n", s.Project)
		fmt.Printf("  Updated: %s\n", s.UpdatedAt)
		fmt.Println()
		fmt.Fprintf(os.Stderr, "To resume: agm session resume %s\n", s.Name)
		fmt.Fprintf(os.Stderr, "Or:        agm last --resume\n")

		return nil
	},
}

func init() {
	lastCmd.Flags().BoolVar(&lastNameOnly, "name", false, "Print just the session name (for scripting)")
	lastCmd.Flags().BoolVar(&lastResume, "resume", false, "Resume the most recently active session")
	sessionCmd.AddCommand(lastCmd)
}
