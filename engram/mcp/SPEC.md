# Engram MCP Server - Specification

## Document Information

- **Version**: 1.1.0
- **Status**: Production
- **Last Updated**: 2026-02-19
- **Phase**: Phase 3 (workflow-improvements-2026)
- **Maintainer**: Engram Project Team

## Table of Contents

1. [Overview](#overview)
2. [Requirements](#requirements)
3. [API Specification](#api-specification)
4. [Performance Requirements](#performance-requirements)
5. [Testing Strategy](#testing-strategy)
6. [Dependencies](#dependencies)
7. [Deployment](#deployment)
8. [References](#references)

---

## Overview

### Purpose

The Engram MCP Server provides Model Context Protocol (MCP) integration for Engram's semantic memory system, enabling Claude Code and other MCP clients to:

- **Retrieve Knowledge**: Search Engram's knowledge base using semantic embeddings
- **Discover Plugins**: List and explore installed Engram plugins
- **Track Progress**: Monitor Wayfinder SDLC project phases
- **Manage Tasks**: Create and track beads (issues/tasks) programmatically

### Scope

**In Scope (Phase 3)**:
- Task 3.2: Basic MCP tools (retrieve, plugins.list, phase.status)
- Task 3.5: Enhanced tools (beads.create, semantic search, performance profiling)
- MCP protocol compliance (initialize, tools/list, tools/call)
- Performance optimization (<100ms target latency)
- Comprehensive testing and documentation

**Out of Scope**:
- Bead editing/deletion (future enhancement)
- Real-time notifications/subscriptions
- Multi-user authentication/authorization
- Web-based UI (CLI/MCP only)
- Phase 4 validation (separate workstream)

### Goals

1. **Productivity**: Reduce context switching by exposing Engram capabilities via MCP
2. **Performance**: All tool invocations complete in <100ms (P95)
3. **Reliability**: Robust error handling, input validation, graceful degradation
4. **Maintainability**: Well-documented, tested, modular architecture
5. **Extensibility**: Plugin-like tool architecture for future additions

---

## Requirements

### Functional Requirements

#### FR-1: Semantic Engram Retrieval

**Description**: Enable semantic search across Engram knowledge base using natural language queries.

**Acceptance Criteria**:
- AC1.1: Accept text query (required) and optional type filter (ai/why/all)
- AC1.2: Return top-k results (1-20, default 5) ranked by semantic similarity
- AC1.3: Support embedding-based similarity scoring (cosine similarity)
- AC1.4: Include metadata (title, type, score, description, tags) in results
- AC1.5: Handle empty result sets gracefully (no errors)
- AC1.6: Cache embeddings for performance (avoid re-encoding same engrams)

**Priority**: P0 (Critical)

**Implementation**: `tools/engram_retrieve.py` using sentence-transformers

**Test Coverage**:
- Basic query (test_mcp_server.py:40-56)
- Type filtering (test_mcp_server.py:57-73)
- Performance benchmarks (benchmark_mcp_server.py)

---

#### FR-2: Plugin Discovery

**Description**: List all installed Engram plugins with metadata.

**Acceptance Criteria**:
- AC2.1: Scan `plugins/` directory in Engram repository
- AC2.2: Extract metadata (name, version, status, description, language)
- AC2.3: Support multiple metadata sources (package.json, go.mod, README.md)
- AC2.4: Return structured JSON array with plugin details
- AC2.5: Handle missing/malformed metadata gracefully (default values)

**Priority**: P1 (High)

**Implementation**: `tools/plugins_list.py`

**Test Coverage**: test_mcp_server.py:74-85

---

#### FR-3: Wayfinder Phase Status

**Description**: Get current phase status for Wayfinder SDLC projects.

**Acceptance Criteria**:
- AC3.1: Accept project path (absolute or tilde-expanded)
- AC3.2: Detect Wayfinder markers (.wayfinder, SPEC.md, ROADMAP.md)
- AC3.3: Parse STATUS files to determine current phase
- AC3.4: Return phase number, name, completion percentage, deliverables
- AC3.5: Validate project exists and is a Wayfinder project (error if not)

**Priority**: P1 (High)

**Implementation**: `tools/wayfinder_status.py`

**Test Coverage**: test_mcp_server.py:86-101 (optional, requires Wayfinder project)

---

#### FR-4: Programmatic Bead Creation

**Description**: Create beads (issues/tasks) programmatically via MCP.

**Acceptance Criteria**:
- AC4.1: Accept title (required), description (required), priority (0-5), labels, estimated_minutes
- AC4.2: Validate inputs (title length, priority range, description length)
- AC4.3: Check for duplicate beads (same title, open status)
- AC4.4: Generate unique bead ID (format: `engram-{NNN}{suffix}`)
- AC4.5: Write to JSONL database (append-only)
- AC4.6: Return bead ID and validation results

**Priority**: P1 (High)

**Implementation**: `tools/beads_create.py`

**Test Coverage**:
- Basic creation (test_mcp_server.py:102-120)
- Duplicate detection (test_mcp_server.py:121-137)
- Invalid priority (test_mcp_server.py:138-155)

---

#### FR-5: Performance Profiling

**Description**: Track and report tool invocation latency.

**Acceptance Criteria**:
- AC5.1: Measure elapsed time for each tool invocation
- AC5.2: Calculate P50, P95, P99 percentiles
- AC5.3: Log warnings when tools exceed 100ms target
- AC5.4: Provide summary statistics via `profiler.get_stats()`
- AC5.5: Minimal overhead (<1ms profiling cost)

**Priority**: P1 (High)

**Implementation**: `performance.py` (PerformanceProfiler class)

**Test Coverage**: benchmark_mcp_server.py (20 iterations per tool)

---

### Non-Functional Requirements

#### NFR-1: Performance

**Requirement**: All tool invocations complete in <100ms (P95 latency)

**Rationale**: MCP tools are invoked frequently during Claude Code sessions. Latency >100ms creates noticeable lag and interrupts workflow.

**Measurement**:
- Target: P95 < 100ms
- Acceptable: P95 < 150ms
- Critical threshold: P95 > 200ms (investigate optimization)

**Optimization Strategies**:
- Embedding caching (EngramRetrieve: ~60ms first call → ~10ms cached)
- Lazy loading (don't load sentence-transformers until first retrieval)
- Efficient file I/O (mmap for large files, streaming for JSONL)
- Minimal JSON serialization overhead

**Validation**: `benchmark_mcp_server.py` measures P95/P99 across 20 iterations

---

#### NFR-2: Reliability

**Requirement**: Graceful error handling with informative error messages

**Acceptance Criteria**:
- All exceptions caught and converted to JSON-RPC error responses
- Error codes follow JSON-RPC spec:
  - `-32700`: Parse error (invalid JSON)
  - `-32601`: Method not found (unknown MCP method)
  - `-32602`: Invalid params (validation failures)
  - `-32603`: Internal error (tool execution failures)
- Error messages sanitized (no stack traces to client, log to stderr)
- Partial failures handled gracefully (e.g., missing engrams don't crash search)

**Test Coverage**: All test cases validate error handling paths

---

#### NFR-3: Security

**Requirement**: Input validation and safe file access

**Acceptance Criteria**:
- All parameters validated against JSON schemas (inputSchema)
- File paths restricted to known safe directories (ENGRAM_ROOT, BEADS_DB)
- No arbitrary command execution (shell injection prevention)
- No sensitive data in error messages (file paths OK, contents NOT OK)
- Environment variables validated (expand ~ but reject ../)

**Implementation**:
- JSON schema validation (MCP protocol layer)
- Path validation in tool implementations
- Python's `Path.resolve()` to prevent directory traversal

---

#### NFR-4: Maintainability

**Requirement**: Well-documented, testable, modular code

**Acceptance Criteria**:
- All modules have docstrings (module, class, function level)
- Type hints for all function signatures
- Clear separation of concerns (tools/, performance.py, engram_mcp_server.py)
- Test coverage >80% of code paths
- Living documentation (SPEC.md, ARCHITECTURE.md, ADRs)

**Standards**:
- PEP 8 (Python style guide)
- Docstring format: Google style
- Logging: structured, contextual (logger per module)
- Error messages: actionable, user-friendly

---

## API Specification

### MCP Protocol Methods

#### initialize

**Purpose**: MCP protocol handshake to establish server capabilities

**Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 0,
  "method": "initialize",
  "params": {}
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 0,
  "result": {
    "protocolVersion": "0.1.0",
    "serverInfo": {
      "name": "engram-mcp-server",
      "version": "1.1.0"
    },
    "capabilities": {
      "tools": {}
    }
  }
}
```

**Errors**: None (always succeeds)

**Implementation**: `EngramMCPServer._handle_initialize()` (lines 97-112)

---

#### tools/list

**Purpose**: List all available MCP tools

**Request**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list"
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "engram_retrieve",
        "description": "Retrieve relevant engrams using semantic search...",
        "inputSchema": { /* JSON Schema */ }
      },
      /* ... other tools ... */
    ]
  }
}
```

**Errors**: None (always returns tool list)

**Implementation**: `EngramMCPServer._handle_list_tools()` (lines 114-214)

---

### Tool 1: engram_retrieve

**Purpose**: Semantic memory retrieval using embeddings

**Method**: `tools/call` with `name: "engram_retrieve"`

**Parameters**:

| Parameter    | Type    | Required | Default | Constraints | Description                        |
|--------------|---------|----------|---------|-------------|------------------------------------|
| query        | string  | Yes      | -       | Non-empty   | Search query (natural language)    |
| type_filter  | string  | No       | "all"   | Enum: ai, why, all | Filter by engram type |
| top_k        | integer | No       | 5       | 1-20        | Number of results to return        |

**Returns**:
```json
{
  "query": "error handling",
  "results": [
    {
      "path": "engrams/patterns/go/error-handling.why.md",
      "title": "error-handling.why",
      "type": "why",
      "score": 0.544,
      "description": "Explains Go's error handling philosophy...",
      "tags": ["go", "errors", "patterns"]
    }
  ],
  "total_searched": 52,
  "method": "semantic_embedding"
}
```

**Errors**:
- `-32602`: Empty query
- `-32602`: Invalid type_filter (not in ai/why/all)
- `-32602`: top_k out of range (not 1-20)
- `-32603`: File I/O errors (rare, logged to stderr)

**Performance**:
- First call (cold): ~60ms (model loading + embedding)
- Cached calls: ~10-20ms (embeddings cached)
- Target: <100ms P95

**Implementation**: `tools/engram_retrieve.py` (EngramRetrieve class)

**Algorithm**:
1. Load sentence-transformers model (all-MiniLM-L6-v2)
2. Find all engram files matching type_filter (*.ai.md, *.why.md)
3. Encode query to embedding vector (384 dimensions)
4. For each engram:
   - Get cached embedding or encode (title + description + content[:1000])
   - Calculate cosine similarity: `dot(query, engram) / (||query|| * ||engram||)`
5. Sort by similarity score (descending)
6. Return top_k results

**Caching**:
- Embedding cache: `{file_path: numpy.ndarray}` (in-memory)
- Metadata cache: `{file_path: {title, description, tags}}` (in-memory)
- Cache invalidation: Manual (server restart) or `clear_cache()` method

---

### Tool 2: engram_plugins_list

**Purpose**: Discover installed Engram plugins

**Method**: `tools/call` with `name: "engram_plugins_list"`

**Parameters**: None (empty object `{}`)

**Returns**:
```json
{
  "plugins": [
    {
      "name": "multi-persona-review",
      "path": "plugins/multi-persona-review",
      "version": "0.1.0",
      "status": "active",
      "language": "typescript",
      "description": "Multi-persona code review plugin for Engram."
    }
  ],
  "total": 8
}
```

**Errors**:
- `-32603`: Engram repository not found (ENGRAM_ROOT misconfigured)

**Performance**: ~5-15ms (filesystem scan + metadata parsing)

**Implementation**: `tools/plugins_list.py` (PluginsList class)

**Algorithm**:
1. Scan `{ENGRAM_ROOT}/plugins/` for subdirectories
2. For each plugin directory:
   - Check for package.json (TypeScript/JavaScript)
   - Check for go.mod (Go)
   - Check for README.md (fallback)
   - Extract: name, version, description, language
3. Return sorted list (by name)

**Metadata Sources** (priority order):
1. **package.json**: name, version, description (TypeScript/JS plugins)
2. **go.mod**: module name, version from git tags (Go plugins)
3. **README.md**: First paragraph as description (fallback)
4. **Defaults**: version="unknown", status="active", language="unknown"

---

### Tool 3: wayfinder_phase_status

**Purpose**: Get current phase status for Wayfinder SDLC projects

**Method**: `tools/call` with `name: "wayfinder_phase_status"`

**Parameters**:

| Parameter    | Type   | Required | Description                           |
|--------------|--------|----------|---------------------------------------|
| project_path | string | Yes      | Path to Wayfinder project (~ supported) |

**Returns**:
```json
{
  "project_path": "the git history",
  "is_wayfinder_project": true,
  "current_phase": 2,
  "phase_name": "Design",
  "completion_percentage": 75,
  "deliverables": [
    "SPEC.md",
    "ARCHITECTURE.md",
    "ADR-001-technology-choices.md"
  ],
  "status_file": ".wayfinder/status"
}
```

**Errors**:
- `-32602`: Project path does not exist
- `-32602`: Not a Wayfinder project (missing markers)
- `-32603`: STATUS file parse errors

**Performance**: ~5-10ms (file I/O)

**Implementation**: `tools/wayfinder_status.py` (WayfinderStatus class)

**Wayfinder Markers** (must have at least one):
- `.wayfinder/` directory
- `SPEC.md` file
- `ROADMAP.md` file

**STATUS File Format**:
```
phase: 2
name: Design
completion: 75
deliverables:
  - SPEC.md
  - ARCHITECTURE.md
```

---

### Tool 4: beads_create

**Purpose**: Create beads (issues/tasks) programmatically

**Method**: `tools/call` with `name: "beads_create"`

**Parameters**:

| Parameter         | Type     | Required | Default | Constraints | Description                        |
|-------------------|----------|----------|---------|-------------|------------------------------------|
| title             | string   | Yes      | -       | Non-empty   | Bead title (brief, imperative)     |
| description       | string   | Yes      | -       | Non-empty   | Detailed description               |
| priority          | integer  | No       | 1       | 0-5         | Priority (0=P0/highest, 5=P5)      |
| labels            | string[] | No       | []      | -           | Tags for categorization            |
| estimated_minutes | integer  | No       | 60      | ≥1          | Time estimate in minutes           |

**Returns**:
```json
{
  "bead_id": "engram-19a",
  "title": "MCP Server Test Bead",
  "status": "created",
  "duplicate_check": "passed",
  "validation": {
    "title_length": 20,
    "description_length": 66,
    "priority": 2,
    "estimated_minutes": 15,
    "labels_count": 2
  }
}
```

**Errors**:
- `-32602`: Empty title or description
- `-32602`: Priority out of range (not 0-5)
- `-32602`: Duplicate bead found (same title, status=open)
- `-32603`: Database write errors

**Performance**: ~10-20ms (JSONL append + duplicate check)

**Implementation**: `tools/beads_create.py` (BeadsCreate class)

**JSONL Database Format**:
```jsonl
{"id":"engram-19a","title":"MCP Server Test Bead","description":"...","priority":2,"labels":["test","mcp-server"],"status":"open","created_at":"2026-02-19T10:30:00Z","estimated_minutes":15}
```

**Duplicate Detection**:
1. Read all beads from JSONL database
2. Filter for `status != "closed"` and `status != "resolved"`
3. Check if any bead has identical title
4. If match found, return error with existing bead ID

**Bead ID Generation**:
- Format: `{project}-{NNN}{suffix}`
- Project: "engram" (hardcoded for now)
- NNN: Zero-padded sequential number (001, 002, ...)
- Suffix: If collision, use 'a', 'b', 'c', ... (rare)
- Example: `engram-001`, `engram-042`, `engram-100a`

---

## Performance Requirements

### Latency Targets

| Tool                    | Target (P95) | Acceptable (P95) | Critical (P95) |
|-------------------------|--------------|------------------|----------------|
| engram_retrieve (cold)  | <100ms       | <150ms           | <200ms         |
| engram_retrieve (cached)| <50ms        | <75ms            | <100ms         |
| engram_plugins_list     | <50ms        | <75ms            | <100ms         |
| wayfinder_phase_status  | <50ms        | <75ms            | <100ms         |
| beads_create            | <50ms        | <75ms            | <100ms         |

**Rationale**:
- Cold retrieval: Model loading (~40ms) + embedding (~20ms)
- Cached retrieval: Embedding lookup (~10ms)
- Other tools: Lightweight file I/O (<50ms)

### Performance Profiling

**Measurement**: `PerformanceProfiler` class (performance.py)

**Metrics**:
- **Count**: Number of invocations
- **Avg**: Average latency (ms)
- **Min/Max**: Minimum and maximum latency (ms)
- **P95**: 95th percentile latency (ms)
- **P99**: 99th percentile latency (ms)
- **Exceeds Target**: Boolean flag if avg > 100ms

**Usage**:
```python
with profiler.profile('engram_retrieve'):
    result = engram_retrieve.retrieve(query)

stats = profiler.get_stats()
# {
#   "engram_retrieve": {
#     "count": 20,
#     "avg_ms": 62.4,
#     "p95_ms": 85.2,
#     "p99_ms": 120.1,
#     "exceeds_target": false
#   }
# }
```

**Logging**:
- Warnings logged when tools exceed 100ms target
- Stats logged after each invocation (INFO level)
- Benchmark results written to stdout (benchmark_mcp_server.py)

---

## Testing Strategy

### Test Coverage

| Test Type       | File                       | Coverage                          |
|-----------------|----------------------------|-----------------------------------|
| Integration     | test_mcp_server.py         | 8 test cases (all 4 tools + errors) |
| Performance     | benchmark_mcp_server.py    | P95/P99 latency benchmarks        |
| Unit            | (Future) test_tools/*.py   | Individual tool logic tests       |

**Target**: 80%+ code path coverage (measured via pytest-cov)

### Integration Tests (test_mcp_server.py)

**Test Cases**:
1. **tools/list**: Verify all 4 tools listed
2. **engram_retrieve - basic query**: Search with defaults
3. **engram_retrieve - type filter**: Filter by 'ai' type
4. **engram_plugins_list**: List all plugins
5. **wayfinder_phase_status**: Get phase status (optional if project exists)
6. **beads_create - success**: Create new bead
7. **beads_create - duplicate**: Detect duplicate title
8. **beads_create - invalid priority**: Validate priority range

**Execution**:
```bash
cd ./engram/mcp-server
.venv/bin/python3 test_mcp_server.py
```

**Success Criteria**: All tests pass (exit code 0)

### Performance Benchmarks (benchmark_mcp_server.py)

**Methodology**:
- Run each tool 20 times
- Measure elapsed time per invocation
- Calculate P50, P95, P99 percentiles
- Compare against 100ms target

**Execution**:
```bash
cd ./engram/mcp-server
.venv/bin/python3 benchmark_mcp_server.py
```

**Output**:
```
Tool: engram_retrieve
  P50: 58.2ms
  P95: 85.1ms
  P99: 120.3ms
  Status: ✅ PASS (P95 < 100ms)
```

### Manual Testing

**Test Scenarios**:
1. **MCP Registration**: Add to `~/.claude/settings.json` and restart Claude Code
2. **Tool Discovery**: Verify tools appear in Claude Code tool list
3. **Tool Invocation**: Invoke each tool from Claude Code chat
4. **Error Handling**: Test invalid inputs (empty query, bad priority)
5. **Performance**: Measure perceived latency during real usage

---

## Dependencies

### Python Runtime

- **Version**: Python 3.9+ (3.10+ recommended)
- **Installation**: System Python or pyenv

### Python Packages

Defined in `requirements.txt`:

```txt
# Semantic search and embeddings (Task 3.5)
sentence-transformers>=2.2.0

# Vector similarity
numpy>=1.24.0
scikit-learn>=1.3.0

# YAML parsing (for engram frontmatter)
PyYAML>=6.0

# Testing
pytest>=7.4.0
pytest-timeout>=2.1.0
```

**Installation**:
```bash
cd ./engram/mcp-server
python3 -m venv .venv
.venv/bin/pip install -r requirements.txt
```

### External Dependencies

- **Engram Repository**: Must exist at `ENGRAM_ROOT` (default: `./engram`)
- **Beads Database**: JSONL file at `BEADS_DB` (default: `~/.beads/issues.jsonl`)
- **Wayfinder Projects**: Optional, for `wayfinder_phase_status` tool

---

## Deployment

### Installation

1. **Clone Engram Repository** (if not already cloned):
   ```bash
   git clone https://github.com/your-org/engram.git ./engram
   ```

2. **Install Dependencies**:
   ```bash
   cd ./engram/mcp-server
   python3 -m venv .venv
   .venv/bin/pip install -r requirements.txt
   ```

3. **Verify Installation**:
   ```bash
   .venv/bin/python3 test_mcp_server.py
   ```

### MCP Registration

Add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "engram": {
      "command": "engram/mcp-server/.venv/bin/python3",
      "args": [
        "engram/mcp-server/engram_mcp_server.py"
      ],
      "env": {
        "ENGRAM_ROOT": "./engram",
        "BEADS_DB": "~/.beads/issues.jsonl"
      }
    }
  }
}
```

**Restart Claude Code** to load the MCP server.

### Configuration

**Environment Variables**:
- `ENGRAM_ROOT`: Path to Engram repository (default: `./engram`)
- `BEADS_DB`: Path to beads JSONL database (default: `~/.beads/issues.jsonl`)

**Command-Line Arguments** (alternative to env vars):
```bash
python3 engram_mcp_server.py \
  --engram-root ./engram \
  --beads-db ~/.beads/issues.jsonl
```

### Logging

**Log Destinations**:
- **stderr**: Server logs (initialization, tool invocations, errors)
- **stdout**: JSON-RPC responses only (MCP protocol)

**Log Levels**:
- `INFO`: Normal operations (server start, tool calls)
- `WARNING`: Performance warnings (>100ms), missing files
- `ERROR`: Exceptions, validation failures, tool errors

**Configuration**:
```python
# In engram_mcp_server.py
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    stream=sys.stderr
)
```

---

## References

### MCP Protocol

- **Specification**: https://modelcontextprotocol.io/
- **JSON-RPC 2.0**: https://www.jsonrpc.org/specification
- **Claude Code MCP**: https://docs.anthropic.com/claude/docs/mcp

### Phase 3 Documentation

- **ROADMAP**: `ROADMAP.md`
- **Task 3.2**: Basic MCP tools (engram.retrieve, plugins.list, phase.status)
- **Task 3.5**: Enhanced tools (beads.create, semantic search, performance profiling)
- **Phase 3 ADRs**:
  - ADR-001: Technology choices (sentence-transformers, JSONL)
  - ADR-002: Performance optimization strategy
  - ADR-003: Error handling patterns

### Related Projects

- **Engram**: https://github.com/your-org/engram
- **Beads**: https://github.com/your-org/beads
- **Wayfinder**: https://github.com/your-org/wayfinder

### Technical Resources

- **sentence-transformers**: https://www.sbert.net/
- **MiniLM Model**: https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2
- **Cosine Similarity**: https://en.wikipedia.org/wiki/Cosine_similarity
- **JSONL Format**: http://jsonlines.org/

---

## Version History

| Version | Date       | Changes                                      | Phase  |
|---------|------------|----------------------------------------------|--------|
| 1.0.0   | 2026-02-15 | Initial release (Task 3.2 basic tools)       | Phase 3|
| 1.1.0   | 2026-02-19 | Task 3.5 enhancements (beads, semantic, perf)| Phase 3|

---

**Document Status**: Living documentation - updated as implementation evolves
**Review Cycle**: After each Phase 3 task completion, Phase 4 validation
**Approval**: Required for Phase 3 → Phase 4 transition
