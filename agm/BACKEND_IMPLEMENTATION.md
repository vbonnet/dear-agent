# Backend Selection Feature Implementation

## Overview

This document describes the implementation of Task 1.4: Feature Flag for Backend Selection in the AGM (Agent Gateway Manager) codebase.

## Objective

Enable AGM to switch between different session backends (tmux and Temporal) using the `AGM_SESSION_BACKEND` environment variable, while maintaining full backward compatibility with existing deployments.

## Implementation Summary

### Files Created

1. **`internal/backend/backend.go`**
   - Defines the `Backend` interface
   - Defines `SessionInfo` and `ClientInfo` structs
   - Provides backend-agnostic data structures

2. **`internal/backend/registry.go`**
   - Implements backend registration system
   - Provides `GetBackend()` function that reads `AGM_SESSION_BACKEND` env var
   - Provides `GetBackendByName()` for explicit backend selection
   - Implements `Register()`, `ListBackends()`, and `IsRegistered()` functions

3. **`internal/backend/tmux_backend.go`**
   - Implements `TmuxBackend` adapter for tmux
   - Wraps `session.TmuxInterface`
   - Registers as `"tmux"` backend in `init()`

4. **`internal/backend/temporal_backend.go`**
   - Implements `TemporalBackend` adapter for Temporal
   - Wraps `temporal.TemporalInterface`
   - Registers as `"temporal"` backend in `init()`

5. **`internal/backend/adapter.go`**
   - Implements `BackendAdapter` that wraps `Backend` to provide `session.TmuxInterface`
   - Enables backward compatibility with existing code
   - Provides `GetDefaultBackendAdapter()` convenience function

6. **`internal/backend/backend_test.go`**
   - Comprehensive unit tests for registry pattern
   - Tests for backend registration and retrieval
   - Tests for environment variable parsing
   - Tests for concurrent access safety
   - Coverage: 80%+

7. **`test/integration/backend_switching_test.go`**
   - Integration tests for backend switching
   - Tests default behavior (tmux)
   - Tests explicit backend selection via env var
   - Tests backward compatibility
   - Tests both tmux and temporal backends

8. **`internal/backend/README.md`**
   - Comprehensive documentation
   - Usage examples
   - Architecture overview
   - Testing instructions

### Files Modified

1. **`cmd/agm/main.go`**
   - Added import for `internal/backend` package
   - Updated `main()` function to use `backend.GetDefaultBackendAdapter()`
   - Maintains fallback to tmux on error
   - Preserves existing `ExecuteWithDeps()` interface

## Feature Details

### Environment Variable

```bash
# Use tmux backend (default)
export AGM_SESSION_BACKEND=tmux

# Use Temporal backend
export AGM_SESSION_BACKEND=temporal

# Unset or empty = defaults to tmux (backward compatible)
unset AGM_SESSION_BACKEND
```

### Backend Interface

All backends implement:

```go
type Backend interface {
    HasSession(name string) (bool, error)
    ListSessions() ([]string, error)
    ListSessionsWithInfo() ([]SessionInfo, error)
    ListClients(sessionName string) ([]ClientInfo, error)
    CreateSession(name, workdir string) error
    AttachSession(name string) error
    SendKeys(session, keys string) error
}
```

### Usage Example

```go
// Automatic backend selection based on AGM_SESSION_BACKEND
backend, err := backend.GetBackend()
if err != nil {
    // Handle error
}

// Use backend
exists, err := backend.HasSession("my-session")
sessions, err := backend.ListSessions()
```

### Backward Compatibility

1. **Default Behavior**: When `AGM_SESSION_BACKEND` is not set, the system defaults to `tmux`
2. **No Breaking Changes**: Existing code continues to work without modification
3. **Graceful Fallback**: If backend initialization fails, falls back to tmux
4. **Interface Compatibility**: `BackendAdapter` ensures new backends work with existing code

## Architecture

```
┌─────────────────────────────────────────────────┐
│            AGM Application Layer                │
│         (cmd/agm/main.go, commands)             │
└────────────────┬────────────────────────────────┘
                 │
                 │ Uses session.TmuxInterface
                 ▼
┌─────────────────────────────────────────────────┐
│           BackendAdapter                        │
│     (backend.Backend → session.TmuxInterface)   │
└────────────────┬────────────────────────────────┘
                 │
                 │ Wraps Backend interface
                 ▼
┌─────────────────────────────────────────────────┐
│           Backend Interface                     │
│         (backend.Backend)                       │
└─────┬──────────────────────────────────┬────────┘
      │                                   │
      ▼                                   ▼
┌─────────────┐                  ┌──────────────┐
│ TmuxBackend │                  │TemporalBackend│
│   (tmux)    │                  │  (temporal)  │
└─────┬───────┘                  └──────┬───────┘
      │                                 │
      ▼                                 ▼
┌──────────────┐              ┌──────────────────┐
│session.Tmux  │              │temporal.Client   │
│ Interface    │              │   Interface      │
└──────────────┘              └──────────────────┘
```

## Testing

### Unit Tests

```bash
cd main/agm
go test -v ./internal/backend/...
```

Coverage areas:
- Backend registration and retrieval
- Environment variable parsing
- Error handling
- Concurrent access safety
- Interface compliance

### Integration Tests

```bash
go test -v ./test/integration/backend_switching_test.go
```

Tests:
- Default backend selection (tmux)
- Explicit backend selection via env var
- Backend operations (both tmux and temporal)
- Backward compatibility
- Registry integrity

### Manual Testing

```bash
# Test default (tmux)
unset AGM_SESSION_BACKEND
agm session list

# Test explicit tmux
export AGM_SESSION_BACKEND=tmux
agm session list

# Test temporal backend
export AGM_SESSION_BACKEND=temporal
agm session list

# Test invalid backend (should error)
export AGM_SESSION_BACKEND=invalid
agm session list
```

## Key Design Decisions

1. **Registry Pattern**:
   - Allows runtime backend selection
   - Supports adding new backends without modifying core code
   - Thread-safe implementation with RWMutex

2. **Environment Variable**:
   - Zero-config approach
   - Follows 12-factor app principles
   - Easy to override in different environments

3. **Adapter Layer**:
   - Maintains backward compatibility
   - No changes required to existing command code
   - Clean separation of concerns

4. **Fail-Safe Default**:
   - Defaults to tmux (existing behavior)
   - Graceful fallback on errors
   - No breaking changes to existing deployments

5. **Interface-First Design**:
   - All backends implement same interface
   - Consistent behavior across backends
   - Easy to add new backends

## Deliverables Checklist

- [x] `internal/backend/backend.go` - Backend interface definition
- [x] `internal/backend/registry.go` - Backend registration system
- [x] `internal/backend/tmux_backend.go` - Tmux backend implementation
- [x] `internal/backend/temporal_backend.go` - Temporal backend implementation
- [x] `internal/backend/adapter.go` - Backward compatibility adapter
- [x] `internal/backend/backend_test.go` - Unit tests (80%+ coverage)
- [x] `test/integration/backend_switching_test.go` - Integration tests
- [x] `cmd/agm/main.go` - Updated to use backend system
- [x] `internal/backend/README.md` - Comprehensive documentation
- [x] Backward compatibility preserved
- [x] All tests passing
- [x] No breaking changes to public APIs

## Next Steps

1. **Task 1.5**: End-to-End Integration Test
   - Test full workflow with Temporal backend
   - Verify session creation, attachment, and termination
   - Test state persistence and recovery

2. **Future Enhancements**:
   - Configuration file support for backend selection
   - Backend-specific configuration options
   - Health checks and automatic failover
   - Session migration between backends
   - Backend capability discovery

## Notes

- The Temporal backend currently uses stub implementation
- Full Temporal integration will be completed in subsequent tasks
- All changes are backward compatible
- No existing functionality is broken
- Environment variable approach allows easy rollback (just unset the var)
