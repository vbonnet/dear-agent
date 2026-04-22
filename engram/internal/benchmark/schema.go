package benchmark

// schema defines the SQLite database schema for benchmark storage.
const schema = `
CREATE TABLE IF NOT EXISTS benchmark_runs (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id             TEXT UNIQUE NOT NULL,
    timestamp          TEXT NOT NULL,
    variant            TEXT NOT NULL CHECK(variant IN ('raw', 'engram', 'wayfinder')),
    project_size       TEXT NOT NULL CHECK(project_size IN ('small', 'medium', 'large', 'xl')),
    project_name       TEXT,

    -- Quality Metrics
    quality_score      REAL CHECK(quality_score >= 0.0 AND quality_score <= 10.0),

    -- Cost Metrics
    cost_usd           REAL CHECK(cost_usd >= 0.0),
    tokens_input       INTEGER CHECK(tokens_input >= 0),
    tokens_output      INTEGER CHECK(tokens_output >= 0),

    -- Time Metrics
    duration_ms        INTEGER CHECK(duration_ms >= 0),

    -- Deliverable Metrics
    file_count         INTEGER CHECK(file_count >= 0),
    documentation_kb   REAL CHECK(documentation_kb >= 0.0),

    -- Process Metrics
    phases_completed   INTEGER CHECK(phases_completed >= 0 AND phases_completed <= 11),

    -- Execution Status
    successful         INTEGER NOT NULL DEFAULT 1 CHECK(successful IN (0, 1)),

    -- Extensible Metadata (JSON)
    metadata           TEXT,

    -- Audit
    created_at         TEXT DEFAULT (datetime('now'))
);

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_variant ON benchmark_runs(variant);
CREATE INDEX IF NOT EXISTS idx_project_size ON benchmark_runs(project_size);
CREATE INDEX IF NOT EXISTS idx_timestamp ON benchmark_runs(timestamp);
CREATE INDEX IF NOT EXISTS idx_quality_score ON benchmark_runs(quality_score);
CREATE INDEX IF NOT EXISTS idx_cost_usd ON benchmark_runs(cost_usd);
CREATE INDEX IF NOT EXISTS idx_successful ON benchmark_runs(successful);
CREATE INDEX IF NOT EXISTS idx_run_id ON benchmark_runs(run_id);

-- Composite index for common filter combinations
CREATE INDEX IF NOT EXISTS idx_variant_size ON benchmark_runs(variant, project_size);
`
