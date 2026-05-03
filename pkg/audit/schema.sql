-- DEAR Audit subsystem schema (ADR-011 §D6).
-- Three additive tables on the workflow engine's runs.db. The
-- workflow schema (pkg/workflow/schema.sql) creates runs, nodes,
-- node_attempts, audit_events, etc.; this file appends. JOINs across
-- the two schemas are valid and used by `workflow audit` queries.
--
-- All statements are IF NOT EXISTS — re-running ApplySchema on a
-- populated DB is a no-op. Every column added in a future revision
-- must come in via a separate ALTER TABLE migration; this file is
-- treated as the canonical "shape at v1" snapshot.

PRAGMA foreign_keys = ON;

-- One row per discovered defect. (repo, fingerprint) is the natural
-- key — reruns of the same check upsert into the same row, which is
-- how the substrate de-duplicates and tracks lifecycles.
CREATE TABLE IF NOT EXISTS audit_findings (
    finding_id    TEXT PRIMARY KEY,
    repo          TEXT NOT NULL,
    fingerprint   TEXT NOT NULL,
    check_id      TEXT NOT NULL,
    severity      TEXT NOT NULL CHECK (severity IN ('P0','P1','P2','P3')),
    state         TEXT NOT NULL CHECK (state IN ('open','acknowledged','resolved','reopened')),
    title         TEXT NOT NULL,
    detail        TEXT NOT NULL DEFAULT '',
    path          TEXT NOT NULL DEFAULT '',
    line          INTEGER NOT NULL DEFAULT 0,
    first_seen    TIMESTAMP NOT NULL,
    last_seen     TIMESTAMP NOT NULL,
    resolved_at   TIMESTAMP,
    state_note    TEXT NOT NULL DEFAULT '',
    suggested_strategy TEXT NOT NULL DEFAULT '',
    suggested_command  TEXT NOT NULL DEFAULT '',
    suggested_patch    TEXT NOT NULL DEFAULT '',
    suggested_title    TEXT NOT NULL DEFAULT '',
    suggested_body     TEXT NOT NULL DEFAULT '',
    evidence_json TEXT NOT NULL DEFAULT '{}',
    UNIQUE (repo, fingerprint)
);
CREATE INDEX IF NOT EXISTS idx_audit_findings_state    ON audit_findings (repo, state);
CREATE INDEX IF NOT EXISTS idx_audit_findings_check    ON audit_findings (repo, check_id);
CREATE INDEX IF NOT EXISTS idx_audit_findings_severity ON audit_findings (repo, severity);
CREATE INDEX IF NOT EXISTS idx_audit_findings_last_seen ON audit_findings (repo, last_seen);

-- One row per audit invocation (one Runner.Run call). The findings_*
-- columns are computed on FinishAuditRun from the post-run state of
-- audit_findings, so they reflect lifecycle deltas, not raw counts.
CREATE TABLE IF NOT EXISTS audit_runs (
    audit_run_id      TEXT PRIMARY KEY,
    repo              TEXT NOT NULL,
    cadence           TEXT NOT NULL,
    started_at        TIMESTAMP NOT NULL,
    finished_at       TIMESTAMP,
    state             TEXT NOT NULL CHECK (state IN ('running','succeeded','failed','partial')),
    triggered_by      TEXT NOT NULL DEFAULT '',
    findings_new      INTEGER NOT NULL DEFAULT 0,
    findings_resolved INTEGER NOT NULL DEFAULT 0,
    findings_open     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_audit_runs_repo ON audit_runs (repo, started_at);
CREATE INDEX IF NOT EXISTS idx_audit_runs_state ON audit_runs (state);

-- One row per Refiner.Propose() output. Persisted in state proposed;
-- the operator transitions via `workflow audit propose --accept|--reject`.
CREATE TABLE IF NOT EXISTS audit_proposals (
    proposal_id   TEXT PRIMARY KEY,
    audit_run_id  TEXT NOT NULL REFERENCES audit_runs(audit_run_id) ON DELETE CASCADE,
    target_layer  TEXT NOT NULL CHECK (target_layer IN ('define','enforce')),
    title         TEXT NOT NULL,
    rationale     TEXT NOT NULL,
    patch         TEXT NOT NULL DEFAULT '',
    state         TEXT NOT NULL CHECK (state IN ('proposed','accepted','rejected','expired')),
    proposed_at   TIMESTAMP NOT NULL,
    decided_at    TIMESTAMP,
    decided_by    TEXT NOT NULL DEFAULT '',
    decision_note TEXT NOT NULL DEFAULT '',
    UNIQUE (audit_run_id, target_layer, title)
);
CREATE INDEX IF NOT EXISTS idx_audit_proposals_state ON audit_proposals (state);
CREATE INDEX IF NOT EXISTS idx_audit_proposals_layer ON audit_proposals (target_layer);
