# EDD Framework - Evaluation-Driven Development

## Overview

The EDD (Evaluation-Driven Development) Framework is a comprehensive system for evaluating LLM-based applications in both offline (pre-deployment) and online (production) environments. It provides a robust foundation for ensuring quality, safety, and performance of AI systems through systematic evaluation.

### Key Features

- **Dual Judge Interfaces**: Support for both simple and detailed evaluation workflows
- **Multiple LLM Judges**: GPT-4 and Claude integration for diverse evaluation perspectives
- **Comprehensive Metrics**: Correctness, safety, and performance evaluation
- **Offline Evaluation**: Pre-deployment quality gates with threshold-based blocking
- **Online Monitoring**: Production monitoring with real-time alerting
- **Feedback Loop**: Automated golden dataset updates via PR creation
- **Flexible Alerting**: Pluggable alerter interface (log, webhook, email)

## Table of Contents

- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Components](#components)
- [Usage Examples](#usage-examples)
- [Configuration](#configuration)
- [Testing](#testing)
- [Contributing](#contributing)

## Quick Start

### Installation

```bash
go get github.com/vbonnet/dear-agent/agm/internal/evaluation
```

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "github.com/vbonnet/dear-agent/agm/internal/evaluation"
)

func main() {
    // Create a judge (GPT-4 or Claude)
    judge := evaluation.NewGPT4Judge("your-openai-api-key", nil)

    // Define evaluation criteria
    criteria := evaluation.EvaluationCriteria{
        Name:        "correctness",
        Description: "The output should be accurate and complete",
        Threshold:   0.8,
    }

    // Evaluate a response
    ctx := context.Background()
    response, err := judge.EvaluateDetailed(
        ctx,
        "What is 2+2?",      // input
        "4",                  // expected output
        criteria,
    )

    if err != nil {
        panic(err)
    }

    fmt.Printf("Pass: %v, Score: %.2f\n", response.Pass, response.Score)
    fmt.Printf("Reasoning: %s\n", response.Reasoning)
}
```

## Architecture

The EDD Framework follows a layered architecture:

```
┌─────────────────────────────────────────────────────────┐
│                   Application Layer                      │
│            (Offline Evaluator, Online Monitor)           │
└─────────────────────────────────────────────────────────┘
                            │
┌─────────────────────────────────────────────────────────┐
│                    Evaluation Layer                      │
│        (Judges, Metrics, Criteria, Validators)           │
└─────────────────────────────────────────────────────────┘
                            │
┌─────────────────────────────────────────────────────────┐
│                   Integration Layer                      │
│           (LLM APIs, Alerters, PR Creation)              │
└─────────────────────────────────────────────────────────┘
```

## Components

### 1. Judge Interfaces

#### Simple Judge Interface (Legacy)

```go
type Judge interface {
    Evaluate(ctx context.Context, prompt string, response string) (float64, error)
}
```

Used for basic score-only evaluations. Returns a normalized score (0.0-1.0).

#### Detailed Judge Interface

```go
type DetailedJudge interface {
    EvaluateDetailed(ctx context.Context, input, expectedOutput string,
                     criteria EvaluationCriteria) (*JudgeResponse, error)
}
```

Returns detailed evaluation including:
- `Pass`: Boolean indicating if criteria met
- `Score`: Normalized score (0.0-1.0)
- `Reasoning`: Explanation of the judgment (includes Chain-of-Thought)

### 2. Judge Implementations

#### GPT4Judge

Uses OpenAI's GPT-4 for evaluation with structured output via response_format.

**Features:**
- JSON schema-based structured output
- Configurable model and temperature
- Chain-of-Thought reasoning
- Error handling and retry logic

**Example:**

```go
config := &evaluation.GPT4Config{
    Model:       "gpt-4-turbo-preview",
    Temperature: 0.1,
    MaxTokens:   1000,
}

judge := evaluation.NewGPT4Judge(apiKey, config)
```

#### ClaudeJudge

Uses Anthropic's Claude for evaluation with prefill-based CoT.

**Features:**
- Structured JSON extraction from freeform output
- Configurable model and temperature
- Built-in Chain-of-Thought via prefill
- Robust JSON parsing with fallbacks

**Example:**

```go
config := &evaluation.ClaudeConfig{
    Model:       "claude-3-7-sonnet-20250219",
    Temperature: 0.1,
    MaxTokens:   1000,
}

judge := evaluation.NewClaudeJudge(apiKey, config)
```

### 3. Metrics

#### CorrectnessMetric

Evaluates if output matches expected result.

**Configuration:**
- `CaseSensitive`: Whether to compare case-sensitively (default: false)
- `ExactMatch`: Require exact match vs similarity-based (default: false)

**Example:**

```go
metric := &evaluation.CorrectnessMetric{
    CaseSensitive: false,
    ExactMatch:    false,
}

score := metric.Evaluate(input, expected, actual)
```

#### SafetyMetric

Checks for harmful content using keyword matching.

**Configuration:**
- `HarmfulKeywords`: List of keywords indicating harmful content
- `Blocklist`: Additional blocked patterns

**Example:**

```go
metric := &evaluation.SafetyMetric{
    HarmfulKeywords: evaluation.DefaultSafetyKeywords(),
    Blocklist:       []string{"forbidden-term"},
}

score := metric.Evaluate(input, expected, actual)
```

#### PerformanceMetric

Checks latency and throughput against thresholds.

**Configuration:**
- `MaxLatencyMs`: Maximum acceptable latency in milliseconds
- `MinThroughput`: Minimum acceptable throughput (requests/second)
- `ActualLatency`: Measured latency
- `ActualRequests`: Actual requests processed
- `TimeWindowSec`: Time window for throughput calculation

**Example:**

```go
metric := &evaluation.PerformanceMetric{
    MaxLatencyMs:  100,
    MinThroughput: 10,
    ActualLatency: 75,
    ActualRequests: 150,
    TimeWindowSec: 10,
}

score := metric.Evaluate(input, expected, actual)
```

### 4. Offline Evaluator

Pre-deployment evaluation with golden dataset testing.

**Features:**
- Batch evaluation of test cases
- Deployment blocking on threshold violations
- Detailed reporting
- Configurable pass/fail criteria

**Example:**

```go
config := &evaluation.OfflineConfig{
    MinPassRate:     0.9,  // 90% must pass
    MinAvgScore:     0.85, // Average score >= 0.85
    BlockOnFailure:  true, // Block deployment on failure
}

evaluator := evaluation.NewOfflineEvaluator(judge, config)

testCases := []evaluation.TestCase{
    {
        Input:          "What is the capital of France?",
        ExpectedOutput: "Paris",
        Criteria: evaluation.EvaluationCriteria{
            Name:        "correctness",
            Description: "Answer should be accurate",
            Threshold:   0.8,
        },
    },
}

report, err := evaluator.EvaluateOffline(ctx, testCases)
if err != nil {
    panic(err)
}

fmt.Println(report.FormattedReport())

if report.ShouldBlock {
    log.Fatal("Deployment blocked due to evaluation failures")
}
```

### 5. Online Evaluator

Production monitoring with real-time metrics collection.

**Features:**
- Sampling-based evaluation (configurable rate)
- Real-time metric collection
- Threshold-based alerting
- Multiple alerter support

**Example:**

```go
// Create alerters
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(),
    evaluation.NewWebhookAlerter("https://alerts.example.com/webhook"),
}

// Create online evaluator
config := &evaluation.OnlineConfig{
    Thresholds: evaluation.DefaultThresholds(),
}

evaluator := evaluation.NewOnlineEvaluator(judge, alerters, config)

// Start monitoring with 10% sampling
err := evaluator.MonitorProduction(ctx, 0.1)
if err != nil {
    panic(err)
}

// Process session events
for event := range sessionEvents {
    evaluator.ProcessEvent(event)
}

// Stop monitoring
evaluator.Stop()
```

### 6. Feedback Loop

Automated golden dataset updates via PR creation.

**Features:**
- Validation of examples before adding
- Deduplication
- PR creation for human review
- File-based golden dataset management

**Example:**

```go
// Create PR creator (mock or real GitHub integration)
prCreator := &MockPRCreator{}

feedbackLoop := evaluation.NewFeedbackLoop(
    "/path/to/golden",
    prCreator,
)

// Add validated examples to golden dataset
examples := []evaluation.GoldenExample{
    {
        Input:          "Sample input",
        ExpectedOutput: "Sample output",
        Actual:         "Sample output",
        Score:          0.95,
        Valid:          true,
    },
}

err := feedbackLoop.UpdateGoldenDataset(ctx, examples)
if err != nil {
    panic(err)
}
```

### 7. Alerting System

Pluggable alerter interface for production monitoring.

#### LogAlerter

Logs alerts to standard logger.

```go
alerter := evaluation.NewLogAlerter()
err := alerter.Alert("Alert message")
```

#### WebhookAlerter

Sends alerts via HTTP webhook (not yet fully implemented).

```go
alerter := evaluation.NewWebhookAlerter("https://example.com/webhook")
err := alerter.Alert("Alert message")
```

#### EmailAlerter

Sends alerts via email (not yet fully implemented).

```go
alerter := evaluation.NewEmailAlerter([]string{"team@example.com"}, "EDD Alert")
err := alerter.Alert("Alert message")
```

## Usage Examples

### Example 1: Basic Offline Evaluation

```go
func main() {
    ctx := context.Background()

    // Setup judge
    judge := evaluation.NewGPT4Judge(os.Getenv("OPENAI_API_KEY"), nil)

    // Create evaluator with default config
    evaluator := evaluation.NewOfflineEvaluator(judge, evaluation.DefaultOfflineConfig())

    // Define test cases
    testCases := []evaluation.TestCase{
        {
            Input:          "Translate 'hello' to Spanish",
            ExpectedOutput: "hola",
            Criteria: evaluation.EvaluationCriteria{
                Name:        "correctness",
                Description: "Translation should be accurate",
                Threshold:   0.9,
            },
        },
    }

    // Run evaluation
    report, err := evaluator.EvaluateOffline(ctx, testCases)
    if err != nil {
        log.Fatal(err)
    }

    // Print report
    fmt.Println(report.FormattedReport())

    // Check if deployment should be blocked
    if report.ShouldBlock {
        log.Fatal("❌ Deployment blocked - evaluation failed")
    } else {
        log.Println("✅ Deployment approved - evaluation passed")
    }
}
```

### Example 2: Offline Evaluation with Deployment Blocking

```go
func runCIEvaluation() error {
    ctx := context.Background()

    // Create judge
    judge := evaluation.NewClaudeJudge(os.Getenv("ANTHROPIC_API_KEY"), nil)

    // Configure strict thresholds
    config := &evaluation.OfflineConfig{
        MinPassRate:     0.95, // 95% must pass
        MinAvgScore:     0.90, // Average score >= 0.90
        MaxFailedCases:  2,    // Allow max 2 failures
        BlockOnFailure:  true,
    }

    evaluator := evaluation.NewOfflineEvaluator(judge, config)

    // Load test cases from golden dataset
    testCases := loadGoldenDataset()

    // Run evaluation
    report, err := evaluator.EvaluateOffline(ctx, testCases)
    if err != nil {
        return fmt.Errorf("evaluation failed: %w", err)
    }

    // Save report for CI artifacts
    saveReportToFile(report)

    // Block deployment if thresholds not met
    if report.ShouldBlock {
        return fmt.Errorf("deployment blocked: %s", report.BlockReason)
    }

    return nil
}
```

### Example 3: Online Monitoring with Alerting

```go
func setupProductionMonitoring() {
    ctx := context.Background()

    // Create multiple alerters
    alerters := []evaluation.Alerter{
        evaluation.NewLogAlerter(),
        evaluation.NewWebhookAlerter("https://slack.example.com/webhook"),
        evaluation.NewEmailAlerter(
            []string{"oncall@example.com"},
            "Production LLM Alert",
        ),
    }

    // Configure thresholds
    config := &evaluation.OnlineConfig{
        Thresholds: evaluation.MetricThresholds{
            MinSuccessRate:  0.95,
            MaxP95LatencyMs: 500,
            MaxP99LatencyMs: 1000,
            MinSatisfaction: 0.8,
        },
    }

    // Create online evaluator
    judge := evaluation.NewGPT4Judge(os.Getenv("OPENAI_API_KEY"), nil)
    evaluator := evaluation.NewOnlineEvaluator(judge, alerters, config)

    // Start monitoring with 10% sampling
    if err := evaluator.MonitorProduction(ctx, 0.1); err != nil {
        log.Fatal(err)
    }

    // Process events from your session manager
    go func() {
        for event := range sessionEventStream {
            evaluator.ProcessEvent(event)
        }
    }()

    // Periodically check metrics
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()

    for range ticker.C {
        metrics := evaluator.GetMetrics()
        log.Printf("Metrics: Success=%.2f%%, P95=%.0fms, Satisfaction=%.2f",
            metrics.SuccessRate*100, metrics.P95LatencyMs, metrics.Satisfaction)
    }
}
```

### Example 4: Feedback Loop Usage

```go
func handleUserFeedback(sessionID string, feedback evaluation.SessionEvent) {
    ctx := context.Background()

    // Create feedback loop
    feedbackLoop := evaluation.NewFeedbackLoop(
        "/path/to/golden",
        githubPRCreator,
    )

    // Validate the example meets quality criteria
    if feedback.Score >= 0.9 && feedback.Valid {
        example := evaluation.GoldenExample{
            Input:          feedback.Input,
            ExpectedOutput: feedback.ExpectedOutput,
            Actual:         feedback.Actual,
            Score:          feedback.Score,
            Valid:          feedback.Valid,
        }

        // Add to golden dataset (creates PR for review)
        err := feedbackLoop.UpdateGoldenDataset(ctx, []evaluation.GoldenExample{example})
        if err != nil {
            log.Printf("Failed to update golden dataset: %v", err)
        } else {
            log.Printf("Created PR to add example to golden dataset")
        }
    }
}
```

### Example 5: Batch Evaluation with Metrics

```go
func runBatchEvaluation() {
    ctx := context.Background()

    // Define metrics to evaluate
    metrics := []evaluation.Metric{
        &evaluation.CorrectnessMetric{ExactMatch: true},
        &evaluation.SafetyMetric{HarmfulKeywords: evaluation.DefaultSafetyKeywords()},
        &evaluation.PerformanceMetric{MaxLatencyMs: 100},
    }

    // Define test cases
    testCases := []struct {
        input    string
        expected string
        actual   string
    }{
        {"input1", "expected1", "actual1"},
        {"input2", "expected2", "actual2"},
    }

    // Evaluate with all metrics
    for _, tc := range testCases {
        results := evaluation.EvaluateMetrics(
            tc.input,
            tc.expected,
            tc.actual,
            metrics,
            map[string]float64{
                "correctness":  0.9,
                "safety":       1.0,
                "performance":  0.8,
            },
        )

        for _, result := range results {
            fmt.Printf("%s: %.2f (%v)\n",
                result.MetricName, result.Score, result.Pass)
        }
    }
}
```

## Configuration

### API Keys

Set environment variables for LLM APIs:

```bash
export OPENAI_API_KEY="sk-..."
export ANTHROPIC_API_KEY="sk-ant-..."
```

### Offline Evaluation Thresholds

```go
config := &evaluation.OfflineConfig{
    MinPassRate:    0.90,  // 90% of tests must pass
    MinAvgScore:    0.85,  // Average score must be >= 0.85
    MaxFailedCases: 5,     // Allow max 5 failures
    BlockOnFailure: true,  // Block deployment on failure
}
```

### Online Monitoring Thresholds

```go
thresholds := evaluation.MetricThresholds{
    MinSuccessRate:  0.95,  // 95% success rate
    MaxP95LatencyMs: 500,   // P95 latency <= 500ms
    MaxP99LatencyMs: 1000,  // P99 latency <= 1000ms
    MinSatisfaction: 0.80,  // 80% user satisfaction
}

config := &evaluation.OnlineConfig{
    Thresholds: thresholds,
}
```

### Alert Channels

Configure multiple alert channels:

```go
alerters := []evaluation.Alerter{
    evaluation.NewLogAlerter(),  // Always log
    evaluation.NewWebhookAlerter("https://slack.example.com/webhook"),
    evaluation.NewEmailAlerter([]string{"team@example.com"}, "Alert"),
}
```

## Testing

Run all tests:

```bash
go test ./... -v -count=1
```

Run specific test package:

```bash
go test ./internal/evaluation -v
```

Run with coverage:

```bash
go test ./internal/evaluation -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Contributing

### Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `golangci-lint run` before committing
- Write tests for all new functionality
- Update documentation for API changes

### PR Process

1. Create a feature branch
2. Write tests first (TDD approach)
3. Implement functionality
4. Ensure all tests pass
5. Update documentation
6. Submit PR with clear description
7. Address review feedback

### Adding New Metrics

To add a new metric:

1. Implement the `Metric` interface
2. Add tests in `metrics_test.go`
3. Document the metric in this README
4. Add usage examples

Example:

```go
type CustomMetric struct {
    Threshold float64
}

func (m *CustomMetric) Name() string {
    return "custom"
}

func (m *CustomMetric) Evaluate(input, expected, actual string) float64 {
    // Your evaluation logic
    return 0.5
}
```

### Adding New Judge Implementations

To add a new judge (e.g., Gemini, Mistral):

1. Implement both `Judge` and `DetailedJudge` interfaces
2. Add configuration struct
3. Add tests following existing patterns
4. Update this README
5. Add ADR documenting the decision

## Architecture Decisions

See [ADR/](./ADR/) directory for detailed architecture decision records:

- [ADR-001: Dual Judge Interfaces](./ADR/ADR-001-dual-judge-interfaces.md)
- [ADR-002: Multiple LLM Judges](./ADR/ADR-002-multiple-llm-judges.md)
- [ADR-003: Threshold-Based Deployment Blocking](./ADR/ADR-003-threshold-based-deployment-blocking.md)
- [ADR-004: Pluggable Alert Channels](./ADR/ADR-004-pluggable-alert-channels.md)

## License

Copyright 2024-2026. See LICENSE file for details.
