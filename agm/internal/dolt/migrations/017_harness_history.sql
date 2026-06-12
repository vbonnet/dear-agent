-- AGM Migration 017: Add harness_history table for ManifestV3 harness-switch tracking
-- Records every harness transition for a session (from_harness -> to_harness + timestamp).
-- Part of ManifestV3 persistence: Harness+Model fields already exist in agm_sessions
-- (added via migration 009); this adds the companion history table.

CREATE TABLE IF NOT EXISTS agm_harness_history (
  id          BIGINT AUTO_INCREMENT PRIMARY KEY,
  session_id  VARCHAR(255) NOT NULL,
  switched_at TIMESTAMP    NOT NULL,
  from_harness VARCHAR(100) NOT NULL,
  to_harness   VARCHAR(100) NOT NULL,

  INDEX idx_hh_session_id (session_id),
  INDEX idx_hh_switched_at (switched_at)
)
