# ADR-002: Automatic TTY Detection

**Status**: Accepted
**Date**: 2026-02-11 (Backfilled)
**Deciders**: Engram Core Team
**Context**: Ensuring progress package works correctly in both interactive terminals and non-interactive environments (CI/CD, pipes, file redirects)

---

## Context

Terminal applications need to adapt their output based on the execution environment:

**Interactive Terminals (TTY)**:
- Support ANSI escape codes (colors, cursor movement)
- Can display animated spinners
- Can render progress bars
- Can use Unicode symbols (✅, ❌, ⏳)

**Non-Interactive Environments (Non-TTY)**:
- No ANSI support (codes appear as garbage characters)
- No animation support (spinners freeze or create repeated lines)
- Limited Unicode support (symbols may not render)
- Examples: CI/CD pipelines, piped output (`| cat`), file redirects (`> log.txt`)

**Problem**: How should the progress package detect the environment and adjust output accordingly?

---

## Decision

We will use **automatic TTY detection** via `golang.org/x/term.IsTerminal()` to determine output mode, with no manual configuration required from users.

**Key Design Elements**:

1. **Detection Mechanism**:
   ```go
   import "golang.org/x/term"

   func IsTTY() bool {
       return term.IsTerminal(int(os.Stdout.Fd()))
   }
   ```

2. **Detection Timing**: TTY detection happens **once** when creating backend (in `newSpinnerBackend` and `newProgressBarBackend`)

3. **Output Adaptation**:
   - **TTY mode**: Full features (ANSI codes, animations, Unicode symbols)
   - **Non-TTY mode**: Plain text only (no ANSI, no animations, ASCII-compatible)

4. **No Manual Override**: No environment variable or option to override detection (trust `IsTerminal()` result)

---

## Consequences

### Positive

**Zero Configuration**:
- Users don't need to set environment variables (e.g., `PROGRESS_MODE=plain`)
- No command-line flags required (e.g., `--no-color`)
- Code works correctly in all environments without changes

**Correct Detection**:
- Detects pipes: `./app | cat` → Non-TTY ✓
- Detects file redirects: `./app > log.txt` → Non-TTY ✓
- Detects CI/CD: GitHub Actions, GitLab CI → Non-TTY ✓
- Detects interactive terminals: `./app` → TTY ✓
- Detects SSH sessions: `ssh server ./app` → TTY ✓
- Detects tmux/screen: `tmux attach` → TTY ✓

**Clean Logs**:
- Non-TTY output is copy-paste friendly (no ANSI garbage)
- CI/CD logs are readable (no escape codes)
- Piped output is parseable (plain text percentages)

**Better User Experience**:
- Animations work when they should (terminal)
- Animations don't spam when they shouldn't (CI/CD)
- Symbols render correctly or use ASCII fallbacks

### Negative

**No Override Mechanism**:
- Users cannot force TTY mode in non-TTY environment
- Users cannot force non-TTY mode in TTY environment
- This is **acceptable** because:
  - Real-world use cases don't require override (IsTerminal() is accurate)
  - Adding override would add API complexity (flags, env vars)
  - If needed, users can redirect output: `./app | cat` forces non-TTY

**Performance Consideration**:
- `IsTerminal()` makes syscall (file descriptor check)
- We cache result in backend struct (called once, not per update)
- Negligible performance impact

**Static Detection**:
- TTY status is checked once at initialization, not per update
- If user redirects output mid-execution (rare), mode doesn't change
- This is **acceptable** because:
  - Mid-execution redirection is extremely rare
  - Indicator lifetime is typically short (single operation)
  - Overhead of checking per update is not justified

---

## Alternatives Considered

### Alternative 1: Environment Variable Configuration

**Approach**:
```bash
export PROGRESS_MODE=plain
./app
```

**Rejected Because**:
- Users must remember to set variable
- Variable conflicts across tools (different naming conventions)
- Adds configuration complexity for rare override use cases
- IsTerminal() detection is already accurate (override rarely needed)

### Alternative 2: Options-Based Override

**Approach**:
```go
progress.New(Options{
    Total: 100,
    ForcePlainText: true,
})
```

**Rejected Because**:
- Adds API surface for rare use case
- Users might set `ForcePlainText: true` even in terminals (defeats purpose)
- IsTerminal() is reliable enough that manual override is unnecessary

### Alternative 3: Check TERM Environment Variable

**Approach**:
```go
func IsTTY() bool {
    term := os.Getenv("TERM")
    return term != "" && term != "dumb"
}
```

**Rejected Because**:
- Less reliable than `IsTerminal()` syscall
- Doesn't detect pipes or file redirects
- `TERM` can be set in non-TTY environments (misleading)
- Example failure: `TERM=xterm ./app | cat` would incorrectly use TTY mode

### Alternative 4: Check isatty() Per Update

**Approach**:
```go
func (i *Indicator) Update(current int, message string) {
    if IsTTY() {
        // TTY mode
    } else {
        // Non-TTY mode
    }
}
```

**Rejected Because**:
- Syscall overhead on every update (hundreds or thousands per operation)
- TTY status rarely changes mid-execution
- Caching result in backend is more efficient

---

## Implementation Notes

**TTY Detection** (in `tty.go`):
```go
package progress

import (
    "os"
    "golang.org/x/term"
)

// IsTTY returns true if stdout is connected to a terminal.
// Returns false for pipes, file redirects, and CI/CD environments.
func IsTTY() bool {
    return term.IsTerminal(int(os.Stdout.Fd()))
}
```

**Caching in Backends** (example from `spinner.go`):
```go
type spinnerBackend struct {
    spinner *spinner.Spinner
    isTTY   bool  // ← Cached detection result
    message string
}

func newSpinnerBackend(opts Options) *spinnerBackend {
    isTTY := IsTTY()  // ← Detect once at creation

    backend := &spinnerBackend{
        isTTY:   isTTY,
        message: opts.Label,
    }

    if isTTY {
        // Create spinner (TTY mode)
        backend.spinner = spinner.New(spinner.CharSets[11], 100*time.Millisecond)
        backend.spinner.Suffix = " " + opts.Label
    } else {
        // Print initial message (non-TTY mode)
        if opts.Label != "" {
            fmt.Println(opts.Label)
        }
    }

    return backend
}
```

**Output Differences**:

| Operation | TTY Mode | Non-TTY Mode |
|-----------|----------|--------------|
| Spinner | Animated (CharSets[11]) | Initial message only |
| Progress Bar | `[=====>   ] 45% (2m)` | `45%` |
| Complete | `✅ Done` | `SUCCESS: Done` |
| Fail | `❌ Error: ...` | `ERROR: ...` |
| Update | Live refresh (cursor movement) | New line per update |

---

## Terminal Width Detection

**Related Decision**: Terminal width detection uses similar approach

**Implementation** (in `tty.go`):
```go
func GetTerminalWidth() int {
    if !IsTTY() {
        return 0  // No terminal, no width
    }

    width, _, err := term.GetSize(int(os.Stdout.Fd()))
    if err != nil || width == 0 {
        return 100  // Default to 100 chars on error
    }

    // Cap at 100 chars (user preference from ~/.claude/CLAUDE.md)
    if width > 100 {
        return 100
    }

    return width
}
```

**Why Cap at 100?**:
- User preference from `~/.claude/CLAUDE.md` (terminal-friendly width)
- Prevents excessively wide progress bars (hard to read)
- Consistent with engram's "~100 char" output guideline

---

## Testing Strategy

**Automated Testing** (unit tests):
- Cannot test TTY detection in standard `go test` (no pty allocation)
- Focus on testing backend behavior, not TTY detection itself
- Assume `IsTTY()` works correctly (trust `term.IsTerminal()` implementation)

**Manual Testing** (required):
```bash
# Test TTY mode (interactive terminal)
go run example.go

# Test non-TTY mode (pipe)
go run example.go | cat

# Test non-TTY mode (file redirect)
go run example.go > output.txt
cat output.txt  # Verify no ANSI codes

# Test non-TTY mode (CI/CD simulation)
CI=true go run example.go

# Test SSH session (should be TTY)
ssh server "cd /path && go run example.go"

# Test tmux (should be TTY)
tmux
go run example.go
```

---

## Related Decisions

- **ADR-001**: Strategy Pattern for Mode Selection (backends use TTY detection)
- **ADR-003**: Idempotent Operations Design (no TTY-dependent idempotency logic)

---

## References

- **Library**: `golang.org/x/term` (official Go extended library)
- **Documentation**: https://pkg.go.dev/golang.org/x/term#IsTerminal
- **User Preference**: `~/.claude/CLAUDE.md` (terminal width ~100 chars)

---

## Revision History

- **2026-02-11**: ADR created (backfilled from existing implementation)
