# BUILD Loop State Machine

8-state TDD-enforcing state machine for Wayfinder v2 S8 (build.implement) phase.

## Quick Start

```go
task := &buildloop.Task{
    ID:          "T1",
    Description: "Add authentication",
    RiskLevel:   buildloop.RiskM,
}

bl := buildloop.NewBuildLoop(task, nil)
result, err := bl.Execute()

if result.Success {
    fmt.Printf("✓ Task completed in %v\n", result.Metrics.Duration)
    fmt.Printf("  Coverage: %.1f%%\n", result.Metrics.CoveragePercent)
}
```

## States

```
TEST_FIRST → CODING → GREEN → REFACTOR → VALIDATION → DEPLOY → MONITORING → COMPLETE
     ↓         ↓              ↓           ↓
  TIMEOUT  TIMEOUT    REVIEW_FAILED  INTEGRATE_FAIL
```

## Files

- `buildloop.go` - Core state machine logic
- `states.go` - State definitions and transitions
- `iteration_tracker.go` - Per-task iteration tracking
- `buildloop-implementation.md` - Complete documentation
- `doc.go` - Package documentation

## Tests

```bash
cd cortex
go test ./cmd/wayfinder-session/internal/buildloop/...        # Run tests
go test -cover ./cmd/wayfinder-session/internal/buildloop/... # With coverage
golangci-lint run ./cmd/wayfinder-session/internal/buildloop/... # Lint
```

## Test Coverage

- **78.1%** code coverage
- 67 test cases covering:
  - State transitions (valid and invalid)
  - Exit criteria validation
  - Risk-based routing
  - Error recovery
  - Concurrent access
  - Edge cases

## Risk-Adaptive Review

| Risk | LOC Range | Review Strategy |
|------|-----------|----------------|
| XS   | < 50      | Batch review   |
| S    | 50-200    | Batch review   |
| M    | 200-500   | Batch review   |
| L    | 500-1000  | Per-task review |
| XL   | > 1000    | Per-task review |

## Quality Gates

- **Assertion Density**: ≥ 0.5 assertions/test
- **Coverage**: ≥ 80% for changed files
- **Complexity**: ≤ 10 cyclomatic complexity

## Configuration

```go
config := &buildloop.Config{
    MaxRetries:           3,
    MinAssertionDensity:  0.5,
    MinCoveragePercent:   80.0,
    TestTimeoutSeconds:   300,
    EnableTDDEnforcement: true,
}

bl := buildloop.NewBuildLoop(task, config)
```

## Design References

- `build-loop-state-machine.md` - State machine specification
- `task-iteration-algorithm.md` - Iteration algorithm
- `buildloop-implementation.md` - Implementation details
