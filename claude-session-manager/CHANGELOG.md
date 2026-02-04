# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Build and Test Failures**: Fixed critical compilation and test issues (oss-lj4)
  - Fixed redundant newline in `fmt.Println` in workflow.go (Go vet error)
  - Fixed template function registration in handoff_prompt.go (template "add" function not defined)
  - Fixed integration test suite to pass required test context parameters
  - Formatted all Go source files using `gofmt` for consistency
  - **Impact**: All tests now pass, clean build without errors

### Improved

- **Code Quality**: Applied Go formatting standards across entire codebase
  - Formatted 20+ files in cmd/csm, internal, and test directories
  - Ensured consistent code style throughout the project
  - All files pass `go vet` and `gofmt` checks

### Removed

- **`--no-lock` flag**: Removed obsolete workaround flag from all CSM commands
  - **Reason**: Flag was never implemented (defined but unused in code)
  - **Background**: Deadlock between `csm new` and `csm associate` was fixed in commit 262c069 by releasing lock before waiting for ready-file
  - **Impact**: No functional change (flag had no effect)
  - **Migration**: Remove `--no-lock` from any scripts (flag will cause "unknown flag" error if used)
