-- ==============================================================================
-- AGM Migration 003: Tool Calls Table
-- ==============================================================================
-- Version: 3
-- Name: create_tool_calls_table
-- Description: Create agm_tool_calls table for tracking tool usage and results
-- Checksum: sha256:placeholder003
-- Tables Created: agm_tool_calls
-- Depends: Migrations 001, 002 (references agm_sessions, agm_messages)
-- Applied By: agm-2.0.0
-- Estimated Time: 35ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS agm_tool_calls (
  -- Primary key
  id VARCHAR(255) PRIMARY KEY
    COMMENT 'Unique tool call identifier (UUID)',

  -- Foreign keys
  message_id VARCHAR(255) NOT NULL
    COMMENT 'Associated message ID (assistant message that made tool call)',

  session_id VARCHAR(255) NOT NULL
    COMMENT 'Associated session ID (denormalized for faster queries)',

  -- Tool metadata
  tool_name VARCHAR(255) NOT NULL
    COMMENT 'Tool name (Read, Write, Edit, Bash, WebSearch, etc.)',

  tool_use_id VARCHAR(255)
    COMMENT 'Tool use ID from LLM API (for matching with results)',

  -- Tool invocation
  arguments JSON
    COMMENT 'Tool input parameters (JSON-encoded)',

  result JSON
    COMMENT 'Tool output/result (JSON-encoded)',

  error TEXT
    COMMENT 'Error message if tool invocation failed',

  -- Timing and performance
  timestamp BIGINT NOT NULL
    COMMENT 'Unix timestamp (milliseconds) when tool called',

  execution_time_ms INT
    COMMENT 'Tool execution duration in milliseconds',

  -- Status tracking
  status ENUM('pending', 'success', 'error', 'timeout') DEFAULT 'pending'
    COMMENT 'Tool call execution status',

  -- Flexible metadata storage
  metadata JSON
    COMMENT 'Additional tool call metadata (retries, context, etc.)',

  -- Foreign key constraints
  FOREIGN KEY (message_id)
    REFERENCES agm_messages(id)
    ON DELETE CASCADE
    COMMENT 'Cascade delete tool calls when message deleted',

  FOREIGN KEY (session_id)
    REFERENCES agm_sessions(id)
    ON DELETE CASCADE
    COMMENT 'Cascade delete tool calls when session deleted',

  -- Indexes for common queries
  INDEX idx_message_id (message_id)
    COMMENT 'Query tool calls by message',

  INDEX idx_session_id (session_id)
    COMMENT 'Query tool calls by session',

  INDEX idx_tool_name (tool_name)
    COMMENT 'Filter/analyze by tool name',

  INDEX idx_timestamp (timestamp)
    COMMENT 'Query tool calls by time',

  INDEX idx_status (status)
    COMMENT 'Filter by execution status',

  INDEX idx_session_tool (session_id, tool_name)
    COMMENT 'Composite index for session + tool analysis',

  INDEX idx_tool_timestamp (tool_name, timestamp)
    COMMENT 'Composite index for tool usage over time'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='AGM tool calls - tracking tool usage, results, and performance';

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the agm_tool_calls table with:
-- ✅ Tool call identification (id, message_id, session_id)
-- ✅ Tool metadata (tool_name, tool_use_id)
-- ✅ Input/output tracking (arguments, result)
-- ✅ Error handling (error field, status enum)
-- ✅ Performance metrics (execution_time_ms)
-- ✅ Cascade deletion (when message or session deleted)
-- ✅ Performance indexes (message_id, session_id, tool_name, status)
-- ==============================================================================
