-- AGM Migration 002: Add Tool Calls Tracking
-- Description: Track tool calls and results for debugging and analysis
-- Author: AGM Team
-- Date: 2026-02-19
-- Depends: Migration 001 (references agm_messages)

CREATE TABLE IF NOT EXISTS agm_tool_calls (
  id VARCHAR(255) PRIMARY KEY,
  message_id VARCHAR(255) NOT NULL,
  session_id VARCHAR(255) NOT NULL,
  tool_name VARCHAR(255) NOT NULL,
  arguments JSON,
  result JSON,
  error TEXT,
  timestamp BIGINT NOT NULL,
  execution_time_ms INT,

  FOREIGN KEY (message_id) REFERENCES agm_messages(id) ON DELETE CASCADE,
  FOREIGN KEY (session_id) REFERENCES agm_sessions(id) ON DELETE CASCADE,
  INDEX idx_message_id (message_id),
  INDEX idx_session_id (session_id),
  INDEX idx_tool_name (tool_name),
  INDEX idx_timestamp (timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='AGM tool calls - tracking tool usage and results';
