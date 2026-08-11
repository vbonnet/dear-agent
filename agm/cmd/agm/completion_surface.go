package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/pkg/notify"
)

// completionSurfacer delivers completion events to the operator: through the
// configured notify dispatchers (log/desktop/webhook/tmux — no person or
// device hardcoded) and, when an orchestrator session is named, as a relay
// message into that session so orchestrators receive worker results without
// polling.
type completionSurfacer struct {
	dispatchers  []notify.Dispatcher
	opCtx        *ops.OpContext
	orchestrator string
	excludes     []string
}

// defaultNotifyConfigPath is where the watcher looks for dispatcher config
// when --notify-config is not given.
func defaultNotifyConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".agm", "notify.yaml")
}

func newCompletionSurfacer(opCtx *ops.OpContext, configPath, orchestrator, excludeCSV string) (*completionSurfacer, error) {
	path := configPath
	if path == "" {
		path = defaultNotifyConfigPath()
	}

	var dispatchers []notify.Dispatcher
	// An explicitly requested config is honored exactly, including when it
	// resolves to zero dispatchers (empty list, or every entry disabled) —
	// that is the only way to turn completion notifications off, and silently
	// installing the log fallback would make it unexpressible.
	explicitConfigLoaded := false
	if path != "" {
		if _, statErr := os.Stat(path); statErr == nil {
			built, err := loadDispatchers(path)
			if err != nil {
				if configPath != "" {
					// An explicitly requested config must be honored exactly.
					return nil, err
				}
				// A malformed IMPLICIT ~/.agm/notify.yaml must never take the
				// whole watch loop down with it: completion watching defaults
				// on inside `agm watch-stalled`, whose stall recovery
				// (permission/no-commit/error-loop monitoring) predates this
				// feature and must survive an optional notification config.
				// Fall back to the stderr log dispatcher and say so.
				slog.Warn("completion-surfacer: implicit notify config is unusable; falling back to log-only notifications (stall recovery unaffected)",
					"path", path, "error", err)
			} else {
				dispatchers = built
				explicitConfigLoaded = configPath != ""
			}
		} else if configPath != "" {
			// An explicitly named config that does not exist is an error; the
			// implicit default location is allowed to be absent.
			return nil, fmt.Errorf("notify config not found: %s", configPath)
		}
	}
	if len(dispatchers) == 0 && !explicitConfigLoaded {
		dispatchers = []notify.Dispatcher{notify.NewLogDispatcher(slog.New(slog.NewTextHandler(os.Stderr, nil)))}
	}

	var excludes []string
	for part := range strings.SplitSeq(excludeCSV, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			excludes = append(excludes, strings.ToLower(trimmed))
		}
	}

	return &completionSurfacer{
		dispatchers:  dispatchers,
		opCtx:        opCtx,
		orchestrator: orchestrator,
		excludes:     excludes,
	}, nil
}

// loadDispatchers reads and builds the notify dispatcher set from one config
// path, keeping load and build failures as one seam for the implicit-config
// fallback above.
func loadDispatchers(path string) ([]notify.Dispatcher, error) {
	cfg, err := notify.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("load notify config %s: %w", path, err)
	}
	built, err := notify.BuildDispatchers(cfg, slog.Default())
	if err != nil {
		return nil, fmt.Errorf("build notify dispatchers: %w", err)
	}
	return built, nil
}

// shouldSurface filters out the orchestrator session itself (a relay into it
// would re-trigger a completion, looping forever) and any name matching the
// configured exclude substrings.
func (cs *completionSurfacer) shouldSurface(event ops.CompletionEvent) bool {
	if cs.orchestrator != "" && event.SessionName == cs.orchestrator {
		return false
	}
	lowerName := strings.ToLower(event.SessionName)
	for _, exclude := range cs.excludes {
		if strings.Contains(lowerName, exclude) {
			return false
		}
	}
	return true
}

// Surface delivers one completion event. Best-effort per channel: a failed
// dispatcher or relay never blocks the watch loop, and errors are returned
// joined for logging only.
func (cs *completionSurfacer) Surface(ctx context.Context, event ops.CompletionEvent) []error {
	var errs []error

	n := &notify.Notification{
		ID:        fmt.Sprintf("completion-%s-%d", event.SessionID, event.DetectedAt.Unix()),
		Title:     fmt.Sprintf("AGM session %s: %s", event.TransitionType, event.SessionName),
		Body:      completionBody(event),
		Level:     slog.LevelInfo,
		Source:    "agm-completion-watcher",
		Timestamp: event.DetectedAt,
		Meta: map[string]any{
			"session_id":   event.SessionID,
			"session_name": event.SessionName,
			"harness":      event.Harness,
			"transition":   event.TransitionType,
		},
	}
	for _, d := range cs.dispatchers {
		if err := d.Dispatch(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("dispatch %s: %w", d.Name(), err))
		}
	}

	if cs.orchestrator != "" {
		message := fmt.Sprintf(
			"[completion-watcher] Session %q (%s) %s. Result tail:\n%s\n(Full output: agm_get_session_output / agm capture %s)",
			event.SessionName, event.Harness, describeTransition(event.TransitionType),
			tailExcerpt(event.Output, 1200), event.SessionName,
		)
		// Propagate the caller's cancellation into the relay: OpContext.Context
		// is the ops layer's cancellation carrier, and the shared cs.opCtx must
		// not be mutated (Surface can run concurrently with other ops users).
		relayCtx := *cs.opCtx
		relayCtx.Context = ctx
		if _, err := ops.SendMessage(&relayCtx, &ops.SendMessageRequest{
			Recipient: cs.orchestrator,
			Message:   message,
		}); err != nil {
			errs = append(errs, fmt.Errorf("orchestrator relay: %w", err))
		}
	}

	return errs
}

func (cs *completionSurfacer) Close() {
	for _, d := range cs.dispatchers {
		_ = d.Close()
	}
}

func completionBody(event ops.CompletionEvent) string {
	return fmt.Sprintf("Session %q (%s) %s.\n%s",
		event.SessionName, event.Harness, describeTransition(event.TransitionType),
		tailExcerpt(event.Output, 500))
}

func describeTransition(transition string) string {
	switch transition {
	case "idle":
		return "finished working and is idle"
	case "exited":
		return "exited (tmux session ended)"
	default:
		return transition
	}
}

// tailExcerpt returns the last maxBytes of output, trimmed to whole lines.
func tailExcerpt(output string, maxBytes int) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "(no output captured)"
	}
	if len(trimmed) <= maxBytes {
		return trimmed
	}
	cut := trimmed[len(trimmed)-maxBytes:]
	if idx := strings.IndexByte(cut, '\n'); idx >= 0 && idx < len(cut)-1 {
		cut = cut[idx+1:]
	}
	return cut
}
