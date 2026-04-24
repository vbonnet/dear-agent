# Integration Test Suite for `engram validate`

## Overview

The integration test suite in `validate_integration_test.go` provides comprehensive end-to-end testing of the `engram validate` command. These tests invoke the actual CLI binary (via `exec.Command`) on real files from the engram repository, testing the complete validation workflow.

## Test Coverage

### Test Functions (13 Integration Tests)

1. **TestIntegration_ValidateRealCorpus**
   - Validates 500+ real files from the engram repository
   - Tests `--all` flag on entire corpus
   - Verifies summary output format
   - **Files**: All validatable files in repo

2. **TestIntegration_EngramValidator**
   - Tests engram validator on real .ai.md files
   - Tests explicit `--type=engram` flag
   - Tests auto-detection for .ai.md files
   - **Files**: Real .ai.md files from engrams/, core/persona/, etc.

3. **TestIntegration_ContentValidator**
   - Tests content validator on core/*.ai.md files
   - Tests explicit `--type=content` flag
   - Tests auto-detection for core/ directory files
   - **Files**: Real core/*.ai.md files

4. **TestIntegration_WayfinderValidator**
   - Tests wayfinder validator on wayfinder-artifact.yaml files
   - Tests explicit `--type=wayfinder` flag
   - **Files**: Real wayfinder-artifact.yaml files

5. **TestIntegration_LinkChecker**
   - Tests link checker on real .ai.md files
   - Tests detection of broken links
   - Tests `--type=linkchecker` flag
   - **Files**: Real .ai.md files + synthetic test file

6. **TestIntegration_YAMLTokenCounter**
   - Tests YAML token counter on real YAML files
   - Tests `--verbose` flag with token counts
   - Tests `--type=yamltokencounter` flag
   - **Files**: Real YAML config files from repo

7. **TestIntegration_RetrospectiveValidator**
   - Tests retrospective validator on real retrospective files
   - Tests auto-detection for *-retrospective.md files
   - Tests `--type=retrospective` flag
   - **Files**: Real S11 retrospective files

8. **TestIntegration_JSONOutput**
   - Tests JSON output format (`--json` flag)
   - Validates JSON structure (ValidationSummary)
   - Tests JSON output with errors
   - **Output**: Structured JSON validation results

9. **TestIntegration_AutoFix**
   - Tests `--fix` flag for auto-fixing issues
   - Tests on copy of real files (non-destructive)
   - Verifies fixes applied reporting
   - **Files**: Copies of real core/*.ai.md files

10. **TestIntegration_PerformanceBenchmark**
    - Validates 500+ files in <10 seconds
    - Measures and reports validation time
    - Performance regression detection
    - **Performance Goal**: <10s for full corpus

11. **TestIntegration_BatchProcessing**
    - Tests `--all` flag on directory
    - Tests batch validation of multiple file types
    - Verifies all files processed
    - **Files**: Synthetic multi-file test directory

12. **TestIntegration_TypeSelection**
    - Tests `--type` flag for each validator type
    - Tests: engram, content, linkchecker, wayfinder, yamltokencounter, retrospective
    - Verifies explicit type selection works correctly
    - **Files**: Synthetic test files

13. **TestIntegration_ErrorHandling**
    - Tests missing file errors
    - Tests empty file handling
    - Tests malformed file handling
    - Tests no arguments error
    - Tests invalid validator type error
    - **Error Scenarios**: Graceful failure handling

### Additional Test Functions (4)

14. **TestIntegration_VerboseOutput**
    - Tests `--verbose` flag output
    - Compares verbose vs normal output
    - **Files**: Real .ai.md files

15. **TestIntegration_MixedFileTypes**
    - Tests validation of mixed file types in one directory
    - Tests that non-validatable files are skipped
    - **Files**: Mixed .ai.md, .yaml, .md files

16. **TestIntegration_ExitCodes**
    - Tests exit code 0 on success
    - Tests exit code 1 on validation errors
    - Verifies proper exit code handling
    - **Exit Codes**: 0 (success), 1 (errors), 2 (crash)

17. **TestIntegration_DirectoryValidation**
    - Tests validating entire directory (not just `--all`)
    - Tests passing directory path as argument
    - **Files**: Synthetic directory with multiple files

## Test Infrastructure

### Test Environment Setup

The `setupValidateTest()` function:
1. Builds the CLI binary (`go build -o /tmp/engram`)
2. Discovers real files from engram repository:
   - .ai.md files (engrams, personas, patterns)
   - YAML files (configs, CI, schemas)
   - core/*.ai.md files (content files)
   - *-retrospective.md files (S11 retrospectives)
3. Collects 500+ file corpus for performance testing

### File Discovery

The `discoverFiles()` function walks the repository tree to find:
- **Patterns**: `**/*.ai.md`, `**/*.yaml`, `**/*retrospective*.md`
- **Exclusions**: vendor/, .git/, node_modules/
- **Limits**: Configurable max files per pattern (for fast tests)

### Test Helpers

- `runValidateCommand()`: Executes CLI with args, returns stdout
- `runValidateCommandSeparateStreams()`: Separate stdout/stderr capture

## Running Tests

### Run All Integration Tests

```bash
cd core
go test -v ./cmd/engram/cmd -run TestIntegration_
```

### Run Specific Test

```bash
cd core
go test -v ./cmd/engram/cmd -run TestIntegration_ValidateRealCorpus
```

### Skip Integration Tests (Short Mode)

```bash
cd core
go test -short ./cmd/engram/cmd
```

All integration tests check `testing.Short()` and skip in short mode.

### Performance Benchmark

```bash
cd core
go test -v ./cmd/engram/cmd -run TestIntegration_PerformanceBenchmark -timeout 30s
```

## Success Criteria

- ✅ All 17+ integration test functions pass
- ✅ 500+ real files validated
- ✅ Performance <10 seconds for full corpus
- ✅ CLI invoked via exec.Command (not library calls)
- ✅ Zero compilation errors
- ✅ Real issues detected in real files
- ✅ All validators tested E2E
- ✅ All CLI flags tested (--all, --type, --json, --verbose, --fix)
- ✅ Error handling tested (missing files, malformed files, etc.)
- ✅ Exit codes correct (0=success, 1=errors, 2=crash)

## Test Isolation

Tests are designed to be non-destructive:
- Use `t.TempDir()` for temporary files
- Copy real files before testing `--fix` flag
- Never modify engram repository files
- Clean up temporary files automatically
- Parallel-safe (can use `t.Parallel()` where applicable)

## Integration with CI

These tests are suitable for CI/CD pipelines:
- Deterministic (same results each run)
- Fast (<30 seconds for full suite)
- Skip in short mode for quick checks
- Clear pass/fail criteria
- Detailed logging on failure

## Real Files Used

The integration tests validate against actual engram repository files:

### .ai.md Files (100+)
- `core/cortex/engrams/workflows/*.ai.md` (S11, D1-D4, S4-S10)
- `core/persona/library/**/*.ai.md` (personas)
- `engrams/patterns/**/*.ai.md` (patterns)
- `engrams/references/**/*.ai.md` (references)

### YAML Files (40+)
- `.github/workflows/*.yml` (CI configs)
- `core/cortex/config/*.yml` (wayfinder configs)
- `config.yaml`, `plugin.yaml` files
- `testdata/**/*.yaml` (test fixtures)

### Core Files (50+)
- `core/**/*.ai.md` (content files requiring token counts)

### Retrospective Files (2+)
- `core/cortex/engrams/workflows/s11-retrospective.ai.md`
- `core/cortex/engrams/workflows/s11-enhanced-retrospective.ai.md`

## Test Output Examples

### Successful Validation
```
Summary:
  Files scanned: 523
  Files validated: 498
  Errors: 0
  Warnings: 3
```

### Validation with Errors
```
invalid.ai.md (engram):
  ❌ [missing_frontmatter]: File missing YAML frontmatter

Summary:
  Files scanned: 1
  Files validated: 1
  Errors: 1
  Warnings: 0
```

### JSON Output
```json
{
  "TotalFiles": 1,
  "FilesValidated": 1,
  "ErrorCount": 1,
  "WarningCount": 0,
  "FixesApplied": 0,
  "Results": [
    {
      "ValidatorType": "engram",
      "FilePath": "invalid.ai.md",
      "Errors": [
        {
          "FilePath": "invalid.ai.md",
          "Line": 0,
          "Type": "missing_frontmatter",
          "Message": "File missing YAML frontmatter"
        }
      ],
      "Warnings": [],
      "FixesApplied": []
    }
  ]
}
```

## Maintenance

When adding new validators:
1. Add test function in `validate_integration_test.go`
2. Test on real files from engram repo
3. Test explicit `--type=<validator>` flag
4. Test auto-detection if applicable
5. Update this documentation

When modifying CLI:
1. Run full integration test suite
2. Check performance benchmark still passes
3. Update tests for new flags/behavior
4. Ensure backward compatibility

## References

- CLI Implementation: `validate.go`
- Unit Tests: `validate_test.go`
- Validators: `core/pkg/validator/`
- Test Pattern: `tokens_estimate_integration_test.go` (reference implementation)
