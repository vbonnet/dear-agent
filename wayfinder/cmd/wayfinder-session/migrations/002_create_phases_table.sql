-- ==============================================================================
-- Wayfinder Migration 002: Phases Table
-- ==============================================================================
-- Version: 2
-- Name: create_phases_table
-- Description: Create wayfinder_phases table for phase tracking
-- Checksum: sha256:placeholder002
-- Tables Created: wayfinder_phases
-- Depends: Migration 001 (wayfinder_projects table)
-- Applied By: wayfinder-0.1.0
-- Estimated Time: 35ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS wayfinder_phases (
  -- Primary key
  phase_id VARCHAR(255) PRIMARY KEY
    COMMENT 'Unique phase identifier (UUID)',

  -- Foreign key to projects
  project_id VARCHAR(255) NOT NULL
    COMMENT 'Parent project identifier',

  -- Phase metadata
  phase_name VARCHAR(100) NOT NULL
    COMMENT 'Phase name (S1-Requirements, S2-Research, S3-Architecture, etc.)',

  status ENUM('pending', 'in_progress', 'completed', 'failed', 'skipped') DEFAULT 'pending'
    COMMENT 'Phase lifecycle status',

  -- Timing information
  start_time TIMESTAMP NULL
    COMMENT 'Phase start timestamp (NULL if not started)',

  end_time TIMESTAMP NULL
    COMMENT 'Phase completion timestamp (NULL if not completed)',

  -- Flexible metadata storage
  metadata JSON
    COMMENT 'Phase-specific metadata (deliverables, notes, validation results, etc.)',

  -- Foreign key constraint
  CONSTRAINT fk_phase_project
    FOREIGN KEY (project_id)
    REFERENCES wayfinder_projects(project_id)
    ON DELETE CASCADE
    ON UPDATE CASCADE
    COMMENT 'Cascade delete phases when project is deleted',

  -- Indexes for common queries
  INDEX idx_project_id (project_id)
    COMMENT 'Query phases by project',

  INDEX idx_status (status)
    COMMENT 'Filter by phase status',

  INDEX idx_phase_name (phase_name)
    COMMENT 'Query by phase name',

  INDEX idx_start_time (start_time)
    COMMENT 'Query phases by start time',

  INDEX idx_end_time (end_time)
    COMMENT 'Query phases by completion time',

  -- Unique constraint: one record per project per phase name
  UNIQUE INDEX idx_project_phase (project_id, phase_name)
    COMMENT 'Ensure each phase appears only once per project'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='Wayfinder phases - SDLC phase tracking for projects';

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the wayfinder_phases table with:
-- ✅ Phase identification (phase_id, phase_name)
-- ✅ Project relationship (project_id with foreign key)
-- ✅ Lifecycle tracking (status, start_time, end_time)
-- ✅ Flexible metadata storage (JSON field)
-- ✅ Referential integrity (CASCADE delete on project removal)
-- ✅ Uniqueness constraint (one record per project per phase)
-- ✅ Performance indexes (project_id, status, phase_name, timestamps)
-- ==============================================================================
