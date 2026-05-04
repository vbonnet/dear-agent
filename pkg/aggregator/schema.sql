-- Signal aggregator SQLite schema (Phase 1 — see ADR-015).
--
-- One table holds every signal, regardless of Kind. Cross-kind joins are
-- handled in Go; the schema's job is durable, indexed time-series storage.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS signals (
    signal_id     TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    subject       TEXT NOT NULL,
    value         REAL NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    collected_at  TIMESTAMP NOT NULL
);

-- Covers Recent(kind, limit) and Range(kind, since): both filter on
-- kind and order by collected_at.
CREATE INDEX IF NOT EXISTS idx_signals_kind_at
    ON signals (kind, collected_at);

-- Covers per-subject drill-down ("show me every signal for pkg/foo").
CREATE INDEX IF NOT EXISTS idx_signals_subject
    ON signals (subject);
