# Backend Package

The `backend` package provides a pluggable backend system for AGM session management, allowing the system to switch between different session implementations (tmux, Temporal, etc.) via environment variable configuration.

## Architecture

The package implements a registry pattern with the following components:

### Core Interfaces

- **`Backend`**: Unified interface for all session backends
- **`SessionInfo`**: Backend-agnostic session information
- **`ClientInfo`**: Backend-agnostic client information

### Components

1. **Registry** (`registry.go`): Backend factory registration and selection
2. **Backends**:
   - **TmuxBackend** (`tmux_backend.go`): Wraps existing tmux implementation
   - **TemporalBackend** (`temporal_backend.go`): Wraps Temporal workflow implementation
3. **Adapter** (`adapter.go`): Converts `Backend` to `session.TmuxInterface` for backward compatibility

## Usage

### Environment Variable

Set the `AGM_SESSION_BACKEND` environment variable to select a backend:

```bash
# Use tmux backend (default)
export AGM_SESSION_BACKEND=tmux

# Use Temporal backend
export AGM_SESSION_BACKEND=temporal

# Unset to use default (tmux)
unset AGM_SESSION_BACKEND
```

### Programmatic Access

```go
import "github.com/vbonnet/ai-tools/agm/internal/backend"

// Get backend based on AGM_SESSION_BACKEND env var
b, err := backend.GetBackend()
if err != nil {
    // Handle error
}

// Use backend
exists, err := b.HasSession("my-session")
sessions, err := b.ListSessions()
err = b.CreateSession("my-session", "/path/to/workdir")
```

### Using Backend Adapter

For compatibility with existing code that expects `session.TmuxInterface`:

```go
import "github.com/vbonnet/ai-tools/agm/internal/backend"

// Get adapter using default backend
adapter, err := backend.GetDefaultBackendAdapter()
if err != nil {
    // Handle error
}

// Use as session.TmuxInterface
exists, err := adapter.HasSession("my-session")
```

### Explicit Backend Selection

```go
import "github.com/vbonnet/ai-tools/agm/internal/backend"

// Get specific backend by name
tmuxBackend, err := backend.GetBackendByName("tmux")
temporalBackend, err := backend.GetBackendByName("temporal")

// List all registered backends
backends := backend.ListBackends()

// Check if backend is registered
if backend.IsRegistered("temporal") {
    // ...
}
```

## Registering New Backends

To add a new backend:

1. Implement the `Backend` interface
2. Register it during package initialization

```go
package mybackend

import "github.com/vbonnet/ai-tools/agm/internal/backend"

type MyBackend struct {
    // ...
}

func (b *MyBackend) HasSession(name string) (bool, error) {
    // Implementation
}

// Implement other Backend methods...

func init() {
    backend.Register("mybackend", func() (backend.Backend, error) {
        return &MyBackend{}, nil
    })
}
```

## Backend Interface

All backends must implement:

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

## Backward Compatibility

- When `AGM_SESSION_BACKEND` is not set, the system defaults to `tmux`
- Existing code continues to work without modification
- The `BackendAdapter` ensures new backends work with existing `session.TmuxInterface` consumers

## Testing

### Unit Tests

```bash
go test ./internal/backend/...
```

### Integration Tests

```bash
go test ./test/integration/backend_switching_test.go
```

### Test Coverage

The package includes comprehensive tests:
- Backend registration and retrieval
- Environment variable parsing
- Backend switching
- Interface compliance
- Concurrent access safety
- Error handling

Target coverage: 80%+

## Implementation Details

### TmuxBackend

- Wraps `session.TmuxInterface`
- Uses `session.NewRealTmux()` for actual tmux operations
- Registered as `"tmux"`
- Default backend when no env var is set

### TemporalBackend

- Wraps `temporal.TemporalInterface`
- Uses `temporal.NewTemporalClient()` for Temporal operations
- Registered as `"temporal"`
- Stub implementation for initial release

### BackendAdapter

- Implements `session.TmuxInterface`
- Wraps any `Backend` implementation
- Provides seamless integration with existing code
- Used in `cmd/agm/main.go` to inject backend into command execution

## Design Decisions

1. **Registry Pattern**: Allows runtime backend selection without recompilation
2. **Environment Variable**: Simple, zero-config way to switch backends
3. **Adapter Layer**: Preserves backward compatibility with existing codebase
4. **Interface-First**: All backends implement the same interface for consistency
5. **Fail-Safe Default**: Falls back to tmux if backend initialization fails

## Future Enhancements

- Configuration file support for backend selection
- Backend-specific configuration options
- Health checks and fallback mechanisms
- Backend capability discovery
- Multi-backend session migration tools
