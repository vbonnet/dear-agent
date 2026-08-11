package ops

import (
	"context"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// CompletionEvent reports that a session finished a unit of work: its harness
// went idle after being busy, or its tmux session ended.
type CompletionEvent struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name"`
	Harness     string `json:"harness"`
	// TransitionType is "idle" (harness composer became input-ready after
	// being busy) or "exited" (the tmux session is gone).
	TransitionType string    `json:"transition_type"`
	Output         string    `json:"output,omitempty"`
	DetectedAt     time.Time `json:"detected_at"`
}

// CompletionWatcher observes sessions from the supervisor side — tmux ground
// truth, no reliance on in-session hooks — so it works identically for every
// harness and every create surface (CLI, MCP, supervisor). Work is detected
// as pane-content change; completion as content stability with an input-ready
// composer. When a session completes, the watcher durably persists the result
// tail onto the session record (FinalOutput) and stamps State, then reports
// the event for notification. This is the capture half of the completion →
// capture → surface pipeline (ce-0zng9).
type CompletionWatcher struct {
	ctx *OpContext
	// IdleConfirmTicks is how many consecutive stable, input-ready
	// observations are required after activity before a completion is
	// reported (debounce; default 2).
	IdleConfirmTicks int
	// CaptureLines is how many trailing pane lines are captured (default 200).
	CaptureLines int
	// MaxCaptureBytes caps the persisted final output (default 16 KiB).
	MaxCaptureBytes int

	observations map[string]*sessionObservation
}

type sessionObservation struct {
	// activitySeen is set when the pane content changed between ticks —
	// harness-agnostic proof the session did work since the last completion.
	// Input readiness alone cannot detect activity: some harnesses (codex-cli)
	// keep the composer input-ready while the model is generating.
	activitySeen bool
	stableTicks  int
	reportedIdle bool
	everExisted  bool
	reportedGone bool
	lastTail     string
	baselined    bool
}

// NewCompletionWatcher creates a watcher with default thresholds.
func NewCompletionWatcher(ctx *OpContext) *CompletionWatcher {
	return &CompletionWatcher{
		ctx:              ctx,
		IdleConfirmTicks: 2,
		CaptureLines:     200,
		MaxCaptureBytes:  16 * 1024,
		observations:     map[string]*sessionObservation{},
	}
}

// Scan observes every non-archived session once and returns completions that
// crossed the detection threshold since the previous scan. Callers run it on
// an interval; the watcher keeps per-session observation state in memory.
func (cw *CompletionWatcher) Scan(ctx context.Context) ([]CompletionEvent, error) {
	filter := &dolt.SessionFilter{ExcludeArchived: true, Limit: 1000}
	sessions, err := cw.ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, ErrStorageError("completion_watcher.list_sessions", err)
	}

	var events []CompletionEvent
	seen := make(map[string]bool, len(sessions))
	for _, m := range sessions {
		seen[m.SessionID] = true
		if event := cw.observe(ctx, m); event != nil {
			events = append(events, *event)
		}
	}
	// Drop observation state for records that left the active set (archived or
	// deleted) so memory does not grow without bound.
	for id := range cw.observations {
		if !seen[id] {
			delete(cw.observations, id)
		}
	}
	return events, nil
}

func (cw *CompletionWatcher) observe(ctx context.Context, m *manifest.Manifest) *CompletionEvent {
	obs := cw.observations[m.SessionID]
	if obs == nil {
		obs = &sessionObservation{}
		cw.observations[m.SessionID] = obs
	}

	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}

	exists := false
	if checker, ok := cw.ctx.Tmux.(interface {
		HasSession(name string) (bool, error)
	}); ok {
		var err error
		exists, err = checker.HasSession(tmuxName)
		if err != nil {
			return nil // an unreadable tmux proves nothing
		}
	} else {
		return nil
	}

	if !exists {
		// Only report an exit for sessions the watcher saw alive: reporting
		// records whose tmux predates the watcher would replay history.
		if !obs.everExisted || obs.reportedGone {
			return nil
		}
		obs.reportedGone = true
		event := &CompletionEvent{
			SessionID:      m.SessionID,
			SessionName:    m.Name,
			Harness:        m.Harness,
			TransitionType: "exited",
			Output:         obs.lastTail,
			DetectedAt:     time.Now(),
		}
		cw.persistCompletion(m, manifest.StateOffline, obs.lastTail)
		return event
	}
	obs.everExisted = true
	obs.reportedGone = false

	// Pane activity is the busy signal: content changed since the last tick
	// means the session did work. This also keeps a rolling tail so an exit
	// between ticks still has output to attach.
	tail, captured := cw.capturePaneTail(tmuxName)
	if captured {
		changed := obs.lastTail != tail
		obs.lastTail = tail
		if !obs.baselined {
			// First observation only establishes the baseline; a difference
			// from the zero value is not activity.
			obs.baselined = true
		} else if changed {
			obs.activitySeen = true
			obs.stableTicks = 0
			obs.reportedIdle = false
			return nil
		}
	}
	if !obs.activitySeen || obs.reportedIdle {
		return nil
	}
	// Content is stable. Completion additionally requires the composer to be
	// input-ready so a session parked on a permission prompt or overlay is
	// left to the stall detector rather than reported as done.
	if !cw.harnessIdle(ctx, tmuxName, m.Harness) {
		return nil
	}
	obs.stableTicks++
	if obs.stableTicks < cw.IdleConfirmTicks {
		return nil
	}

	obs.reportedIdle = true
	obs.activitySeen = false
	obs.stableTicks = 0
	event := &CompletionEvent{
		SessionID:      m.SessionID,
		SessionName:    m.Name,
		Harness:        m.Harness,
		TransitionType: "idle",
		Output:         obs.lastTail,
		DetectedAt:     time.Now(),
	}
	cw.persistCompletion(m, manifest.StateDone, obs.lastTail)
	return event
}

// harnessIdle reports whether the harness composer is input-ready. A session
// that cannot be classified is treated as busy — a failed probe proves nothing
// and must not fire a completion.
func (cw *CompletionWatcher) harnessIdle(ctx context.Context, tmuxName, harness string) bool {
	checker, ok := cw.ctx.Tmux.(session.InputReadinessChecker)
	if !ok {
		return false
	}
	readiness, err := checker.CheckInputReadiness(ctx, tmuxName, harness)
	if err != nil {
		return false
	}
	return readiness.Ready
}

func (cw *CompletionWatcher) capturePaneTail(tmuxName string) (string, bool) {
	capturer, ok := cw.ctx.Tmux.(session.PaneOutputCapturer)
	if !ok {
		return "", false
	}
	tail, err := capturer.CapturePaneTail(tmuxName, cw.CaptureLines)
	if err != nil || strings.TrimSpace(tail) == "" {
		return "", false
	}
	if len(tail) > cw.MaxCaptureBytes {
		tail = tail[len(tail)-cw.MaxCaptureBytes:]
	}
	return tail, true
}

// persistCompletion durably records the completion on the session record so
// the result survives pane teardown and is readable by every metadata surface.
// Best-effort: a storage failure must not stop the watch loop, and the event
// is still reported for notification.
func (cw *CompletionWatcher) persistCompletion(m *manifest.Manifest, state string, output string) {
	m.State = state
	m.StateUpdatedAt = time.Now()
	m.StateSource = "completion-watcher"
	if output != "" {
		m.FinalOutput = output
		m.FinalOutputAt = time.Now()
	}
	_ = cw.ctx.Storage.UpdateSession(m)
}
