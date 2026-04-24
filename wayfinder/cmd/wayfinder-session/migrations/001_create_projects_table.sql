-- ==============================================================================
-- Wayfinder Migration 001: Projects Table
-- ==============================================================================
-- Version: 1
-- Name: create_projects_table
-- Description: Create wayfinder_projects table with workspace isolation
-- Checksum: sha256:placeholder001
-- Tables Created: wayfinder_projects
-- Depends: None (initial migration)
-- Applied By: wayfinder-0.1.0
-- Estimated Time: 30ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS wayfinder_projects (
  -- Primary key
  project_id VARCHAR(255) PRIMARY KEY
    COMMENT 'Unique project identifier (directory name or UUID)',

  -- Workspace isolation
  workspace VARCHAR(255) NOT NULL
    COMMENT 'Workspace identifier (oss, acme, etc.) - for reference only',

  -- Project metadata
  name VARCHAR(255) NOT NULL
    COMMENT 'Human-readable project name',

  status ENUM('active', 'archived', 'completed', 'failed') DEFAULT 'active'
    COMMENT 'Project lifecycle status',

  current_phase VARCHAR(100)
    COMMENT 'Current phase name (S1-Requirements, S2-Research, etc.)',

  -- Timestamps
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    COMMENT 'Project creation timestamp',

  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
    COMMENT 'Last update timestamp',

  -- Flexible metadata storage
  metadata JSON
    COMMENT 'Additional project metadata (custom fields, configuration, etc.)',

  -- Indexes for common queries
  INDEX idx_workspace (workspace)
    COMMENT 'Filter by workspace (even though isolated at DB level)',

  INDEX idx_status (status)
    COMMENT 'Filter by project status',

  INDEX idx_created_at (created_at)
    COMMENT 'Query projects by creation date',

  INDEX idx_updated_at (updated_at)
    COMMENT 'Query projects by last activity',

  INDEX idx_name (name)
    COMMENT 'Search projects by name',

  INDEX idx_current_phase (current_phase)
    COMMENT 'Filter by current phase'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='Wayfinder projects - workspace-scoped SDLC project tracking';

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the core wayfinder_projects table with:
-- ✅ Project identification (project_id, name)
-- ✅ Workspace isolation (workspace field)
-- ✅ Lifecycle tracking (status, current_phase, timestamps)
-- ✅ Flexible metadata storage (JSON field)
-- ✅ Performance indexes (workspace, status, created_at, name)
-- ==============================================================================
