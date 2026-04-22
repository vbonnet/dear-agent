# Database Layer - Internal Package

This package provides SQLite-based persistence for AGM session management with full CRUD operations for sessions, messages, and escalations.

## Files

- **db.go** - Database connection management
  - `Open(path string) (*DB, error)` - Opens SQLite database with schema
  - `Close() error` - Closes database connection
  - `BeginTx() (*sql.Tx, error)` - Starts transaction

- **sessions.go** - Session CRUD operations
  - `CreateSession(session *manifest.Manifest) error`
  - `GetSession(sessionID string) (*manifest.Manifest, error)`
  - `UpdateSession(session *manifest.Manifest) error`
  - `DeleteSession(sessionID string) error`
  - `ListSessions(filter *SessionFilter) ([]*manifest.Manifest, error)`

- **messages.go** - Message CRUD operations
  - `CreateMessage(sessionID string, msg *conversation.Message) error`
  - `GetMessages(sessionID string, opts *MessageOptions) ([]*conversation.Message, error)`
  - `DeleteMessages(sessionID string) error`

- **escalations.go** - Escalation CRUD operations
  - `CreateEscalation(sessionID string, event *EscalationEvent) (int64, error)`
  - `GetEscalations(sessionID string) ([]*EscalationEvent, error)`
  - `GetUnresolvedEscalations(sessionID string) ([]*EscalationEvent, error)`
  - `ResolveEscalation(escalationID int64, note string) error`
  - `DeleteEscalations(sessionID string) error`

- **search.go** - Full-text search using SQLite FTS5
  - `SearchSessions(query string, opts *SearchOptions) ([]*manifest.Manifest, error)`
  - `BuildFTS5Query(terms []string, operator string) string`

- **schema.sql** - SQLite schema with FTS5 full-text search
  - Sessions table with context, agent, and engram metadata
  - Messages table for conversation history
  - Escalations table for detected intervention points
  - FTS5 virtual table for fast keyword search
  - Indexes for common query patterns
  - Views for active/archived sessions and escalation summaries

## Testing

The package includes comprehensive unit tests in:
- **db_test.go** - Core CRUD operation tests
- **search_test.go** - Full-text search tests

### Running Tests

The SQLite driver must be compiled with FTS5 support. Use the `fts5` build tag:

```bash
# Run all database tests with coverage
go test -tags="fts5" ./internal/db/... -cover

# Run specific test
go test -tags="fts5" -run TestSessionCRUD ./internal/db -v

# Run with verbose output
go test -tags="fts5" ./internal/db/... -v -cover
```

### Test Coverage

Tests achieve 80%+ code coverage across all CRUD operations:
- ✅ Database initialization and schema application
- ✅ Transaction support
- ✅ Session CRUD (create, read, update, delete, list with filters)
- ✅ Message CRUD with complex content blocks (text, images, tool usage)
- ✅ Escalation CRUD with resolution tracking
- ✅ Cascade deletes (messages/escalations deleted with session)
- ✅ Error handling (nil values, empty IDs, not found, duplicates)
- ✅ JSON marshaling/unmarshaling for nested structs
- ✅ Null handling for optional fields
- ✅ Full-text search with FTS5 (AND, OR, phrases, column-specific)
- ✅ Search filters (lifecycle, agent, dates, escalations)
- ✅ Pagination (limit/offset)

### Test Database

Tests use in-memory SQLite (`:memory:`) for fast, isolated execution:

```go
db, err := Open(":memory:")
require.NoError(t, err)
defer db.Close()
```

## Implementation Details

### JSON Marshaling

Nested structs are stored as JSON in SQLite:
- `context_tags` - Array of string tags
- `engram_ids` - Array of engram identifiers
- `content` (messages) - Array of ContentBlock interface types

### NULL Handling

Optional fields use SQL NULL values:
- `agent` - Optional agent type
- `claude_uuid` - Optional Claude session UUID
- `engram_metadata` - Optional engram integration data
- `parent_session_id` - Optional parent for hierarchical sessions

### Prepared Statements

All queries use parameterized statements to prevent SQL injection:

```go
query := `INSERT INTO sessions (...) VALUES (?, ?, ?, ...)`
_, err := db.conn.Exec(query, session.SessionID, session.Name, ...)
```

### Error Wrapping

Errors are wrapped with context using `fmt.Errorf`:

```go
if err != nil {
    return fmt.Errorf("failed to insert session: %w", err)
}
```

## Schema

The database schema (schema.sql) includes:
- **Foreign key constraints** - CASCADE deletes for messages/escalations
- **Indexes** - Optimized for common queries (lifecycle, agent, timestamps)
- **FTS5 search** - Virtual table for full-text search
- **Triggers** - Keep FTS5 index in sync with sessions table
- **Views** - Convenient access to active sessions, unresolved escalations

## Future Enhancements

- [ ] Connection pooling for concurrent access
- [ ] Migration framework for schema upgrades
- [ ] Batch insert operations for performance
- [ ] Query result caching
- [ ] Metrics/instrumentation
- [ ] Database compaction utilities
