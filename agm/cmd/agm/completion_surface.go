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
	// router routes completions to a live supervisor. Held rather than
	// built per event so one dedupe/queue configuration governs the run.
	router *ops.AlertRouter
	// notificationsDisabled records that an explicitly loaded config
	// resolved to zero dispatchers, which is the documented way to turn
	// completion notifications off.
	notificationsDisabled bool
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
		router:       ops.NewAlertRouter(opCtx),
		// An explicit config that resolves to zero dispatchers means the
		// operator turned completion notifications off. Routing has to
		// honor that too: otherwise the alert route would discover a
		// supervisor and deliver (or durably record) every completion
		// anyway, making the documented off switch unexpressible.
		notificationsDisabled: explicitConfigLoaded && len(dispatchers) == 0,
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

// relayPlan is one completion event's routing decision, taken from a single
// resolution of the alert router's target.
//
// The self-filter is only sound if the target it filters against is the
// target delivery uses. --orchestrator now defaults to empty and routing
// discovers a live supervisor, so filtering on cs.orchestrator alone could
// not recognize the discovered session: a completion from Dispatch would
// pass the filter and then be routed straight back into Dispatch, waking
// it so it completes again. Resolving once per event and passing that
// snapshot to Route as the explicit target closes the loop by
// construction.
type relayPlan struct {
	// surface reports whether this event should be delivered at all.
	surface bool
	// target is the recipient this event was filtered against, and the
	// only one it may be delivered to.
	target string
}

// planFor decides how one completion event is routed. It filters out the
// routing target itself and any name matching the configured excludes.
func (cs *completionSurfacer) planFor(event ops.CompletionEvent) relayPlan {
	if cs.notificationsDisabled {
		return relayPlan{surface: false}
	}
	target := cs.router.ResolveTarget(cs.orchestrator)
	if target != "" && eventIsSession(event, target) {
		return relayPlan{surface: false, target: target}
	}
	lowerName := strings.ToLower(event.SessionName)
	for _, exclude := range cs.excludes {
		if strings.Contains(lowerName, exclude) {
			return relayPlan{surface: false, target: target}
		}
	}
	return relayPlan{surface: true, target: target}
}

// minIDPrefixMatch is the shortest UUID prefix the self-filter will treat
// as naming a session. The two ways to be wrong are not symmetric: failing
// to match relays a session's completion into itself and loops, while
// matching too eagerly silently drops an unrelated worker's result. Short
// prefixes are the only plausible over-match, so they are not matched, and
// 8 hex characters make an accidental collision negligible.
const minIDPrefixMatch = 8

// eventIsSession reports whether target names event's session, across every
// identifier AGM accepts for one (see ops.SendMessageRequest.Recipient):
// its name, its full session ID, or a UUID prefix of that ID.
func eventIsSession(event ops.CompletionEvent, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(event.SessionName), target) {
		return true
	}
	id := strings.TrimSpace(event.SessionID)
	if id == "" {
		return false
	}
	if strings.EqualFold(id, target) {
		return true
	}
	return len(target) >= minIDPrefixMatch &&
		len(target) < len(id) &&
		strings.EqualFold(id[:len(target)], target)
}

// Surface delivers one completion event. Best-effort per channel: a failed
// dispatcher or relay never blocks the watch loop, and errors are returned
// joined for logging only.
func (cs *completionSurfacer) Surface(ctx context.Context, event ops.CompletionEvent, plan relayPlan) []error {
	// Defense in depth: never deliver an event its own plan rejected. The
	// caller checks plan.surface, but this is the guard against relaying a
	// session's completion into itself, so it is re-asserted at delivery.
	if !plan.surface {
		return nil
	}

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

	if _, err := cs.router.Route(ctx, ops.AlertRequest{
		Kind:          "completion",
		Source:        "agm-completion-watcher",
		Title:         fmt.Sprintf("AGM session %s: %s", event.TransitionType, event.SessionName),
		Body:          completionBody(event) + "\nFull output: " + fullOutputHint(event),
		Subject:       event.SessionName,
		Severity:      ops.AlertSeverityInfo,
		Actionability: ops.AlertAgentActionable,
		// plan.target, not a fresh resolve: this event was filtered
		// against that snapshot, and letting Route rediscover its own
		// would reopen the self-relay window planFor exists to close.
		Target:     plan.target,
		OccurredAt: event.DetectedAt,
		// Two completions from one long-lived session share every other
		// dedupe field, so without an occurrence identity the second
		// result would be discarded as a duplicate and never relayed.
		DedupeKey: fmt.Sprintf("%s:%d", event.SessionID, event.DetectedAt.UnixNano()),
		Meta: map[string]any{
			"session_id":   event.SessionID,
			"session_name": event.SessionName,
			"harness":      event.Harness,
			"transition":   event.TransitionType,
		},
	}); err != nil {
		errs = append(errs, fmt.Errorf("alert route: %w", err))
	}

	return errs
}

func (cs *completionSurfacer) Close() {
	for _, d := range cs.dispatchers {
		_ = d.Close()
	}
}

// fullOutputHint names a recovery path that actually works for the transition
// being reported. `agm capture` reads the live pane and errors once the tmux
// session is gone, so advertising it for an "exited" event would send the
// operator to a command that cannot return the durable capture. Only the
// get_session_output read path falls back to the persisted final output.
func fullOutputHint(event ops.CompletionEvent) string {
	if event.TransitionType == "exited" {
		return "agm_get_session_output — the pane is gone, so only the durable final capture remains"
	}
	return fmt.Sprintf("agm_get_session_output / agm capture %s", event.SessionName)
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
