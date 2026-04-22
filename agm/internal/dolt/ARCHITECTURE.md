# AGM Dolt Storage - Architecture

**Version**: 1.0
**Status**: Phase 1 Complete
**Last Updated**: 2026-03-08

## System Overview

The Dolt storage layer provides Git-like database capabilities for AGM session management, replacing the previous dual-layer YAML manifest + JSONL storage system.

### High-Level Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                     AGM CLI Commands                          │
│  (agm session create/list/get/update/delete)                 │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────────────┐
│                  Storage Adapter Layer                        │
│                   (internal/dolt/)                            │
│                                                               │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐ │
│  │  Adapter.go    │  │  Sessions.go   │  │  Messages.go   │ │
│  │                │  │                │  │                │ │
│  │ • Config       │  │ • CreateSession│  │ • CreateMessage│ │
│  │ • Connection   │  │ • GetSession   │  │ • GetMessages  │ │
│  │ • Migrations   │  │ • UpdateSession│  │ • BatchInsert  │ │
│  └────────────────┘  └────────────────┘  └────────────────┘ │
│                                                               │
│  ┌────────────────┐  ┌────────────────┐                     │
│  │  ToolCalls.go  │  │  Migrations.go │                     │
│  │                │  │                │                     │
│  │ • CreateToolCall│ │ • Registry     │                     │
│  │ • GetStats     │  │ • Apply        │                     │
│  │ • Analytics    │  │ • Validate     │                     │
│  └────────────────┘  └────────────────┘                     │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────────────┐
│             MySQL Driver (go-sql-driver/mysql)                │
└───────────────────────┬──────────────────────────────────────┘
                        │ MySQL Protocol (port 3307)
                        ▼
┌──────────────────────────────────────────────────────────────┐
│                     Dolt SQL Server                           │
│                                                               │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐ │
│  │  OSS Database  │  │ Acme Corp Database│  │  Test Database │ │
│  │  (port 3307)   │  │  (port 3307)   │  │  (port 3307)   │ │
│  │                │  │                │  │                │ │
│  │ agm_sessions   │  │ agm_sessions   │  │ agm_sessions   │ │
│  │ agm_messages   │  │ agm_messages   │  │ agm_messages   │ │
│  │ agm_tool_calls │  │ agm_tool_calls │  │ agm_tool_calls │ │
│  └────────────────┘  └────────────────┘  └────────────────┘ │
└───────────────────────┬──────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────────────┐
│                   Dolt Storage Layer                          │
│                                                               │
│  • Git-like commit history                                   │
│  • Branch/merge capabilities                                 │
│  • Version control for data                                  │
│  • Filesystem: ~/.dolt/dolt-db/                  │
└──────────────────────────────────────────────────────────────┘
```

## Component Design

### 1. Adapter Layer (`adapter.go`)

**Responsibility**: Database connection management and configuration

**Key Components**:
```go
type Adapter struct {
    conn         *sql.DB      // MySQL connection to Dolt
    workspace    string       // Workspace name (oss/acme)
    port         string       // Dolt server port
    migrationsApplied bool    // Migration state
}

type Config struct {
    Workspace string  // Workspace isolation
    Port      string  // Server port (default: 3307)
    Host      string  // Server host (default: 127.0.0.1)
    Database  string  // Database name (default: workspace name)
    User      string  // DB user (default: root)
    Password  string  // DB password (default: empty)
}
```

**Design Decisions**:
- **Single connection per adapter**: Simplifies lifecycle management
- **Workspace-aware configuration**: Database name defaults to workspace name
- **Lazy migration**: Migrations applied on first operation, not construction
- **Environment-driven config**: Uses WORKSPACE, DOLT_PORT env vars

### 2. Session Operations (`sessions.go`)

**Responsibility**: Session CRUD operations

**Key Functions**:
```go
CreateSession(session *manifest.Manifest) error
GetSession(sessionID string) (*manifest.Manifest, error)
UpdateSession(session *manifest.Manifest) error
DeleteSession(sessionID string) error
ListSessions(filter *SessionFilter) ([]*manifest.Manifest, error)
```

**Design Patterns**:
- **Manifest compatibility**: Uses existing `manifest.Manifest` struct
- **JSON encoding**: Tags and metadata stored as JSON columns
- **Timestamp handling**: parseTime=true in DSN for automatic conversion
- **Workspace filtering**: All queries include workspace WHERE clause

**Schema Mapping**:
```
Manifest Field          → Database Column
--------------            ---------------
SessionID               → id (VARCHAR 255 PRIMARY KEY)
Name                    → name (VARCHAR 255)
Agent                   → agent (VARCHAR 100)
CreatedAt               → created_at (TIMESTAMP)
UpdatedAt               → updated_at (TIMESTAMP)
Status                  → status (VARCHAR 20)
Context.Project         → context_project (TEXT)
Context.Purpose         → context_purpose (TEXT)
Context.Tags            → context_tags (JSON)
Claude.UUID             → claude_uuid (VARCHAR 255)
Tmux.SessionName        → tmux_session_name (VARCHAR 255)
```

### 3. Message Operations (`messages.go`)

**Responsibility**: Conversation message storage

**Key Functions**:
```go
CreateMessage(msg *Message) error
CreateMessages(msgs []*Message) error  // Batch insert
GetSessionMessages(sessionID string) ([]*Message, error)
```

**Message Structure**:
```go
type Message struct {
    ID             string  // UUID
    SessionID      string  // Foreign key
    Role           string  // user/assistant
    Content        string  // JSON array of content blocks
    SequenceNumber int     // Message ordering
    Timestamp      int64   // Unix timestamp (ms)
    TokensInput    int     // Token counts
    TokensOutput   int
    ModelName      string
}
```

**Optimizations**:
- **Batch insert**: `CreateMessages()` uses single transaction for migration
- **JSON content**: Stores Claude API format directly
- **Sequence ordering**: Explicit sequence number for reliable message order
- **Indexed queries**: Foreign key indexes for fast session lookup

### 4. Tool Call Tracking (`tool_calls.go`)

**Responsibility**: Record and analyze tool usage

**Key Functions**:
```go
CreateToolCall(call *ToolCall) error
GetToolCall(id string) (*ToolCall, error)
GetMessageToolCalls(messageID string) ([]*ToolCall, error)
GetSessionToolCalls(sessionID string) ([]*ToolCall, error)
```

**ToolCall Structure**:
```go
type ToolCall struct {
    ID              string
    MessageID       string               // FK to message
    SessionID       string               // FK to session
    ToolName        string               // e.g., "read_file"
    Arguments       map[string]any       // JSON
    Result          map[string]any       // JSON
    Error           string               // Error message if failed
    Timestamp       int64                // Unix timestamp (ms)
    ExecutionTimeMs int                  // Performance tracking
}
```

**Use Cases**:
- Performance analysis (which tools are slow?)
- Usage patterns (most common tools)
- Error tracking (which tools fail often?)
- Session debugging (what tools were used?)

### 5. Migration System (`migrations.go`)

**Responsibility**: Schema evolution and version management

**Migration Registry**:
```go
type Migration struct {
    Version       int           // Sequential version number
    Name          string        // Descriptive name
    SQL           string        // Embedded SQL content
    Checksum      string        // SHA256 hash
    TablesCreated []string      // For tracking
}
```

**Migration Flow**:
```
1. ensureMigrationRegistry()
   ↓ Create agm_migrations table if not exists

2. getAppliedMigrations()
   ↓ SELECT version FROM agm_migrations WHERE component='agm'

3. For each pending migration:
   ↓ BEGIN TRANSACTION
   ↓ Execute migration SQL
   ↓ Record in agm_migrations (version, name, checksum)
   ↓ COMMIT

4. Validate checksums
   ↓ Compare stored checksum with current SQL file
   ↓ Error if mismatch (SQL modified after apply)
```

**Safety Features**:
- **Idempotent**: Safe to run multiple times
- **Transactional**: All-or-nothing execution
- **Checksum validation**: Detects SQL modifications
- **Component isolation**: Multi-component support via prefix

## Data Flow

### Session Creation Flow

```
1. CLI: agm session create "My Session"
   ↓
2. Adapter.CreateSession(manifest)
   ↓
3. Generate UUID if not set
   ↓
4. INSERT INTO agm_sessions (id, name, workspace, ...)
   VALUES (?, ?, ?, ...)
   ↓
5. Return session ID to user
```

### Session Listing Flow

```
1. CLI: agm session list --harness claude-code --limit 10
   ↓
2. Adapter.ListSessions(&SessionFilter{Agent: "claude", Limit: 10})
   ↓
3. SELECT * FROM agm_sessions
   WHERE workspace = ? AND agent = ?
   ORDER BY created_at DESC LIMIT ?
   ↓
4. Convert rows to manifest.Manifest structs
   ↓
5. Return session list to CLI
```

### Message Storage Flow

```
1. Conversation message generated
   ↓
2. Adapter.CreateMessage(msg)
   ↓
3. Generate message UUID
   ↓
4. INSERT INTO agm_messages (id, session_id, role, content, ...)
   VALUES (?, ?, ?, ?, ...)
   ↓
5. Message stored with sequence number
```

## Workspace Isolation

### Design Goal

**ZERO data leakage between workspaces**. A developer must never see professional work in OSS workspace or vice versa.

### Implementation Strategy

**Layer 1: Separate Databases** (Primary Defense)
```
oss workspace    → database "oss"    (port 3307)
acme workspace → database "acme" (port 3307)
test workspace   → database "test"   (port 3307)
```

**Layer 2: Workspace Column** (Defense in Depth)
```sql
CREATE TABLE agm_sessions (
    id VARCHAR(255) PRIMARY KEY,
    workspace VARCHAR(255) NOT NULL,  -- Always set, always filtered
    ...
);
```

**Layer 3: Filtered Queries** (Runtime Enforcement)
```go
// All queries include workspace filter
query := `SELECT * FROM agm_sessions WHERE workspace = ?`
```

### Validation

**TestWorkspaceIsolation** (8 subtests):
1. Workspace names correctly set
2. Sessions created in separate databases
3. GetSession() returns only own workspace data
4. Messages isolated per workspace
5. ListSessions() returns only own workspace
6. Tool calls isolated per workspace
7. Updates don't affect other workspaces
8. Deletes don't affect other workspaces

**Result**: All 8/8 tests pass, confirming zero leakage.

## Configuration Management

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| WORKSPACE | (required) | Workspace name (oss/acme/test) |
| DOLT_PORT | 3307 | Dolt server port |
| DOLT_HOST | 127.0.0.1 | Dolt server host |
| DOLT_DATABASE | {WORKSPACE} | Database name (defaults to workspace) |
| DOLT_USER | root | Database user |
| DOLT_PASSWORD | "" | Database password |

### Configuration Precedence

1. Explicit `Config` struct (highest priority)
2. Environment variables (`DOLT_*`)
3. Default values (fallback)

### DSN Construction

```go
// Format: user:password@tcp(host:port)/database?parseTime=true
dsn := "root:@tcp(127.0.0.1:3307)/oss?parseTime=true"
```

**parseTime=true**: Automatically converts TIMESTAMP columns to `time.Time`

## Migration Architecture

### Migration File Organization

```
internal/dolt/migrations/
├── 001_initial_schema.sql      # Sessions table
├── 002_messages_table.sql      # Messages table
├── 003_add_tool_calls.sql      # Tool calls table
├── 004_add_session_tags.sql    # Tags table (future)
├── 005_add_message_embeddings.sql  # Embeddings (future)
└── 006_add_performance_indexes.sql # Indexes
```

### Embedded Migration System

**go:embed Directive**:
```go
//go:embed migrations/001_initial_schema.sql
var migration001 string

//go:embed migrations/002_messages_table.sql
var migration002 string
// ...
```

**Benefits**:
- Migrations embedded in binary (no external files)
- Version control ensures consistency
- Cannot be modified at runtime

### Migration Registry Schema

```sql
CREATE TABLE agm_migrations (
    id INT AUTO_INCREMENT PRIMARY KEY,
    component VARCHAR(255) NOT NULL,     -- 'agm', 'corpus-callosum', etc.
    version INT NOT NULL,                -- Sequential version number
    name VARCHAR(255) NOT NULL,          -- Migration name
    checksum VARCHAR(128) NOT NULL,      -- SHA256 hash (64 hex chars)
    applied_at TIMESTAMP,                -- When applied
    applied_by VARCHAR(255),             -- Who/what applied it
    execution_time_ms INT,               -- Performance tracking
    tables_created JSON,                 -- Audit trail
    UNIQUE KEY (component, version)
);
```

## Error Handling

### Connection Failures

```go
// Retry logic with exponential backoff
func (a *Adapter) reconnect() error {
    for attempt := 1; attempt <= 3; attempt++ {
        if err := a.conn.Ping(); err == nil {
            return nil
        }
        time.Sleep(time.Duration(attempt) * time.Second)
    }
    return fmt.Errorf("failed to connect after 3 attempts")
}
```

### Transaction Rollback

```go
tx, err := a.conn.Begin()
defer tx.Rollback()  // Rollback if not committed

// ... operations ...

if err := tx.Commit(); err != nil {
    return fmt.Errorf("transaction failed: %w", err)
}
```

### Migration Errors

**Checksum Mismatch**:
```
Error: migration 2 checksum mismatch
  stored:   3be8628fddeb855d333ecc65f3302b9875926d663f6ace0731884d9da724a80b
  expected: 8184630b6383da6be728aaaf3a979bd4c83b1cf7c7f8cd4b92af768dc5198058
```

**Resolution**: Don't modify existing migrations. Create new migration instead.

## Performance Considerations

### Query Optimization

**Indexes**:
```sql
-- Session lookups
INDEX idx_workspace (workspace)
INDEX idx_created_at (created_at)
INDEX idx_status (status)

-- Message queries
INDEX idx_session_id (session_id)
INDEX idx_timestamp (timestamp)

-- Tool call analytics
INDEX idx_tool_name (tool_name)
INDEX idx_execution_time (execution_time_ms)
```

**Composite Indexes** (Migration 006):
```sql
INDEX idx_status_created (status, created_at)
INDEX idx_workspace_status (workspace, status)
```

### Batch Operations

**Batch Insert** (messages):
```go
// Instead of N separate INSERT statements
for _, msg := range messages {
    adapter.CreateMessage(msg)  // BAD: N round-trips
}

// Use single transaction
adapter.CreateMessages(messages)  // GOOD: 1 round-trip
```

### Connection Pooling

```go
db.SetMaxOpenConns(25)       // Max concurrent connections
db.SetMaxIdleConns(5)        // Keep 5 idle for reuse
db.SetConnMaxLifetime(5 * time.Minute)
```

## Testing Architecture

### Test Database Isolation

**Separate test database**: `test` (not `oss` or `acme`)
**Environment**: `DOLT_TEST_INTEGRATION=1`
**Cleanup**: Tests drop all tables after completion

### Test Fixtures

```go
func getTestAdapter(t *testing.T) *Adapter {
    os.Setenv("WORKSPACE", "test")
    os.Setenv("DOLT_PORT", "3307")
    os.Unsetenv("DOLT_DATABASE")  // Force workspace name

    config, _ := DefaultConfig()
    adapter, _ := New(config)
    adapter.ApplyMigrations()

    return adapter
}
```

### Integration Test Pattern

```go
func TestSessionCRUD(t *testing.T) {
    adapter := getTestAdapter(t)
    defer adapter.Close()

    // Create
    session := &manifest.Manifest{...}
    err := adapter.CreateSession(session)
    assert.NoError(t, err)

    // Read
    retrieved, err := adapter.GetSession(session.SessionID)
    assert.NoError(t, err)
    assert.Equal(t, session.Name, retrieved.Name)

    // Update
    session.Name = "Updated"
    err = adapter.UpdateSession(session)
    assert.NoError(t, err)

    // Delete
    err = adapter.DeleteSession(session.SessionID)
    assert.NoError(t, err)
}
```

## Security Architecture

### SQL Injection Prevention

**Parameterized Queries** (exclusive use):
```go
// SAFE
query := "SELECT * FROM agm_sessions WHERE id = ?"
rows, err := db.Query(query, sessionID)

// NEVER USED
query := fmt.Sprintf("SELECT * FROM agm_sessions WHERE id = '%s'", sessionID)  // UNSAFE
```

### Access Control

**Database Level**:
- Single user (root) with no password
- Acceptable for single-user local system
- No network exposure (127.0.0.1 only)

**Application Level**:
- No user authentication (single user system)
- Workspace isolation via database name
- No authorization checks (trusted local access)

### Data Privacy

**Encryption**:
- No encryption at rest (local filesystem trust)
- No encryption in transit (localhost only)
- Acceptable trade-off for single-user system

**Access**:
- Database files: chmod 700 (owner only)
- Dolt server: localhost binding only
- No remote access

## Future Enhancements

### Phase 2: CLI Integration (Partially Complete - March 2026)
- ✅ `agm session list` command integrated with Dolt backend
- ✅ Storage helper (`cmd/agm/storage.go`) provides Dolt adapter instances
- ✅ Legacy YAML backend deprecated (`agm session list-yaml`)
- ⏳ Storage backend abstraction (interface) - pending
- ⏳ Configuration option: `storage.backend: dolt` - pending
- ⏳ Remaining commands (`new`, `resume`, `archive`, `delete`) - pending

### Phase 3: Advanced Features
- Semantic search via message embeddings
- Tool usage analytics dashboard
- Session tagging and categorization
- Cross-session knowledge graph

### Phase 4: Multi-User (Future)
- Authentication and authorization
- User-level workspace isolation
- Role-based access control
- Encryption at rest

## References

- **Specification**: [SPEC.md](./SPEC.md)
- **ADRs**: [adr/](./adr/)
- **Dolt Documentation**: https://docs.dolthub.com/
- **Go SQL Driver**: https://github.com/go-sql-driver/mysql
