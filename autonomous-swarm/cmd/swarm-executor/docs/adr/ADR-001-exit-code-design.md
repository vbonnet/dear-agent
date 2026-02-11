# ADR-001: Three-Tier Exit Code Design

## Status

**Accepted** - Implemented in swarm-executor v0.1.0

## Context

swarm-executor needs to communicate execution results to callers (human users, launcher scripts,
CI/CD systems). The primary mechanism for this in Unix-like systems is the exit code returned
by the process.

### Requirements

1. **Success/Failure Distinction**: Clearly indicate if bead execution succeeded or failed
2. **Escalation Detection**: Enable automation to detect when human intervention is required
3. **Script Integration**: Support programmatic branching in shell scripts and launchers
4. **Unix Convention Compliance**: Follow established conventions (0=success, non-zero=error)
5. **Clear Semantics**: Each exit code should have unambiguous meaning

### Constraints

- Exit codes limited to 0-255 (POSIX standard)
- Common conventions: 0=success, 1=error, 2=misuse, 126=not executable, 127=not found, 128+=signal
- Must avoid conflicts with shell-reserved codes (126-165)

## Decision

Implement a **three-tier exit code system**:

```
Exit Code 0: SUCCESS
  - Bead executed and completed successfully
  - --version or --help flag provided

Exit Code 1: ERROR
  - Missing required flags
  - Queue file not found or invalid
  - CSM session creation failed
  - Execution errors (non-escalation)
  - Any recoverable or fatal error

Exit Code 2: ESCALATION
  - Max iterations exceeded
  - Explicit ESCALATE: signal detected
  - Human intervention required
```

### Implementation

```go
func executeBead(queuePath, beadID, sessionName string) int {
    // ... execution logic ...

    if err != nil {
        if executor.IsEscalationError(err) {
            fmt.Fprintf(os.Stderr, "Escalation: %v\n", err)
            logError(logger, beadID, fmt.Sprintf("escalation: %v", err))
            generateRoadmap()  // Best effort
            return 2  // ESCALATION
        }

        fmt.Fprintf(os.Stderr, "Error: Execution failed: %v\n", err)
        logError(logger, beadID, fmt.Sprintf("execution failed: %v", err))
        generateRoadmap()  // Best effort
        return 1  // ERROR
    }

    // Success path
    logComplete(logger, beadID)
    generateRoadmap()
    return 0  // SUCCESS
}
```

### Exit Code Detection

```go
// pkg/executor/errors.go
func IsEscalationError(err error) bool {
    if err == nil {
        return false
    }
    if execErr, ok := err.(*ExecutionError); ok {
        return execErr.Type == ErrorEscalation
    }
    return false
}
```

## Consequences

### Positive

**Clear Automation Semantics**:
```bash
#!/bin/bash
swarm-executor --queue Q.yaml --bead-id B --session S

case $? in
  0)
    echo "Success - continue with next bead"
    ;;
  1)
    echo "Error - retry or skip"
    exit 1
    ;;
  2)
    echo "Escalation - pause execution for human review"
    exit 2
    ;;
esac
```

**Programmatic Escalation Handling**:
- Launcher can automatically pause on exit code 2
- CI/CD can create tickets for escalated beads
- Scripts can distinguish transient errors (retry) from escalations (block)

**Exit Code Stability**:
- 3-tier system provides clear categories without excessive granularity
- Room for future codes (3-125) if needed
- Backward compatible - existing scripts continue to work

**Observable Behavior**:
- Exit code matches logged event type (success/error/escalation)
- Consistent with EXECUTION-LOG.jsonl events
- Roadmap generated regardless of exit code

### Negative

**Limited Granularity**:
- Cannot distinguish error types via exit code alone (must parse stderr)
- All errors grouped into exit code 1 (queue load, CSM failure, validation, etc.)
- Future: Could use codes 3-10 for specific error categories if needed

**Convention Conflict**:
- Exit code 2 traditionally means "misuse of shell builtin" (Bash convention)
- We repurpose it for escalation - may confuse some tools
- Mitigated by: Clear documentation, uncommon in modern tooling

**Escalation Type Ambiguity**:
- Exit code 2 doesn't distinguish max iterations from explicit escalation
- Must parse stderr or execution log for escalation reason
- Acceptable: Primary goal is "human needed" signal, not reason classification

## Alternatives Considered

### Alternative 1: Binary Exit Codes (0/1 only)

**Approach**: Use 0 for success, 1 for all failures (errors + escalations)

**Pros**:
- Simplest implementation
- Full Unix convention compliance
- No ambiguity about exit code meaning

**Cons**:
- Automation cannot distinguish escalation from errors
- Scripts must parse stderr to detect escalation
- Launcher cannot pause automatically on escalation
- **REJECTED**: Insufficient information for automation

### Alternative 2: Fine-Grained Exit Codes

**Approach**: Use distinct codes for error types:
```
0 = Success
1 = General error
2 = Queue load error
3 = CSM creation error
4 = Validation failure
5 = Max iterations exceeded
6 = Explicit escalation
...
```

**Pros**:
- Rich information without parsing output
- Scripts can handle specific errors differently
- Clear error categorization

**Cons**:
- Over-engineering for v0.1.0 needs
- Exit code explosion (hard to remember/document)
- Difficult to maintain backward compatibility
- Most scripts only care about success/retry/escalate
- **REJECTED**: Unnecessary complexity for current use cases

### Alternative 3: Exit Code + JSON Output

**Approach**: Use 0/1 exit codes + structured JSON on stdout for details

**Example**:
```json
{
  "status": "escalation",
  "reason": "max iterations exceeded",
  "bead_id": "bead-1",
  "iterations": 3
}
```

**Pros**:
- Rich structured data for parsing
- Exit codes remain simple
- Easy to extend without breaking compatibility

**Cons**:
- Stdout pollution (breaks pipeline usage)
- Requires jq or similar for script parsing
- Redundant with EXECUTION-LOG.jsonl
- **REJECTED**: Violates stdout/stderr separation, redundant with telemetry

### Alternative 4: Signal-Based Communication

**Approach**: Use signals to communicate state (SIGUSR1 for escalation)

**Pros**:
- Out-of-band communication
- Doesn't interfere with exit codes

**Cons**:
- Complex for callers (signal handlers required)
- Not portable across all environments
- Overkill for simple status communication
- **REJECTED**: Complexity far exceeds benefit

## Implementation Notes

### Exit Code Consistency

**Invariant**: Exit code must match execution log events

```go
// Example: Success path
logger.LogEvent(&ExecutionEvent{
    BeadID: beadID,
    Event:  "complete",
    Details: map[string]interface{}{"status": "success"},
})
return 0  // Exit code matches "complete" event
```

```go
// Example: Escalation path
logger.LogEvent(&ExecutionEvent{
    BeadID: beadID,
    Event:  "error",
    Details: map[string]interface{}{"message": "escalation: ..."},
})
return 2  // Exit code 2 for escalation
```

### Testing Exit Codes

```go
func TestExecuteBeadEscalation(t *testing.T) {
    // Simulate max iterations escalation
    binary := buildBinary(t)
    cmd := exec.Command(binary, "--queue", queueWithMaxIterations,
                        "--bead-id", "bead-1", "--session", "test")
    err := cmd.Run()

    // Verify exit code 2
    if exitErr, ok := err.(*exec.ExitError); ok {
        if exitErr.ExitCode() != 2 {
            t.Errorf("expected exit code 2, got %d", exitErr.ExitCode())
        }
    }
}
```

### Documentation Requirements

All user-facing documentation must include exit code table:

```
Exit Codes:
  0  Success - Bead executed successfully
  1  Error - Execution failed (see stderr for details)
  2  Escalation - Bead requires human intervention
```

**Locations**:
- `--help` output (printUsage function)
- README.md usage section
- SPEC.md interface specification
- Man page (future)

## Related Decisions

- **ADR-003: Error Classification** - Defines ErrorEscalation type used for exit code 2
- **ADR-005: Telemetry Events** - Exit codes correlate with event types

## References

- [Advanced Bash-Scripting Guide - Exit Codes](http://www.tldp.org/LDP/abs/html/exitcodes.html)
- [GNU Coreutils - Exit Status](https://www.gnu.org/software/coreutils/manual/html_node/Exit-status.html)
- [POSIX Exit Status](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#tag_18_08_02)

## Revision History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-02-11 | Initial decision record | Backfill Documentation |
