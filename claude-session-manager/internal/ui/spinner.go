package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Spinner provides a simple terminal spinner for long-running operations
type Spinner struct {
	frames  []string
	delay   time.Duration
	message string
	writer  io.Writer
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
	active  bool
}

// NewSpinner creates a new spinner with the given message
func NewSpinner(message string) *Spinner {
	return &Spinner{
		frames:  []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		delay:   100 * time.Millisecond,
		message: message,
		writer:  os.Stdout,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

// Start begins the spinner animation
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.mu.Unlock()

	go s.run()
}

// Stop stops the spinner and clears the line
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	close(s.stop)
	<-s.done // Wait for goroutine to finish

	// Clear the line
	fmt.Fprintf(s.writer, "\r\033[K")
}

// Success stops the spinner and prints a success message
func (s *Spinner) Success(message string) {
	s.Stop()
	fmt.Fprintf(s.writer, "✅ %s\n", message)
}

// Warning stops the spinner and prints a warning message
func (s *Spinner) Warning(message string) {
	s.Stop()
	fmt.Fprintf(s.writer, "⚠️  %s\n", message)
}

// Error stops the spinner and prints an error message
func (s *Spinner) Error(message string) {
	s.Stop()
	fmt.Fprintf(s.writer, "❌ %s\n", message)
}

// UpdateMessage changes the spinner message while it's running
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	s.message = message
	s.mu.Unlock()
}

func (s *Spinner) run() {
	defer close(s.done)

	i := 0
	ticker := time.NewTicker(s.delay)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			s.mu.Lock()
			s.active = false
			s.mu.Unlock()
			return
		case <-ticker.C:
			s.mu.Lock()
			frame := s.frames[i%len(s.frames)]
			msg := s.message
			s.mu.Unlock()

			fmt.Fprintf(s.writer, "\r%s %s", frame, msg)
			i++
		}
	}
}
