# ADR-014: Structured Logging with stdlib log/slog

**Status**: Accepted
**Date**: 2026-03-20
**Deciders**: Engineering Team
**Tags**: logging, observability, dependencies

---

## Context

Prior to this decision, agm used the basic `log` package from the Go standard library for all logging operations. This approach had several limitations:

1. **Unstructured output**: Log messages were freeform strings with printf-style formatting
2. **No log levels**: No distinction between debug, info, warn, error severity
3. **Poor queryability**: Difficult to filter, aggregate, or analyze logs programmatically
4. **Manual instrumentation**: Developers had to manually format context (session IDs, error codes, etc.)
5. **No JSON support**: CLI tools mixed structured data with log output, making parsing difficult

With the introduction of `log/slog` in Go 1.21 (released August 2023), the Go standard library now provides first-class support for structured logging with zero external dependencies.

## Decision

We will **migrate all logging to use stdlib `log/slog`** across agm and all ai-tools modules.

### Migration Strategy

1. **Create logging package** (`internal/logging/slog.go`):
   - `DefaultLogger()`: Text format for CLI output
   - `JSONLogger()`: JSON format for daemon/programmatic consumption
   - `DebugLogger()`: Text format with DEBUG level
   - `NewTextLogger(io.Writer)`: Custom writer support for testing

2. **Structured field naming conventions**:
   - `error`: Error values
   - `session_id`, `session`: Session identifiers
   - `message_id`: Message identifiers
   - `state`: Session states (DONE, WORKING, etc.)
   - `count`: Counters (delivered, failed, etc.)
   - `interval`: Time intervals
   - `attempt`, `max_retries`: Retry logic fields
   - `pid`, `signal`: Process management fields

3. **Log level mapping**:
   - `logger.Info()`: Normal operations, state transitions
   - `logger.Warn()`: Warnings, recoverable errors, retries
   - `logger.Error()`: Fatal errors, failures requiring intervention
   - `logger.Debug()`: Verbose debugging (not enabled by default)

4. **Pattern for Fatal replacements**:
   ```go
   // Before
   log.Fatalf("error: %v", err)

   // After
   logger.Error("operation failed", "error", err)
   os.Exit(1)  // Explicit exit
   ```

### Affected Components

- **daemon**: ~50 log calls migrated (major refactor)
  - Breaking change: `Config.Logger` type changed from `*log.Logger` to `*slog.Logger`
- **eventbus**: WebSocket connection management, client lifecycle
- **tui**: EventBus client, reconnection logic
- **tmux**: Pane monitoring, state detection
- **persistence**: Dual-write cache warnings
- **monitoring**: OpenCode publisher, circuit breaker, Astrocyte watcher
- **config/evaluation**: Environment validation, alert system
- **examples/tests**: Test helpers, example code

**Total**: ~150 log calls migrated in agm

### Documentation

- Updated all package documentation
- Created ADR-014 (this document)
- Updated test infrastructure to use slog
- Documented structured field conventions

## Consequences

### Positive

1. **Zero dependencies**: Uses Go 1.21+ stdlib, no external libraries required
2. **Structured output**: JSON-ready for log aggregation (Datadog, ELK, Loki)
3. **Better filtering**: Query logs by structured fields (session ID, error type, etc.)
4. **Consistent levels**: Clear severity distinction (Info/Warn/Error)
5. **Performance**: More efficient than `fmt.Sprintf` string building
6. **Context support**: Ready for distributed tracing, correlation IDs
7. **Type safety**: Structured fields are type-checked at compile time
8. **Backward compatible**: Text format remains CLI-friendly
9. **Testability**: Easy to capture and assert on structured log output
10. **Industry standard**: Follows observability best practices

### Negative

1. **Breaking change**: `daemon.Config.Logger` type changed
   - **Mitigation**: All internal consumers updated in same migration
2. **Go version requirement**: Requires Go 1.21+ for log/slog
   - **Mitigation**: Already required by other dependencies
3. **Learning curve**: Developers must learn structured logging patterns
   - **Mitigation**: Clear examples in `internal/logging/slog.go`
4. **Verbosity**: Structured calls are longer than printf-style
   - **Mitigation**: Improved clarity outweighs brevity

### Neutral

1. **Documentation**: Some doc.go files still have old log examples
   - **Action**: Update in future documentation pass (non-blocking)
2. **External dependencies**: No external logger dependencies removed
   - **Reason**: None existed before (was using stdlib log)

## Alternatives Considered

### 1. Keep basic log package

**Rejected**. No structured logging support, no log levels, poor observability.

### 2. Use zerolog

**Rejected**. Adds external dependency. slog is now part of stdlib and sufficient for our needs.

### 3. Use logrus

**Rejected**. Adds external dependency. Logrus is in maintenance mode, authors recommend slog.

### 4. Use zap

**Rejected**. Adds external dependency. Optimized for extreme performance scenarios we don't have.

### 5. Gradual migration with both log and slog

**Rejected**. Mixed logging approaches create confusion and inconsistent output.

## Implementation

### Phase 1: Core Infrastructure (34 calls)
- eventbus: WebSocket hub, client lifecycle
- tui: EventBus client, reconnection
- tmux: Pane monitoring
- persistence: Dual-write cache

### Phase 2: Monitoring & Config (15 calls)
- claude: History parsing
- monitoring: OpenCode publisher, event parser
- config: Environment validation
- evaluation: Alert system

### Phase 3: Daemon Core (~50 calls)
- **Breaking change**: daemon.Config.Logger type
- All lifecycle, message delivery, state detection
- cmd/agm-daemon: Startup logging

### Phase 4: Test Infrastructure
- daemon tests: Updated testLogger()
- adapter tests: slog.New with test writer
- logging package: Added NewTextLogger()

### Phase 5: Examples & Tests (43 calls)
- examples/monitoring_example.go
- internal/agent/openai/example_test.go
- internal/astrocyte/integration_example.go
- internal/llm/example_test.go
- internal/orchestrator/state/example_test.go
- scripts/create-test-data.go

### Phase 6: CLI Commands (12 calls)
- cmd/agm/new.go: Warning logs
- cmd/agm-mcp-server: MCP stdio transport (stderr logger)

**Total Migration**: 7 commits, ~150 log calls migrated

### Verification

```bash
# All packages build
go build ./...

# All tests pass
go test ./...

# No remaining basic log calls
grep -r "log\.(Print|Fatal|Panic)" --include="*.go" .
```

## Related

- **Go slog proposal**: https://go.dev/blog/slog
- **Go 1.21 release**: https://go.dev/doc/go1.21
- **Structured logging best practices**: https://www.structuredlogging.com/
- **internal/logging/slog.go**: Logger factory implementation
- **ADR-006**: Message Queue Architecture (uses structured logging)
- **ADR-007**: Hook-Based State Detection (uses structured logging)

## Notes

This migration aligns with Phase 7 of the Language Audit & Migration project. The structured logging foundation enables future observability improvements including:

- Distributed tracing integration
- Log aggregation system integration
- Metrics extraction from structured logs
- Advanced alerting based on structured fields
- Performance profiling hooks

## Validation

- ✅ All modules build successfully
- ✅ All tests compile and pass
- ✅ Zero undefined log references
- ✅ golangci-lint passes
- ✅ Structured field conventions documented
- ✅ Logger factory patterns established
- ✅ Breaking changes identified and resolved

---

**Author**: Engineering Team
**Reviewed**: 2026-03-20
**Status**: Implemented in Phase 7
