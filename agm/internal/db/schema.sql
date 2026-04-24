-- AGM SQLite Schema (Phase 2: SQLite Persistence)
-- Maps to internal/manifest/manifest.go Manifest struct
-- Supports conversations, escalations, and full-text search

-- Enable WAL mode for better concurrent access
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

-- ============================================================================
-- SESSIONS TABLE
-- Maps to Manifest struct from internal/manifest/manifest.go
-- ============================================================================

CREATE TABLE IF NOT EXISTS sessions (
    -- Primary fields
    session_id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    schema_version TEXT NOT NULL DEFAULT '2.0',

    -- Timestamps
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,

    -- Lifecycle state
    lifecycle TEXT NOT NULL DEFAULT '', -- '' (active/stopped) or 'archived'

    -- Harness metadata
    harness TEXT, -- 'claude-code', 'gemini-cli', 'codex-cli', 'opencode-cli'
    model TEXT,   -- Model within the harness (e.g., 'claude-opus-4-6', 'gemini-2.5-flash')

    -- Context (stored as JSON)
    context_project TEXT,
    context_purpose TEXT,
    context_tags TEXT, -- JSON array
    context_notes TEXT,

    -- Claude metadata
    claude_uuid TEXT, -- Claude session UUID for resume

    -- Tmux metadata
    tmux_session_name TEXT,

    -- Engram metadata (stored as JSON)
    engram_enabled INTEGER DEFAULT 0, -- Boolean: 0=false, 1=true
    engram_query TEXT,
    engram_ids TEXT, -- JSON array of engram IDs
    engram_loaded_at TIMESTAMP,
    engram_count INTEGER DEFAULT 0,

    -- Hierarchical swarm support (Phase 4)
    parent_session_id TEXT,

    FOREIGN KEY (parent_session_id) REFERENCES sessions(session_id) ON DELETE SET NULL
);

-- ============================================================================
-- MESSAGES TABLE
-- Maps to internal/conversation/types.go Message struct
-- Stores conversation history for sessions
-- ============================================================================

CREATE TABLE IF NOT EXISTS messages (
    -- Primary key
    message_id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- Foreign key to sessions
    session_id TEXT NOT NULL,

    -- Message metadata
    timestamp TIMESTAMP NOT NULL,
    role TEXT NOT NULL, -- 'user' or 'assistant'
    harness TEXT NOT NULL, -- 'claude-code', 'gemini-cli', 'codex-cli', 'opencode-cli'

    -- Content (stored as JSON ContentBlock array)
    content TEXT NOT NULL, -- JSON array of content blocks

    -- Token usage
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,

    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE CASCADE
);

-- ============================================================================
-- ESCALATIONS TABLE
-- Maps to internal/db/escalations.go EscalationEvent struct
-- Stores detected escalations requiring user intervention
-- ============================================================================

CREATE TABLE IF NOT EXISTS escalations (
    -- Primary key
    escalation_id INTEGER PRIMARY KEY AUTOINCREMENT,

    -- Foreign key to sessions
    session_id TEXT NOT NULL,

    -- Escalation metadata
    type TEXT NOT NULL, -- 'error', 'prompt', 'warning'
    pattern TEXT NOT NULL, -- Regex pattern that triggered escalation
    line TEXT NOT NULL, -- The actual line from output
    line_number INTEGER, -- Line number in output stream
    detected_at TIMESTAMP NOT NULL,
    description TEXT, -- Human-readable description

    -- Resolution tracking
    resolved INTEGER DEFAULT 0, -- Boolean: 0=unresolved, 1=resolved
    resolved_at TIMESTAMP,
    resolution_note TEXT,

    FOREIGN KEY (session_id) REFERENCES sessions(session_id) ON DELETE CASCADE
);

-- ============================================================================
-- INDEXES
-- Optimize common queries
-- ============================================================================

-- Sessions indexes
CREATE INDEX IF NOT EXISTS idx_sessions_lifecycle ON sessions(lifecycle);
CREATE INDEX IF NOT EXISTS idx_sessions_harness ON sessions(harness);
CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_parent ON sessions(parent_session_id);
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at DESC);

-- Messages indexes
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_messages_role ON messages(role);

-- Escalations indexes
CREATE INDEX IF NOT EXISTS idx_escalations_session_id ON escalations(session_id);
CREATE INDEX IF NOT EXISTS idx_escalations_type ON escalations(type);
CREATE INDEX IF NOT EXISTS idx_escalations_resolved ON escalations(resolved);
CREATE INDEX IF NOT EXISTS idx_escalations_detected_at ON escalations(detected_at DESC);

-- ============================================================================
-- FULL-TEXT SEARCH (FTS5)
-- Enable fast keyword search across sessions
-- ============================================================================

CREATE VIRTUAL TABLE IF NOT EXISTS sessions_fts USING fts5(
    session_id UNINDEXED, -- Don't index the ID itself
    name,
    context_project,
    context_purpose,
    context_notes,
    content='sessions', -- Links to sessions table
    content_rowid='rowid'
);

-- Triggers to keep FTS5 table in sync with sessions table
CREATE TRIGGER IF NOT EXISTS sessions_fts_insert AFTER INSERT ON sessions BEGIN
    INSERT INTO sessions_fts(rowid, session_id, name, context_project, context_purpose, context_notes)
    VALUES (new.rowid, new.session_id, new.name, new.context_project, new.context_purpose, new.context_notes);
END;

CREATE TRIGGER IF NOT EXISTS sessions_fts_delete AFTER DELETE ON sessions BEGIN
    INSERT INTO sessions_fts(sessions_fts, rowid, session_id, name, context_project, context_purpose, context_notes)
    VALUES('delete', old.rowid, old.session_id, old.name, old.context_project, old.context_purpose, old.context_notes);
END;

CREATE TRIGGER IF NOT EXISTS sessions_fts_update AFTER UPDATE ON sessions BEGIN
    INSERT INTO sessions_fts(sessions_fts, rowid, session_id, name, context_project, context_purpose, context_notes)
    VALUES('delete', old.rowid, old.session_id, old.name, old.context_project, old.context_purpose, old.context_notes);
    INSERT INTO sessions_fts(rowid, session_id, name, context_project, context_purpose, context_notes)
    VALUES (new.rowid, new.session_id, new.name, new.context_project, new.context_purpose, new.context_notes);
END;

-- ============================================================================
-- VIEWS
-- Convenience views for common queries
-- ============================================================================

-- Active sessions view (not archived)
CREATE VIEW IF NOT EXISTS active_sessions AS
SELECT * FROM sessions
WHERE lifecycle != 'archived'
ORDER BY updated_at DESC;

-- Archived sessions view
CREATE VIEW IF NOT EXISTS archived_sessions AS
SELECT * FROM sessions
WHERE lifecycle = 'archived'
ORDER BY updated_at DESC;

-- Unresolved escalations view
CREATE VIEW IF NOT EXISTS unresolved_escalations AS
SELECT e.*, s.name as session_name
FROM escalations e
JOIN sessions s ON e.session_id = s.session_id
WHERE e.resolved = 0
ORDER BY e.detected_at DESC;

-- Session summary view (with message and escalation counts)
CREATE VIEW IF NOT EXISTS session_summary AS
SELECT
    s.session_id,
    s.name,
    s.harness,
    s.lifecycle,
    s.created_at,
    s.updated_at,
    s.parent_session_id,
    COUNT(DISTINCT m.message_id) as message_count,
    COUNT(DISTINCT e.escalation_id) as escalation_count,
    SUM(CASE WHEN e.resolved = 0 THEN 1 ELSE 0 END) as unresolved_escalation_count
FROM sessions s
LEFT JOIN messages m ON s.session_id = m.session_id
LEFT JOIN escalations e ON s.session_id = e.session_id
GROUP BY s.session_id;
