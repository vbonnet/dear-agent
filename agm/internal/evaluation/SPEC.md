# EDD Framework Specification

## Purpose

The EDD (Evaluation-Driven Development) Framework provides a systematic approach to evaluating LLM-based applications throughout their lifecycle. It addresses the unique challenges of AI system quality assurance by providing:

1. **Pre-deployment Quality Gates**: Prevent regressions before code reaches production
2. **Production Monitoring**: Detect issues in real-time with minimal overhead
3. **Automated Feedback Loops**: Continuously improve quality through golden dataset updates
4. **Multi-dimensional Evaluation**: Assess correctness, safety, and performance

## Requirements

### Functional Requirements

#### FR-1: Offline Evaluation
The system MUST support batch evaluation of test cases against golden datasets before deployment.

**Acceptance Criteria:**
- Load test cases from structured format
- Evaluate each test case using configured judge
- Calculate aggregate metrics (pass rate, average score)
- Generate detailed reports with per-case results
- Support threshold-based deployment blocking

#### FR-2: Online Monitoring
The system MUST support production monitoring with configurable sampling rates.

**Acceptance Criteria:**
- Sample events at configurable rate (0.0-1.0)
- Collect real-time metrics (success rate, latency, satisfaction)
- Check metrics against configurable thresholds
- Trigger alerts when thresholds violated
- Support multiple concurrent alerter channels

#### FR-3: LLM-as-Judge Evaluation
The system MUST support using LLMs to evaluate output quality.

**Acceptance Criteria:**
- Support multiple LLM providers (GPT-4, Claude)
- Provide structured evaluation criteria
- Return normalized scores (0.0-1.0)
- Include reasoning/explanation in results
- Handle API errors gracefully

#### FR-4: Multiple Evaluation Metrics
The system MUST support multiple types of evaluation metrics.

**Acceptance Criteria:**
- Correctness metric (exact match or similarity)
- Safety metric (harmful content detection)
- Performance metric (latency and throughput)
- Extensible architecture for custom metrics
- Combined multi-metric evaluation

#### FR-5: Feedback Loop
The system MUST support automated golden dataset updates from production data.

**Acceptance Criteria:**
- Validate examples meet quality criteria
- Deduplicate similar examples
- Create pull requests for human review
- Store examples in structured format
- Track example count and metadata

#### FR-6: Flexible Alerting
The system MUST support pluggable alerting mechanisms.

**Acceptance Criteria:**
- Alerter interface for extensibility
- Built-in log, webhook, and email alerters
- Support multiple concurrent alerters
- Graceful handling of alerter failures
- Configurable alert content

### Non-Functional Requirements

#### NFR-1: Latency
- **Offline Evaluation**: Complete evaluation of 100 test cases in < 5 minutes (with API calls)
- **Online Monitoring**: Process event in < 10ms (excluding judge evaluation)
- **Metric Calculation**: Calculate aggregate metrics in < 100ms

#### NFR-2: Throughput
- **Online Monitoring**: Support 1000+ events/second (at 10% sampling = 100 evaluations/second)
- **Batch Evaluation**: Process 10+ test cases concurrently

#### NFR-3: Reliability
- **API Resilience**: Handle LLM API errors without crashing (return error, allow retry)
- **Graceful Degradation**: Continue functioning if alerters fail
- **Data Persistence**: Ensure golden dataset updates are atomic

#### NFR-4: Scalability
- **Memory Usage**: Support 10,000+ events in online monitoring window
- **Concurrent Requests**: Handle 100+ concurrent offline evaluations
- **Golden Dataset Size**: Support 10,000+ examples without performance degradation

#### NFR-5: Maintainability
- **Code Coverage**: Minimum 80% test coverage
- **Documentation**: All public APIs documented with examples
- **Backward Compatibility**: Maintain compatibility for simple Judge interface

## Use Cases

### UC-1: Pre-Deployment Validation

**Actor**: CI/CD Pipeline

**Preconditions:**
- Golden dataset exists with test cases
- API keys configured for judge
- Thresholds configured

**Main Flow:**
1. CI pipeline triggers on pull request
2. System loads golden dataset
3. System evaluates all test cases using judge
4. System calculates aggregate metrics
5. System compares metrics to thresholds
6. System blocks deployment if thresholds not met
7. System generates HTML/JSON report

**Postconditions:**
- Deployment proceeds only if metrics meet thresholds
- Report available for review

**Alternative Flows:**
- **3a**: Judge API fails -> Return error, retry up to 3 times
- **6a**: Metrics below threshold -> Block deployment, provide reason
- **7a**: No test cases found -> Error

### UC-2: Production Monitoring

**Actor**: Online Evaluator Service

**Preconditions:**
- Online evaluator running
- Sampling rate configured (e.g., 10%)
- Alerters configured
- Metric thresholds configured

**Main Flow:**
1. Session event occurs in production
2. System samples event based on configured rate
3. System processes event (if sampled)
4. System updates metrics (success rate, latency, satisfaction)
5. System checks metrics against thresholds
6. System triggers alerts if violations detected
7. System continues monitoring

**Postconditions:**
- Metrics updated in real-time
- Alerts sent if thresholds violated

**Alternative Flows:**
- **2a**: Event not sampled -> Skip processing, return immediately
- **3a**: Judge evaluation fails -> Log error, continue monitoring
- **6a**: Alerter fails -> Log error, try remaining alerters

### UC-3: Automated Feedback Loop

**Actor**: Feedback Loop Service

**Preconditions:**
- Production data available
- Golden dataset directory configured
- PR creation mechanism configured
- Validation criteria defined

**Main Flow:**
1. System receives validated production examples
2. System validates each example (score, format, etc.)
3. System checks for duplicates in existing golden dataset
4. System adds new unique examples to dataset files
5. System creates pull request with changes
6. Human reviewer approves/rejects PR
7. Changes merged to golden dataset

**Postconditions:**
- New examples added to golden dataset
- PR created for human review

**Alternative Flows:**
- **2a**: Example fails validation -> Skip example, continue with others
- **3a**: Duplicate found -> Skip example
- **5a**: PR creation fails -> Log error, return error

## API Specification

### Judge Interface

```go
// Simple interface (legacy)
type Judge interface {
    Evaluate(ctx context.Context, prompt string, response string) (float64, error)
}
```

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `prompt`: Input prompt/question
- `response`: LLM response to evaluate

**Returns:**
- `float64`: Normalized score (0.0-1.0)
- `error`: Error if evaluation fails

### DetailedJudge Interface

```go
type DetailedJudge interface {
    EvaluateDetailed(ctx context.Context, input, expectedOutput string,
                     criteria EvaluationCriteria) (*JudgeResponse, error)
}
```

**Parameters:**
- `ctx`: Context for cancellation and timeout
- `input`: Input to the system being evaluated
- `expectedOutput`: Expected/reference output
- `criteria`: Evaluation criteria with threshold

**Returns:**
- `*JudgeResponse`: Detailed response with pass/fail, score, reasoning
- `error`: Error if evaluation fails

### Evaluator Interfaces

#### OfflineEvaluator

```go
func (e *OfflineEvaluator) EvaluateOffline(ctx context.Context,
    testCases []TestCase) (*OfflineReport, error)
```

**Parameters:**
- `ctx`: Context for cancellation
- `testCases`: List of test cases to evaluate

**Returns:**
- `*OfflineReport`: Report with results, metrics, and block decision
- `error`: Error if evaluation fails

#### OnlineEvaluator

```go
func (e *OnlineEvaluator) MonitorProduction(ctx context.Context,
    samplingRate float64) error

func (e *OnlineEvaluator) ProcessEvent(event SessionEvent)

func (e *OnlineEvaluator) GetMetrics() ProductionMetrics

func (e *OnlineEvaluator) Stop()
```

**MonitorProduction Parameters:**
- `ctx`: Context for cancellation
- `samplingRate`: Sampling rate (0.0-1.0)

**ProcessEvent Parameters:**
- `event`: Session event to process

**GetMetrics Returns:**
- `ProductionMetrics`: Current metric values

## Data Models

### JudgeResponse

```go
type JudgeResponse struct {
    Pass      bool    // Whether output meets criteria
    Score     float64 // Normalized score 0.0-1.0
    Reasoning string  // Explanation of judgment
}
```

**Validation:**
- `Score` must be between 0.0 and 1.0
- `Reasoning` should not be empty for failed evaluations

### EvaluationCriteria

```go
type EvaluationCriteria struct {
    Name        string  // Name of criteria
    Description string  // Detailed description
    Threshold   float64 // Minimum score to pass (0.0-1.0)
}
```

**Validation:**
- `Name` must not be empty
- `Threshold` must be between 0.0 and 1.0
- `Description` should provide clear evaluation guidance

### OfflineReport

```go
type OfflineReport struct {
    TotalCases    int
    PassedCases   int
    FailedCases   int
    PassRate      float64
    AverageScore  float64
    ShouldBlock   bool
    BlockReason   string
    Results       []TestCaseResult
    GeneratedAt   time.Time
}
```

**Invariants:**
- `PassedCases + FailedCases == TotalCases`
- `PassRate == float64(PassedCases) / float64(TotalCases)`
- If `ShouldBlock == true`, `BlockReason` must not be empty

### SessionEvent

```go
type SessionEvent struct {
    SessionID      string
    Input          string
    ExpectedOutput string
    Actual         string
    LatencyMs      int64
    Success        bool
    Score          float64
    UserFeedback   string
    Timestamp      time.Time
}
```

**Validation:**
- `SessionID` must not be empty
- `LatencyMs` must be >= 0
- `Score` must be between 0.0 and 1.0 (if set)

### ProductionMetrics

```go
type ProductionMetrics struct {
    SuccessRate   float64 // Success rate (0.0-1.0)
    P95LatencyMs  float64 // 95th percentile latency
    P99LatencyMs  float64 // 99th percentile latency
    Satisfaction  float64 // User satisfaction (0.0-1.0)
    TotalEvents   int
    CollectedAt   time.Time
}
```

**Invariants:**
- All rate/satisfaction values between 0.0 and 1.0
- Latency values >= 0
- `P99LatencyMs >= P95LatencyMs`

## Error Handling

### Error Categories

#### Transient Errors
- **LLM API Timeout**: Retry up to 3 times with exponential backoff
- **Network Error**: Retry up to 3 times
- **Rate Limit**: Retry with backoff

#### Permanent Errors
- **Invalid API Key**: Return error immediately, do not retry
- **Invalid Request Format**: Return error immediately
- **Context Canceled**: Return error immediately

#### Degradation Errors
- **Alerter Failure**: Log error, continue with remaining alerters
- **Metric Collection Failure**: Log error, use default/cached values
- **Sample Evaluation Failure**: Log error, continue monitoring

### Error Responses

All errors should:
1. Include context (which operation failed)
2. Include root cause
3. Be wrapped using `fmt.Errorf("context: %w", err)`
4. Be logged before returning

Example:
```go
if err != nil {
    return fmt.Errorf("failed to evaluate test case %s: %w", tc.Name, err)
}
```

## Retry Logic

### LLM API Calls

```
MaxRetries: 3
BackoffStrategy: Exponential
BaseDelay: 1 second
MaxDelay: 30 seconds
Multiplier: 2
```

Retry sequence: 1s → 2s → 4s

### Transient Failures Only

Do NOT retry on:
- Authentication errors
- Invalid request format
- Context cancellation

## Performance Budgets

### Offline Evaluation

| Operation | Budget | Measured |
|-----------|--------|----------|
| Load golden dataset (100 cases) | 100ms | < 50ms |
| Evaluate 1 test case | 2s | 500ms-2s (API dependent) |
| Evaluate 100 test cases (concurrent) | 5 min | 2-5 min (API dependent) |
| Generate report | 100ms | < 50ms |

### Online Monitoring

| Operation | Budget | Measured |
|-----------|--------|----------|
| Sample decision | 1ms | < 1ms |
| Update metrics | 10ms | < 5ms |
| Check thresholds | 10ms | < 1ms |
| Trigger alert | 100ms | < 50ms |
| Judge evaluation (async) | 2s | 500ms-2s (API dependent) |

## Security Considerations

### API Key Management

- API keys stored in environment variables only
- Never log API keys
- Never include API keys in reports
- Support key rotation without downtime

### PII Detection

- Built-in PII detection in validators
- Check for: emails, credit cards, SSNs, phone numbers, API keys
- Configurable blocklist for sensitive terms
- Fail validation if PII detected

### Golden Dataset Security

- Store in version control (no sensitive data)
- Sanitize production data before adding to golden dataset
- PR review required for all changes
- Audit trail of all additions/modifications

## Extensibility Points

### Custom Metrics

Implement `Metric` interface:

```go
type CustomMetric struct {}

func (m *CustomMetric) Name() string {
    return "custom"
}

func (m *CustomMetric) Evaluate(input, expected, actual string) float64 {
    // Custom logic
    return score
}
```

### Custom Judges

Implement `Judge` and `DetailedJudge` interfaces:

```go
type CustomJudge struct {}

func (j *CustomJudge) Evaluate(ctx context.Context,
    prompt, response string) (float64, error) {
    // Custom logic
    return score, nil
}

func (j *CustomJudge) EvaluateDetailed(ctx context.Context,
    input, expected string, criteria EvaluationCriteria) (*JudgeResponse, error) {
    // Custom logic
    return &JudgeResponse{}, nil
}
```

### Custom Alerters

Implement `Alerter` interface:

```go
type CustomAlerter struct {}

func (a *CustomAlerter) Alert(message string) error {
    // Custom alerting logic
    return nil
}
```

## Future Enhancements

### Planned Features (Not Yet Implemented)

1. **Comparison Judges**: Compare outputs from multiple models
2. **Multi-turn Evaluation**: Evaluate conversation flows
3. **Cost Tracking**: Track evaluation costs per judge
4. **Advanced Sampling**: Stratified sampling based on input characteristics
5. **Real-time Dashboards**: Web UI for metrics visualization
6. **Automated Threshold Tuning**: ML-based threshold optimization
7. **A/B Test Support**: Compare evaluation metrics between model versions

### Backward Compatibility Commitment

- Simple `Judge` interface will remain supported
- Existing golden dataset formats will remain supported
- Breaking changes will require major version bump
