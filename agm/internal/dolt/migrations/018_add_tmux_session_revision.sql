-- Migration 018: add an ownership token for provisional tmux-name changes.
-- The token lets resume compensation compare-and-swap identically through
-- both MySQL/Dolt and the isolated SQLite test adapter.

ALTER TABLE agm_sessions
  ADD COLUMN tmux_session_revision VARCHAR(64) NULL;
