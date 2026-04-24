# Corpus Callosum — Architecture

## System Overview

Corpus Callosum follows a three-layer architecture: client interfaces, core library,
and persistent storage.

```
┌─────────────────────────────────────────────────┐
│              Client Interfaces                  │
├──────────────────────┬──────────────────────────┤
│   CLI (cc binary)    │  MCP Server (cc-mcp)     │
│   9 commands         │  5 JSON-RPC tools        │
└──────────┬───────────┴───────────┬──────────────┘
           │                       │
           ▼                       ▼
┌─────────────────────────────────────────────────┐
│              Core Library                       │
├─────────────────────────────────────────────────┤
│  registry/db.go     Schema + component storage  │
│  schema/validator   JSON Schema validation      │
│  query/query.go     Filter, sort, paginate      │
│  mcp/server.go      JSON-RPC 2.0 handler        │
└──────────────────────┬──────────────────────────┘
                       │
                       ▼
              SQLite Database
         (~/.config/corpus-callosum/)
```

## Key Packages

### `internal/registry`

SQLite-backed storage for schemas and component metadata. Manages:
- Schema versions with semantic versioning
- Component registration and discovery
- Version history and timestamps

**Database tables:**

| Table | Purpose |
|-------|---------|
| `cc_schemas` | Schema documents, versions, compatibility modes |
| `cc_components` | Component metadata (name, description, owner) |
| `cc_query_cache` | Optional query result caching |

### `internal/schema`

JSON Schema Draft 2020-12 validation engine. Responsibilities:
- Validate data against registered schemas
- Check compatibility between schema versions (4 modes)
- Parse and normalize schema documents

### `internal/query`

Query engine for component metadata. Supports:
- Filtering by component, version, timestamp
- Sorting (ascending/descending on any field)
- Pagination via `--limit` and `--offset`

### `internal/mcp`

MCP server implementing JSON-RPC 2.0. Wraps core library operations
as LLM-accessible tools for Claude Desktop integration.

## Data Flow

### Schema Registration

```
CLI/API → registry.RegisterSchema()
        → schema.Validate() (check schema is valid JSON Schema)
        → schema.CheckCompatibility() (if prior version exists)
        → db.InsertSchema() (persist to SQLite)
```

### Data Validation

```
CLI/API → registry.GetSchema()
        → schema.ValidateData(schema, data)
        → return ValidationResult{Valid, Errors[]}
```

### Component Discovery

```
CLI/API → registry.ListComponents()
        → query.Apply(filters, sort, limit)
        → return []Component
```

## Design Decisions

- **SQLite over network DB**: Local-first, zero-config, fast (~1-5ms operations)
- **Workspace isolation**: Separate databases prevent cross-workspace schema leaks
- **Optional dependency**: Components function without corpus-callosum installed;
  it enhances but doesn't gate workflows
- **Dual interface**: CLI for human operators, MCP for LLM agents
- **JSON Schema standard**: Industry-standard validation avoids custom formats
