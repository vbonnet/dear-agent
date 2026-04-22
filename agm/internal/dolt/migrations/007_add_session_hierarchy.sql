-- AGM Migration 007: Add Session Hierarchy Support
-- Adds parent_session_id column to agm_sessions table for parent-child relationships
-- Author: AGM Team
-- Date: 2026-03-18
-- Updated: 2026-03-18 - Initial creation

-- Add parent_session_id column to support session hierarchy
-- Parent session ID for execution sessions spawned from planning sessions
-- Also adds foreign key constraint and index in a single ALTER TABLE statement
ALTER TABLE agm_sessions
  ADD COLUMN parent_session_id VARCHAR(255) NULL,
  ADD CONSTRAINT fk_parent_session
    FOREIGN KEY (parent_session_id)
    REFERENCES agm_sessions(id)
    ON DELETE SET NULL,
  ADD INDEX idx_parent_session_id (parent_session_id)
