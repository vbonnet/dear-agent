-- AGM Migration 013: Add context usage columns to agm_sessions
-- Persists context window usage for status line display
-- Author: AGM Team
-- Date: 2026-03-31

ALTER TABLE agm_sessions
  ADD COLUMN context_total_tokens INT DEFAULT NULL
    COMMENT 'Total available tokens in context window',
  ADD COLUMN context_used_tokens INT DEFAULT NULL
    COMMENT 'Currently used tokens',
  ADD COLUMN context_percentage_used DOUBLE DEFAULT NULL
    COMMENT 'Percentage of context used (0-100)'
