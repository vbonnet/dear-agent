-- AGM Migration 010: Add is_test column to agm_sessions
-- Marks sessions created with --test flag so they can be filtered from production list
-- Author: AGM Team
-- Date: 2026-03-24

ALTER TABLE agm_sessions
  ADD COLUMN is_test BOOLEAN NOT NULL DEFAULT FALSE
    COMMENT 'Whether session was created with --test flag',
  ADD INDEX idx_is_test (is_test)
