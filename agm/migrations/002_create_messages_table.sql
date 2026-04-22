-- ==============================================================================
-- AGM Migration 002: Messages Table
-- ==============================================================================
-- Version: 2
-- Name: create_messages_table
-- Description: Create agm_messages table for conversation history
-- Checksum: sha256:placeholder002
-- Tables Created: agm_messages
-- Depends: Migration 001 (references agm_sessions)
-- Applied By: agm-2.0.0
-- Estimated Time: 30ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS agm_messages (
  -- Primary key
  id VARCHAR(255) PRIMARY KEY
    COMMENT 'Unique message identifier (UUID)',

  -- Foreign key to session
  session_id VARCHAR(255) NOT NULL
    COMMENT 'Associated session ID',

  -- Message metadata
  role ENUM('user', 'assistant', 'system') NOT NULL
    COMMENT 'Message role in conversation',

  content TEXT NOT NULL
    COMMENT 'Message content (text, markdown, etc.)',

  -- Ordering and timing
  timestamp BIGINT NOT NULL
    COMMENT 'Unix timestamp (milliseconds)',

  sequence_number INT NOT NULL
    COMMENT 'Message order within session (0-indexed)',

  -- Token tracking (for cost/context management)
  input_tokens INT
    COMMENT 'Input token count (for assistant messages)',

  output_tokens INT
    COMMENT 'Output token count (for assistant messages)',

  -- Cache metadata (Claude Code cache tiers)
  cache_creation_tokens INT
    COMMENT 'Tokens used to create cache',

  cache_read_tokens INT
    COMMENT 'Tokens read from cache',

  -- Flexible metadata storage
  metadata JSON
    COMMENT 'Additional message metadata (tool_calls, thinking, attachments, etc.)',

  -- Foreign key constraint
  FOREIGN KEY (session_id)
    REFERENCES agm_sessions(id)
    ON DELETE CASCADE
    COMMENT 'Cascade delete messages when session deleted',

  -- Indexes for common queries
  INDEX idx_session_id (session_id)
    COMMENT 'Query messages by session',

  INDEX idx_timestamp (timestamp)
    COMMENT 'Query messages by time',

  INDEX idx_sequence (session_id, sequence_number)
    COMMENT 'Ordered message retrieval within session',

  INDEX idx_role (role)
    COMMENT 'Filter by message role',

  INDEX idx_session_timestamp (session_id, timestamp)
    COMMENT 'Composite index for session + time queries'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='AGM messages - individual messages within conversation sessions';

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the agm_messages table with:
-- ✅ Message identification (id, session_id)
-- ✅ Role-based conversation structure (user, assistant, system)
-- ✅ Content storage (text field)
-- ✅ Sequence ordering (sequence_number)
-- ✅ Token tracking (input/output/cache tokens)
-- ✅ Cascade deletion (when session deleted)
-- ✅ Performance indexes (session_id, timestamp, sequence)
-- ==============================================================================
