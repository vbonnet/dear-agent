package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/safety"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

var (
	safetyCheckJSON       bool
	safetySkipTyping      bool
	safetySkipInit        bool
	safetySkipMidResponse bool
)

var safetyCmd = &cobra.Command{
	Use:   "safety",
	Short: "Session interaction safety tools",
	Long:  `Safety tools for checking session state before automated interaction.`,
	Args:  cobra.ArbitraryArgs,
	RunE:  groupRunE,
}

var safetyCheckCmd = &cobra.Command{
	Use:   "check <session-name>",
	Short: "Check session safety guards before interaction",
	Long: `Check all safety guards for a session before sending messages or performing actions.

Returns exit code 0 if safe, exit code 1 if any guards are violated.

Guards checked:
  • human_typing      - Unsent text in the prompt line
  • session_uninit    - Claude hasn't started or is on welcome screen
  • claude_mid_resp   - Claude is actively generating a response

Examples:
  # Check all guards (human-readable output)
  agm safety check my-session

  # Machine-readable output for automation
  agm safety check my-session --json

  # Skip specific guards
  agm safety check my-session --skip-typing --skip-mid-response`,
	Args: cobra.ExactArgs(1),
	RunE: runSafetyCheck,
}

func init() {
	safetyCheckCmd.Flags().BoolVar(&safetyCheckJSON, "json", false, "Output in JSON format (for automation)")
	safetyCheckCmd.Flags().BoolVar(&safetySkipTyping, "skip-typing", false, "Skip human typing detection")
	safetyCheckCmd.Flags().BoolVar(&safetySkipInit, "skip-init", false, "Skip session uninitialized detection")
	safetyCheckCmd.Flags().BoolVar(&safetySkipMidResponse, "skip-mid-response", false, "Skip Claude mid-response detection")

	safetyCmd.AddCommand(safetyCheckCmd)
	rootCmd.AddCommand(safetyCmd)
}

func runSafetyCheck(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	// Verify session exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session '%s' does not exist in tmux", sessionName)
	}

	result := safety.Check(sessionName, safety.GuardOptions{
		SkipHumanTyping:   safetySkipTyping,
		SkipUninitialized: safetySkipInit,
		SkipMidResponse:   safetySkipMidResponse,
	})

	if safetyCheckJSON {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		if result.Safe {
			fmt.Printf("✓ Session '%s' is safe for interaction\n", sessionName)
		} else {
			fmt.Printf("✗ Session '%s' has safety violations:\n\n", sessionName)
			fmt.Print(result.Error())
			fmt.Printf("To bypass: use --force flag on the command, or --skip-* flags on safety check\n")
		}
	}

	if !result.Safe {
		// Use a silent error to set exit code 1 without printing "Error:"
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		return fmt.Errorf("safety check failed")
	}

	return nil
}
