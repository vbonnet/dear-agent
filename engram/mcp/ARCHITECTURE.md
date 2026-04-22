# Engram MCP Server - Architecture

## Document Information

- **Version**: 1.1.0
- **Status**: Production
- **Last Updated**: 2026-02-19
- **Phase**: Phase 3 (workflow-improvements-2026)
- **Maintainer**: Engram Project Team

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture Principles](#architecture-principles)
3. [Component Architecture](#component-architecture)
4. [Data Flow](#data-flow)
5. [Tool Implementations](#tool-implementations)
6. [Performance Architecture](#performance-architecture)
7. [Error Handling](#error-handling)
8. [Testing Architecture](#testing-architecture)
9. [Security Architecture](#security-architecture)
10. [Deployment Architecture](#deployment-architecture)
11. [Future Extensibility](#future-extensibility)

---

## System Overview

### High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     MCP Client (Claude Code)                 │
│                                                              │
│  - Tool Discovery (tools/list)                              │
│  - Tool Invocation (tools/call)                             │
│  - Response Processing                                       │
└──────────────────────┬───────────────────────────────────────┘
                       │ JSON-RPC over stdio
                       │ (stdin: requests, stdout: responses)
                       ▼
┌─────────────────────────────────────────────────────────────┐
│              EngramMCPServer (Core Orchestrator)            │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Request Router                                       │  │
│  │  - initialize → _handle_initialize()                 │  │
│  │  - tools/list → _handle_list_tools()                │  │
│  │  - tools/call → _handle_call_tool()                 │  │
│  └───────────────────┬──────────────────────────────────┘  │
│                      │                                      │
│  ┌───────────────────▼──────────────────────────────────┐  │
│  │  PerformanceProfiler (Context Manager)               │  │
│  │  - profile(tool_name) → measure latency              │  │
│  │  - get_stats() → P50/P95/P99 metrics                │  │
│  └───────────────────┬──────────────────────────────────┘  │
│                      │                                      │
└──────────────────────┼───────────────────────────────────────┘
                       │
       ┌───────────────┼───────────────┐
       │               │               │
       ▼               ▼               ▼               ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│ EngramRetrieve│ PluginsList │ WayfinderStatus│ BeadsCreate │
│               │             │               │             │
│ - retrieve()  │ - list_     │ - get_status()│ - create()  │
│               │   plugins() │               │             │
└──────┬────────┘ └──────┬────┘ └──────┬──────┘ └──────┬──────┘
       │                 │             │               │
       ▼                 ▼             ▼               ▼
┌─────────────┐ ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
│sentence-    │ │Filesystem   │ │STATUS File  │ │JSONL DB     │
│transformers │ │(plugins/)   │ │Parser       │ │(append-only)│
└─────────────┘ └─────────────┘ └─────────────┘ └─────────────┘
```

### Component Summary

| Component            | Responsibility                                | Dependencies          |
|----------------------|-----------------------------------------------|-----------------------|
| EngramMCPServer      | JSON-RPC routing, MCP protocol handling       | tools/*, performance  |
| EngramRetrieve       | Semantic search via embeddings                | sentence-transformers |
| PluginsList          | Plugin discovery and metadata extraction      | filesystem, JSON/YAML |
| WayfinderStatus      | SDLC phase tracking                           | filesystem, YAML      |
| BeadsCreate          | Task/issue creation with validation           | JSONL database        |
| PerformanceProfiler  | Latency measurement and reporting             | time.perf_counter     |

---

## Architecture Principles

### 1. Separation of Concerns

**Principle**: Each component has a single, well-defined responsibility.

**Implementation**:
- **EngramMCPServer**: Protocol handling only (no business logic)
- **Tool classes**: Business logic only (no JSON-RPC awareness)
- **PerformanceProfiler**: Cross-cutting concern (performance monitoring)

**Benefits**:
- Testable in isolation
- Easy to modify individual components
- Clear ownership boundaries

---

### 2. Fail-Safe Defaults

**Principle**: Graceful degradation when resources unavailable.

**Examples**:
- Missing engrams directory → return empty results (not error)
- Missing plugin metadata → use defaults (version="unknown")
- Cache miss → compute on-the-fly (slower but functional)

**Benefits**:
- Robust to environment variability
- Better user experience (partial results > crashes)
- Easier deployment (fewer hard dependencies)

---

### 3. Performance by Design

**Principle**: Optimize for <100ms latency from architecture, not afterthought.

**Strategies**:
- **Caching**: Embedding cache (60ms → 10ms), metadata cache
- **Lazy Loading**: Don't load sentence-transformers until first retrieval
- **Streaming**: JSONL append-only (no full database rewrites)
- **Profiling**: Built-in measurement (PerformanceProfiler)

**Trade-offs**:
- Memory usage (caches) vs. latency
- First-call latency (model loading) vs. cached latency
- Accepted: Cold start ~60ms, cached <20ms

---

### 4. Extensibility

**Principle**: Easy to add new tools without modifying core server.

**Pattern**: Plugin-like tool registration

**Adding a New Tool**:
1. Create `tools/new_tool.py` with class implementing `execute(**kwargs)`
2. Import in `engram_mcp_server.py`: `from tools.new_tool import NewTool`
3. Instantiate in `__init__`: `self.new_tool = NewTool()`
4. Add to `_handle_list_tools()`: JSON schema for new tool
5. Add to `_handle_call_tool()`: Route to `self.new_tool.execute()`

**No modifications required**: Core routing logic, error handling, performance profiling

---

## Component Architecture

### 1. EngramMCPServer (Core)

**File**: `engram_mcp_server.py` (360 lines)

**Responsibilities**:
- **Protocol Handling**: Parse JSON-RPC requests, format responses
- **Method Routing**: Dispatch to appropriate handler (initialize, tools/list, tools/call)
- **Tool Orchestration**: Invoke tool implementations, wrap in performance profiling
- **Error Handling**: Catch exceptions, convert to JSON-RPC error responses
- **Lifecycle Management**: stdio transport loop, graceful shutdown

**Key Methods**:

| Method                     | Lines    | Purpose                                |
|----------------------------|----------|----------------------------------------|
| `__init__()`               | 37-71    | Initialize tools and profiler          |
| `handle_request()`         | 72-95    | Route JSON-RPC request to handler      |
| `_handle_initialize()`     | 97-112   | MCP protocol handshake                 |
| `_handle_list_tools()`     | 114-214  | Return tool schemas                    |
| `_handle_call_tool()`      | 216-282  | Invoke tool, profile performance       |
| `_error_response()`        | 284-293  | Format JSON-RPC error response         |
| `run()`                    | 295-328  | stdio transport loop                   |

**Initialization**:
```python
def __init__(self, engram_root: Path = None, beads_db: Path = None):
    # 1. Set defaults (./engram, ~/.beads/issues.jsonl)
    # 2. Instantiate tool implementations
    self.engram_retrieve = EngramRetrieve(engram_root)
    self.beads_create = BeadsCreate(beads_db)
    self.plugins_list = PluginsList(engram_root)
    self.wayfinder_status = WayfinderStatus()

    # 3. Initialize performance profiler
    self.profiler = PerformanceProfiler()
```

**Request Flow**:
```
stdin → JSON.parse → handle_request() → method routing:
  - "initialize" → _handle_initialize() → version/capabilities
  - "tools/list" → _handle_list_tools() → tool schemas
  - "tools/call" → _handle_call_tool() → profile(tool) → result
```

**Error Response Format**:
```json
{
  "jsonrpc": "2.0",
  "id": <request_id>,
  "error": {
    "code": -32602,
    "message": "Validation error: Query cannot be empty"
  }
}
```

---

### 2. Tool Implementations

#### 2.1 EngramRetrieve (Semantic Search)

**File**: `tools/engram_retrieve.py` (254 lines)

**Responsibilities**:
- **Semantic Search**: Encode queries and engrams to embeddings
- **Similarity Ranking**: Cosine similarity between query and engram vectors
- **Caching**: In-memory embedding and metadata caches
- **Type Filtering**: Support .ai.md vs .why.md filtering

**Architecture**:
```
Query → Encode (sentence-transformers) → Query Embedding (384-dim)
                                               ↓
Engrams → Find Files (*.ai.md, *.why.md) → For Each Engram:
  ├─ Get Cached Embedding OR
  ├─ Read Content → Encode (title + desc + content[:1000])
  ├─ Cache Embedding
  └─ Cosine Similarity(query_emb, engram_emb) → Score
                                               ↓
All Scores → Sort Descending → Top-K Results
```

**Embedding Model**:
- **Model**: `all-MiniLM-L6-v2` (sentence-transformers)
- **Dimensions**: 384
- **Speed**: ~20ms per encode (CPU)
- **Quality**: 0.5-0.7 similarity for relevant matches

**Caching Strategy**:
```python
# Embedding cache: avoid re-encoding same files
_embedding_cache: Dict[str, np.ndarray] = {}
# Key: file path
# Value: 384-dimensional numpy array

# Metadata cache: avoid re-parsing YAML frontmatter
_engram_cache: Dict[str, Dict[str, Any]] = {}
# Key: file path
# Value: {title, description, tags, type}
```

**Performance**:
- **Cold Start** (first call): ~60ms
  - Model loading: ~40ms (one-time)
  - Query encoding: ~20ms
  - Engram encoding: ~20ms per file (cached after)
- **Cached** (subsequent calls): ~10-20ms
  - Query encoding: ~20ms
  - Cache lookups: <1ms
  - Sorting: <1ms

**Cosine Similarity**:
```python
def _cosine_similarity(vec1: np.ndarray, vec2: np.ndarray) -> float:
    """Calculate cosine similarity: cos(θ) = (A·B) / (||A|| ||B||)"""
    dot_product = np.dot(vec1, vec2)
    norm1 = np.linalg.norm(vec1)
    norm2 = np.linalg.norm(vec2)
    return dot_product / (norm1 * norm2)
```

**Score Interpretation**:
- **0.7-1.0**: Highly relevant (exact/near match)
- **0.5-0.7**: Relevant (semantic match)
- **0.3-0.5**: Somewhat relevant (tangential)
- **0.0-0.3**: Not relevant (unrelated)

---

#### 2.2 PluginsList (Plugin Discovery)

**File**: `tools/plugins_list.py` (~150 lines)

**Responsibilities**:
- **Plugin Discovery**: Scan `plugins/` directory
- **Metadata Extraction**: Parse package.json, go.mod, README.md
- **Multi-Language Support**: TypeScript, Go, Python, Bash

**Architecture**:
```
{ENGRAM_ROOT}/plugins/ → List Subdirectories → For Each Plugin:
  ├─ Check package.json → Extract name, version, description (TS/JS)
  ├─ Check go.mod → Extract module name (Go)
  ├─ Check README.md → Extract first paragraph (Fallback)
  └─ Defaults: version="unknown", status="active"
                                               ↓
All Plugins → Sort by Name → Return List
```

**Metadata Precedence**:
1. **package.json**: Highest priority (TypeScript/JavaScript)
   ```json
   {
     "name": "multi-persona-review",
     "version": "0.1.0",
     "description": "Multi-persona code review plugin"
   }
   ```
2. **go.mod**: Second priority (Go)
   ```
   module github.com/your-org/engram/plugins/invariants
   go 1.21
   ```
3. **README.md**: Lowest priority (all languages)
   - First paragraph used as description

**Language Detection**:
- **TypeScript/JavaScript**: Has `package.json` or `tsconfig.json`
- **Go**: Has `go.mod`
- **Python**: Has `setup.py` or `pyproject.toml`
- **Unknown**: No recognizable metadata files

**Performance**: ~5-15ms (filesystem scan + file reads)

---

#### 2.3 WayfinderStatus (SDLC Phase Tracking)

**File**: `tools/wayfinder_status.py` (~120 lines)

**Responsibilities**:
- **Project Detection**: Validate Wayfinder markers
- **Phase Parsing**: Read and parse STATUS files
- **Deliverables Tracking**: List phase deliverables

**Architecture**:
```
Project Path → Expand ~ → Validate Exists → Check Markers:
  ├─ .wayfinder/ directory
  ├─ SPEC.md file
  └─ ROADMAP.md file
       (Must have at least one) ↓
Read .wayfinder/status → Parse YAML:
  - phase: 2
  - name: "Design"
  - completion: 75
  - deliverables: [SPEC.md, ARCHITECTURE.md]
                                               ↓
Return Structured Status
```

**Wayfinder Markers** (detection heuristics):
```python
def _is_wayfinder_project(project_path: Path) -> bool:
    markers = [
        project_path / ".wayfinder",
        project_path / "SPEC.md",
        project_path / "ROADMAP.md"
    ]
    return any(marker.exists() for marker in markers)
```

**STATUS File Format** (YAML):
```yaml
phase: 2
name: Design
completion: 75
deliverables:
  - SPEC.md
  - ARCHITECTURE.md
  - ADR-001-technology-choices.md
```

**Error Handling**:
- Path doesn't exist → `-32602: Project path does not exist`
- Not Wayfinder project → `-32602: Not a Wayfinder project (missing markers)`
- STATUS parse error → `-32603: Failed to parse STATUS file`

**Performance**: ~5-10ms (file I/O + YAML parsing)

---

#### 2.4 BeadsCreate (Task/Issue Creation)

**File**: `tools/beads_create.py` (~200 lines)

**Responsibilities**:
- **Input Validation**: Title, description, priority, labels
- **Duplicate Detection**: Check for existing open beads with same title
- **ID Generation**: Create unique bead IDs (e.g., `engram-042`)
- **Database Append**: Write to JSONL database

**Architecture**:
```
Input Params → Validate:
  ├─ Title: non-empty
  ├─ Description: non-empty
  ├─ Priority: 0-5
  └─ Labels: array of strings
                     ↓
Read JSONL DB → Filter open beads → Check duplicates:
  - Same title + status != closed → Error
                     ↓
Generate Bead ID:
  - Format: {project}-{NNN}{suffix}
  - Find max existing ID → Increment
                     ↓
Create Bead Record (JSON) → Append to JSONL → Return Result
```

**JSONL Database**:
```jsonl
{"id":"engram-001","title":"Fix bug","status":"open","created_at":"2026-02-19T10:00:00Z"}
{"id":"engram-002","title":"Add feature","status":"closed","created_at":"2026-02-19T11:00:00Z"}
{"id":"engram-003","title":"Write docs","status":"open","created_at":"2026-02-19T12:00:00Z"}
```

**Benefits**:
- Append-only: Fast writes (~10ms)
- Human-readable: Easy debugging (cat issues.jsonl)
- Git-friendly: Merge-friendly format (line-based)

**Duplicate Detection Logic**:
```python
def _check_duplicates(title: str) -> Optional[Dict]:
    beads = self._read_all_beads()
    for bead in beads:
        if (bead['title'] == title and
            bead['status'] in ['open', 'in_progress']):
            return bead  # Duplicate found
    return None
```

**ID Generation**:
```python
def _generate_bead_id() -> str:
    # Find max existing ID (e.g., engram-042)
    max_num = max([extract_number(bead['id']) for bead in beads])
    next_num = max_num + 1

    # Format: engram-001, engram-042, etc.
    return f"engram-{next_num:03d}"

    # If collision (rare), append suffix: engram-042a, engram-042b
```

**Validation Rules**:
| Field             | Rule                         | Error Code |
|-------------------|------------------------------|------------|
| title             | Non-empty string             | -32602     |
| description       | Non-empty string             | -32602     |
| priority          | Integer 0-5                  | -32602     |
| labels            | Array of strings             | -32602     |
| estimated_minutes | Integer ≥1                   | -32602     |

**Performance**: ~10-20ms (JSONL read + append)

---

### 3. PerformanceProfiler

**File**: `performance.py` (128 lines)

**Responsibilities**:
- **Latency Measurement**: Measure elapsed time for tool invocations
- **Statistical Aggregation**: Calculate P50, P95, P99 percentiles
- **Threshold Warnings**: Log when tools exceed 100ms target
- **Reporting**: Provide human-readable summaries

**Architecture**:
```
Tool Invocation → with profiler.profile(tool_name):
                    ├─ start_time = perf_counter()
                    ├─ Execute tool
                    └─ end_time = perf_counter()
                    ├─ latency_ms = (end - start) * 1000
                    └─ metrics[tool_name].append(latency_ms)
                                               ↓
                  get_stats() → Calculate:
                    ├─ Count, Avg, Min, Max
                    ├─ P95 (sorted[int(count * 0.95)])
                    ├─ P99 (sorted[int(count * 0.99)])
                    └─ exceeds_target (avg > 100ms)
```

**Context Manager Pattern**:
```python
@contextmanager
def profile(self, tool_name: str):
    start = time.perf_counter()
    try:
        yield
    finally:
        end = time.perf_counter()
        latency_ms = (end - start) * 1000.0
        self.metrics[tool_name].append(latency_ms)
```

**Usage**:
```python
# In EngramMCPServer._handle_call_tool()
with self.profiler.profile(tool_name):
    result = self.engram_retrieve.retrieve(query)
```

**Statistics Calculation**:
```python
def get_stats() -> Dict[str, Dict[str, float]]:
    sorted_latencies = sorted(latencies)
    p95_index = int(len(latencies) * 0.95)
    p99_index = int(len(latencies) * 0.99)

    return {
        "avg_ms": sum(latencies) / len(latencies),
        "p95_ms": sorted_latencies[p95_index],
        "p99_ms": sorted_latencies[p99_index],
        "exceeds_target": avg_ms > 100.0
    }
```

**Warning Thresholds**:
```python
if latency_ms > self.target_latency_ms:  # 100ms
    logger.warning(f"Tool {tool_name} exceeded target: {latency_ms:.2f}ms")
```

**Performance**: <1ms overhead per measurement (perf_counter is fast)

---

## Data Flow

### End-to-End Request Flow

**Scenario**: User asks Claude Code to search for "error handling patterns"

```
1. Claude Code (MCP Client)
   ├─ User query: "Find engrams about error handling"
   └─ Generate MCP request:
      {
        "jsonrpc": "2.0",
        "id": 42,
        "method": "tools/call",
        "params": {
          "name": "engram_retrieve",
          "arguments": {"query": "error handling", "top_k": 5}
        }
      }
                    ↓ (stdin)
2. EngramMCPServer.run()
   ├─ Read line from stdin
   ├─ Parse JSON
   └─ Call handle_request(request)
                    ↓
3. handle_request()
   ├─ Extract method: "tools/call"
   ├─ Extract params: {name: "engram_retrieve", arguments: {...}}
   └─ Route to _handle_call_tool(id=42, params)
                    ↓
4. _handle_call_tool()
   ├─ Extract tool_name: "engram_retrieve"
   ├─ Extract arguments: {"query": "error handling", "top_k": 5}
   └─ with profiler.profile("engram_retrieve"):
        result = self.engram_retrieve.retrieve(**arguments)
                    ↓
5. EngramRetrieve.retrieve()
   ├─ Validate inputs (query non-empty, top_k in 1-20)
   ├─ Find engram files (*.ai.md, *.why.md)
   ├─ Encode query: "error handling" → embedding (384-dim)
   ├─ For each engram:
   │  ├─ Get cached embedding OR encode (title + desc + content)
   │  └─ Calculate cosine_similarity(query_emb, engram_emb)
   ├─ Sort by score descending
   ├─ Take top 5 results
   └─ Return {"query": "...", "results": [...], "total_searched": 52}
                    ↓
6. _handle_call_tool() (continued)
   ├─ Serialize result to JSON
   ├─ Log performance: "engram_retrieve: 62.4ms (avg)"
   └─ Return response:
      {
        "jsonrpc": "2.0",
        "id": 42,
        "result": {
          "content": [{"type": "text", "text": "<JSON results>"}]
        }
      }
                    ↓ (stdout)
7. Claude Code (MCP Client)
   ├─ Parse response
   ├─ Extract results
   └─ Present to user:
      "Found 5 relevant engrams:
       1. error-handling.why.md (score: 0.54)
       2. error-wrapping.why.md (score: 0.49)
       ..."
```

**Latency Breakdown** (typical engram_retrieve call):
- JSON parsing: ~1ms
- Input validation: ~1ms
- Embedding query: ~20ms
- Cache lookups (52 engrams): ~5ms
- Sorting/ranking: ~1ms
- JSON serialization: ~2ms
- **Total**: ~30ms (cached), ~60ms (cold)

---

## Performance Architecture

### Caching Strategy

**Rationale**: Embedding computation is expensive (~20ms per file). Cache to amortize cost.

**Cache Design**:
```python
# In-memory dictionaries (EngramRetrieve)
_embedding_cache: Dict[str, np.ndarray] = {}
_engram_cache: Dict[str, Dict[str, Any]] = {}

# Invalidation: None (manual server restart or clear_cache())
# Trade-off: Stale data vs. performance (acceptable for engrams)
```

**Cache Hit Rate**:
- First call: 0% hit rate → ~60ms latency
- Subsequent calls: ~95% hit rate → ~10-20ms latency
- Assumption: Engrams change infrequently (cache valid for session)

**Memory Usage**:
- Embedding: 384 floats × 4 bytes = 1.5 KB per engram
- 100 engrams = 150 KB (negligible)
- Metadata: ~500 bytes per engram
- 100 engrams = 50 KB (negligible)
- **Total**: <1 MB for typical Engram repository

---

### Lazy Loading

**Rationale**: sentence-transformers model loading takes ~40ms. Delay until needed.

**Implementation**:
```python
class EngramRetrieve:
    def __init__(self, engram_root: Path):
        self.engram_root = engram_root
        # DO NOT load model here (too slow for server initialization)
        self.model = None  # Lazy-loaded on first retrieve()

    def retrieve(self, query: str):
        if self.model is None:
            logger.info("Loading sentence-transformers model...")
            self.model = SentenceTransformer('all-MiniLM-L6-v2')
        # ... rest of retrieval logic
```

**Benefits**:
- Fast server startup (~10ms vs ~50ms)
- Other tools (plugins_list, beads_create) don't pay model loading cost
- First engram_retrieve call pays one-time cost, subsequent calls fast

**Trade-off**: First call latency (~60ms) vs. startup time (prefer fast startup)

---

### Profiling Overhead

**Measurement**: PerformanceProfiler uses `time.perf_counter()` (nanosecond precision)

**Overhead**:
- `perf_counter()` call: ~50ns (0.00005ms)
- Two calls per measurement (start + end): ~100ns
- List append: ~50ns
- **Total**: <1ms per tool invocation (negligible)

**Verification**:
```python
# Benchmark profiler overhead
start = time.perf_counter()
for _ in range(10000):
    with profiler.profile("test"):
        pass
end = time.perf_counter()
# Result: ~1ms for 10,000 iterations → ~0.0001ms per iteration
```

---

## Error Handling

### Error Categories

| Category          | JSON-RPC Code | Example                                    | Handling                   |
|-------------------|---------------|--------------------------------------------|-----------------------------|
| Parse Error       | -32700        | Invalid JSON in request                    | Return error, log warning   |
| Method Not Found  | -32601        | Unknown method (e.g., "tools/delete")      | Return error                |
| Invalid Params    | -32602        | Validation failures (empty query, bad priority) | Return error with details |
| Internal Error    | -32603        | Tool execution failures, I/O errors        | Return error, log exception |

### Error Handling Flow

```
Request → handle_request() → try:
                               ├─ Method routing
                               └─ Tool invocation
                             except ValueError:
                               └─ _error_response(id, -32602, str(e))
                             except Exception:
                               └─ _error_response(id, -32603, str(e))
                                   ├─ logger.error(exc_info=True)
                                   └─ Sanitize error message (no stack traces)
                                             ↓
                             Return JSON-RPC error response
```

### Error Message Design

**Principles**:
1. **Actionable**: Tell user how to fix (not just "error occurred")
2. **Contextual**: Include relevant context (which parameter failed)
3. **Sanitized**: No sensitive data (file contents, credentials)
4. **Structured**: Consistent format across tools

**Examples**:

**Good**:
```json
{
  "error": {
    "code": -32602,
    "message": "Priority must be between 0 and 5, got 10"
  }
}
```

**Bad**:
```json
{
  "error": {
    "code": -32603,
    "message": "Error in beads_create.py line 42: ValueError"
  }
}
```

### Logging Strategy

**Levels**:
- **INFO**: Normal operations (tool invocations, performance stats)
- **WARNING**: Recoverable issues (missing metadata, slow tools)
- **ERROR**: Failures requiring investigation (exceptions, I/O errors)

**Destinations**:
- **stderr**: All logs (INFO, WARNING, ERROR)
- **stdout**: JSON-RPC responses ONLY (never logs)

**Log Format**:
```
2026-02-19 10:30:42 - engram-mcp-server.retrieve - INFO - Retrieving engrams for query: 'error handling'
2026-02-19 10:30:42 - engram-mcp-server.retrieve - WARNING - Tool engram_retrieve exceeded target latency: 120.5ms > 100ms
2026-02-19 10:30:42 - engram-mcp-server - ERROR - Tool execution error: Permission denied: /root/engram
```

---

## Testing Architecture

### Test Pyramid

```
       ┌────────────────┐
       │  Manual Tests  │  (Real Claude Code sessions)
       └────────────────┘
      ┌──────────────────┐
      │ Integration Tests │  (test_mcp_server.py: 8 cases)
      └──────────────────┘
    ┌──────────────────────┐
    │ Performance Benchmarks│  (benchmark_mcp_server.py: P95/P99)
    └──────────────────────┘
  ┌────────────────────────────┐
  │  Unit Tests (Future)        │  (test_tools/*.py: individual tool logic)
  └────────────────────────────┘
```

### Integration Tests (test_mcp_server.py)

**Architecture**:
```
Test Script → Start Server (subprocess):
                ├─ stdin: send JSON-RPC requests
                ├─ stdout: read JSON-RPC responses
                └─ stderr: server logs (ignored during tests)
                                 ↓
              For Each Test Case:
                ├─ Send request (JSON)
                ├─ Read response (JSON)
                ├─ Validate response structure
                └─ Assert expected result
                                 ↓
              Terminate Server → Print Summary
```

**Test Cases**:
1. Initialize handshake (verify server info)
2. List tools (verify all 4 tools present)
3. engram_retrieve: basic query
4. engram_retrieve: type filter (ai only)
5. engram_plugins_list: list plugins
6. wayfinder_phase_status: get phase (optional)
7. beads_create: create new bead
8. beads_create: detect duplicate
9. beads_create: invalid priority

**Assertions**:
```python
# Success case
assert 'result' in response
assert len(response['result']['tools']) == 4

# Error case
assert 'error' in response
assert response['error']['code'] == -32602
```

**Test Isolation**:
- Each test case independent (no shared state)
- Beads created during tests marked with "test" label
- No cleanup required (test beads can be deleted manually)

---

### Performance Benchmarks (benchmark_mcp_server.py)

**Architecture**:
```
For Each Tool:
  ├─ Run 20 iterations
  ├─ Measure latency per iteration
  ├─ Calculate P50, P95, P99
  └─ Compare against 100ms target
                     ↓
  Print Summary:
    Tool: engram_retrieve
      P50: 58.2ms ✅
      P95: 85.1ms ✅
      P99: 120.3ms ⚠️  (exceeds target)
```

**Benchmark Scenarios**:
1. **engram_retrieve (cold)**: First call (model loading)
2. **engram_retrieve (cached)**: Subsequent calls
3. **engram_plugins_list**: Filesystem scan
4. **beads_create**: JSONL append

**Statistical Rigor**:
- 20 iterations (sufficient for P95/P99 estimates)
- Warmup: First iteration discarded (outlier due to imports)
- Percentile calculation: Sorted array, index = count × percentile

---

## Security Architecture

### Threat Model

**Assumptions**:
- **Trusted Client**: MCP client (Claude Code) is trusted
- **Untrusted Input**: User-provided parameters may be malicious
- **Local Environment**: Server runs locally (no network exposure)

**Threats**:
1. **Path Traversal**: User provides `../../../etc/passwd` as project_path
2. **Command Injection**: User provides `; rm -rf /` in query
3. **Information Disclosure**: Error messages leak sensitive data
4. **Resource Exhaustion**: User requests top_k=999999 (DoS)

### Security Controls

#### 1. Input Validation

**Mechanism**: JSON schema validation (MCP protocol layer) + custom validation

**Examples**:
```python
# JSON Schema (MCP layer)
{
  "type": "integer",
  "minimum": 1,
  "maximum": 20  # Prevents resource exhaustion
}

# Custom validation (tool layer)
if not query.strip():
    raise ValueError("Query cannot be empty")

if not (0 <= priority <= 5):
    raise ValueError(f"Priority must be 0-5, got {priority}")
```

**Coverage**: All parameters validated before processing

---

#### 2. Path Validation

**Mechanism**: Restrict file access to known safe directories

**Implementation**:
```python
# Expand ~ but reject ../
project_path = Path(project_path).expanduser().resolve()

# Validate within allowed roots
allowed_roots = [
    Path.home() / "src/ws/oss/wf",  # Wayfinder projects
    Path.home() / "src/ws/oss/repos/engram",  # Engram repo
]

if not any(project_path.is_relative_to(root) for root in allowed_roots):
    raise ValueError("Project path outside allowed directories")
```

**Protection**: Prevents reading `/etc/passwd`, `/root/.ssh/id_rsa`, etc.

---

#### 3. Command Injection Prevention

**Mechanism**: No shell invocation (pure Python)

**Examples**:
```python
# SAFE: No subprocess, no eval, no exec
content = engram_path.read_text()  # Python file I/O
metadata = yaml.safe_load(frontmatter)  # Safe YAML parser

# UNSAFE (NOT USED):
# os.system(f"cat {engram_path}")  # Shell injection risk
# eval(user_input)  # Arbitrary code execution
```

**Coverage**: 100% (no shell commands in MCP server)

---

#### 4. Information Disclosure Prevention

**Mechanism**: Sanitize error messages (no stack traces to client)

**Implementation**:
```python
try:
    result = tool.execute(**arguments)
except Exception as e:
    # Log full exception (with stack trace) to stderr
    logger.error(f"Tool execution error: {e}", exc_info=True)

    # Return sanitized error to client (no stack trace)
    return _error_response(request_id, -32603, f"Tool execution failed: {e}")
```

**Protection**: Stack traces may reveal file paths, internal structure

---

## Deployment Architecture

### Deployment Model

```
┌─────────────────────────────────────────────────────────────┐
│                    User Environment                         │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Claude Code (Desktop App)                            │  │
│  │  - Reads ~/.claude/settings.json                      │  │
│  │  - Spawns MCP servers as subprocesses                 │  │
│  └───────────────────────┬──────────────────────────────┘  │
│                          │ (stdio)                          │
│  ┌───────────────────────▼──────────────────────────────┐  │
│  │  Engram MCP Server (Python subprocess)                │  │
│  │  - Working dir: ./engram/mcp-server  │  │
│  │  - Python: .venv/bin/python3                          │  │
│  │  - Env: ENGRAM_ROOT, BEADS_DB                         │  │
│  └───────────────────────┬──────────────────────────────┘  │
│                          │                                  │
│  ┌───────────────────────▼──────────────────────────────┐  │
│  │  Engram Repository                                    │  │
│  │  - engrams/ (knowledge base)                          │  │
│  │  - plugins/ (plugin directory)                        │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  Beads Database                                       │  │
│  │  - ~/.beads/issues.jsonl                              │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### MCP Registration

**Configuration File**: `~/.claude/settings.json`

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

**Lifecycle**:
1. Claude Code starts → reads settings.json
2. Spawns `python3 engram_mcp_server.py` as subprocess
3. Server runs in background (stdio transport)
4. Claude Code exits → kills subprocess (SIGTERM)

---

### Environment Configuration

**Environment Variables**:

| Variable     | Default                                 | Description                    |
|--------------|-----------------------------------------|--------------------------------|
| ENGRAM_ROOT  | ./engram               | Engram repository root         |
| BEADS_DB     | ~/.beads/issues.jsonl                   | Beads database path            |

**Alternative**: Command-line arguments (overrides env vars)

```bash
python3 engram_mcp_server.py \
  --engram-root ~/custom/engram \
  --beads-db ~/custom/beads.jsonl
```

---

## Future Extensibility

### Planned Extensions (Phase 4)

1. **Bead Editing**: Update existing beads (title, description, status)
2. **Bead Search**: Search beads by query, labels, status
3. **Engram Creation**: Create new engrams from MCP
4. **Real-Time Updates**: Subscribe to engram/bead changes (WebSocket?)
5. **Multi-Repo Support**: Search across multiple Engram repositories

### Architecture Support for Extensions

**Tool Addition** (easy):
- Add `tools/bead_search.py`
- Register in `_handle_list_tools()` and `_handle_call_tool()`
- No changes to core server logic

**Protocol Extensions** (moderate):
- Implement `notifications/subscribe` method
- Add WebSocket transport (parallel to stdio)
- Requires MCP protocol updates

**Multi-Tenancy** (complex):
- Support multiple Engram repositories
- Add `repository_id` parameter to all tools
- Requires EngramRetrieve refactoring (per-repo caches)

---

**Document Status**: Living documentation - updated as architecture evolves
**Review Cycle**: After major refactorings, before Phase 4 transition
**Approval**: Required for architectural changes impacting API compatibility
