# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Session Communication Commands** (v2.1): New unified command namespace for session interactions
  - `agm session send` - Send messages with sender attribution and audit logging
  - `agm session reject` - Reject permission prompts with custom reasons
  - `agm session recover` - Soft recovery for stuck sessions (ESC/Ctrl-C)
  - `agm session select-option` - Programmatically answer AskUserQuestion prompts
  - **Sender Attribution**: All messages tagged with sender name and unique IDs
  - **Message Logging**: Audit trail in `~/.agm/logs/messages/*.jsonl`
  - **Message Threading**: Support for --reply-to to link related messages
  - **Impact**: Enables automated session orchestration, monitoring, and recovery

### Removed

- **Unused Commands** (v2.2 - Phase 3): Removed based on telemetry analysis (0% usage over 3 days, 484 events)
  - `agm backup` - Manifest backup management (0 uses)
  - `agm deadlock-report` - Deadlock metrics reporting (0 uses)
  - `agm metrics-log` - Manual metrics logging (0 uses)
  - `agm agent list` - List available AI agents (1 use, 0.2%)
  - **Rationale**: Telemetry data showed zero or near-zero usage
  - **Impact**: Reduced binary size, simplified CLI surface area
  - **Migration**: No migration needed - commands had no active users

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

- **`--no-lock` flag**: Removed obsolete workaround flag from all AGM commands
  - **Reason**: Flag was never implemented (defined but unused in code)
  - **Background**: Deadlock between `csm new` and `csm associate` was fixed in commit 262c069 by releasing lock before waiting for ready-file
  - **Impact**: No functional change (flag had no effect)
  - **Migration**: Remove `--no-lock` from any scripts (flag will cause "unknown flag" error if used)
