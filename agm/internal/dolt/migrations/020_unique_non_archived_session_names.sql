-- AGM Migration 020: Unique Non-Archived Session Names
-- Enforces the shared CLI/MCP session-name invariant at the durable write seam.

ALTER TABLE agm_sessions
  ADD UNIQUE KEY uq_agm_sessions_workspace_non_archived_name (
    workspace,
    non_archived_name
  );
