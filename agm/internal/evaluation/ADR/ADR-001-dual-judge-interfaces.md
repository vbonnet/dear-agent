# ADR-001: Use Both Simple and Detailed Judge Interfaces

## Status

Accepted

## Context

The EDD Framework initially provided a simple `Judge` interface that returned only a numeric score:

```go
type Judge interface {
    Evaluate(ctx context.Context, prompt string, response string) (float64, error)
}
```

This interface was sufficient for basic use cases, but as the framework evolved, we identified several limitations:

1. **Limited Information**: Users only received a score without understanding why
2. **No Pass/Fail Indication**: Users had to manually compare scores to thresholds
3. **Missing Reasoning**: No explanation of the judgment for debugging
4. **No Criteria Support**: Couldn't specify evaluation criteria

Meanwhile, several production users had already integrated the simple `Judge` interface into their systems. Changing or removing this interface would break their code.

## Decision

We will maintain **both** the simple `Judge` interface and a new, more detailed `DetailedJudge` interface:

```go
// Simple interface (legacy, maintained for backward compatibility)
type Judge interface {
    Evaluate(ctx context.Context, prompt string, response string) (float64, error)
}

// Detailed interface (new, recommended for new code)
type DetailedJudge interface {
    EvaluateDetailed(ctx context.Context, input, expectedOutput string,
                     criteria EvaluationCriteria) (*JudgeResponse, error)
}

// JudgeResponse provides rich evaluation results
type JudgeResponse struct {
    Pass      bool    // Whether the output meets the criteria
    Score     float64 // Normalized score 0.0-1.0
    Reasoning string  // Explanation of the judgment (includes CoT)
}
```

All judge implementations (GPT4Judge, ClaudeJudge) will implement **both** interfaces.

## Consequences

### Positive

1. **Backward Compatibility**: Existing code using `Judge` continues to work without changes
2. **Smooth Migration Path**: Users can migrate to `DetailedJudge` at their own pace
3. **Richer Information**: New interface provides pass/fail, score, and reasoning
4. **Better Debugging**: Reasoning helps users understand why evaluations fail
5. **Criteria Support**: Explicit criteria makes expectations clear

### Negative

1. **API Surface Complexity**: Two interfaces instead of one increases cognitive load
2. **Maintenance Burden**: Must maintain both interfaces in perpetuity
3. **Documentation Overhead**: Must document both interfaces and migration path
4. **Testing Effort**: Must test both interfaces for all judge implementations

### Neutral

1. **Code Duplication**: Some logic duplicated between implementations
2. **Interface Evolution**: Future enhancements must consider both interfaces

## Implementation Notes

### Migration Path for Users

**Old Code (still works):**
```go
judge := evaluation.NewGPT4Judge(apiKey, nil)
score, err := judge.Evaluate(ctx, prompt, response)
if err != nil {
    return err
}
if score >= 0.8 {
    // Pass
}
```

**New Code (recommended):**
```go
judge := evaluation.NewGPT4Judge(apiKey, nil)
criteria := evaluation.EvaluationCriteria{
    Name:        "correctness",
    Description: "The answer should be accurate",
    Threshold:   0.8,
}
resp, err := judge.EvaluateDetailed(ctx, input, expected, criteria)
if err != nil {
    return err
}
if resp.Pass {
    log.Printf("Passed with score %.2f: %s", resp.Score, resp.Reasoning)
}
```

### Internal Implementation

Both methods can share the same core evaluation logic:

```go
func (j *GPT4Judge) Evaluate(ctx context.Context, prompt, response string) (float64, error) {
    // Simple wrapper around detailed evaluation
    criteria := EvaluationCriteria{
        Name:        "quality",
        Description: "Evaluate the quality of the response",
        Threshold:   0.0, // No threshold for simple interface
    }
    resp, err := j.EvaluateDetailed(ctx, prompt, "", criteria)
    if err != nil {
        return 0, err
    }
    return resp.Score, nil
}
```

## Alternatives Considered

### Alternative 1: Replace the Simple Interface

**Rejected Reason**: Would break existing code and force all users to migrate immediately

### Alternative 2: Add Optional Parameters to Existing Interface

```go
type Judge interface {
    Evaluate(ctx context.Context, prompt, response string, opts ...Option) (Result, error)
}
```

**Rejected Reason**:
- Still breaks backward compatibility (return type changes)
- Optional parameters make API confusing
- Hard to maintain type safety

### Alternative 3: Separate Judge Types

```go
type SimpleJudge interface { Evaluate(...) (float64, error) }
type DetailedJudge interface { EvaluateDetailed(...) (*Response, error) }
```

**Rejected Reason**:
- Fragments the API
- Forces users to choose upfront
- No migration path

## References

- Initial discussion: GitHub issue #123
- Migration guide: docs/migration.md
- API documentation: README.md#judge-interfaces

## Revision History

- 2024-12-01: Initial decision
- 2025-01-15: Updated with implementation notes
- 2026-02-20: Added to EDD Framework documentation
