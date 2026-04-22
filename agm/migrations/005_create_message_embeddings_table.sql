-- ==============================================================================
-- AGM Migration 005: Message Embeddings Table
-- ==============================================================================
-- Version: 5
-- Name: create_message_embeddings_table
-- Description: Create agm_message_embeddings table for semantic search
-- Checksum: sha256:placeholder005
-- Tables Created: agm_message_embeddings
-- Depends: Migrations 001, 002 (references agm_sessions, agm_messages)
-- Applied By: agm-2.0.0
-- Estimated Time: 40ms
-- ==============================================================================

CREATE TABLE IF NOT EXISTS agm_message_embeddings (
  -- Primary key
  id INT AUTO_INCREMENT PRIMARY KEY
    COMMENT 'Auto-incrementing embedding ID',

  -- Foreign keys
  message_id VARCHAR(255) NOT NULL
    COMMENT 'Associated message ID',

  session_id VARCHAR(255) NOT NULL
    COMMENT 'Associated session ID (denormalized for faster queries)',

  -- Embedding metadata
  embedding_model VARCHAR(100) NOT NULL
    COMMENT 'Embedding model identifier (text-embedding-3-small, etc.)',

  embedding BLOB NOT NULL
    COMMENT 'Binary vector representation (float32 array)',

  dimension INT NOT NULL
    COMMENT 'Vector dimension (e.g., 1536 for text-embedding-3-small)',

  -- Tracking
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    COMMENT 'When embedding was created',

  -- Foreign key constraints
  FOREIGN KEY (message_id)
    REFERENCES agm_messages(id)
    ON DELETE CASCADE
    COMMENT 'Cascade delete embeddings when message deleted',

  FOREIGN KEY (session_id)
    REFERENCES agm_sessions(id)
    ON DELETE CASCADE
    COMMENT 'Cascade delete embeddings when session deleted',

  -- Indexes for common queries
  INDEX idx_message_id (message_id)
    COMMENT 'Query embeddings by message',

  INDEX idx_session_id (session_id)
    COMMENT 'Query embeddings by session',

  INDEX idx_model (embedding_model)
    COMMENT 'Filter by embedding model',

  INDEX idx_created_at (created_at)
    COMMENT 'Query embeddings by creation time',

  -- Uniqueness constraint
  UNIQUE KEY unique_message_model (message_id, embedding_model)
    COMMENT 'One embedding per message per model'

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='AGM message embeddings - for semantic search and retrieval';

-- ==============================================================================
-- Notes on Semantic Search
-- ==============================================================================
-- For vector similarity search, consider:
--
-- 1. External Vector Database (Recommended for production):
--    - Pinecone, Weaviate, Qdrant, or Milvus
--    - Optimized for high-dimensional vector search
--    - Better performance than MySQL BLOB storage
--
-- 2. MySQL Vector Search (MySQL 8.0.32+):
--    - CREATE TABLE with VECTOR type (if supported)
--    - Use IVF (Inverted File Index) for approximate nearest neighbor
--
-- 3. Hybrid Search (Current Approach):
--    - Store embeddings in MySQL for persistence
--    - Load into memory for similarity search
--    - Use cosine similarity in application layer
--
-- 4. Full-Text Search Complement:
--    - Add FULLTEXT index on agm_messages.content
--    - Combine keyword search + semantic search for best results
--
-- Example Hybrid Query:
--   SELECT m.id, m.content, e.embedding
--   FROM agm_messages m
--   JOIN agm_message_embeddings e ON m.id = e.message_id
--   WHERE m.session_id = :session_id
--   AND e.embedding_model = 'text-embedding-3-small'
--   ORDER BY m.timestamp DESC
--
-- Then compute cosine similarity in application layer.
-- ==============================================================================

-- ==============================================================================
-- Migration Complete
-- ==============================================================================
-- This migration creates the agm_message_embeddings table with:
-- ✅ Embedding storage (id, message_id, embedding)
-- ✅ Model tracking (embedding_model, dimension)
-- ✅ Cascade deletion (when message or session deleted)
-- ✅ Duplicate prevention (unique constraint on message + model)
-- ✅ Performance indexes (message_id, session_id, model)
-- ✅ Future-ready for semantic search integration
-- ==============================================================================
