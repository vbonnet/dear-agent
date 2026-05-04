package cleanup

import (
	"context"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

// DoltWorktreeStore adapts dolt.Adapter to the WorktreeStore interface.
type DoltWorktreeStore struct {
	Adapter *dolt.Adapter
}

// ListWorktreesBySession returns all worktrees recorded for a session.
func (d *DoltWorktreeStore) ListWorktreesBySession(ctx context.Context, sessionName string) ([]WorktreeRecord, error) {
	records, err := d.Adapter.ListWorktreesBySession(ctx, sessionName)
	if err != nil {
		return nil, err
	}
	result := make([]WorktreeRecord, len(records))
	for i, r := range records {
		result[i] = WorktreeRecord{
			WorktreePath: r.WorktreePath,
			RepoPath:     r.RepoPath,
			Branch:       r.Branch,
			SessionName:  r.SessionName,
		}
	}
	return result, nil
}

// UntrackWorktree removes the tracking row for the given worktree path.
func (d *DoltWorktreeStore) UntrackWorktree(ctx context.Context, worktreePath string) error {
	return d.Adapter.UntrackWorktree(ctx, worktreePath)
}
