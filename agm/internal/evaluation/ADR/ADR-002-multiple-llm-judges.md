# ADR-002: Support Multiple LLM Judges (GPT-4 and Claude)

## Status

Accepted

## Context

LLM-as-a-judge has become a standard technique for evaluating AI system outputs. However, different LLM providers have different characteristics:

### GPT-4 (OpenAI)
- **Strengths**: Structured output via response_format, consistent JSON, well-documented API
- **Weaknesses**: Higher cost, vendor lock-in, rate limits
- **Use Cases**: Production systems requiring reliability, structured evaluations

### Claude (Anthropic)
- **Strengths**: Longer context windows, strong reasoning, competitive pricing
- **Weaknesses**: Less structured output (must parse from freeform), newer API
- **Use Cases**: Complex reasoning tasks, cost optimization, long-context evaluation

### Key Concerns

1. **Vendor Lock-in**: Relying on a single LLM provider creates dependency risk
2. **Cost Optimization**: Different providers have different pricing models
3. **Quality Variation**: Some models better at specific evaluation types
4. **Availability**: Provider outages or rate limits affect all evaluations
5. **Flexibility**: Users want choice based on their specific needs

## Decision

We will support **multiple LLM judges** by:

1. Defining a common `Judge` and `DetailedJudge` interface
2. Implementing both GPT-4 and Claude judge variants
3. Allowing users to choose their preferred judge at runtime
4. Ensuring consistent behavior across implementations

**Initial Implementations:**
- `GPT4Judge`: Uses OpenAI GPT-4 API
- `ClaudeJudge`: Uses Anthropic Claude API

**Future Implementations:**
- Other providers can be added by implementing the interface
- Users can implement custom judges for proprietary models

## Consequences

### Positive

1. **Vendor Independence**: Not locked into single provider
2. **Cost Flexibility**: Choose based on budget constraints
3. **Quality Options**: Select best model for specific use cases
4. **Reliability**: Fallback to alternative if primary unavailable
5. **Competition**: Providers compete on quality and pricing
6. **Ensemble Evaluation**: Can run multiple judges for consensus

### Negative

1. **Implementation Cost**: Must implement and maintain multiple judge variants
2. **Testing Complexity**: Must test with multiple APIs
3. **Documentation Burden**: Document differences and trade-offs
4. **API Key Management**: Users must manage multiple API keys
5. **Inconsistent Results**: Different models produce different scores
6. **Feature Parity**: New provider features may not be universally available

### Neutral

1. **Interface Design**: Must be general enough for all providers
2. **Configuration**: Need flexible config system for provider-specific options
3. **Error Handling**: Different APIs have different error patterns

## Implementation Details

### Common Interface

```go
type DetailedJudge interface {
    EvaluateDetailed(ctx context.Context, input, expectedOutput string,
                     criteria EvaluationCriteria) (*JudgeResponse, error)
}

type JudgeResponse struct {
    Pass      bool
    Score     float64
    Reasoning string
}
```

### GPT4Judge Implementation

**Key Features:**
- Uses `response_format` for structured JSON output
- Guarantees valid JSON responses
- Leverages OpenAI's function calling infrastructure

**Configuration:**
```go
type GPT4Config struct {
    Model       string  // Default: "gpt-4-turbo-preview"
    Temperature float64 // Default: 0.1
    MaxTokens   int     // Default: 1000
}
```

**Prompting:**
- System message with evaluation instructions
- User message with input, expected, and criteria
- JSON schema for structured response

### ClaudeJudge Implementation

**Key Features:**
- Uses assistant prefill for Chain-of-Thought
- Extracts JSON from freeform text
- Robust parsing with multiple fallback strategies

**Configuration:**
```go
type ClaudeConfig struct {
    Model       string  // Default: "claude-3-7-sonnet-20250219"
    Temperature float64 // Default: 0.1
    MaxTokens   int     // Default: 1000
}
```

**Prompting:**
- System message in dedicated field
- Assistant prefill: "Let me analyze this step by step:"
- Extracts JSON from ```json blocks or direct objects

### Usage Example

```go
// User chooses judge at runtime
var judge evaluation.DetailedJudge

if useGPT4 {
    judge = evaluation.NewGPT4Judge(openaiKey, &evaluation.GPT4Config{
        Model:       "gpt-4-turbo-preview",
        Temperature: 0.1,
    })
} else {
    judge = evaluation.NewClaudeJudge(anthropicKey, &evaluation.ClaudeConfig{
        Model:       "claude-3-7-sonnet-20250219",
        Temperature: 0.1,
    })
}

// Same evaluation code works with either judge
criteria := evaluation.EvaluationCriteria{
    Name:        "correctness",
    Description: "Answer should be accurate",
    Threshold:   0.8,
}

resp, err := judge.EvaluateDetailed(ctx, input, expected, criteria)
```

### Ensuring Consistency

Both implementations must:
1. Return scores normalized to 0.0-1.0 range
2. Include reasoning in response
3. Handle identical error cases consistently
4. Support cancellation via context
5. Retry on transient errors (network, timeout)

### Quality Parity

We do NOT guarantee identical scores across judges because:
- Different models have different reasoning capabilities
- Prompting strategies differ due to API differences
- This is an acceptable trade-off for flexibility

Users should:
- **Choose one judge** for a specific use case and stick with it
- **NOT compare** scores across different judge implementations
- **Calibrate thresholds** for their chosen judge

## Migration and Compatibility

### Existing Code

Code using `GPT4Judge` continues to work unchanged.

### New Adoption

New users can choose either judge based on their needs:

**Decision Factors:**
- **Reliability Required**: Choose GPT-4 for guaranteed structured output
- **Cost Sensitive**: Choose Claude for better pricing
- **Long Context**: Choose Claude for longer context windows
- **Structured Output**: Choose GPT-4 for native JSON schemas

## Testing Strategy

### Unit Tests

Each judge implementation has comprehensive tests:
- Mock HTTP responses from APIs
- Test error handling (API errors, malformed responses)
- Verify JSON parsing/extraction logic
- Test configuration options

### Integration Tests

- Test with real APIs using test API keys (optional, CI-only)
- Verify consistency of behavior across judges
- Ensure both return similar scores for identical inputs

### Quality Benchmarking

Not currently implemented, but future work:
- Evaluate both judges on standard benchmarks
- Document quality differences
- Provide guidance on judge selection

## Future Enhancements

### Additional Providers

The architecture supports adding more judges:

**Potential Additions:**
- Gemini (Google)
- Mistral
- Llama (local/hosted)
- Custom fine-tuned models

**Implementation Process:**
1. Implement `Judge` and `DetailedJudge` interfaces
2. Add configuration struct
3. Write comprehensive tests
4. Document in README
5. Add usage examples

### Ensemble Judges

Future enhancement to run multiple judges and aggregate results:

```go
type EnsembleJudge struct {
    judges []DetailedJudge
    aggregator ScoreAggregator // mean, median, consensus, etc.
}
```

### Judge Routing

Automatically select judge based on criteria:

```go
type RouterJudge struct {
    router func(criteria EvaluationCriteria) DetailedJudge
}
```

## Alternatives Considered

### Alternative 1: GPT-4 Only

**Rejected Reason**: Vendor lock-in, single point of failure

### Alternative 2: Abstract API Layer

Create abstraction over LLM APIs before judge implementation.

**Rejected Reason**:
- Over-engineering for current needs
- Adds complexity without clear benefit
- Judge implementations already provide abstraction

### Alternative 3: Plugin System

Load judges dynamically at runtime via plugins.

**Rejected Reason**:
- Unnecessary complexity
- Harder to maintain type safety
- Direct imports simpler and more Go-idiomatic

## References

- GPT-4 API: https://platform.openai.com/docs/api-reference
- Claude API: https://docs.anthropic.com/claude/reference
- LLM-as-a-judge paper: https://arxiv.org/abs/2306.05685
- Implementation code: `gpt4_judge.go`, `claude_judge.go`

## Revision History

- 2024-11-15: Initial decision to support GPT-4 only
- 2024-12-10: Expanded to include Claude support
- 2025-01-20: Added implementation details and usage examples
- 2026-02-20: Added to EDD Framework documentation
