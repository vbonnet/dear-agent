-- ==============================================================================
-- Wayfinder Migration 003: Phase Validations Table
-- ==============================================================================
-- Version: 3
-- Name: create_phase_validations_table
-- Description: Create wayfinder_phase_validations table for validation results
-- Checksum: sha256:placeholder003
-- Tables Created: wayfinder_phase_validations
-- Depends: Migration 002 (wayfinder_phases table)
-- Applied By: wayfinder-0.1.0
-- Estimated Time: 40ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS wayfinder_phase_validations (
  -- Primary key
  validation_id VARCHAR(255) PRIMARY KEY
    COMMENT 'Unique validation identifier (UUID)',

  -- Foreign key to phases
  phase_id VARCHAR(255) NOT NULL
    COMMENT 'Phase being validated',

  -- Validation metadata
  validator_name VARCHAR(100) NOT NULL
    COMMENT 'Name of validation gate (deliverable, compilation, tests, d2, multi-persona, etc.)',

  passed BOOLEAN NOT NULL
    COMMENT 'Whether validation passed (TRUE) or failed (FALSE)',

  message TEXT
    COMMENT 'Validation result message (error details, success message, etc.)',

  -- Timing
  timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    COMMENT 'Validation execution timestamp',

  -- Flexible details storage
  details JSON
    COMMENT 'Additional validation details (test results, file paths, error codes, etc.)',

  -- Foreign key constraint
  CONSTRAINT fk_validation_phase
    FOREIGN KEY (phase_id)
    REFERENCES wayfinder_phases(phase_id)
    ON DELETE CASCADE
    ON UPDATE CASCADE
    COMMENT 'Cascade delete validations when phase is deleted',

  -- Indexes for common queries
  INDEX idx_phase_id (phase_id)
    COMMENT 'Query validations by phase',

  INDEX idx_validator_name (validator_name)
    COMMENT 'Query by validator type',

  INDEX idx_passed (passed)
    COMMENT 'Filter by pass/fail status',

  INDEX idx_timestamp (timestamp)
    COMMENT 'Query validations by execution time',

  -- Composite index for common queries
  INDEX idx_phase_validator (phase_id, validator_name)
    COMMENT 'Query specific validator results for a phase',

  INDEX idx_phase_passed (phase_id, passed)
    COMMENT 'Query pass/fail results for a phase'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='Wayfinder phase validations - validation gate results for SDLC phases';

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the wayfinder_phase_validations table with:
-- ✅ Validation identification (validation_id, validator_name)
-- ✅ Phase relationship (phase_id with foreign key)
-- ✅ Result tracking (passed, message, timestamp)
-- ✅ Flexible details storage (JSON field for test results, errors, etc.)
-- ✅ Referential integrity (CASCADE delete on phase removal)
-- ✅ Performance indexes (phase_id, validator_name, passed, timestamp)
-- ✅ Composite indexes for common query patterns
-- ==============================================================================
