# Multi-Persona Review Package

Risk-adaptive code review system for Wayfinder V2 BUILD loop.

## Quick Start

```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/review"

// Create review engine
engine := review.NewReviewEngine(projectDir, status)

// Review a task
result, err := engine.ReviewTask(task)
if !result.Passed {
    fmt.Println("Review failed:", result.BlockingIssues)
}
```

## Features

- **5 Specialized Personas**: Security, Performance, Maintainability, UX, Reliability
- **Risk-Adaptive Strategy**: XS/S/M = Batch review, L/XL = Per-task review
- **Automated Risk Calculation**: LOC, file criticality, patterns, complexity
- **Harness Profiles**: Lite/Standard/Deep profiles via `ClassifyRisk()` for risk-adaptive process depth
- **Multiple Report Formats**: Text, Markdown, JSON
- **Comprehensive Testing**: 500+ lines of unit and integration tests

## Files

| File | Purpose |
|------|---------|
| `review_engine.go` | Core orchestration logic |
| `personas.go` | Persona definitions and patterns |
| `risk_adapter.go` | Risk calculation and escalation |
| `report.go` | Report generation (text/markdown/JSON) |
| `review_engine_test.go` | Unit tests (17 tests) |
| `persona_integration_test.go` | Integration tests (7 tests) |
| `harness_profile.go` | HarnessProfile types (Lite/Standard/Deep) and ProfileConfig |
| `harness_profile_test.go` | Tests for profile classification and ClassifyRisk |
| `multi-persona-review-implementation.md` | Full documentation |

## Testing

```bash
# Run all tests
go test -v ./...

# Run with coverage
go test -cover ./...

# Run linter
golangci-lint run ./...

# Or use the test script
chmod +x run-tests.sh
./run-tests.sh
```

## Documentation

See [multi-persona-review-implementation.md](multi-persona-review-implementation.md) for:
- Architecture details
- Usage examples
- Integration with BUILD loop
- Configuration options
- Metrics and reporting

## Quality Gates

✅ All tests pass
✅ Code compiles without errors
✅ golangci-lint clean
✅ Comprehensive documentation

## Related

- [Risk-Adaptive Review Rules](risk-adaptive-review-rules.md)
- [BUILD Loop State Machine](build-loop-state-machine.md)

---

**Bead**: oss-w5iz
**Date**: 2026-02-20
**Swarm**: wayfinder-v2-consolidation
