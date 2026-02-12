# ADR-004: Tmux Integration Strategy

**Status:** Accepted
**Date:** 2026-01-10
**Deciders:** Foundation Engineering Team
**Related:** AGM legacy design decisions

---

## Context

AGM needs persistent session management that survives terminal disconnections, window manager crashes, and SSH session timeouts. Tmux provides multiplexing and session persistence, but how should AGM integrate with it?

### Requirements

**R1**: Sessions must persist across terminal disconnections
**R2**: Sessions must survive window manager crashes
**R3**: Users can work on multiple sessions concurrently
**R4**: Sessions can be resumed from any terminal
**R5**: Integration must be transparent (users shouldn't need tmux expertise)

### User Scenarios

**Scenario 1**: Developer creates session, gets disconnected, resumes later
**Scenario 2**: Developer switches between 3 active sessions
**Scenario 3**: Developer crashes terminal, sessions continue running

---

## Decision

We will **tightly integrate with tmux** as a required dependency, using tmux control mode for programmatic access.

**Architecture**:
1. **Tmux as Foundation**: Every AGM session is a tmux session (1:1 mapping)
2. **Control Mode**: Use `tmux -C` for programmatic control (not user-facing tmux)
3. **Session Naming**: AGM manages tmux session names (user-provided or auto-generated)
4. **Lock Management**: Global tmux lock prevents concurrent tmux commands
5. **Health Checking**: Cached health checks (5s cache) to minimize tmux overhead

---

## Alternatives Considered

### Alternative 1: Terminal-Based (No Tmux)

**Approach**: Run agent CLI directly in terminal, save PID to file

**Pros**:
- No tmux dependency
- Simple implementation
- Lower resource usage

**Cons**:
- ❌ Sessions die on terminal disconnect
- ❌ Can't resume sessions (PID doesn't survive reboots)
- ❌ Can't work on multiple sessions concurrently
- ❌ Doesn't meet R1, R2, R4

**Verdict**: Rejected. Fails core requirements.

---

### Alternative 2: Custom Daemon

**Approach**: AGM daemon manages sessions, clients connect to daemon

**Pros**:
- Full control over session lifecycle
- No tmux dependency
- Could integrate with systemd

**Cons**:
- ❌ Complex: Must implement multiplexing, persistence, IPC
- ❌ Reinvents tmux (25+ years of battle-testing)
- ❌ Platform-specific (systemd on Linux, launchd on macOS)
- ❌ Higher maintenance burden

**Verdict**: Rejected. Don't reinvent tmux.

---

### Alternative 3: Tight Tmux Integration (CHOSEN)

**Approach**: Every AGM session is a tmux session, use control mode for programmatic access

**Pros**:
- ✅ Meets all requirements (R1-R5)
- ✅ Battle-tested (tmux is rock-solid)
- ✅ Cross-platform (Linux, macOS, BSD)
- ✅ Users can use native tmux commands if needed
- ✅ Transparent (AGM handles tmux complexity)

**Cons**:
- ⚠️ tmux is required dependency
- ⚠️ Must handle tmux versioning
- ⚠️ Tmux socket contention under heavy load

**Verdict**: ACCEPTED. Best balance of reliability and simplicity.

---

### Alternative 4: Loose Tmux Integration

**Approach**: Optionally use tmux if available, fallback to terminal mode

**Pros**:
- Works without tmux
- Gradual adoption (users can try without tmux)

**Cons**:
- ❌ Two code paths (with/without tmux)
- ❌ Feature parity issues (some features only work with tmux)
- ❌ Testing complexity (must test both modes)
- ❌ Confusing UX (why does it work differently?)

**Verdict**: Rejected. Complexity not worth optional dependency.

---

## Implementation Details

### Tmux Control Mode

**Standard Tmux** (for users):
```bash
tmux attach-session -t my-session
```

**Control Mode** (for AGM):
```bash
tmux -C new-session -d -s my-session
```

**Benefits of Control Mode**:
- Machine-parseable output (not ANSI escape codes)
- Synchronous command execution (wait for completion)
- Error detection (exit codes, stderr)

**Usage in AGM**:
```go
func (t *TmuxClient) NewSession(name string) error {
    cmd := exec.Command("tmux", "-C", "new-session", "-d", "-s", name)
    return cmd.Run()
}

func (t *TmuxClient) HasSession(name string) (bool, error) {
    cmd := exec.Command("tmux", "-C", "has-session", "-t", name)
    err := cmd.Run()
    if err == nil {
        return true, nil
    }
    // Exit code 1 = session not found (expected)
    if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
        return false, nil
    }
    // Other errors (tmux not installed, socket issues)
    return false, err
}
```

---

### Session Naming Convention

**User-Provided Names**:
```bash
agm new my-project        # Tmux session: "my-project"
agm new feature-xyz       # Tmux session: "feature-xyz"
```

**Auto-Generated Names** (if not provided):
```bash
agm new                   # Prompt user for name
# Tmux session: "<user-input>"
```

**Validation Rules**:
- No spaces (tmux limitation)
- No special characters (`:`, `.`, `$` reserved by tmux)
- Max 128 characters
- Must be unique across all sessions

---

### Lock Management

**Problem**: Tmux commands can race (e.g., two `agm new` calls simultaneously)

**Solution**: Global tmux lock

```go
// File: internal/tmux/lock.go
type TmuxLock struct {
    path    string       // /tmp/agm-tmux.lock
    timeout time.Duration // 5 seconds default
}

func AcquireTmuxLock() (*TmuxLock, error) {
    lock := &TmuxLock{
        path:    "/tmp/agm-tmux.lock",
        timeout: 5 * time.Second,
    }
    return lock.Acquire()
}

func (l *TmuxLock) Release() error {
    return os.Remove(l.path)
}
```

**Lock Scope**: Global (not per-session)
- Prevents concurrent tmux server modifications
- Minimal contention (tmux commands are fast)
- Auto-release on process exit (defer pattern)

**Lock-Free Operations**:
- Reading manifest (doesn't touch tmux)
- Listing sessions (read-only tmux command)

---

### Health Checking

**Problem**: `tmux has-session` is slow (10-20ms per call), `agm list` would be too slow

**Solution**: Cached health checks

```go
type HealthChecker struct {
    cache      map[string]CacheEntry
    cacheTTL   time.Duration // 5 seconds
    mu         sync.RWMutex
}

type CacheEntry struct {
    status    string
    timestamp time.Time
}

func (h *HealthChecker) GetStatus(sessionName string) string {
    h.mu.RLock()
    entry, exists := h.cache[sessionName]
    h.mu.RUnlock()

    if exists && time.Since(entry.timestamp) < h.cacheTTL {
        return entry.status // Cache hit
    }

    // Cache miss: Check tmux
    status := h.checkTmuxSession(sessionName)

    h.mu.Lock()
    h.cache[sessionName] = CacheEntry{status, time.Now()}
    h.mu.Unlock()

    return status
}
```

**Cache TTL**: 5 seconds (balances accuracy vs performance)
**Cache Invalidation**: Time-based only (no explicit invalidation)

---

### Tmux Version Compatibility

**Minimum Version**: 3.0 (released 2019)

**Version Detection**:
```go
func GetTmuxVersion() (string, error) {
    out, err := exec.Command("tmux", "-V").Output()
    if err != nil {
        return "", err
    }
    // Output: "tmux 3.2a"
    version := strings.TrimPrefix(string(out), "tmux ")
    return strings.TrimSpace(version), nil
}

func ValidateTmuxVersion() error {
    version, err := GetTmuxVersion()
    if err != nil {
        return fmt.Errorf("tmux not installed: %w", err)
    }

    if !versionAtLeast(version, "3.0") {
        return fmt.Errorf("tmux 3.0+ required, found %s", version)
    }

    return nil
}
```

**Checked When**:
- `agm doctor --validate`
- Before creating first session (lazy validation)

---

### Output Capture (for Message Sending)

**Use Case**: `agm send` must send messages to session via tmux

**Implementation**:
```go
func (t *TmuxClient) SendKeys(sessionName, text string) error {
    cmd := exec.Command("tmux", "send-keys", "-t", sessionName, text, "C-m")
    return cmd.Run()
}
```

**`C-m`**: Carriage return (simulates Enter key)

**Quote Escaping**:
```go
func escapeForTmux(text string) string {
    // Escape single quotes for tmux
    return strings.ReplaceAll(text, "'", "'\\''")
}
```

---

## Consequences

### Positive

✅ **Rock-Solid Persistence**: Sessions survive crashes, reboots, disconnects
✅ **Battle-Tested**: Tmux has 25+ years of stability
✅ **Cross-Platform**: Linux, macOS, BSD (all major platforms)
✅ **User Control**: Users can use native tmux commands if needed
✅ **No Reinvention**: Leverage existing, proven technology

### Negative

⚠️ **Required Dependency**: Users must install tmux (not default on all systems)
⚠️ **Version Compatibility**: Must support tmux 3.0+ (older versions may break)
⚠️ **Socket Contention**: Under heavy load, tmux socket can be bottleneck
⚠️ **Platform Quirks**: Tmux behavior differs slightly on macOS vs Linux

### Neutral

🔄 **Tmux Expertise**: Users don't need tmux knowledge, but can use it if they have it
🔄 **Resource Usage**: Tmux adds ~10MB RAM per session (acceptable overhead)

---

## Mitigations

**Required Dependency**:
- Clear installation instructions in docs
- `agm doctor --validate` checks tmux availability
- Package managers (brew, apt) make installation trivial

**Version Compatibility**:
- Version check at startup (`agm doctor`)
- Error message includes upgrade instructions
- CI tests against tmux 3.0, 3.1, 3.2, 3.3

**Socket Contention**:
- Global lock prevents concurrent modifications
- Health check caching reduces tmux calls
- Lock timeout (5s) prevents deadlocks

**Platform Quirks**:
- Integration tests on Linux and macOS
- Document platform-specific issues
- Fallback logic for platform differences

---

## Validation

**BDD Scenarios**:
- Create session → tmux session exists
- Kill terminal → session still running
- Resume session → attaches to tmux session
- Multiple sessions → all run concurrently

**Integration Tests**:
- Tmux 3.0, 3.1, 3.2, 3.3 compatibility
- Linux and macOS platforms
- Socket permission issues
- Concurrent session creation

**Performance Tests**:
- `agm list` < 100ms for 10 sessions
- Health check cache hit rate > 80%
- Lock contention < 1% under normal load

---

## Related Decisions

- **ADR-005**: Manifest Schema (tmux_session_name field)
- **ADR-006**: Lock Management Strategy (global tmux lock)
- **AGM Legacy**: Original tmux integration (this ADR formalizes it)

---

## Future Considerations

**v3.1+**:
- Support tmux 2.x with feature detection (degrade gracefully)
- Parallel tmux operations (batch commands to reduce overhead)
- Tmux plugin for richer integration (status bar, key bindings)

**v4.0+**:
- Alternative backends (GNU Screen, Zellij) via adapter pattern
- Headless mode (no terminal multiplexer, daemon only)

---

## References

- **Tmux Documentation**: https://github.com/tmux/tmux/wiki
- **Control Mode**: https://github.com/tmux/tmux/wiki/Control-Mode
- **Similar Tools**:
  - screen (older multiplexer)
  - Zellij (newer Rust-based multiplexer)

---

**Implementation Status:** ✅ Complete (Inherited from AGM, formalized in AGM v3.0)
**Date Formalized:** 2026-01-10
