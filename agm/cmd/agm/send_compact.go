package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/compaction"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/override"
)

var (
	compactFocus  string
	compactVerify bool
	compactDryRun bool
	compactForce  bool
	compactReason string
)

var requireCompactionOverride = override.RequireAudited

var sendCompactCmd = &cobra.Command{
	Use:   "compact <identifier>",
	Short: "Trigger /compact in a registered session with safety checks",
	Long: `Send the /compact slash command to an AGM-registered running session with
stable-ID resolution, pre-flight checks, anti-loop safety, auto-generated
prompts, and an audit trail. Raw or unmanaged tmux panes are intentionally not
accepted because compaction delivery and accounting require durable identity.

Both registered compaction commands use the same stable-session-ID prompt,
anti-loop, and audit accounting path before delivery.

Features:
  - Pre-flight checks: proves the registered harness owns an empty composer
  - Atomic delivery: revalidates the exact pane while sending
  - Anti-loop safety: 2-hour cooldown, max 3 compactions per rolling 24-hour window
  - Auto-generated prompt: includes session context for preservation
  - Audit trail: saves each prompt to ~/.agm/compaction-prompts/

Examples:
  # Trigger compaction with auto-generated prompt
  agm send compact my-session

  # Compact with custom preservation instructions
  agm send compact my-session --focus "preserve context about auth refactor"

  # Preview the prompt without sending
  agm send compact my-session --dry-run

  # Send and verify completion
  agm send compact my-session --verify

  # Override safety limits
  agm send compact my-session --force --reason "urgent context preservation"

See Also:
  • agm session compact  - Same accounting path with default monitoring
  • agm send msg         - Send messages to sessions`,
	Args: withCompactionJSONErrorBoundary(validateCompactionArgs),
	RunE: withCompactionJSONErrorBoundary(runSendCompact),
}

func init() {
	sendCompactCmd.SetFlagErrorFunc(handleCompactionFlagParseError)
	sendCompactCmd.Flags().StringVar(&compactFocus, "focus", "", "Custom preservation instructions appended to compaction prompt")
	sendCompactCmd.Flags().BoolVar(&compactVerify, "verify", false, "Poll session state every 10s until compaction completes")
	sendCompactCmd.Flags().BoolVar(&compactDryRun, "dry-run", false, "Output the compaction prompt without sending")
	sendCompactCmd.Flags().BoolVar(&compactForce, "force", false, "Override anti-loop safety (cooldown and max compactions) — requires --reason")
	sendCompactCmd.Flags().StringVar(&compactReason, "reason", "", "Justification for --force, recorded in the override audit log")
	sendGroupCmd.AddCommand(sendCompactCmd)
}

func handleCompactionFlagParseError(cmd *cobra.Command, err error) error {
	if err == nil || !isJSONOutput() {
		return err
	}
	configureCompactionOutput(cmd)
	problem := ops.ErrInvalidInput("flags", err.Error())
	problem.Instance = compactionCommandInstance(cmd)
	problem.Parameters["command"] = problem.Instance
	return renderCompactionJSONError(cmd, problem)
}

// prepareCompactionFlagErrorOutput mirrors the root's output-mode precedence
// before Cobra starts parsing. Flag parsing stops at the first invalid flag,
// so PersistentPreRunE may never see a later --output json or --agent. The
// command-local flag error handler still decides whether the resolved command
// is one of the two compaction surfaces; this preparse changes no other
// command's error owner. Arguments after -- are positional and intentionally
// cannot change output mode.
func prepareCompactionFlagErrorOutput(args []string) {
	explicitFormat, hasExplicitFormat, rawForceAgent, rawForceNoAgent := preparseOutputIntent(args)
	outputMode = resolveOutputMode(rawForceAgent, rawForceNoAgent)
	if hasExplicitFormat {
		outputFormat = explicitFormat
		return
	}
	if outputMode == ModeAgent {
		outputFormat = "json"
		return
	}
	outputFormat = "text"
}

func preparseOutputIntent(args []string) (format string, explicit, agent, noAgent bool) {
	intent := preparsedOutputIntent{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if next, handled := parseOutputIntentFlag(args, i, &intent); handled {
			i = next
			continue
		}
		if parseAgentIntentFlag(arg, &intent) {
			continue
		}
		if compactionFlagConsumesValue(arg) && i+1 < len(args) {
			i++
		}
	}
	return intent.format, intent.explicit, intent.agent, intent.noAgent
}

type preparsedOutputIntent struct {
	format   string
	explicit bool
	agent    bool
	noAgent  bool
}

func parseOutputIntentFlag(args []string, index int, intent *preparsedOutputIntent) (int, bool) {
	arg := args[index]
	switch {
	case arg == "--output" || arg == "-o":
		if index+1 < len(args) {
			intent.format = args[index+1]
			intent.explicit = true
			return index + 1, true
		}
		return index, true
	case strings.HasPrefix(arg, "--output="):
		intent.format = strings.TrimPrefix(arg, "--output=")
		intent.explicit = true
		return index, true
	case strings.HasPrefix(arg, "-o="):
		intent.format = strings.TrimPrefix(arg, "-o=")
		intent.explicit = true
		return index, true
	case strings.HasPrefix(arg, "-o") && len(arg) > len("-o"):
		intent.format = strings.TrimPrefix(arg, "-o")
		intent.explicit = true
		return index, true
	default:
		return index, false
	}
}

func parseAgentIntentFlag(arg string, intent *preparsedOutputIntent) bool {
	switch {
	case arg == "--agent":
		intent.agent = true
		return true
	case strings.HasPrefix(arg, "--agent="):
		if value, err := strconv.ParseBool(strings.TrimPrefix(arg, "--agent=")); err == nil {
			intent.agent = value
		}
		return true
	case arg == "--no-agent":
		intent.noAgent = true
		return true
	case strings.HasPrefix(arg, "--no-agent="):
		if value, err := strconv.ParseBool(strings.TrimPrefix(arg, "--no-agent=")); err == nil {
			intent.noAgent = value
		}
		return true
	default:
		return false
	}
}

// compactionFlagConsumesValue mirrors pflag for flags reachable on the two
// compaction command paths. Their following token is a value even when it
// begins with a dash, so it cannot also express output intent.
func compactionFlagConsumesValue(arg string) bool {
	switch arg {
	case "--directory", "-C", "--config", "--sessions-dir", "--log-level", "--timeout",
		"--workspace", "--fields", "--focus", "--reason", "--compact-args":
		return true
	default:
		return false
	}
}

// buildCompactCommand constructs the /compact command string with optional args.
// Preserved for backward compatibility with session_compact.go.
func buildCompactCommand(args string) string {
	args = strings.TrimSpace(args)
	if args == "" {
		return "/compact"
	}
	return "/compact " + args
}

// agmBaseDir returns ~/.agm, or AGM_HOME if set.
func agmBaseDir() string {
	if d := os.Getenv("AGM_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".agm")
}

func runSendCompact(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	identifier := args[0]
	if err := validateRawCompactionInput("focus", compactFocus); err != nil {
		return err
	}
	if err := validateSendCompactionOptions(compactForce, compactDryRun); err != nil {
		return handleError(err)
	}
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()
	opCtx.Context = ctx
	opCtx.CompactionBaseDir = agmBaseDir()

	getResult, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: identifier,
		ActiveOnly: true,
	})
	if opErr != nil {
		return handleError(opErr)
	}
	target := getResult.Session
	if err := validateCompactionTarget(target); err != nil {
		return handleError(err)
	}
	sessionName := target.Name
	if sessionName == "" {
		sessionName = identifier
	}
	tmuxName := target.TmuxSession
	if tmuxName == "" {
		tmuxName = sessionName
	}
	harness := target.Harness
	if harness == "" {
		harness = manifest.DefaultHarness
	}
	harness = agent.NormalizeHarnessName(harness)

	baseDir := opCtx.CompactionBaseDir

	return withSendCompactionGovernance(
		ctx, sessionName, tmuxName, harness, compactForce, compactReason,
		compaction.ObserveExpectedHarnessSession,
		func() error {
			// Build from one resolved preservation snapshot. Delivery recomposes the
			// same prompt under the stable-session lock before allocating its audit.
			command, stateErr := ops.ComposeSessionCompactionPreservation(baseDir, target, compactFocus)
			if stateErr != nil {
				return fmt.Errorf("cannot use preservation state for session '%s': %w\n\nUse --focus to provide preservation instructions:\n  agm send compact %s --focus \"preserve context about ...\"", sessionName, stateErr, sessionName)
			}
			if err := validateCompactionCommandForSurface(command); err != nil {
				return err
			}
			if compactDryRun {
				return runCompactionDryRun(baseDir, target.ID, command)
			}

			delivery, opErr := ops.DeliverSessionCompaction(opCtx, &ops.SessionCompactionDeliveryRequest{
				Identifier: target.ID,
				Command:    command,
				Forced:     compactForce,
				ExpectedPreservation: &ops.SessionCompactionPreservationExpectation{
					Focus: compactFocus,
				},
			})
			return finishCompactionDelivery(delivery, opErr, func(confirmed *ops.SessionCompactionDeliveryResult) error {
				return finishSendCompactionSuccess(ctx, confirmed, sessionName, compactVerify, runCompactionVerifier)
			})
		})
}

func validateSendCompactionOptions(force, dryRun bool) error {
	if force && dryRun {
		return ops.ErrInvalidInput("force", "--force cannot be combined with --dry-run because a preview does not bypass anti-loop policy.")
	}
	return nil
}

func withSendCompactionGovernance(
	ctx context.Context,
	displayName, tmuxName, harness string,
	force bool,
	reason string,
	observe initialCompactionObserver,
	run func() error,
) error {
	if err := validateInitialCompactionReadiness(ctx, displayName, tmuxName, harness, observe); err != nil {
		return handleError(err)
	}
	if force {
		if err := requireCompactionOverride(ctx, override.Guard{
			Tool: "agm send-compact",
			Flag: "--force",
			Gate: "anti-loop guard (compaction cooldown and max-compactions cap)",
			Risk: override.RiskP0,
		}, reason); err != nil {
			return err
		}
	}
	return run()
}

func validateCompactionCommandForSurface(command string) error {
	if err := ops.ValidateCompactionCommandText(command); err != nil {
		return handleError(ops.ErrInvalidInput("command", err.Error()))
	}
	return nil
}

// validateRawCompactionInput rejects terminal controls before session
// resolution, tmux observation, audit allocation, or override logging. LF is
// retained because preservation instructions may intentionally be multiline.
func validateRawCompactionInput(field, value string) error {
	if err := ops.ValidateCompactionCommandText(value); err != nil {
		return handleError(ops.ErrInvalidInput(field, err.Error()))
	}
	return nil
}

func validateCompactionTarget(target ops.SessionDetail) error {
	if target.Lifecycle == manifest.LifecycleLegacy {
		harness := agent.NormalizeHarnessName(target.Harness)
		if harness == "openai" || harness == "gpt" {
			name := target.Name
			if name == "" {
				name = target.ID
			}
			return ops.ErrSessionNotReady(name, "PURE_API_SESSION")
		}
		return nil
	}
	name := target.Name
	if name == "" {
		name = target.ID
	}
	if target.Lifecycle == manifest.LifecycleArchived {
		return ops.ErrSessionArchived(name)
	}
	return ops.ErrSessionNotReady(name, "LIFECYCLE_"+target.Lifecycle)
}

type initialCompactionObserver func(context.Context, string, string) (*session.DetectionResult, error)

func validateInitialCompactionReadiness(
	ctx context.Context,
	displayName, tmuxName, harness string,
	observe initialCompactionObserver,
) error {
	observation, err := observe(ctx, tmuxName, harness)
	if err != nil {
		return compactionReadinessError(displayName, "OBSERVATION_FAILED", err)
	}
	if observation == nil {
		return compactionReadinessError(displayName, "OBSERVATION_UNAVAILABLE", errors.New("observer returned no result"))
	}
	if err := compaction.ValidateReady(*observation); err != nil {
		reason := observation.State
		if reason == "" {
			reason = "UNKNOWN"
		}
		return compactionReadinessError(displayName, reason, err)
	}
	return nil
}

func compactionReadinessError(name, reason string, cause error) error {
	problem := ops.ErrSessionNotReady(name, reason)
	if cause != nil {
		problem.Detail += " Initial compaction readiness check failed: " + cause.Error()
	}
	return problem
}

func runCompactionDryRun(baseDir, stableSessionID, command string) error {
	if err := validateCompactionCommandForSurface(command); err != nil {
		return err
	}
	promptFile, err := allocateDryRunCompactionPrompt(baseDir, stableSessionID, command)
	if err != nil {
		return fmt.Errorf("save stable-ID compaction prompt audit: %w", err)
	}
	return reportCompactionDryRun(command, promptFile)
}

func configureCompactionOutput(cmd *cobra.Command) {
	if cmd != nil && isJSONOutput() {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	}
}

func isCompactionCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	switch compactionCommandInstance(cmd) {
	case "send/compact", "session/compact":
		return true
	default:
		return false
	}
}

func isCompactionJSONCommand(cmd *cobra.Command) bool {
	return isCompactionCommand(cmd) && isJSONOutput()
}

// withRootCompactionJSONErrorBoundary extends the command-local JSON owner to
// the one earlier Cobra phase that runs outside Args and RunE. It remains exact
// to the two registered compaction paths, so generic sends retain their legacy
// root behavior.
func withRootCompactionJSONErrorBoundary(
	run func(*cobra.Command, []string) error,
) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := run(cmd, args)
		if !isCompactionJSONCommand(cmd) {
			return err
		}
		configureCompactionOutput(cmd)
		return renderCompactionJSONError(cmd, err)
	}
}

const compactionCommandFailureCode = "AGM-022"

// withCompactionJSONErrorBoundary gives both Cobra argument validation and the
// command body one JSON error owner. Text mode keeps Cobra's existing behavior.
func withCompactionJSONErrorBoundary(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		configureCompactionOutput(cmd)
		return renderCompactionJSONError(cmd, run(cmd, args))
	}
}

func validateCompactionArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		if isJSONOutput() {
			problem := ops.ErrInvalidInput("arguments", err.Error())
			problem.Instance = compactionCommandInstance(cmd)
			problem.Parameters["command"] = problem.Instance
			return problem
		}
		return err
	}
	return nil
}

func compactionCommandInstance(cmd *cobra.Command) string {
	if cmd == nil || cmd.CommandPath() == "" {
		return "compaction"
	}
	return strings.TrimPrefix(strings.ReplaceAll(cmd.CommandPath(), " ", "/"), "agm/")
}

// renderCompactionJSONError is the sole JSON renderer for errors leaving a
// compaction Cobra boundary. A marked exitError means a typed problem was
// already rendered; an unmarked root-setup exit still needs an envelope. Raw
// failures receive a stable generic command classification instead of being
// mislabeled as storage, validation, or delivery outcomes.
func renderCompactionJSONError(cmd *cobra.Command, err error) error {
	if err == nil || !isJSONOutput() {
		return err
	}
	var rendered *exitError
	if errors.As(err, &rendered) && rendered.rendered {
		return err
	}

	var problem *ops.OpError
	if !errors.As(err, &problem) {
		commandPath := compactionCommandInstance(cmd)
		problem = &ops.OpError{
			Status:   500,
			Type:     "command/compaction_failed",
			Code:     compactionCommandFailureCode,
			Title:    "Compaction command failed",
			Detail:   fmt.Sprintf("The compaction command failed before a typed operation outcome was available: %v", err),
			Instance: commandPath,
			Suggestions: []string{
				"Inspect the session and compaction audit state before deciding whether a retry is safe.",
				"Run `agm admin doctor` if the detail indicates a runtime or storage failure.",
			},
			Parameters: map[string]string{"command": commandPath},
		}
	}
	return handleError(problem)
}

const (
	compactionStatusDryRun   = "dry_run"
	compactionStatusSent     = "sent"
	compactionStatusVerified = "verified"
)

type compactionCommandResult struct {
	Operation    string                               `json:"operation"`
	Status       string                               `json:"status"`
	Delivery     *ops.SessionCompactionDeliveryResult `json:"delivery"`
	Verification *compactionVerificationReceipt       `json:"verification"`
	Command      string                               `json:"command,omitempty"`
	PromptFile   string                               `json:"prompt_file"`
}

type compactionVerificationReceipt struct {
	Proof               compaction.CompletionProof `json:"proof"`
	ElapsedMilliseconds int64                      `json:"elapsed_ms"`
}

func reportCompactionDryRun(command, promptFile string) error {
	result := compactionCommandResult{
		Operation:  "deliver_session_compaction",
		Status:     compactionStatusDryRun,
		Command:    command,
		PromptFile: promptFile,
	}
	return printResult(result, func() {
		fmt.Printf("=== Dry Run: Compaction Prompt ===\n\n%s\n\n=== Saved to: %s ===\n", command, promptFile)
	})
}

func reportCompactionSuccess(delivery *ops.SessionCompactionDeliveryResult, verification *compaction.Verification, text func()) error {
	status := compactionStatusSent
	var verificationReceipt *compactionVerificationReceipt
	if verification != nil {
		status = compactionStatusVerified
		verificationReceipt = &compactionVerificationReceipt{
			Proof:               verification.Proof,
			ElapsedMilliseconds: verification.Elapsed.Milliseconds(),
		}
	}
	operation := "deliver_session_compaction"
	promptFile := ""
	if delivery != nil {
		if delivery.Operation != "" {
			operation = delivery.Operation
		}
		promptFile = delivery.PromptFile
	}
	return printResult(compactionCommandResult{
		Operation:    operation,
		Status:       status,
		Delivery:     delivery,
		Verification: verificationReceipt,
		PromptFile:   promptFile,
	}, text)
}

func finishSendCompactionSuccess(
	ctx context.Context,
	delivery *ops.SessionCompactionDeliveryResult,
	displayName string,
	verify bool,
	run compactionVerifierRunner,
) error {
	if !isJSONOutput() {
		ui.PrintSuccess(fmt.Sprintf("Sent compaction to session '%s' (prompt saved: %s)", displayName, delivery.PromptFile))
		return runOptionalCompactionVerification(verify, func() error {
			fmt.Println()
			if err := verifyCompactionWithRunner(ctx, verificationTarget(delivery), 5*time.Minute, run); err != nil {
				return handleCompactionVerificationFailure(delivery, err)
			}
			return nil
		})
	}

	var verification *compaction.Verification
	if verify {
		confirmed, err := run(ctx, verificationTarget(delivery), 5*time.Minute, 10*time.Second)
		if err != nil {
			return handleCompactionVerificationFailure(delivery, err)
		}
		verification = &confirmed
	}
	return reportCompactionSuccess(delivery, verification, func() {})
}

func handleCompactionVerificationFailure(delivery *ops.SessionCompactionDeliveryResult, cause error) error {
	name := ""
	if delivery != nil {
		name = delivery.Name
	}
	reason := "unverified"
	var unverified *compaction.UnverifiedError
	if errors.As(cause, &unverified) && unverified.Reason != "" {
		reason = string(unverified.Reason)
	}
	opErr := ops.ErrCompactionVerification(name, reason, cause)
	attachCompactionRecoveryReceipt(opErr, delivery)
	if !isJSONOutput() {
		printCompactionRecoveryReceipt(delivery)
	}
	return handleError(opErr)
}

func allocateDryRunCompactionPrompt(baseDir, sessionID, command string) (string, error) {
	allocation, err := compaction.AllocatePromptExclusive(baseDir, sessionID, command)
	if err != nil {
		return "", err
	}
	return allocation.Path, nil
}

func finishCompactionDelivery(delivery *ops.SessionCompactionDeliveryResult, opErr error, onConfirmed func(*ops.SessionCompactionDeliveryResult) error) error {
	if opErr != nil {
		return finishCompactionDeliveryError(delivery, opErr)
	}
	if err := validateConfirmedCompactionDelivery(delivery); err != nil {
		return err
	}
	return onConfirmed(delivery)
}

func finishCompactionDeliveryError(delivery *ops.SessionCompactionDeliveryResult, opErr error) error {
	if delivery != nil && (delivery.MayHaveStarted || delivery.AccountingPending) {
		attachCompactionRecoveryReceipt(opErr, delivery)
		if !isJSONOutput() {
			printCompactionRecoveryReceipt(delivery)
		}
	}
	return handleError(opErr)
}

func validateConfirmedCompactionDelivery(delivery *ops.SessionCompactionDeliveryResult) error {
	if delivery == nil {
		return fmt.Errorf("compaction delivery returned no receipt")
	}
	if !delivery.Delivered {
		return fmt.Errorf("compaction delivery returned without confirmed delivery")
	}
	if delivery.AccountingPending {
		return fmt.Errorf("compaction delivery accounting is incomplete; do not retry")
	}
	if delivery.AttemptOutcome != compaction.AttemptOutcomeConfirmed {
		return fmt.Errorf("compaction delivery returned without confirmed durable accounting")
	}
	if delivery.PromptFile == "" {
		return fmt.Errorf("compaction delivery returned without a durable prompt audit path")
	}
	if !hasExactCompactionRuntimeReceipt(delivery) {
		return fmt.Errorf("compaction delivery returned without an exact pane/process receipt")
	}
	return nil
}

func hasExactCompactionRuntimeReceipt(delivery *ops.SessionCompactionDeliveryResult) bool {
	return delivery.PaneID != "" && delivery.PanePID > 0 && delivery.TargetPID > 0 &&
		delivery.HarnessStartTime != "" && delivery.TmuxSessionID != ""
}

func attachCompactionRecoveryReceipt(err error, delivery *ops.SessionCompactionDeliveryResult) {
	if err == nil || delivery == nil {
		return
	}
	var opErr *ops.OpError
	if !errors.As(err, &opErr) {
		return
	}
	if opErr.Parameters == nil {
		opErr.Parameters = make(map[string]string)
	}
	opErr.Parameters["session_id"] = delivery.SessionID
	opErr.Parameters["session_name"] = delivery.Name
	opErr.Parameters["tmux_name"] = delivery.TmuxName
	opErr.Parameters["harness"] = delivery.Harness
	opErr.Parameters["pane_id"] = delivery.PaneID
	opErr.Parameters["pane_pid"] = strconv.Itoa(delivery.PanePID)
	opErr.Parameters["target_pid"] = strconv.Itoa(delivery.TargetPID)
	opErr.Parameters["harness_start_time"] = delivery.HarnessStartTime
	opErr.Parameters["tmux_session_id"] = delivery.TmuxSessionID
	opErr.Parameters["attempt_id"] = delivery.AttemptID
	opErr.Parameters["attempt_outcome"] = string(delivery.AttemptOutcome)
	opErr.Parameters["prompt_file"] = delivery.PromptFile
	opErr.Parameters["may_have_started"] = strconv.FormatBool(delivery.MayHaveStarted)
	opErr.Parameters["post_submit_processing_observed"] = strconv.FormatBool(delivery.PostSubmitProcessing)
	opErr.Parameters["accounting_pending"] = strconv.FormatBool(delivery.AccountingPending)
}

func printCompactionRecoveryReceipt(delivery *ops.SessionCompactionDeliveryResult) {
	if delivery == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "Compaction recovery receipt (non-success):")
	fmt.Fprintf(os.Stderr, "  session_id: %s\n", delivery.SessionID)
	fmt.Fprintf(os.Stderr, "  session_name: %s\n", delivery.Name)
	fmt.Fprintf(os.Stderr, "  tmux_name: %s\n", delivery.TmuxName)
	fmt.Fprintf(os.Stderr, "  harness: %s\n", delivery.Harness)
	fmt.Fprintf(os.Stderr, "  pane_id: %s\n", delivery.PaneID)
	fmt.Fprintf(os.Stderr, "  pane_pid: %d\n", delivery.PanePID)
	fmt.Fprintf(os.Stderr, "  target_pid: %d\n", delivery.TargetPID)
	fmt.Fprintf(os.Stderr, "  harness_start_time: %s\n", delivery.HarnessStartTime)
	fmt.Fprintf(os.Stderr, "  tmux_session_id: %s\n", delivery.TmuxSessionID)
	fmt.Fprintf(os.Stderr, "  attempt_id: %s\n", delivery.AttemptID)
	fmt.Fprintf(os.Stderr, "  attempt_outcome: %s\n", delivery.AttemptOutcome)
	fmt.Fprintf(os.Stderr, "  prompt_file: %s\n", delivery.PromptFile)
	fmt.Fprintf(os.Stderr, "  may_have_started: %t\n", delivery.MayHaveStarted)
	fmt.Fprintf(os.Stderr, "  post_submit_processing_observed: %t\n", delivery.PostSubmitProcessing)
	fmt.Fprintf(os.Stderr, "  accounting_pending: %t\n", delivery.AccountingPending)
}

// verifyCompaction requires a positive post-delivery transition and stable live
// readiness before reporting completion.
func verifyCompaction(ctx context.Context, target compaction.VerificationTarget, timeout time.Duration) error {
	return verifyCompactionWithRunner(ctx, target, timeout, runCompactionVerifier)
}

type compactionVerifierRunner func(context.Context, compaction.VerificationTarget, time.Duration, time.Duration) (compaction.Verification, error)

func verifyCompactionWithRunner(ctx context.Context, target compaction.VerificationTarget, timeout time.Duration, run compactionVerifierRunner) error {
	const pollInterval = 10 * time.Second

	fmt.Printf("Verifying compaction completion (polling every %s, timeout %s)...\n", pollInterval, timeout)
	verification, err := run(ctx, target, timeout, pollInterval)
	if err != nil {
		return err
	}
	ui.PrintSuccess(fmt.Sprintf("Compaction verified complete in %s", verification.Elapsed.Round(time.Second)))
	return nil
}

func runOptionalCompactionVerification(enabled bool, verify func() error) error {
	if !enabled {
		return nil
	}
	return verify()
}

func verificationTarget(delivery *ops.SessionCompactionDeliveryResult) compaction.VerificationTarget {
	if delivery == nil {
		return compaction.VerificationTarget{}
	}
	return compaction.VerificationTarget{
		SessionName:               delivery.TmuxName,
		Harness:                   delivery.Harness,
		PaneID:                    delivery.PaneID,
		PanePID:                   delivery.PanePID,
		TargetPID:                 delivery.TargetPID,
		HarnessStartTime:          delivery.HarnessStartTime,
		TargetSessionID:           delivery.TmuxSessionID,
		StableSessionID:           delivery.SessionID,
		InitialProcessingObserved: delivery.PostSubmitProcessing,
	}
}

func runCompactionVerifier(ctx context.Context, target compaction.VerificationTarget, timeout, pollInterval time.Duration) (compaction.Verification, error) {
	startedAt := time.Now()
	lastState := ""
	lastEvidence := ""
	observer := compaction.StateObserver(func(ctx context.Context, observedTarget compaction.VerificationTarget) (*session.DetectionResult, error) {
		result, err := compaction.ObserveVerificationTarget(ctx, observedTarget)
		if err != nil {
			return nil, err
		}
		if result == nil {
			return nil, fmt.Errorf("session detector returned no result")
		}
		if !isJSONOutput() && (result.State != lastState || string(result.Evidence) != lastEvidence) {
			fmt.Printf("  [%s] State: %s (evidence: %s)\n", time.Since(startedAt).Round(time.Second), result.State, result.Evidence)
			lastState = result.State
			lastEvidence = string(result.Evidence)
		}
		return result, nil
	})
	return compaction.NewVerifier(observer, pollInterval).Verify(ctx, target, timeout)
}
