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

	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}

	if capturer, ok := ctx.Tmux.(session.PaneOutputCapturer); ok && (status == "active" || status == "zombie") {
		output, captureErr := capturer.CapturePaneTail(tmuxName, lines)
		if captureErr == nil && strings.TrimSpace(output) != "" {
			result.Source = "live-pane"
			result.Output = output
			result.CapturedAt = time.Now().UTC().Format(time.RFC3339)
			return result, nil
		}
	}

	if m.FinalOutput != "" {
		result.Source = "final-capture"
		result.Output = m.FinalOutput
		if !m.FinalOutputAt.IsZero() {
			result.CapturedAt = m.FinalOutputAt.UTC().Format(time.RFC3339)
		}
		return result, nil
	}

	return nil, ErrInvalidInput("identifier", "No output is available for this session: the tmux pane is not readable and no final output was captured.")
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
