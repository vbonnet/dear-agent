# AGM Comprehensive Testing Plan

## Executive Summary

**Current Coverage**: Individual package coverage shown below
**Target Coverage**: 80%+ (but focus on **effective testing**, not just coverage numbers)
**Testing Philosophy**: Mix of unit, integration, and E2E tests following Go best practices

## Coverage Analysis (2026-01-13)

### ✅ High Coverage (>80%) - Maintain Quality
- `internal/backup`: 80.8%
- `internal/claude`: 90.9%
- `internal/config`: 83.9%
- `internal/fix`: 89.4%
- `internal/fuzzy`: 95.2%
- `internal/history`: 91.6%
- `internal/lock`: 87.5%
- `internal/transcript`: 92.3%
- `internal/uuid`: 80.2%

### ⚠️  Medium Coverage (40-80%) - Needs Improvement
- `internal/detection`: 68.2%
- `internal/fileutil`: 60.9%
- `internal/readiness`: 62.2%
- `internal/tokenlogger`: 76.3%
- `internal/manifest`: 51.2%

### ❌ Low/Zero Coverage (<40%) - CRITICAL GAPS
- **`cmd/csm`**: 9.0% 🔴 Main CLI commands
- **`cmd/csm-reaper`**: 0.0% 🔴 No tests
- **`internal/cli`**: 0.0% 🔴 No tests
- **`internal/debug`**: 0.0% 🔴 No tests
- **`internal/discovery`**: 0.0% 🔴 No tests
- **`internal/llm`**: 22.9%
- **`internal/reaper`**: 7.3%
- **`internal/session`**: 33.6% 🔴 Core session logic
- **`internal/tmux`**: 35.2% 🔴 Core tmux interaction
- **`internal/ui`**: 17.9%
- **`internal/validate`**: 10.9%

---

## Critical Test Priorities

### Priority 1: Core Functionality (HIGHEST RISK)

#### 1.1 `internal/tmux` (35.2% → 80%+)

**Why Critical**: Core tmux interaction, recent regression (SendCommand Enter key bug)

**Test Coverage Needed**:
- [x] `SendCommand()` - REGRESSION TEST FOR ENTER KEY BUG
  - Test that command text and Enter are sent separately
  - Test that Enter is not included in command string
  - Verify no newline created in prompt
- [ ] `NewSession()` - Session creation with settings injection
- [ ] `AttachSession()` - TTY detection, attach vs switch-client
- [ ] `HasSession()` - Session existence checking
- [ ] `WaitForProcessReady()` - Process polling with timeout
- [ ] `IsProcessRunning()` - Foreground process detection
- [ ] `GetCurrentWorkingDirectory()` - Pane CWD extraction
- [ ] Socket path handling and isolation
- [ ] Timeout handling and error cases

**Test Type**: Unit + Integration
**Files**: `internal/tmux/tmux_test.go`, `internal/tmux/send_command_test.go`

#### 1.2 `cmd/csm` (9.0% → 60%+)

**Why Critical**: Main user-facing CLI commands, most execution paths

**Commands to Test**:
- [ ] `agm session new` - Session creation flow
  - `/rename` sent before `/agm-assoc` (NEW BEHAVIOR)
  - Manifest creation with proper UUID
  - Trust prompt prevention via additionalDirectories
  - Ready-file wait and attach timing
- [ ] `agm session resume` - Session attachment
- [ ] `agm session list` - Session listing and filtering
- [ ] `agm session archive` - Archival with lifecycle state (PARTIALLY TESTED)
- [ ] `agm admin sync` - UUID population from history
- [ ] `agm session status` - Session status reporting
- [ ] `agm admin doctor` - Health checks

**Test Type**: Integration + E2E (using testscript or re-exec pattern)
**Files**: `cmd/csm/*_test.go`, `test/e2e/*_test.go`

#### 1.3 `internal/session` (33.6% → 75%+)

**Why Critical**: Core session management logic

**Test Coverage Needed**:
- [ ] Session CRUD operations
- [ ] Lifecycle state transitions
- [ ] Status detection logic
- [ ] Discovery and resolution
- [ ] Manifest updates

**Test Type**: Unit
**Files**: `internal/session/session_test.go` (expand existing)

---

### Priority 2: Missing Test Coverage (0%)

#### 2.1 `internal/cli` (0% → 70%+)

**Test Needed**:
- [ ] Command initialization
- [ ] Flag parsing
- [ ] Cobra command structure
- [ ] Help text generation

**Test Type**: Unit
**Files**: `internal/cli/cli_test.go` (NEW)

#### 2.2 `internal/discovery` (0% → 70%+)

**Test Needed**:
- [ ] Session discovery from filesystem
- [ ] UUID extraction from history
- [ ] Tmux session detection
- [ ] Session resolution by identifier

**Test Type**: Unit + Integration
**Files**: `internal/discovery/discovery_test.go` (NEW)

#### 2.3 `internal/debug` (0% → 60%+)

**Test Needed**:
- [ ] Debug logging initialization
- [ ] Phase tracking
- [ ] Log file creation and rotation

**Test Type**: Unit
**Files**: `internal/debug/debug_test.go` (NEW)

#### 2.4 `cmd/csm-reaper` (0% → 50%+)

**Test Needed**:
- [ ] Background reaper process
- [ ] Session cleanup logic
- [ ] Async archival

**Test Type**: Integration
**Files**: `cmd/csm-reaper/reaper_test.go` (NEW)

---

### Priority 3: Improve Medium Coverage

#### 3.1 `internal/manifest` (51.2% → 80%+)

**Test Gaps**:
- [ ] Migration v1 → v2 edge cases
- [ ] Concurrent manifest updates
- [ ] Schema validation errors
- [ ] Lock file interaction

**Test Type**: Unit
**Files**: `internal/manifest/manifest_test.go` (expand)

#### 3.2 `internal/ui` (17.9% → 60%+)

**Test Gaps**:
- [ ] Form rendering
- [ ] Input validation
- [ ] Accessibility features
- [ ] Error message formatting

**Test Type**: Unit
**Files**: `internal/ui/ui_test.go` (expand)

#### 3.3 `internal/validate` (10.9% → 75%+)

**Test Gaps**:
- [ ] Validation error reporting
- [ ] Fix suggestions
- [ ] Classification logic

**Test Type**: Unit
**Files**: `internal/validate/*_test.go` (expand)

---

## Testing Strategy

### Test Types

#### Unit Tests (60% of effort)
- **Location**: Alongside source (`package_test.go`)
- **Pattern**: Table-driven tests with `t.Run()`
- **Mocking**: Interface-based dependency injection
- **Tools**: Standard library `testing`, `testify/assert` where helpful

#### Integration Tests (25% of effort)
- **Location**: `test/integration/`
- **Pattern**: Multi-component workflows
- **Examples**:
  - `agm session new` → check manifest → verify tmux session
  - Lock contention across multiple AGM commands
  - Concurrent session creation

#### E2E Tests (15% of effort)
- **Location**: `test/e2e/`
- **Pattern**: Full CLI execution with real binaries
- **Approach**: testscript (preferred) or re-exec pattern
- **Examples**:
  - Complete session lifecycle (new → work → archive)
  - Multi-session scenarios
  - Recovery from failures

### Testing Patterns

#### 1. Table-Driven Tests
```go
tests := []struct {
    name    string
    input   string
    want    string
    wantErr bool
}{
    {"valid case", "input", "output", false},
    {"error case", "bad", "", true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        got, err := Function(tt.input)
        if (err != nil) != tt.wantErr {
            t.Errorf("unexpected error: %v", err)
        }
        if got != tt.want {
            t.Errorf("got %v, want %v", got, tt.want)
        }
    })
}
```

#### 2. testdata Fixtures
```
internal/tmux/
├── tmux.go
├── tmux_test.go
└── testdata/
    ├── session-list.txt
    ├── pane-output.golden
    └── control-mode-events.txt
```

#### 3. Test Helpers
```go
func setupTestSession(t *testing.T, name string) (cleanup func()) {
    t.Helper()
    // Create test session
    return func() {
        // Cleanup
    }
}
```

#### 4. Subtests for Cleanup
```go
t.Run("group", func(t *testing.T) {
    setup := createResource()
    defer cleanup(setup)

    t.Run("subtest1", func(t *testing.T) { ... })
    t.Run("subtest2", func(t *testing.T) { ... })
})
```

---

## Implementation Phases

### Phase 1: Critical Regressions (Week 1)
**Goal**: Prevent known bugs from recurring

- [ ] Add `internal/tmux/send_command_test.go` - SendCommand Enter key regression
- [ ] Add `cmd/csm/new_test.go` - /rename before /csm-assoc
- [ ] Add `internal/manifest/uuid_test.go` - Proper UUID generation (not session-{name})

**Success Metric**: Known regression bugs have failing tests

### Phase 2: Core Coverage (Week 2-3)
**Goal**: Reach 60%+ coverage on critical packages

- [ ] Expand `internal/tmux/` tests to 80%+
- [ ] Add `cmd/csm/` integration tests for main commands
- [ ] Expand `internal/session/` tests to 75%+
- [ ] Add tests for 0% coverage packages (cli, discovery, debug)

**Success Metric**: No critical package below 60%

### Phase 3: Comprehensive Coverage (Week 4-5)
**Goal**: Reach 80%+ overall coverage

- [ ] Improve medium-coverage packages
- [ ] Add E2E tests with testscript
- [ ] Add performance regression benchmarks
- [ ] Document testing patterns in CONTRIBUTING.md

**Success Metric**: Overall coverage >80%, all packages >50%

### Phase 4: CI/CD Integration (Week 6)
**Goal**: Automated testing in CI pipeline

- [ ] Add GitHub Actions workflow for tests
- [ ] Add coverage reporting (codecov/coveralls)
- [ ] Add race detector (`go test -race`)
- [ ] Add benchmark comparison in PRs

**Success Metric**: All PRs automatically tested

---

## Best Practices from Research

### From Go Community
1. **Table-driven tests** for comprehensive case coverage
2. **testdata directories** for fixtures and golden files
3. **Interface-based mocking** for dependency injection
4. **Subtests with t.Run()** for organization
5. **t.Parallel()** for independent tests

### From CLI Projects (kubectl, gh, helm)
1. **testscript** for CLI command testing
2. **Re-exec pattern** for subprocess behavior
3. **Programmatic Cobra execution** for unit tests
4. **Golden files** for output validation
5. **Coverage for integration tests** (`go build -cover`)

### AGM-Specific
1. **Tmux mocking** - Mock tmux socket interactions for unit tests
2. **Filesystem isolation** - Use t.TempDir() for manifest tests
3. **Time-sensitive tests** - Mock time.Now() for timeout tests
4. **Concurrent tests** - Test lock contention scenarios

---

## Success Metrics

### Quantitative
- [ ] Overall coverage >80%
- [ ] No package <50% coverage
- [ ] Critical packages (cmd/csm, internal/tmux, internal/session) >75%
- [ ] All regression bugs have tests
- [ ] CI passes on all PRs

### Qualitative
- [ ] Tests catch real bugs (not just line coverage)
- [ ] Tests are maintainable (clear, concise, well-organized)
- [ ] Tests run fast (<5s for unit, <30s for integration, <2min for E2E)
- [ ] New contributors can understand test patterns
- [ ] Regression prevention confidence

---

## Next Steps

1. **Review and approve this plan** with maintainers
2. **Start Phase 1** - Regression tests for known bugs
3. **Set up testing infrastructure** - testscript, mocks, helpers
4. **Iterate on coverage** - Measure weekly progress
5. **Document patterns** - Update CONTRIBUTING.md with testing guide

---

**Document Version**: 1.0
**Last Updated**: 2026-01-13
**Owner**: AGM Testing Initiative
