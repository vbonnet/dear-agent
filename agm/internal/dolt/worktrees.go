package dolt

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// WorktreeRecord represents a tracked worktree in the database
type WorktreeRecord struct {
	ID           int
	SessionName  string
	RepoPath     string
	WorktreePath string
	Branch       string
	CreatedAt    time.Time
	RemovedAt    *time.Time
	Status       string // "active", "removed", "orphaned"
}

// TrackWorktree records a newly created worktree
func (a *Adapter) TrackWorktree(ctx context.Context, sessionName, repoPath, worktreePath, branch string) error {
	query := `
		INSERT INTO agm_worktrees (session_name, repo_path, worktree_path, branch, status)
		VALUES (?, ?, ?, ?, 'active')
		ON DUPLICATE KEY UPDATE
			session_name = VALUES(session_name),
			branch = VALUES(branch),
			status = 'active',
			removed_at = NULL
	`
	_, err := a.conn.ExecContext(ctx, query, sessionName, repoPath, worktreePath, branch)
	if err != nil {
		return fmt.Errorf("failed to track worktree: %w", err)
	}
	return nil
}

// UntrackWorktree marks a worktree as removed
func (a *Adapter) UntrackWorktree(ctx context.Context, worktreePath string) error {
	query := `
		UPDATE agm_worktrees
		SET status = 'removed', removed_at = NOW()
		WHERE worktree_path = ? AND status = 'active'
	`
	_, err := a.conn.ExecContext(ctx, query, worktreePath)
	if err != nil {
		return fmt.Errorf("failed to untrack worktree: %w", err)
	}
	return nil
}

// ListActiveWorktrees returns all worktrees that are still active
func (a *Adapter) ListActiveWorktrees(ctx context.Context) ([]WorktreeRecord, error) {
	query := `
		SELECT id, session_name, repo_path, worktree_path, branch, created_at, status
		FROM agm_worktrees
		WHERE status = 'active'
		ORDER BY created_at DESC
	`
	return a.queryWorktrees(ctx, query)
}

// ListWorktreesBySession returns all active worktrees for a given session
func (a *Adapter) ListWorktreesBySession(ctx context.Context, sessionName string) ([]WorktreeRecord, error) {
	query := `
		SELECT id, session_name, repo_path, worktree_path, branch, created_at, status
		FROM agm_worktrees
		WHERE session_name = ? AND status = 'active'
		ORDER BY created_at DESC
	`
	return a.queryWorktrees(ctx, query, sessionName)
}

// MarkOrphaned marks worktrees as orphaned (session ended but worktree still exists)
func (a *Adapter) MarkOrphaned(ctx context.Context, sessionName string) error {
	query := `
		UPDATE agm_worktrees
		SET status = 'orphaned'
		WHERE session_name = ? AND status = 'active'
	`
	_, err := a.conn.ExecContext(ctx, query, sessionName)
	if err != nil {
		return fmt.Errorf("failed to mark worktrees as orphaned: %w", err)
	}
	return nil
}

// ListOrphanedWorktrees returns all orphaned worktrees
func (a *Adapter) ListOrphanedWorktrees(ctx context.Context) ([]WorktreeRecord, error) {
	query := `
		SELECT id, session_name, repo_path, worktree_path, branch, created_at, status
		FROM agm_worktrees
		WHERE status = 'orphaned'
		ORDER BY created_at ASC
	`
	return a.queryWorktrees(ctx, query)
}

// queryWorktrees is a helper that executes a query and scans WorktreeRecord rows
func (a *Adapter) queryWorktrees(ctx context.Context, query string, args ...interface{}) ([]WorktreeRecord, error) {
	rows, err := a.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query worktrees: %w", err)
	}
	defer rows.Close()

	var records []WorktreeRecord
	for rows.Next() {
		var r WorktreeRecord
		if err := rows.Scan(&r.ID, &r.SessionName, &r.RepoPath, &r.WorktreePath, &r.Branch, &r.CreatedAt, &r.Status); err != nil {
			return nil, fmt.Errorf("failed to scan worktree row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating worktree rows: %w", err)
	}
	return records, nil
}

// DeleteWorktreeRecord permanently removes a worktree record from the database
func (a *Adapter) DeleteWorktreeRecord(ctx context.Context, id int) error {
	query := `DELETE FROM agm_worktrees WHERE id = ?`
	result, err := a.conn.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete worktree record: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
