# Astrocyte - Session Monitor (Go Implementation)

Astrocyte is a daemon that monitors Claude agent sessions for stuck states and provides automated recovery through pattern-based violation detection. It serves as Tier 3 (last resort) in the unified violation enforcement system.

## Architecture

Astrocyte is built with a **shared violation enforcement library** that can be used across multiple tools:

- **pkg/enforcement/** - Publicly exported library for violation detection
- **internal/** - Astrocyte-specific implementation (daemon, tmux, config)
- **cmd/** - Binary entry points

## Enforcement Library

The `pkg/enforcement` package provides a reusable library for violation detection and enforcement. It's designed to be used by:

1. **Astrocyte daemon** (Tier 3 - session recovery)
2. **PreTool hooks** (Tier 2 - command validation)
3. **Analysis tools** - Pattern effectiveness, violation metrics

### Features

- **Pattern loading** from YAML files
- **Violation detection** with regex matching
- **Context-aware detection** (e.g., git worktree checks)
- **Pre-compiled patterns** for performance (10x faster than Python)
- **90%+ test coverage** for reliability

### Quick Start

```go
package main

import (
    "fmt"
    "github.com/vbonnet/ai-tools/astrocyte/pkg/enforcement"
)

func main() {
    // Load patterns from YAML
    patterns, err := enforcement.LoadPatternsByType("bash")
    if err != nil {
        panic(err)
    }

    // Create detector
    detector, err := enforcement.NewDetector(patterns)
    if err != nil {
        panic(err)
    }

    // Detect violations
    command := "cd /repo && git push"
    pattern, err := detector.Detect(command)
    if pattern != nil {
        fmt.Printf("Violation: %s\n", pattern.Reason)
        fmt.Printf("Alternative: %s\n", pattern.Alternative)
    }
}
```

## Pattern Databases

Astrocyte uses centralized pattern databases located at:

- `~/src/ws/oss/repos/engram/patterns/bash-anti-patterns.yaml`
- `~/src/ws/oss/repos/engram/patterns/beads-anti-patterns.yaml`
- `~/src/ws/oss/repos/engram/patterns/git-anti-patterns.yaml`

Each pattern includes:

- **id** - Unique identifier
- **regex** - Pattern matching expression
- **reason** - Why this is a violation
- **alternative** - Recommended alternative
- **severity** - low, medium, high, critical
- **tier flags** - Which enforcement tiers use this pattern

## API Reference

### Pattern Loading

```go
// Load patterns from specific file
patterns, err := enforcement.LoadPatterns("/path/to/patterns.yaml")

// Load patterns by type
patterns, err := enforcement.LoadPatternsByType("bash")
```

### Violation Detection

```go
// Create detector
detector, err := enforcement.NewDetector(patterns)

// Detect violation
pattern, err := detector.Detect("cd /repo && git push")

// Detect all violations
patterns, err := detector.DetectAll(command)
```

### Context-Aware Detection

```go
// Create context
ctx := enforcement.Context{
    HasWorktrees: true,
    IsMainWorktree: true,
    WorkingDir: "/home/user/repo",
}

// Detect with context
pattern, err := detector.DetectWithContext(command, ctx)
```

### Pattern Utilities

```go
// Get pattern by ID
pattern := db.GetPattern("cd-chaining")

// Filter by severity
highPatterns := db.FilterBySeverity("high")

// Filter by tier
tier3Patterns := db.FilterByTier("tier3")
```

## Testing

Run all tests:

```bash
go test ./pkg/enforcement/...
```

Run with coverage:

```bash
go test -cover ./pkg/enforcement/...
```

Run with verbose output:

```bash
go test -v ./pkg/enforcement/...
```

### Test Coverage Requirements

- **Overall target**: 90%+
- **pkg/enforcement/patterns.go**: 95% (critical - pattern loading)
- **pkg/enforcement/detector.go**: 95% (critical - violation detection)
- **pkg/enforcement/matcher.go**: 90% (regex edge cases)

## Development

### Building

```bash
go build ./pkg/enforcement/...
```

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage report
go test -cover ./...

# Run with race detection
go test -race ./...

# Run benchmarks
go test -bench=. ./pkg/enforcement/...
```

### Code Organization

```
astrocyte-go/
├── pkg/
│   └── enforcement/          # Shared enforcement library
│       ├── patterns.go       # Pattern loading from YAML
│       ├── patterns_test.go
│       ├── detector.go       # Violation detection
│       ├── detector_test.go
│       ├── matcher.go        # Regex matching utilities
│       └── matcher_test.go
├── internal/                 # Astrocyte-specific (not exported)
│   ├── daemon/              # Session monitoring & stuck detection
│   │   ├── detector.go      # StuckSessionDetector
│   │   ├── detector_test.go
│   │   └── README.md
│   ├── tmux/                # Tmux client & pane inspection
│   │   ├── client.go        # TmuxClient
│   │   ├── pane.go          # PaneInfo & pattern detection
│   │   ├── client_test.go
│   │   ├── pane_test.go
│   │   ├── integration_test.go
│   │   └── README.md
│   └── config/              # Configuration (future)
├── cmd/
│   └── astrocyte/           # Main binary (future)
├── examples/
│   ├── simple-detector.go   # Basic enforcement usage
│   └── session-monitor.go   # Session monitoring demo
├── test/
│   ├── integration/         # Integration tests (future)
│   └── fixtures/            # Test data (future)
├── go.mod
└── README.md
```

## Performance

The Go implementation provides significant performance improvements:

- **Pattern detection**: <1ms per command (vs ~10ms Python)
- **Session monitoring**: <100ms per session (vs ~1s Python)
- **Memory usage**: <50MB (vs ~150MB Python)

These improvements are achieved through:

- Pre-compiled regex patterns (cached, not recompiled)
- Efficient YAML parsing
- Minimal allocations in hot paths

## Integration

### Pattern Database Schema

```yaml
version: "1.0"
updated: "2026-02-15"
purpose: "Centralized pattern database"
used_by:
  - "Tier 1: AI instructions"
  - "Tier 2: PreTool hooks"
  - "Tier 3: Astrocyte daemon"

patterns:
  - id: cd-chaining
    regex: 'cd\s+[^\s]+\s+&&'
    reason: "Command chaining with cd"
    alternative: "Use tool-specific -C flag"
    examples:
      - "cd /repo && git push"
    severity: high
    tier2_validation: true
    tier3_rejection: true
```

### Context Checks

Some patterns require environmental context:

- `has_worktrees` - Repository has git worktrees
- `is_main_worktree` - Current directory is main worktree
- `not_main_worktree` - Current directory is not main worktree

## License

MIT

## References

- **SPEC**: `~/src/ws/oss/swarm/unified-violation-enforcement-20260214/SPEC-ASTROCYTE.md`
- **Pattern databases**: `~/src/ws/oss/repos/engram/patterns/`
- **Temporal platform**: `~/src/ws/oss/wf/temporal-orchestration-platform/`

## Internal Packages

### internal/tmux

Tmux client for session monitoring and pane inspection.

**Features:**
- Multi-socket support (AGM + system default)
- Session listing and pane content capture
- Cursor position tracking
- Key sending (for recovery)
- Pattern-based stuck detection

**Quick Start:**

```go
import "github.com/vbonnet/ai-tools/astrocyte/internal/tmux"

// Create client
client := tmux.NewClient()

// List sessions
sessions, _ := client.ListSessions()

// Capture pane info
pane, _ := tmux.CapturePaneInfo(client, "session-name")

// Check if stuck
if pane.IsStuck() {
    reason := pane.GetStuckReason()
    // Handle stuck session
}
```

See [internal/tmux/README.md](internal/tmux/README.md) for detailed documentation.

### internal/daemon

Stuck session detection with multi-indicator analysis.

**Features:**
- Session history tracking (cursor movement)
- Configurable timeout thresholds
- False positive prevention
- Integration with tmux.PaneInfo

**Quick Start:**

```go
import "github.com/vbonnet/ai-tools/astrocyte/internal/daemon"

// Create detector
detector := daemon.NewStuckSessionDetector()

// Track cursor movement
detector.TrackSession("session-name", cursorX, cursorY)

// Detect stuck sessions
info := detector.DetectStuckSession(pane)
if info != nil {
    fmt.Printf("Session stuck: %s\n", info.Reason)
}
```

See [internal/daemon/README.md](internal/daemon/README.md) for detailed documentation.

## Examples

See the [examples/](examples/) directory for complete working examples:

- **simple-detector.go** - Basic enforcement pattern detection
- **session-monitor.go** - Full session monitoring workflow

Run examples:

```bash
go run examples/session-monitor.go
```

## Status

**Phase 8.1**: ✅ Foundation complete
- Go module structure
- Pattern loading library
- Violation detection engine
- Regex matching utilities
- 90%+ test coverage

**Phase 8.2**: ✅ Pattern detection complete
- Pattern matching engine
- Context-aware detection
- Performance optimization

**Phase 8.3**: ✅ Rejection messages complete
- Message generation
- Violation logging

**Phase 8.4**: ✅ Tmux client complete
- Tmux client implementation
- Pane inspection and pattern detection
- Session monitoring logic
- Stuck session detection
- 90%+ test coverage
- Comprehensive integration tests

**Next**: Phase 8.5 - Daemon and recovery logic
