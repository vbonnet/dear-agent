-- Migration 008: Add permission mode tracking
-- Adds permission_mode columns to agm_sessions table for Claude Code mode persistence

ALTER TABLE agm_sessions
  ADD COLUMN permission_mode VARCHAR(20) DEFAULT 'default'
    COMMENT 'Claude Code permission mode: default, plan, ask, allow',
  ADD COLUMN permission_mode_updated_at TIMESTAMP NULL
    COMMENT 'When mode was last changed',
  ADD COLUMN permission_mode_source VARCHAR(50) DEFAULT 'hook'
    COMMENT 'How mode was detected: hook, manual, resume',
  ADD INDEX idx_permission_mode (permission_mode)
