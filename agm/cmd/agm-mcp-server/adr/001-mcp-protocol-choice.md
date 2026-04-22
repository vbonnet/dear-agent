# ADR 001: MCP Protocol Choice

## Status

Accepted

## Context

AGM sessions store metadata and conversation history in local filesystem directories. We need a way for Claude Code (and potentially other AI assistants) to discover and query AGM session metadata without directly accessing the filesystem or conversation content.

### Requirements

1. Enable Claude Code to list, search, and retrieve AGM session metadata
2. Maintain privacy by exposing only metadata, never conversation content
3. Use a standardized protocol for AI assistant interoperability
4. Minimize implementation complexity for V1
5. Support future enhancements (real-time updates, session modification)

### Options Considered

#### Option 1: Custom REST API

**Pros**:
- Familiar HTTP/JSON interface
- Rich ecosystem of tools and libraries
- Easy to test with curl/Postman
- Network-accessible (remote sessions)

**Cons**:
- Requires running an HTTP server (port management, security)
- Network overhead for local communication
- No standardized schema for AI assistant tools
- Requires custom authentication/authorization
- Overkill for local-only use case

#### Option 2: Custom CLI with JSON Output

**Pros**:
- Simple implementation (just print JSON)
- No server process required
- Direct filesystem access

**Cons**:
- No standardized tool discovery
- Claude Code would need custom parsing logic
- No protocol for input validation
- Not reusable across AI assistants
- Each query spawns new process (performance overhead)

#### Option 3: Model Context Protocol (MCP)

**Pros**:
- Standardized protocol designed for AI assistants
- Official support from Anthropic (Claude Code native integration)
- Stdio transport (no port management)
- Built-in tool discovery and schema validation
- Reusable across MCP-compatible clients
- Strong ecosystem (go-sdk, TypeScript SDK)
- Privacy-first design (local processes)

**Cons**:
- Newer protocol (less mature than HTTP)
- Limited to MCP-compatible clients
- Requires learning MCP SDK
- Stdio transport limits network accessibility

#### Option 4: gRPC

**Pros**:
- Efficient binary protocol
- Strong typing with Protocol Buffers
- Bidirectional streaming
- Rich ecosystem

**Cons**:
- Requires running gRPC server
- No native Claude Code integration
- Overkill for simple query operations
- Complex service definition
- Requires code generation

## Decision

We will use the **Model Context Protocol (MCP)** with stdio transport.

## Rationale

1. **Native Claude Code Integration**: MCP is designed by Anthropic specifically for Claude Code tool integration. This means zero-friction integration with the primary client.

2. **Privacy by Design**: MCP's stdio transport model (local process) aligns perfectly with AGM's privacy requirements. No network exposure, no authentication complexity.

3. **Standardization**: MCP provides a standard way to define tools, input schemas, and output formats. This makes the server reusable across any MCP-compatible client (not just Claude Code).

4. **Performance**: Stdio transport has minimal overhead. The MCP server runs as a child process of Claude Code, communicating via stdin/stdout with sub-millisecond latency.

5. **Future-Proof**: MCP supports advanced features we may need in V2:
   - Resources (expose session files as MCP resources)
   - Prompts (session templates)
   - Bidirectional communication (real-time updates)

6. **Official SDK**: The `github.com/modelcontextprotocol/go-sdk` provides a well-documented, type-safe implementation of the MCP protocol. This reduces implementation complexity.

7. **Developer Experience**: MCP's tool schema system provides automatic input validation and self-documenting APIs. Claude Code can discover available tools and their parameters without manual configuration.

## Consequences

### Positive

- **Seamless Integration**: Claude Code automatically discovers and uses AGM tools without custom client code
- **Type Safety**: MCP SDK provides compile-time type checking for tool inputs/outputs
- **Extensibility**: Easy to add new tools (V2 features) without breaking existing clients
- **Privacy**: Stdio transport ensures no network exposure
- **Performance**: Child process model avoids repeated startup costs (unlike CLI per query)
- **Reusability**: Any MCP client can use this server (e.g., future VSCode MCP plugin)

### Negative

- **MCP-Only**: Limited to MCP-compatible clients (not usable via curl/Postman)
- **Learning Curve**: Team needs to learn MCP protocol and SDK
- **Debugging**: Stdio transport harder to debug than HTTP (no browser DevTools)
- **Maturity Risk**: MCP is newer than HTTP/gRPC (potential for breaking changes)

### Neutral

- **Single Process**: Server runs per Claude Code session (not a long-running daemon)
- **Local Only**: Cannot query remote AGM sessions (future feature requires different transport)

## Implementation Notes

### MCP SDK Version

We will use `github.com/modelcontextprotocol/go-sdk@v1.2.0` which provides:
- `mcp.NewServer()` for server creation
- `mcp.AddTool()` for tool registration
- `mcp.StdioTransport` for stdio communication
- Automatic JSON-RPC 2.0 handling

### Tool Design

Each AGM operation maps to an MCP tool:
- `agm_list_sessions` (list with filters)
- `agm_search_sessions` (search by name)
- `agm_get_session_metadata` (get by ID)

### Transport Configuration

- **Transport**: stdio (stdin/stdout for RPC, stderr for logs)
- **Format**: JSON-RPC 2.0
- **Logging**: All logs to stderr (critical for stdio transport)

### Registration

V1: Manual registration via Claude Code settings
V2: Auto-registration by writing to `~/.config/claude/mcp_servers.json`

## Alternatives Considered for Future

If MCP proves insufficient, we can add alternative transports:
- HTTP transport for remote access (V2)
- WebSocket transport for real-time updates (V3)
- Unix socket transport for local IPC without stdio

These can coexist with the MCP stdio implementation.

## References

- MCP Specification: https://modelcontextprotocol.io
- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- Claude Code MCP Integration: https://docs.anthropic.com/claude-code/mcp
- Engram MCP Server: ./engram/main/plugins/mcp-server/ (reference implementation)

## Decision Date

2025-01-15

## Reviewers

- author
