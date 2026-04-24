package ops

import (
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// DashboardRequest defines the input for the dashboard operation.
type DashboardRequest struct {
	// IncludeArchived shows archived sessions too.
	IncludeArchived bool `json:"include_archived,omitempty"`
}

// DashboardEntry is a single session row in the dashboard.
type DashboardEntry struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	TimeInState    string `json:"time_in_state"`
	PermissionMode string `json:"permission_mode"`
	InterruptCount int    `json:"interrupt_count"`
	Model          string `json:"model,omitempty"`
	Project        string `json:"project,omitempty"`
	TmuxAlive      bool   `json:"tmux_alive"`
}

// DashboardResult is the output of the Dashboard operation.
type DashboardResult struct {
	Operation string           `json:"operation"`
	Entries   []DashboardEntry `json:"entries"`
	Total     int              `json:"total"`
	Timestamp string           `json:"timestamp"`
}

// Dashboard returns a consolidated view of all active session states.
func Dashboard(ctx *OpContext, req *DashboardRequest) (*DashboardResult, error) {
	if req == nil {
		req = &DashboardRequest{}
	}

	// List sessions from storage
	listReq := &ListSessionsRequest{
		Status: "active",
		Limit:  1000,
	}
	if req.IncludeArchived {
		listReq.Status = "all"
	}

	listResult, err := ListSessions(ctx, listReq)
	if err != nil {
		return nil, err
	}

	// Get tmux session set for alive checks
	tmuxSet := make(map[string]bool)
	if ctx.Tmux != nil {
		if lister, ok := ctx.Tmux.(interface {
			ListSessions() ([]string, error)
		}); ok {
			if sessions, err := lister.ListSessions(); err == nil {
				for _, s := range sessions {
					tmuxSet[s] = true
				}
			}
		}
	}

	// Get full manifests for state details
	manifests, err := ctx.Storage.ListSessions(buildDashboardFilter(req))
	if err != nil {
		return nil, ErrStorageError("dashboard", err)
	}

	// Index manifests by name for lookup
	manifestByName := make(map[string]*manifest.Manifest, len(manifests))
	for _, m := range manifests {
		manifestByName[m.Name] = m
	}

	entries := make([]DashboardEntry, 0, len(listResult.Sessions))
	for _, s := range listResult.Sessions {
		m := manifestByName[s.Name]

		entry := DashboardEntry{
			Name:           s.Name,
			State:          effectiveState(m, tmuxSet),
			TimeInState:    timeInState(m),
			PermissionMode: permMode(m),
			InterruptCount: interrupt.GetInterruptCount(s.Name),
			Model:          model(m),
			Project:        s.Project,
			TmuxAlive:      isTmuxAlive(m, tmuxSet),
		}
		entries = append(entries, entry)
	}

	return &DashboardResult{
		Operation: "dashboard",
		Entries:   entries,
		Total:     len(entries),
		Timestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// effectiveState returns the best-known state, preferring manifest state
// but falling back to OFFLINE if tmux session is gone.
func effectiveState(m *manifest.Manifest, tmuxSet map[string]bool) string {
	if m == nil {
		return "UNKNOWN"
	}

	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}

	// If tmux session is gone, it's offline regardless of manifest state
	if !tmuxSet[tmuxName] {
		return manifest.StateOffline
	}

	if m.State != "" {
		return m.State
	}
	return "UNKNOWN"
}

func timeInState(m *manifest.Manifest) string {
	if m == nil || m.StateUpdatedAt.IsZero() {
		return "-"
	}
	return formatDuration(time.Since(m.StateUpdatedAt))
}

func permMode(m *manifest.Manifest) string {
	if m == nil || m.PermissionMode == "" {
		return "default"
	}
	return m.PermissionMode
}

func model(m *manifest.Manifest) string {
	if m == nil {
		return ""
	}
	if m.LastKnownModel != "" {
		return m.LastKnownModel
	}
	return m.Model
}

func isTmuxAlive(m *manifest.Manifest, tmuxSet map[string]bool) bool {
	if m == nil {
		return false
	}
	tmuxName := m.Tmux.SessionName
	if tmuxName == "" {
		tmuxName = m.Name
	}
	return tmuxSet[tmuxName]
}

func buildDashboardFilter(req *DashboardRequest) *dolt.SessionFilter {
	f := &dolt.SessionFilter{
		Limit: 1000,
	}
	if !req.IncludeArchived {
		f.ExcludeArchived = true
	}
	return f
}

// formatDuration formats a duration into a human-readable short string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours()) / 24
	return fmt.Sprintf("%dd", days)
}
