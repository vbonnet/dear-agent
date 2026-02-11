# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records for the AGM MCP Server.

## What are ADRs?

Architecture Decision Records (ADRs) document significant architectural decisions made during the development of this project. Each ADR captures:
- The context and problem being solved
- The decision made
- The rationale behind the decision
- The consequences of the decision

## ADR Index

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [001](./001-mcp-protocol-choice.md) | MCP Protocol Choice | Accepted | 2025-01-15 |
| [002](./002-caching-strategy.md) | Caching Strategy | Accepted | 2025-01-15 |
| [003](./003-metadata-only-exposure.md) | Metadata-Only Exposure | Accepted | 2025-01-15 |
| [004](./004-tool-granularity.md) | Tool Granularity | Accepted | 2025-01-15 |
| [005](./005-stdio-transport-logging.md) | Stdio Transport Logging Strategy | Accepted | 2025-01-15 |

## ADR Summary

### [001: MCP Protocol Choice](./001-mcp-protocol-choice.md)

**Decision**: Use Model Context Protocol (MCP) with stdio transport instead of REST API, CLI, or gRPC.

**Key Rationale**:
- Native Claude Code integration
- Privacy by design (local process, no network)
- Standardized protocol for AI assistants
- Official Go SDK support

### [002: Caching Strategy](./002-caching-strategy.md)

**Decision**: Implement in-memory cache with 5-second TTL for session list.

**Key Rationale**:
- Meets p99 latency targets (<100ms for 1000 sessions)
- Simple implementation (30 lines of code)
- Thread-safe with double-check locking
- Acceptable staleness tradeoff

### [003: Metadata-Only Exposure](./003-metadata-only-exposure.md)

**Decision**: Expose only session metadata via MCP, never conversation content.

**Key Rationale**:
- Privacy protection (no API keys, credentials exposed)
- Clear security boundary (manifest.json only, never history.jsonl)
- Performance (manifest files <1KB vs history files up to 10MB)
- Sufficient for all V1 use cases

### [004: Tool Granularity](./004-tool-granularity.md)

**Decision**: Provide three focused MCP tools (list, search, get) instead of monolithic or fine-grained tools.

**Key Rationale**:
- Clear separation of concerns (list vs search vs get)
- Discoverable (3 tools, obvious purposes)
- Flexible filtering (filters as params, not separate tools)
- Avoids combinatorial explosion

### [005: Stdio Transport Logging Strategy](./005-stdio-transport-logging.md)

**Decision**: Log all diagnostics to stderr using Go's standard `log` package.

**Key Rationale**:
- MCP stdio protocol requirement (stdout reserved for JSON-RPC)
- Standard Unix convention (stderr for diagnostics)
- Simple implementation (no file management)
- Claude Code integration (captures stderr for developer console)

## Decision Process

### When to Create an ADR

Create an ADR for:
- Architectural choices with long-term impact
- Trade-offs between multiple viable options
- Decisions that affect external interfaces (MCP tools, config format)
- Choices that future developers may question

### ADR Template

Each ADR follows this structure:
1. **Status**: Proposed, Accepted, Deprecated, Superseded
2. **Context**: Problem being solved, requirements, constraints
3. **Decision**: What was decided
4. **Rationale**: Why this decision was made
5. **Consequences**: Positive, negative, and neutral impacts
6. **References**: Links to relevant documentation

### ADR Lifecycle

- **Proposed**: Under discussion
- **Accepted**: Decision implemented
- **Deprecated**: No longer recommended
- **Superseded**: Replaced by newer ADR

## Contributing

When making significant architectural changes:

1. **Propose**: Create draft ADR with "Proposed" status
2. **Discuss**: Review with team/maintainers
3. **Decide**: Update status to "Accepted" or reject
4. **Implement**: Code the decision
5. **Document**: Update README if needed

## References

- ADR best practices: https://github.com/joelparkerhenderson/architecture-decision-record
- Lightweight ADRs: https://www.thoughtworks.com/radar/techniques/lightweight-architecture-decision-records
- When to use ADRs: https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions
