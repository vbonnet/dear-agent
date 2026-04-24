-- Violations table for instruction compliance observability
CREATE TABLE IF NOT EXISTS violations (
    id TEXT PRIMARY KEY,
    timestamp TEXT NOT NULL,
    instruction_type TEXT NOT NULL,
    instruction_rule TEXT NOT NULL,
    violation_type TEXT NOT NULL,
    confidence TEXT NOT NULL CHECK(confidence IN ('HIGH', 'MEDIUM', 'LOW')),
    agent TEXT NOT NULL,
    context TEXT NOT NULL,
    detection_method TEXT NOT NULL CHECK(detection_method IN ('external', 'self_reported')),
    project_path TEXT,
    phase TEXT
);

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_violations_timestamp
    ON violations(timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_violations_instruction_type
    ON violations(instruction_type);

CREATE INDEX IF NOT EXISTS idx_violations_confidence
    ON violations(confidence);

CREATE INDEX IF NOT EXISTS idx_violations_agent
    ON violations(agent);

-- FTS5 virtual table for full-text search on violation context
CREATE VIRTUAL TABLE IF NOT EXISTS violations_fts USING fts5(
    context,
    content=violations,
    content_rowid=rowid
);

-- Triggers to keep FTS5 index synced with violations table
CREATE TRIGGER IF NOT EXISTS violations_ai AFTER INSERT ON violations BEGIN
    INSERT INTO violations_fts(rowid, context) VALUES (new.rowid, new.context);
END;

CREATE TRIGGER IF NOT EXISTS violations_ad AFTER DELETE ON violations BEGIN
    DELETE FROM violations_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS violations_au AFTER UPDATE ON violations BEGIN
    UPDATE violations_fts SET context = new.context WHERE rowid = new.rowid;
END;
