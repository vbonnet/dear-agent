## Multi-Persona Review Implementation

### Overview

This package implements Multi-Persona Review Integration for Wayfinder V2, enabling risk-adaptive code reviews with multiple specialized personas during the BUILD loop's VALIDATION state.

### Architecture

```
review/
├── review_engine.go           # Core review orchestration logic
├── personas.go                # Persona definitions and configurations
├── risk_adapter.go            # Risk-adaptive review strategy
├── report.go                  # Review report generation
├── review_engine_test.go      # Unit tests for review engine
└── persona_integration_test.go # Integration tests for personas
```

### Components

#### 1. Review Engine (`review_engine.go`)

The `ReviewEngine` orchestrates multi-persona reviews with risk-adaptive strategies.

**Key Functions:**
- `NewReviewEngine(projectDir, status)` - Creates review engine instance
- `ReviewTask(task)` - Performs per-task review for high-risk tasks
- `ReviewBatch(tasks)` - Performs batch review for low/medium-risk tasks
- `ShouldTriggerPerTaskReview(task)` - Determines if per-task review needed
- `ShouldTriggerBatchReview(tasks)` - Determines if batch review needed

**Review Flow:**
1. Calculate task risk level
2. Select appropriate personas based on risk
3. Execute persona reviews (parallel)
4. Aggregate results and calculate scores
5. Extract blocking issues
6. Generate review report

#### 2. Personas (`personas.go`)

Defines five specialized review personas, each with specific focus areas:

##### Security Persona
- **Focus**: Vulnerabilities, threats, injection attacks
- **Patterns**:
  - P0: Hardcoded credentials, eval/exec, command injection
  - P1: SQL injection, XSS, unsafe deserialization
  - P2: Insecure HTTP, security TODOs

##### Performance Persona
- **Focus**: Bottlenecks, resource usage, efficiency
- **Patterns**:
  - P1: N+1 queries, long sleeps, string concatenation in queries
  - P2: Defer in loops, append without capacity
  - P3: Inefficient string formatting

##### Maintainability Persona
- **Focus**: Code quality, complexity, maintainability
- **Patterns**:
  - P1: Very long functions (>100 lines)
  - P2: TODOs, FIXMEs, HACKs
  - P3: Poor naming, single-letter variables

##### UX Persona
- **Focus**: User experience, error messages, accessibility
- **Patterns**:
  - P1: Panics, printing errors to console
  - P2: Silent failures, fatal logs
  - P3: UX TODOs

##### Reliability Persona
- **Focus**: Error handling, edge cases, robustness
- **Patterns**:
  - P0: Division by zero, array access without bounds
  - P1: Swallowing errors, ignoring error returns
  - P2: Infinite loops, select without default

#### 3. Risk Adapter (`risk_adapter.go`)

Implements risk-adaptive review strategy based on multiple factors.

**Risk Levels:**
- **XS** (1-50 LOC): Batch review only
- **S** (51-200 LOC): Batch review only
- **M** (201-500 LOC): Batch review only
- **L** (501-1000 LOC): Per-task review
- **XL** (1001+ LOC): Per-task review

**Risk Calculation Factors:**
1. **Lines of Code** (30% weight)
2. **File Criticality** (30% weight) - auth, security, payment, etc.
3. **Change Type** (20% weight) - new feature vs bug fix
4. **Coverage Risk** (10% weight) - existing test coverage
5. **Pattern Detection** (10% weight) - risky code patterns

**Escalation Rules:**
- Critical files (auth, security, payment) → minimum L risk
- 3+ critical files → XL risk
- High complexity (>15) → upgrade one level
- Cross-cutting changes (3+ domains) → upgrade one level

#### 4. Report Generation (`report.go`)

Generates human-readable review reports in multiple formats.

**Formats:**
- **Text Report**: Console-friendly format with tables
- **Markdown Report**: GitHub/GitLab compatible format
- **JSON Report**: Machine-readable format for tooling

**Report Sections:**
- Overview (task ID, risk level, status, score)
- Metrics Summary (issues by severity, persona scores)
- Persona Reviews (findings per persona)
- Blocking Issues (P0/P1 issues that block deployment)
- Recommendations (suggestions from all personas)

### Usage

#### Basic Usage

```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/review"

// Create review engine
st := &status.StatusV2{ProjectName: "my-project"}
engine := review.NewReviewEngine("/path/to/project", st)

// Review a single task
task := &status.Task{
    ID:          "task-1",
    Description: "implement authentication",
    Deliverables: []string{"auth/handler.go"},
}

result, err := engine.ReviewTask(task)
if err != nil {
    log.Fatal(err)
}

// Check if review passed
if !result.Passed {
    fmt.Println("Review failed with blocking issues:")
    for _, issue := range result.BlockingIssues {
        fmt.Printf("- [%s] %s\n", issue.Severity, issue.Message)
    }
}

// Generate report
report := engine.GenerateReport(result)
fmt.Println(report)
```

#### Batch Review

```go
// Review multiple low-risk tasks together
tasks := []*status.Task{
    {ID: "task-1", Description: "fix typo", Deliverables: []string{"README.md"}},
    {ID: "task-2", Description: "update comment", Deliverables: []string{"utils.go"}},
}

if engine.ShouldTriggerBatchReview(tasks) {
    result, err := engine.ReviewBatch(tasks)
    if err != nil {
        log.Fatal(err)
    }

    // Save report
    engine.SaveReportToFile(result, "review-report.txt")
    engine.SaveReportJSON(result, "review-result.json")
}
```

#### Risk-Adaptive Strategy

```go
// Determine if per-task review is needed
if engine.ShouldTriggerPerTaskReview(task) {
    // High risk (L/XL) - immediate review
    result, _ := engine.ReviewTask(task)
} else {
    // Low/medium risk (XS/S/M) - defer to batch review
    // Continue with next task
}
```

### Integration with BUILD Loop

The review engine integrates with the BUILD loop state machine at the VALIDATION state:

```
tests_passing → reviewing (if L/XL risk)
              ↓
         task_complete (if no blocking issues)
              ↓
         deploying (batch review if XS/S/M risk)
```

**Per-Task Review (L/XL):**
1. Task completes with tests passing
2. Risk adapter calculates risk level
3. If L or XL, trigger per-task review
4. Review executes with all personas
5. If blocking issues (P0/P1), block deployment
6. If no blocking issues, mark task complete

**Batch Review (XS/S/M):**
1. All tasks complete
2. Collect unreviewed tasks
3. Execute batch review with limited personas
4. If blocking issues found, block deployment
5. If passed, proceed to deployment

### Configuration

Review behavior is configurable via `ReviewConfig`:

```go
config := &review.ReviewConfig{
    // Quality gates
    MinAssertionDensity: 0.5,
    MinCoveragePercent:  80.0,

    // Timeouts
    TestExecutionSeconds:   300,
    ReviewExecutionSeconds: 600,

    // Risk thresholds
    XSMaxLOC: 50,
    SMaxLOC:  200,
    MMaxLOC:  500,
    LMaxLOC:  1000,
    XLMinLOC: 1001,

    // Review triggers
    PerTaskMinRisk:     review.RiskLevelL,
    BatchReviewMaxRisk: review.RiskLevelM,
}

engine := &review.ReviewEngine{
    projectDir: "/path/to/project",
    config:     config,
}
```

### Testing

#### Unit Tests (`review_engine_test.go`)

Tests core review engine functionality:
- Engine initialization
- Risk-based review triggering
- Per-task review execution
- Batch review execution
- Metrics calculation
- Issue extraction
- Report generation

**Run tests:**
```bash
cd cortex/cmd/wayfinder-session/internal/review
go test -v -run TestReviewEngine
```

#### Integration Tests (`persona_integration_test.go`)

Tests end-to-end review workflows:
- Complete review workflow (low → high risk)
- Security persona detection
- Performance persona detection
- Maintainability persona detection
- Reliability persona detection
- Risk-adaptive strategy
- Report generation

**Run integration tests:**
```bash
go test -v -run TestIntegration
```

**Run all tests:**
```bash
go test -v ./...
```

### Quality Gates

Before committing, ensure all quality gates pass:

```bash
# Run linter
golangci-lint run ./...

# Run all tests
go test ./...

# Check test coverage
go test -cover ./...
```

### Metrics

The review engine tracks the following metrics:

**Issue Metrics:**
- Total issues count
- Issues by severity (P0, P1, P2, P3)

**Persona Scores (0-100):**
- Security score
- Performance score
- Maintainability score
- UX score
- Reliability score

**Performance Metrics:**
- Review duration (milliseconds)

### Future Enhancements

1. **External Tool Integration:**
   - golangci-lint for Go code
   - eslint for JavaScript/TypeScript
   - security scanners (gosec, semgrep)

2. **ML-Based Detection:**
   - Train models on historical issues
   - Predict risk based on patterns

3. **Caching:**
   - Cache persona reviews for unchanged files
   - Incremental reviews

4. **Parallel Execution:**
   - Run persona reviews in parallel
   - Reduce total review time

5. **Custom Personas:**
   - Allow users to define custom personas
   - Domain-specific review rules

### References

- [Risk-Adaptive Review Rules](risk-adaptive-review-rules.md)
- [BUILD Loop State Machine](build-loop-state-machine.md)
- [Wayfinder V2 Status Schema](cortex/cmd/wayfinder-session/internal/status/types_v2.go)

---

**Implementation Date**: 2026-02-20
**Bead ID**: oss-w5iz
**Package**: github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/review
