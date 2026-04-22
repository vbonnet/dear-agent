# Progress Package - Specification

**Version**: 1.0.0
**Last Updated**: 2026-02-11
**Status**: Implemented
**Package**: github.com/vbonnet/engram/core/pkg/progress

---

## Vision

The progress package provides a unified, user-friendly API for displaying progress indicators in terminal applications. It solves the problem of managing different output modes (TTY vs non-TTY) and progress types (spinner vs progress bar) with a single, consistent interface.

Terminal applications need to provide feedback during long-running operations, but must adapt their output based on the environment (interactive terminal, CI/CD pipeline, or file redirect). This package abstracts that complexity, allowing developers to focus on their application logic while providing appropriate progress feedback in any environment.

---

## Goals

### 1. Unified Progress API

Provide a single, consistent API for all progress indication needs, eliminating the need for developers to choose between spinner and progress bar libraries.

**Success Metric**: Developers can use a single `Indicator` type for both indeterminate (spinner) and determinate (progress bar) operations, with automatic mode selection based on options.

### 2. Automatic TTY Detection

Automatically detect terminal vs non-terminal environments and adjust output accordingly without developer intervention.

**Success Metric**: Output automatically switches between rich formatting (ANSI codes, animations) in terminals and plain text in CI/CD environments without code changes.

### 3. Zero Breaking Changes for Users

When switching from TTY to non-TTY environments (e.g., piping to a file), no panics, errors, or garbled output occur.

**Success Metric**: All operations are idempotent and safe in both TTY and non-TTY modes. Applications work correctly when run interactively, in CI/CD, or when output is redirected.

### 4. Wayfinder Phase Support

Provide first-class support for multi-phase workflows like Wayfinder (W0-S11), with clear phase tracking and progress display.

**Success Metric**: `UpdatePhase()` method formats phase information consistently (e.g., "Phase 3/11: D2 - Existing Solutions") and integrates seamlessly with progress bars.

---

## Architecture

### High-Level Design

The package uses a **strategy pattern** to switch between two backends (spinner and progress bar) based on configuration. A single `Indicator` type provides a unified interface, delegating to the appropriate backend at runtime.

```
┌─────────────────┐
│   Application   │
└────────┬────────┘
         │ Uses
         v
    ┌────────────┐
    │ Indicator  │──────┐ Owns
    └────┬───────┘      │
         │ Delegates    │
         │              v
    ┌────┴──────┐    ┌──────────────┐
    │  Spinner  │    │ Progress Bar │
    │  Backend  │    │   Backend    │
    └───────────┘    └──────────────┘
         │                  │
         v                  v
    ┌────────────────────────────┐
    │ TTY Detection (IsTTY)      │
    └────────────────────────────┘
```

### Components

**Component 1: Indicator**
- **Purpose**: Unified progress indicator interface
- **Responsibilities**:
  - Mode selection (spinner vs progress bar) based on `Options.Total`
  - Lifecycle management (Start, Update, Complete, Fail)
  - Delegation to appropriate backend
  - Idempotency guarantees (safe multiple calls to Start/Complete)
- **Interfaces**: Public API used by applications (`New`, `Start`, `Update`, `UpdatePhase`, `Complete`, `Fail`)

**Component 2: Spinner Backend**
- **Purpose**: Indeterminate progress display (unknown duration)
- **Responsibilities**:
  - Animated spinner in TTY mode (using briandowns/spinner)
  - Plain text output in non-TTY mode
  - Label updates via `Update()` method
- **Interfaces**: Internal backend interface (`Start`, `Update`, `Stop`)

**Component 3: Progress Bar Backend**
- **Purpose**: Determinate progress display (known total steps)
- **Responsibilities**:
  - Progress bar with percentage and ETA in TTY mode (using schollz/progressbar)
  - Percentage output in non-TTY mode
  - Current step tracking and completion calculation
  - Terminal width detection and bar scaling
- **Interfaces**: Internal backend interface (`Start`, `Update`, `Stop`)

**Component 4: TTY Detection**
- **Purpose**: Environment detection (terminal vs non-terminal)
- **Responsibilities**:
  - Detect if stdout is connected to a terminal
  - Measure terminal width for progress bar scaling
  - Cap width at 100 characters (user preference)
- **Interfaces**: `IsTTY() bool`, `GetTerminalWidth() int`

### Data Flow

1. **Initialization**: Application calls `progress.New(options)`
   - Indicator inspects `options.Total` to select mode
   - Creates appropriate backend (spinner or progress bar)
   - Backend checks TTY status to determine output format

2. **Execution**: Application calls lifecycle methods
   - `Start()` → Backend starts animation (TTY) or prints initial message (non-TTY)
   - `Update()` → Backend updates display (TTY) or prints percentage (non-TTY)
   - `UpdatePhase()` → Formats phase message and calls `Update()`
   - `Complete()` / `Fail()` → Backend stops, prints final message with status icon

3. **TTY vs Non-TTY Handling**:
   - **TTY**: Full features (ANSI codes, animations, colors, symbols ✅/❌)
   - **Non-TTY**: Plain text only (no ANSI codes, "SUCCESS:" / "ERROR:" prefixes)

### Key Design Decisions

- **Decision: Strategy Pattern for Mode Selection** (See ADR-001)
  - Single `Indicator` type with two backend implementations
  - Mode selected at initialization based on `Options.Total`
  - Avoids exposing separate Spinner/ProgressBar types to users

- **Decision: Automatic TTY Detection** (See ADR-002)
  - Use `golang.org/x/term.IsTerminal()` instead of environment variables
  - Detects pipes, file redirects, and CI/CD environments automatically
  - No manual configuration required from users

- **Decision: Idempotent Operations** (See ADR-003)
  - Multiple calls to `Start()` are safe (no-op if already started)
  - `Complete()`/`Fail()` without `Start()` are safe (no-op)
  - Prevents panics from incorrect usage patterns

---

## Success Metrics

### Primary Metrics

- **API Simplicity**: 5 public methods (`New`, `Start`, `Update`, `UpdatePhase`, `Complete`, `Fail`)
- **Zero Configuration**: No environment-specific configuration required
- **No Runtime Errors**: All operations are safe and idempotent

### Secondary Metrics

- **TTY Detection Accuracy**: Correctly identifies terminal vs non-terminal in all environments (interactive, SSH, CI/CD, pipes, redirects)
- **Test Coverage**: ≥80% coverage on core logic (mode selection, idempotency, phase formatting)
- **Documentation**: README with examples for all use cases (spinner, progress bar, phases, errors)

---

## What This SPEC Doesn't Cover

- **Custom Spinner Animations**: Uses standard spinner character set (engram standard: CharSets[11])
- **Custom Progress Bar Themes**: Uses standard theme (= for progress, spaces for padding)
- **Multi-Line Progress**: Single line of output per indicator
- **Concurrent Indicators**: One indicator at a time (no multiplexing)
- **Programmatic Progress History**: No logging or storage of progress updates
- **Custom TTY Detection**: No override mechanism for TTY detection

Future considerations:
- Color customization (currently fixed: green ✅, red ❌)
- Multiple concurrent indicators (stacked or split-screen)
- Progress history/logging for debugging

---

## Assumptions & Constraints

### Assumptions

- Applications run in environments with stdout available
- Terminal width is stable during execution (doesn't handle dynamic resizing)
- Progress updates are called from single goroutine (not thread-safe by design)
- CI/CD environments are detected as non-TTY (GitHub Actions, GitLab CI, etc.)

### Constraints

- **Dependency Constraints**:
  - Requires `github.com/briandowns/spinner` for spinner animations
  - Requires `github.com/schollz/progressbar/v3` for progress bars
  - Requires `golang.org/x/term` for TTY detection
- **Output Constraints**:
  - Maximum terminal width capped at 100 characters
  - Progress bar requires minimum 20 character width
  - Non-TTY mode prints to stdout (not configurable)
- **Design Constraints**:
  - Single-threaded usage (no mutex protection)
  - No custom backend implementations exposed

---

## Dependencies

### External Libraries

- `github.com/briandowns/spinner` - Animated spinners for indeterminate progress
- `github.com/schollz/progressbar/v3` - Progress bars with ETA calculation
- `golang.org/x/term` - Terminal detection and width measurement

### Internal Dependencies

- None (self-contained package within engram core)

---

## API Reference

### Types

```go
type Indicator struct {
    // Internal: mode, opts, backends, state
}

type Options struct {
    Total       int    // Total steps (0 = spinner, >0 = progress bar)
    Label       string // Description/label
    ShowETA     bool   // Show ETA (default: true)
    ShowPercent bool   // Show percentage (default: true)
}

type Mode int
const (
    ModeSpinner Mode = iota
    ModeProgressBar
)
```

### Functions

```go
// New creates a progress indicator (mode selected by opts.Total)
func New(opts Options) *Indicator

// DefaultOptions returns default configuration
func DefaultOptions() Options

// IsTTY detects terminal vs non-terminal environment
func IsTTY() bool

// GetTerminalWidth returns terminal width (capped at 100)
func GetTerminalWidth() int
```

### Methods

```go
// Start begins displaying progress (idempotent)
func (i *Indicator) Start()

// Update updates progress
// - Spinner mode: message updates label (current ignored)
// - Progress bar mode: current is step number, message is description
func (i *Indicator) Update(current int, message string)

// UpdatePhase formats phase information (e.g., "Phase 3/11: Implementation")
func (i *Indicator) UpdatePhase(current, total int, name string)

// Complete stops progress and shows success message
func (i *Indicator) Complete(message string)

// Fail stops progress and shows error message
func (i *Indicator) Fail(err error)
```

---

## Testing Strategy

### Unit Tests

- Mode selection (Total == 0 → Spinner, Total > 0 → ProgressBar)
- Idempotent operations (multiple Start/Complete calls)
- Phase formatting (`UpdatePhase` message construction)
- Default options validation

### Manual Tests

- TTY mode: Verify animations and symbols display correctly
- Non-TTY mode: Verify plain text output (pipe to file, check content)
- CI/CD mode: Verify no ANSI codes in GitHub Actions logs
- Terminal width scaling: Test narrow vs wide terminals

---

## Version History

- **1.0.0** (2026-02-11): Initial implementation and documentation backfill

---

**Note**: This package is in active use by engram core and ai-tools projects. Changes must maintain backward compatibility. See ARCHITECTURE.md for detailed design and ADRs for decision rationale.
