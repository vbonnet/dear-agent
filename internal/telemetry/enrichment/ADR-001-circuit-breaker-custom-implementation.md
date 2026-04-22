# ADR-001: Circuit Breaker Custom Implementation

**Status**: Accepted
**Date**: 2026-03-21
**Phase**: Language Audit Phase 6 - Go Library Modernization
**Task**: 6.3 - Circuit Breaker Evaluation
**Bead**: src-7b8
**Decision Maker**: Language Audit Team

---

## Context

The telemetry enrichment pipeline requires fault isolation to prevent cascading failures when enrichers (external API calls, context providers) fail repeatedly. A circuit breaker pattern is essential to:

- Prevent wasting resources on known-bad operations
- Fail fast when services are degraded
- Automatically attempt recovery after timeout periods
- Protect the overall pipeline from individual enricher failures

**Current Implementation**: Custom 3-state circuit breaker in `circuit_breaker.go` (~80 lines of logic)

**Evaluation Trigger**: Phase 6 of language-audit swarm aims to replace custom implementations with industry-standard Go libraries where beneficial.

---

## Decision

**We will KEEP the custom circuit breaker implementation** rather than migrate to a third-party library (github.com/cerr/circuit, sony/gobreaker, or rubyist/circuitbreaker).

---

## Alternatives Considered

### 1. github.com/cerr/circuit (Hypothetical - Not Found)

**Research Finding**: No active library found at github.com/cerr/circuit. Possible confusion with:
- cep21/circuit (Netflix Hystrix-compatible)
- sony/gobreaker (most popular)

Evaluated both alternatives instead.

### 2. cep21/circuit (812 ⭐, Netflix Hystrix Compatible)

**Pros**:
- Rich metrics integration (Prometheus, expvar, Hystrix dashboard)
- SLO tracking capabilities ("X% of requests < Y ms")
- Thread-safe runtime reconfiguration
- Panic recovery built-in
- Active maintenance, 195 commits

**Cons**:
- **Heavy API**: Complex configuration (ExecutionConfig, CommandPropertiesConstructor, factories)
- **Overkill**: ~2000+ lines for features we don't need (SLO tracking, Hystrix compat)
- **Generic wrapper**: `Execute(func() (T, error))` requires adapter layer for domain-specific `Execute(ctx, enricher, event, ec)` signature
- **Migration effort**: 3-5 days for adapter layer + testing

**Rejected**: Too complex for the enrichment use case. We don't need Hystrix compatibility or SLO tracking.

---

### 3. sony/gobreaker (3,600 ⭐, Most Popular)

**Pros**:
- **Mature and widely used**: 3,600 stars, 215 forks, well-maintained
- **Go 1.18+ generics**: Type-safe `NewCircuitBreaker[T any]`
- Rolling/fixed window strategies for rate-based triggering
- Custom callbacks: `ReadyToTrip`, `IsSuccessful`, `OnStateChange`
- Simple API: `Execute(req func() (T, error))`

**Cons**:
- **Generic wrapper required**: Domain-specific signature doesn't match `func() (T, error)` pattern
- **Adapter complexity**: Must wrap enricher execution in closure, losing type safety
- **Bucket complexity**: Rolling windows add complexity we don't need (consecutive failures is sufficient)
- **Counts struct exposure**: Exposes internal metrics, may encourage coupling
- **Migration effort**: 3-5 days for generics adaptation + testing

**Rejected**: Generic wrapper introduces unnecessary abstraction. Our domain-specific API is clearer.

---

### 4. rubyist/circuitbreaker (1,200 ⭐)

**Pros**:
- Multiple strategies (Threshold, Consecutive, Rate)
- Event subscription system for observability
- Manual control methods (Ready, Success, Fail)
- HTTP client integration helpers

**Cons**:
- **Strategy complexity**: 3 different breaker types vs our simple consecutive model
- **Event subscriptions**: Unused feature adds complexity
- **Generic wrapper**: Same adapter layer issue as others
- **HTTP-focused**: Features designed for HTTP clients, not enrichment pipeline
- **Migration effort**: 4-6 days for strategy selection + adapter

**Rejected**: Strategy complexity and HTTP focus don't align with telemetry enrichment needs.

---

## Rationale

### 1. Perfect Domain Fit

The custom `Execute(ctx, enricher, event, ec)` signature is purpose-built for the enrichment pipeline:

```go
func (cb *CircuitBreaker) Execute(
    ctx context.Context,
    enricher Enricher,
    event *TelemetryEvent,
    ec EnrichmentContext,
) (*TelemetryEvent, error)
```

**Benefits**:
- Direct integration with `Enricher` interface
- Returns original event on failure (graceful degradation)
- No generic wrappers or adapter layers
- Type-safe at call sites

**Third-party alternatives** require wrapping:
```go
// Required adapter with third-party library
result, err := cb.Execute(func() (*TelemetryEvent, error) {
    return enricher.Enrich(ctx, event, ec)  // Loses direct signature
})
```

This indirection reduces readability and introduces allocations in the hot path.

---

### 2. Simplicity and Auditability

**Custom implementation**: 80 lines of logic
- 3-state machine (Closed → Open → HalfOpen)
- Consecutive failure/success counting
- RWMutex for thread safety
- Stdlib-only dependencies (sync, time, context, fmt)

**Third-party alternatives**: 300-2000+ lines
- Complex configuration structs
- Generic wrappers and factories
- Features we don't use (SLO tracking, rolling windows, Hystrix dashboards)
- External dependencies to track and update

**Principle**: "Simple is better than complex." 80 lines of domain-specific code is easier to maintain than 40 lines + adapter + external dependency.

---

### 3. Zero Supply Chain Risk

**Custom implementation**:
- No external dependencies
- No version conflicts
- No CVE monitoring required
- No supply chain attacks possible

**Third-party libraries**:
- External dependency to track
- Potential security vulnerabilities
- Version conflicts with other dependencies
- Supply chain attack surface

**Critical**: Telemetry enrichment is production infrastructure. Zero dependencies = zero supply chain risk.

---

### 4. Comprehensive Test Coverage

**Current tests** (`circuit_breaker_test.go`, 6 tests):
1. State transitions (Closed → Open → HalfOpen → Closed)
2. Fail-fast behavior when circuit open
3. Timeout-based recovery attempts
4. Success-based recovery from half-open
5. Manual reset functionality
6. Thread safety (100 concurrent goroutines)

**Tests are domain-specific**:
- Use actual `Enricher` interface
- Test `TelemetryEvent` return behavior
- Verify graceful degradation (original event on failure)

**Migration would require**:
- Rewriting all tests for generic wrappers
- Adapting to library-specific behavior
- Risk of coverage gaps during migration

---

### 5. ROI Analysis

**Migration costs**:
- Development: 3-5 days (adapter layer + testing + debugging)
- Risk: Behavioral changes in production telemetry
- Ongoing: Dependency tracking, updates, security monitoring

**Migration benefits**:
- Metrics hooks (can add in 10-20 lines if needed)
- Community bugfixes (simple code has low bug surface)
- Features (SLO tracking, rolling windows - **we don't need these**)

**Net LOC reduction**: ~40 lines after adapter layer (not the target 120 LOC)

**Conclusion**: Poor ROI. Migration effort outweighs benefits.

---

## Consequences

### Positive

✅ **No external dependencies**: Zero supply chain risk
✅ **Domain-specific API**: Clean, type-safe enricher integration
✅ **Simple codebase**: 80 lines easy to audit and debug
✅ **Well-tested**: Comprehensive coverage of all states
✅ **Production-proven**: Used in telemetry pipeline since implementation

### Negative

❌ **No built-in metrics**: Must add hooks manually if observability needed
❌ **No community updates**: Responsible for own bugfixes (but simple code = low bug rate)
❌ **No advanced features**: No rolling windows, SLO tracking, rate-based triggers

### Neutral

⚪ **Extensibility**: Can add metrics hooks in 10-20 lines if needed
⚪ **Observability**: Can implement callbacks for state changes if required
⚪ **Monitoring**: Simple enough to add Prometheus integration if needed

---

## Implementation

**No code changes required.** This ADR documents the decision to keep the current implementation.

**Future enhancements** (if needed):
1. Add metrics hooks interface (~15-20 LOC)
2. Add state change callbacks (~10-15 LOC)
3. Add Prometheus integration (~50-75 LOC if observability requirement emerges)

---

## Future Review Triggers

Reconsider this decision if any of the following occur:

1. **Metrics requirement**: Prometheus/observability becomes critical for circuit breaker monitoring
2. **Advanced strategies**: Rate-based or sliding window triggering needed (current consecutive-only insufficient)
3. **Multiple use cases**: Circuit breaker pattern needed across 5+ packages (standardization benefit increases)
4. **Organizational standard**: Company-wide library standardization mandate
5. **Maintenance burden**: Custom implementation requires significant ongoing work (unlikely given simplicity)

**Current status**: None of these triggers apply as of 2026-03-21.

---

## References

**Current Implementation**:
- `core/internal/telemetry/enrichment/circuit_breaker.go` - 80 lines of logic
- `core/internal/telemetry/enrichment/circuit_breaker_test.go` - 6 comprehensive tests
- `core/internal/telemetry/enrichment/pipeline.go` - Integration point showing domain-specific usage

**Evaluation Documentation**:
- Language Audit ROADMAP: Phase 6, Task 6.3
- Third-party library comparison matrix (see Alternatives Considered section)

**Related ADRs**:
- (Future) ADR-002: If metrics hooks added
- (Future) ADR-003: If observability integration added

---

**Approved By**: Language Audit Phase 6 Evaluation
**Review Date**: 2026-03-21
**Next Review**: When review triggers occur (see Future Review Triggers)
