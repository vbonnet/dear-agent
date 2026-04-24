package main

import (
	"github.com/spf13/cobra"
)

var sendGroupCmd = &cobra.Command{
	Use:   "send",
	Short: "Send messages and signals to sessions",
	Long: `The send command group provides operations for sending messages and control signals
to running AGM sessions.

Available Commands:
  msg        Send a message/prompt to a running session
  approve    Approve a permission prompt with optional reason
  reject     Reject a permission prompt with custom reason
  mode       Switch AI harness mode (plan, auto, default)
  set-model  Change the AI model of a running session
  enter      Send Enter to submit content in the input line
  clear      Clear the input prompt contents without submitting
  stash      Stash the current input message (Ctrl+S)

Examples:
  agm send msg my-session --prompt "Please review the code"
  agm send approve my-session
  agm send reject my-session --reason "Use Read tool instead"
  agm send mode plan my-session
  agm send set-model my-session opus-1m
  agm send enter my-session
  agm send clear my-session
  agm send stash my-session

See Also:
  • agm send msg --help       - Full message sending documentation
  • agm send approve --help   - Full approval documentation
  • agm send reject --help    - Full rejection documentation
  • agm send mode --help      - Full mode switching documentation
  • agm send set-model --help - Full model switching documentation
  • agm send enter --help     - Full enter documentation
  • agm send clear --help     - Full clear documentation
  • agm send stash --help     - Full stash documentation`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

func init() {
	rootCmd.AddCommand(sendGroupCmd)
}
