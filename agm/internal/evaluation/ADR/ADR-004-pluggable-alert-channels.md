# ADR-004: Use Pluggable Alerter Interface

## Status

Accepted

## Context

The online evaluation system monitors production systems and needs to alert teams when metric violations occur (low success rate, high latency, low user satisfaction).

### The Problem

Different teams use different alerting systems:
- **Team A**: Slack webhooks for real-time notifications
- **Team B**: PagerDuty for on-call escalation
- **Team C**: Email for daily summaries
- **Team D**: Custom internal alerting system
- **Team E**: Multiple channels (Slack + PagerDuty + email)

Hard-coding a specific alerting mechanism would:
1. Limit adoption to teams using that specific system
2. Require framework changes for each new alert channel
3. Prevent teams from using multiple channels simultaneously
4. Make testing harder (tight coupling to external services)

### Requirements

1. **Flexibility**: Support any alerting mechanism
2. **Extensibility**: Easy to add new alert channels without modifying framework
3. **Composability**: Support multiple alert channels simultaneously
4. **Testability**: Easy to mock for testing
5. **Reliability**: Graceful handling when alerter fails
6. **Simplicity**: Simple interface, easy to implement

## Decision

We will use a **pluggable Alerter interface** with built-in implementations for common channels:

### Core Interface

```go
type Alerter interface {
    Alert(message string) error
}
```

**Design Rationale:**
- **Simple**: Single method, clear purpose
- **Flexible**: Implementers control how alerts are sent
- **Testable**: Easy to create mock implementations
- **Error Handling**: Returns error but doesn't crash system

### Built-in Implementations

**LogAlerter** (always available, no dependencies):
```go
type LogAlerter struct{}

func (a *LogAlerter) Alert(message string) error {
    log.Printf("[ALERT] %s", message)
    return nil // Never fails
}
```

**WebhookAlerter** (HTTP POST to webhook URL):
```go
type WebhookAlerter struct {
    WebhookURL string
    Timeout    time.Duration
}

func (a *WebhookAlerter) Alert(message string) error {
    // HTTP POST to webhook
    // Returns error on failure
}
```

**EmailAlerter** (send email via SMTP or API):
```go
type EmailAlerter struct {
    Recipients []string
    Subject    string
}

func (a *EmailAlerter) Alert(message string) error {
    // Send email to recipients
    // Returns error on failure
}
```

### Multi-Alerter Support

The `OnlineEvaluator` accepts multiple alerters:

```go
type OnlineEvaluator struct {
    alerters []Alerter
    // ...
}

func NewOnlineEvaluator(judge DetailedJudge, alerters []Alerter,
                        config *OnlineConfig) *OnlineEvaluator {
    return &OnlineEvaluator{
        alerters: alerters,
        // ...
    }
}
```

### Graceful Failure Handling

If one alerter fails, others still execute:

```go
func (e *OnlineEvaluator) sendAlerts(message string) {
    for _, alerter := range e.alerters {
        if err := alerter.Alert(message); err != nil {
            log.Printf("Alert failed: %v", err)
            // Continue with next alerter
        }
    }
}
```

## Consequences

### Positive

1. **Team Flexibility**: Each team can use their preferred alerting system
2. **Zero Framework Changes**: Add new channels without modifying EDD Framework
3. **Multiple Channels**: Support simultaneous alerts to different systems
4. **Easy Testing**: Simple to create mock alerters for tests
5. **Graceful Degradation**: System continues if alerter fails
6. **Simple Interface**: One method, easy to understand and implement

### Negative

1. **Configuration Complexity**: Users must configure each alerter
2. **Error Handling**: Must handle errors from alerter implementations
3. **No Built-in Retry**: Alerter implementations must handle retries
4. **Limited Built-ins**: Only 3 built-in implementations (log, webhook, email)
5. **Testing Burden**: Each alerter implementation needs its own tests

### Neutral

1. **Interface Evolution**: Future enhancements must maintain interface compatibility
2. **Documentation**: Must document each built-in alerter and how to implement custom ones

## Implementation Examples

### Example 1: Simple Logging (Development)

```go
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(),
}

evaluator := evaluation.NewOnlineEvaluator(judge, alerters, config)
```

### Example 2: Slack Webhook (Production)

```go
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(), // Always log
    evaluation.NewWebhookAlerter("https://hooks.slack.com/services/YOUR/WEBHOOK"),
}

evaluator := evaluation.NewOnlineEvaluator(judge, alerters, config)
```

### Example 3: Multi-Channel (Critical System)

```go
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(),
    evaluation.NewWebhookAlerter("https://hooks.slack.com/..."),
    evaluation.NewEmailAlerter(
        []string{"team@example.com", "oncall@example.com"},
        "Production LLM Alert",
    ),
}

evaluator := evaluation.NewOnlineEvaluator(judge, alerters, config)
```

### Example 4: Custom Alerter (PagerDuty)

```go
type PagerDutyAlerter struct {
    IntegrationKey string
    Severity       string
}

func (a *PagerDutyAlerter) Alert(message string) error {
    // Call PagerDuty Events API
    payload := map[string]interface{}{
        "routing_key":  a.IntegrationKey,
        "event_action": "trigger",
        "payload": map[string]interface{}{
            "summary":  message,
            "severity": a.Severity,
            "source":   "edd-framework",
        },
    }

    resp, err := http.Post("https://events.pagerduty.com/v2/enqueue", ...)
    if err != nil {
        return fmt.Errorf("PagerDuty alert failed: %w", err)
    }

    if resp.StatusCode != 202 {
        return fmt.Errorf("PagerDuty returned %d", resp.StatusCode)
    }

    return nil
}

// Usage
alerters := []evaluation.Alerter{
    &PagerDutyAlerter{
        IntegrationKey: "your-integration-key",
        Severity:       "error",
    },
}
```

### Example 5: Mock Alerter (Testing)

```go
type MockAlerter struct {
    Messages []string
    Err      error
}

func (a *MockAlerter) Alert(message string) error {
    a.Messages = append(a.Messages, message)
    return a.Err
}

// In tests
func TestAlertingBehavior(t *testing.T) {
    mock := &MockAlerter{}
    evaluator := evaluation.NewOnlineEvaluator(judge, []evaluation.Alerter{mock}, config)

    // Trigger alert...

    assert.Equal(t, 1, len(mock.Messages))
    assert.Contains(t, mock.Messages[0], "Success rate below threshold")
}
```

## Built-in Alerter Details

### LogAlerter

**Purpose**: Development, debugging, baseline alerting

**Configuration:** None

**Behavior:**
- Logs to standard logger with `[ALERT]` prefix
- Never fails (returns nil error)
- Suitable for all environments

**Pros:**
- Zero configuration
- Always available
- Never fails

**Cons:**
- Not actionable without log aggregation
- Easy to miss in production logs

### WebhookAlerter

**Purpose**: Slack, Discord, custom webhooks

**Configuration:**
```go
type WebhookAlerter struct {
    WebhookURL string        // Required
    Timeout    time.Duration // Default: 5s
}
```

**Behavior:**
- HTTP POST to webhook URL
- JSON payload: `{"message": "alert text"}`
- 5-second timeout
- Returns error on failure (doesn't retry)

**Pros:**
- Works with most chat platforms
- Simple HTTP integration
- Fast notification

**Cons:**
- No built-in retry (caller must implement)
- Requires public webhook URL
- No delivery guarantee

**Example (Slack):**
```go
// Slack expects {"text": "message"} format
// WebhookAlerter sends {"message": "..."}, so you may need a wrapper:

type SlackAlerter struct {
    WebhookURL string
}

func (a *SlackAlerter) Alert(message string) error {
    payload := map[string]string{"text": message}
    body, _ := json.Marshal(payload)

    resp, err := http.Post(a.WebhookURL, "application/json", bytes.NewBuffer(body))
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return fmt.Errorf("Slack returned %d", resp.StatusCode)
    }

    return nil
}
```

### EmailAlerter

**Purpose**: Email summaries, alerts to on-call

**Configuration:**
```go
type EmailAlerter struct {
    Recipients []string // Required
    Subject    string   // Optional, default: "EDD Alert"
}
```

**Behavior:**
- Currently logs intent to send email (not fully implemented)
- Future: Will send via SMTP or email API

**Status:** Placeholder implementation

**Future Implementation:**
```go
func (a *EmailAlerter) Alert(message string) error {
    // Use SMTP or SendGrid/AWS SES
    return sendEmail(a.Recipients, a.Subject, message)
}
```

## Testing Strategy

### Unit Tests for Alerters

Each alerter implementation should have tests:

```go
func TestLogAlerter(t *testing.T) {
    alerter := evaluation.NewLogAlerter()
    err := alerter.Alert("test message")
    assert.NoError(t, err)
}

func TestWebhookAlerter(t *testing.T) {
    // Use httptest for mocking webhook server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(200)
    }))
    defer server.Close()

    alerter := evaluation.NewWebhookAlerter(server.URL)
    err := alerter.Alert("test message")
    assert.NoError(t, err)
}
```

### Integration Tests

Test multi-alerter behavior:

```go
func TestMultipleAlerters(t *testing.T) {
    mock1 := &MockAlerter{}
    mock2 := &MockAlerter{Err: errors.New("fail")}
    mock3 := &MockAlerter{}

    alerters := []evaluation.Alerter{mock1, mock2, mock3}
    evaluator := evaluation.NewOnlineEvaluator(judge, alerters, config)

    // Trigger alert
    evaluator.sendAlerts("test")

    // All should receive alert, even though mock2 failed
    assert.Equal(t, 1, len(mock1.Messages))
    assert.Equal(t, 1, len(mock2.Messages))
    assert.Equal(t, 1, len(mock3.Messages))
}
```

## Configuration Best Practices

### Development Environment

```go
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(), // Just log
}
```

### Staging Environment

```go
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(),
    evaluation.NewWebhookAlerter("https://hooks.slack.com/.../staging-alerts"),
}
```

### Production Environment

```go
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(), // Always log for audit
    evaluation.NewWebhookAlerter("https://hooks.slack.com/.../prod-alerts"),
    evaluation.NewEmailAlerter([]string{"oncall@example.com"}, "PROD Alert"),
    &PagerDutyAlerter{...}, // Custom implementation
}
```

## Alternatives Considered

### Alternative 1: Hard-coded Slack Integration

Directly integrate with Slack API.

**Rejected Reason:**
- Not flexible for teams using other systems
- Tight coupling to Slack
- Framework changes needed for other channels

### Alternative 2: Callback Function

Pass a callback function instead of interface:

```go
type AlertFunc func(message string) error

func NewOnlineEvaluator(judge DetailedJudge, alertFunc AlertFunc, ...) *OnlineEvaluator
```

**Rejected Reason:**
- Less Go-idiomatic (interfaces preferred)
- Harder to compose multiple alerters
- No clear type for implementations

### Alternative 3: Channel-Based Alerting

Use Go channels for alerts:

```go
func NewOnlineEvaluator(judge DetailedJudge, alertChan chan<- string, ...) *OnlineEvaluator
```

**Rejected Reason:**
- Unclear who consumes channel
- Synchronization complexity
- Error handling unclear

### Alternative 4: Event Bus

Use publish-subscribe event bus.

**Rejected Reason:**
- Over-engineering for current needs
- Adds external dependency
- More complex to understand

## Future Enhancements

### Planned Features

1. **Rich Alert Context**
   - Include metric values, thresholds, timestamp
   - Structured alert payload (not just string)

2. **Alert Severity Levels**
   - Warning, Error, Critical
   - Alerters can filter by severity

3. **Alert Throttling**
   - Prevent alert storms
   - Rate limit alerts per channel

4. **Built-in Retry Logic**
   - Retry transient failures automatically
   - Exponential backoff

5. **Alert Templates**
   - Customizable message templates
   - Interpolate metric values

### Enhanced Interface (Future)

```go
type Alert struct {
    Message   string
    Severity  string
    Metrics   map[string]float64
    Timestamp time.Time
}

type Alerter interface {
    Alert(alert Alert) error
    ShouldAlert(severity string) bool
}
```

## References

- Implementation: `online_evaluator.go`, `online_metrics.go`
- Tests: `online_evaluator_test.go`, `online_metrics_test.go`
- Similar patterns: Observer pattern, Plugin architecture

## Revision History

- 2024-09-15: Initial decision
- 2024-10-20: Added built-in implementations
- 2025-01-05: Added examples and configuration best practices
- 2026-02-20: Added to EDD Framework documentation
