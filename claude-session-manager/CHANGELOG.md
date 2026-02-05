# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **`csm new` bash prompt false positives**: Fixed race condition causing `/rename` and `/csm-assoc` commands to execute in bash shell instead of Claude
  - **Root cause**: Prompt detector matched bash prompts ("$", ">", "#") in addition to Claude prompt ("❯"), causing `WaitForPromptSimple` to return too early when bash shell appeared briefly during startup
  - **Fix**: Added `containsClaudePromptPattern` that only matches Claude's specific "❯" prompt (Unicode U+276F), excluding all bash prompt patterns
  - **InitSequence improvements**: Added `waitForClaudePrompt` method with 100ms polling to ensure commands are sent to Claude (not bash)
  - **Error handling**: Changed `WaitForPromptSimple` failure from warning to blocking error with session cleanup
  - **Impact**: `csm new` now reliably starts sessions without "command not found" errors for `/rename` and `/csm-assoc`
  - **Commits**: 4ff847f (pattern matcher), b47aa60 (readiness checks), 9c1e6e2 (error handling)

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
