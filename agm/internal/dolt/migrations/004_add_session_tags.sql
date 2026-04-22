-- AGM Migration 003: Add Session Tags
-- Enable categorization and filtering of sessions via tags
-- Author: AGM Team
-- Date: 2026-02-19

CREATE TABLE IF NOT EXISTS agm_session_tags (
  id INT AUTO_INCREMENT PRIMARY KEY,
  session_id VARCHAR(255) NOT NULL,
  tag VARCHAR(255) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  created_by VARCHAR(255),

  FOREIGN KEY (session_id) REFERENCES agm_sessions(id) ON DELETE CASCADE,
  INDEX idx_session_id (session_id),
  INDEX idx_tag (tag),
  INDEX idx_session_tag (session_id, tag),
  UNIQUE KEY unique_session_tag (session_id, tag)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='AGM session tags - categorization and filtering';
