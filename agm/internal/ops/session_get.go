package ops

import (
	"errors"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// GetSessionRequest defines the input for getting a single session.
type GetSessionRequest struct {
	// Identifier is a session ID, name, or UUID prefix.
	Identifier string `json:"identifier"`
}

// SessionDetail is the full session output with all fields.
type SessionDetail struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Status           string                   `json:"status"`
	State            string                   `json:"state"`
	Harness          string                   `json:"harness"`
	Model            string                   `json:"model,omitempty"`
	Project          string                   `json:"project"`
	Purpose          string                   `json:"purpose,omitempty"`
	Tags             []string                 `json:"tags,omitempty"`
	TmuxSession      string                   `json:"tmux_session"`
	ClaudeUUID       string                   `json:"claude_uuid,omitempty"`
	ParentSessionID  string                   `json:"parent_session_id,omitempty"`
	Workspace        string                   `json:"workspace"`
	WorkingDirectory string                   `json:"working_directory,omitempty"`
	Lifecycle        string                   `json:"lifecycle"`
	CreatedAt        string                   `json:"created_at"`
	UpdatedAt        string                   `json:"updated_at"`
	ContextUsage     *ContextUsage            `json:"context_usage,omitempty"`
	PermissionMode   string                   `json:"permission_mode,omitempty"`
	HarnessHistory   []manifest.HarnessSwitch `json:"harness_history,omitempty"`
	FinalOutput      string                   `json:"final_output,omitempty"`
	FinalOutputAt    string                   `json:"final_output_at,omitempty"`
}

// ContextUsage mirrors manifest.ContextUsage for JSON output.
type ContextUsage struct {
	TotalTokens    int     `json:"total_tokens"`
	UsedTokens     int     `json:"used_tokens"`
	PercentageUsed float64 `json:"percentage_used"`
}

// GetSessionResult is the output of GetSession.
type GetSessionResult struct {
	Operation string        `json:"operation"`
	Session   SessionDetail `json:"session"`
}

// GetSession retrieves a single session by identifier (ID, name, or UUID prefix).
func GetSession(ctx *OpContext, req *GetSessionRequest) (*GetSessionResult, error) {
	if req == nil || req.Identifier == "" {
		return nil, ErrInvalidInput("identifier", "Session identifier is required. Provide a session ID, name, or UUID prefix.")
	}

	// Try exact ID match first.
	m, err := ctx.Storage.GetSession(req.Identifier)
	if err != nil {
		// Try name-based lookup; propagate real storage errors but fall through
		// to the UUID lookup below on not-found — Stop hooks pass a Claude UUID
		// that won't match any AGM ID or session name.
		var nameErr error
		m, nameErr = findByName(ctx, req.Identifier)
		if nameErr != nil {
			var opErr *OpError
			if !errors.As(nameErr, &opErr) || opErr.Code != ErrCodeSessionNotFound {
				return nil, nameErr
			}
			// nameErr is "not found" — continue to Claude UUID fallback.
		}
	}

	// Fall back to Claude UUID lookup — Stop hooks pass the Claude session_id
	// which is distinct from the AGM session ID. This lookup finds the session
	// whose manifest.Claude.UUID matches, enabling lifecycle spans from hooks.
	if m == nil {
		m, err = ctx.Storage.GetSessionByUUID(req.Identifier)
		if err != nil {
			return nil, err
		}
	}

	if m == nil {
		return nil, ErrSessionNotFound(req.Identifier)
	}

	// Compute live status
	status := computeSessionStatus(m, ctx.Tmux)

	detail := toSessionDetail(m, status)

	// Populate harness history (best-effort; ignore errors).
	if history, err := ctx.Storage.GetHarnessHistory(m.SessionID); err == nil {
		detail.HarnessHistory = history
	}

	return &GetSessionResult{
		Operation: "get_session",
		Session:   detail,
	}, nil
}

// findByName searches for a session by name or tmux session name.
func findByName(ctx *OpContext, name string) (*manifest.Manifest, error) {
	return findByNameWithFilter(ctx, name, nil)
}

// findActiveByName excludes retired identities before resolving a reusable
// manifest or tmux name for a live operation such as message delivery.
func findActiveByName(ctx *OpContext, name string) (*manifest.Manifest, error) {
	return findByNameWithFilter(ctx, name, &dolt.SessionFilter{ExcludeArchived: true})
}

func findByNameWithFilter(ctx *OpContext, name string, filter *dolt.SessionFilter) (*manifest.Manifest, error) {
	manifests, err := ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, ErrStorageError("find_by_name", err)
	}

	for _, m := range manifests {
		if m.Name == name || m.Tmux.SessionName == name {
			return m, nil
		}
	}

	return nil, ErrSessionNotFound(name)
}

func toSessionDetail(m *manifest.Manifest, status string) SessionDetail {
	d := SessionDetail{
		ID:               m.SessionID,
		Name:             m.Name,
		Status:           status,
		State:            m.State,
		Harness:          m.Harness,
		Model:            m.Model,
		Project:          m.Context.Project,
		Purpose:          m.Context.Purpose,
		Tags:             m.Context.Tags,
		TmuxSession:      m.Tmux.SessionName,
		ClaudeUUID:       m.Claude.UUID,
		Workspace:        m.Workspace,
		WorkingDirectory: m.WorkingDirectory,
		Lifecycle:        m.Lifecycle,
		CreatedAt:        m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		PermissionMode:   m.PermissionMode,
		FinalOutput:      m.FinalOutput,
	}
	if !m.FinalOutputAt.IsZero() {
		d.FinalOutputAt = m.FinalOutputAt.UTC().Format("2006-01-02T15:04:05Z")
	}

	if m.ParentSessionID != nil {
		d.ParentSessionID = *m.ParentSessionID
	}

	if m.ContextUsage != nil {
		d.ContextUsage = &ContextUsage{
			TotalTokens:    m.ContextUsage.TotalTokens,
			UsedTokens:     m.ContextUsage.UsedTokens,
			PercentageUsed: m.ContextUsage.PercentageUsed,
		}
	}

	return d
}

func computeSessionStatus(m *manifest.Manifest, tmux interface{}) string {
	if m.Lifecycle == "archived" {
		return "archived"
	}

	if tmux == nil {
		return "unknown"
	}

	ti, ok := tmux.(interface {
		HasSession(name string) (bool, error)
	})
	if !ok {
		return "unknown"
	}

	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}

	has, err := ti.HasSession(tmuxName)
	if err != nil {
		return "unknown"
	}
	if !has {
		return "stopped"
	}
	// The tmux session exists — but existence alone is a false-green liveness
	// signal (ce-axsr): the session survives the harness exiting and the pane
	// falling back to a bare shell. When the backend can prove process
	// liveness, a dead-harness session reports "zombie" instead of "active".
	if checker, ok := tmux.(session.HarnessLivenessChecker); ok {
		if info, lerr := checker.HarnessLiveness(tmuxName); lerr == nil && info.SessionExists && !info.HarnessAlive {
			return "zombie"
		}
	}
	return "active"
}
