# CI Pipeline Executor Specification

## Overview

The CI package provides local CI/CD pipeline execution for AGM sessions using industry-standard tools like nektos/act (GitHub Actions simulator). This enables pre-merge quality gates that prevent bad code from reaching the main branch.

## Goals

1. **Local CI/CD**: Run GitHub Actions workflows locally before pushing
2. **Pre-Merge Gates**: Block merges that would fail CI
3. **Fast Feedback**: Catch issues in seconds, not minutes (vs remote CI)
4. **Reproducible**: Same environment as GitHub Actions
5. **Extensible**: Support multiple executors (act, Dagger, GitLab CI)

## Architecture

### Executor Interface

```go
type PipelineExecutor interface {
    // Execute runs a CI/CD pipeline
    Execute(ctx context.Context, req PipelineRequest) (*PipelineResult, error)

    // Validate checks if the executor is properly configured
    Validate(ctx context.Context) error

    // Name returns the executor identifier
    Name() string
}
```

### Pipeline Request

```go
type PipelineRequest struct {
    WorkflowFile string            // Path to workflow file (.github/workflows/*)
    Event        string            // Trigger event (pull_request, push, etc.)
    WorkingDir   string            // Directory to run pipeline in
    Secrets      map[string]string // Secrets for pipeline
    Variables    map[string]string // Environment variables
    EventPayload interface{}       // Event-specific payload
    OutputCallback func(string)    // Stream output to caller
}
```

### Pipeline Result

```go
type PipelineResult struct {
    Success   bool          // Did pipeline succeed?
    Output    string        // Combined stdout/stderr
    Duration  time.Duration // How long it took
    ExitCode  int           // Process exit code
    Error     error         // Infrastructure error (vs pipeline failure)
}
```

**Key Distinction**:
- `Success = false`: Pipeline ran but tests failed (expected)
- `Error != nil`: Infrastructure failure (act binary missing, etc.)

## Implementations

### ActExecutor (`act/executor.go`)

**Purpose**: Execute GitHub Actions workflows locally using nektos/act

**Binary**: https://github.com/nektos/act

**Features**:
- Runs workflows in Docker containers
- Simulates GitHub Actions environment
- Supports secrets, variables, event payloads
- Streams real-time output

**Critical Enhancement**: Auto-inject `--artifact-server-path`
- **Why**: Without this, artifact uploads fail silently
- **Implementation**: Add flag automatically if not present in user args

### Configuration

```go
type ActExecutor struct {
    ActBinaryPath       string // Path to act binary (default: "act")
    DockerHost          string // Docker daemon URL
    ArtifactServerPath  string // Path for artifact uploads
    DefaultTimeout      time.Duration
}
```

## Usage

### Basic Execution

```go
executor := act.NewActExecutor()

result, err := executor.Execute(ctx, ci.PipelineRequest{
    WorkflowFile: ".github/workflows/test.yml",
    Event:        "pull_request",
    WorkingDir:   "/path/to/repo",
})

if err != nil {
    // Infrastructure error
    log.Fatalf("Failed to run pipeline: %v", err)
}

if !result.Success {
    // Pipeline failed (tests failed, etc.)
    log.Printf("Pipeline failed: %s", result.Output)
}
```

### With Secrets

```go
result, err := executor.Execute(ctx, ci.PipelineRequest{
    WorkflowFile: ".github/workflows/deploy.yml",
    Event:        "push",
    Secrets: map[string]string{
        "ANTHROPIC_API_KEY": os.Getenv("ANTHROPIC_API_KEY"),
        "DEPLOY_TOKEN":      loadToken(),
    },
})
```

### Streaming Output

```go
result, err := executor.Execute(ctx, ci.PipelineRequest{
    WorkflowFile: ".github/workflows/test.yml",
    Event:        "pull_request",
    OutputCallback: func(line string) {
        fmt.Println(line) // Real-time output
    },
})
```

## Error Handling

### Infrastructure Errors

```go
if err != nil {
    var ciErr *ci.Error
    if errors.As(err, &ciErr) {
        switch ciErr.Code {
        case ci.ErrCodeExecutorNotFound:
            // act binary not installed
        case ci.ErrCodeInvalidWorkflow:
            // Workflow file missing or malformed
        case ci.ErrCodeTimeoutExceeded:
            // Pipeline exceeded timeout
        }
    }
}
```

### Pipeline Failures

```go
if !result.Success {
    // Test failure, lint error, build failure, etc.
    // This is NOT an error - pipeline ran successfully
    // but tests/checks failed
}
```

## Integration with Git Hooks

### pre-merge-commit Hook

**Purpose**: Block merges to main if CI fails

**Implementation**: `cmd/agm-hooks/pre-merge-commit/main.go`

**Workflow**:
1. Detect merge to main/master (check `.git/MERGE_HEAD`)
2. Run `act pull_request` to simulate GitHub Actions
3. If pipeline fails, rollback merge via `git reset --merge`
4. If pipeline succeeds, allow merge to proceed

**Installation**:
```bash
# Copy hook to repository
cp pre-merge-commit .git/hooks/
chmod +x .git/hooks/pre-merge-commit

# Test merge
git merge feature-branch
# Hook runs automatically, blocks if CI fails
```

## Performance Characteristics

### ActExecutor

- **Startup**: 1-2s (Docker container launch)
- **Execution**: Depends on workflow (typically 10-60s for tests)
- **Cleanup**: 1s (container teardown)

**Total**: Usually 15-65s for typical test suite

### Comparison to Remote CI

| Aspect | Local (act) | Remote (GitHub Actions) |
|--------|-------------|------------------------|
| Startup | 1-2s | 30-120s (queue + provision) |
| Network | None | Fetch deps every time |
| Caching | Local Docker | Remote cache (slower) |
| Feedback | 15-65s | 2-10 minutes |

**Speedup**: 2-10x faster than remote CI

## Configuration

### AGM Config

```yaml
ci:
  enabled: true
  executor: act
  pre_merge_hook: true
  workflows:
    - .github/workflows/test.yml
    - .github/workflows/lint.yml
```

### Executor Config

```yaml
ci:
  act:
    binary_path: /usr/local/bin/act
    docker_host: unix:///var/run/docker.sock
    artifact_server_path: /tmp/act-artifacts
    timeout: 10m
```

## Testing Strategy

### Unit Tests

- **Mock Executor**: Test logic without running actual pipelines
- **Contract Tests**: Verify all executors implement interface
- **Error Injection**: Test error handling paths

### Integration Tests

- **Real Act**: Execute actual GitHub Actions workflows
- **Secret Passing**: Verify secrets injected correctly
- **Output Streaming**: Verify real-time output callback
- **Timeout Handling**: Verify context cancellation

### Edge Cases

- Missing act binary
- Invalid workflow file
- Docker daemon not running
- Pipeline timeout
- Secret file cleanup failure

## Security Considerations

### Secrets

- **Temporary Files**: Secrets written to temp files with 0600 permissions
- **Cleanup**: Deferred cleanup ensures removal even on error
- **No Logging**: Secrets never logged to stdout/stderr

### Docker

- **Socket Access**: Requires access to Docker daemon
- **Container Isolation**: Pipelines run in isolated containers
- **Privilege**: Act runs as current user (no root required)

## Future Enhancements

### Phase 2

- [ ] Parallel workflow execution
- [ ] Workflow result caching
- [ ] Artifact retention policy

### Phase 3

- [ ] Dagger executor (portable, no Docker required)
- [ ] GitLab CI executor
- [ ] Custom executor plugin system

### Phase 4

- [ ] Distributed execution (run on remote machines)
- [ ] Result aggregation dashboard
- [ ] Cost tracking (Docker resources)

## Related

- **ARCHITECTURE.md**: Detailed executor design
- **ADR-004**: Act vs Dagger Comparison
- **Sandbox Integration**: CI runs in sandbox environment (Phase 2)

## References

- **nektos/act**: https://github.com/nektos/act
- **GitHub Actions**: https://docs.github.com/en/actions
- **Implementation**: `internal/ci/act/executor.go`
