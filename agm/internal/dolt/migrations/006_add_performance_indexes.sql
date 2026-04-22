-- AGM Migration 005: Performance Optimization Indexes
-- Add composite indexes for common query patterns
-- Author: AGM Team
-- Date: 2026-02-19

-- Optimize session queries by status and date
CREATE INDEX idx_status_created ON agm_sessions (status, created_at);
-- CREATE INDEX idx_workspace_status ON agm_sessions (workspace, status);

-- Optimize message queries by session and timestamp
-- CREATE INDEX idx_session_timestamp ON agm_messages (session_id, timestamp);

-- Optimize tool call queries by session and tool name
-- CREATE INDEX idx_session_tool ON agm_tool_calls (session_id, tool_name);
-- CREATE INDEX idx_tool_timestamp ON agm_tool_calls (tool_name, timestamp);

-- Optimize tag-based session lookup
-- CREATE INDEX idx_tag_created ON agm_session_tags (tag, created_at);
