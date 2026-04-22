-- ==============================================================================
-- AGM Migration 004: Session Tags Table
-- ==============================================================================
-- Version: 4
-- Name: create_session_tags_table
-- Description: Create agm_session_tags table for session categorization
-- Checksum: sha256:placeholder004
-- Tables Created: agm_session_tags
-- Depends: Migration 001 (references agm_sessions)
-- Applied By: agm-2.0.0
-- Estimated Time: 25ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS agm_session_tags (
  -- Primary key
  id INT AUTO_INCREMENT PRIMARY KEY
    COMMENT 'Auto-incrementing tag assignment ID',

  -- Foreign key
  session_id VARCHAR(255) NOT NULL
    COMMENT 'Associated session ID',

  -- Tag metadata
  tag VARCHAR(255) NOT NULL
    COMMENT 'Tag value (e.g., "bug-fix", "research", "implementation")',

  -- Tracking
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    COMMENT 'When tag was added',

  created_by VARCHAR(255)
    COMMENT 'User or component that added tag (for audit)',

  -- Foreign key constraint
  FOREIGN KEY (session_id)
    REFERENCES agm_sessions(id)
    ON DELETE CASCADE
    COMMENT 'Cascade delete tags when session deleted',

  -- Indexes for common queries
  INDEX idx_session_id (session_id)
    COMMENT 'Query tags by session',

  INDEX idx_tag (tag)
    COMMENT 'Filter sessions by tag',

  INDEX idx_session_tag (session_id, tag)
    COMMENT 'Composite index for session + tag lookup',

  INDEX idx_tag_created (tag, created_at)
    COMMENT 'Composite index for tag-based session listing by date',

  -- Uniqueness constraint
  UNIQUE KEY unique_session_tag (session_id, tag)
    COMMENT 'Prevent duplicate tags on same session'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='AGM session tags - categorization and filtering of sessions';

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the agm_session_tags table with:
-- ✅ Tag assignment (id, session_id, tag)
-- ✅ Audit tracking (created_at, created_by)
-- ✅ Cascade deletion (when session deleted)
-- ✅ Duplicate prevention (unique constraint on session + tag)
-- ✅ Performance indexes (session_id, tag, composite)
-- ==============================================================================
