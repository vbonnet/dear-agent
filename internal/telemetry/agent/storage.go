package agent

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // sqlite3 driver
)

// AgentLaunch represents a logged agent launch with features and outcome.
type AgentLaunch struct {
	ID                    int64
	Timestamp             time.Time
	PromptText            string
	Model                 string
	TaskDescription       string
	SessionID             string
	ParentAgentID         string
	WordCount             int
	TokenCount            int
	SpecificityScore      float64
	HasExamples           bool
	HasConstraints        bool
	ContextEmbeddingScore float64
	Outcome               string
	RetryCount            int
	TokensUsed            int
	ErrorMessage          string
	DurationMs            int
	CreatedAt             time.Time
}

// Storage provides SQLite persistence for agent telemetry.
type Storage struct {
	db *sql.DB
}

// schema defines the SQLite table structure.
const schema = `
CREATE TABLE IF NOT EXISTS agent_launches (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp TEXT NOT NULL,
    prompt_text TEXT NOT NULL,
    model TEXT NOT NULL,
    task_description TEXT,
    session_id TEXT,
    parent_agent_id TEXT,

    -- Prompt features
    word_count INTEGER NOT NULL,
    token_count INTEGER NOT NULL,
    specificity_score REAL NOT NULL,
    has_examples INTEGER NOT NULL,
    has_constraints INTEGER NOT NULL,
    context_embedding_score REAL NOT NULL,

    -- Outcome data (updated on completion)
    outcome TEXT CHECK(outcome IN ('success', 'failure', 'partial', NULL)),
    retry_count INTEGER DEFAULT 0,
    tokens_used INTEGER,
    error_message TEXT,
    duration_ms INTEGER,

    created_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_outcome ON agent_launches(outcome);
CREATE INDEX IF NOT EXISTS idx_timestamp ON agent_launches(timestamp);
CREATE INDEX IF NOT EXISTS idx_model ON agent_launches(model);
CREATE INDEX IF NOT EXISTS idx_session_id ON agent_launches(session_id);
`

// NewStorage creates a new Storage instance with SQLite database.
//
// Database location: ~/.engram/telemetry.db
//
// Example:
//
//	storage, err := NewStorage()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer storage.Close()
func NewStorage() (*Storage, error) {
	return NewStorageAt(DefaultDatabasePath())
}

// NewStorageAt creates a Storage instance at the specified database path.
//
// The database file and parent directories are created if they don't exist.
// WAL mode is enabled for better concurrent write performance.
func NewStorageAt(path string) (*Storage, error) {
	// Expand ~ to home directory
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	storage := &Storage{db: db}

	// Initialize schema
	if err := storage.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return storage, nil
}

// DefaultDatabasePath returns the default database path.
func DefaultDatabasePath() string {
	return "~/.engram/telemetry.db"
}

// initSchema creates the database schema if it doesn't exist.
func (s *Storage) initSchema() error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// DB returns the underlying database connection for custom queries.
func (s *Storage) DB() *sql.DB {
	return s.db
}

// LogLaunch records an agent launch with extracted features.
//
// Returns the launch ID for later updating with outcome data.
//
// Example:
//
//	features := ExtractFeatures(prompt)
//	id, err := storage.LogLaunch(ctx, prompt, "claude-sonnet-4.5", features)
func (s *Storage) LogLaunch(ctx context.Context, prompt, model string, features Features) (int64, error) {
	return s.LogLaunchFull(ctx, prompt, model, "", "", "", features)
}

// LogLaunchFull records an agent launch with all metadata.
func (s *Storage) LogLaunchFull(ctx context.Context, prompt, model, taskDesc, sessionID, parentID string, features Features) (int64, error) {
	query := `
		INSERT INTO agent_launches (
			timestamp, prompt_text, model, task_description, session_id, parent_agent_id,
			word_count, token_count, specificity_score, has_examples, has_constraints, context_embedding_score
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := s.db.ExecContext(ctx, query,
		time.Now().Format(time.RFC3339),
		prompt,
		model,
		taskDesc,
		sessionID,
		parentID,
		features.WordCount,
		features.TokenCount,
		features.SpecificityScore,
		boolToInt(features.HasExamples),
		boolToInt(features.HasConstraints),
		features.ContextEmbeddingScore,
	)

	if err != nil {
		return 0, fmt.Errorf("failed to insert launch: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	return id, nil
}

// UpdateOutcome updates an agent launch record with completion data.
//
// Example:
//
//	err := storage.UpdateOutcome(ctx, id, "success", 1500)
func (s *Storage) UpdateOutcome(ctx context.Context, id int64, outcome string, tokensUsed int) error {
	return s.UpdateOutcomeFull(ctx, id, outcome, tokensUsed, 0, "", 0)
}

// UpdateOutcomeFull updates an agent launch with all completion data.
func (s *Storage) UpdateOutcomeFull(ctx context.Context, id int64, outcome string, tokensUsed, retryCount int, errorMsg string, durationMs int) error {
	query := `
		UPDATE agent_launches
		SET outcome = ?, tokens_used = ?, retry_count = ?, error_message = ?, duration_ms = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query, outcome, tokensUsed, retryCount, errorMsg, durationMs, id)
	if err != nil {
		return fmt.Errorf("failed to update outcome: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no rows updated for ID %d", id)
	}

	return nil
}

// QueryFilters specifies filters for querying agent launches.
type QueryFilters struct {
	Outcome string    // Filter by outcome ("success", "failure", "partial")
	Model   string    // Filter by model name
	Since   time.Time // Filter by timestamp (after this time)
	Limit   int       // Result limit (default 100)
}

// Query retrieves agent launches matching the specified filters.
//
// Example:
//
//	filters := QueryFilters{
//	    Outcome: "success",
//	    Model:   "claude-sonnet-4.5",
//	    Limit:   50,
//	}
//	launches, err := storage.Query(ctx, filters)
func (s *Storage) Query(ctx context.Context, filters QueryFilters) ([]AgentLaunch, error) {
	query, args := buildLaunchQuery(filters)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query launches: %w", err)
	}
	defer rows.Close()

	var launches []AgentLaunch
	for rows.Next() {
		launch, err := scanAgentLaunch(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		launches = append(launches, launch)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return launches, nil
}

// buildLaunchQuery constructs SQL query and arguments from filters
func buildLaunchQuery(filters QueryFilters) (string, []interface{}) {
	query := "SELECT id, timestamp, prompt_text, model, task_description, session_id, parent_agent_id, " +
		"word_count, token_count, specificity_score, has_examples, has_constraints, context_embedding_score, " +
		"outcome, retry_count, tokens_used, error_message, duration_ms, created_at " +
		"FROM agent_launches WHERE 1=1"

	args := []interface{}{}

	if filters.Outcome != "" {
		query += " AND outcome = ?"
		args = append(args, filters.Outcome)
	}

	if filters.Model != "" {
		query += " AND model = ?"
		args = append(args, filters.Model)
	}

	if !filters.Since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, filters.Since.Format(time.RFC3339))
	}

	query += " ORDER BY timestamp DESC"

	if filters.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filters.Limit)
	} else {
		query += " LIMIT 100" // Default limit
	}

	return query, args
}

// scanAgentLaunch scans a database row into an AgentLaunch struct
func scanAgentLaunch(rows *sql.Rows) (AgentLaunch, error) {
	var l AgentLaunch
	var timestampStr, createdAtStr string
	var outcomeStr, taskDesc, sessionID, parentID, errorMsg sql.NullString
	var retryCount, tokensUsed, durationMs sql.NullInt32
	var hasExamples, hasConstraints int

	err := rows.Scan(
		&l.ID, &timestampStr, &l.PromptText, &l.Model, &taskDesc, &sessionID, &parentID,
		&l.WordCount, &l.TokenCount, &l.SpecificityScore, &hasExamples, &hasConstraints, &l.ContextEmbeddingScore,
		&outcomeStr, &retryCount, &tokensUsed, &errorMsg, &durationMs, &createdAtStr,
	)
	if err != nil {
		return l, err
	}

	// Parse timestamps
	l.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
	l.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)

	// Parse nullable fields
	if outcomeStr.Valid {
		l.Outcome = outcomeStr.String
	}
	if taskDesc.Valid {
		l.TaskDescription = taskDesc.String
	}
	if sessionID.Valid {
		l.SessionID = sessionID.String
	}
	if parentID.Valid {
		l.ParentAgentID = parentID.String
	}
	if errorMsg.Valid {
		l.ErrorMessage = errorMsg.String
	}
	if retryCount.Valid {
		l.RetryCount = int(retryCount.Int32)
	}
	if tokensUsed.Valid {
		l.TokensUsed = int(tokensUsed.Int32)
	}
	if durationMs.Valid {
		l.DurationMs = int(durationMs.Int32)
	}

	l.HasExamples = intToBool(hasExamples)
	l.HasConstraints = intToBool(hasConstraints)

	return l, nil
}

// Stats returns aggregate statistics for agent launches.
func (s *Storage) Stats(ctx context.Context, model string) (*AgentStats, error) {
	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN outcome = 'success' THEN 1 ELSE 0 END) as success_count,
			AVG(tokens_used) as avg_tokens,
			AVG(specificity_score) as avg_specificity,
			AVG(context_embedding_score) as avg_context
		FROM agent_launches
		WHERE 1=1
	`

	args := []interface{}{}
	if model != "" {
		query += " AND model = ?"
		args = append(args, model)
	}

	var stats AgentStats
	var successCount sql.NullInt64
	var avgTokens, avgSpec, avgContext sql.NullFloat64

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.Total, &successCount, &avgTokens, &avgSpec, &avgContext,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	if successCount.Valid {
		stats.SuccessCount = int(successCount.Int64)
	}
	stats.AvgTokensUsed = avgTokens.Float64
	stats.AvgSpecificityScore = avgSpec.Float64
	stats.AvgContextScore = avgContext.Float64

	if stats.Total > 0 {
		stats.SuccessRate = float64(stats.SuccessCount) / float64(stats.Total)
	}

	return &stats, nil
}

// AgentStats represents aggregate statistics.
type AgentStats struct {
	Total               int
	SuccessCount        int
	SuccessRate         float64
	AvgTokensUsed       float64
	AvgSpecificityScore float64
	AvgContextScore     float64
}

// Helper functions

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(i int) bool {
	return i != 0
}
