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

// relayPlan is one completion event's routing decision, taken from a single
// read of the live relay target.
//
// Resolving once per event is what makes the self-filter sound. The target
// is live state that Dispatch can rewrite at any moment, so filtering
// against one read and then delivering against a second leaves a window:
// if the target changes to the very session being reported, the event
// passes the filter (old target) and is then relayed into that session
// (new target), waking it up so it completes again and relays again. One
// snapshot per event closes that window by construction.
type relayPlan struct {
	// surface reports whether this event should be delivered at all.
	surface bool
	// target is the relay recipient this event was filtered against, and
	// the only one it may be delivered to.
	target string
}

// planFor decides how one completion event is routed. It filters out the
// relay target itself (a relay into it would re-trigger a completion,
// looping forever) and any name matching the configured exclude
// substrings.
func (cs *completionSurfacer) planFor(event ops.CompletionEvent) relayPlan {
	target := cs.relayTarget()
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
// as naming a session. Both directions of a wrong answer hurt, but not
// equally: failing to match relays a session's completion into itself and
// loops, while matching too eagerly silently drops an unrelated worker's
// result. A short prefix is the only case where over-matching is
// plausible, so prefixes below this length are not matched at all, and 8
// hex characters make a collision with an unrelated session negligible.
const minIDPrefixMatch = 8

// eventIsSession reports whether target names event's session. The relay
// target may be set to any of the three identifiers AGM accepts for a
// session (see ops.SendMessageRequest.Recipient): its name, its full
// session ID, or a UUID prefix of that ID. Comparing only the name, as
// this filter first did, let every ID-shaped target slip past.
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

// Surface delivers one completion event using the plan planFor produced for
// it. Best-effort per channel: a failed dispatcher or relay never blocks the
// watch loop, and errors are returned joined for logging only.
func (cs *completionSurfacer) Surface(ctx context.Context, event ops.CompletionEvent, plan relayPlan) []error {
	// Defense in depth. The caller checks plan.surface before calling, but
	// the self-filter is the guard against relaying a session's completion
	// into itself, so it is re-asserted where delivery happens rather than
	// trusted to stay correct at every future call site.
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

	// plan.target, not a fresh resolve: this event was filtered against
	// that snapshot, and re-reading here would reopen the self-relay window
	// planFor exists to close.
	if target := plan.target; target != "" {
		message := fmt.Sprintf(
			"[completion-watcher] Session %q (%s) %s. Result tail:\n%s\n(Full output: %s)",
			event.SessionName, event.Harness, describeTransition(event.TransitionType),
			tailExcerpt(event.Output, 1200), fullOutputHint(event),
		)
		// Propagate the caller's cancellation into the relay: OpContext.Context
		// is the ops layer's cancellation carrier, and the shared cs.opCtx must
		// not be mutated (Surface can run concurrently with other ops users).
		relayCtx := *cs.opCtx
		relayCtx.Context = ctx
		if _, err := ops.SendMessage(&relayCtx, &ops.SendMessageRequest{
			Recipient: target,
			Message:   message,
		}); err != nil {
			errs = append(errs, fmt.Errorf("orchestrator relay: %w", err))
		}
	}

	return errs
}

func (cs *completionSurfacer) relayTarget() string {
	return resolveCompletionRelayTarget(cs.orchestrator)
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
