-- ==============================================================================
-- Wayfinder Migration 004: Rename Phases to Waypoints
-- ==============================================================================
-- Version: 4
-- Name: rename_phases_to_waypoints
-- Description: Rename phase terminology to waypoint terminology
-- Checksum: sha256:placeholder004
-- Tables Affected: wayfinder_phases → wayfinder_waypoints
-- Breaking: YES (backward-compatible views provided)
-- Depends: Migration 003 (wayfinder_phase_validations table)
-- Rollback: See ROLLBACK_004.sql
-- Applied By: wayfinder-2.0.0
-- Estimated Time: 150ms
-- ==============================================================================

START TRANSACTION;

-- Step 1: Rename main table
ALTER TABLE wayfinder_phases RENAME TO wayfinder_waypoints;

-- Step 2: Rename columns
ALTER TABLE wayfinder_waypoints
  CHANGE COLUMN phase_id waypoint_id VARCHAR(255),
  CHANGE COLUMN phase_name waypoint_name VARCHAR(100);

-- Step 3: Rename indexes
ALTER TABLE wayfinder_waypoints
  DROP INDEX idx_phase_name,
  ADD INDEX idx_waypoint_name (waypoint_name)
    COMMENT 'Query by waypoint name';

ALTER TABLE wayfinder_waypoints
  DROP INDEX idx_project_phase,
  ADD UNIQUE INDEX idx_project_waypoint (project_id, waypoint_name)
    COMMENT 'Ensure each waypoint appears only once per project';

-- Step 4: Rename foreign key constraint
ALTER TABLE wayfinder_waypoints
  DROP FOREIGN KEY fk_phase_project,
  ADD CONSTRAINT fk_waypoint_project
    FOREIGN KEY (project_id) REFERENCES wayfinder_projects(project_id)
    ON DELETE CASCADE ON UPDATE CASCADE
    COMMENT 'Cascade delete waypoints when project is deleted';

-- Step 5: Update phase_validations table
ALTER TABLE wayfinder_phase_validations RENAME TO wayfinder_waypoint_validations;

ALTER TABLE wayfinder_waypoint_validations
  CHANGE COLUMN phase_id waypoint_id VARCHAR(255);

ALTER TABLE wayfinder_waypoint_validations
  DROP FOREIGN KEY fk_validation_phase,
  ADD CONSTRAINT fk_validation_waypoint
    FOREIGN KEY (waypoint_id) REFERENCES wayfinder_waypoints(waypoint_id)
    ON DELETE CASCADE ON UPDATE CASCADE
    COMMENT 'Cascade delete validations when waypoint is deleted';

-- Step 6: Create backward-compatible views (v1.x compatibility)
CREATE VIEW wayfinder_phases AS
SELECT
  waypoint_id AS phase_id,
  project_id,
  waypoint_name AS phase_name,
  status,
  start_time,
  end_time,
  metadata
FROM wayfinder_waypoints
COMMENT='Backward-compatible view for v1.x clients - maps waypoints to phases';

CREATE VIEW wayfinder_phase_validations AS
SELECT
  validation_id,
  waypoint_id AS phase_id,
  validator_name,
  status,
  validation_time,
  metadata
FROM wayfinder_waypoint_validations
COMMENT='Backward-compatible view for v1.x clients - maps waypoint validations to phase validations';

COMMIT;

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration renames phase terminology to waypoint terminology:
-- ✅ Table renamed: wayfinder_phases → wayfinder_waypoints
-- ✅ Columns renamed: phase_id → waypoint_id, phase_name → waypoint_name
-- ✅ Indexes renamed: idx_phase_name → idx_waypoint_name, idx_project_phase → idx_project_waypoint
-- ✅ Foreign keys renamed: fk_phase_project → fk_waypoint_project
-- ✅ Validations table renamed: wayfinder_phase_validations → wayfinder_waypoint_validations
-- ✅ Backward-compatible views created for v1.x clients
-- ✅ Zero data loss - all existing data preserved
-- ==============================================================================
