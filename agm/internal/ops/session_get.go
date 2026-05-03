package ops

import (
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// GetSessionRequest defines the input for getting a single session.
type GetSessionRequest struct {
	// Identifier is a session ID, name, or UUID prefix.
	Identifier string `json:"identifier"`
}

// SessionDetail is the full session output with all fields.
type SessionDetail struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Status           string        `json:"status"`
	State            string        `json:"state"`
	Harness          string        `json:"harness"`
	Model            string        `json:"model,omitempty"`
	Project          string        `json:"project"`
	Purpose          string        `json:"purpose,omitempty"`
	Tags             []string      `json:"tags,omitempty"`
	TmuxSession      string        `json:"tmux_session"`
	ClaudeUUID       string        `json:"claude_uuid,omitempty"`
	ParentSessionID  string        `json:"parent_session_id,omitempty"`
	Workspace        string        `json:"workspace"`
	WorkingDirectory string        `json:"working_directory,omitempty"`
	Lifecycle        string        `json:"lifecycle"`
	CreatedAt        string        `json:"created_at"`
	UpdatedAt        string        `json:"updated_at"`
	ContextUsage     *ContextUsage `json:"context_usage,omitempty"`
	PermissionMode   string        `json:"permission_mode,omitempty"`
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

	// Try exact ID match first
	m, err := ctx.Storage.GetSession(req.Identifier)
	if err != nil {
		// Try name-based lookup via list
		m, err = findByName(ctx, req.Identifier)
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

	return &GetSessionResult{
		Operation: "get_session",
		Session:   detail,
	}, nil
}

// findByName searches for a session by name or tmux session name.
func findByName(ctx *OpContext, name string) (*manifest.Manifest, error) {
	manifests, err := ctx.Storage.ListSessions(nil)
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
	if has {
		return "active"
	}
	return "stopped"
}
