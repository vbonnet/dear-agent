package progress

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
)

// spinnerBackend implements indeterminate progress using an animated spinner
type spinnerBackend struct {
	spinner *spinner.Spinner
	isTTY   bool
	message string
}

// newSpinnerBackend creates a new spinner backend
func newSpinnerBackend(opts Options) *spinnerBackend {
	isTTY := IsTTY()

	backend := &spinnerBackend{
		isTTY:   isTTY,
		message: opts.Label,
	}

	if isTTY {
		// Use engram's standard spinner configuration
		// CharSets[11] is the standard spinner style
		// 100ms refresh rate for smooth animation
		backend.spinner = spinner.New(spinner.CharSets[11], 100*time.Millisecond)
		backend.spinner.Suffix = " " + opts.Label
	} else {
		// Non-TTY mode: Print initial message once
		if opts.Label != "" {
			fmt.Println(opts.Label)
		}
	}

	return backend
}

// Start begins the spinner animation (TTY mode only)
func (s *spinnerBackend) Start() {
	if s.isTTY && s.spinner != nil {
		s.spinner.Start()
	}
}

// Update changes the spinner message/label
// In TTY mode: updates spinner suffix
// In non-TTY mode: silent (avoids log spam)
func (s *spinnerBackend) Update(message string) {
	if message == "" {
		return
	}
	s.message = message

	if s.isTTY && s.spinner != nil {
		s.spinner.Suffix = " " + message
	}
	// Non-TTY: Silent updates (avoid flooding CI logs)
}

// Stop halts the spinner animation
func (s *spinnerBackend) Stop() {
	if s.isTTY && s.spinner != nil {
		s.spinner.Stop()
	}
}
