package ops

import (
	"sort"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

// LastSessionRequest defines input for finding the most recently active session.
type LastSessionRequest struct {
	// NameOnly returns just the session name (for scripting).
	NameOnly bool `json:"name_only,omitempty"`
}

// LastSessionResult is the output of LastSession.
type LastSessionResult struct {
	Operation string         `json:"operation"`
	Session   SessionSummary `json:"session"`
}

// LastSession returns the most recently active session by UpdatedAt timestamp.
func LastSession(ctx *OpContext, _ *LastSessionRequest) (*LastSessionResult, error) {
	// List all non-archived sessions
	filter := &dolt.SessionFilter{
		ExcludeArchived: true,
	}

	manifests, err := ctx.Storage.ListSessions(filter)
	if err != nil {
		return nil, ErrStorageError("last_session", err)
	}

	if len(manifests) == 0 {
		return nil, ErrSessionNotFound("(most recent)")
	}

	// Sort by UpdatedAt descending to find the most recent
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].UpdatedAt.After(manifests[j].UpdatedAt)
	})

	most := manifests[0]

	// Compute status and attachment
	statuses := make(map[string]string)
	attached := make(map[string]bool)
	if ctx.Tmux != nil {
		statuses, attached = computeStatusesWithAttachment(manifests[:1], ctx.Tmux)
	}

	return &LastSessionResult{
		Operation: "last_session",
		Session:   toSessionSummary(most, statuses, attached),
	}, nil
}
