package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
)

var (
	interruptType    string
	interruptReason  string
	interruptContext []string // key=value pairs
)

var interruptCmd = &cobra.Command{
	Use:   "interrupt [session-name]",
	Short: "Send a soft interrupt to a running session",
	Long: `Send a flag-based interrupt to a running AGM session.

Unlike tmux key injection (Ctrl-C), this mechanism is non-destructive:
the session's pre-tool hook checks for the flag before each tool call
and responds appropriately based on the interrupt type.

Interrupt Types:
  stop   - Block all subsequent tool calls until the flag is cleared.
           The session stops after the current tool completes.
  steer  - Inject guidance into the session. The hook passes the reason
           as a user-facing message and the session continues working.
  kill   - Block all tool calls immediately. Use as a last resort.
           The sentinel will escalate to Ctrl-C after 60s if needed.

The flag file is written atomically to ~/.agm/interrupts/{session}.json.

Examples:
  # Stop a session gracefully
  agm interrupt my-session --type stop --reason "Need to review progress"

  # Redirect a session's work
  agm interrupt my-session --type steer --reason "Focus on tests instead"

  # Emergency kill (sentinel escalates to Ctrl-C after 60s)
  agm interrupt my-session --type kill --reason "Session is looping"

  # With additional context
  agm interrupt my-session --type stop --reason "Budget exceeded" \
    --context cost=42.50 --context threshold=40.00`,
	Args: cobra.ExactArgs(1),
	RunE: runInterrupt,
}

var interruptClearCmd = &cobra.Command{
	Use:   "clear [session-name]",
	Short: "Clear a pending interrupt flag",
	Long:  `Remove an interrupt flag file for a session without it being consumed by a hook.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runInterruptClear,
}

var interruptStatusCmd = &cobra.Command{
	Use:   "status [session-name]",
	Short: "Check if a session has a pending interrupt",
	Args:  cobra.ExactArgs(1),
	RunE:  runInterruptStatus,
}

func init() {
	interruptCmd.Flags().StringVar(&interruptType, "type", "stop",
		"Interrupt type: stop, steer, kill")
	interruptCmd.Flags().StringVar(&interruptReason, "reason", "",
		"Reason for the interrupt (required)")
	interruptCmd.Flags().StringSliceVar(&interruptContext, "context", nil,
		"Additional context as key=value pairs")
	_ = interruptCmd.MarkFlagRequired("reason")

	interruptCmd.AddCommand(interruptClearCmd)
	interruptCmd.AddCommand(interruptStatusCmd)
	rootCmd.AddCommand(interruptCmd)
}

func runInterrupt(cmd *cobra.Command, args []string) error {
	sessionName := args[0]

	iType, err := interrupt.ValidateType(interruptType)
	if err != nil {
		return err
	}

	// Parse context key=value pairs
	ctx := make(map[string]string)
	for _, kv := range interruptContext {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				ctx[kv[:i]] = kv[i+1:]
				break
			}
		}
	}

	// Determine sender
	sender := "user"
	if s := os.Getenv("AGM_SESSION_NAME"); s != "" {
		sender = s
	}

	flag := &interrupt.Flag{
		Type:     iType,
		Reason:   interruptReason,
		IssuedBy: sender,
		IssuedAt: time.Now().UTC(),
		Context:  ctx,
	}

	dir := interrupt.DefaultDir()
	if err := interrupt.Write(dir, sessionName, flag); err != nil {
		return fmt.Errorf("failed to write interrupt flag: %w", err)
	}

	fmt.Printf("✓ Interrupt sent to '%s' [type: %s]\n", sessionName, iType)
	fmt.Printf("  Reason: %s\n", interruptReason)
	fmt.Printf("  Flag: %s\n", interrupt.FlagPath(dir, sessionName))

	return nil
}

func runInterruptClear(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	dir := interrupt.DefaultDir()

	if err := interrupt.Clear(dir, sessionName); err != nil {
		return err
	}

	fmt.Printf("✓ Interrupt flag cleared for '%s'\n", sessionName)
	return nil
}

func runInterruptStatus(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	dir := interrupt.DefaultDir()

	flag, err := interrupt.Read(dir, sessionName)
	if err != nil {
		return err
	}

	if flag == nil {
		fmt.Printf("No pending interrupt for '%s'\n", sessionName)
		return nil
	}

	fmt.Printf("Pending interrupt for '%s':\n", sessionName)
	fmt.Printf("  Type:      %s\n", flag.Type)
	fmt.Printf("  Reason:    %s\n", flag.Reason)
	fmt.Printf("  Issued by: %s\n", flag.IssuedBy)
	fmt.Printf("  Issued at: %s\n", flag.IssuedAt.Format(time.RFC3339))
	if len(flag.Context) > 0 {
		fmt.Printf("  Context:\n")
		for k, v := range flag.Context {
			fmt.Printf("    %s: %s\n", k, v)
		}
	}

	return nil
}
