# ADR-003: API Ranking Fallback Strategy

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Handling API ranking availability and failures

---

## Context

The retrieval service supports two search modes:

1. **Index-Only**: Fast frontmatter filtering (tags, type) without semantic ranking
2. **API-Powered**: Semantic relevance ranking via Anthropic API (tier-2 in ecphory)

API-powered ranking requires ANTHROPIC_API_KEY and has failure scenarios:

- **No API Key**: Key not set in environment (common in CI/CD, development)
- **Ranker Init Failure**: API key invalid or network issues during ranker creation
- **Ranking Failure**: API call fails mid-operation (rate limit, network error)

**Question**: How should the service handle API ranking failures?

**Options**:
1. **Fail Hard**: Return error on any API issue (explicit failure)
2. **Silent Fallback**: Always fall back to index-only (invisible degradation)
3. **Conditional Fallback**: Fall back on missing key, error on other failures

---

## Decision

We will use **Conditional Fallback** strategy:

- **Missing API Key** → Silent fallback to index-only (expected configuration state)
- **Ranker Errors** → Return error to caller (API was available but failed)
- **Ranking Errors** → Return error to caller (API call failed mid-operation)

**Implementation**:
```go
func (s *Service) Search(ctx context.Context, opts SearchOptions) ([]*SearchResult, error) {
    // ... filter candidates ...

    if opts.UseAPI {
        apiKey := os.Getenv("ANTHROPIC_API_KEY")
        if apiKey == "" {
            // Silent fallback: No API key → index-only
            resultPaths = s.limitResults(candidates, opts.Limit)
        } else {
            // API key present: Try ranking, error on failure
            ranker, err := ecphory.NewRanker()
            if err != nil {
                return nil, fmt.Errorf("failed to create ranker: %w", err)
            }

            ranked, err := ranker.Rank(ctx, opts.Query, candidates)
            if err != nil {
                return nil, fmt.Errorf("failed to rank candidates: %w", err)
            }

            // Extract top results
            resultPaths = extractTopN(ranked, opts.Limit)
        }
    } else {
        // Index-only: No API requested
        resultPaths = s.limitResults(candidates, opts.Limit)
    }

    // ... parse and return results ...
}
```

**Decision Matrix**:
| Scenario | UseAPI | API Key | Behavior |
|----------|--------|---------|----------|
| Index-only requested | false | N/A | Index-only (no API check) |
| API requested, no key | true | missing | Fallback to index-only |
| API requested, key present | true | present | Use API, error on failure |

---

## Consequences

### Positive

**Graceful Degradation in Common Case**:
- CI/CD environments (no API key) → searches work, just without semantic ranking
- Development (no API key) → developers can test without credentials
- No surprise failures for users who don't configure API key

**Clear Error Signals for Real Failures**:
- Invalid API key → error (helps user discover misconfiguration)
- Network issues → error (alerts user to transient failure)
- Rate limits → error (user can retry or switch to index-only)

**Explicit Intent Respecting**:
- UseAPI=false → never touches API (even if key present)
- UseAPI=true + no key → fallback (expected config state)
- UseAPI=true + key present → strict mode (errors on failure)

**Observability**:
- Callers can detect fallback (results have Score=0, Ranking="")
- Errors are descriptive (includes wrapped ecphory error)
- Logs distinguish fallback from error cases

### Negative

**Silent Fallback May Hide Issues**:
- User requests API (UseAPI=true) but gets index-only due to missing key
- No warning message (only detectable via result metadata)
- This is **acceptable** because:
  - Missing key is expected state (not an error)
  - Result metadata reveals mode (Score=0 → index-only)
  - Verbose logging would spam CI/CD output

**No Partial Degradation**:
- If API call fails, entire search fails (no "rank what we can")
- Could cache partial ranking results and continue
- This is **acceptable** because:
  - Partial rankings are misleading (incomplete semantic scoring)
  - Retry is simple (just re-run search)
  - Index-only fallback is available (UseAPI=false)

**API Key Check Every Search**:
- `os.Getenv("ANTHROPIC_API_KEY")` called per search (not cached)
- Minimal overhead (~microseconds) but repeats work
- This is **acceptable** because:
  - Allows dynamic API key changes (e.g., credential rotation)
  - Per-search index building pattern (consistent with ADR-001)
  - Overhead negligible compared to index build (~1s)

---

## Alternatives Considered

### Alternative 1: Fail Hard (No Fallback)

**Approach**:
```go
if opts.UseAPI {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    if apiKey == "" {
        return nil, errors.New("ANTHROPIC_API_KEY not set")
    }
    // ... rank or error ...
}
```

**Rejected Because**:
- **Poor CI/CD Experience**: Tests fail without API key (requires mocking)
- **Development Friction**: Every developer needs API credentials
- **Unnecessary Strictness**: Missing key is expected config, not error
- **Use Case**: CLI users want quick filter (tags) without API overhead

### Alternative 2: Silent Fallback Always

**Approach**:
```go
if opts.UseAPI {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    if apiKey == "" {
        // Fallback
        resultPaths = s.limitResults(candidates, opts.Limit)
    } else {
        ranker, err := ecphory.NewRanker()
        if err != nil {
            // Fallback on error (silent)
            resultPaths = s.limitResults(candidates, opts.Limit)
        } else {
            ranked, err := ranker.Rank(ctx, opts.Query, candidates)
            if err != nil {
                // Fallback on error (silent)
                resultPaths = s.limitResults(candidates, opts.Limit)
            }
            // Use ranked results
        }
    }
}
```

**Rejected Because**:
- **Hides Real Errors**: Network failures, invalid keys are silent
- **Debugging Nightmare**: Users don't know why API ranking isn't working
- **Violated Expectations**: UseAPI=true implies "try hard to use API"
- **No Error Feedback**: Caller can't distinguish intentional vs broken fallback

### Alternative 3: Warning Logs + Fallback

**Approach**:
```go
if apiKey == "" {
    log.Warn("ANTHROPIC_API_KEY not set, falling back to index-only")
    // Fallback
}
```

**Rejected Because**:
- **Log Spam**: CI/CD logs filled with warnings (noise)
- **Not Actionable**: Warning doesn't help (missing key is expected)
- **Inconsistent**: Some callers may not want warnings (e.g., testing)
- **Better Alternative**: Caller can log if needed (check Score=0 in results)

### Alternative 4: Fallback Flag in Options

**Approach**:
```go
type SearchOptions struct {
    UseAPI bool
    FallbackOnAPIFailure bool // New field
}

if opts.UseAPI {
    // ... try API ...
    if err != nil && opts.FallbackOnAPIFailure {
        // Fallback
    } else if err != nil {
        return nil, err
    }
}
```

**Rejected Because**:
- **API Complexity**: Two boolean flags for single decision
- **Confusing Semantics**: UseAPI=true + FallbackOnAPIFailure=false means what?
- **No Clear Use Case**: When would user want strict API-only mode?
- **YAGNI**: Current strategy covers all known scenarios

---

## Implementation Notes

**API Key Check** (lines 112-116 in `retrieval.go`):
```go
if opts.UseAPI {
    apiKey := os.Getenv("ANTHROPIC_API_KEY")
    if apiKey == "" {
        // Fallback to index-only
        resultPaths = s.limitResults(candidates, opts.Limit)
    } else {
        // Attempt API ranking
```

**Error Propagation** (lines 118-126 in `retrieval.go`):
```go
ranker, err := ecphory.NewRanker()
if err != nil {
    return nil, fmt.Errorf("failed to create ranker: %w", err)
}

ranked, err := ranker.Rank(ctx, opts.Query, candidates)
if err != nil {
    return nil, fmt.Errorf("failed to rank candidates: %w", err)
}
```

**Fallback Detection** (in results):
```go
// Index-only results have no score/ranking
for _, r := range results {
    if r.Score == 0 && r.Ranking == "" {
        // Index-only mode (fallback or UseAPI=false)
    }
}
```

**Example: CLI with Fallback**:
```bash
# No API key → silent fallback
$ engram retrieve --query "testing" --use-api
# Returns index-only results (no error)

# API key present, network error → error returned
$ ANTHROPIC_API_KEY=sk-... engram retrieve --query "testing" --use-api
Error: failed to rank candidates: network timeout
```

---

## Related Decisions

- **ADR-001**: Per-Search Index Building (fallback uses same index)
- **ADR-002**: Tracking Integration (tracks results regardless of ranking mode)

---

## Testing Strategy

**Unit Tests** (lines 366-431 in `retrieval_test.go`):
```go
func TestService_Search_WithAPIFallback(t *testing.T) {
    t.Run("api key missing fallback", func(t *testing.T) {
        os.Unsetenv("ANTHROPIC_API_KEY")
        results, err := service.Search(ctx, SearchOptions{UseAPI: true})
        // Verify: no error, results returned, Score=0 (fallback)
    })

    t.Run("useAPI false skips ranking", func(t *testing.T) {
        results, err := service.Search(ctx, SearchOptions{UseAPI: false})
        // Verify: no API call, Score=0
    })
}
```

**Integration Tests** (lines 10-206 in `retrieval_integration_test.go`):
```go
func TestRetrieval_WithConfig_Integration(t *testing.T) {
    if os.Getenv("ANTHROPIC_API_KEY") == "" {
        t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
    }
    // Test real API ranking (requires key)
}
```

---

## Future Considerations

**Metrics/Observability**:
Add optional callback for fallback events:
```go
type FallbackObserver interface {
    OnFallback(reason string)
}

func (s *Service) SetObserver(obs FallbackObserver) {
    s.observer = obs
}

// In Search():
if apiKey == "" {
    s.observer.OnFallback("missing_api_key")
    // Fallback...
}
```

**Retry Logic**:
Add automatic retry for transient errors:
```go
ranked, err := ranker.RankWithRetry(ctx, query, candidates, 3)
if err != nil {
    // Only error after retries exhausted
}
```

---

## References

- **Pattern**: Graceful Degradation
- **Trade-off**: Availability vs Strictness
- **Related Package**: `github.com/vbonnet/engram/core/pkg/ecphory`

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)
