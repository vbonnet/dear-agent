package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	sessionCompactArgs    string
	sessionCompactMonitor bool
	sessionCompactTimeout time.Duration
)

var sessionCompactCmd = &cobra.Command{
	Use:   "compact <identifier>",
	Short: "Trigger context compaction and monitor for completion",
	Long: `Trigger /compact in a running session with state detection and optional monitoring.

This registered-session compaction surface:
  1. Resolves the session by name, ID, or UUID
  2. Requires the registered harness to own an empty composer
  3. Uses stable-ID prompt, anti-loop, and audit accounting before delivery
  4. Atomically sends /compact to that exact pane
  5. Verifies an active transition followed by stable readiness

Examples:
  # Trigger compaction and wait for completion
  agm session compact my-session

  # Compact with preservation instructions
  agm session compact my-session --compact-args "preserve context about auth refactor"

  # Fire and forget (don't wait for completion)
  agm session compact my-session --monitor=false

See Also:
  • agm send compact    - Same accounting path with prompt preservation and --force
  • agm session context - Show current context usage`,
	Args: withCompactionJSONErrorBoundary(validateCompactionArgs),
	RunE: withCompactionJSONErrorBoundary(runSessionCompact),
}

func init() {
	sessionCompactCmd.SetFlagErrorFunc(handleCompactionFlagParseError)
	sessionCompactCmd.Flags().StringVar(&sessionCompactArgs, "compact-args", "", "Compaction instructions (text appended after /compact)")
	sessionCompactCmd.Flags().BoolVar(&sessionCompactMonitor, "monitor", true, "Wait for compaction to complete")
	sessionCompactCmd.Flags().DurationVar(&sessionCompactTimeout, "timeout", 5*time.Minute, "Maximum time to wait for compaction")
	sessionCmd.AddCommand(sessionCompactCmd)
}

func runSessionCompact(cmd *cobra.Command, args []string) error {
	identifier := args[0]
	if err := validateRawCompactionInput("compact_args", sessionCompactArgs); err != nil {
		return err
	}
	if err := validateSessionCompactionOptions(); err != nil {
		return handleError(err)
	}

	// Resolve session via Dolt
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()
	opCtx.Context = cmd.Context()
	opCtx.CompactionBaseDir = agmBaseDir()

	getResult, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: identifier,
		ActiveOnly: true,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	s := getResult.Session
	if err := validateCompactionTarget(s); err != nil {
		return handleError(err)
	}
	tmuxName := s.TmuxSession
	if tmuxName == "" {
		tmuxName = s.Name
	}
	harness := s.Harness
	if harness == "" {
		harness = manifest.DefaultHarness
	}
	harness = agent.NormalizeHarnessName(harness)
	if err := validateInitialCompactionReadiness(
		cmd.Context(), s.Name, tmuxName, harness, compaction.ObserveExpectedHarnessSession,
	); err != nil {
		return handleError(err)
	}

	command := buildCompactCommand(sessionCompactArgs)
	if err := validateCompactionCommandForSurface(command); err != nil {
		return err
	}
	delivery, opErr := ops.DeliverSessionCompaction(opCtx, &ops.SessionCompactionDeliveryRequest{
		Identifier: s.ID,
		Command:    command,
		Forced:     false,
	})
	return finishCompactionDelivery(delivery, opErr, func(confirmed *ops.SessionCompactionDeliveryResult) error {
		return finishSessionCompactionSuccess(
			cmd.Context(), confirmed, command, s.Name,
			sessionCompactMonitor, sessionCompactTimeout, runCompactionVerifier,
		)
	})
}

func validateSessionCompactionOptions() error {
	if sessionCompactMonitor && sessionCompactTimeout <= 0 {
		return ops.ErrInvalidInput("timeout", "Compaction monitoring requires a positive --timeout duration.")
	}
	return nil
}

func finishSessionCompactionSuccess(
	ctx context.Context,
	delivery *ops.SessionCompactionDeliveryResult,
	command, displayName string,
	monitor bool,
	timeout time.Duration,
	run compactionVerifierRunner,
) error {
	if !isJSONOutput() {
		ui.PrintSuccess(fmt.Sprintf("Sent %s to session '%s' (prompt saved: %s)", command, displayName, delivery.PromptFile))
		return runOptionalCompactionVerification(monitor, func() error {
			fmt.Println()
			if err := monitorCompactionWithRunner(ctx, verificationTarget(delivery), displayName, timeout, run); err != nil {
				return handleCompactionVerificationFailure(delivery, err)
			}
			return nil
		})
	}

	var verification *compaction.Verification
	if monitor {
		confirmed, err := run(ctx, verificationTarget(delivery), timeout, 2*time.Second)
		if err != nil {
			return handleCompactionVerificationFailure(delivery, err)
		}
		verification = &confirmed
	}
	return reportCompactionSuccess(delivery, verification, func() {})
}

// monitorCompaction requires a positive post-delivery transition and stable
// live readiness before reporting completion.
func monitorCompaction(ctx context.Context, target compaction.VerificationTarget, displayName string, timeout time.Duration) error {
	return monitorCompactionWithRunner(ctx, target, displayName, timeout, runCompactionVerifier)
}

func monitorCompactionWithRunner(ctx context.Context, target compaction.VerificationTarget, displayName string, timeout time.Duration, run compactionVerifierRunner) error {
	const pollInterval = 2 * time.Second

	verification, err := run(ctx, target, timeout, pollInterval)
	if err != nil {
		fmt.Printf("\nCheck status:\n  agm session get %s\n  agm session context %s\n", displayName, displayName)
		return err
	}
	ui.PrintSuccess(fmt.Sprintf("Compaction completed in %s", verification.Elapsed.Round(time.Second)))
	return nil
}
