package ops

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

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
	// ReadinessBlindConfirmMultiplier extends the idle debounce for harnesses
	// whose composer stays input-ready while the model is generating
	// (codex-cli's own test fixture documents this), so readiness cannot
	// distinguish idle from a quiet generation or tool interval. A longer
	// stability window sharply reduces — it cannot fully eliminate —
	// premature completions for those harnesses; a definitive per-harness
	// idle signal is tracked as follow-up. Default 3.
	ReadinessBlindConfirmMultiplier int

	observations map[string]*sessionObservation
}

func readinessBlindHarness(harness string) bool {
	switch strings.ToLower(strings.TrimSpace(harness)) {
	case "codex-cli", "codex":
		return true
	}
	return false
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
		ctx:                             ctx,
		IdleConfirmTicks:                2,
		CaptureLines:                    200,
		MaxCaptureBytes:                 16 * 1024,
		ReadinessBlindConfirmMultiplier: 3,
		observations:                    map[string]*sessionObservation{},
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

	exists, err := cw.sessionExists(ctx, tmuxName)
	if err != nil {
		return nil // a tmux backend failure proves nothing about the session
	}

	if !exists {
		return cw.observeGone(obs, m)
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
			// First observation establishes the content baseline. A restart
			// (launchd bounce, host reboot) must not swallow a completion
			// that happened while the watcher was down: the durable session
			// state — persisted by this same pipeline — says whether the
			// session was mid-work when last seen. Treat a persisted WORKING
			// state as observed activity so the normal stability + readiness
			// path can emit the missed completion; sessions with no such
			// evidence stay baseline-only, so pre-watcher history is never
			// replayed.
			obs.baselined = true
			if strings.EqualFold(strings.TrimSpace(m.State), manifest.StateWorking) {
				obs.activitySeen = true
			}
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
	if obs.stableTicks < cw.requiredStableTicks(m.Harness) {
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
	cw.persistCompletion(m.SessionID, manifest.StateDone, obs.lastTail)
	return event
}

// observeGone handles a session whose tmux target is confirmed absent. Only
// sessions the watcher saw alive report an exit: reporting records whose tmux
// predates the watcher would replay history.
func (cw *CompletionWatcher) observeGone(obs *sessionObservation, m *manifest.Manifest) *CompletionEvent {
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
	cw.persistCompletion(m.SessionID, manifest.StateOffline, obs.lastTail)
	return event
}

// requiredStableTicks is the idle debounce for one harness: the configured
// baseline, extended for readiness-blind harnesses whose composer cannot
// signal busy.
func (cw *CompletionWatcher) requiredStableTicks(harness string) int {
	required := cw.IdleConfirmTicks
	if readinessBlindHarness(harness) && cw.ReadinessBlindConfirmMultiplier > 1 {
		required *= cw.ReadinessBlindConfirmMultiplier
	}
	return required
}

// sessionExists resolves tmux ground truth for exit detection. It prefers the
// strict capability, which distinguishes an absent exact target from socket,
// permission, and timeout failures — the plain HasSession collapses every
// tmux exec failure into "absent", which would turn a transient tmux outage
// into a wave of false "exited" completions and OFFLINE records. When only
// the plain checker exists (test fakes), its answer is used as-is.
func (cw *CompletionWatcher) sessionExists(ctx context.Context, tmuxName string) (bool, error) {
	if strict, ok := cw.ctx.Tmux.(session.StrictSessionExistenceChecker); ok {
		return strict.HasSessionStrict(ctx, tmuxName)
	}
	return cw.ctx.Tmux.HasSession(tmuxName)
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
		// A byte slice can start mid-rune; drop the partial rune so the
		// persisted capture stays valid UTF-8.
		for i := 0; i < len(tail) && i < utf8.UTFMax; i++ {
			if utf8.RuneStart(tail[i]) {
				tail = tail[i:]
				break
			}
		}
	}
	return tail, true
}

// persistCompletion durably records the completion on the session record so
// the result survives pane teardown and is readable by every metadata surface.
// It re-reads the current record and patches only the completion fields — the
// manifest observed at scan start may be stale, and rewriting it wholesale
// could clobber concurrent updates (tags, notes, harness switches) made since.
// Best-effort: a storage failure must not stop the watch loop and the event is
// still reported for notification (the notification itself carries the output,
// so the operator never loses the result), but the failure is logged loudly
// rather than discarded.
func (cw *CompletionWatcher) persistCompletion(sessionID string, state string, output string) {
	current, err := cw.ctx.Storage.GetSession(sessionID)
	if err != nil || current == nil {
		slog.Warn("completion-watcher: could not load session for durable completion write; notification still carries the output",
			"session_id", sessionID, "error", err)
		return
	}
	current.State = state
	current.StateUpdatedAt = time.Now()
	current.StateSource = "completion-watcher"
	if output != "" {
		current.FinalOutput = output
		current.FinalOutputAt = time.Now()
	}
	if err := cw.ctx.Storage.UpdateSession(current); err != nil {
		slog.Warn("completion-watcher: durable completion write failed; notification still carries the output",
			"session_id", sessionID, "state", state, "error", err)
	}
}
