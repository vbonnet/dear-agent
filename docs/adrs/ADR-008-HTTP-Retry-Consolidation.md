# ADR-008: HTTP Retry Consolidation with hashicorp/go-retryablehttp

**Status**: Draft
**Date**: 2026-03-20
**Context**: Task 7.2 - HTTP Retry Consolidation

## Context

The ai-tools repository contains multiple custom HTTP retry implementations:

1. **tools/deep-research/research/retry.go** (122 LOC)
   - Custom `RetryConfig` struct
   - `IsRetryable()` function (handles 4xx/5xx logic)
   - `RetryWithBackoff()` with exponential backoff
   - Full test coverage (8 tests in retry_test.go)

2. **internal/sandbox/cleanup.go**
   - `unmountWithRetry()` - filesystem operation retry
   - `removeWithRetry()` - directory removal retry
   - NOT HTTP-related (filesystem operations)

3. **agm/internal/temporal/workflows/escalation_workflow.go**
   - `getRetryPolicyForSeverity()` - Temporal workflow retry policies
   - NOT HTTP retry (Temporal-specific)

**Current State**: Custom retry logic is duplicated across projects, inconsistent retry policies, no circuit breaker patterns, 150+ LOC that could be replaced by standard library.

## Decision

Migrate HTTP retry logic to **hashicorp/go-retryablehttp** for:
- tools/deep-research (HTTP Deep Research API calls)
- Any other HTTP client code requiring retry logic

**Do NOT migrate**:
- internal/sandbox filesystem retries (not HTTP-related)
- agm Temporal retries (framework-specific)

## Rationale

### Why hashicorp/go-retryablehttp?

**Pros**:
- Industry-standard library (15k+ GitHub stars)
- Drop-in replacement for `http.Client`
- Configurable retry policies (custom `CheckRetry` function)
- Exponential backoff with jitter
- Circuit breaker support
- Request/response logging hooks
- Well-tested and maintained

**Cons**:
- Additional dependency
- Learning curve for custom retry logic

### Alternatives Considered

1. **Keep custom retry.go**
   - ❌ Duplicates standard functionality
   - ❌ More code to maintain
   - ❌ Missing features (jitter, circuit breaker)

2. **stdlib net/http with custom retry**
   - ❌ Still custom code
   - ❌ No retry-specific features

3. **go-resty/resty**
   - ❌ Full HTTP client library (overkill)
   - ❌ Different API than stdlib

## Implementation Plan

### Phase 1: Multi-Deep-Research Migration

**Files to modify**:
- `tools/deep-research/research/client.go` - Use retryablehttp.Client
- `tools/deep-research/research/retry.go` - Delete or repurpose
- `tools/deep-research/research/retry_test.go` - Update tests
- `tools/deep-research/go.mod` - Add dependency

**Steps**:
1. Add `github.com/hashicorp/go-retryablehttp` to go.mod
2. Create `NewRetryableHTTPClient()` wrapper
3. Configure retry policy matching current behavior:
   - Max retries: 3
   - Initial backoff: 1s
   - Max backoff: 30s
   - Retry on: 5xx, 429, network errors
   - No retry on: 4xx (except 429)
4. Update all HTTP call sites
5. Migrate tests to verify retry behavior
6. Delete old retry.go after verification

### Phase 2: Other HTTP Clients (If Any)

Audit codebase for other HTTP retry implementations and migrate.

## Migration Strategy

### Retry Policy Mapping

**Current (research/retry.go)**:
```go
IsRetryable(err error) bool {
    - Retry on context.DeadlineExceeded
    - Retry on 5xx errors
    - Retry on 429 (rate limit)
    - NO retry on 4xx (except 429)
}
```

**New (retryablehttp)**:
```go
CheckRetry: func(ctx context.Context, resp *http.Response, err error) (bool, error) {
    // Same logic as IsRetryable
    if err != nil {
        return err == context.DeadlineExceeded, nil
    }
    if resp.StatusCode >= 500 && resp.StatusCode < 600 {
        return true, nil
    }
    if resp.StatusCode == 429 {
        return true, nil
    }
    return false, nil
}
```

### Backoff Configuration

**Current**: Exponential backoff: `2^attempt * InitialBackoff` capped at `MaxBackoff`

**New**: `retryablehttp.DefaultBackoff` provides exponential backoff with jitter (better than our custom implementation)

## Testing Strategy

1. **Unit tests**: Verify retry policy with mock HTTP server
2. **Integration tests**: Test real API calls with retry scenarios
3. **Regression tests**: Ensure no behavior changes from old implementation
4. **Performance tests**: Benchmark retry overhead

## Risks

**Risk 1: Behavior Changes**
- Mitigation: Comprehensive test coverage, A/B testing with old vs new

**Risk 2: Dependency Issues**
- Mitigation: hashicorp libs are stable, well-maintained, widely used

**Risk 3: Performance Regression**
- Mitigation: Benchmark before/after, retryablehttp is performant

## Success Criteria

- ✅ All HTTP retry logic uses retryablehttp
- ✅ Zero regressions in retry behavior
- ✅ 100+ LOC reduction (retry.go deleted)
- ✅ All tests passing
- ✅ No custom retry implementations for HTTP

## References

- hashicorp/go-retryablehttp: https://github.com/hashicorp/go-retryablehttp
- Current implementation: `tools/deep-research/research/retry.go`
- Tests: `tools/deep-research/research/retry_test.go`
