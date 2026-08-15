package ops

import (
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// GetSessionOutputRequest defines the input for reading a session's output.
type GetSessionOutputRequest struct {
	// Identifier is a session ID, name, or UUID prefix.
	Identifier string `json:"identifier"`
	// Lines caps how many trailing pane lines to capture (default 100, max 2000).
	Lines int `json:"lines,omitempty"`
}

// GetSessionOutputResult is the output of GetSessionOutput.
type GetSessionOutputResult struct {
	Operation string `json:"operation"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	State     string `json:"state,omitempty"`
	// Source records where Output came from: "live-pane" when captured from the
	// running tmux pane, "final-capture" when returned from the durable
	// completion capture persisted on the session record.
	Source     string `json:"source"`
	Output     string `json:"output"`
	CapturedAt string `json:"captured_at,omitempty"`
}

const (
	defaultOutputLines = 100
	maxOutputLines     = 2000
)

// GetSessionOutput returns the tail of a session's terminal output. It reads
// the live tmux pane when the session is running; when the pane is gone it
// falls back to the durable final-output capture, so results remain readable
// after the session ends. This is the read path orchestrators use to collect
// worker results without attaching to panes.
func GetSessionOutput(ctx *OpContext, req *GetSessionOutputRequest) (*GetSessionOutputResult, error) {
	if req == nil || req.Identifier == "" {
		return nil, ErrInvalidInput("identifier", "Session identifier is required. Provide a session ID, name, or UUID prefix.")
	}
	lines := req.Lines
	if lines <= 0 {
		lines = defaultOutputLines
	}
	if lines > maxOutputLines {
		lines = maxOutputLines
	}

	m, err := resolveSessionByIdentifier(ctx, req.Identifier)
	if err != nil {
		return nil, err
	}

	status := computeSessionStatus(m, ctx.Tmux)
	result := &GetSessionOutputResult{
		Operation: "get_session_output",
		SessionID: m.SessionID,
		Name:      m.Name,
		Status:    status,
		State:     m.State,
	}

	// Canonical helper: the raw display name can normalize to a different
	// target than the pane the session was created with (see CompletionWatcher).
	tmuxName := session.TmuxSessionName(m)

	if done, err := captureLiveOutput(ctx, result, status, tmuxName, lines); done {
		return result, err
	} else if err != nil {
		return nil, err
	}

	if m.FinalOutput != "" {
		// The durable capture describes an EARLIER completion, so it may only
		// answer for a pane that is provably gone. Reaching here on a non-live
		// status is not that proof: a tmux socket outage or permission failure
		// makes the plain existence check report "absent", which computes as
		// status=stopped and would serve a previous task's output as current.
		if err := requireProvenPaneAbsence(ctx, tmuxName); err != nil {
			return nil, err
		}
		result.Source = "final-capture"
		result.Output = tailLines(m.FinalOutput, lines)
		if !m.FinalOutputAt.IsZero() {
			result.CapturedAt = m.FinalOutputAt.UTC().Format(time.RFC3339)
		}
		return result, nil
	}

	return nil, ErrInvalidInput("identifier", "No output is available for this session: the tmux pane is not readable and no final output was captured.")
}

// requireProvenPaneAbsence returns nil only when the tmux target is confirmed
// absent. It prefers the strict checker, which separates "no such session"
// from socket, permission, and timeout failures; the plain checker collapses
// every exec failure into "absent", so a backend outage would otherwise look
// like a finished session. When only the plain checker exists (test fakes),
// its answer is used as-is.
func requireProvenPaneAbsence(ctx *OpContext, tmuxName string) error {
	reason := "session liveness could not be confirmed, so the durable final capture (which describes an earlier completion) was not served"
	if strict, ok := ctx.Tmux.(session.StrictSessionExistenceChecker); ok {
		exists, err := strict.HasSessionStrict(requestContext(ctx), tmuxName)
		if err != nil || exists {
			return ErrOutputUnavailable(tmuxName, reason, err)
		}
		return nil
	}
	exists, err := ctx.Tmux.HasSession(tmuxName)
	if err != nil || exists {
		return ErrOutputUnavailable(tmuxName, reason, err)
	}
	return nil
}

// captureLiveOutput attempts the live-pane read for an active session. It
// reports done=true with the populated result on success. A transient capture
// failure on a still-live pane must not silently answer with a durable capture
// from an earlier task — the caller asked for current output and a stale
// result would misrepresent the session — so that case returns an error and
// the final-capture fallback is reserved for a pane that is actually gone.
func captureLiveOutput(ctx *OpContext, result *GetSessionOutputResult, status, tmuxName string, lines int) (bool, error) {
	capturer, ok := ctx.Tmux.(session.PaneOutputCapturer)
	if !ok || (status != "active" && status != "zombie") {
		return false, nil
	}
	output, captureErr := capturer.CapturePaneTail(tmuxName, lines)
	if captureErr == nil && strings.TrimSpace(output) != "" {
		result.Source = "live-pane"
		result.Output = output
		result.CapturedAt = time.Now().UTC().Format(time.RFC3339)
		return true, nil
	}
	// Serve the durable capture only when the pane is PROVEN gone: a liveness
	// probe failure (socket outage, permission) cannot distinguish a dead
	// session from an unreachable one, and answering with an earlier task's
	// capture would misrepresent the current one. Prefer the strict checker
	// for the same reason requireProvenPaneAbsence does: the plain HasSession
	// collapses socket/permission errors into (false, nil), which would
	// misclassify a backend outage as a proven-absent pane.
	var stillExists bool
	var existsErr error
	if strict, ok := ctx.Tmux.(session.StrictSessionExistenceChecker); ok {
		stillExists, existsErr = strict.HasSessionStrict(requestContext(ctx), tmuxName)
	} else {
		stillExists, existsErr = ctx.Tmux.HasSession(tmuxName)
	}
	if existsErr != nil || stillExists {
		return false, ErrOutputUnavailable(tmuxName,
			"live capture failed for a running session (a durable final capture, if any, describes an earlier completion, not the current task)",
			existsErr)
	}
	return false, nil
}

// tailLines returns the last n lines of s, honoring the caller's requested
// line budget for durable final-capture output exactly as the live pane path
// does.
func tailLines(s string, n int) string {
	if n <= 0 {
		return s
	}
	// A trailing newline is a delimiter, not an empty final line — without
	// trimming it, the last requested "line" would be the empty string.
	trimmed := strings.TrimSuffix(s, "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// resolveSessionByIdentifier resolves ID, then name/tmux-name, then Claude UUID
// — the same order GetSession uses.
func resolveSessionByIdentifier(ctx *OpContext, identifier string) (*manifest.Manifest, error) {
	m, err := ctx.Storage.GetSession(identifier)
	if err != nil || m == nil {
		if m2, nameErr := findByName(ctx, identifier); nameErr == nil {
			return m2, nil
		}
		m, err = ctx.Storage.GetSessionByUUID(identifier)
		if err != nil {
			return nil, err
		}
	}
	if m == nil {
		return nil, ErrSessionNotFound(identifier)
	}
	return m, nil
}
