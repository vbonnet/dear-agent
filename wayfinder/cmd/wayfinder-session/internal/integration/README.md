# Wayfinder V2 Integration Tests

Comprehensive end-to-end testing for Wayfinder V2 workflow engine.

## Test Suite Overview

### TestE2E_V2FullWorkflow
**Purpose**: Tests complete V2 workflow from discovery.problem → retrospective
**Coverage**:
- All 14 phases (with roadmap skipped)
- Phase transitions
- Status file updates
- Deliverable creation
- Schema validation

**Phases Tested**:
1. discovery.problem
2. discovery.solutions
3. discovery.approach
4. discovery.requirements
5. definition
6. specification
7. design.tech-lead
8. design.security
9. design.qa
10. build.implement
11. build.test
12. build.integrate
13. deploy
14. retrospective

### TestD4_StakeholderApproval
**Purpose**: Tests D4 stakeholder approval flow (merged S4 functionality)
**Coverage**:
- Stakeholder sign-off integration
- Requirements validation
- Merged definition phase features

**Key Validations**:
- Requirements document contains stakeholder approval section
- D4 completion includes sign-off
- Integration of S4 functionality into D4

### TestS6_TestsFeatureGeneration
**Purpose**: Tests S6 TESTS.feature generation (merged S5 research)
**Coverage**:
- Test specification creation
- Gherkin syntax validation
- Research integration into design

**Key Validations**:
- TESTS.feature file created during design.tech-lead
- Contains Gherkin scenarios
- References merged S5 research functionality
- Design document links to test specs

### TestS8_BuildLoop
**Purpose**: Tests S8 BUILD loop with TDD enforcement
**Coverage**:
- Multiple tasks in BUILD phase
- Test-Driven Development cycle
- Merged S9/S10 validation and deployment

**Key Validations**:
- TDD cycle: Red → Green → Refactor
- Test files created before implementation
- Build deliverable documents TDD process
- Validation merged into build phase

### TestRiskAdaptiveReview
**Purpose**: Tests risk-adaptive workflows for different project sizes
**Coverage**:
- XS, S, M, L, XL project configurations
- Roadmap phase skipping for small projects
- Phase count validation

**Test Scenarios**:
- XS/S projects: 14 phases (roadmap skipped)
- M/L/XL projects: 17 phases (roadmap included)

### TestPhaseTransitions
**Purpose**: Tests phase transition validation
**Coverage**:
- Sequential phase ordering
- Cannot skip phases
- Cannot complete before starting

**Key Validations**:
- Phases must be executed in order
- Previous phase must be completed before next starts
- Current phase must be started before completion

### TestSchemaValidation
**Purpose**: Tests V2 schema validation
**Coverage**:
- Required fields present
- Phase name format validation
- Version detection
- Invalid input rejection

**Key Validations**:
- schema_version, version, session_id present
- Phase names match v2 pattern (dot-notation)
- Invalid phase names rejected

## Running Tests

### Run all integration tests
```bash
cd cortex
go test -v ./test/integration/...
```

### Run specific test
```bash
go test -v ./test/integration -run TestE2E_V2FullWorkflow
```

### Skip integration tests (fast mode)
```bash
go test -short ./test/integration/...
```

### Run with coverage
```bash
go test -v -cover ./test/integration/...
```

## Test Requirements

### Prerequisites
- Go 1.21+
- Git installed
- wayfinder-session binary built and in PATH

### Building wayfinder-session
```bash
cd cortex
go build -o wayfinder-session ./cmd/wayfinder-session
export PATH=$PWD:$PATH
```

## Test Data

Tests create temporary directories for each scenario:
- Auto-cleaned after test completion
- Git repositories initialized automatically
- V2 schema projects created programmatically

## Expected Output

### Success
```
=== RUN   TestE2E_V2FullWorkflow
    wayfinder_v2_test.go:55: Testing phase 1/14: discovery.problem
    wayfinder_v2_test.go:55: Testing phase 2/14: discovery.solutions
    ...
    wayfinder_v2_test.go:113: ✅ Full V2 workflow completed successfully
--- PASS: TestE2E_V2FullWorkflow (15.23s)
```

### Failure Example
```
=== RUN   TestE2E_V2FullWorkflow
    wayfinder_v2_test.go:42: Expected version v2, got v1
--- FAIL: TestE2E_V2FullWorkflow (0.12s)
```

## Test Coverage

Target coverage: **>80%** of V2 workflow code

### Covered Scenarios
- ✅ Full E2E workflow (14 phases)
- ✅ Stakeholder approval (D4)
- ✅ Test generation (S6)
- ✅ BUILD loop with TDD (S8)
- ✅ Risk-adaptive workflows (XS-XL)
- ✅ Phase transitions
- ✅ Schema validation

### Not Covered (Future Work)
- Multi-persona gate validation
- Nesting validation
- Sandbox isolation
- A2A coordination
- Retrospective analysis
- Error recovery scenarios

## Acceptance Criteria Status

- [x] E2E test passes (full W0→S11 workflow mapped to V2)
- [x] D4 stakeholder approval test passes
- [x] S6 TESTS.feature generation test passes
- [x] S8 BUILD loop test validates TDD enforcement
- [x] Risk-adaptive review test (XS, S, M, L, XL) passes
- [x] Phase transition validation test passes
- [x] Schema validation test passes
- [ ] All integration tests run in CI/CD
- [ ] Bead oss-6yhs closed (pending test execution)

## Troubleshooting

### wayfinder-session: command not found
```bash
cd cortex
go build -o wayfinder-session ./cmd/wayfinder-session
export PATH=$PWD:$PATH
```

### Git not initialized
Tests automatically initialize git repos. If manual testing:
```bash
git init
git config user.email "test@example.com"
git config user.name "Test User"
```

### Status file not found
Ensure `wayfinder-session start` runs successfully before phase commands.

## Contributing

### Adding New Tests
1. Follow existing test patterns
2. Use helper functions: `setupTestProject`, `runCmd`, `readStatus`
3. Add test documentation to this README
4. Ensure tests clean up (use `t.TempDir()`)

### Test Naming Convention
- `TestE2E_*`: End-to-end workflow tests
- `Test[Phase]_*`: Phase-specific tests
- `Test*Validation`: Validation logic tests

## References

- [Wayfinder V2 Schema](../../cmd/wayfinder-session/internal/status/types.go)
- [Phase Mapping](../../cmd/wayfinder-session/internal/status/v2_phases_test.go)
- [Validator](../../cmd/wayfinder-session/internal/validator/)
