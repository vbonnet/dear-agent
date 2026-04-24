# ADR-001: Provider Registry Pattern

**Status**: Accepted
**Date**: 2026-03-20
**Authors**: Claude Sonnet 4.5
**Context**: Phase 1 - Core Implementation

## Context

The sandbox subsystem needs to support multiple isolation technologies (OverlayFS on Linux, APFS on macOS) while maintaining a clean, extensible architecture. Initial implementation attempted direct imports from `factory.go` to provider packages, resulting in import cycles:

```
sandbox (factory.go) → overlayfs → sandbox (provider interface)
        ↑_______________________________________________↓
                    Import Cycle!
```

## Decision

Implement a **provider registry pattern** where providers self-register via `init()` functions instead of being directly imported by the factory.

### Implementation

**Registry (`providers.go`)**:
```go
var (
    providerRegistry = make(map[string]func() Provider)
    registryMu       sync.RWMutex
)

func RegisterProvider(name string, factory func() Provider) {
    registryMu.Lock()
    defer registryMu.Unlock()
    providerRegistry[name] = factory
}

func getProvider(name string) (Provider, error) {
    registryMu.RLock()
    defer registryMu.RUnlock()

    factory, ok := providerRegistry[name]
    if !ok {
        return nil, ErrProviderNotRegistered
    }
    return factory(), nil
}
```

**Provider Registration (`overlayfs/provider.go`)**:
```go
func init() {
    sandbox.RegisterProvider("overlayfs", func() sandbox.Provider {
        return NewProvider()
    })
}
```

**Factory Usage (`factory.go`)**:
```go
func NewProviderForPlatform(name string) (Provider, error) {
    return getProvider(name) // No direct imports of provider packages
}
```

## Consequences

### Positive

1. **Eliminates Import Cycles**: Factory no longer imports provider packages directly
2. **Extensibility**: New providers can be added without modifying factory code
3. **Plugin-Like Architecture**: Providers are self-contained modules
4. **Thread-Safe**: RWMutex ensures safe concurrent access
5. **Testing Flexibility**: Can inject mock providers for testing

### Negative

1. **Runtime Discovery**: Provider availability unknown until runtime (vs compile-time)
2. **Init Order Dependency**: Relies on Go's `init()` execution order
3. **Debugging Complexity**: Registration happens implicitly, harder to trace
4. **No Compile-Time Checks**: Typos in provider names not caught until runtime

### Mitigations

- **Comprehensive Tests**: Contract tests verify all expected providers exist
- **Clear Naming**: Provider names are constants, reducing typo risk
- **Documentation**: ARCHITECTURE.md explains registration flow
- **Validation**: Factory validates provider exists before use

## Alternatives Considered

### 1. Interface-Only Approach (No Registry)

**Rejected**: Still requires factory to import provider packages (import cycle)

### 2. Builder Pattern

```go
type ProviderBuilder struct {
    overlayfsFactory func() Provider
    apfsFactory      func() Provider
}
```

**Rejected**: Requires passing builder to factory (complex initialization)

### 3. Dependency Injection via Constructor

```go
func NewFactory(providers map[string]Provider) *Factory
```

**Rejected**: Breaks auto-detection, requires manual wiring

## Related Decisions

- ADR-002: Platform Detection Strategy (uses registry for provider lookup)
- ADR-004: Mock Provider Design (registers via same mechanism)

## Examples

### Adding a New Provider

```go
// internal/sandbox/fuse/provider.go
package fuse

import "github.com/vbonnet/dear-agent/internal/sandbox"

func init() {
    sandbox.RegisterProvider("fuse-overlayfs", func() sandbox.Provider {
        return NewFuseProvider()
    })
}
```

No changes to factory required!

### Using in Tests

```go
func TestProviderRegistry(t *testing.T) {
    // Mock provider auto-registers via init()
    provider, err := sandbox.NewProviderForPlatform("mock")
    if err != nil {
        t.Fatal(err)
    }

    // Can now use provider without importing mock package
}
```

## References

- **Similar Pattern**: database/sql driver registration
- **Go Blog**: https://go.dev/blog/laws-of-reflection (init functions)
- **Implementation**: `internal/sandbox/providers.go`
