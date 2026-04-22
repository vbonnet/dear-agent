-- AGM Migration 001: Initial Schema
-- Description: Create core AGM tables (sessions and messages)
-- Author: AGM Team
-- Date: 2026-02-19
-- Depends: None (initial migration)

CREATE TABLE IF NOT EXISTS agm_sessions (
  id VARCHAR(255) PRIMARY KEY,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  status ENUM('active', 'archived', 'deleted') DEFAULT 'active',
  workspace VARCHAR(255) NOT NULL,
  model VARCHAR(100),
  metadata JSON,

  INDEX idx_created_at (created_at),
  INDEX idx_status (status),
  INDEX idx_workspace (workspace)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='AGM sessions - workspace-scoped conversation sessions';

CREATE TABLE IF NOT EXISTS agm_messages (
  id VARCHAR(255) PRIMARY KEY,
  session_id VARCHAR(255) NOT NULL,
  role ENUM('user', 'assistant', 'system') NOT NULL,
  content TEXT NOT NULL,
  timestamp BIGINT NOT NULL,
  sequence_number INT NOT NULL,
  metadata JSON,

  FOREIGN KEY (session_id) REFERENCES agm_sessions(id) ON DELETE CASCADE,
  INDEX idx_session_id (session_id),
  INDEX idx_timestamp (timestamp),
  INDEX idx_sequence (session_id, sequence_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='AGM messages - individual messages within sessions';
