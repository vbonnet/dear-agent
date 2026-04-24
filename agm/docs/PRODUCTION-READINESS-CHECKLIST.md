# Production Readiness Checklist - AGM Gemini CLI Adapter

**Project**: Claude Session Manager - Gemini CLI Integration
**Date**: 2026-03-11
**Phase**: 4 (E2E Validation)
**Bead**: scheduling-infrastructure-consolidation-1o2

---

## Overview

This checklist validates production readiness for the Gemini CLI adapter integration into AGM. All items must be complete before advancing to Phase 5 (merge to main).

---

## 1. Security Audit

### 1.1 API Key Handling

- [ ] **Environment Variable**: API keys read from `GEMINI_API_KEY` environment variable only
- [ ] **No Hardcoded Secrets**: Grep codebase for hardcoded API keys/secrets
  ```bash
  grep -r "GEMINI_API_KEY\s*=\s*\"[^\"]*\"" --include="*.go"
  ```
- [ ] **Session Store**: API keys NOT persisted to session metadata
- [ ] **Command Arguments**: API keys NOT passed as CLI arguments (process list exposure)
- [ ] **Test Files**: Test API keys use placeholder values, not real keys

**Validation**:
```bash
# Search for potential secret leaks
cd ./agm
grep -r "api.*key" --include="*.go" -i | grep -v "GEMINI_API_KEY" | grep -v "test" | grep -v "comment"
```

### 1.2 Logging Security

- [ ] **No Secrets in Logs**: Log statements never output API keys or sensitive data
- [ ] **Redaction**: Sensitive fields redacted in structured logs
- [ ] **Error Messages**: Error messages don't leak credentials or internal paths
- [ ] **Debug Mode**: Debug logging doesn't expose secrets

**Validation**:
```bash
# Check log statements
grep -r "log\." --include="*.go" internal/agent/gemini* | grep -v "test" | head -50
```

### 1.3 File Permissions

- [ ] **Session Metadata**: Session files created with 0600 (owner read/write only)
- [ ] **History Files**: Conversation history protected from other users
- [ ] **Temp Files**: Temporary files cleaned up and properly protected
- [ ] **Config Files**: Configuration files have secure permissions

**Validation**:
```bash
# Check for file creation with unsafe permissions
grep -r "os.Create\|os.WriteFile\|os.OpenFile" internal/agent/gemini* --include="*.go" -A 2
```

---

## 2. Error Handling Completeness

### 2.1 Error Path Coverage

- [ ] **All Returns Checked**: Every function call checks error returns
- [ ] **No Silent Failures**: Errors logged or propagated, never silently ignored
- [ ] **Context Propagation**: Error context preserved through call stack
- [ ] **User-Facing Messages**: User-friendly error messages (no stack traces to users)

**Validation**:
```bash
# Find unchecked errors
go test -C ./agm ./internal/agent/... -v 2>&1 | grep "unchecked"
```

### 2.2 Error Recovery

- [ ] **Session Failures**: Session creation failures don't leak resources
- [ ] **Network Errors**: Transient network failures handled gracefully
- [ ] **API Errors**: Gemini API errors surfaced with actionable messages
- [ ] **Timeout Handling**: Operations timeout appropriately (no infinite hangs)

**Validation**:
```bash
# Review timeout values
grep -r "time.After\|context.WithTimeout\|time.Sleep" internal/agent/gemini* --include="*.go"
```

### 2.3 Edge Cases

- [ ] **Empty Input**: Functions handle empty strings, nil pointers, zero values
- [ ] **Invalid UUID**: Invalid session UUIDs handled without panics
- [ ] **Missing Files**: Missing history/config files handled gracefully
- [ ] **Race Conditions**: Concurrent access handled (locks, atomic operations)

**Validation**: Run race detector
```bash
go test -C ./agm -race ./internal/agent/...
```

---

## 3. Logging and Observability

### 3.1 Structured Logging

- [ ] **Log Levels**: Appropriate log levels (ERROR, WARN, INFO, DEBUG)
- [ ] **Structured Fields**: Logs use structured fields (not string concatenation)
- [ ] **Consistency**: Log format consistent with Claude adapter
- [ ] **Contextual Info**: Logs include session ID, operation, timestamp

**Validation**:
```bash
# Check logging patterns
grep -r "log\." internal/agent/gemini_cli_adapter.go --include="*.go" | head -30
```

### 3.2 Metrics and Instrumentation

- [ ] **Operation Timing**: Key operations logged with duration
- [ ] **Success/Failure Rates**: Session creation, command execution tracked
- [ ] **Resource Usage**: Sessions created/terminated counts available
- [ ] **Health Checks**: Adapter health status queryable

**Validation**: Review adapter interface implementation
```bash
grep -r "GetSessionStatus\|GetHealth" internal/agent/gemini_cli_adapter.go
```

### 3.3 Debug Support

- [ ] **Verbose Mode**: Debug mode available for troubleshooting
- [ ] **State Inspection**: Session state dumpable for debugging
- [ ] **Trace IDs**: Operations traceable through logs
- [ ] **Error Context**: Errors include enough context for diagnosis

---

## 4. Performance Benchmarks

### 4.1 Session Operations

**Target**: Session creation < 5s, commands < 1s

- [ ] **CreateSession**: Measured < 5s on average
  ```bash
  time agm session new --harness gemini-cli test-perf-session
  ```

- [ ] **ResumeSession**: Measured < 3s on average

- [ ] **TerminateSession**: Measured < 2s on average

**Validation**: Run benchmark tests
```bash
go test -C ./agm -bench=. -benchtime=10s ./test/...
```

### 4.2 Command Execution

**Target**: Commands execute in < 1s

- [ ] **CommandSetDir**: < 500ms average
- [ ] **CommandClearHistory**: < 200ms average
- [ ] **CommandSetSystemPrompt**: < 300ms average
- [ ] **CommandRename**: < 500ms average

**Validation**: See Task 4.1 E2E tests (command load testing)

### 4.3 Scalability

- [ ] **Concurrent Sessions**: 10+ concurrent Gemini sessions stable
- [ ] **Large History**: 1000+ message history retrievable
- [ ] **Memory Usage**: No memory leaks during long-running sessions
- [ ] **File Descriptors**: No file descriptor leaks

**Validation**:
```bash
# Check for resource leaks
go test -C ./agm ./test/integration/... -run TestConcurrent -v
```

---

## 5. Resource Cleanup

### 5.1 Tmux Session Management

- [ ] **No Leaked Sessions**: Terminated sessions cleaned from tmux
- [ ] **Session List**: `tmux ls` shows only active sessions
- [ ] **Orphan Detection**: Orphaned tmux sessions detected and cleaned
- [ ] **Naming Conflicts**: Unique tmux session names prevent collisions

**Validation**:
```bash
# Before test
tmux ls | wc -l

# Run session lifecycle test
go test -C ./agm ./test/integration/lifecycle/... -run TestSessionLifecycle -v

# After test - count should be same
tmux ls | wc -l
```

### 5.2 Temporary Files

- [ ] **Ready Files**: `~/.agm/ready-*` files cleaned after session termination
- [ ] **Temp Directories**: Test temp directories removed after tests
- [ ] **Lock Files**: Lock files released and removed appropriately
- [ ] **PID Files**: Process ID files cleaned up

**Validation**:
```bash
# Check for leftover files
ls -la ~/.agm/ready-* 2>/dev/null | wc -l  # Should be 0 after cleanup
```

### 5.3 Process Management

- [ ] **No Zombie Processes**: Terminated Gemini CLI processes fully cleaned
- [ ] **Signal Handling**: SIGTERM/SIGINT handled gracefully
- [ ] **Child Processes**: All child processes terminated with parent
- [ ] **Resource Limits**: Sessions respect system resource limits

**Validation**:
```bash
# Check for zombie processes
ps aux | grep gemini | grep defunct
```

---

## 6. Documentation Complete and Accurate

### 6.1 Living Documentation Updated

- [x] **AGENT-COMPARISON.md**: Gemini features accurately documented
  - File: `docs/AGENT-COMPARISON.md`
  - Verified: Phase 3 completion

- [x] **gemini-parity-analysis.md**: Parity score updated to 94/100
  - File: `docs/gemini-parity-analysis.md`
  - Verified: Line 51, 473

- [x] **SPEC.md**: Gemini CLI capabilities documented
  - File: `SPEC.md`
  - Verified: Phase 3 completion

- [x] **ARCHITECTURE.md**: Gemini CLI integration patterns documented
  - File: `docs/ARCHITECTURE.md`
  - Verified: Phase 3 completion

### 6.2 User Guide

- [x] **gemini-cli.md**: Comprehensive user guide created
  - File: `docs/agents/gemini-cli.md`
  - Verified: 700+ lines, installation, features, troubleshooting

- [x] **EXAMPLES.md**: 50+ Gemini-specific examples added
  - File: `docs/EXAMPLES.md`
  - Verified: Phase 3 completion

### 6.3 ADRs (Architecture Decision Records)

- [x] **ADR-001**: Multi-agent architecture updated with Gemini
- [x] **ADR-002**: Command translation documented
- [x] **ADR-011**: Gemini CLI vs API adapter strategy
  - File: `docs/adr/ADR-011-gemini-cli-adapter-strategy.md`
  - Verified: Created in Phase 3

### 6.4 Code Documentation

- [ ] **Godoc Comments**: All public functions have godoc comments
- [ ] **Internal Comments**: Complex logic explained inline
- [ ] **Interface Compliance**: Interface implementations documented
- [ ] **TODO/FIXME**: No unresolved TODO/FIXME comments

**Validation**:
```bash
# Check for missing godoc
go doc -all github.com/vbonnet/dear-agent/agm/internal/agent | grep "GeminiCLI"
```

---

## 7. All Tests Passing

### 7.1 Unit Tests

- [ ] **Agent Package**: All unit tests pass
  ```bash
  go test -C ./agm ./internal/agent/... -v
  ```

- [ ] **Coverage**: Unit test coverage ≥ 80%
  ```bash
  go test -C ./agm ./internal/agent/... -coverprofile=coverage.out
  go tool cover -func=coverage.out
  ```

### 7.2 Integration Tests

- [ ] **Gemini CLI Integration**: Real Gemini CLI tests pass
  ```bash
  GEMINI_API_KEY=xxx go test -C ./agm ./test/integration/... -run TestGeminiCLI -v
  ```

- [ ] **Agent Parity Suite**: All parity tests pass (4 agents)
  ```bash
  go test -C ./agm ./test/integration/... -tags integration -v
  ```

### 7.3 BDD Tests

- [ ] **Session Lifecycle**: All scenarios pass for all 4 agents
  ```bash
  go test -C ./agm ./test/bdd/... -v
  ```

- [ ] **18 Feature Files**: All BDD features verified

### 7.4 E2E Tests

- [x] **E2E Test Suite Created**: `test/e2e/gemini_phase4_e2e_test.go`
  - Multi-session workflow
  - Long-running session (1M token advantage)
  - Command execution under load
  - Cross-agent compatibility (Claude ↔ Gemini)
  - Crash recovery

- [ ] **E2E Tests Pass**: All 5 scenarios pass
  ```bash
  GEMINI_API_KEY=xxx go test -C ./agm ./test/e2e/... -tags e2e -v -run Phase4
  ```

---

## 8. Additional Production Checks

### 8.1 Code Quality

- [ ] **Linting**: `golangci-lint` passes with no errors
  ```bash
  golangci-lint run ./internal/agent/...
  ```

- [ ] **Formatting**: Code formatted with `gofmt`
  ```bash
  gofmt -l internal/agent/gemini* | wc -l  # Should be 0
  ```

- [ ] **Vet**: `go vet` passes
  ```bash
  go vet ./internal/agent/...
  ```

### 8.2 Dependency Management

- [ ] **Go Modules**: `go.mod` up to date
- [ ] **Vulnerability Scan**: No known vulnerabilities
  ```bash
  go list -json -m all | nancy sleuth
  ```
- [ ] **License Compliance**: All dependencies have compatible licenses

### 8.3 Backward Compatibility

- [ ] **Existing Features**: Claude adapter functionality unchanged
- [ ] **API Compatibility**: No breaking changes to AGM CLI interface
- [ ] **Migration Path**: Users can switch between agents seamlessly
- [ ] **Data Format**: Session metadata format compatible

---

## Completion Criteria

All checkboxes above must be checked (✓) before Phase 4 completion.

**Validation Gate Requirements** (from ROADMAP):
- All E2E tests passing ✓ (when complete)
- Production readiness checklist complete ✓ (when all items checked)
- No critical bugs ✓ (when validation complete)
- Beta feedback incorporated ✓ (Task 4.3)

---

## Execution Notes

**Started**: 2026-03-11
**Status**: In Progress
**Assignee**: Claude Sonnet 4.5 (agm-gemini-parity swarm)

**Next Actions**:
1. Execute each checklist item sequentially
2. Document findings and fixes
3. Update this document with validation results
4. Advance to Task 4.3 (Beta Testing) after completion

---

## References

- ROADMAP: `swarm/agm-gemini-parity/ROADMAP.md`
- Gemini CLI Adapter: `internal/agent/gemini_cli_adapter.go`
- Test Suite: `test/integration/gemini_cli_integration_test.go`
- E2E Tests: `test/e2e/gemini_phase4_e2e_test.go`
- Parity Analysis: `docs/gemini-parity-analysis.md`

---

**End of Checklist**
