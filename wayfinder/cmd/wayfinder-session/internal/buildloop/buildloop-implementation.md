# BUILD Loop State Machine Implementation

## Overview

This package implements the 8-state BUILD loop state machine for the S8 (build.implement)
phase of Wayfinder v2. It enforces TDD principles, continuous validation, and risk-adaptive
reviews through tight feedback cycles.

## Architecture

### Core Components

1. **buildloop.go** - Main state machine orchestration
2. **states.go** - State definitions, transitions, and exit criteria
3. **iteration_tracker.go** - Per-task iteration tracking and metrics

### State Machine Design

The BUILD loop implements 8 primary states with 3 error/recovery states:

```
TEST_FIRST → CODING → GREEN → REFACTOR → VALIDATION → DEPLOY → MONITORING → COMPLETE
     ↓         ↓                  ↓           ↓
  TIMEOUT  TIMEOUT         REVIEW_FAILED  INTEGRATE_FAIL
```

## States

### Primary States

1. **TEST_FIRST** (Red Phase)
   - Entry point for TDD cycle
   - Tests must fail as expected
   - Validates test scope matches task requirements
   - Exit: Failures analyzed, ready to write code

2. **CODING**
   - Write minimal code to pass failing tests
   - Track code changes
   - Exit: Code committed, ready for validation

3. **GREEN** (Tests Pass)
   - All tests passing
   - Run quality gates (assertion density, coverage)
   - Calculate risk level
   - Exit: Tests green, quality evaluated

4. **REFACTOR**
   - Improve code quality without breaking tests
   - Maintain green tests throughout
   - Exit: Refactoring complete, tests still pass

5. **VALIDATION** (Multi-persona Review)
   - Automated code review
   - Check for P0/P1 blocking issues
   - Exit: Review complete with issue count

6. **DEPLOY** (Integration Testing)
   - Run integration tests
   - Execute deployment pipeline
   - Exit: Deployment successful/failed

7. **MONITORING** (Production/Staging)
   - Observe deployed code
   - Detect runtime issues
   - Exit: Monitoring complete, no issues

8. **COMPLETE** (Task Done)
   - Task successfully completed
   - Metrics recorded
   - Exit: Ready for next task or integration

### Error/Recovery States

- **TIMEOUT** - Test execution timeout, add timeout guards
- **REVIEW_FAILED** - P0/P1 issues found, implement fixes
- **INTEGRATE_FAIL** - Integration tests failed, rework task

## State Transitions

### Valid Transitions

| From State     | To States                                    |
|----------------|---------------------------------------------|
| TEST_FIRST     | CODING, TIMEOUT, TEST_FIRST                 |
| CODING         | GREEN, TEST_FIRST, TIMEOUT                  |
| GREEN          | REFACTOR, VALIDATION, COMPLETE, CODING      |
| REFACTOR       | VALIDATION, GREEN, CODING                   |
| VALIDATION     | COMPLETE, REVIEW_FAILED, CODING             |
| DEPLOY         | MONITORING, REVIEW_FAILED, INTEGRATE_FAIL   |
| MONITORING     | COMPLETE, CODING                            |
| COMPLETE       | TEST_FIRST, DEPLOY                          |
| TIMEOUT        | CODING, TEST_FIRST                          |
| REVIEW_FAILED  | CODING, TEST_FIRST                          |
| INTEGRATE_FAIL | TEST_FIRST, DEPLOY                          |

### Transition Guards

Each transition is validated using `ValidateTransition()` which checks:
- Source state is valid
- Target state is valid
- Transition is allowed per StateTransitions map

## Exit Criteria

Each state has defined exit criteria enforced through validators:

```go
type ExitCriteria struct {
    State       State
    Description string
    Validator   func(ctx *BuildContext) (bool, error)
}
```

### Examples

- **TEST_FIRST**: Test failures match task scope, ready to code
- **CODING**: Code changes committed, ready for validation
- **GREEN**: Tests pass AND quality gates evaluated
- **VALIDATION**: Review completed with P0/P1 count

## Risk-Adaptive Review

Tasks are routed based on risk level:

| Risk Level | LOC Range  | Review Strategy      |
|------------|------------|---------------------|
| XS         | < 50       | No per-task review  |
| S          | 50-200     | No per-task review  |
| M          | 200-500    | No per-task review  |
| L          | 500-1000   | Per-task review     |
| XL         | > 1000     | Per-task review     |

```go
if task.RiskLevel.RequiresPerTaskReview() {
    return StateValidation
}
return StateRefactor
```

## Iteration Tracking

The `IterationTracker` records metrics per task:

- Current iteration number
- State visits per iteration
- Test run count
- Duration metrics
- Success/failure rate

### Metrics Collected

```go
type TaskMetrics struct {
    Duration         time.Duration
    RetryCount       int
    TestRunCount     int
    AssertionDensity float64
    CoveragePercent  float64
    StateTransitions int
}
```

## Configuration

Default configuration values:

```go
Config{
    MaxRetries:              3,
    MinAssertionDensity:     0.5,     // 0.5 assertions per test
    MinCoveragePercent:      80.0,    // 80% coverage
    TestTimeoutSeconds:      300,     // 5 minutes
    ReviewTimeoutSeconds:    600,     // 10 minutes
    EnableTDDEnforcement:    true,
    EnableParallelExecution: false,
}
```

## Usage

### Basic Usage

```go
task := &Task{
    ID:          "T1",
    Description: "Add authentication",
    Files:       []string{"auth.go", "auth_test.go"},
    RiskLevel:   RiskM,
}

bl := NewBuildLoop(task, nil) // Use default config
result, err := bl.Execute()

if err != nil {
    log.Fatalf("BUILD loop failed: %v", err)
}

if result.Success {
    fmt.Printf("Task %s completed in %v\n",
        task.ID, result.Metrics.Duration)
}
```

### Custom Configuration

```go
config := &Config{
    MaxRetries:           5,
    MinAssertionDensity:  0.7,
    MinCoveragePercent:   90.0,
    EnableTDDEnforcement: true,
}

bl := NewBuildLoop(task, config)
result, err := bl.Execute()
```

### Accessing Metrics

```go
result, _ := bl.Execute()

fmt.Printf("Duration: %v\n", result.Metrics.Duration)
fmt.Printf("Retries: %d\n", result.Metrics.RetryCount)
fmt.Printf("Test runs: %d\n", result.Metrics.TestRunCount)
fmt.Printf("Coverage: %.1f%%\n", result.Metrics.CoveragePercent)
fmt.Printf("State transitions: %d\n", result.Metrics.StateTransitions)
```

### State History

```go
result, _ := bl.Execute()

for _, rec := range bl.GetStateHistory() {
    fmt.Printf("%s -> %s (%s) at %v\n",
        rec.From, rec.To, rec.Trigger, rec.Timestamp)
}
```

## Testing

### Running Tests

```bash
# From module directory
cd cortex

# Run all tests
go test ./cmd/wayfinder-session/internal/buildloop/...

# Run with coverage
go test -cover ./cmd/wayfinder-session/internal/buildloop/...

# Run specific test
go test -run TestStateMachineHappyPath ./cmd/wayfinder-session/internal/buildloop/
```

### Test Coverage

The package includes comprehensive tests:

1. **states_test.go** - State validation, transitions, exit criteria
2. **buildloop_test.go** - Core BUILD loop logic, state execution
3. **iteration_tracker_test.go** - Iteration tracking, metrics
4. **state_machine_test.go** - End-to-end state machine workflows

Coverage areas:
- All state transitions
- Exit criteria validation
- Risk-based routing
- Error recovery
- Concurrent access
- Edge cases

## Integration with Wayfinder

The BUILD loop integrates with Wayfinder v2 at the S8 phase:

```
discovery.problem → ... → build.implement (S8) → ... → deploy.release
                              ↓
                         BUILD Loop
                              ↓
                    Task → TEST_FIRST → ... → COMPLETE
```

### Task Flow

1. ROADMAP.md contains tasks with dependencies
2. Wayfinder selects ready task (dependencies met)
3. BUILD loop executes task through states
4. Task completion unblocks dependent tasks
5. Loop continues until all tasks complete

## Quality Gates

### Assertion Density Check

Minimum 0.5 assertions per test:

```go
assertionDensity := totalAssertions / totalTests
if assertionDensity < 0.5 {
    // Fail quality gate
}
```

### Coverage Check

Minimum 80% coverage for changed files:

```go
coverage := coveredLines / totalLines * 100
if coverage < 80.0 {
    // Fail quality gate
}
```

## Error Handling

### Timeout Handling

```go
if testResult.Timeout {
    return StateTimeout
}
```

### Review Failures

```go
if len(reviewResult.P0Issues) > 0 || len(reviewResult.P1Issues) > 0 {
    return StateReviewFailed
}
```

### Retry Logic

```go
for !state.IsTerminal() && retryCount < maxRetries {
    // Execute state
    if state.IsErrorState() {
        retryCount++
    }
}
```

## Best Practices

1. **TDD Enforcement** - Always start with failing tests
2. **Minimal Code** - Write only enough code to pass tests
3. **Quality Gates** - Never skip quality checks
4. **Risk Awareness** - Route high-risk tasks through review
5. **Metrics Collection** - Track all iterations for learning

## Future Enhancements

1. **Parallel Execution** - Execute independent tasks concurrently
2. **AI-Assisted Review** - Use LLM for code review
3. **Adaptive Thresholds** - Adjust quality gates based on project
4. **Integration with CI/CD** - Hook into existing pipelines
5. **Real-time Monitoring** - Live dashboard of BUILD loop progress

## References

- Design: `build-loop-state-machine.md`
- Algorithm: `task-iteration-algorithm.md`
- Wayfinder: `cortex/cmd/wayfinder-session/`

## License

Copyright 2026 - Part of the Engram project
