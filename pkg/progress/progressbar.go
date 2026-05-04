package progress

import (
	"fmt"

	"github.com/schollz/progressbar/v3"
)

// progressBarBackend implements determinate progress using a progress bar
type progressBarBackend struct {
	bar   *progressbar.ProgressBar
	isTTY bool
	total int
}

// newProgressBarBackend creates a new progress bar backend
func newProgressBarBackend(opts Options) *progressBarBackend {
	isTTY := IsTTY()

	backend := &progressBarBackend{
		isTTY: isTTY,
		total: opts.Total,
	}

	if isTTY {
		// Calculate bar width (respect ~100 char terminal width preference)
		termWidth := GetTerminalWidth()
		barWidth := termWidth - 40 // Reserve space for: "Phase X/Y: Name [...] 27% (2m)"
		if barWidth < 20 {
			barWidth = 20 // Minimum bar width
		}

		// Create progress bar with custom options
		backend.bar = progressbar.NewOptions(opts.Total,
			progressbar.OptionSetDescription(opts.Label),
			progressbar.OptionSetPredictTime(opts.ShowETA),
			progressbar.OptionShowCount(),
			progressbar.OptionSetWidth(barWidth),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
	} else if opts.Label != "" {
		// Non-TTY mode: Print initial message
		fmt.Println(opts.Label)
	}

	return backend
}

// Start initializes the progress bar (already created in newProgressBarBackend)
func (p *progressBarBackend) Start() {
	// Bar is already initialized in newProgressBarBackend
	// Nothing to do here
}

// Update sets the current progress value and optionally changes the description
// current: current step number (1-based)
// message: optional description/label update
func (p *progressBarBackend) Update(current int, message string) {
	if p.isTTY && p.bar != nil {
		// Update description if provided
		if message != "" {
			p.bar.Describe(message)
		}
		// Set current progress value
		p.bar.Set(current)
	} else if p.total > 0 {
		// Non-TTY mode: Print progress percentage
		pct := (current * 100) / p.total
		if message != "" {
			fmt.Printf("%s %d%%\n", message, pct)
		} else {
			fmt.Printf("%d%%\n", pct)
		}
	}
}

// Stop completes the progress bar
func (p *progressBarBackend) Stop() {
	if p.isTTY && p.bar != nil {
		p.bar.Finish()
	}
}
