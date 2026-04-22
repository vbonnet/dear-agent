# Progress Package - Architecture

**Version**: 1.0.0
**Last Updated**: 2026-02-11
**Status**: Implemented
**Package**: github.com/vbonnet/engram/core/pkg/progress

---

## Overview

The progress package provides unified progress indication for terminal applications through a strategy pattern that selects between spinner (indeterminate) and progress bar (determinate) backends at runtime.

**Key Principles**:
- Single public API for all progress types
- Automatic TTY detection (no manual configuration)
- Idempotent operations (safe incorrect usage)
- Zero dependencies on engram-specific internals (reusable package)

---

## Architecture Diagram

```
┌──────────────────────────────────────────────────────────────┐
│                      Application Layer                       │
│                                                               │
│  Uses: progress.New(options) → *Indicator                   │
│  Calls: Start(), Update(), UpdatePhase(), Complete(), Fail() │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         v
┌──────────────────────────────────────────────────────────────┐
│                    Indicator (Facade)                        │
│                                                               │
│  - Mode selection (based on Options.Total)                   │
│  - Lifecycle management (started flag)                       │
│  - Delegation to backends                                    │
│  - Idempotency enforcement                                   │
└───────────┬────────────────────────────┬─────────────────────┘
            │                            │
   If Total == 0                 If Total > 0
            │                            │
            v                            v
┌─────────────────────┐      ┌──────────────────────┐
│  spinnerBackend     │      │  progressBarBackend  │
│                     │      │                      │
│ - spinner: *Spinner │      │ - bar: *ProgressBar  │
│ - isTTY: bool       │      │ - isTTY: bool        │
│ - message: string   │      │ - total: int         │
│                     │      │                      │
│ Methods:            │      │ Methods:             │
│ - Start()           │      │ - Start()            │
│ - Update(msg)       │      │ - Update(cur, msg)   │
│ - Stop()            │      │ - Stop()             │
└─────────┬───────────┘      └──────────┬───────────┘
          │                             │
          v                             v
┌────────────────────────────────────────────────────┐
│              TTY Detection Layer                   │
│                                                     │
│  IsTTY() bool           - term.IsTerminal()        │
│  GetTerminalWidth() int - term.GetSize() (cap 100) │
└────────────────────────────────────────────────────┘
          │                             │
          v                             v
┌─────────────────────┐      ┌──────────────────────┐
│  TTY Mode Output    │      │  Non-TTY Mode Output │
│                     │      │                      │
│ - ANSI animations   │      │ - Plain text         │
│ - Colored symbols   │      │ - "SUCCESS:"/"ERROR:"│
│ - Progress bars     │      │ - Percentage only    │
│ - ✅/❌ symbols      │      │ - No ANSI codes      │
└─────────────────────┘      └──────────────────────┘
```

---

## Component Details

### 1. Indicator (Public API)

**File**: `indicator.go`

**Purpose**: Unified facade for progress indication

**Responsibilities**:
- Select backend based on `Options.Total` (0 = spinner, >0 = progress bar)
- Manage lifecycle state (`started` flag)
- Delegate method calls to appropriate backend
- Enforce idempotency (prevent duplicate starts, safe multiple completes)
- Format phase messages (`UpdatePhase` → `Update` with formatted string)

**Key Fields**:
```go
type Indicator struct {
    mode           Mode                 // ModeSpinner or ModeProgressBar
    opts           Options              // Configuration
    spinnerBackend *spinnerBackend      // Nil if progress bar mode
    barBackend     *progressBarBackend  // Nil if spinner mode
    started        bool                 // Lifecycle state
}
```

**Key Methods**:
```go
func New(opts Options) *Indicator
    → Inspects opts.Total
    → Creates spinnerBackend (Total == 0) or barBackend (Total > 0)

func (i *Indicator) Start()
    → Sets started = true (idempotent check first)
    → Delegates to backend.Start()

func (i *Indicator) Update(current int, message string)
    → Checks started flag
    → Delegates to backend.Update()

func (i *Indicator) UpdatePhase(current, total int, name string)
    → Formats: "Phase {current}/{total}: {name}"
    → Calls Update(current, formatted_message)

func (i *Indicator) Complete(message string)
    → Stops backend
    → Prints success message (✅ in TTY, "SUCCESS:" in non-TTY)
    → Sets started = false

func (i *Indicator) Fail(err error)
    → Stops backend
    → Prints error message (❌ in TTY, "ERROR:" in non-TTY)
    → Sets started = false
```

**Design Decisions**:
- **Why single Indicator type?** → Simplifies API, users don't need to choose type
- **Why mode field?** → Enables different delegation logic for Update()
- **Why started flag?** → Prevents double-start bugs and nil pointer dereferences
- **Why Complete/Fail set started = false?** → Allows restart with new Start() call

---

### 2. Spinner Backend (Indeterminate Progress)

**File**: `spinner.go`

**Purpose**: Handle indeterminate progress (unknown duration)

**Responsibilities**:
- Start/stop animated spinner in TTY mode
- Print initial message in non-TTY mode
- Update spinner label dynamically
- Silent updates in non-TTY mode (avoid log spam)

**Key Fields**:
```go
type spinnerBackend struct {
    spinner *spinner.Spinner  // From github.com/briandowns/spinner
    isTTY   bool              // Cached TTY detection result
    message string            // Current label/message
}
```

**Key Methods**:
```go
func newSpinnerBackend(opts Options) *spinnerBackend
    → Calls IsTTY() to detect environment
    → TTY mode: Creates spinner (CharSets[11], 100ms refresh)
    → Non-TTY mode: Prints initial message once
    → Sets spinner.Suffix = " " + opts.Label

func (s *spinnerBackend) Start()
    → TTY mode: Starts spinner animation
    → Non-TTY mode: No-op (message already printed in constructor)

func (s *spinnerBackend) Update(message string)
    → TTY mode: Updates spinner.Suffix
    → Non-TTY mode: Silent (avoids flooding CI logs)

func (s *spinnerBackend) Stop()
    → TTY mode: Stops spinner animation
    → Non-TTY mode: No-op
```

**Design Decisions**:
- **Why CharSets[11]?** → Engram standard spinner style (consistent across tools)
- **Why 100ms refresh?** → Smooth animation without excessive CPU usage
- **Why silent updates in non-TTY?** → CI/CD logs already show final status, intermediate updates create noise
- **Why print message in constructor?** → User sees immediate feedback that operation started

---

### 3. Progress Bar Backend (Determinate Progress)

**File**: `progressbar.go`

**Purpose**: Handle determinate progress (known total steps)

**Responsibilities**:
- Display progress bar with percentage and ETA in TTY mode
- Print percentage updates in non-TTY mode
- Calculate bar width based on terminal size
- Track current step and total steps

**Key Fields**:
```go
type progressBarBackend struct {
    bar   *progressbar.ProgressBar  // From github.com/schollz/progressbar/v3
    isTTY bool                       // Cached TTY detection result
    total int                        // Total steps (for percentage calculation)
}
```

**Key Methods**:
```go
func newProgressBarBackend(opts Options) *progressBarBackend
    → Calls IsTTY() and GetTerminalWidth()
    → Calculates bar width: termWidth - 40 (reserves space for label + stats)
    → Minimum bar width: 20 characters
    → TTY mode: Creates progressbar with:
        - Description: opts.Label
        - Predictive time: opts.ShowETA
        - Show count: true
        - Width: calculated bar width
        - Theme: "=" (saucer), " " (padding), "[" (start), "]" (end)
    → Non-TTY mode: Prints initial label

func (p *progressBarBackend) Start()
    → No-op (bar already initialized in constructor)

func (p *progressBarBackend) Update(current int, message string)
    → TTY mode:
        - If message != "": Updates bar.Describe(message)
        - Calls bar.Set(current)
    → Non-TTY mode:
        - Calculates percentage: (current * 100) / total
        - Prints "{message} {pct}%" or "{pct}%" if no message

func (p *progressBarBackend) Stop()
    → TTY mode: Calls bar.Finish()
    → Non-TTY mode: No-op
```

**Design Decisions**:
- **Why reserve 40 characters?** → Accommodates "Phase X/Y: Name [...] 27% (6m remaining)"
- **Why minimum 20 chars?** → Below 20, bar is too small to be useful (better show percentage only)
- **Why set in Update not Add?** → Supports non-sequential updates (e.g., parallel processing with out-of-order completion)
- **Why print percentage in non-TTY?** → Provides feedback without ANSI codes (CI/CD compatible)

---

### 4. TTY Detection Layer

**File**: `tty.go`

**Purpose**: Detect terminal vs non-terminal environments and measure terminal size

**Responsibilities**:
- Determine if stdout is connected to a terminal
- Measure terminal width (with 100-character cap)
- Provide consistent detection across platforms

**Key Functions**:
```go
func IsTTY() bool
    → Calls term.IsTerminal(int(os.Stdout.Fd()))
    → Returns true: Interactive terminal, SSH session, tmux/screen
    → Returns false: Pipe, file redirect, CI/CD environment

func GetTerminalWidth() int
    → If !IsTTY(): Returns 0
    → Calls term.GetSize(int(os.Stdout.Fd()))
    → On error or width == 0: Returns 100 (default)
    → If width > 100: Returns 100 (caps at user preference)
    → Otherwise: Returns actual width
```

**Design Decisions**:
- **Why use term.IsTerminal() not os.Getenv("TERM")?** → More reliable, detects pipes/redirects correctly
- **Why cap at 100 characters?** → User preference from ~/.claude/CLAUDE.md (terminal-friendly width)
- **Why return 0 in non-TTY?** → Signals "no terminal" to callers (progressbar skips width logic)
- **Why default to 100 on error?** → Graceful degradation (better than panic or zero-width bar)

---

### 5. Options Configuration

**File**: `options.go`

**Purpose**: Configure indicator behavior

**Responsibilities**:
- Define configuration structure
- Provide defaults
- Package-level documentation

**Key Types**:
```go
type Options struct {
    Total       int    // Total steps (0 = spinner, >0 = progress bar)
    Label       string // Description/label
    ShowETA     bool   // Show ETA (progress bar only)
    ShowPercent bool   // Show percentage (progress bar only)
}

func DefaultOptions() Options
    → Returns Options{Total: 0, Label: "", ShowETA: true, ShowPercent: true}
```

**Design Decisions**:
- **Why Total in Options not separate New functions?** → Single API, mode selected automatically
- **Why ShowETA/ShowPercent defaults to true?** → Most users want these features, can opt out
- **Why Label is string not functional option?** → Simple, flat config (no builder pattern needed)

---

## Data Flow

### Initialization Flow

```
Application calls New(Options{Total: 100, Label: "Processing"})
    ↓
Indicator.New() inspects opts.Total
    ↓
Total > 0 → Create progressBarBackend
    ↓
newProgressBarBackend() calls IsTTY()
    ↓
IsTTY() returns true (running in terminal)
    ↓
newProgressBarBackend() calls GetTerminalWidth() → 120
    ↓
Calculates bar width: min(120, 100) - 40 = 60
    ↓
Creates progressbar.NewOptions(100, width=60, ...)
    ↓
Returns Indicator{mode: ModeProgressBar, barBackend: {...}}
```

### Update Flow (TTY Mode)

```
Application calls indicator.Update(50, "Halfway done")
    ↓
Indicator.Update() checks started flag → true
    ↓
Delegates to barBackend.Update(50, "Halfway done")
    ↓
progressBarBackend.Update() checks isTTY → true
    ↓
Calls bar.Describe("Halfway done")
    ↓
Calls bar.Set(50)
    ↓
progressbar library updates terminal display:
    "Halfway done [============>        ] 50% (2m 30s remaining)"
```

### Update Flow (Non-TTY Mode)

```
Application calls indicator.Update(50, "Halfway done")
    ↓
Indicator.Update() checks started flag → true
    ↓
Delegates to barBackend.Update(50, "Halfway done")
    ↓
progressBarBackend.Update() checks isTTY → false
    ↓
Calculates percentage: (50 * 100) / 100 = 50
    ↓
Prints to stdout: "Halfway done 50%"
```

### Complete Flow

```
Application calls indicator.Complete("All files processed")
    ↓
Indicator.Complete() checks started → true
    ↓
Calls barBackend.Stop()
    ↓
progressBarBackend.Stop() calls bar.Finish() (TTY mode)
    ↓
Indicator.Complete() checks IsTTY() → true
    ↓
Prints: "✅ All files processed"
    ↓
Sets started = false
```

---

## Threading Model

**Single-Threaded by Design**: The progress package is **not thread-safe** and assumes single-threaded usage.

**Rationale**:
- Progress indicators represent sequential operations (one step after another)
- Concurrent progress updates would require complex synchronization
- Most use cases are single-threaded loops
- Adding mutex protection would add overhead for uncommon use case

**Recommendation for Concurrent Usage**:
- Use separate `Indicator` instances per goroutine
- OR synchronize calls to `Update()` with external mutex
- OR use single-threaded aggregator goroutine (fan-in pattern)

---

## Error Handling

**Philosophy**: Progress indication is **non-critical** and should never cause application failures.

**Error Handling Strategy**:
- **No errors returned**: All methods are `void` (no error returns)
- **Defensive programming**: Check nil pointers, started flag before operations
- **Graceful degradation**: If terminal width detection fails, use default (100)
- **Idempotency**: Multiple Start/Complete calls are safe (no panics)
- **Silent failures in backends**: Spinner/progressbar library errors are ignored (operation continues)

**Examples**:
- `Update()` before `Start()` → No-op (started flag check)
- `Complete()` without `Start()` → No-op (started flag check)
- Multiple `Start()` calls → First call starts, subsequent calls no-op
- Terminal width error → Use default 100 characters

---

## Testing Strategy

### Unit Tests (indicator_test.go)

**Test Coverage**:
- Mode selection (Total == 0 → Spinner, Total > 0 → ProgressBar)
- Idempotent operations (multiple Start/Complete calls)
- Phase formatting (UpdatePhase message construction)
- Default options validation

**Test Philosophy**:
- No mocking (backends are internal implementation details)
- Focus on public API contracts (mode selection, idempotency, formatting)
- No TTY-dependent tests (would require pty allocation)

### Manual Testing

**Required Manual Tests**:
- TTY mode: Verify animations and symbols display correctly
- Non-TTY mode: Run `go run main.go | cat`, verify plain text output
- CI/CD mode: Check GitHub Actions logs for ANSI codes (should be none)
- Terminal width: Test in narrow terminal (e.g., 40 chars wide)

**Test Scenarios**:
```bash
# TTY mode (interactive)
go run example.go

# Non-TTY mode (pipe)
go run example.go | cat

# Non-TTY mode (file redirect)
go run example.go > output.txt

# Non-TTY mode (CI/CD simulation)
CI=true go run example.go
```

---

## Dependencies

### External Dependencies

**github.com/briandowns/spinner**
- Used by: `spinnerBackend`
- Purpose: Animated spinner characters and rendering
- Why chosen: Well-maintained, standard library for spinner animations

**github.com/schollz/progressbar/v3**
- Used by: `progressBarBackend`
- Purpose: Progress bar rendering with ETA calculation
- Why chosen: Feature-rich (ETA, themes, width control), actively maintained

**golang.org/x/term**
- Used by: `tty.go`
- Purpose: Terminal detection and size measurement
- Why chosen: Official Go extended library, cross-platform support

### Dependency Isolation

**Strategy**: All external dependencies are isolated in backend implementations

**Benefits**:
- Users only interact with `Indicator` (no direct dependency exposure)
- Backends can be swapped without breaking public API
- Testing focuses on `Indicator` behavior, not backend internals

**Example**: If we switch from `schollz/progressbar` to custom implementation, only `progressbar.go` changes.

---

## Performance Considerations

### Memory Usage

**Indicator**: ~200 bytes (includes backend pointer, options, state)
**spinnerBackend**: ~1 KB (spinner library allocations)
**progressBarBackend**: ~2 KB (progressbar library allocations)

**Total per Indicator**: ~2-3 KB (negligible for most applications)

### CPU Usage

**Spinner Animation**: 100ms refresh rate → 10 updates/second → negligible CPU
**Progress Bar**: Updates on demand (application-controlled) → no background CPU

**Recommendation**: Safe to use in tight loops (thousands of updates/second)

### Terminal I/O

**TTY Mode**: Each Update() triggers terminal write (ANSI codes)
**Non-TTY Mode**: Each Update() triggers stdout write (plain text)

**Recommendation**: For very high-frequency updates (>1000/second), consider throttling updates (e.g., update every 100ms instead of every iteration)

---

## Future Enhancements

### Potential Improvements (Not in Scope for v1.0.0)

**Color Customization**:
- Currently: Fixed green (✅) and red (❌) symbols
- Future: Allow `Options.SuccessColor` and `Options.FailureColor`

**Multiple Concurrent Indicators**:
- Currently: Single indicator at a time
- Future: Stacked progress bars (like htop) or split-screen layout

**Progress History/Logging**:
- Currently: No history or logging
- Future: Optional logging of progress updates to file (debugging/monitoring)

**Dynamic Terminal Resizing**:
- Currently: Terminal width cached at initialization
- Future: Recalculate width on SIGWINCH signal

**Thread Safety**:
- Currently: Not thread-safe
- Future: Optional mutex protection (opt-in via `Options.ThreadSafe`)

---

## ADR References

See `docs/adrs/` directory for detailed decision records:

- **ADR-001**: Strategy Pattern for Mode Selection
- **ADR-002**: Automatic TTY Detection
- **ADR-003**: Idempotent Operations Design

---

## Version History

- **1.0.0** (2026-02-11): Initial implementation and architecture documentation backfill

---

**Maintained by**: Engram Core Team
**Questions**: See README.md for usage examples, SPEC.md for requirements
