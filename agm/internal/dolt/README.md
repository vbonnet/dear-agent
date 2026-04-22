# AGM Dolt Storage Implementation

**Status: Phase 1 Complete - Data Migrated ✅**

This package implements Dolt-based storage for AGM (Agent-Generated Messaging) sessions, replacing the previous YAML manifest storage system.

## Current State (2026-03-08)

- ✅ **Adapter implemented**: Full Dolt storage adapter with CRUD operations
- ✅ **Migration system**: 6 migrations for schema evolution
- ✅ **Testing**: Unit and integration tests passing (8/8)
- ✅ **Data migrated**: 116 sessions migrated from YAML to Dolt
- ✅ **Migration tool**: Built and tested (`cmd/agm-migrate-dolt/`)
- ✅ **Documentation**: SPEC.md, ARCHITECTURE.md, ADRs complete
- ⏳ **AGM daemon integration**: Pending (Phase 2)
- ⏳ **Production use**: Backend ready, CLI integration pending

**See main README**: `main/agm/README.md` (Storage Backend section)

## Overview

**Current Storage**: AGM uses YAML manifest files (`~/.agm/sessions/*/manifest.yaml`) for session metadata and relies on Claude CLI's `~/.claude/history.jsonl` for conversation history.

**Future Storage**: Dolt provides a Git-like database where:
- Database operations are atomic and transactional
- History is preserved automatically (every change is a commit)
- Advanced SQL queries enable analytics and semantic search
- Workspace isolation prevents cross-contamination between OSS and professional work

## Architecture

### Per-Workspace Databases

Each workspace has its own isolated Dolt instance:

```
~/.dolt/          # OSS workspace database (port 3307)
~/src/ws/acme/.dolt/       # Acme Corp workspace database (port 3308)
```

### Table Schema

All AGM tables use the `agm_` prefix for namespace isolation:

- **agm_sessions**: Session metadata (replaces SQLite sessions table)
- **agm_messages**: Conversation messages (replaces JSONL files)
- **agm_tool_calls**: Tool usage tracking (new feature)
- **agm_session_tags**: Session categorization (future)
- **agm_message_embeddings**: Semantic search support (future)
- **agm_worktrees**: Git worktree lifecycle tracking (session, path, branch, created/removed timestamps)

### Migration System

Migrations are tracked in the `agm_migrations` registry table:

1. **Migration 001**: Initial schema (sessions table)
2. **Migration 002**: Messages table
3. **Migration 003**: Add tool calls tracking
4. **Migration 004**: Add session tags
5. **Migration 005**: Add message embeddings
6. **Migration 006**: Performance optimization indexes
7. **Migration 011**: Add `agm_worktrees` table for worktree lifecycle tracking

## Usage

### Basic Setup

```go
import "github.com/vbonnet/ai-tools/agm/internal/dolt"

// Connect to Dolt (uses WORKSPACE and DOLT_PORT env vars)
config, err := dolt.DefaultConfig()
adapter, err := dolt.New(config)
defer adapter.Close()

// Migrations are applied automatically
err = adapter.ApplyMigrations()
```

### Session Operations

```go
// Create session
session := &manifest.Manifest{
    SessionID: "session-123",
    Name:      "My Session",
    // ... other fields
}
err = adapter.CreateSession(session)

// Get session
session, err = adapter.GetSession("session-123")

// Update session
session.Name = "Updated Name"
err = adapter.UpdateSession(session)

// List sessions
sessions, err = adapter.ListSessions(&dolt.SessionFilter{
    Agent: "claude",
    Limit: 10,
})

// Delete session
err = adapter.DeleteSession("session-123")
```

### Message Operations

```go
// Create single message
msg := &dolt.Message{
    SessionID:      "session-123",
    Role:           "user",
    Content:        `[{"type":"text","text":"Hello!"}]`,
    SequenceNumber: 0,
}
err = adapter.CreateMessage(msg)

// Batch create messages (optimized)
messages := []*dolt.Message{...}
err = adapter.CreateMessages(messages)

// Get session messages
messages, err = adapter.GetSessionMessages("session-123")
```

### Tool Call Tracking

```go
// Record tool usage
toolCall := &dolt.ToolCall{
    MessageID:       "msg-456",
    SessionID:       "session-123",
    ToolName:        "read_file",
    Arguments:       map[string]interface{}{"path": "/test.txt"},
    Result:          map[string]interface{}{"content": "..."},
    ExecutionTimeMs: 150,
}
err = adapter.CreateToolCall(toolCall)

// Get tool call statistics
stats, err = adapter.GetToolCallStats("session-123")
// Returns: {"total_calls": 5, "tools": [...]}
```

## Migration from YAML Manifests (Future)

**Note**: This migration process is designed but not yet fully implemented. Current AGM uses YAML manifests, not SQLite.

### Step 1: Set Up Dolt Workspace

```bash
# Navigate to workspace
cd ~/projects/myworkspace

# Initialize Dolt database (if not already done)
dolt init

# Start Dolt server
dolt sql-server --config=.dolt/server.yaml &
```

### Step 2: Run Migration Tool (When Available)

```bash
# The migration tool will need to:
# 1. Read YAML manifests from ~/.agm/sessions/*/manifest.yaml
# 2. Parse conversation history from ~/.claude/history.jsonl
# 3. Insert into Dolt tables (agm_sessions, agm_messages)

# Expected usage (not yet implemented):
go run cmd/agm-migrate-dolt/main.go \
  --sessions-dir ~/.agm/sessions \
  --history-file ~/.claude/history.jsonl \
  --dry-run \
  --verbose

# Actual migration (after testing)
go run cmd/agm-migrate-dolt/main.go \
  --sessions-dir ~/.agm/sessions \
  --history-file ~/.claude/history.jsonl
```

### Step 3: Verify Migration

```bash
# Connect to Dolt
dolt sql

# Verify session count
SELECT COUNT(*) FROM agm_sessions;

# Verify message count
SELECT COUNT(*) FROM agm_messages;

# Check migration registry
SELECT * FROM dolt_migrations WHERE component='agm';

# Compare with YAML count
find ~/.agm/sessions -name manifest.yaml | wc -l
```

## Testing

### Unit Tests

```bash
# Run unit tests (no Dolt server required)
go test -v ./internal/dolt -run TestNew
go test -v ./internal/dolt -run TestDefaultConfig
```

### Integration Tests

Requires running Dolt server on port 3307:

```bash
# Start Dolt server
cd ./.dolt
dolt sql-server --config=server.yaml &

# Run integration tests
DOLT_TEST_INTEGRATION=1 WORKSPACE=test DOLT_PORT=3307 \
  go test -v ./internal/dolt
```

### Performance Benchmarks

Target: <10ms query latency (acceptable vs SQLite ~1ms)

```bash
# Run performance tests
go test -v ./internal/dolt -bench=. -benchtime=10s
```

## Configuration

### Environment Variables

- `WORKSPACE`: Workspace name (e.g., "oss", "acme")
- `DOLT_PORT`: Dolt server port (default: "3307")
- `DOLT_HOST`: Dolt server host (default: "127.0.0.1")
- `DOLT_DATABASE`: Database name (default: "workspace")
- `DOLT_USER`: Database user (default: "root")
- `DOLT_PASSWORD`: Database password (default: "")

### Dolt Server Configuration

Example `~/.dolt/server.yaml`:

```yaml
log_level: info

user:
  name: root
  password: ""

listener:
  host: 127.0.0.1
  port: 3307
  max_connections: 100

databases:
  - name: workspace
    path: ~/.dolt/dolt-db

behavior:
  autocommit: true
  read_only: false

performance:
  query_parallelism: 4
```

## Integration Roadmap

**What's needed to complete Dolt integration:**

1. **Migration Tool Implementation**
   - Build/implement `cmd/agm-migrate-dolt/` tool
   - Add YAML manifest parser
   - Add history.jsonl conversation importer
   - Add dry-run and validation modes

2. **CLI Integration**
   - Add storage backend selection to config (`storage.backend: yaml|dolt`)
   - Update all AGM commands to support Dolt adapter
   - Implement storage abstraction layer (interface for YAML vs Dolt)
   - Add fallback/migration detection logic

3. **Testing & Validation**
   - End-to-end tests with Dolt backend
   - Performance benchmarks vs YAML
   - Migration validation (data integrity checks)
   - Rollback procedures

4. **Documentation**
   - Migration guide for users
   - Troubleshooting for Dolt-specific issues
   - Performance tuning recommendations

5. **Deployment**
   - Dolt server automation (systemd service)
   - Backup/restore procedures
   - Monitoring and alerting

**Current Status**: Step 1 (adapter implementation) is complete. Steps 2-5 are pending.

## Benefits

### 1. Corruption Prevention

**Current**: YAML files can be manually edited, risk of syntax errors
**Future**: Database operations are atomic and transactional

### 2. Workspace Isolation

**Current**: Logical isolation via directory structure
**Future**: Physical isolation per workspace (OSS vs Acme) with separate databases

### 3. Version History

**Current**: Git history of YAML files (requires manual commits)
**Future**: Git-like database history, automatic commits for every change

### 4. Performance

**Expected**: ~3.5x slower than direct file access (3-5ms vs <1ms)
**Acceptable**: Still well within ms-level tolerance (not seconds)

### 5. Future Features

- Semantic search via message embeddings
- Session tagging and categorization
- Tool usage analytics
- Cross-component data aggregation (via Corpus Callosum)
- Advanced SQL queries for session insights

## Troubleshooting

### Connection Failed

```bash
# Check if Dolt server is running
ps aux | grep dolt

# Start Dolt server
cd ./.dolt
dolt sql-server --config=server.yaml &

# Verify connection
dolt sql -q "SELECT 1"
```

### Migration Checksum Mismatch

```
Error: migration 1 checksum mismatch
```

This means the migration SQL file was modified after being applied. Solutions:

1. **Rollback and re-apply** (development only):
   ```bash
   dolt checkout HEAD~1  # Go back before migration
   # Re-run migration with updated SQL
   ```

2. **Create new migration** (production):
   ```bash
   # Don't modify existing migrations
   # Create migration 006 with the changes
   ```

### Performance Degradation

If queries exceed 10ms threshold:

1. **Check indexes**: `EXPLAIN SELECT ...`
2. **Add composite indexes** (migration 005)
3. **Enable query parallelism** (server.yaml)
4. **Consider connection pooling**

## References

- **Specification**: `specs/dolt-storage.md`
- **Migration Examples**: `examples/dolt-agm-migrations.sql`
- **Dolt Documentation**: https://docs.dolthub.com/
- **Task**: Phase 3, Task 3.3 - AGM Migration to Dolt Storage
