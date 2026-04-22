-- Migration 014: Add monitors column to agm_sessions
-- Stores a JSON array of session names that monitor this session's loop heartbeat.
-- Example: ["orchestrator-v2", "meta-orchestrator"]
ALTER TABLE agm_sessions ADD COLUMN monitors TEXT DEFAULT NULL;
