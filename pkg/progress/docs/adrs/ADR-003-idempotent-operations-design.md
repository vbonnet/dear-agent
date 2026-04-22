# ADR-003: Idempotent Operations Design

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Ensuring progress package is robust against incorrect usage patterns and doesn't cause application crashes

---

## Context

Progress indicators are often used in complex control flow with conditional branches, error handling, and cleanup logic. Common usage patterns that can lead to bugs:

**Double Start**:
```go
p := progress.New(...)
p.Start()
// ... some code ...
p.Start()  // ← Accidentally called again
```

**Complete Without Start**:
```go
p := progress.New(...)
if someCondition {
    p.Start()
}
// Later in cleanup code (runs regardless of condition)
p.Complete("Done")  // ← Called even if Start() wasn't
```

**Multiple Complete Calls**:
```go
p := progress.New(...)
p.Start()
// ... work ...
p.Complete("Done")
// ... more code ...
p.Complete("Done")  // ← Accidentally called again (e.g., in deferred cleanup)
```

**Update After Complete**:
```go
p := progress.New(...)
p.Start()
p.Complete("Done")
p.Update(50, "Halfway")  // ← Called after completion
```

**Problem**: Should these scenarios cause panics, errors, or be handled gracefully?

---

## Decision

We will make all operations **idempotent** and **safe** to prevent application crashes from incorrect usage.

**Key Design Principles**:

1. **No Panics**: Progress operations never panic, even with incorrect usage
2. **No Errors**: Progress operations don't return errors (void methods)
3. **State Guards**: Internal `started` flag prevents invalid state transitions
4. **Graceful No-ops**: Invalid operations are silently ignored (no-op)

**Idempotency Guarantees**:

| Operation | Multiple Calls | Behavior |
|-----------|----------------|----------|
| `Start()` | Multiple | First call starts, subsequent calls are no-op |
| `Update()` | Before `Start()` | No-op (not started) |
| `Update()` | After `Complete()`/`Fail()` | No-op (not started) |
| `Complete()`/`Fail()` | Without `Start()` | No-op (not started) |
| `Complete()`/`Fail()` | Multiple | First call completes, subsequent calls are no-op |

---

## Consequences

### Positive

**No Application Crashes**:
- Invalid usage patterns don't cause panics
- Progress indication is **non-critical** (shouldn't break application)
- Applications remain robust even with bugs in progress handling

**Simplified Error Handling**:
- No need to check if `Start()` was called before `Complete()`
- No need to track progress state in application code
- Cleanup code can safely call `Complete()` regardless of earlier flow

**Flexible Usage Patterns**:
- Safe to call `Complete()` in deferred cleanup:
  ```go
  p := progress.New(...)
  defer p.Complete("Done")  // Safe even if Start() wasn't called
  p.Start()
  // ... work ...
  ```
- Safe to call `Start()` multiple times (e.g., in retry loops)
- Safe to nest progress indicators with complex control flow

**Defensive Programming**:
- Internal state (`started` flag) prevents nil pointer dereferences
- Backends can be safely stopped multiple times
- No race conditions from multiple completions

### Negative

**Silent Failures**:
- Incorrect usage is not reported (no errors, no panics)
- Developers might not notice bugs in progress handling
- This is **acceptable** because:
  - Progress indication is non-critical (silent failure is better than crash)
  - Most incorrect usage is harmless (e.g., calling `Start()` twice)
  - Manual testing will reveal missing progress updates

**Cannot Detect Incorrect Usage**:
- No way to distinguish between intentional no-op and accidental no-op
- No logging or warnings for invalid operations
- This is **acceptable** because:
  - Adding logging would spam output in valid patterns (e.g., deferred cleanup)
  - Unit tests catch incorrect usage during development
  - Production crashes are worse than silent no-ops

**State Management Overhead**:
- `started` flag adds 1 byte per Indicator
- State checks add ~3 instructions per method call
- This is **acceptable** because:
  - Memory overhead is negligible (1 byte)
  - CPU overhead is negligible (branch prediction handles checks efficiently)

---

## Alternatives Considered

### Alternative 1: Panic on Invalid Usage

**Approach**:
```go
func (i *Indicator) Start() {
    if i.started {
        panic("progress: Start() called twice")
    }
    // ...
}
```

**Rejected Because**:
- Panics crash application (unacceptable for non-critical feature)
- Requires users to track progress state manually
- Error-prone in complex control flow (cleanup code, conditionals)
- Real-world usage: accidental double calls are common and harmless

### Alternative 2: Return Errors

**Approach**:
```go
func (i *Indicator) Start() error {
    if i.started {
        return fmt.Errorf("already started")
    }
    // ...
    return nil
}
```

**Rejected Because**:
- Forces users to check errors for non-critical operations
- Clutters application code with error handling
- Users would likely ignore errors anyway (`p.Start()` without checking)
- Progress indication should be fire-and-forget (no error checking needed)

### Alternative 3: State Machine with Validation

**Approach**:
```go
type State int
const (
    StateNotStarted State = iota
    StateRunning
    StateCompleted
    StateFailed
)

func (i *Indicator) Update(...) {
    if i.state != StateRunning {
        log.Printf("warning: Update() called in state %v", i.state)
        return
    }
    // ...
}
```

**Rejected Because**:
- More complex state tracking (4 states vs 1 boolean)
- Logging clutters output (especially in loops with thousands of updates)
- State machine is overkill for simple lifecycle (start → update* → complete)

### Alternative 4: No Idempotency (Trust Users)

**Approach**:
```go
func (i *Indicator) Start() {
    // Just start, assume user called correctly
    i.spinnerBackend.Start()
}
```

**Rejected Because**:
- Incorrect usage causes nil pointer dereferences (if backend not initialized)
- Double `Start()` might cause backend panics (spinner library)
- Double `Complete()` might print success message twice (confusing)
- Defensive programming is better for robustness

---

## Implementation Notes

**State Tracking** (in `indicator.go`):
```go
type Indicator struct {
    mode           Mode
    opts           Options
    spinnerBackend *spinnerBackend
    barBackend     *progressBarBackend
    started        bool  // ← State flag
}
```

**Idempotent Start** (in `indicator.go`):
```go
func (i *Indicator) Start() {
    if i.started {
        return  // ← No-op if already started
    }
    i.started = true

    if i.mode == ModeSpinner {
        i.spinnerBackend.Start()
    } else {
        i.barBackend.Start()
    }
}
```

**Guarded Update** (in `indicator.go`):
```go
func (i *Indicator) Update(current int, message string) {
    if !i.started {
        return  // ← No-op if not started
    }

    if i.mode == ModeSpinner {
        i.spinnerBackend.Update(message)
    } else {
        i.barBackend.Update(current, message)
    }
}
```

**Idempotent Complete** (in `indicator.go`):
```go
func (i *Indicator) Complete(message string) {
    if !i.started {
        return  // ← No-op if not started
    }

    // Stop backend
    if i.mode == ModeSpinner {
        i.spinnerBackend.Stop()
    } else {
        i.barBackend.Stop()
    }

    // Print success message
    if IsTTY() {
        fmt.Printf("✅ %s\n", message)
    } else {
        fmt.Printf("SUCCESS: %s\n", message)
    }

    i.started = false  // ← Reset state (allows restart)
}
```

**Why Reset `started` to `false` in Complete?**:
- Allows restarting indicator with new `Start()` call
- Useful for retry loops or reusable indicator instances
- Alternative: Never reset → indicator is single-use (rejected because less flexible)

---

## Usage Patterns

### Pattern 1: Deferred Cleanup (Safe)

```go
func processFiles() error {
    p := progress.New(progress.Options{Total: 100})
    defer p.Complete("Processing complete")  // ← Safe even if Start() fails

    p.Start()

    for i := 0; i < 100; i++ {
        if err := processFile(i); err != nil {
            p.Fail(err)  // ← Early exit, deferred Complete() becomes no-op
            return err
        }
        p.Update(i+1, "")
    }

    return nil
}
```

**Why This Works**:
- If `Start()` is called: `defer Complete()` prints success
- If `Fail()` is called: Sets `started = false`, `defer Complete()` becomes no-op
- No panics, no duplicate messages

### Pattern 2: Conditional Start (Safe)

```go
func search(verbose bool) {
    p := progress.New(progress.Options{Label: "Searching..."})

    if verbose {
        p.Start()  // ← Only start if verbose mode
    }

    // ... search logic ...

    p.Complete("Found results")  // ← Safe to call regardless of verbose flag
}
```

**Why This Works**:
- If `verbose == true`: `Start()` called, `Complete()` prints success
- If `verbose == false`: `Start()` not called, `Complete()` is no-op
- No need to track `verbose` flag for cleanup code

### Pattern 3: Retry Loop (Safe)

```go
func retryOperation() error {
    p := progress.New(progress.Options{Label: "Attempting operation..."})

    for attempt := 1; attempt <= 3; attempt++ {
        p.Start()  // ← Multiple Start() calls (no-op after first)

        err := doOperation()
        if err == nil {
            p.Complete(fmt.Sprintf("Success on attempt %d", attempt))
            return nil
        }

        p.Update(0, fmt.Sprintf("Attempt %d failed, retrying...", attempt))
    }

    p.Fail(fmt.Errorf("all attempts failed"))
    return fmt.Errorf("operation failed")
}
```

**Why This Works**:
- First `Start()` call starts indicator
- Subsequent `Start()` calls are no-op (already started)
- Final `Complete()` or `Fail()` prints result

---

## Testing Strategy

**Unit Tests** (in `indicator_test.go`):

```go
func TestIdempotentOperations(t *testing.T) {
    t.Run("Multiple Start calls are safe", func(t *testing.T) {
        p := New(Options{Total: 0, Label: "Test"})
        p.Start()
        p.Start()  // ← Should not panic or error
        if !p.started {
            t.Error("Expected started to be true")
        }
    })

    t.Run("Complete without Start is safe", func(t *testing.T) {
        p := New(Options{Total: 0, Label: "Test"})
        p.Complete("Done")  // ← Should not panic or error
        if p.started {
            t.Error("Expected started to be false")
        }
    })

    t.Run("Multiple Complete calls are safe", func(t *testing.T) {
        p := New(Options{Total: 0, Label: "Test"})
        p.Start()
        p.Complete("Done")
        p.Complete("Done")  // ← Should not panic or error
        if p.started {
            t.Error("Expected started to be false")
        }
    })
}
```

**Manual Tests**:
- Run applications with intentional incorrect usage (verify no panics)
- Check output for duplicate messages (should not occur)
- Test complex control flow (nested conditionals, error handling)

---

## Related Decisions

- **ADR-001**: Strategy Pattern for Mode Selection (backends must support idempotent operations)
- **ADR-002**: Automatic TTY Detection (no TTY-dependent idempotency logic)

---

## Design Philosophy

**Core Belief**: Progress indication is **non-critical** and should never break applications.

**Implications**:
- No panics, no errors
- Silent no-ops for invalid operations
- Defensive programming (state guards, nil checks)
- Trust that correct usage is more common than incorrect usage
- Optimize for robustness over error reporting

**Trade-off**: We sacrifice **error detection** for **robustness**.

This is the right trade-off because:
- Progress indication is for user feedback (not critical business logic)
- Silent failure is better than application crash
- Most incorrect usage is harmless (e.g., double `Start()`)
- Unit tests catch bugs during development

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)
