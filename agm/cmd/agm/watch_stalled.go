package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/binstamp"
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
		"Default session for alerts and completion relays (optional). This is the "+
			"fallback, not a pin: the live relay target (set via 'agm completion "+
			"relay-target set' or agm_set_completion_relay_target, else "+
			"AGM_COMPLETION_RELAY_TARGET) takes precedence so a running watcher can "+
			"be retargeted without a restart.")
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
		// One plan per event: it carries the single target resolution this
		// event is both filtered against and delivered to.
		plan := surfacer.planFor(event)
		if !dryRun && plan.surface {
			if errs := surfacer.Surface(ctx, event, plan); len(errs) > 0 {
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

	// A KeepAlive LaunchAgent restarts this daemon when it crashes, never
	// when its binary is merely reinstalled: the running process keeps the
	// old inode and serves pre-fix behavior indefinitely. Watching our own
	// executable turns a redeploy into a clean exit that launchd answers by
	// starting the new build.
	selfBinary := binstamp.NewWatcher()

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
			// Checked before the scan so a redeploy takes effect on the next
			// tick rather than after another full round of old-code work.
			if selfBinary.Replaced() {
				out := binaryReplacedOutput{
					Timestamp: time.Now().Format(time.RFC3339),
					EventType: "binary_replaced",
					Binary:    selfBinary.Path(),
					Action:    "exiting for supervisor restart",
				}
				data, err := json.Marshal(out)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error marshaling binary replaced event: %v\n", err)
					return nil
				}
				fmt.Println(string(data))
				fmt.Fprintf(os.Stderr, "%s was replaced on disk; exiting so the supervisor restarts this daemon on the new build\n", selfBinary.Path())
				return nil
			}
			// Drain first: an alert queued on an earlier scan reached nobody,
			// and this loop is the only thing that reliably notices a
			// supervisor has come back. Without it, an alert raised during a
			// transient outage would sit in the file until someone looked.
			if !stalledDryRun {
				drainQueuedAlerts(ctx, opCtx)
			}
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

// resolveCompletionRelayTarget reports where completions and stall alerts
// should be delivered right now. fallback is the --orchestrator default,
// which the live relay-target state deliberately outranks; see the flag
// help for why the flag is a fallback rather than a pin.
func resolveCompletionRelayTarget(fallback string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("completion relay: cannot locate home dir; using fallback target",
			"fallback", fallback, "error", err)
		return strings.TrimSpace(fallback)
	}
	result := dispatchstate.ResolveRelayTarget(home, fallback, os.Getenv)
	if result.Reason != "" {
		// A state read that failed for any reason other than "not set" is
		// a degraded resolve, not an absent override: say so rather than
		// letting it look identical to no target having been configured.
		slog.Warn("completion relay: live relay-target state unreadable; falling back",
			"reason", result.Reason, "source", result.Source, "target", result.Target)
	}
	return result.Target
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

// drainQueuedAlerts re-attempts delivery for alerts still recorded as
// queued. It is best-effort: a drain failure must never stop the watch loop
// that stall detection depends on.
func drainQueuedAlerts(ctx context.Context, opCtx *ops.OpContext) {
	delivered, err := ops.NewAlertRouter(opCtx).DrainQueued(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error draining queued alerts: %v\n", err)
		return
	}
	if delivered > 0 {
		data, _ := json.Marshal(map[string]any{
			"timestamp":  time.Now().Format(time.RFC3339),
			"event_type": "alerts_drained",
			"delivered":  delivered,
		})
		fmt.Println(string(data))
	}
}

// binaryReplacedOutput is the JSON event emitted when the daemon notices
// its own executable was reinstalled and exits so the supervisor can
// restart it on the new build.
type binaryReplacedOutput struct {
	Timestamp string `json:"timestamp"`
	EventType string `json:"event_type"`
	Binary    string `json:"binary"`
	Action    string `json:"action"`
}
