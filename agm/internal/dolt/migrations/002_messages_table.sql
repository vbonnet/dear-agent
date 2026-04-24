-- AGM Migration 002: Messages Table
-- Creates agm_messages table
-- Author: AGM Team
-- Date: 2026-02-19
-- Updated: 2026-03-07 - Split from 001 for Dolt compatibility

CREATE TABLE IF NOT EXISTS agm_messages (
  id VARCHAR(255) PRIMARY KEY,
  session_id VARCHAR(255) NOT NULL,
  role VARCHAR(20) NOT NULL,
  content TEXT NOT NULL,
  timestamp BIGINT NOT NULL,
  sequence_number INT NOT NULL,
  agent VARCHAR(100),
  input_tokens INT DEFAULT 0,
  output_tokens INT DEFAULT 0,
  metadata JSON,

  INDEX idx_session_id (session_id),
  INDEX idx_timestamp (timestamp),
  INDEX idx_sequence (session_id, sequence_number)
)
