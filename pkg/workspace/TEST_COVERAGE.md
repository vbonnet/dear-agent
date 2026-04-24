# Workspace Detection Library - Test Coverage

Comprehensive unit test suite for the workspace detection library.

## Test Files

### 1. `detector_test.go` - Workspace Detection Priority System

Tests the 6-priority detection algorithm and core detector functionality.

#### Priority Level Tests
- **Priority 1 (Explicit Flag)**: Tests explicit `--workspace` flag (highest priority)
  - Valid workspace selection
  - Non-existent workspace error handling

- **Priority 2 (Environment Variable)**: Tests `WORKSPACE` env var detection
  - Valid env var workspace selection
  - Invalid workspace from env var error handling
  - Custom env var name support

- **Priority 3 (Auto-detect from PWD)**: Tests automatic detection from current directory
  - Detection from workspace root
  - Detection from nested subdirectories (multiple levels deep)
  - Multiple workspace configurations
  - Disabled workspace filtering

- **Priority 4 (Default Workspace)**: Tests default workspace fallback
  - Using default when no other method applies

- **Priority 5 (Interactive Prompt)**: Tested in `interactive_test.go`

- **Priority 6 (Error)**: Tests error when no workspace can be determined
  - `ErrNoWorkspaceFound` when all methods fail

#### Additional Detector Tests
- **matchWorkspace()**: Tests workspace path matching logic
  - Exact path matches
  - Subdirectory matches
  - Deep nesting
  - Non-matches (different paths, parent dirs, similar prefixes)

- **GetWorkspace()**: Tests workspace retrieval by name
  - Found workspaces
  - Not found error (`ErrWorkspaceNotFound`)
  - Disabled workspace error (`ErrWorkspaceNotEnabled`)

- **ListWorkspaces()**: Tests listing all workspaces
  - Returns all workspaces including disabled ones
  - Returns copy (mutation safety)

- **GetConfig()**: Tests config retrieval
  - Returns copy of configuration

- **Priority Order Integration**: Tests that priorities are respected
  - Flag > Env > Auto-detect > Default
  - Multiple detection methods active simultaneously

**Total Tests**: 18+ test functions with ~50+ test cases

---

### 2. `config_test.go` - Configuration Loading and Validation

Tests YAML config parsing, validation, and path expansion.

#### LoadConfig Tests
- Valid YAML configs
- Tilde (`~`) expansion in paths
- Invalid YAML syntax error handling
- File not found error handling
- Complex configs with multiple workspaces
- Tool-specific settings and custom fields

#### SaveConfig Tests
- Saving valid configurations
- Creating parent directories automatically
- Validation before saving
- File permission verification (0600)

#### ValidateConfig Tests
- **Missing Fields**: Nil config, empty workspaces, empty names, empty roots
- **Duplicate Names**: Detects duplicate workspace names
- **Version Validation**: Tests version 0, 1, 2, 99 (only v1 valid)
- **No Enabled Workspaces**: Requires at least one enabled workspace
- **Invalid Default Workspace**: Default workspace must exist in workspaces list

#### ExpandPaths Tests
- **Tilde Expansion**: `~/workspace` → `$HOME/workspace`
- **Environment Variable Expansion**: `$HOME/workspace`, `$CUSTOM_VAR/path`
- **Output Dir Handling**:
  - Expansion of custom output_dir
  - Default to root when output_dir is empty
- **Relative to Absolute**: Converts `./relative` paths to absolute
- **Invalid Path Handling**: Empty root paths

#### Utility Function Tests
- **GetDefaultConfigPath()**: Tests default config location generation
- **GenerateDefaultConfig()**: Tests default config creation

**Total Tests**: 20+ test functions with ~40+ test cases

---

### 3. `paths_test.go` - Path Manipulation and Validation

Tests all path utility functions with comprehensive edge cases.

#### ExpandHome Tests
- Tilde alone: `~` → `/home/user`
- Tilde with path: `~/workspace/src`
- No tilde: `/absolute/path`, `relative/path` (unchanged)
- Empty string handling
- Edge cases: `~/`, `~/.`, `~//path`

#### NormalizePath Tests
- **Absolute Paths**: No change
- **Relative Paths**: Converted to absolute
- **Tilde Expansion**: `~/workspace` → absolute path
- **Environment Variables**: `$HOME`, `$CUSTOM_VAR` expansion
- **Path Cleaning**:
  - Remove `./` prefixes
  - Resolve `..` parent references
  - Remove trailing slashes
- **Complex Transformations**: Combined `~/$VAR/../$VAR/./src`
- **Root Path**: `/` handling
- **Benchmarks**: Performance testing

#### IsSubpath Tests
- **Exact Matches**: `/foo/bar` vs `/foo/bar`
- **Direct Children**: `/foo/bar` vs `/foo/bar/baz`
- **Deep Nesting**: `/foo` vs `/foo/bar/baz/qux/deep/nested`
- **Non-Subpaths**:
  - Different paths
  - Parent of parent (reversed relationship)
  - Similar prefixes: `/foo` vs `/foobar` (boundary case)
- **Tilde Expansion**: Works with `~` paths
- **Relative Paths**: Normalizes before comparison
- **Trailing Slashes**: Handles correctly
- **Dots in Paths**: Normalizes `.` and `..`
- **Symlink Behavior**: Documents behavior (platform-specific)
- **Benchmarks**: Performance testing

#### ValidateAbsolutePath Tests
- **Absolute Paths**: Valid
- **Tilde Paths**: Valid (expanded to absolute)
- **Relative Paths**: Current behavior (converts to absolute)
- **Empty String**: Error
- **Environment Variables**: `$HOME/workspace` validation

**Total Tests**: 25+ test functions with ~60+ test cases + 2 benchmarks

---

### 4. `interactive_test.go` - Interactive Prompts

Tests TTY-based interactive workspace selection and confirmation prompts.

#### PromptWorkspace Tests
- **Valid Selections**:
  - Select 1st, 2nd, 3rd workspace
  - Input with spaces/newlines
- **Invalid Then Valid**: Recovery from errors
  - Invalid numbers (0, 5, 999, -1)
  - Text input
  - Empty input
  - Multiple retries before valid selection
- **Non-TTY Mode**: Error when `isTTY = false`
- **No Enabled Workspaces**: `ErrNoEnabledWorkspaces` error
- **Disabled Workspace Filtering**: Only enabled workspaces shown
- **Output Format**: Verifies prompt message format
- **Edge Cases**:
  - Single workspace
  - Many workspaces (20+)
  - Long workspace names/paths
  - Boundary conditions (min/max selection)
- **Whitespace Handling**: Leading/trailing spaces, tabs
- **Read Errors**: EOF handling

#### PromptConfirm Tests
- **Yes Responses**: `y`, `Y`, `yes`, `YES`, `Yes` (case-insensitive)
- **No Responses**: `n`, `N`, `no`, `NO`, empty, random text, numbers
- **Non-TTY Mode**: Error when not in TTY
- **Output Format**: Verifies `[y/N]` prompt format
- **Case Sensitivity**: Comprehensive case testing
- **Read Errors**: EOF handling

#### Helper Tests
- **NewPrompter()**: Default prompter creation
- **NewPrompterWithIO()**: Custom IO for testing

**Total Tests**: 20+ test functions with ~70+ test cases

---

## Test Data Files

Located in `testdata/` directory:

- `valid_config.yaml` - Multi-workspace valid config
- `invalid_version.yaml` - Unsupported version
- `duplicate_names.yaml` - Duplicate workspace names
- `no_workspaces.yaml` - Empty workspace list
- `minimal_config.yaml` - Minimal valid config
- `with_env_vars.yaml` - Environment variable usage
- `README.md` - Test data documentation

## Testing Best Practices Used

1. **Table-Driven Tests**: Used extensively for testing multiple scenarios
2. **Temporary Directories**: `t.TempDir()` for filesystem tests (auto-cleanup)
3. **Mocked I/O**: Custom stdin/stdout for interactive tests
4. **Test Isolation**: Each test is independent, no shared state
5. **Error Validation**: Tests both success and failure cases
6. **Edge Cases**: Comprehensive boundary condition testing
7. **Benchmarks**: Performance testing for critical paths
8. **Documentation**: Clear test names describing what is tested

## Running Tests

```bash
# Run all tests
cd pkg/workspace
go test -v ./...

# Run specific test file
go test -v -run TestDetect

# Run with coverage
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out

# Run benchmarks
go test -bench=. -benchmem
```

## Coverage Summary

| File | Functions Tested | Coverage Areas |
|------|-----------------|----------------|
| `detector.go` | All | Priority detection, workspace matching, listing, retrieval |
| `config.go` | All | Loading, saving, validation, path expansion |
| `paths.go` | All | Path normalization, home expansion, subpath checking |
| `interactive.go` | All | Workspace prompts, confirmation, TTY handling |

**Estimated Total Test Cases**: 220+
**Estimated Code Coverage**: 90%+ (when tests pass)

## Known Limitations

1. **Symlink Tests**: Platform-specific behavior, may skip on some systems
2. **TTY Detection**: Cannot fully test real TTY behavior (uses mocked IO)
3. **Filesystem Tests**: Some tests assume Unix-like filesystem semantics

## Next Steps

1. Run tests to verify all pass
2. Generate coverage report
3. Add integration tests for end-to-end scenarios
4. Add property-based tests for path manipulation (if needed)
5. Consider adding tests for concurrent access patterns
