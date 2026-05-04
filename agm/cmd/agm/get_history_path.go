package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/uuid"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
)

var (
	historyJSONOutput  bool
	historyVerifyPaths bool
)

var getHistoryPathCmd = &cobra.Command{
	Use:   "get-history-path [session-name]",
	Short: "Get conversation history file paths for a session",
	Long: `Returns conversation history file paths for an AGM session.

This command constructs file paths based on the harness type (Claude Code,
Gemini CLI, OpenCode, Codex) used by the session. The harness determines
where conversation logs are stored.

If session-name is not provided:
  - If running inside tmux: uses the current tmux session
  - If not in tmux: auto-detects the current Claude session from history

Examples:
  # Get paths for current session (auto-detect)
  agm session get-history-path

  # Get paths for specific session
  agm session get-history-path my-session

  # Get paths in JSON format
  agm session get-history-path my-session --json

  # Verify that files exist
  agm session get-history-path my-session --verify

  # Use in shell script
  HISTORY_PATH=$(agm session get-history-path --json | jq -r '.paths[0]')`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var sessionName string

		// Determine which session to look up
		if len(args) > 0 {
			// User provided session name
			sessionName = args[0]
		} else {
			// No argument - try to get current tmux session first
			tmuxSessionName, err := tmux.GetCurrentSessionName()
			if err == nil {
				// In tmux, use tmux session name
				sessionName = tmuxSessionName
			} else {
				// Not in tmux - auto-detect current Claude session from history
				parser := history.NewParser("")
				sessions, err := parser.ReadConversations(1) // Get most recent session
				if err != nil {
					return fmt.Errorf("not in tmux and failed to auto-detect Claude session: %w", err)
				}
				if len(sessions) == 0 || sessions[0].SessionID == "" {
					return fmt.Errorf("not in tmux and no recent Claude sessions found in history")
				}

				// Use the auto-detected UUID directly for current session
				currentUUID := sessions[0].SessionID

				// Try to get agent type from AGM for this UUID
				adapter, err := getStorage()
				if err != nil {
					// Can't connect to AGM, default to Claude
					return outputHistoryLocation(cmd, "claude-code", currentUUID, "", nil)
				}
				defer adapter.Close()

				// Search for session by UUID
				manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
				if err != nil {
					// Can't query sessions, default to Claude
					return outputHistoryLocation(cmd, "claude-code", currentUUID, "", nil)
				}

				// Find session with matching Claude UUID
				for _, m := range manifests {
					if m.Claude.UUID == currentUUID {
						// Found the session, use its working directory
						workingDir := m.Context.Project
						harness := m.Harness
						if harness == "" {
							harness = "claude-code"
						}
						return outputHistoryLocation(cmd, harness, currentUUID, workingDir, m)
					}
				}

				// Session not found in AGM, default to Claude with no working dir
				return outputHistoryLocation(cmd, "claude-code", currentUUID, "", nil)
			}
		}

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer adapter.Close()

		// Create manifest search function for uuid.Discover
		findInManifests := func(name string) (*manifest.Manifest, error) {
			manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
			if err != nil {
				return nil, fmt.Errorf("failed to list sessions: %w", err)
			}

			for _, m := range manifests {
				if m.Tmux.SessionName == name || m.Name == name {
					return m, nil
				}
			}
			return nil, fmt.Errorf("no AGM session found for: %s", name)
		}

		// Use 3-level fallback to discover UUID
		discoveredUUID, err := uuid.Discover(sessionName, findInManifests, false)
		if err != nil {
			return fmt.Errorf("failed to discover UUID for session '%s': %w", sessionName, err)
		}

		// Get session metadata from database
		session, err := adapter.ResolveIdentifier(sessionName)
		if err != nil {
			return fmt.Errorf("failed to get session metadata: %w", err)
		}

		// Extract harness type and working directory
		harness := session.Harness
		if harness == "" {
			harness = "claude-code" // Default to claude-code if not set
		}

		workingDir := session.Context.Project

		// Output the history location
		return outputHistoryLocation(cmd, harness, discoveredUUID, workingDir, session)
	},
}

// outputHistoryLocation constructs and outputs the history location
func outputHistoryLocation(cmd *cobra.Command, agent, uuid, workingDir string, session *manifest.Manifest) error {
	location, err := history.GetHistoryPaths(agent, uuid, workingDir, historyVerifyPaths)
	if err != nil {
		if historyJSONOutput {
			outputHistoryError(cmd, agent, uuid, err)
			return nil
		}
		return err
	}

	if session != nil {
		location.SessionName = session.Name
		location.SessionID = session.SessionID
	}

	if historyJSONOutput {
		formatter, err := cliframe.NewFormatter(cliframe.FormatJSON, cliframe.WithPrettyPrint(true))
		if err != nil {
			return fmt.Errorf("failed to create JSON formatter: %w", err)
		}
		writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
		writer = writer.WithFormatter(formatter)
		return writer.Output(location)
	}
	printHistoryHumanReadable(location)
	return nil
}

// outputHistoryError emits a JSON HistoryLocation describing the failure when
// path construction fails in --json mode.
func outputHistoryError(cmd *cobra.Command, agent, uuid string, err error) {
	errorLoc := &history.HistoryLocation{
		Harness: agent,
		UUID:    uuid,
		Paths:   []string{},
		Exists:  false,
	}
	var locErr *history.LocationError
	if errors.As(err, &locErr) {
		errorLoc.Error = locErr
	} else {
		errorLoc.Error = &history.LocationError{
			Code:    "PATH_CONSTRUCTION_FAILED",
			Message: err.Error(),
		}
	}
	formatter, _ := cliframe.NewFormatter(cliframe.FormatJSON, cliframe.WithPrettyPrint(true))
	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())
	writer = writer.WithFormatter(formatter)
	if outErr := writer.Output(errorLoc); outErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to write error output: %v\n", outErr)
	}
}

// printHistoryHumanReadable prints the human-readable view of a successfully
// resolved history location.
func printHistoryHumanReadable(location *history.HistoryLocation) {
	if location.SessionName != "" {
		fmt.Printf("Session: %s\n", location.SessionName)
	}
	fmt.Printf("Harness: %s\n", location.Harness)
	fmt.Printf("UUID:    %s\n\n", location.UUID)

	fmt.Println("Conversation History:")
	for _, path := range location.Paths {
		fmt.Printf("  %s\n", path)
	}

	if location.Metadata != nil {
		fmt.Println("\nMetadata:")
		if workingDir, ok := location.Metadata["working_directory"]; ok {
			fmt.Printf("  Working Directory: %s\n", workingDir)
		}
		if encoding, ok := location.Metadata["encoding_method"]; ok {
			fmt.Printf("  Encoding Method:   %s\n", encoding)
		}
		if hashMethod, ok := location.Metadata["hash_method"]; ok {
			fmt.Printf("  Hash Method:       %s\n", hashMethod)
		}
	}

	if historyVerifyPaths {
		fmt.Println()
		if location.Exists {
			fmt.Println("✓ All files exist")
		} else {
			fmt.Println("✗ Some files do not exist")
		}
	}
}

func init() {
	sessionCmd.AddCommand(getHistoryPathCmd)
	getHistoryPathCmd.Flags().BoolVar(&historyJSONOutput, "json", false, "Output in JSON format")
	getHistoryPathCmd.Flags().BoolVar(&historyVerifyPaths, "verify", false, "Verify that files exist on filesystem")
}
