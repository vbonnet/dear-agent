-- Workflow engine SQLite schema (Phase 0 — substrate-quality work-item layer).
-- Canonical DDL is ADR-010 §5; deviations from that ADR are bugs in this file.
--
-- Phase 0 populates: runs, nodes, node_attempts, audit_events.
-- Later phases populate node_outputs (Phase 1.6) and approvals (Phase 2).
-- The full schema lands together so adapters and CLIs can JOIN against the
-- final shape without a migration step between phases.

PRAGMA foreign_keys = ON;

-- Workflow definitions, cached by canonical YAML hash.
-- A row is written the first time a runner sees a given (name, version, hash).
CREATE TABLE IF NOT EXISTS workflows (
    workflow_id    TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    version        TEXT NOT NULL,
    yaml_canonical TEXT NOT NULL,
    registered_at  TIMESTAMP NOT NULL,
    UNIQUE (name, version, workflow_id)
);
CREATE INDEX IF NOT EXISTS idx_workflows_name ON workflows (name);

-- One row per run. The work-item.
CREATE TABLE IF NOT EXISTS runs (
    run_id        TEXT PRIMARY KEY,
    workflow_id   TEXT NOT NULL REFERENCES workflows (workflow_id),
    state         TEXT NOT NULL CHECK (state IN
                  ('pending','running','awaiting_hitl','succeeded','failed','cancelled')),
    inputs_json   TEXT NOT NULL,
    started_at    TIMESTAMP NOT NULL,
    finished_at   TIMESTAMP,
    total_tokens  INTEGER NOT NULL DEFAULT 0,
    total_dollars REAL    NOT NULL DEFAULT 0,
    error         TEXT,
    trigger       TEXT,
    triggered_by  TEXT
);
CREATE INDEX IF NOT EXISTS idx_runs_state       ON runs (state);
CREATE INDEX IF NOT EXISTS idx_runs_started_at  ON runs (started_at);
CREATE INDEX IF NOT EXISTS idx_runs_workflow_id ON runs (workflow_id);

-- Per-run, per-node aggregate state.
CREATE TABLE IF NOT EXISTS nodes (
    run_id        TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    node_id       TEXT NOT NULL,
    state         TEXT NOT NULL CHECK (state IN
                  ('pending','running','awaiting_hitl','succeeded','failed','skipped')),
    attempts      INTEGER NOT NULL DEFAULT 0,
    role_used     TEXT,
    model_used    TEXT,
    tokens_used   INTEGER NOT NULL DEFAULT 0,
    dollars_spent REAL    NOT NULL DEFAULT 0,
    output        TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMP,
    finished_at   TIMESTAMP,
    error         TEXT,
    PRIMARY KEY (run_id, node_id)
);
CREATE INDEX IF NOT EXISTS idx_nodes_state ON nodes (state);

-- Per-attempt detail (one row per execution attempt; retries are visible).
CREATE TABLE IF NOT EXISTS node_attempts (
    attempt_id    TEXT PRIMARY KEY,
    run_id        TEXT NOT NULL,
    node_id       TEXT NOT NULL,
    attempt_no    INTEGER NOT NULL,
    state         TEXT NOT NULL,
    model_used    TEXT,
    prompt_hash   TEXT,
    response_hash TEXT,
    tokens_used   INTEGER NOT NULL DEFAULT 0,
    dollars_spent REAL    NOT NULL DEFAULT 0,
    started_at    TIMESTAMP NOT NULL,
    finished_at   TIMESTAMP,
    error_class   TEXT,
    error_message TEXT,
    FOREIGN KEY (run_id, node_id) REFERENCES nodes (run_id, node_id) ON DELETE CASCADE,
    UNIQUE (run_id, node_id, attempt_no)
);
CREATE INDEX IF NOT EXISTS idx_node_attempts_run_node ON node_attempts (run_id, node_id);

-- Declared, durable artifacts. Populated by Phase 1.6 (outputs[] writer).
CREATE TABLE IF NOT EXISTS node_outputs (
    run_id        TEXT NOT NULL,
    node_id       TEXT NOT NULL,
    output_key    TEXT NOT NULL,
    path          TEXT NOT NULL,
    content_type  TEXT,
    durability    TEXT NOT NULL CHECK (durability IN
                  ('ephemeral','local_disk','git_committed','engram_indexed')),
    size_bytes    INTEGER,
    hash          TEXT,
    indexed_at    TIMESTAMP,
    PRIMARY KEY (run_id, node_id, output_key),
    FOREIGN KEY (run_id, node_id) REFERENCES nodes (run_id, node_id) ON DELETE CASCADE
);

-- One row per state transition. The substrate's audit log.
CREATE TABLE IF NOT EXISTS audit_events (
    event_id     TEXT PRIMARY KEY,
    run_id       TEXT NOT NULL,
    node_id      TEXT,
    attempt_no   INTEGER,
    from_state   TEXT,
    to_state     TEXT NOT NULL,
    reason       TEXT,
    actor        TEXT NOT NULL,
    occurred_at  TIMESTAMP NOT NULL,
    payload_json TEXT
);
CREATE INDEX IF NOT EXISTS idx_audit_events_run      ON audit_events (run_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor    ON audit_events (actor);
CREATE INDEX IF NOT EXISTS idx_audit_events_to_state ON audit_events (to_state);

-- HITL records. Populated by Phase 2.2 (HITL).
CREATE TABLE IF NOT EXISTS approvals (
    approval_id   TEXT PRIMARY KEY,
    run_id        TEXT NOT NULL,
    node_id       TEXT NOT NULL,
    requested_at  TIMESTAMP NOT NULL,
    resolved_at   TIMESTAMP,
    decision      TEXT CHECK (decision IN ('approve','reject','timeout')),
    approver      TEXT,
    approver_role TEXT,
    reason        TEXT,
    FOREIGN KEY (run_id, node_id) REFERENCES nodes (run_id, node_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_approvals_pending
    ON approvals (run_id, node_id) WHERE resolved_at IS NULL;
