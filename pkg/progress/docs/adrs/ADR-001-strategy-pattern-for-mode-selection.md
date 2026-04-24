# ADR-001: Strategy Pattern for Mode Selection

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Initial design of progress package

---

## Context

The progress package needs to support two different types of progress indication:

1. **Indeterminate Progress (Spinner)**: For operations with unknown duration or total steps (e.g., "Searching knowledge base...", "Waiting for API response...")

2. **Determinate Progress (Progress Bar)**: For operations with known total steps (e.g., "Processing 100 files", "Wayfinder Phase 3/11")

Users should not need to know which library to use (spinner vs progress bar) or how to switch between them. The package should provide a unified API that automatically selects the appropriate mode.

---

## Decision

We will use the **Strategy Pattern** with a single public `Indicator` type that delegates to one of two backend implementations (`spinnerBackend` or `progressBarBackend`) selected at initialization time.

**Key Design Elements**:

1. **Single Public Type**: `Indicator` is the only type users interact with

2. **Mode Selection Logic**: Mode is selected based on `Options.Total`:
   - `Total == 0` → Spinner (indeterminate)
   - `Total > 0` → Progress bar (determinate)

3. **Backend Delegation**: `Indicator` delegates method calls to the active backend:
   ```go
   func (i *Indicator) Update(current int, message string) {
       if i.mode == ModeSpinner {
           i.spinnerBackend.Update(message)
       } else {
           i.barBackend.Update(current, message)
       }
   }
   ```

4. **Encapsulation**: Backend types (`spinnerBackend`, `progressBarBackend`) are private (not exported)

---

## Consequences

### Positive

**Simplified API**:
- Users call `progress.New(options)` for all use cases
- No need to import separate `Spinner` or `ProgressBar` types
- Single set of methods works for both modes (`Start`, `Update`, `Complete`, `Fail`)

**Automatic Mode Selection**:
- No manual decision required from users
- Natural mapping: `Total == 0` means "I don't know how many steps" → spinner
- Clear and intuitive: `Total = 100` means "100 steps" → progress bar

**Future Extensibility**:
- Can add new backend types (e.g., `MultiBarBackend` for concurrent tasks)
- Can add new selection logic (e.g., `Options.Mode` for explicit override)
- Backends can be swapped without breaking public API

**Encapsulation**:
- Backend implementation details are hidden
- Can change spinner/progressbar libraries without breaking users
- Users don't need to understand backend differences

### Negative

**Method Parameter Overloading**:
- `Update(current int, message string)` has different semantics per mode:
  - Spinner: `current` is ignored, `message` updates label
  - Progress bar: `current` is step number, `message` is description
- This is **acceptable** because:
  - Users typically use one mode per indicator (don't switch mid-operation)
  - Documentation clearly explains parameter usage per mode
  - Type safety enforced at compile time (both modes accept same signature)

**Mode Field Overhead**:
- `Indicator` stores both `mode` enum and backend pointers
- One backend pointer is always nil (small memory waste)
- This is **acceptable** because:
  - Overhead is ~8 bytes per Indicator (negligible)
  - Simpler than complex union types or interface{} casting

**No Mid-Operation Mode Changes**:
- Once created, an Indicator cannot switch modes (spinner → progress bar)
- This is **acceptable** because:
  - Real-world use cases don't require mid-operation mode changes
  - Users can create new Indicator if mode change is needed

---

## Alternatives Considered

### Alternative 1: Separate Spinner and ProgressBar Types

**Approach**:
```go
spinner := progress.NewSpinner("Loading...")
progressBar := progress.NewProgressBar(100, "Processing")
```

**Rejected Because**:
- Users must choose type upfront (adds cognitive load)
- Two separate APIs to learn and maintain
- Harder to refactor code when switching between modes

### Alternative 2: Interface-Based Approach

**Approach**:
```go
type Indicator interface {
    Start()
    Update(current int, message string)
    Complete(message string)
}

var spinner Indicator = NewSpinner(...)
var bar Indicator = NewProgressBar(...)
```

**Rejected Because**:
- Still requires users to choose type (no automatic selection)
- Interface abstraction adds complexity without benefits
- Harder to add mode-specific methods (e.g., `UpdatePhase`)

### Alternative 3: Functional Options for Mode Selection

**Approach**:
```go
progress.New(
    progress.WithLabel("Processing"),
    progress.WithMode(progress.ModeProgressBar),
    progress.WithTotal(100),
)
```

**Rejected Because**:
- More verbose than simple struct options
- Mode selection is implicit in `Total` (no need for explicit `WithMode`)
- Functional options add complexity for minimal benefit

---

## Implementation Notes

**Mode Selection Logic** (in `indicator.go`):
```go
func New(opts Options) *Indicator {
    if opts.Total == 0 {
        // Indeterminate mode: Spinner
        return &Indicator{
            mode:           ModeSpinner,
            opts:           opts,
            spinnerBackend: newSpinnerBackend(opts),
        }
    }

    // Determinate mode: Progress Bar
    return &Indicator{
        mode:       ModeProgressBar,
        opts:       opts,
        barBackend: newProgressBarBackend(opts),
    }
}
```

**Delegation Example** (in `indicator.go`):
```go
func (i *Indicator) Update(current int, message string) {
    if !i.started {
        return
    }

    if i.mode == ModeSpinner {
        i.spinnerBackend.Update(message)
    } else {
        i.barBackend.Update(current, message)
    }
}
```

---

## Related Decisions

- **ADR-002**: Automatic TTY Detection (affects backend implementations)
- **ADR-003**: Idempotent Operations Design (affects `Indicator` state management)

---

## References

- **Design Pattern**: Strategy Pattern (Gang of Four)
- **Related Libraries**:
  - `github.com/briandowns/spinner` (spinner backend)
  - `github.com/schollz/progressbar/v3` (progress bar backend)

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)
