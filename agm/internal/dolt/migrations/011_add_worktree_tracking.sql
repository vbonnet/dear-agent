-- AGM Migration 011: Add worktree tracking table
-- Tracks git worktrees created during sessions for lifecycle management
-- Author: AGM Team
-- Date: 2026-03-26

CREATE TABLE IF NOT EXISTS agm_worktrees (
  id INT AUTO_INCREMENT PRIMARY KEY,
  session_name VARCHAR(255) NOT NULL
    COMMENT 'AGM session that created this worktree',
  repo_path VARCHAR(1024) NOT NULL
    COMMENT 'Absolute path to the parent git repository',
  worktree_path VARCHAR(1024) NOT NULL
    COMMENT 'Absolute path to the worktree directory',
  branch VARCHAR(255) NOT NULL DEFAULT ''
    COMMENT 'Branch checked out in the worktree',
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  removed_at TIMESTAMP NULL DEFAULT NULL
    COMMENT 'When the worktree was removed (NULL if still active)',
  status VARCHAR(20) NOT NULL DEFAULT 'active'
    COMMENT 'active, removed, or orphaned',
  INDEX idx_session_name (session_name),
  INDEX idx_status (status),
  INDEX idx_repo_path (repo_path(255)),
  UNIQUE KEY uk_worktree_path (worktree_path(512))
)
