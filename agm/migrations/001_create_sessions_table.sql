-- ==============================================================================
-- AGM Migration 001: Initial Schema - Sessions Table
-- ==============================================================================
-- Version: 1
-- Name: create_sessions_table
-- Description: Create core AGM sessions table with basic metadata
-- Checksum: sha256:placeholder001
-- Tables Created: agm_sessions
-- Depends: None (initial migration)
-- Applied By: agm-2.0.0
-- Estimated Time: 25ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS agm_sessions (
  -- Primary key
  id VARCHAR(255) PRIMARY KEY
    COMMENT 'Unique session identifier (UUID or human-readable name)',

  -- Session metadata
  name VARCHAR(255) NOT NULL
    COMMENT 'Human-readable session name',

  tmux_session VARCHAR(255)
    COMMENT 'Associated tmux session name',

  -- Timestamps
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    COMMENT 'Session creation timestamp',

  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
    COMMENT 'Last update timestamp',

  -- Status tracking
  status ENUM('active', 'archived', 'deleted') DEFAULT 'active'
    COMMENT 'Session lifecycle status',

  -- Workspace isolation (for reference, actual isolation via separate databases)
  workspace VARCHAR(255) NOT NULL
    COMMENT 'Workspace identifier (oss, personal, etc.) - for reference only',

  -- Agent/model tracking
  model VARCHAR(100)
    COMMENT 'LLM model identifier (claude-sonnet-4-5, gemini-2.0-flash, etc.)',

  agent_mode VARCHAR(50)
    COMMENT 'Agent operating mode (implementer, researcher, reviewer, etc.)',

  -- Flexible metadata storage
  metadata JSON
    COMMENT 'Additional session metadata (custom fields, configuration, etc.)',

  -- Indexes for common queries
  INDEX idx_created_at (created_at)
    COMMENT 'Query sessions by creation date',

  INDEX idx_updated_at (updated_at)
    COMMENT 'Query sessions by last activity',

  INDEX idx_status (status)
    COMMENT 'Filter by session status',

  INDEX idx_workspace (workspace)
    COMMENT 'Filter by workspace (even though isolated)',

  INDEX idx_name (name)
    COMMENT 'Search sessions by name',

  INDEX idx_tmux_session (tmux_session)
    COMMENT 'Lookup by tmux session name'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='AGM sessions - workspace-scoped AI conversation sessions (Claude/Gemini)';

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the core agm_sessions table with:
-- ✅ Session identification (id, name, tmux_session)
-- ✅ Lifecycle tracking (status, timestamps)
-- ✅ Workspace isolation (workspace field)
-- ✅ Agent metadata (model, agent_mode)
-- ✅ Flexible metadata storage (JSON field)
-- ✅ Performance indexes (created_at, status, workspace, name)
-- ==============================================================================
