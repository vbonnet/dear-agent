-- AGM Migration 002: Add Tool Calls Tracking
-- Track tool calls and results for debugging and analysis
-- Author: AGM Team
-- Date: 2026-02-19
-- Updated: 2026-03-07 - Made Dolt-compatible (removed MySQL-specific syntax)

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

  INDEX idx_message_id (message_id),
  INDEX idx_session_id (session_id),
  INDEX idx_tool_name (tool_name),
  INDEX idx_timestamp (timestamp)
);
