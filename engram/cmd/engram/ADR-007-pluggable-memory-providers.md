# ADR-007: Pluggable Memory Provider Architecture

**Status**: Accepted

**Date**: 2024-02-15

**Context**: AI agents need to store and retrieve various types of memory:
- **Working Context**: Active session data (short-term)
- **Session History**: Recent session logs (medium-term)
- **Long-term Memory**: Persistent patterns and learnings
- **Artifacts**: Generated files and outputs

Different deployment scenarios require different storage backends:
- **Local development**: File-based storage
- **Team environments**: Shared database (SQLite, Postgres)
- **Production systems**: Scalable storage (Redis, S3)

Hard-coding a single storage mechanism limits flexibility and future extensibility.

**Decision**: Implement a pluggable Provider interface for memory storage with multiple backend implementations.

**Provider Interface**:

```go
type Provider interface {
    // Store a new memory entry
    Store(namespace, memoryID string, content []byte) error

    // Retrieve memories matching query
    Retrieve(namespace string, query Query) ([]Memory, error)

    // Update existing memory entry
    Update(namespace, memoryID string, update Update) error

    // Delete memory entry
    Delete(namespace, memoryID string) error
}

type Query struct {
    Type      string    // episodic, semantic, procedural
    Tags      []string
    DateFrom  time.Time
    DateTo    time.Time
    Limit     int
}

type Update struct {
    AppendContent string
    SetTags       []string
    SetType       string
}

type Memory struct {
    ID        string
    Namespace string
    Type      string
    Content   []byte
    Tags      []string
    Created   time.Time
    Modified  time.Time
}
```

**Providers** (v0.1.0 - v0.3.0 roadmap):

**1. Simple Provider** (v0.1.0 - Default)
- File-based storage: `~/.engram/memory/{namespace}/{memoryID}.json`
- No dependencies, works offline
- Suitable for single-user, local development
- Linear search (fast for <1000 memories)

**2. SQLite Provider** (v0.2.0 - Planned)
- Embedded database: `~/.engram/memory.db`
- No server required
- SQL queries for filtering
- Suitable for single-user, 1000s of memories

**3. Postgres Provider** (v0.3.0 - Planned)
- Shared database
- Concurrent access for teams
- Full-text search
- Suitable for teams, production

**4. Redis Provider** (Future)
- In-memory cache + persistence
- Fast retrieval
- Pub/sub for real-time updates
- Suitable for high-performance systems

**Configuration**:

**Priority**:
1. `--provider` flag
2. `ENGRAM_MEMORY_PROVIDER` environment variable
3. Default: `simple`

**Config File** (v0.2.0+):
```yaml
# ~/.engram/memory.yaml
provider: simple

providers:
  simple:
    path: ~/.engram/memory

  sqlite:
    database: ~/.engram/memory.db

  postgres:
    host: localhost
    port: 5432
    database: engram
    user: engram
    password: ${ENGRAM_DB_PASSWORD}
```

**Note (v0.1.0)**: The `--config` flag currently specifies the storage directory path, not a YAML file. Full YAML config support in v0.2.0.

**Provider Factory**:

```go
func NewProvider(providerType string, config Config) (Provider, error) {
    switch providerType {
    case "simple":
        return NewSimpleProvider(config.Path)
    case "sqlite":
        return NewSQLiteProvider(config.Database)
    case "postgres":
        return NewPostgresProvider(config.ConnectionString)
    default:
        return nil, fmt.Errorf("unknown provider: %s", providerType)
    }
}
```

**Memory Namespaces**:

Namespaces provide isolation between different contexts:
- `user,alice` - User-specific memories
- `team,backend` - Team-specific memories
- `project,engram` - Project-specific memories
- `global` - System-wide memories

**Memory Types**:

Following cognitive science memory taxonomy:
- `episodic` - Event-based memories (what happened)
- `semantic` - Fact-based memories (what is true)
- `procedural` - Skill-based memories (how to do)

**Rationale**:

1. **Flexibility**: Switch storage backends without code changes
2. **Extensibility**: Add new providers easily
3. **Simple Default**: File-based provider for easy onboarding
4. **Scalability**: Upgrade to database as needs grow
5. **Testing**: Mock provider for unit tests

**Alternatives Considered**:

1. **File-only**: Not scalable, no concurrent access
2. **Database-only**: High barrier to entry for local development
3. **Hard-coded storage**: No flexibility, future refactoring pain
4. **External service**: Network dependency, complexity

**Consequences**:

**Positive**:
- Users start with simple file-based storage
- Teams can upgrade to shared database
- Production can use scalable backend
- Easy to test with mock provider
- Future-proof architecture

**Negative**:
- Provider abstraction adds complexity
- Must maintain multiple implementations
- Configuration more complex (mitigated by defaults)

**Migration Strategy**:

When upgrading providers:
```bash
# Export from simple provider
engram memory export --provider simple --output memories.jsonl

# Import to postgres provider
engram memory import --provider postgres --input memories.jsonl
```

**Implementation Phases**:

**Phase 1 (v0.1.0)**: Simple Provider
- [x] File-based storage
- [x] Basic CRUD operations
- [x] Namespace support
- [ ] Config via directory path

**Phase 2 (v0.2.0)**: SQLite Provider
- [ ] SQLite implementation
- [ ] YAML config file support
- [ ] Migration tools
- [ ] Full-text search

**Phase 3 (v0.3.0)**: Postgres Provider
- [ ] Postgres implementation
- [ ] Connection pooling
- [ ] Concurrent access
- [ ] Team collaboration features

**Security Considerations**:

1. **Namespace isolation**: Validate namespace format
2. **Path validation**: Prevent path traversal in simple provider
3. **SQL injection**: Use parameterized queries in SQL providers
4. **Credentials**: Environment variables for passwords, not config files
5. **Access control**: Future work for multi-user scenarios

**Performance Targets**:

| Provider | Store | Retrieve | Update | Delete |
|----------|-------|----------|--------|--------|
| Simple   | <10ms | <50ms    | <10ms  | <10ms  |
| SQLite   | <5ms  | <20ms    | <5ms   | <5ms   |
| Postgres | <10ms | <30ms    | <10ms  | <10ms  |
| Redis    | <1ms  | <5ms     | <1ms   | <1ms   |

**Testing Strategy**:

1. **Provider interface tests**: Test contract, not implementation
2. **Mock provider**: For testing commands
3. **Integration tests**: Real provider instances
4. **Migration tests**: Verify export/import

**Related Decisions**:
- ADR-004: Three-Tier Retrieval System
- ADR-005: Hierarchical Workspace Structure
- ADR-006: Security-First Input Validation
