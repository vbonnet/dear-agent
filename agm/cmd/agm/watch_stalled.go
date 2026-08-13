package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dispatchstate"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// stallEventOutput represents the JSON output for a stall event.
type stallEventOutput struct {
	Timestamp         string `json:"timestamp"`
	SessionName       string `json:"session_name"`
	StallType         string `json:"stall_type"`
	Duration          string `json:"duration"`
	Severity          string `json:"severity"`
	Evidence          string `json:"evidence"`
	RecommendedAction string `json:"recommended_action"`
}

// recoveryActionOutput represents the JSON output for a recovery action.
type recoveryActionOutput struct {
	Timestamp   string `json:"timestamp"`
	SessionName string `json:"session_name"`
	ActionType  string `json:"action_type"`
	Description string `json:"description"`
	Sent        bool   `json:"sent"`
	Error       string `json:"error,omitempty"`
}

var (
	stalledCheckInterval        time.Duration
	stalledPermissionTimeout    time.Duration
	stalledNoCommitTimeout      time.Duration
	stalledErrorRepeatThreshold int
	stalledOrchestratorName     string
	stalledDryRun               bool
	stalledNotifyConfigPath     string
	stalledWatchCompletions     bool
	stalledCompletionExcludes   string
)

// completionEventOutput represents the JSON output for a completion event.
type completionEventOutput struct {
	Timestamp      string `json:"timestamp"`
	EventType      string `json:"event_type"`
	SessionName    string `json:"session_name"`
	SessionID      string `json:"session_id"`
	Harness        string `json:"harness"`
	TransitionType string `json:"transition_type"`
	OutputBytes    int    `json:"output_bytes"`
	SurfaceErrors  string `json:"surface_errors,omitempty"`
}

var watchStalledCmd = &cobra.Command{
	Use:   "watch-stalled",
	Short: "Watch for stalled sessions and attempt recovery",
	Long: `Monitor active AGM sessions for stall conditions and attempt automated recovery.

Detects three types of stalls:
  1. Permission Prompt (critical): Sessions stuck in permission dialog > 5m
  2. No Commit (warning): Workers making no commits > 15m
  3. Error Loop (warning): Same error appearing 3+ times in output

Actions are JSON-emitted for integration with external monitoring systems.
Use --dry-run to detect stalls without taking recovery actions.

Examples:
  agm watch-stalled
  agm watch-stalled --check-interval 30s --dry-run
  agm watch-stalled --orchestrator my-orchestrator | jq .`,
	RunE: runWatchStalled,
}

func init() {
	watchStalledCmd.Flags().DurationVar(&stalledCheckInterval, "check-interval", 30*time.Second,
		"How often to check for stalls")
	watchStalledCmd.Flags().DurationVar(&stalledPermissionTimeout, "permission-timeout", 5*time.Minute,
		"Timeout for permission prompt stalls")
	watchStalledCmd.Flags().DurationVar(&stalledNoCommitTimeout, "no-commit-timeout", 15*time.Minute,
		"Timeout for no-commit stalls")
	watchStalledCmd.Flags().IntVar(&stalledErrorRepeatThreshold, "error-repeat-threshold", 3,
		"How many repeats of an error = loop")
	watchStalledCmd.Flags().StringVar(&stalledOrchestratorName, "orchestrator", "",
		"Orchestrator session name for alerts (optional)")
	watchStalledCmd.Flags().BoolVar(&stalledDryRun, "dry-run", false,
		"Detect stalls without taking recovery actions")
	watchStalledCmd.Flags().StringVar(&stalledNotifyConfigPath, "notify-config", "",
		"Path to a notify dispatcher config (YAML). Defaults to ~/.agm/notify.yaml; falls back to the log dispatcher when absent.")
	watchStalledCmd.Flags().BoolVar(&stalledWatchCompletions, "watch-completions", true,
		"Also watch for session completions and surface their results")
	watchStalledCmd.Flags().StringVar(&stalledCompletionExcludes, "completion-exclude", "orchestrator,overseer,meta-",
		"Comma-separated name substrings whose sessions never emit completion notifications")
	rootCmd.AddCommand(watchStalledCmd)
}

// emitCompletions scans for finished sessions, prints each event as JSON, and
// surfaces it through the configured channels.
//
// A dry run keeps the JSON preview but delivers nothing: dispatchers can POST
// to webhooks and raise desktop/tmux notifications, and the relay injects a
// message into a live orchestrator session. Those are externally visible acts,
// so a command documented as leaving no trace must skip them exactly as the
// watcher skips its durable writes.
func emitCompletions(ctx context.Context, watcher *ops.CompletionWatcher, surfacer *completionSurfacer, dryRun bool) {
	events, err := watcher.Scan(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error detecting completions: %v\n", err)
		return
	}
	for _, event := range events {
		out := completionEventOutput{
			Timestamp:      time.Now().Format(time.RFC3339),
			EventType:      "completion",
			SessionName:    event.SessionName,
			SessionID:      event.SessionID,
			Harness:        event.Harness,
			TransitionType: event.TransitionType,
			OutputBytes:    len(event.Output),
		}
		if !dryRun && surfacer.shouldSurface(event) {
			if errs := surfacer.Surface(ctx, event); len(errs) > 0 {
				parts := make([]string, len(errs))
				for i, surfaceErr := range errs {
					parts[i] = surfaceErr.Error()
				}
				out.SurfaceErrors = strings.Join(parts, "; ")
			}
		}
		data, _ := json.Marshal(out)
		fmt.Println(string(data))
	}
}

func runWatchStalled(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return handleError(err)
	}
	defer cleanup()

	fmt.Fprintf(os.Stderr, "Watching for stalled sessions...\n")
	fmt.Fprintf(os.Stderr, "  Check interval: %v\n", stalledCheckInterval)
	fmt.Fprintf(os.Stderr, "  Permission timeout: %v\n", stalledPermissionTimeout)
	fmt.Fprintf(os.Stderr, "  No-commit timeout: %v\n", stalledNoCommitTimeout)
	fmt.Fprintf(os.Stderr, "  Dry run: %v\n", stalledDryRun)
	fmt.Fprintf(os.Stderr, "\n")

	ctx := cmd.Context()

	// Create detector and recovery handler
	detector := ops.NewStallDetector(opCtx)
	detector.PermissionTimeout = stalledPermissionTimeout
	detector.NoCommitTimeout = stalledNoCommitTimeout
	detector.ErrorRepeatThreshold = stalledErrorRepeatThreshold

	recovery := ops.NewStallRecovery(opCtx, stalledOrchestratorName)
	recovery.SetOrchestratorTargetResolver(resolveCompletionRelayTarget)

	var completions *ops.CompletionWatcher
	var surfacer *completionSurfacer
	if stalledWatchCompletions {
		completions = ops.NewCompletionWatcher(opCtx)
		// --dry-run must leave no trace: observe and report, but never write
		// State/FinalOutput onto session records.
		completions.DryRun = stalledDryRun
		surfacer, err = newCompletionSurfacer(opCtx, stalledNotifyConfigPath, stalledOrchestratorName, stalledCompletionExcludes)
		if err != nil {
			return handleError(err)
		}
		defer surfacer.Close()
	}

	// Main watch loop
	ticker := time.NewTicker(stalledCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "\nShutting down watcher...")
			return nil
		case <-ticker.C:
			if completions != nil {
				emitCompletions(ctx, completions, surfacer, stalledDryRun)
			}
			events, err := detector.DetectStalls(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error detecting stalls: %v\n", err)
				continue
			}

			// Emit each detected event
			for _, event := range events {
				out := stallEventOutput{
					Timestamp:         time.Now().Format(time.RFC3339),
					SessionName:       event.SessionName,
					StallType:         event.StallType,
					Duration:          formatDurationForJSON(event.Duration),
					Severity:          event.Severity,
					Evidence:          event.Evidence,
					RecommendedAction: event.RecommendedAction,
				}
				data, _ := json.Marshal(out)
				fmt.Println(string(data))

				// Attempt recovery unless dry-run
				if !stalledDryRun {
					action, err := recovery.Recover(ctx, event)
					if err != nil {
						action.Error = err.Error()
					}

					recOut := recoveryActionOutput{
						Timestamp:   time.Now().Format(time.RFC3339),
						SessionName: action.SessionName,
						ActionType:  action.ActionType,
						Description: action.Description,
						Sent:        action.Sent,
						Error:       action.Error,
					}
					data, _ := json.Marshal(recOut)
					fmt.Println(string(data))
				}
			}
		}
	}
}

func resolveCompletionRelayTarget(fallback string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return strings.TrimSpace(fallback)
	}
	return dispatchstate.ResolveRelayTarget(home, fallback, os.Getenv).Target
}

// formatDurationForJSON returns a human-readable duration string.
func formatDurationForJSON(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}
