-- AGM Migration 009: Rename agent column to harness in agm_sessions
-- Author: AGM Team
-- Date: 2026-03-22

ALTER TABLE agm_sessions CHANGE COLUMN agent harness VARCHAR(100)
