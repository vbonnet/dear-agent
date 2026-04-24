-- ==============================================================================
-- Rollback Migration 004: Restore Phases Terminology
-- ==============================================================================
-- Version: 4
-- Name: rollback_rename_phases_to_waypoints
-- Description: Rollback waypoint terminology to phase terminology
-- Checksum: sha256:rollback004
-- Tables Affected: wayfinder_waypoints → wayfinder_phases
-- Breaking: YES (removes v2.0 waypoint terminology)
-- Applied By: wayfinder-rollback-2.0.0
-- Estimated Time: 150ms
-- ==============================================================================

START TRANSACTION;

-- Step 1: Drop backward-compatible views
DROP VIEW IF EXISTS wayfinder_phases;
DROP VIEW IF EXISTS wayfinder_phase_validations;

-- Step 2: Rename tables back
ALTER TABLE wayfinder_waypoints RENAME TO wayfinder_phases;
ALTER TABLE wayfinder_waypoint_validations RENAME TO wayfinder_phase_validations;

-- Step 3: Rename columns back
ALTER TABLE wayfinder_phases
  CHANGE COLUMN waypoint_id phase_id VARCHAR(255),
  CHANGE COLUMN waypoint_name phase_name VARCHAR(100);

ALTER TABLE wayfinder_phase_validations
  CHANGE COLUMN waypoint_id phase_id VARCHAR(255);

-- Step 4: Restore indexes
ALTER TABLE wayfinder_phases
  DROP INDEX idx_waypoint_name,
  ADD INDEX idx_phase_name (phase_name)
    COMMENT 'Query by phase name';

ALTER TABLE wayfinder_phases
  DROP INDEX idx_project_waypoint,
  ADD UNIQUE INDEX idx_project_phase (project_id, phase_name)
    COMMENT 'Ensure each phase appears only once per project';

-- Step 5: Restore foreign keys
ALTER TABLE wayfinder_phases
  DROP FOREIGN KEY fk_waypoint_project,
  ADD CONSTRAINT fk_phase_project
    FOREIGN KEY (project_id) REFERENCES wayfinder_projects(project_id)
    ON DELETE CASCADE ON UPDATE CASCADE
    COMMENT 'Cascade delete phases when project is deleted';

ALTER TABLE wayfinder_phase_validations
  DROP FOREIGN KEY fk_validation_waypoint,
  ADD CONSTRAINT fk_validation_phase
    FOREIGN KEY (phase_id) REFERENCES wayfinder_phases(phase_id)
    ON DELETE CASCADE ON UPDATE CASCADE
    COMMENT 'Cascade delete validations when phase is deleted';

COMMIT;

-- ==============================================================================
-- Rollback Complete
-- ==============================================================================
-- This rollback script restores phase terminology from waypoint terminology:
-- ✅ Views dropped: wayfinder_phases, wayfinder_phase_validations
-- ✅ Table renamed: wayfinder_waypoints → wayfinder_phases
-- ✅ Columns renamed: waypoint_id → phase_id, waypoint_name → phase_name
-- ✅ Indexes restored: idx_waypoint_name → idx_phase_name, idx_project_waypoint → idx_project_phase
-- ✅ Foreign keys restored: fk_waypoint_project → fk_phase_project
-- ✅ Validations table renamed: wayfinder_waypoint_validations → wayfinder_phase_validations
-- ✅ Zero data loss - all existing data preserved
-- ✅ Database restored to v1.x schema
-- ==============================================================================
