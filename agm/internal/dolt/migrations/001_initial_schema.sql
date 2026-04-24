-- AGM Migration 001: Initial Schema - Sessions Table
-- Creates agm_sessions table
-- Author: AGM Team
-- Date: 2026-02-19
-- Updated: 2026-03-07 - Made Dolt-compatible, split into separate files

CREATE TABLE IF NOT EXISTS agm_sessions (
  id VARCHAR(255) PRIMARY KEY,
  created_at TIMESTAMP,
  updated_at TIMESTAMP,
  status VARCHAR(20),
  workspace VARCHAR(255) NOT NULL,
  model VARCHAR(100),
  name VARCHAR(255),
  agent VARCHAR(100),
  context_project TEXT,
  context_purpose TEXT,
  context_tags JSON,
  context_notes TEXT,
  claude_uuid VARCHAR(255),
  tmux_session_name VARCHAR(255),
  metadata JSON,

  INDEX idx_created_at (created_at),
  INDEX idx_status (status),
  INDEX idx_workspace (workspace),
  INDEX idx_updated_at (updated_at)
)
