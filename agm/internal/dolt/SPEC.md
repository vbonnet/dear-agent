# AGM Dolt Storage - Specification

**Version**: 2.0
**Status**: Phase 6 Complete - Dolt-Only Architecture (YAML Backend Removed)
**Last Updated**: 2026-03-18

## Overview

This specification defines the Dolt-based storage implementation for AGM (Agent-Generated Messaging) sessions. Dolt provides a Git-like SQL database that replaces the previous dual-layer YAML manifest + JSONL storage system.

## Goals

### Primary Goals
1. **Data Integrity**: Prevent session corruption through transactional database operations
2. **Workspace Isolation**: Ensure zero cross-contamination between OSS and professional workspaces
3. **Version History**: Enable rollback to any point in session history
4. **Query Performance**: Maintain acceptable latency (<10ms for typical operations)
5. **Migration Safety**: Zero data loss when migrating from YAML/JSONL storage

### Non-Goals
- Backwards compatibility with YAML storage (breaking migration)
- Multi-user support (single-user system)
- Real-time synchronization across workspaces
- Performance parity with direct file access (acceptable if <10ms)

## Architecture

### Database Layout

Each workspace maintains an isolated Dolt database:

```
Workspace      Database Location                    Port
--------       ------------------                   -----
OSS            ~/.dolt/dolt-db          3307
Acme         ~/src/ws/acme/.dolt/dolt-db       3308
```

### Schema Design

All tables use `agm_` prefix for namespace isolation within shared Dolt instances.

#### Core Tables

**agm_sessions**: Session metadata (replaces YAML manifests)
- Primary key: `id` (session UUID)
- Workspace column ensures isolation
- Tracks session lifecycle (created_at, updated_at, status)
- Stores context (project, purpose, tags, notes)
- References Claude UUID and tmux session

**agm_messages**: Conversation history (replaces JSONL files)
- Primary key: `id` (message UUID)
- Foreign key: `session_id` references agm_sessions
- Sequence number for message ordering
- Content stored as JSON array (Claude API format)
- Tracks role (user/assistant), timestamp, token usage

**agm_tool_calls**: Tool usage tracking (new feature)
- Primary key: `id` (tool call UUID)
- Foreign keys: `message_id`, `session_id`
- Stores tool name, arguments (JSON), results (JSON)
- Tracks execution time for performance analysis
- Enables tool usage analytics

#### Future Tables

**agm_session_tags**: Session categorization (Phase 2)
- Primary key: `id` (tag UUID)
- Foreign key: `session_id`
- Enables flexible tagging system

**agm_message_embeddings**: Semantic search (Phase 3)
- Primary key: `message_id`
- Stores vector embeddings for messages
- Enables similarity search across sessions

### Migration System

**agm_migrations**: Migration registry
- Tracks applied migrations by version number
- Stores checksums to detect SQL file modifications
- Records execution time and tables created
- Ensures idempotent migration application

**component_registry**: Component tracking
- Tracks installed components (agm, corpus-callosum, etc.)
- Enables multi-component database sharing
- Prevents table prefix collisions

#### Migration Files

| Version | File | Description | Tables Created |
|---------|------|-------------|----------------|
| 001 | initial_schema.sql | Sessions table | agm_sessions |
| 002 | messages_table.sql | Messages table | agm_messages |
| 003 | add_tool_calls.sql | Tool tracking | agm_tool_calls |
| 004 | add_session_tags.sql | Tagging system | agm_session_tags |
| 005 | add_message_embeddings.sql | Semantic search | agm_message_embeddings |
| 006 | add_performance_indexes.sql | Query optimization | (indexes only) |

## Functional Requirements

### FR1: Session CRUD Operations

**Create Session**
```go
session := &manifest.Manifest{
    SessionID: uuid.New().String(),
    Name: "Session Name",
    // ... fields
}
err := adapter.CreateSession(session)
```

**Read Session**
```go
session, err := adapter.GetSession(sessionID)
```

**Resolve Session by Identifier** (NEW in v1.2)
```go
// Resolves by session ID, tmux session name, or manifest name
// Excludes archived sessions automatically
session, err := adapter.ResolveIdentifier(identifier)
```

**Update Session**
```go
session.Status = "completed"
err := adapter.UpdateSession(session)
```

**Delete Session**
```go
err := adapter.DeleteSession(sessionID)
```

**List Sessions**
```go
sessions, err := adapter.ListSessions(&SessionFilter{
    Status: "active",
    Workspace: "oss",
    Limit: 50,
})
```

### FR2: Message Operations

**Single Message**
```go
msg := &Message{
    SessionID: sessionID,
    Role: "user",
    Content: `[{"type":"text","text":"Hello"}]`,
}
err := adapter.CreateMessage(msg)
```

**Batch Messages** (optimized for migration)
```go
messages := []*Message{...} // bulk insert
err := adapter.CreateMessages(messages)
```

**Retrieve Messages**
```go
messages, err := adapter.GetSessionMessages(sessionID)
```

### FR3: Tool Call Tracking

**Record Tool Usage**
```go
toolCall := &ToolCall{
    MessageID: msgID,
    SessionID: sessionID,
    ToolName: "read_file",
    Arguments: map[string]any{"path": "/file.txt"},
    Result: map[string]any{"content": "..."},
    ExecutionTimeMs: 150,
}
err := adapter.CreateToolCall(toolCall)
```

**Query Tool Statistics**
```go
stats, err := adapter.GetToolCallStats(sessionID)
// Returns: total calls, tool breakdown, avg execution time
```

### FR4: Workspace Isolation

**Requirement**: Sessions in one workspace MUST NOT be visible to another workspace.

**Implementation**:
- Each workspace uses separate database name (oss → "oss", acme → "acme")
- Workspace column in agm_sessions table (defense in depth)
- All queries filter by workspace automatically

**Validation**: TestWorkspaceIsolation verifies zero data leakage

### FR5: Migration Safety

**Requirements**:
- Dry-run mode to preview migration without changes
- Checksum verification for data integrity
- Verbose logging for troubleshooting
- Support for YAML-only migration (no SQLite dependency)

**Implementation**:
- `--dry-run` flag prevents database writes
- `--verbose` flag shows detailed progress
- `--yaml-only` flag for manifest-based migration
- Session count validation after migration

## Non-Functional Requirements

### NFR1: Performance

**Query Latency Targets**:
- Single session lookup: <5ms (p50), <10ms (p99)
- List sessions (50 records): <10ms (p50), <20ms (p99)
- Message batch insert (100 msgs): <50ms
- Full session with messages: <20ms

**Acceptable Degradation**: 3-5x slower than direct file access is acceptable for reliability benefits.

### NFR2: Data Integrity

**Guarantees**:
- All writes are transactional (ACID compliance)
- Foreign key constraints enforced
- Checksum validation on migration
- No partial writes on failure

**Validation**:
- Integration tests verify CRUD operations
- Migration tool validates session/message counts
- Workspace isolation tests prevent data leakage

### NFR3: Maintainability

**Code Quality**:
- Unit test coverage >80% for core adapter
- Integration tests for all CRUD operations
- Documented migration procedures
- Clear error messages with context

**Documentation**:
- SPEC.md (this file) defines requirements
- ARCHITECTURE.md explains design decisions
- ADR files capture architectural choices
- README.md provides usage examples

### NFR4: Reliability

**Failure Handling**:
- Database connection failures: retry with backoff
- Migration errors: rollback transaction
- Schema mismatches: clear error messages
- Disk space: fail gracefully with warning

**Recovery**:
- Old YAML/JSONL files archived (not deleted)
- Dolt history enables rollback
- Migration can be re-run safely (idempotent)

## Data Migration

### Source Data

**YAML Manifests**: `.agm/sessions/*/manifest.yaml`
- Schema version 2.0
- Fields: session_id, name, agent, created_at, updated_at, etc.
- Context metadata (project, purpose, tags, notes)
- Claude UUID and tmux session references

**JSONL History** (optional, not yet implemented):
- `~/.claude/history.jsonl`
- Conversation messages in Claude API format
- Requires session UUID → Claude UUID mapping

### Migration Process

1. **Backup**: Copy `~/.agm/` to `~/.agm.backup-{timestamp}/`
2. **Dry-run**: `agm-migrate-dolt --dry-run --verbose`
3. **Validate**: Check session counts, no errors
4. **Migrate**: `agm-migrate-dolt --yaml-only`
5. **Verify**: `SELECT COUNT(*) FROM agm_sessions`
6. **Test**: Run AGM commands to confirm functionality

### Validation Criteria

- Session count matches: `find ~/.agm/sessions -name manifest.yaml | wc -l`
- All sessions have workspace: `SELECT COUNT(*) FROM agm_sessions WHERE workspace IS NULL` = 0
- No duplicate IDs: `SELECT id, COUNT(*) FROM agm_sessions GROUP BY id HAVING COUNT(*) > 1` = 0
- Timestamps preserved: Spot-check 5-10 random sessions

## Testing Strategy

### Unit Tests

**Package**: `internal/dolt`
**Files**: `adapter_test.go`, `workspace_isolation_test.go`
**Coverage Target**: >80%

**Test Categories**:
- Configuration parsing (TestNew, TestDefaultConfig)
- DSN construction (TestBuildDSN)
- Session CRUD (TestSessionCRUD)
- Message CRUD (TestMessageCRUD)
- Tool call tracking (TestToolCallTracking)

### Integration Tests

**Requirement**: Running Dolt server on port 3307
**Environment**: `DOLT_TEST_INTEGRATION=1`
**Database**: `test` workspace (isolated from production)

**Test Categories**:
- Workspace isolation (TestWorkspaceIsolation - 8 subtests)
- Edge cases (TestWorkspaceFilterEdgeCases - 3 subtests)
- Data integrity validation
- Performance benchmarks (future)

**Passing Criteria**: 8/8 tests must pass with NO EXCEPTIONS

### Migration Testing

**Tool**: `cmd/agm-migrate-dolt`
**Test Data**: Real user sessions from `~/.agm/`

**Validation**:
- Dry-run produces no errors
- Session count matches source
- Spot-check 10 random sessions for accuracy
- No data corruption detected

## Security Considerations

### Workspace Isolation

**Threat**: Cross-contamination between OSS and professional work
**Mitigation**: Separate databases, workspace column, filtered queries
**Validation**: TestWorkspaceIsolation verifies zero leakage

### Data Privacy

**Threat**: Sensitive conversation data exposure
**Mitigation**: Database files are user-accessible only (chmod 700)
**Limitation**: No encryption at rest (acceptable for single-user system)

### Injection Attacks

**Threat**: SQL injection through user input
**Mitigation**: Parameterized queries exclusively, no string concatenation
**Validation**: All queries use `?` placeholders

## Success Criteria

### Phase 1: Dolt Setup (Complete - March 8, 2026)
- ✅ Dolt database initialized and running (port 3307)
- ✅ 7 migrations applied successfully
- ✅ Migration tool built (`cmd/agm-migrate-dolt/`)
- ✅ All integration tests passing
- ✅ Workspace isolation verified
- ✅ Documentation created (SPEC, ARCHITECTURE, ADRs)

### Phase 2: Data Migration (Complete - March 8, 2026)
- ✅ 40 sessions migrated from YAML to Dolt (100% success rate)
- ✅ Session/message counts validated
- ✅ Zero data loss verified
- ✅ Original YAML manifests preserved for rollback

### Phase 3: Documentation (Complete - March 8, 2026)
- ✅ MIGRATION-REPORT.md created (8.2KB)
- ✅ Server startup automation documented
- ✅ Troubleshooting guide included
- ✅ SWARM-RETROSPECTIVE.md comprehensive

### Phase 4: CLI Integration (Complete - March 9, 2026)
- ✅ `agm session list` command uses Dolt backend
- ✅ Environment variable issue fixed (DOLT_DATABASE)
- ✅ Archived session display support added
- ✅ All 40/40 sessions visible (was 1/40 before fix)
- ✅ Integration tests passing (workspace isolation)
- ✅ Legacy YAML backend deprecated (`list-yaml`)

### Phase 5: Archive Command Migration (Complete - March 12, 2026)
- ✅ `ResolveIdentifier()` method added to Dolt adapter
- ✅ `agm session archive` migrated to use Dolt storage
- ✅ Archive by session ID, tmux name, or manifest name supported
- ✅ Archived sessions excluded from identifier resolution
- ✅ Unit tests added (TestResolveIdentifier, TestResolveIdentifierExcludesArchived, TestResolveIdentifierWithDuplicateNames)
- ✅ Integration tests added (5 Dolt-based archive scenarios)
- ✅ Comprehensive documentation created (ARCHIVE-DOLT-MIGRATION.md, BUILD-AND-VERIFY.md, testing runbooks)

### Phase 6: YAML Backend Removal (Complete - March 18, 2026)
- ✅ Complete command migration (`resume`, `kill`, `unarchive`, `new`, `archive`, `list`)
- ✅ Removed all YAML backend code (9 files, ~1,200 lines deleted)
  - internal/manifest/read.go, write.go, lock.go, migrate.go, unified_storage.go
  - cmd/agm/list.go, migrate.go, migrate_tmux.go, migrate_workspace.go
- ✅ MCP server cache layer migrated to Dolt
- ✅ Fixed SQL schema compatibility (removed parent_session_id from queries)
- ✅ Test infrastructure migrated (27/27 critical tests passing)
- ✅ Zero YAML file dependencies in production code
- ✅ Dolt-only architecture achieved

### Phase 7 (Future)
- ⏳ Message embeddings for semantic search
- ⏳ Tool usage analytics dashboard
- ⏳ Multi-workspace Dolt deployment (Acme workspace)
- ⏳ Performance optimization (<5ms p50 latency target)
- ⏳ Session tagging implementation

## References

- **Architecture**: [ARCHITECTURE.md](./ARCHITECTURE.md)
- **ADRs**: [adr/](./adr/)
- **README**: [README.md](./README.md)
- **Dolt Docs**: https://docs.dolthub.com/
- **Roadmap**: `swarm/agm-dolt-storage-migration/ROADMAP.md`
