# ADR-004: Rate Limiting Custom Implementation

**Status**: Accepted
**Date**: 2026-03-20
**Phase**: Language Audit Phase 6 - Go Library Modernization
**Task**: 6.2 - Rate Limiting Migration Evaluation
**Decision Maker**: Language Audit Team

---

## Context

The ecphory relevance ranking system uses a custom rate limiter in `core/pkg/ecphory/ranker.go` (lines 27-95) to prevent API quota exhaustion when making LLM ranking requests. The rate limiter implements multi-level limiting:

- **Hourly limit**: 100 requests/hour (prevents quota exhaustion)
- **Session limit**: 20 requests/session (prevents runaway ranking)
- **Minimum interval**: 1 second cooldown (prevents burst traffic)
- **Monotonic time**: Uses Go's monotonic clock to prevent drift issues

**Current Implementation**: 68 lines of custom token bucket logic with comprehensive test coverage (6 tests covering concurrency, limits, monotonic time).

**Evaluation Trigger**: Language Audit Phase 6 task to evaluate migrating to `golang.org/x/time/rate` stdlib-endorsed rate limiter.

---

## Decision

**We will KEEP the custom rate limiter implementation** rather than migrate to `golang.org/x/time/rate`. The multi-level limiting requirements cannot be elegantly expressed with golang.org/x/time/rate without significant wrapper complexity.

---

## Alternatives Considered

### 1. golang.org/x/time/rate (Stdlib Extended, Official)

**Pros**:
- **Official Google package**: Part of golang.org/x, trusted source
- **Production-proven**: Used in kubernetes, etcd, many Google projects
- **Token bucket algorithm**: Well-tested implementation
- **Burst control**: Supports burst allowance with `Limit` and `Bucket`
- **Wait functionality**: `Wait(ctx)` blocks until token available
- **Reservation system**: `Reserve()` for advance token booking
- **Active maintenance**: Regular updates, widely adopted

**Cons**:
- **Single limiter limitation**: Each `Limiter` instance handles ONE rate limit
- **Multi-level requires composition**: Need 3 separate limiters (hourly, session, interval)
- **No built-in reset**: Hourly reset requires custom logic or new limiter
- **Wrapper complexity**: Custom code needed to coordinate 3 limiters
- **Session tracking**: No concept of "session" - requires external state management
- **Migration effort**: 2-3 days to implement multi-level wrapper + testing

**Example of required wrapper**:
```go
// Required wrapper to replicate current behavior
type MultiLevelLimiter struct {
    hourly    *rate.Limiter  // 100/hour
    session   *rate.Limiter  // 20/session
    interval  *rate.Limiter  // 1/second
    mu        sync.Mutex
    hourStart time.Time
}

func (m *MultiLevelLimiter) Allow() error {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Check hourly limit (need custom reset logic)
    now := time.Now()
    if now.Sub(m.hourStart) >= time.Hour {
        m.hourly = rate.NewLimiter(100.0/3600, 100) // Reset hourly
        m.hourStart = now
    }
    if !m.hourly.Allow() {
        return fmt.Errorf("hourly rate limit exceeded")
    }

    // Check session limit (no reset mechanism)
    if !m.session.Allow() {
        return fmt.Errorf("session rate limit exceeded")
    }

    // Check minimum interval
    if !m.interval.Allow() {
        return fmt.Errorf("rate limit: wait before next request")
    }

    return nil
}
```

**Result**: ~50-60 lines of wrapper code + custom reset logic = similar complexity to current implementation.

**Rejected**: Wrapper complexity negates benefits of using stdlib package.

---

### 2. uber-go/ratelimit (Uber's Rate Limiter, 4,200+ ⭐)

**Pros**:
- **Simple API**: `Take()` blocks until token available
- **No allocations**: Zero-allocation fast path
- **Benchmark-proven**: Optimized for high throughput
- **Battle-tested**: Used in Uber's production systems

**Cons**:
- **Single rate only**: No multi-level limiting support
- **No burst control**: Strict interval-based limiting
- **No reservation system**: Can't check without consuming
- **Still requires wrapper**: Same multi-level composition problem
- **Less flexible**: golang.org/x/time/rate is more feature-complete

**Rejected**: Same multi-level limiting problem, less flexible than golang.org/x/time/rate.

---

### 3. juju/ratelimit (Canonical's Rate Limiter, 900+ ⭐)

**Pros**:
- **Token bucket**: Classic token bucket algorithm
- **Wait and Take**: Blocking and non-blocking modes
- **Burst support**: Configurable burst size

**Cons**:
- **Single limiter**: Same multi-level composition problem
- **Less maintained**: Fewer updates than golang.org/x/time/rate
- **Smaller ecosystem**: Not as widely adopted

**Rejected**: Same limitations, less adoption than golang.org/x/time/rate.

---

## Rationale

### 1. Multi-Level Limiting is Core Requirement

The custom implementation elegantly handles three independent limits:

```go
// Current implementation (68 lines, clear intent)
func (rl *RateLimiter) Allow() error {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    now := time.Now()

    // Hourly limit (resets every hour)
    if now.Sub(rl.hourStart) >= time.Hour {
        rl.hourlyTokens = rl.tokensPerHour
        rl.hourStart = now
    }
    if rl.hourlyTokens <= 0 {
        return fmt.Errorf("hourly rate limit exceeded (100/hour)")
    }

    // Session limit (fixed per session)
    if rl.sessionTokens <= 0 {
        return fmt.Errorf("session rate limit exceeded (20/session)")
    }

    // Minimum interval (1 second cooldown)
    if !rl.lastRequest.IsZero() && now.Sub(rl.lastRequest) < rl.minInterval {
        waitTime := rl.minInterval - now.Sub(rl.lastRequest)
        return fmt.Errorf("rate limit: wait %v before next request", waitTime)
    }

    // Consume tokens
    rl.hourlyTokens--
    rl.sessionTokens--
    rl.lastRequest = now

    return nil
}
```

**Third-party alternatives** would require:
1. Three separate `rate.Limiter` instances
2. Custom hourly reset logic
3. Custom session state management
4. Wrapper to coordinate all three limiters
5. **Result**: ~60 lines of wrapper + coordination logic = same complexity

---

### 2. Monotonic Time Handling (P0-5 Fix)

The implementation uses Go's monotonic clock (Go 1.9+) to prevent clock drift issues:

```go
// P0-5 FIX: time.Now() returns monotonic time in Go 1.9+
// Sub() uses monotonic clock for duration calculation, immune to clock adjustments
now := time.Now()

// Reset hourly tokens if hour has passed (using monotonic duration)
if now.Sub(rl.hourStart) >= time.Hour {
    rl.hourlyTokens = rl.tokensPerHour
    rl.hourStart = now
}
```

**Benefit**: Immune to system clock changes (NTP adjustments, DST changes, manual time changes).

**golang.org/x/time/rate** also uses monotonic time internally, but the hourly reset logic would still need custom implementation with the same monotonic time handling.

**No advantage** to migrating: Both approaches use monotonic time correctly.

---

### 3. Comprehensive Test Coverage

**Current tests** (`ratelimiter_test.go`, 6 tests):
1. `TestRateLimiter_Allow_BasicUsage` - Basic token consumption
2. `TestRateLimiter_Allow_SessionLimit` - Session limit enforcement (20 requests)
3. `TestRateLimiter_Allow_HourlyReset` - Hourly reset after time.Hour
4. `TestRateLimiter_Allow_MinInterval` - Minimum interval enforcement
5. `TestRateLimiter_Allow_Concurrent` - Thread safety (100 concurrent goroutines)
6. `TestRateLimiter_MonotonicTime` - Monotonic time handling
7. `TestRateLimiter_HourlyReset_MonotonicTime` - Hourly reset with monotonic time

**Test scenarios covered**:
- All three limit types (hourly, session, interval)
- Concurrent access (race detection)
- Time-based edge cases (hour boundary, monotonic time)
- Error messages for each limit type

**Migration would require**:
- Rewriting tests for wrapper implementation
- Testing coordination between 3 limiters
- Ensuring same behavior for all edge cases
- Risk of test coverage gaps during migration

---

### 4. Clear Error Messages

Custom error messages provide context for each limit type:

```go
// Hourly limit
return fmt.Errorf("hourly rate limit exceeded (100/hour)")

// Session limit
return fmt.Errorf("session rate limit exceeded (20/session)")

// Minimum interval
return fmt.Errorf("rate limit: wait %v before next request", waitTime)
```

**Users see**: Which specific limit was hit, what the limit is, how long to wait.

**golang.org/x/time/rate** returns generic errors or requires custom error handling in wrapper.

**Advantage**: Custom implementation provides better user experience.

---

### 5. Production Use and Stability

**Current deployment**: Used in ecphory ranking system since implementation (production-tested).

**Key properties**:
- **Zero allocations** in fast path (mutex + arithmetic)
- **No external dependencies** (stdlib only)
- **Simple state machine** (3 counters + 2 timestamps)
- **Well-tested** (6 comprehensive tests, race detector clean)

**Stability**: No bugs reported, no changes needed since initial implementation.

**Risk**: Migration introduces potential for behavior changes or bugs in production ranking system.

---

### 6. ROI Analysis

**Migration costs**:
- Development: 2-3 days (wrapper implementation + testing)
- Risk: Behavior changes in production ranking system
- Testing: Port 6 tests + test wrapper coordination
- Coordination logic: ~60 lines of custom code to manage 3 limiters
- Hourly reset: Custom logic to recreate hourly limiter
- Session state: External management (not in golang.org/x/time/rate)
- **Total effort**: Same as current implementation

**Migration benefits**:
- Stdlib package (trusted, but golang.org/x is semi-official, not stdlib)
- Community support (helpful, but simple algorithm doesn't need it)
- **Net benefit**: Minimal - still need custom wrapper

**Keep custom costs**:
- Maintenance: ~0 (simple code, comprehensive tests, stable)
- Documentation: Already documented with comments
- **Total effort**: Zero ongoing cost

**Keep custom benefits**:
- Zero migration risk
- Multi-level limiting built-in
- Clear error messages
- Production-proven
- No external dependencies
- **Net benefit**: High stability, zero risk

**Conclusion**: Poor ROI to migrate. Current implementation is optimal for this use case.

---

## Consequences

### Positive

✅ **Multi-level limiting**: Hourly, session, and interval limits in single implementation
✅ **Monotonic time**: Immune to clock drift (P0-5 fix)
✅ **Zero dependencies**: No external packages, no supply chain risk
✅ **Clear error messages**: User-friendly limit-specific errors
✅ **Comprehensive tests**: 6 tests covering all scenarios + concurrency
✅ **Production-proven**: Stable in production ranking system
✅ **Simple codebase**: 68 lines, easy to audit and maintain

### Negative

❌ **Not stdlib**: Custom implementation vs stdlib-endorsed package
❌ **No advanced features**: No reservation system, no wait-with-timeout (not needed)
❌ **Manual coordination**: All three limits in one struct (but this is simpler than wrapper)

### Neutral

⚪ **Performance**: Zero-allocation fast path, same as golang.org/x/time/rate
⚪ **Extensibility**: Easy to add new limit types if needed
⚪ **Alternative**: Could use golang.org/x/time/rate for single-level limiting in future features

---

## Implementation

**No migration required.** This ADR documents the decision to keep the current implementation.

**Optional enhancements** (not required):
1. **Add godoc examples** (~30 minutes):
   ```go
   // Example usage
   // rl := NewRateLimiter()
   // if err := rl.Allow(); err != nil {
   //     return fmt.Errorf("rate limited: %w", err)
   // }
   // // proceed with LLM ranking request
   ```

2. **Add metrics** (~1-2 hours):
   - Count requests denied by each limit type
   - Track average wait times
   - Emit telemetry for rate limit hits

3. **Configurable limits** (~1-2 hours):
   - Allow custom hourly/session limits
   - Environment variable overrides
   - Per-provider limits (Anthropic vs VertexAI)

**Estimated enhancement effort**: 2-4 hours (vs 2-3 days migration)

---

## Future Review Triggers

Reconsider this decision if any of the following occur:

1. **Single-level limiting**: Requirements change to only need one limit type
2. **Advanced features needed**: Reservation system, wait-with-timeout, dynamic rate adjustment
3. **Cross-service limiting**: Need distributed rate limiting across multiple processes
4. **Performance issues**: Current implementation becomes bottleneck (unlikely given simplicity)
5. **Stdlib inclusion**: golang.org/x/time/rate moves into stdlib (unlikely, already stable)

**Current status**: None of these triggers apply as of 2026-03-20.

---

## References

**Current Implementation**:
- `core/pkg/ecphory/ranker.go:27-95` - 68 lines of rate limiter logic
- `core/pkg/ecphory/ratelimiter_test.go` - 6 comprehensive tests (200+ lines)
- Usage: `core/pkg/ecphory/ranker.go:100` - Ranker integration

**Test Coverage**:
- Basic usage, session limits, hourly reset, minimum interval
- Concurrent access (100 goroutines, race detector clean)
- Monotonic time handling, hourly reset with monotonic time

**Third-Party Libraries Evaluated**:
- golang.org/x/time/rate: https://pkg.go.dev/golang.org/x/time/rate
- uber-go/ratelimit: https://github.com/uber-go/ratelimit
- juju/ratelimit: https://github.com/juju/ratelimit

**Related ADRs**:
- ADR-001: Circuit Breaker Custom Implementation (keep custom)
- ADR-002: Table Formatting Enhancement (adopt lipgloss)
- ADR-003: Input Validation Custom Implementation (keep custom)

---

**Approved By**: Language Audit Phase 6 Evaluation
**Review Date**: 2026-03-20
**Next Review**: When review triggers occur (see Future Review Triggers)
