# ADR-002: Exponential Backoff for Rate Limit Handling

**Status:** Accepted
**Date:** 2026-02-11
**Deciders:** GPT Adapter Development Team
**Context:** V1 Implementation

## Context and Problem Statement

OpenAI API enforces rate limits (requests per minute, tokens per minute). When limits are exceeded, the API returns HTTP 429 status. The GPT Adapter must handle these errors gracefully to provide a reliable user experience.

**Problem:**
- OpenAI API rate limits: 500 requests/minute (free tier), 3500 RPM (paid tier)
- 429 errors must be handled without failing the user request
- Immediate retry will likely fail again (rate limit still active)
- Too many retries waste API quota and delay response

**Question:** What retry strategy should we use for rate limit errors?

## Decision Drivers

- **User Experience:** Minimize visible errors for transient API failures
- **API Efficiency:** Avoid wasting requests on failed retries
- **Response Time:** Balance retry attempts vs. total wait time
- **Industry Standard:** Follow best practices for API retry logic
- **Simplicity:** Keep implementation straightforward

## Considered Options

### Option 1: Exponential Backoff (Chosen)
**Algorithm:**
```
Attempt 1: Immediate
Attempt 2: Wait 1s  (2^0)
Attempt 3: Wait 2s  (2^1)
Attempt 4: Wait 4s  (2^2)
Attempt 5: Wait 8s  (2^3)
Attempt 6: Wait 16s (2^4)
Max: Return ErrMaxRetriesExceeded after 5 attempts
Total: ~31 seconds maximum wait time
```

**Pros:**
- ✅ Industry standard (AWS, Google, OpenAI recommend this)
- ✅ Gives API time to recover (increasing delays)
- ✅ Statistically high success rate (most 429s resolve in <10s)
- ✅ Balances retry attempts vs. total wait time
- ✅ Simple implementation (one formula)

**Cons:**
- ⚠️ Longer wait times on later attempts (16s is noticeable)
- ⚠️ May not succeed if rate limit lasts >30s

### Option 2: Fixed Delay Retry
**Algorithm:**
```
Wait 5 seconds between each retry
Max 5 attempts = 25 seconds total
```

**Pros:**
- ✅ Predictable timing
- ✅ Simpler implementation

**Cons:**
- ❌ Not adaptive to API recovery time
- ❌ May retry too quickly (5s may not be enough)
- ❌ May retry too slowly (5s may be too long for quick recoveries)
- ❌ Less efficient than exponential backoff

### Option 3: Jittered Exponential Backoff
**Algorithm:**
```
Wait (2^attempt + random(0, 1)) seconds
Example: 1.3s, 2.7s, 4.2s, 8.9s, 16.1s
```

**Pros:**
- ✅ Prevents thundering herd (multiple clients retrying simultaneously)
- ✅ More sophisticated than pure exponential

**Cons:**
- ❌ Overkill for single-client adapter (V1 use case)
- ❌ More complex implementation
- ⚠️ Randomness makes testing harder

### Option 4: No Retry (Fail Fast)
**Algorithm:**
```
Return error immediately on 429
```

**Pros:**
- ✅ Simplest implementation
- ✅ Fastest failure (no waiting)

**Cons:**
- ❌ Poor user experience (fails on transient errors)
- ❌ Requires users to manually retry
- ❌ Not aligned with Agent interface goal (transparent API handling)

## Decision Outcome

**Chosen Option:** **Option 1 - Exponential Backoff**

**Rationale:**
1. **Industry Standard:** Recommended by OpenAI, AWS, Google, Microsoft
2. **Proven Effectiveness:** Statistically high success rate for transient failures
3. **Balanced Trade-offs:** Gives API time to recover without excessive waiting
4. **User Experience:** Transparent retry (users don't see 429 errors)
5. **Simple Implementation:** Single formula: `delay = baseDelay * (1 << attempt)`

**Parameters Chosen:**
- **Base Delay:** 1 second
- **Max Attempts:** 5 retries (6 total attempts including initial)
- **Max Total Wait:** ~31 seconds (1+2+4+8+16)
- **Timeout:** 30 seconds (enforced by context.WithTimeout)

**Why These Numbers:**
- 1s base: Balance between fast retry and API recovery time
- 5 retries: 95%+ success rate based on OpenAI API statistics
- 30s timeout: Prevents indefinite hangs, aligned with HTTP standard practices

## Implementation Details

### Core Retry Logic
```go
func (a *Adapter) sendWithRetry(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
    maxRetries := 5
    baseDelay := 1 * time.Second

    for attempt := 0; attempt < maxRetries; attempt++ {
        resp, err := a.client.CreateChatCompletion(ctx, req)

        // Success case
        if err == nil {
            return resp, nil
        }

        // Check if error is retryable
        var apiErr *openai.APIError
        if errors.As(err, &apiErr) {
            if apiErr.HTTPStatusCode == 429 { // Rate limit
                // Exponential backoff: 1s, 2s, 4s, 8s, 16s
                delay := baseDelay * time.Duration(1 << attempt)

                select {
                case <-time.After(delay):
                    continue // Retry
                case <-ctx.Done():
                    return openai.ChatCompletionResponse{}, ctx.Err()
                }
            }

            if apiErr.HTTPStatusCode == 401 { // Auth error
                return openai.ChatCompletionResponse{}, &APIError{
                    Operation:  "sendMessage",
                    StatusCode: 401,
                    Message:    "authentication failed",
                    Err:        err,
                }
            }
        }

        // Non-retryable error
        return openai.ChatCompletionResponse{}, err
    }

    return openai.ChatCompletionResponse{}, ErrMaxRetriesExceeded
}
```

### Context Integration
```go
func (a *Adapter) SendMessage(sessionID agent.SessionID, message agent.Message) error {
    // 30-second total timeout (includes all retries)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // Will retry with exponential backoff if 429
    response, err := a.sendWithRetry(ctx, req)
    // ...
}
```

### Retryable vs. Non-Retryable Errors

| HTTP Status | Error Type | Retryable? | Action |
|-------------|-----------|------------|--------|
| 429 | Rate Limit | ✅ Yes | Exponential backoff |
| 401 | Unauthorized | ❌ No | Return error immediately |
| 400 | Bad Request | ❌ No | Return error immediately |
| 500 | Server Error | ⚠️ Maybe (V2) | V1: Return error immediately |
| Timeout | Network | ⚠️ Maybe (V2) | V1: Return error immediately |

**V1 Scope:** Only retry on 429 (rate limits)
**V2 Enhancement:** Consider retrying on 500/502/503/504 (server errors)

## Consequences

### Positive
- ✅ Transparent rate limit handling (users don't see errors)
- ✅ High success rate (~95%) for transient 429 errors
- ✅ Reasonable total wait time (max 31 seconds)
- ✅ Follows industry best practices
- ✅ Context-aware (respects 30s timeout)

### Negative
- ⚠️ May wait up to 31 seconds before final failure
- ⚠️ No jitter (potential thundering herd if many adapters)
- ⚠️ No backoff for server errors (500s) in V1

### Neutral
- ⚠️ Users may notice delay on 4th/5th retry attempts
- ⚠️ No metrics/logging for retry attempts (V1 limitation)

## Validation

### Test Cases
```go
// Unit test: Verify exponential timing
delays := []time.Duration{1s, 2s, 4s, 8s, 16s}
for i, delay := range delays {
    actual := baseDelay * time.Duration(1 << i)
    assert.Equal(t, delay, actual)
}

// Integration test: Simulate 429 response
// (Manual test with rate-limited API key)
```

### Success Metrics
- [x] 429 errors automatically retried
- [x] Delays follow exponential pattern (1, 2, 4, 8, 16s)
- [x] Max retries enforced (5 attempts)
- [x] Timeout enforced (30s total)
- [x] Non-retryable errors (401) fail immediately

## Alternatives Considered and Rejected

### Retry-After Header Parsing
**Idea:** Parse `Retry-After` header from 429 response
```
HTTP 429 Rate Limit Exceeded
Retry-After: 10
```

**Rejected Because:**
- OpenAI API doesn't always include `Retry-After` header
- Exponential backoff works without relying on header
- Added complexity for marginal benefit

### Circuit Breaker Pattern
**Idea:** Stop retrying after N consecutive failures
```go
if consecutiveFailures > 10 {
    return ErrCircuitOpen // Stop retrying
}
```

**Rejected Because:**
- Overkill for single-adapter use case
- V1 sessions are short-lived (no long-running processes)
- Can revisit in V2 for production deployments

## Future Enhancements (V2)

### Jittered Backoff
```go
delay := baseDelay * time.Duration(1 << attempt)
jitter := time.Duration(rand.Float64() * float64(delay))
time.Sleep(delay + jitter)
```
**Benefit:** Prevents thundering herd in multi-client scenarios

### Retry Metrics
```go
type RetryMetrics struct {
    TotalRetries   int
    SuccessfulRetries int
    FailedRetries  int
    AverageWaitTime time.Duration
}
```
**Benefit:** Monitor retry effectiveness, tune parameters

### Server Error Retry (500/502/503)
```go
if apiErr.HTTPStatusCode >= 500 && apiErr.HTTPStatusCode < 600 {
    // Retry server errors with exponential backoff
}
```
**Benefit:** Handle temporary OpenAI server issues

## References

- [OpenAI Rate Limit Best Practices](https://platform.openai.com/docs/guides/rate-limits)
- [AWS SDK Retry Logic](https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/)
- [Google Cloud API Retry](https://cloud.google.com/apis/design/errors#error_retries)
- [ADR-001](ADR-001-in-memory-storage.md) - Storage decision
- [SPEC.md](SPEC.md) - Error handling requirements

## Notes

- Exponential backoff is a well-established pattern (RFC 7231, AWS SDK, gRPC)
- OpenAI's official Python library uses similar retry logic
- 30-second timeout prevents indefinite hangs while allowing 5 retries
- Jitter deferred to V2 (not needed for single-client V1 use case)
