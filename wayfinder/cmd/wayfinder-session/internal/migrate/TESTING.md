# Migration Package Testing Guide

This document describes the testing strategy and test coverage for the Wayfinder V1→V2 migration package.

## Test Structure

The test suite is organized into 4 files:

1. **converter_test.go** - Core conversion logic tests
2. **integration_test.go** - End-to-end migration workflows
3. **fixtures_test.go** - Real-world V1 file scenarios
4. **edge_cases_test.go** - Edge cases and boundary conditions

## Test Coverage Goals

Target: **>70% code coverage** (as specified in acceptance criteria)

### Current Coverage (by file)

- `converter.go` - Core conversion logic
- `report.go` - Migration reporting
- `doc.go` - Package documentation (N/A for coverage)

## Running Tests

### Run All Tests

```bash
cd cmd/wayfinder-session/internal/migrate
go test -v
```

### Run with Coverage

```bash
go test -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run Specific Test Suite

```bash
# Converter tests only
go test -v -run TestV1ToV2

# Integration tests only
go test -v -run TestMigration

# Edge cases only
go test -v -run TestEdgeCases
```

### Run with Race Detection

```bash
go test -v -race
```

## Test Categories

### 1. Phase Mapping Tests

Tests individual V1→V2 phase mappings.

**File**: `converter_test.go`

**Tests**:
- `TestV1ToV2PhaseMapping` - Validates all 13 phase mappings
  - W0 → W0 (unchanged)
  - D1-D4 → D1-D4 (unchanged)
  - S4 → D4 (merged)
  - S5 → S6 (merged)
  - S6 → S6 (unchanged)
  - S7 → S7 (unchanged)
  - S8 → S8 (BUILD phase)
  - S9 → S8 (merged - validation)
  - S10 → S8 (merged - deployment)
  - S11 → S11 (unchanged)

**Coverage**: 100% of phase mapping logic

### 2. Basic Field Conversion Tests

Tests that basic V1 fields are correctly converted to V2.

**File**: `converter_test.go`

**Tests**:
- `TestConvertV1ToV2_BasicFields` - Validates:
  - Schema version conversion (1.0 → 2.0)
  - Project name derivation
  - Current phase mapping
  - Status mapping
  - Timestamp preservation
  - Phase history creation

**Coverage**: All required V2 fields

### 3. Phase Merging Tests

Tests the consolidation of multiple V1 phases into single V2 phases.

**File**: `converter_test.go`

**Tests**:
- `TestConvertV1ToV2_PhaseMerging` - Validates:
  - S4 → D4 merge (stakeholder_approved field)
  - S5 → S6 merge (research_notes field)
  - S8/S9/S10 → S8 merge (validation_status, deployment_status)
  - Merged phase metadata
  - Timestamp consolidation

**Coverage**: All merge scenarios

### 4. Status Mapping Tests

Tests V1 to V2 status value conversion.

**File**: `converter_test.go`

**Tests**:
- `TestConvertV1ToV2_StatusMapping` - Validates:
  - in_progress → in-progress
  - completed → completed
  - abandoned → abandoned
  - blocked → blocked
  - obsolete → abandoned

**Coverage**: All status values

### 5. Validation Tests

Tests input validation and error handling.

**File**: `converter_test.go`

**Tests**:
- `TestConvertV1ToV2_ValidationChecks` - Validates:
  - Nil input rejection
  - Missing schema version
  - Wrong schema version
  - Missing required fields
  - Error message accuracy

**Coverage**: All validation paths

### 6. Dry-Run Tests

Tests preview mode without file modification.

**File**: `converter_test.go`

**Tests**:
- `TestConvertV1ToV2_DryRun` - Validates:
  - Dry-run flag is respected
  - V2 object is generated
  - No side effects occur

**Coverage**: Dry-run code path

### 7. Data Preservation Tests

Tests that no V1 data is lost during conversion.

**File**: `converter_test.go`

**Tests**:
- `TestConvertV1ToV2_PreserveAllData` - Validates:
  - All timestamps preserved
  - All phase data preserved
  - Completion dates preserved
  - No data loss occurs

**Coverage**: Data preservation guarantees

### 8. Integration Tests

Tests complete migration workflows.

**File**: `integration_test.go`

**Tests**:
- `TestMigrationIntegration` - End-to-end flow:
  1. Create V1 status file
  2. Read V1 status
  3. Convert to V2
  4. Write V2 status
  5. Read V2 back
  6. Verify round-trip integrity

- `TestMigrationWithAllPhases` - Complete 13-phase migration:
  - All phases present
  - Correct merging
  - Phase metadata preservation

- `TestMigrationPreservesTimestamps` - Timestamp accuracy:
  - created_at = started_at
  - completion_date preserved
  - Phase timestamps preserved

**Coverage**: Complete workflows

### 9. Real-World Scenario Tests

Tests migration with realistic V1 files.

**File**: `fixtures_test.go`

**Tests**:
- `TestWithRealV1File` - Real YAML file format
- `TestMigrationWithCompletedProject` - Completed project
- `TestMigrationWithBlockedProject` - Blocked project
- `TestMigrationWithSkippedPhases` - Skipped phases
- `TestMigrationWithMinimalV1` - Minimal V1 data

**Coverage**: Real-world use cases

### 10. Edge Case Tests

Tests boundary conditions and unusual inputs.

**File**: `edge_cases_test.go`

**Tests**:
- `TestEdgeCases_EmptyPhases` - No phases
- `TestEdgeCases_OnlyMergedPhases` - Only S4, S5, S9, S10
- `TestEdgeCases_MultipleBuildPhases` - S8+S9+S10 merging
- `TestEdgeCases_OutOfOrderPhases` - Non-chronological input
- `TestEdgeCases_DuplicatePhases` - Duplicate phase entries
- `TestEdgeCases_AllProjectTypes` - All project types
- `TestEdgeCases_AllRiskLevels` - All risk levels
- `TestEdgeCases_PreserveSessionID` - Session ID preservation
- `TestEdgeCases_VeryLongProjectPath` - Long paths
- `TestEdgeCases_ZeroTimestamps` - Nil/zero timestamps
- `TestEdgeCases_CustomProjectName` - Custom metadata

**Coverage**: Edge cases and error paths

## Test Utilities

### Helper Functions

Common test utilities defined in test files:

```go
// Create time pointer
func timePtr(t time.Time) *time.Time

// Check string contains substring
func contains(s, substr string) bool

// Create realistic V1 status
func createRealisticV1Status() *status.Status

// Create complete V1 status with all phases
func createCompleteV1Status() *status.Status

// Create all completed phases
func createAllCompletedPhases() []status.Phase
```

## Manual Testing Checklist

In addition to automated tests, perform these manual tests:

### Pre-Migration

- [ ] V1 file exists and is readable
- [ ] V1 schema version is "1.0"
- [ ] All required V1 fields present
- [ ] Phase names are valid V1 phases

### Migration Process

- [ ] Dry-run shows accurate preview
- [ ] Backup file is created (if enabled)
- [ ] V2 file is written successfully
- [ ] Original V1 file preserved (backup mode)

### Post-Migration

- [ ] V2 schema version is "2.0"
- [ ] All V1 phases mapped to V2
- [ ] Merged phases have correct metadata
- [ ] Timestamps preserved accurately
- [ ] No data lost during conversion
- [ ] V2 file validates correctly

### CLI Commands

- [ ] `wayfinder-session migrate . --dry-run`
- [ ] `wayfinder-session migrate /path/to/project`
- [ ] `wayfinder-session migrate . --verbose`
- [ ] `wayfinder-session migrate . --project-type feature`
- [ ] `wayfinder-session migrate . --risk-level XL`
- [ ] `wayfinder-session migrate . --preserve-session-id`
- [ ] `wayfinder-session migrate . --backup=false`

## Performance Testing

### Benchmarks

```bash
# Run benchmarks
go test -bench=. -benchmem
```

Expected performance:
- Small projects (W0-D3): <10ms
- Medium projects (W0-S8): ~20ms
- Complete projects (W0-S11): ~30ms

### Memory Usage

Expected memory usage:
- Small projects: <1MB
- Medium projects: ~2MB
- Large projects: ~5MB

## Regression Testing

When adding new features, ensure:

1. All existing tests still pass
2. Coverage doesn't decrease
3. No breaking changes to public API
4. Migration output is backward compatible

## Continuous Integration

### GitHub Actions Workflow

```yaml
- name: Run migration tests
  run: |
    cd cmd/wayfinder-session/internal/migrate
    go test -v -coverprofile=coverage.out
    go tool cover -func=coverage.out
```

### Coverage Requirements

- Minimum coverage: 70%
- Target coverage: 85%
- Critical paths: 100%

## Known Limitations

1. **Phase File Migration**: Currently only migrates WAYFINDER-STATUS.md
   - Future: Migrate phase-specific files (D1.md, S8.md, etc.)

2. **Task Migration**: V1 tasks not yet supported
   - Future: Migrate ROADMAP.md tasks to V2 roadmap

3. **Test File Generation**: Not yet implemented
   - Future: Auto-generate TESTS.outline and TESTS.feature

## Future Test Enhancements

1. **Fuzz Testing**: Random V1 input generation
2. **Property-Based Testing**: Invariant checking
3. **Load Testing**: Large project migration (1000+ phases)
4. **Concurrency Testing**: Parallel migrations
5. **Rollback Testing**: V2→V1 conversion

## Troubleshooting Tests

### Test Failures

If tests fail:

1. Check Go version (requires 1.21+)
2. Verify dependencies: `go mod verify`
3. Clean test cache: `go clean -testcache`
4. Run with verbose output: `go test -v`

### Coverage Issues

If coverage is low:

1. Identify uncovered code: `go tool cover -html=coverage.out`
2. Add tests for uncovered paths
3. Consider if code is testable
4. Refactor if needed

### Flaky Tests

If tests are flaky:

1. Check for time-dependent logic
2. Look for race conditions: `go test -race`
3. Ensure deterministic test data
4. Add proper test isolation

## Documentation

All tests should include:

- Clear test name describing what is tested
- Table-driven tests for multiple scenarios
- Comments explaining complex assertions
- Error messages that aid debugging

Example:

```go
func TestV1ToV2PhaseMapping(t *testing.T) {
    tests := []struct {
        name           string
        v1Phase        string
        expectedV2     string
        preserveFields map[string]bool
    }{
        {
            name:       "W0 maps to W0",
            v1Phase:    "W0",
            expectedV2: status.PhaseV2W0,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := mapV1PhaseToV2(tt.v1Phase)
            if got != tt.expectedV2 {
                t.Errorf("mapV1PhaseToV2(%q) = %q, want %q",
                    tt.v1Phase, got, tt.expectedV2)
            }
        })
    }
}
```

## Review Checklist

Before merging:

- [ ] All tests pass
- [ ] Coverage >70%
- [ ] No race conditions
- [ ] Documentation updated
- [ ] Examples verified
- [ ] Performance acceptable
- [ ] Manual testing complete
