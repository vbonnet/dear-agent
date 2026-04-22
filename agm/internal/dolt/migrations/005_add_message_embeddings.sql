-- AGM Migration 004: Add Message Embeddings
-- Store message embeddings for semantic search and retrieval
-- Author: AGM Team
-- Date: 2026-02-19

CREATE TABLE IF NOT EXISTS agm_message_embeddings (
  id INT AUTO_INCREMENT PRIMARY KEY,
  message_id VARCHAR(255) NOT NULL,
  session_id VARCHAR(255) NOT NULL,
  embedding_model VARCHAR(100) NOT NULL,
  embedding BLOB NOT NULL,
  dimension INT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

  FOREIGN KEY (message_id) REFERENCES agm_messages(id) ON DELETE CASCADE,
  FOREIGN KEY (session_id) REFERENCES agm_sessions(id) ON DELETE CASCADE,
  INDEX idx_message_id (message_id),
  INDEX idx_session_id (session_id),
  INDEX idx_model (embedding_model),
  UNIQUE KEY unique_message_model (message_id, embedding_model)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='AGM message embeddings - for semantic search';
