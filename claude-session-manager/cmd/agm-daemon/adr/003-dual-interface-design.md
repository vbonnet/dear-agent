# ADR 003: Dual Interface Design (HTTP + File)

**Status**: Accepted

**Date**: 2026-02-11

## Context

The AGM Daemon needs to expose session state information to various external consumers. Different consumers have different access patterns and requirements:

1. **Programmatic Tools**: Need structured data, low latency, language-agnostic
2. **Shell Scripts**: Prefer simple file reads, standard Unix tools
3. **Tmux Status Bar**: Needs fast file reads, no subprocess overhead
4. **Monitoring Systems**: Need HTTP endpoints for health checks

Two primary interface options were considered:

1. **Single Interface**: Either HTTP API or file-based status
2. **Dual Interface**: Both HTTP API and file-based status

### Requirements

- **Programmatic Access**: Tools query via HTTP for real-time data
- **Shell Integration**: Scripts read files with `cat`, `jq`
- **Low Latency**: Sub-second access for both interfaces
- **Simplicity**: Minimal client-side dependencies
- **Reliability**: Redundant access methods

### Constraints

- Daemon runs as background service
- Status updates every 2s (polling interval)
- Must work in SSH/remote environments
- No external dependencies

## Decision

**Provide both HTTP API and file-based status updates.**

### HTTP Interface

```bash
# Query all sessions
curl http://localhost:8765/status

# Query specific session
curl http://localhost:8765/status/my-session
```

**Response**: JSON (in-memory cache, <10ms latency)

### File Interface

```bash
# Read session status
cat ~/.agm/status/my-session.json

# Parse with jq
cat ~/.agm/status/my-session.json | jq '.state'
```

**Format**: JSON (written every poll, ~2s freshness)

### Data Flow

```
Monitoring Loop (every 2s)
    ↓
Detect state
    ↓
    ├─→ Update HTTP cache (in-memory)
    └─→ Write status file (atomic)
```

## Rationale

### Why Both Interfaces?

1. **Different Use Cases**

**HTTP Best For**:
- Programmatic querying (Python, Go, JavaScript)
- Real-time queries (immediate response)
- Centralized access (single endpoint for all sessions)
- Language-agnostic integration

**Files Best For**:
- Shell scripts (`cat ~/.agm/status/*.json`)
- Tmux status bar integration (no subprocess overhead)
- Offline access (daemon not running)
- Simple debugging (`cat` vs `curl`)

2. **Complementary Strengths**

| Feature | HTTP | Files |
|---------|------|-------|
| Latency | <10ms (in-memory) | <1ms (local file) |
| Freshness | Real-time (current cache) | ~2s (last write) |
| Discovery | Single endpoint `/status` | Glob `~/.agm/status/*.json` |
| Dependencies | HTTP client | None (cat, jq) |
| Works Offline | No (daemon must run) | Yes (stale data) |

3. **Redundancy for Reliability**

If daemon crashes:
- HTTP: Unavailable
- Files: Last known state available

If filesystem slow:
- HTTP: Still fast (in-memory)
- Files: May be slow/stale

4. **Cost Is Low**

**Overhead of Dual Interface**:
- HTTP cache: ~1KB per session (already needed for API)
- File writes: ~500 bytes per session per 2s
- Implementation: ~50 lines of code

**Benefit**:
- Supports all integration patterns
- No client forced to use unsuitable interface

### Why Not HTTP Only?

**Problem**: Requires daemon running for all access

**Use Cases Broken**:
```bash
# Tmux status bar (can't spawn curl in tight loop)
set -g status-right "#(curl -s http://localhost:8765/status/#{session_name} | jq -r .state)"
# Problem: curl overhead (~50ms), can't run every status refresh

# Shell script (requires daemon)
if agm-daemon not running → can't check state
```

### Why Not Files Only?

**Problem**: Stale data (written every 2s, not on demand)

**Use Cases Broken**:
```bash
# Real-time query (wrong result)
curl http://localhost:8765/status/session-1  # ❌ Not available

# Monitoring dashboard (polling files inefficient)
while true; do cat ~/.agm/status/*.json; sleep 1; done
# Problem: Wasteful file I/O, 1s behind anyway
```

## Consequences

### Positive

1. **Universal Access**: All tools can integrate
   ```bash
   # HTTP (programmatic)
   curl http://localhost:8765/status | jq '.sessions[].state'

   # Files (shell)
   cat ~/.agm/status/*.json | jq -s '.[].state'
   ```

2. **Performance Optimized**: Each interface optimized for use case
   - HTTP: In-memory cache → <10ms
   - Files: Local read → <1ms

3. **Fault Tolerance**: Redundant access methods
   - Daemon down? Files have last known state
   - Filesystem slow? HTTP still fast

4. **Simple Integration**: Choose interface that fits
   - Python/Go? Use HTTP
   - Bash/tmux? Use files
   - Both? Use both!

5. **Zero Conflicts**: Interfaces don't interfere
   - HTTP reads from cache
   - Files written independently
   - No locking between them

### Negative

1. **Dual Maintenance**: Must maintain both interfaces
   - **Mitigation**: Single source of truth (DetectionResult)
   - **Impact**: ~50 lines of code for file writer
   - **Benefit**: Both interfaces share same data model

2. **Disk I/O Overhead**: Writing files every 2s
   - **Impact**: 10 sessions × 500 bytes/2s = 5KB/2s = 2.5KB/s
   - **Mitigation**: Atomic writes (temp + rename), minimal data
   - **Result**: Negligible overhead

3. **Potential Staleness**: Files lag HTTP by up to 2s
   - **Impact**: File reads may be 0-2s stale
   - **Mitigation**: Documented in SPEC.md
   - **Result**: Acceptable for file-based use cases

4. **Directory Management**: Must create/clean ~/.agm/status/
   - **Impact**: Auto-created on startup, files deleted on session end
   - **Mitigation**: Well-defined lifecycle
   - **Result**: Standard practice (similar to /tmp)

### Neutral

1. **Two Documentation Paths**: Must document both interfaces
   - HTTP: API reference in SPEC.md
   - Files: Format documented in SPEC.md
   - Both: Clear guidance on when to use each

2. **Testing Complexity**: Must test both interfaces
   - HTTP: Endpoint response tests
   - Files: File I/O tests
   - Both: Integration tests verify consistency

## Implementation Details

### Status File Format

**Path**: `~/.agm/status/{session-name}.json`

**Content**: Same as HTTP response (StatusResponse struct)

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

### Atomic File Writes

```go
func (w *StatusFileWriter) WriteStatus(sessionName, result) error {
    // 1. Marshal JSON
    data := json.MarshalIndent(status)

    // 2. Write to temp file
    tempPath := filePath + ".tmp"
    os.WriteFile(tempPath, data, 0644)

    // 3. Atomic rename
    os.Rename(tempPath, filePath)

    // 4. Cleanup on error
    defer os.Remove(tempPath)
}
```

**Benefit**: Readers never see partial writes

### File Permissions

- **Files**: `0644` (user read/write, group/other read)
- **Directory**: `0755` (user full, group/other read+execute)

**Rationale**: Public read for integration, write restricted to daemon

## Use Case Examples

### Example 1: Tmux Status Bar (Files)

```bash
# ~/.tmux.conf
set -g status-right "#(cat ~/.agm/status/#{session_name}.json 2>/dev/null | jq -r '.state // \"unknown\"')"
```

**Why Files**:
- Fast (`cat` < 1ms, no subprocess overhead)
- No daemon dependency (shows stale if daemon down)
- Simple (standard Unix tools)

### Example 2: Monitoring Dashboard (HTTP)

```python
import requests

response = requests.get("http://localhost:8765/status")
sessions = response.json()["sessions"]

for session in sessions:
    print(f"{session['session_name']}: {session['state']}")
```

**Why HTTP**:
- Real-time data (current cache)
- Structured response (JSON parsing)
- Centralized (single request for all sessions)

### Example 3: Shell Script (Files)

```bash
#!/bin/bash
# Check if session is ready before sending command

state=$(cat ~/.agm/status/my-session.json 2>/dev/null | jq -r '.state')

if [ "$state" = "ready" ]; then
    echo "Session ready, sending command..."
    agm session send my-session --prompt "some command"
else
    echo "Session busy (state: $state), waiting..."
fi
```

**Why Files**:
- Simple (no HTTP client needed)
- Fast (local file read)
- Works offline (stale data better than no data)

### Example 4: Health Check (HTTP)

```bash
#!/bin/bash
# Systemd health check

curl -f http://localhost:8765/health || exit 1
```

**Why HTTP**:
- Verifies daemon is running (file would show stale)
- Standard health check pattern
- Simple (curl -f checks status code)

## Consistency Guarantees

### Data Consistency

Both interfaces return same data at any point:
```
HTTP cache ≡ Status files (modulo 0-2s delay)
```

**Mechanism**:
1. Single source: DetectionResult
2. Parallel updates: HTTP cache + file write
3. Same struct: StatusResponse

### Temporal Consistency

Files may be slightly stale:
- HTTP: Immediate (in-memory cache)
- Files: Last write (0-2s ago)

**Acceptable**: File use cases don't require instant freshness

## Migration Path (Future)

### V2: Unified Freshness

Add file watch + push writes for instant freshness:
```
State change detected
    ↓
    ├─→ Update HTTP cache (immediate)
    └─→ Write status file (immediate, not wait 2s)
```

**Benefit**: Files always fresh (no 2s lag)

### V2: WebSocket Push

Add WebSocket for real-time updates:
```
Client connects → receives state changes as events
```

**Benefit**: Sub-second updates without polling

## References

- HTTP API: internal/api/server.go
- File Writer: internal/api/status_file.go
- Atomic Writes: Go os.Rename() atomicity guarantee

## Related ADRs

- ADR 001: HTTP API for State Exposure
- ADR 002: Polling-Based State Detection
