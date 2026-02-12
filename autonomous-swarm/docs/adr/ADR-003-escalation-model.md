# ADR-003: Three-Tier Error Classification and Escalation Model

## Status

Accepted

## Context

Autonomous Swarm executes beads (autonomous agent tasks) through AGM (Agent Session Manager) sessions. During execution, various errors can occur:

1. **Transient failures**: AGM timeout, network hiccup, temporary resource unavailability
2. **Agent uncertainty**: Agent determines human decision needed
3. **Fatal errors**: Missing configuration, invalid bead ID, file not found
4. **Iteration limits**: Bead retried too many times without success

The system must differentiate between errors that should:
- **Retry**: Temporary failures that may succeed on retry
- **Escalate**: Situations requiring human intervention
- **Fail immediately**: Unrecoverable errors that retries won't fix

Without error classification, the system would:
- Retry unrecoverable errors (wasting resources)
- Fail on temporary issues (reducing reliability)
- Miss signals for human intervention (causing downstream failures)

We need a clear error handling strategy that enables autonomous operation while recognizing when to involve humans.

## Decision

We will implement a **three-tier error classification model** with typed errors and structured escalation handling.

### Error Type Hierarchy

**File**: `pkg/executor/errors.go`

```go
type ErrorType int

const (
    ErrorRecoverable ErrorType = iota  // Retry
    ErrorEscalation                    // Move to blocked queue
    ErrorFatal                         // Stop immediately
)

type ExecutionError struct {
    Type      ErrorType
    BeadID    string
    Iteration int
    Cause     error
    Message   string
}
```

### Classification Rules

#### 1. Recoverable Errors (Retry)

**Conditions**:
- AGM session timeout (session didn't start in time)
- YAML parse errors (malformed output)
- tmux connection issues
- Temporary file system errors (disk momentarily busy)

**Handling**:
```go
if isRecoverable(err) && iteration < maxIterations {
    iteration++
    return retry()
}
```

**Rationale**: These errors often resolve on retry; don't waste autonomous execution opportunity.

**Max Iterations**: 3 attempts per bead
- Balances persistence vs wasted resources
- 3 retries = ~exponential backoff opportunity
- After 3 failures, likely systemic issue

#### 2. Escalation Errors (Human Required)

**Conditions**:
- Max iterations exceeded (3 failed attempts)
- Explicit escalation signal from agent
- Agent detects ambiguous requirement
- Multiple valid approaches, decision needed

**Handling**:
```go
if iteration >= maxIterations || detectEscalation(output) {
    moveToBlocked(beadID, reason)
    return exitCode(2) // Escalation exit code
}
```

**Rationale**: Autonomous system recognizes its limits; human judgment more efficient than continued retries.

**Escalation Signal** (`pkg/executor/escalation.go`):
```go
func DetectEscalation(output string) (bool, string) {
    if strings.Contains(output, "ESCALATE:") {
        reason := extractAfter(output, "ESCALATE:")
        return true, reason
    }
    return false, ""
}
```

**Agent Usage**:
```
ESCALATE: Requires human decision on API version choice (v1 vs v2)
```

#### 3. Fatal Errors (Immediate Stop)

**Conditions**:
- Configuration file not found
- Invalid bead ID (not in queue)
- AGM binary not in PATH
- Queue file corrupted (YAML parse failure)
- Missing required dependencies (not in completed queue)

**Handling**:
```go
if isFatal(err) {
    logError(err)
    return exitCode(1) // Error exit code
}
```

**Rationale**: These errors won't resolve with retries; fail fast and alert user.

### Escalation Workflow

```
Execute Bead
    ↓
Recoverable error?
    ├─ Yes → iteration < 3?
    │         ├─ Yes → Increment iteration, retry
    │         └─ No → Escalate (max iterations)
    └─ No → Escalation signal?
              ├─ Yes → Move to blocked, exit 2
              └─ No → Fatal error?
                        ├─ Yes → Log error, exit 1
                        └─ No → Success, exit 0
```

### State Transitions

```
Bead: ready → in_progress (claim)
    ↓
Execute via AGM
    ↓
Error occurs
    ├─ Recoverable → Retry (bead stays in_progress, iteration++)
    ├─ Escalation → Move to blocked (human intervention)
    └─ Fatal → Log error, exit (bead stays in_progress)
```

**Blocked Queue**:
- Beads requiring human intervention
- Can be manually fixed and moved back to ready
- Automatic unblocking when dependencies complete (not escalation)

### Exit Codes

| Code | Meaning | User Action |
|------|---------|-------------|
| 0 | Success | None - continue execution |
| 1 | Fatal error | Check logs, fix configuration |
| 2 | Escalation | Review blocked queue, provide decision |

**Rationale**: Standard Unix exit code convention; scriptable for automation.

### Rejected Alternatives

#### Alternative 1: Binary Error Model (Success/Failure)

**Approach**: All errors treated the same - either succeed or fail

**Rejected Because**:
- No distinction between "retry might help" and "retries useless"
- Wastes resources on unrecoverable errors
- Fails on temporary issues (reduces reliability)
- No mechanism for human-in-the-loop

#### Alternative 2: Exponential Backoff Without Classification

**Approach**: Retry all errors with increasing delays

**Rejected Because**:
- Still retries fatal errors (wasting time)
- Doesn't recognize need for human intervention
- Delays failure detection on unrecoverable errors
- Complexity of backoff logic without classification benefit

#### Alternative 3: Manual Escalation Only

**Approach**: No automatic escalation; agent must always signal explicitly

**Rejected Because**:
- Max iteration case unhandled (infinite retries)
- Agent may not recognize when to escalate
- System can't protect itself from runaway retries

#### Alternative 4: Exception-Based Hierarchy

**Approach**: Use panic/recover and exception types

**Rejected Because**:
- Go idiom prefers explicit error handling
- Harder to trace error flow
- Panic/recover breaks structured control flow
- Less readable than typed error structs

## Consequences

### Positive

1. **Reliability**: Automatic retry on transient failures
2. **Efficiency**: No wasted retries on fatal errors
3. **Autonomy**: System recognizes its own limits
4. **Observability**: Clear error types in logs
5. **User Experience**: Exit codes enable scripting/automation

### Negative

1. **Classification Burden**: Each error must be classified correctly
2. **False Positives**: Misclassified recoverable as fatal = premature failure
3. **False Negatives**: Misclassified fatal as recoverable = wasted retries
4. **Complexity**: Three error paths vs single error path

### Trade-offs

- **Simplicity vs Robustness**: Error classification complexity buys resilience
- **Autonomy vs Control**: Automatic escalation reduces flexibility but prevents runaway retries
- **Immediate Failure vs Persistence**: Max iterations balances both

## Implementation Notes

### Typed Error Construction

```go
// Recoverable
return NewRecoverableError(beadID, iteration, "AGM timeout", err)

// Escalation
return NewEscalationError(beadID, iteration, "max iterations", nil)

// Fatal
return NewFatalError(beadID, iteration, "bead not found", err)
```

**Type Safety**: Compile-time checking of error type; impossible to forget classification.

### Escalation Signal Format

**Agent Output**:
```
[... normal output ...]
ESCALATE: <reason>
[... optional context ...]
```

**Parser**:
```go
func DetectEscalation(output string) (bool, string) {
    if strings.Contains(output, "ESCALATE:") {
        lines := strings.Split(output, "\n")
        for _, line := range lines {
            if strings.Contains(line, "ESCALATE:") {
                reason := strings.TrimPrefix(line, "ESCALATE:")
                return true, strings.TrimSpace(reason)
            }
        }
    }
    return false, ""
}
```

**Simplicity**: No structured format (JSON) required; plain text signal.

### Metadata Tracking

**Bead Metadata**:
```go
type BeadMetadata struct {
    SessionName       string
    Iterations        int       // Incremented on each retry
    LastAttempt       time.Time // Timestamp of last execution
    EscalationReason  string    // Populated on escalation
}
```

**Rationale**: Enables post-mortem analysis, retry tracking, human context on escalation.

### Iteration Reset

**Question**: Should iterations reset after manual intervention?

**Decision**: No - iterations persist in metadata
- Tracks total attempts including manual fixes
- Prevents infinite escalation loops
- Human can manually reset if desired

### Future Enhancements

1. **Configurable Max Iterations**: Per-bead or global setting
2. **Backoff Delays**: Wait between retries (exponential backoff)
3. **Escalation Webhooks**: Notify humans via Slack/email
4. **Error Metrics**: Track escalation rate, most common errors
5. **Smart Classification**: ML-based error type prediction

## Testing

**Test Coverage**: `pkg/executor/errors_test.go`

```go
func TestErrorClassification(t *testing.T) {
    tests := []struct {
        name     string
        err      error
        wantType ErrorType
    }{
        {"AGM timeout", csmTimeoutErr, ErrorRecoverable},
        {"Bead not found", beadNotFoundErr, ErrorFatal},
        {"Max iterations", maxIterErr, ErrorEscalation},
    }
    // ...
}
```

**Escalation Detection**: `pkg/executor/escalation_test.go`

```go
func TestDetectEscalation(t *testing.T) {
    output := "Some output\nESCALATE: Need human decision\nMore output"
    escalated, reason := DetectEscalation(output)
    assert.True(t, escalated)
    assert.Equal(t, "Need human decision", reason)
}
```

## Observability

**Telemetry Events** (`EXECUTION-LOG.jsonl`):

```json
{"timestamp":"2024-01-01T12:00:00Z","bead_id":"bead-1","event":"error","details":{"type":"recoverable","iteration":1,"message":"AGM timeout"}}
{"timestamp":"2024-01-01T12:05:00Z","bead_id":"bead-1","event":"escalate","details":{"reason":"max iterations exceeded","iterations":3}}
```

**Roadmap** (`ROADMAP.md`):
```markdown
## Blocked
- **bead-1**: First bead (reason: max iterations exceeded, iterations: 3)
```

**User Workflow**:
1. Check roadmap for blocked beads
2. Read escalation reason in EXECUTION-LOG.jsonl
3. Manually fix issue (update bead, fix dependency)
4. Move bead back to ready queue
5. Re-execute

## Real-World Inspiration

- **Kubernetes**: Pod restart policy (Always, OnFailure, Never)
- **CircleCI**: Automatic retry on flaky tests
- **GitHub Actions**: `continue-on-error` and manual approval steps
- **AWS Step Functions**: Retry configuration with max attempts

## References

- **Implementation**: `pkg/executor/errors.go`, `pkg/executor/escalation.go`
- **Tests**: `pkg/executor/errors_test.go`, `pkg/executor/escalation_test.go`
- **Related**: [ADR-001: Dependency Graph](ADR-001-dependency-graph.md)
- **Related**: [SPEC.md](../../SPEC.md) Section 4.2 (Error Handling)

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0.0 | 2026-02-11 | Initial decision record | Backfill Documentation |
