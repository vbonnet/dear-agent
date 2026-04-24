-- Migration 012: Add frecency scoring columns to agm_sessions
-- Enables zoxide-style frecency ranking (frequency * recency decay)
ALTER TABLE agm_sessions
    ADD COLUMN access_count INT NOT NULL DEFAULT 0,
    ADD COLUMN last_accessed_at TIMESTAMP NULL,
    ADD INDEX idx_access_count (access_count),
    ADD INDEX idx_last_accessed_at (last_accessed_at)
