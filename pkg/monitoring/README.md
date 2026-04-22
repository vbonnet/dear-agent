# Sub-Agent Monitoring

Monitor and validate sub-agent executions to detect fake implementations and ensure quality.

## Quick Start

```go
import "github.com/vbonnet/engram/core/pkg/monitoring"

// Create monitor for sub-agent
monitor, err := monitoring.NewAgentMonitor("agent-123", "/tmp/agent-123")
if err != nil {
    log.Fatal(err)
}

// Start monitoring (file watcher, git hooks)
if err := monitor.Start(); err != nil {
    log.Fatal(err)
}
defer monitor.Stop()

// ... sub-agent executes task ...

// Validate results
result, err := monitor.Validate()
if err != nil {
    log.Fatal(err)
}

if !result.Passed {
    log.Warnf("Validation failed (score: %.2f): %s", result.Score, result.Summary)
} else {
    log.Infof("Validation passed (score: %.2f)", result.Score)
}
```

## Architecture

Sub-agent monitoring uses multiple signals to detect fake implementations:

1. **File Watcher** (fsnotify): Tracks file creation/modification in real-time
2. **Git Hooks**: Monitors commits with post-commit hook
3. **Output Parser**: Detects test execution from stdout/stderr
4. **Validator**: Multi-signal scoring with fallback verification

## Validation Signals

| Signal | Weight | Default Threshold | Description |
|--------|--------|-------------------|-------------|
| Git Commits | 0.2 | >= 2 | Number of git commits made |
| File Count | 0.2 | >= 3 | Number of files created |
| Line Count | 0.2 | >= 50 | Total lines of code |
| Test Execution | 0.3 | >= 1 | Number of test runs |
| Stub Keywords | 0.1 | <= 3 | Count of TODO/FIXME/NotImplemented |

**Pass Threshold**: 0.6 (60%)

## Configuration

### Default Configuration

```go
config := monitoring.DefaultValidationConfig
// MinFileCount: 3, MinLineCount: 50, MinCommitCount: 2,
// MinTestRuns: 1, MaxStubKeywords: 3, PassThreshold: 0.6
```

### Simple Task Configuration

```go
config := monitoring.SimpleTaskConfig
// Relaxed thresholds for trivial tasks
```

### Complex Task Configuration

```go
config := monitoring.ComplexTaskConfig
// Strict thresholds for multi-component features
```

### Custom Configuration

```go
monitor := monitoring.NewAgentMonitor("agent-123", workDir)
monitor.SetValidationConfig(monitoring.ValidationConfig{
    MinFileCount:    5,
    MinLineCount:    200,
    MinCommitCount:  5,
    MinTestRuns:     2,
    MaxStubKeywords: 0,
    PassThreshold:   0.7,
})
```

## Examples

### Example 1: Monitor with Output Parsing

```go
monitor, _ := monitoring.NewAgentMonitor("agent-123", "/tmp/agent-123")
monitor.Start()
defer monitor.Stop()

// Capture sub-agent output
cmd := exec.Command("your-sub-agent", "task")
stdout, _ := cmd.StdoutPipe()

go monitor.ParseOutput(stdout)

cmd.Run()

result, _ := monitor.Validate()
fmt.Printf("Score: %.2f, Passed: %v\n", result.Score, result.Passed)
```

### Example 2: Get Real-Time Stats

```go
monitor, _ := monitoring.NewAgentMonitor("agent-123", workDir)
monitor.Start()

// ... agent executes ...

stats := monitor.GetStats()
fmt.Printf("Files created: %d\n", stats.FilesCreated)
fmt.Printf("Commits: %d\n", stats.CommitsDetected)
fmt.Printf("Tests run: %d\n", stats.TestRuns)
fmt.Printf("Duration: %v\n", stats.Duration)
```

### Example 3: Inspect Validation Signals

```go
result, _ := monitor.Validate()

for _, signal := range result.Signals {
    status := "✅"
    if !signal.Passed {
        status = "❌"
    }
    fmt.Printf("%s %s: %v (expected %v)\n",
        status, signal.Name, signal.Value, signal.Expected)
}
```

## Validation Result

```go
type ValidationResult struct {
    Passed  bool               // Overall pass/fail
    Score   float64            // Aggregate score (0.0-1.0)
    Signals []ValidationSignal // Individual signal results
    Summary string             // Human-readable summary
}
```

**Example Output**:
```
Validation Result: FAILED (Score: 0.40)

Signals:
  ✅ Git Commits:     3 commits >= 2 expected (PASS, weight: 0.2)
  ❌ File Count:      2 files < 3 expected (FAIL, weight: 0.2)
  ✅ Line Count:      85 lines >= 50 expected (PASS, weight: 0.2)
  ❌ Test Execution:  0 test runs < 1 expected (FAIL, weight: 0.3)
  ✅ Stub Keywords:   1 stubs <= 3 expected (PASS, weight: 0.1)

Summary: Validation failed (score: 0.40). Failed signals: 2 files < 3 expected; 0 test runs < 1 expected
```

## Components

### AgentMonitor

Coordinates all monitoring components.

```go
monitor := monitoring.NewAgentMonitor(agentID, workDir)
monitor.Start()    // Initialize watchers, install hooks
monitor.Stop()     // Clean up watchers, uninstall hooks
monitor.Validate() // Run multi-signal validation
monitor.GetStats() // Get real-time statistics
```

### FileWatcher

Monitors file system changes using fsnotify.

- Detects: Create, Modify, Delete, Rename
- Filters: `.git/`, `*.swp`, `.vscode/`, `node_modules/`
- Publishes: `sub_agent.file.created`, `sub_agent.file.modified`, `sub_agent.file.deleted`

### GitHookManager

Manages git post-commit hooks.

- Installs: Backup existing hook, write monitoring hook
- Captures: Commit hash, message, author, files changed, insertions, deletions
- Uninstalls: Restore backup, remove monitoring hook

### OutputParser

Parses sub-agent output to detect commands.

- Patterns: Go tests, npm tests, pytest, git commands, make
- Detects: Test execution, build commands, git operations
- Publishes: `sub_agent.test.started`, `sub_agent.test.passed`

### Validator

Multi-signal validation with weighted scoring.

- Signals: 5 validation dimensions
- Fallbacks: Event log → direct verification (git log, file count, run tests)
- Scoring: Weighted sum, configurable threshold

## Testing

Run integration tests:

```bash
cd engram/
go test -v ./pkg/monitoring/
```

Expected output:
```
=== RUN   TestValidationScoring
    Fake implementation score: 0.10
    Validation failed (expected)
--- PASS: TestValidationScoring (0.01s)
```

## Dependencies

- `github.com/fsnotify/fsnotify v1.7.0` - File system monitoring
- `github.com/vbonnet/engram/core/pkg/eventbus` - Event publishing (optional)
- `github.com/google/uuid` - Agent ID generation (via eventbus)

## License

See LICENSE in engram repository.
