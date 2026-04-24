# Engram MCP Server - Quick Start Guide

Get started with the Engram MCP Server in 5 minutes.

## Step 1: Install Dependencies

```bash
cd ./engram/mcp-server
pip install -r requirements.txt
```

**Note**: First run downloads sentence-transformers model (~90MB). This happens once.

## Step 2: Test the Server

```bash
# Run integration tests
./test_mcp_server.py

# Expected output:
# ✅ Server initialized
# ✅ All tests passed!
```

## Step 3: Run Performance Benchmarks

```bash
# Measure tool latency
./benchmark_mcp_server.py

# Expected output:
# ✅ All performance targets met!
# engram_retrieve:           42.5ms
# beads_create:              18.7ms
# ...
```

## Step 4: Configure Claude Code

Edit `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "engram": {
      "command": "python",
      "args": [
        "engram/mcp-server/engram_mcp_server.py"
      ]
    }
  }
}
```

## Step 5: Use the Tools

### Example 1: Semantic Search

```
You: Find engrams about error handling patterns in Go

Claude Code uses engram_retrieve:
{
  "query": "error handling patterns go",
  "top_k": 5
}

Returns:
- engrams/patterns/go/error-handling.ai.md (score: 0.87)
- engrams/patterns/go/error-wrapping.why.md (score: 0.82)
- ...
```

### Example 2: Create a Bead

```
You: Create a bead to fix the authentication bug

Claude Code uses beads_create:
{
  "title": "Fix authentication bug in OIDC flow",
  "description": "Users unable to login with OIDC providers...",
  "priority": 0,
  "labels": ["bug", "auth", "p0"]
}

Returns:
{
  "bead_id": "engram-a3f",
  "status": "created"
}
```

### Example 3: Check Wayfinder Status

```
You: What's the current phase of the batch-edit project?

Claude Code uses wayfinder_phase_status:
{
  "project_path": "the git history"
}

Returns:
{
  "current_phase": "S6",
  "phase_name": "Implementation Spec",
  "completion_status": "in_progress",
  "next_phase": "S8"
}
```

### Example 4: List Plugins

```
You: What Engram plugins are installed?

Claude Code uses engram_plugins_list:
{}

Returns:
- beads-connector (go, v1.0.0)
- multi-persona-review (typescript, v0.2.0)
- ...
```

## Troubleshooting

### Server won't start

```bash
# Check Python version (need 3.9+)
python --version

# Install dependencies
pip install -r requirements.txt

# Test manually
python engram_mcp_server.py
# Then send: {"jsonrpc":"2.0","id":1,"method":"initialize"}
```

### Slow first query

**Expected**: First `engram_retrieve` query takes ~1-2 seconds (model loading + embedding generation).

**Subsequent queries**: <100ms (cached embeddings).

### "Duplicate bead" error

```
Error: Duplicate bead found: 'Fix auth bug' (ID: engram-xyz)
```

**Solution**: Either:
1. Use a different title
2. Close the existing bead first
3. Update the existing bead instead

### Wayfinder project not found

```
Error: Not a Wayfinder project (missing .wayfinder, SPEC.md, or ROADMAP.md)
```

**Solution**: Ensure you're pointing to a valid Wayfinder project directory with one of:
- `.wayfinder` file
- `SPEC.md` file
- `ROADMAP.md` file

## Next Steps

1. **Read API docs**: [ENGRAM-MCP-SERVER-API.md](./ENGRAM-MCP-SERVER-API.md)
2. **Review implementation**: [IMPLEMENTATION.md](./IMPLEMENTATION.md)
3. **Run benchmarks**: `./benchmark_mcp_server.py`
4. **Integrate with workflows**: Use tools in Claude Code sessions

## Support

- **Documentation**: See README.md and ENGRAM-MCP-SERVER-API.md
- **Tests**: `./test_mcp_server.py` for validation
- **Benchmarks**: `./benchmark_mcp_server.py` for performance
- **Issues**: Report in engram repository

---

**Ready!** Your Engram MCP Server is now configured and ready to use.
