-- pkg/source/sqlite schema (Phase 3 — addressable knowledge surface).
-- Two tables back the adapter:
--   - sources      : the canonical row, one per URI
--   - sources_fts  : FTS5 virtual table, queried by Fetch
--
-- The FTS5 table is content-less ("contentless" mode) — sources owns the
-- bytes; sources_fts owns only the term index. This keeps writes cheap
-- (no double-storage of Content) and lets us order results by the
-- adapter's own scoring rules. We mirror Title + Snippet + Content into
-- the FTS table so MATCH queries cover all three.
--
-- Cues are stored as a tab-separated string in sources.cues (no FTS
-- index — the adapter scans them in WHERE because Cues filtering is
-- exact-match, not full-text). Custom is JSON-encoded.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS sources (
    uri          TEXT PRIMARY KEY,
    title        TEXT NOT NULL DEFAULT '',
    snippet      TEXT NOT NULL DEFAULT '',
    content      BLOB,
    cues         TEXT NOT NULL DEFAULT '',  -- tab-separated, leading+trailing tab for exact match
    work_item    TEXT NOT NULL DEFAULT '',
    role         TEXT NOT NULL DEFAULT '',
    confidence   REAL NOT NULL DEFAULT 0,
    src_origin   TEXT NOT NULL DEFAULT '',  -- Metadata.Source — renamed to avoid SQL keyword
    custom_json  TEXT NOT NULL DEFAULT '',
    indexed_at   TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sources_indexed_at ON sources (indexed_at);
CREATE INDEX IF NOT EXISTS idx_sources_work_item  ON sources (work_item);

CREATE VIRTUAL TABLE IF NOT EXISTS sources_fts USING fts5(
    uri,
    title,
    snippet,
    content,
    tokenize = 'unicode61 remove_diacritics 2'
);
