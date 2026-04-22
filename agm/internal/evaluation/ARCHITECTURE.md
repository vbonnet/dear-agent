# EDD Framework Architecture

## System Overview

The EDD (Evaluation-Driven Development) Framework follows a layered architecture with clear separation of concerns:

```
┌──────────────────────────────────────────────────────────────────┐
│                         User Code                                 │
│          (CI/CD Pipelines, Production Services)                   │
└────────────────────────┬─────────────────────────────────────────┘
                         │
┌────────────────────────┴─────────────────────────────────────────┐
│                   Application Layer                               │
│  ┌──────────────────┐  ┌──────────────────┐  ┌────────────────┐ │
│  │ OfflineEvaluator │  │ OnlineEvaluator  │  │ FeedbackLoop   │ │
│  └──────────────────┘  └──────────────────┘  └────────────────┘ │
└────────────────────────┬─────────────────────────────────────────┘
                         │
┌────────────────────────┴─────────────────────────────────────────┐
│                   Evaluation Layer                                │
│  ┌──────────┐  ┌───────────┐  ┌────────────────────────────────┐│
│  │  Judges  │  │  Metrics  │  │  Validators & Criteria         ││
│  └──────────┘  └───────────┘  └────────────────────────────────┘│
└────────────────────────┬─────────────────────────────────────────┘
                         │
┌────────────────────────┴─────────────────────────────────────────┐
│                   Integration Layer                               │
│  ┌──────────┐  ┌─────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │ LLM APIs │  │ Alerters│  │  PR Creator  │  │  File I/O    │ │
│  └──────────┘  └─────────┘  └──────────────┘  └──────────────┘ │
└───────────────────────────────────────────────────────────────────┘
```

## Architecture Principles

### 1. Interface-Driven Design

All major components are defined as interfaces, enabling:
- **Testability**: Easy mocking for unit tests
- **Flexibility**: Multiple implementations (GPT-4, Claude, etc.)
- **Extension**: Users can add custom implementations
- **Decoupling**: Components depend on abstractions, not concrete types

**Example:**
```go
// Interface definition
type Judge interface {
    Evaluate(ctx context.Context, prompt, response string) (float64, error)
}

// Multiple implementations
type GPT4Judge struct { ... }
type ClaudeJudge struct { ... }
type CustomJudge struct { ... }
```

### 2. Dependency Injection

Components receive their dependencies through constructors:
- **Explicit Dependencies**: Clear what each component needs
- **Configuration**: Dependencies configured at application startup
- **Testing**: Easy to inject mocks for testing

**Example:**
```go
// Constructor with dependencies injected
func NewOfflineEvaluator(judge DetailedJudge, config *OfflineConfig) *OfflineEvaluator {
    return &OfflineEvaluator{
        judge:  judge,
        config: config,
    }
}
```

### 3. Graceful Degradation

The system continues functioning even when components fail:
- **Alerter Failures**: Try remaining alerters if one fails
- **Sampling**: Continue monitoring even if individual evaluations fail
- **Metrics**: Use cached/default values if collection fails

**Example:**
```go
for _, alerter := range e.alerters {
    if err := alerter.Alert(message); err != nil {
        log.Printf("Alert failed: %v", err) // Log but continue
    }
}
```

### 4. Fail-Fast for Critical Errors

While gracefully degrading for non-critical errors, fail fast for:
- Invalid configuration (missing API keys, invalid thresholds)
- Programming errors (nil pointers, invalid state)
- Unrecoverable errors (context canceled)

## Component Details

### Application Layer

#### OfflineEvaluator

**Purpose**: Pre-deployment evaluation of test cases

**Dependencies:**
- `DetailedJudge`: For evaluating test cases
- `OfflineConfig`: Configuration (thresholds, blocking rules)

**Key Methods:**
```go
func (e *OfflineEvaluator) EvaluateOffline(ctx context.Context,
    testCases []TestCase) (*OfflineReport, error)
```

**Data Flow:**
```
TestCases → Load → Evaluate (parallel) → Aggregate → Check Thresholds → Report
```

**Concurrency:**
- Evaluates test cases in parallel (up to 10 concurrent goroutines)
- Thread-safe result aggregation using channels
- Context-aware for cancellation

**Error Handling:**
- Returns error if judge is nil
- Returns error if no test cases provided
- Continues evaluation if single test case fails (logs error)

#### OnlineEvaluator

**Purpose**: Production monitoring with real-time metrics

**Dependencies:**
- `DetailedJudge`: For evaluating sampled events
- `[]Alerter`: List of alerter implementations
- `OnlineConfig`: Configuration (thresholds, sampling rate)

**Key Methods:**
```go
func (e *OnlineEvaluator) MonitorProduction(ctx context.Context, samplingRate float64) error
func (e *OnlineEvaluator) ProcessEvent(event SessionEvent)
func (e *OnlineEvaluator) GetMetrics() ProductionMetrics
func (e *OnlineEvaluator) Stop()
```

**Data Flow:**
```
Event → Sample? → Update Metrics → Check Thresholds → Alert (if violation)
```

**Concurrency:**
- Single goroutine for monitoring loop
- Thread-safe metrics collection using sync.RWMutex
- Non-blocking event processing

**State Management:**
- `running`: Boolean flag protected by mutex
- `metrics`: Metrics collector protected by mutex
- `stop`: Channel for shutdown signal

#### FeedbackLoop

**Purpose**: Automated golden dataset updates from production

**Dependencies:**
- `PRCreator`: Interface for creating pull requests
- `goldenDir`: Path to golden dataset directory

**Key Methods:**
```go
func (f *FeedbackLoop) UpdateGoldenDataset(ctx context.Context,
    examples []GoldenExample) error
func (f *FeedbackLoop) GetExampleCount() (int, error)
```

**Data Flow:**
```
Examples → Validate → Deduplicate → Write Files → Create PR
```

**File Management:**
- Creates one file per example: `{timestamp}_{sanitized_name}.json`
- Atomic file writes (write temp, then rename)
- Sanitizes filenames (removes special characters)

### Evaluation Layer

#### Judge Interface Hierarchy

```
         Judge (simple)
            ↑
            │ extends
            │
     DetailedJudge
         ↑     ↑
         │     │
    GPT4Judge  ClaudeJudge
```

**Design Rationale:**
- `Judge`: Legacy interface for backward compatibility
- `DetailedJudge`: New interface with richer responses
- Both interfaces implemented by same types for smooth migration

#### GPT4Judge

**LLM Integration:**
- Uses OpenAI API
- Model: `gpt-4-turbo-preview` (configurable)
- Structured output via `response_format` with JSON schema

**Prompting Strategy:**
```
System: You are an expert evaluator...
User: Evaluate based on criteria...
```

**Response Format:**
```json
{
  "pass": boolean,
  "score": number (0.0-1.0),
  "reasoning": string
}
```

**Error Handling:**
- Retries on transient errors (network, timeout)
- Returns error on authentication failures
- Handles malformed JSON responses

#### ClaudeJudge

**LLM Integration:**
- Uses Anthropic API
- Model: `claude-3-7-sonnet-20250219` (configurable)
- Chain-of-Thought via assistant prefill

**Prompting Strategy:**
```
System: <evaluation_instructions>
User: <input_and_criteria>
Assistant: Let me analyze this step by step:
```

**Response Parsing:**
- Extracts JSON from freeform text
- Multiple extraction strategies (```json, {}, etc.)
- Robust fallback parsing

**Error Handling:**
- Same retry logic as GPT4Judge
- Handles non-text content blocks
- Validates extracted JSON structure

#### Metrics

**CorrectnessMetric:**
- Exact match mode: Binary 0.0 or 1.0
- Similarity mode: Jaccard similarity on character sets
- Case-sensitive option
- O(n) time complexity where n = length of strings

**SafetyMetric:**
- Keyword-based detection
- Case-insensitive matching
- Score = 1.0 - (violations / total_checks)
- Configurable keyword lists

**PerformanceMetric:**
- Latency evaluation: score = max / actual (if over threshold)
- Throughput evaluation: score = actual / min (if under threshold)
- Combined score: average of all configured metrics

### Integration Layer

#### LLM APIs

**HTTP Client Configuration:**
- Timeout: 30 seconds
- Retries: 3 attempts with exponential backoff
- Headers: API key, content-type, user-agent

**Request Structure:**

GPT-4:
```json
{
  "model": "gpt-4-turbo-preview",
  "messages": [...],
  "temperature": 0.1,
  "max_tokens": 1000,
  "response_format": {"type": "json_schema", "json_schema": {...}}
}
```

Claude:
```json
{
  "model": "claude-3-7-sonnet-20250219",
  "messages": [...],
  "system": "...",
  "temperature": 0.1,
  "max_tokens": 1000
}
```

#### Alerters

**Interface:**
```go
type Alerter interface {
    Alert(message string) error
}
```

**Implementations:**

**LogAlerter:**
- Logs to standard logger
- Never fails (returns nil)
- Format: `[ALERT] message`

**WebhookAlerter:**
- HTTP POST to configured URL
- Payload: `{"message": "..."}`
- Timeout: 5 seconds
- Returns error on failure (doesn't retry)

**EmailAlerter:**
- SMTP or API-based email (not fully implemented)
- Multiple recipients supported
- Configurable subject line
- Returns error on failure

## Data Flow Diagrams

### Offline Evaluation Flow

```
┌────────┐
│  User  │
└───┬────┘
    │
    │ testCases
    ↓
┌────────────────────┐
│ OfflineEvaluator   │
└─────┬──────────────┘
      │
      │ For each test case (parallel):
      │
      ├──→ ┌─────────────┐
      │    │    Judge    │──→ LLM API
      │    └─────────────┘       │
      │           ↓               │
      │    ┌─────────────┐       │
      │    │JudgeResponse│ ←─────┘
      │    └─────────────┘
      │           │
      ├───────────┘
      │
      │ Aggregate results
      ↓
┌────────────────────┐
│  OfflineReport     │
│  - PassRate        │
│  - AverageScore    │
│  - ShouldBlock     │
│  - Results[]       │
└────────────────────┘
      │
      │ If ShouldBlock
      ↓
┌────────────────────┐
│  Block Deployment  │
└────────────────────┘
```

### Online Monitoring Flow

```
┌────────────────┐
│Production Event│
└───────┬────────┘
        │
        ↓
┌──────────────────────┐
│  OnlineEvaluator     │
│  - Sample decision   │
└──────────┬───────────┘
           │
     ┌─────┴─────┐
     │ Sample?   │
     └─────┬─────┘
      Yes  │  No
           │   └──→ (skip)
           │
           ↓
    ┌──────────────┐
    │Update Metrics│
    │ - Success    │
    │ - Latency    │
    │ - Feedback   │
    └──────┬───────┘
           │
           ↓
    ┌──────────────┐
    │Check         │
    │Thresholds    │
    └──────┬───────┘
           │
     ┌─────┴─────┐
     │Violation? │
     └─────┬─────┘
      Yes  │  No
           │   └──→ (continue)
           │
           ↓
    ┌──────────────┐
    │Trigger Alerts│
    │ - Log        │
    │ - Webhook    │
    │ - Email      │
    └──────────────┘
```

### Feedback Loop Flow

```
┌────────────────────┐
│Production Examples │
│ (validated)        │
└─────────┬──────────┘
          │
          ↓
   ┌──────────────┐
   │FeedbackLoop  │
   └──────┬───────┘
          │
          │ For each example:
          │
          ├──→ Validate
          │      - Score >= threshold
          │      - Required fields present
          │
          ├──→ Check duplicates
          │      - Load existing examples
          │      - Compare inputs
          │
          ├──→ Write file
          │      - {timestamp}_{name}.json
          │      - Atomic write
          │
          └──→ Create PR
                 - Batch all new files
                 - Request human review
```

## Integration Points

### CI/CD Integration

**GitHub Actions Example:**
```yaml
- name: Run EDD Evaluation
  run: |
    go run ./cmd/evaluate \
      --golden-dir ./testdata/golden \
      --min-pass-rate 0.95 \
      --min-avg-score 0.85 \
      --block-on-failure
```

**Exit Code:**
- 0: All tests passed, deployment approved
- 1: Tests failed, deployment blocked
- 2: Evaluation error (API failure, etc.)

### Production Integration

**Session Manager Hook:**
```go
// After session completes
event := evaluation.SessionEvent{
    SessionID:    session.ID,
    Input:        session.Input,
    Actual:       session.Output,
    LatencyMs:    session.Duration.Milliseconds(),
    Success:      session.Success,
    Timestamp:    time.Now(),
}

onlineEvaluator.ProcessEvent(event)
```

### Alert Channel Integration

**Slack Webhook:**
```go
webhookAlerter := evaluation.NewWebhookAlerter(
    "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
)

evaluator := evaluation.NewOnlineEvaluator(
    judge,
    []evaluation.Alerter{webhookAlerter},
    config,
)
```

## Design Decisions

### Why Dual Interfaces (Judge vs DetailedJudge)?

**Problem:**
- Existing code uses simple `Judge` interface
- New features need richer responses (pass/fail, reasoning)
- Don't want to break backward compatibility

**Solution:**
- Keep `Judge` interface for backward compatibility
- Add `DetailedJudge` interface for new features
- Implement both in GPT4Judge and ClaudeJudge

**Tradeoffs:**
- ✅ Backward compatible
- ✅ Smooth migration path
- ❌ More complex API surface
- ❌ Two methods to maintain

See [ADR-001](./ADR/ADR-001-dual-judge-interfaces.md) for details.

### Why Multiple LLM Judges (GPT-4 and Claude)?

**Problem:**
- Different models have different strengths
- Vendor lock-in risk
- Cost optimization opportunities

**Solution:**
- Support multiple judge implementations
- Shared interface allows easy switching
- Users choose based on their needs

**Tradeoffs:**
- ✅ Flexibility and choice
- ✅ Vendor independence
- ✅ Ensemble evaluation possible
- ❌ Higher implementation cost
- ❌ Inconsistent eval quality across judges

See [ADR-002](./ADR/ADR-002-multiple-llm-judges.md) for details.

### Why Threshold-Based Deployment Blocking?

**Problem:**
- Need automatic regression detection
- Manual review doesn't scale
- Must prevent quality degradation

**Solution:**
- Configurable thresholds (pass rate, avg score)
- Automatic blocking in CI/CD
- Clear pass/fail criteria

**Tradeoffs:**
- ✅ Automated quality gates
- ✅ Prevents regressions
- ✅ Clear criteria
- ❌ Requires threshold tuning
- ❌ False positives possible

See [ADR-003](./ADR/ADR-003-threshold-based-deployment-blocking.md) for details.

### Why Pluggable Alerter Interface?

**Problem:**
- Different teams use different alert systems
- Slack, PagerDuty, email, custom systems
- Hard-coding limits flexibility

**Solution:**
- Alerter interface for extensibility
- Multiple built-in implementations
- Users can add custom alerters

**Tradeoffs:**
- ✅ Flexible and extensible
- ✅ Team-specific integrations
- ✅ Multiple channels simultaneously
- ❌ Requires interface implementation
- ❌ More complex configuration

See [ADR-004](./ADR/ADR-004-pluggable-alert-channels.md) for details.

## Performance Characteristics

### Time Complexity

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Evaluate single test case | O(API) | Dominated by LLM API latency |
| Evaluate N test cases | O(N/C * API) | C = concurrency (default 10) |
| Update metrics | O(1) | Constant time operations |
| Check thresholds | O(M) | M = number of metrics |
| Calculate percentile | O(N log N) | Sorting latencies |
| Deduplicate examples | O(N * M) | N = new, M = existing |

### Space Complexity

| Component | Complexity | Notes |
|-----------|-----------|-------|
| MetricsCollector | O(E) | E = events in window |
| OfflineReport | O(T) | T = test cases |
| Golden dataset | O(E) | E = total examples |
| Judge response cache | O(1) | Not implemented yet |

### Scalability Limits

**Current Limits:**
- Online monitoring: 10,000 events in memory
- Concurrent evaluations: 10 goroutines
- Golden dataset: 10,000 examples tested

**Future Improvements:**
- Sliding window metrics (bounded memory)
- Configurable concurrency
- Database-backed golden dataset

## Security Architecture

### Threat Model

**Threats:**
- API key exposure
- PII leakage in golden dataset
- Malicious input injection
- Denial of service via excessive API calls

**Mitigations:**
- API keys in environment variables only
- PII detection in validators
- Input sanitization (filename, content)
- Rate limiting (via sampling)

### Defense in Depth

**Layer 1: Input Validation**
- Validate all user inputs
- Sanitize filenames
- Check data types and ranges

**Layer 2: PII Detection**
- Built-in regex patterns
- Configurable blocklist
- Fail-safe (reject if PII detected)

**Layer 3: API Security**
- HTTPS only
- API key rotation support
- Never log sensitive data

**Layer 4: Output Validation**
- Validate LLM responses
- Handle malformed JSON
- Sanitize error messages

## Testing Strategy

### Unit Tests

- Mock all external dependencies (LLM APIs, file I/O, alerters)
- Test each component in isolation
- Focus on edge cases and error handling
- Target: 80%+ coverage

**Example:**
```go
func TestGPT4Judge_EvaluateDetailed(t *testing.T) {
    // Mock HTTP client
    client := &MockHTTPClient{
        Response: `{"pass": true, "score": 0.95, "reasoning": "..."}`,
    }

    judge := &GPT4Judge{client: client}
    // Test evaluation logic without real API calls
}
```

### Integration Tests

- Test interactions between components
- Use real file I/O but mock LLM APIs
- Verify data flows correctly

### End-to-End Tests

- Not currently implemented
- Would test full flow with real APIs
- Run in CI with test API keys

## Future Enhancements

### Planned Architectural Changes

1. **Response Caching**
   - Cache judge responses by input hash
   - Reduces API costs and latency
   - Invalidation strategy needed

2. **Async Processing**
   - Queue-based evaluation for offline mode
   - Better scaling for large test suites
   - Progress reporting

3. **Distributed Evaluation**
   - Split test cases across workers
   - Aggregate results from multiple instances
   - Kubernetes-ready design

4. **Metric Storage**
   - Persist metrics to time-series database
   - Historical analysis and trending
   - Long-term storage without memory limits

5. **Advanced Sampling**
   - Stratified sampling by input characteristics
   - Adaptive sampling based on confidence
   - Cost-optimized evaluation
