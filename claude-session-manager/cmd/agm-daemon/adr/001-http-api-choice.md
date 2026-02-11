# ADR 001: HTTP API for State Exposure

**Status**: Accepted

**Date**: 2026-02-11

## Context

The AGM Daemon needs to expose session state information to external tools and scripts. Several approaches were considered:

1. **HTTP API**: RESTful endpoints on localhost
2. **CLI Commands**: `agm-daemon status <session>` commands
3. **File-Only**: Only write status JSON files, no API
4. **Unix Sockets**: Domain socket IPC
5. **D-Bus/IPC**: System message bus integration

### Requirements

- **Programmatic Access**: Tools need to query states programmatically
- **Low Latency**: Sub-second response times
- **Language Agnostic**: Works from any language/tool
- **Simple Integration**: Minimal client-side dependencies
- **Real-Time Queries**: Fresh state information on demand

### Constraints

- Daemon runs as background service
- Must work in SSH/remote environments
- No external dependencies (databases, message queues)
- Security: local access only, no authentication needed

## Decision

**Use HTTP API on localhost:8765 with RESTful endpoints.**

### API Design

```
GET /status              → All sessions
GET /status/{name}       → Single session
GET /health              → Server health
```

### Response Format

```json
{
  "session_name": "session-1",
  "state": "ready",
  "timestamp": "2026-02-11T10:30:00Z",
  "evidence": "Claude prompt (❯) detected",
  "confidence": "high",
  "last_updated": "2026-02-11T10:30:01Z"
}
```

## Rationale

### Why HTTP API?

1. **Universal Protocol**: HTTP works everywhere
   - Shell scripts: `curl http://localhost:8765/status`
   - Python: `requests.get()`
   - JavaScript: `fetch()`
   - Go: `http.Get()`

2. **RESTful Design**: Intuitive resource-based endpoints
   - `/status` = all sessions
   - `/status/{name}` = specific session
   - Standard HTTP methods (GET)

3. **JSON Response**: Standard data format
   - Easy parsing in all languages
   - Human-readable
   - Type-safe schema

4. **Real-Time Queries**: No polling files
   - Client requests → immediate response
   - Always fresh data (from in-memory cache)
   - Sub-10ms response times

5. **Simple Server**: Go standard library
   - No external dependencies
   - `net/http` package handles all routing
   - Built-in JSON encoding

### Why Not CLI Commands?

**Rejected**: CLI commands would require subprocess spawning for each query.

**Problems**:
- High overhead (process creation per query)
- Slower than HTTP (100ms+ vs 10ms)
- Harder to integrate in monitoring loops
- Requires parsing stdout (less structured than JSON)

### Why Not File-Only?

**Rejected**: File-only interface would require polling files for changes.

**Problems**:
- Stale data (file written every 2s, not on demand)
- Polling overhead (client must stat files repeatedly)
- Race conditions (file mid-write)
- No central query interface

**Note**: Files are still provided as supplementary interface (see ADR 003).

### Why Not Unix Sockets?

**Rejected**: Unix sockets add complexity without clear benefit.

**Problems**:
- Less universal than HTTP (not all languages have good socket clients)
- Custom protocol required (vs standard HTTP)
- No advantage over localhost HTTP in performance
- Harder to test/debug (can't use `curl`)

### Why Not D-Bus?

**Rejected**: D-Bus is overkill for simple state queries.

**Problems**:
- Heavy dependency (D-Bus daemon)
- Complex API (introspection, type marshaling)
- Linux-only (not portable to macOS)
- Harder to test/debug

## Consequences

### Positive

1. **Easy Integration**: Any tool can query via HTTP
   ```bash
   curl http://localhost:8765/status/my-session | jq '.state'
   ```

2. **Low Latency**: In-memory cache → <10ms responses
   - No disk I/O for queries
   - Thread-safe concurrent reads

3. **Standard Tooling**: Works with existing HTTP clients
   - `curl`, `wget`, `httpie` for testing
   - Standard HTTP libraries in all languages

4. **Discoverable API**: RESTful design is intuitive
   - `/status` → list all
   - `/status/{name}` → get one
   - `/health` → check health

5. **Future Extensibility**: Easy to add new endpoints
   - POST endpoints for session control (V2)
   - WebSocket for real-time updates (V2)
   - Metrics endpoint for monitoring (V2)

### Negative

1. **Port Binding**: Requires port 8765 to be available
   - **Mitigation**: Configurable via `-port` flag
   - **Impact**: Minimal (unlikely conflict on localhost)

2. **No Authentication**: Anyone on localhost can query
   - **Mitigation**: Bind to 127.0.0.1 only (no network exposure)
   - **Impact**: Acceptable (local process, trusted environment)

3. **HTTP Overhead**: JSON encoding/decoding per request
   - **Mitigation**: Negligible (<1ms) for small payloads
   - **Impact**: Minimal (in-memory cache, simple JSON)

4. **Server Lifecycle**: Must start/stop HTTP server gracefully
   - **Mitigation**: Go's `http.Server.Shutdown()` handles gracefully
   - **Impact**: Well-solved problem, standard pattern

### Neutral

1. **Port Standardization**: Port 8765 is semi-arbitrary
   - Chosen to avoid common ports (8080, 8000, etc.)
   - Configurable if conflicts arise

2. **HTTP/1.1 Only**: No HTTP/2 support
   - Not needed for simple GET requests
   - Can be added later if needed

## Alternatives Considered

### Option 1: CLI Commands (Rejected)

```bash
agm-daemon status my-session
```

**Why Rejected**:
- Subprocess overhead (100ms+ per query)
- Requires daemon to parse args and format output
- Harder to integrate in monitoring loops

### Option 2: File-Only Interface (Rejected)

```bash
cat ~/.agm/status/my-session.json | jq '.state'
```

**Why Rejected**:
- Stale data (files updated every 2s)
- No central query point
- Polling overhead for clients

**Note**: Files still provided as supplementary interface (ADR 003).

### Option 3: gRPC (Rejected)

```proto
service AGMDaemon {
  rpc GetStatus(SessionRequest) returns (SessionResponse);
}
```

**Why Rejected**:
- Requires protobuf compiler and codegen
- Heavier dependencies than standard HTTP
- Less universal tooling (can't use `curl`)

## References

- Go HTTP Server: https://pkg.go.dev/net/http
- RESTful API Design: https://restfulapi.net/
- State Detection: internal/state/detector.go
- Server Implementation: internal/api/server.go

## Related ADRs

- ADR 003: Dual Interface Design (HTTP + File)
- ADR 002: Polling-Based State Detection
