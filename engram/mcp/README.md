# Engram MCP Server

Model Context Protocol (MCP) server providing programmatic access to Engram's memory retrieval, beads management, and Wayfinder workflow systems.

## Features

### Core Tools (Task 3.2 - Basic)

1. **engram.retrieve()** - Semantic memory retrieval with ecphory
   - Search engrams by query
   - Filter by type (.ai.md, .why.md)
   - Return top-k most relevant results

2. **engram.plugins.list()** - List available Engram plugins
   - Enumerate installed plugins
   - Show plugin metadata and status

3. **wayfinder.phase.status()** - Get Wayfinder project phase status
   - Query current phase
   - Get phase completion status
   - Access phase deliverables

### Enhanced Tools (Task 3.5 - Enhanced)

4. **beads.create()** - Programmatic bead creation
   - Create beads with title, description, priority, labels, estimate
   - Validate against duplicates
   - Return bead ID

5. **Advanced Ecphory** - Semantic search with embedding-based similarity
   - Embedding-based similarity search (using sentence-transformers)
   - Top-k retrieval with relevance scoring
   - Contextual ranking

6. **Performance Profiling** - Tool invocation latency tracking
   - Measure response times (<100ms target)
   - Log performance metrics
   - Optimize slow operations

## Installation

```bash
# Install dependencies
cd ./engram/mcp-server
pip install -r requirements.txt

# Test the server
python test_mcp_server.py
```

## Usage

### As MCP Server (stdio)

```bash
# Start server (stdio transport)
python engram_mcp_server.py

# Configure in ~/.claude/settings.json
{
  "mcpServers": {
    "engram": {
      "command": "python",
      "args": ["engram/mcp-server/engram_mcp_server.py"]
    }
  }
}
```

### Testing Tools

```bash
# List all tools
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | python engram_mcp_server.py

# Test engram.retrieve()
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"engram_retrieve","arguments":{"query":"error handling","top_k":3}}}' | python engram_mcp_server.py

# Test beads.create()
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"beads_create","arguments":{"title":"Fix bug","description":"Fix authentication bug","priority":1}}}' | python engram_mcp_server.py
```

## Architecture

```
mcp-server/
├── engram_mcp_server.py      # Main MCP server
├── tools/
│   ├── engram_retrieve.py    # Memory retrieval with embeddings
│   ├── beads_create.py       # Bead management
│   ├── plugins_list.py       # Plugin enumeration
│   └── wayfinder_status.py   # Wayfinder integration
├── performance.py            # Performance profiling
├── requirements.txt          # Dependencies
├── test_mcp_server.py        # Integration tests
└── README.md                 # This file
```

## Performance Targets

- Tool invocation latency: <100ms (measured via performance profiling)
- Embedding generation: <50ms (cached after first use)
- Bead validation: <20ms (database lookup)
- Wayfinder status: <30ms (file system read)

## API Documentation

See [ENGRAM-MCP-SERVER-API.md](./ENGRAM-MCP-SERVER-API.md) for complete API reference.

## Development

```bash
# Run tests
python test_mcp_server.py

# Run performance benchmarks
python benchmark_mcp_server.py

# Check latency
python -c "from performance import PerformanceProfiler; p = PerformanceProfiler(); print(p.get_stats())"
```

## Dependencies

- Python 3.9+
- sentence-transformers (for embeddings)
- sqlite3 (for beads)
- Standard library (json, sys, pathlib, subprocess)

## License

Part of the Engram project.
