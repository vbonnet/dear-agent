package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/term"
)

// ProgressIndicator manages progress display (spinner or text)
type ProgressIndicator struct {
	spinner *spinner.Spinner
	isTTY   bool
	message string
}

// NewProgress creates a new progress indicator
// If running in TTY (interactive terminal), shows spinner
// If running in non-TTY (CI/logs), shows simple text
func NewProgress(message string) *ProgressIndicator {
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	p := &ProgressIndicator{
		isTTY:   isTTY,
		message: message,
	}

	if isTTY {
		// Interactive terminal - use spinner
		p.spinner = spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		p.spinner.Suffix = " " + message
	} else {
		// Non-interactive (CI) - print simple message
		fmt.Println(message)
	}

	return p
}

// Start begins showing progress
func (p *ProgressIndicator) Start() {
	if p.isTTY && p.spinner != nil {
		p.spinner.Start()
	}
}

// Update changes the progress message
func (p *ProgressIndicator) Update(message string) {
	p.message = message
	if p.isTTY && p.spinner != nil {
		p.spinner.Suffix = " " + message
	}
	// In non-TTY mode, don't print updates (avoid log spam)
}

// Stop halts the progress indicator
func (p *ProgressIndicator) Stop() {
	if p.isTTY && p.spinner != nil {
		p.spinner.Stop()
	}
}

// Complete stops progress and prints final success message
func (p *ProgressIndicator) Complete(message string) {
	p.Stop()
	PrintSuccess(message)
}

// Fail stops progress and prints error message
func (p *ProgressIndicator) Fail(message string) {
	p.Stop()
	PrintError(message)
}
