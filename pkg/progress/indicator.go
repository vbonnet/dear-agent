package progress

import "fmt"

// Mode defines the progress indicator mode
type Mode int

const (
	// ModeSpinner indicates indeterminate progress (animated spinner)
	ModeSpinner Mode = iota

	// ModeProgressBar indicates determinate progress (progress bar with percentage)
	ModeProgressBar
)

// Indicator is the main progress indicator interface
type Indicator struct {
	mode           Mode
	opts           Options
	spinnerBackend *spinnerBackend
	barBackend     *progressBarBackend
	started        bool
}

// New creates a new progress indicator.
// Mode selection:
//   - If opts.Total == 0: Uses spinner (indeterminate progress)
//   - If opts.Total > 0: Uses progress bar (determinate progress)
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

// Start begins displaying progress.
// Idempotent: safe to call multiple times (no-op if already started).
func (i *Indicator) Start() {
	if i.started {
		return // Already started
	}
	i.started = true

	if i.mode == ModeSpinner {
		i.spinnerBackend.Start()
	} else {
		i.barBackend.Start()
	}
}

// Update updates the progress indicator.
// For spinner mode: message updates the label (current value ignored)
// For progress bar mode: current is the step number (1-based), message updates description
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

// UpdatePhase updates progress with phase information.
// Formats the message as "Phase current/total: name" and calls Update().
//
// Example:
//
//	p.UpdatePhase(3, 11, "Implementation")
//	// Displays: "Phase 3/11: Implementation"
func (i *Indicator) UpdatePhase(current, total int, name string) {
	message := fmt.Sprintf("Phase %d/%d: %s", current, total, name)
	i.Update(current, message)
}

// Complete stops the progress indicator and displays a success message.
// In TTY mode: Shows ✅ symbol
// In non-TTY mode: Shows "SUCCESS:" prefix
// Idempotent: safe to call multiple times (no-op if not started).
func (i *Indicator) Complete(message string) {
	if !i.started {
		return
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

	i.started = false
}

// Fail stops the progress indicator and displays an error message.
// In TTY mode: Shows ❌ symbol
// In non-TTY mode: Shows "ERROR:" prefix
// Idempotent: safe to call multiple times (no-op if not started).
func (i *Indicator) Fail(err error) {
	if !i.started {
		return
	}

	// Stop backend
	if i.mode == ModeSpinner {
		i.spinnerBackend.Stop()
	} else {
		i.barBackend.Stop()
	}

	// Print error message
	if IsTTY() {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Printf("ERROR: %v\n", err)
	}

	i.started = false
}
