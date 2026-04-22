// Package progress provides a unified API for displaying progress indicators
// (spinners and progress bars) in terminal applications.
//
// It automatically detects TTY vs non-TTY environments and adjusts output accordingly:
// - TTY mode: Full features (animated spinners, progress bars with colors)
// - Non-TTY mode: Plain text output (no ANSI codes, suitable for CI/CD and pipes)
//
// Example usage:
//
//	// Indeterminate progress (spinner)
//	p := progress.New(progress.Options{Label: "Loading..."})
//	p.Start()
//	// ... work ...
//	p.Complete("Done")
//
//	// Determinate progress (progress bar)
//	p := progress.New(progress.Options{Total: 100, Label: "Processing"})
//	p.Start()
//	for i := 0; i < 100; i++ {
//	    p.Update(i+1, "")
//	}
//	p.Complete("Finished")
//
//	// Phase tracking (e.g., Wayfinder phases)
//	p := progress.New(progress.Options{Total: 11, Label: "Wayfinder"})
//	p.Start()
//	for i := 0; i < 11; i++ {
//	    p.UpdatePhase(i+1, 11, fmt.Sprintf("Phase %s", phases[i]))
//	    // ... work ...
//	}
//	p.Complete("All phases complete")
package progress

// Options configures progress indicator behavior
type Options struct {
	// Total steps for determinate progress (progress bar)
	// If Total == 0, uses indeterminate progress (spinner)
	// If Total > 0, uses determinate progress (progress bar with percentage)
	Total int

	// Label is the description/message for the operation
	Label string

	// ShowETA enables estimated time to completion display (progress bar only)
	// Default: true
	ShowETA bool

	// ShowPercent enables percentage display (progress bar only)
	// Default: true
	ShowPercent bool
}

// DefaultOptions returns default configuration for progress indicators
func DefaultOptions() Options {
	return Options{
		Total:       0,
		Label:       "",
		ShowETA:     true,
		ShowPercent: true,
	}
}
