# Progress - User Progress Display System

A unified Go package for displaying progress indicators (spinners and progress bars) in terminal applications. Automatically detects TTY vs non-TTY environments and adjusts output accordingly.

## Features

- **Spinner** for indeterminate operations (unknown duration)
- **Progress bar** for determinate operations (known total steps)
- **Phase indicators** for multi-step workflows (e.g., "Phase 3/11: Implementation")
- **TTY-aware**: Full features in terminal, plain text in CI/CD and pipes
- **ETA calculation**: Estimated time to completion for progress bars
- **Simple API**: 5 methods (New, Start, Update, UpdatePhase, Complete/Fail)

## Installation

```go
import "github.com/vbonnet/engram/core/pkg/progress"
```

## Usage

### Spinner (Indeterminate Progress)

For operations with unknown duration:

```go
p := progress.New(progress.Options{Label: "Searching knowledge base..."})
p.Start()
// ... long operation ...
p.Complete("Found 42 results")
```

**Output (TTY)**:
```
⏳ Searching knowledge base...
✅ Found 42 results
```

**Output (non-TTY)**:
```
Searching knowledge base...
SUCCESS: Found 42 results
```

### Progress Bar (Determinate Progress)

For operations with known total steps:

```go
p := progress.New(progress.Options{Total: 100, Label: "Processing files"})
p.Start()
for i := 0; i < 100; i++ {
    p.Update(i+1, "")
    // ... process file ...
}
p.Complete("All files processed")
```

**Output (TTY)**:
```
Processing files [=====>    ] 45% (2m 30s remaining)
✅ All files processed
```

**Output (non-TTY)**:
```
Processing files
45%
100%
SUCCESS: All files processed
```

### Phase Indicators (Multi-Step Workflows)

For workflows like Wayfinder phases (W0-S11):

```go
phases := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}
p := progress.New(progress.Options{Total: len(phases), Label: "Wayfinder"})
p.Start()

for i, phase := range phases {
    p.UpdatePhase(i+1, len(phases), fmt.Sprintf("%s - %s", phase, phaseName))
    // ... execute phase ...
}

p.Complete(fmt.Sprintf("All phases complete (%d/%d)", len(phases), len(phases)))
```

**Output (TTY)**:
```
⏳ Phase 3/11: D2 - Existing Solutions [=====>    ] 27% (6m remaining)
✅ All phases complete (11/11)
```

**Output (non-TTY)**:
```
Wayfinder
Phase 1/11: W0 - Project Framing 9%
Phase 2/11: D1 - Problem Validation 18%
Phase 3/11: D2 - Existing Solutions 27%
...
SUCCESS: All phases complete (11/11)
```

### Error Handling

```go
p := progress.New(progress.Options{Label: "Uploading file"})
p.Start()

err := uploadFile()
if err != nil {
    p.Fail(err)
    return err
}

p.Complete("File uploaded successfully")
```

**Output (TTY, on error)**:
```
⏳ Uploading file
❌ Error: network timeout
```

## API Reference

### `Options`

```go
type Options struct {
    Total       int    // Total steps (0 = spinner, >0 = progress bar)
    Label       string // Description/label
    ShowETA     bool   // Show ETA (default: true)
    ShowPercent bool   // Show percentage (default: true)
}
```

### `New(opts Options) *Indicator`

Creates a new progress indicator.
- If `opts.Total == 0`: Uses spinner (indeterminate)
- If `opts.Total > 0`: Uses progress bar (determinate)

### `Start()`

Begins displaying progress. Idempotent (safe to call multiple times).

### `Update(current int, message string)`

Updates progress.
- **Spinner mode**: `message` updates label (current ignored)
- **Progress bar mode**: `current` is step number (1-based), `message` updates description

### `UpdatePhase(current, total int, name string)`

Updates with phase information. Formats as "Phase current/total: name".

Example:
```go
p.UpdatePhase(3, 11, "Implementation")
// Displays: "Phase 3/11: Implementation"
```

### `Complete(message string)`

Stops progress and shows success message.
- **TTY**: Shows ✅ symbol
- **Non-TTY**: Shows "SUCCESS:" prefix

### `Fail(err error)`

Stops progress and shows error message.
- **TTY**: Shows ❌ symbol
- **Non-TTY**: Shows "ERROR:" prefix

## TTY Detection

The package automatically detects terminal vs non-terminal environments:

| Environment | Detected as | Output |
|-------------|-------------|--------|
| Interactive terminal | TTY | Full features (spinner/bar, colors, ✅/❌) |
| Pipe (`\| cat`) | Non-TTY | Plain text, no ANSI codes |
| File redirect (`> log.txt`) | Non-TTY | Plain text |
| CI/CD (GitHub Actions) | Non-TTY | Plain text |
| SSH session | TTY | Full features |
| tmux/screen | TTY | Full features |

## Terminal Width

Progress bars respect terminal width (capped at 100 characters by default). The bar automatically scales to fit narrow terminals.

## Dependencies

- `github.com/briandowns/spinner` - Animated spinners
- `github.com/schollz/progressbar/v3` - Progress bars with ETA
- `golang.org/x/term` - TTY detection

## License

Same as engram core (check repository LICENSE file).
