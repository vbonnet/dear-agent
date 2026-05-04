// Package tmux provides tmux session management.
package tmux

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// OutputWatcher monitors tmux control mode output stream
// It handles octal-escaped characters and provides pattern matching
type OutputWatcher struct {
	scanner *bufio.Scanner
	buffer  []string // Recent output lines for debugging
	maxSize int      // Maximum buffer size
}

// NewOutputWatcher creates a watcher for tmux control mode output
func NewOutputWatcher(reader io.Reader) *OutputWatcher {
	return &OutputWatcher{
		scanner: bufio.NewScanner(reader),
		buffer:  make([]string, 0, 100),
		maxSize: 100,
	}
}

// WaitForPattern waits for a specific pattern in the output stream
// Handles octal-escaped characters in tmux %output notifications
// Returns nil when pattern is found, error on timeout or scanner error
func (w *OutputWatcher) WaitForPattern(pattern string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if !w.scanner.Scan() {
			if err := w.scanner.Err(); err != nil {
				return fmt.Errorf("scanner error: %w", err)
			}
			// EOF - no more output
			return fmt.Errorf("EOF reached while waiting for pattern: %s", pattern)
		}

		line := w.scanner.Text()
		w.addToBuffer(line)

		// Check for %output notifications (tmux control mode)
		if strings.HasPrefix(line, "%output") {
			// Extract and decode the output content
			content := ExtractOutputContent(line)
			if strings.Contains(content, pattern) {
				return nil // Pattern found!
			}
		}

		// Also check raw line (for non-%output patterns like %end, %error)
		if strings.Contains(line, pattern) {
			return nil
		}
	}

	return fmt.Errorf("timeout waiting for pattern: %s (waited %v)", pattern, timeout)
}

// addToBuffer adds a line to the buffer with size limit
func (w *OutputWatcher) addToBuffer(line string) {
	w.buffer = append(w.buffer, line)

	// Keep buffer size manageable
	if len(w.buffer) > w.maxSize {
		w.buffer = w.buffer[1:]
	}
}

// ExtractOutputContent extracts and unescapes content from %output line
// Example: "%output %0 hello\040world\012" → "hello world\n"
func ExtractOutputContent(line string) string {
	// Format: %output %pane_id content
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return ""
	}

	content := parts[2]

	// Unescape octal sequences
	return unescapeOctal(content)
}

// unescapeOctal converts tmux octal escapes to actual characters
// Example: "hello\040world\012" → "hello world\n"
// tmux uses \NNN format where NNN are octal digits (0-7)
func unescapeOctal(s string) string {
	var result strings.Builder
	i := 0

	for i < len(s) {
		if s[i] == '\\' && i+3 < len(s) {
			// Check if next 3 chars are octal digits
			octalStr := s[i+1 : i+4]
			if isOctal(octalStr) {
				// Parse octal to byte
				var code int
				_, err := fmt.Sscanf(octalStr, "%o", &code)
				if err == nil && code >= 0 && code <= 255 {
					result.WriteByte(byte(code)) //nolint:gosec // bounded above
					i += 4
					continue
				}
			}
		}
		result.WriteByte(s[i])
		i++
	}

	return result.String()
}

// isOctal checks if string contains only octal digits (0-7)
func isOctal(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '7' {
			return false
		}
	}
	return true
}

// GetRecentOutput returns the last N lines of output
// Useful for debugging when pattern matching fails
func (w *OutputWatcher) GetRecentOutput(n int) []string {
	if n > len(w.buffer) {
		n = len(w.buffer)
	}
	if n <= 0 {
		return []string{}
	}
	return w.buffer[len(w.buffer)-n:]
}

// ReadLine reads and returns the next line from the output stream
// Returns empty string and error if EOF or timeout
func (w *OutputWatcher) ReadLine(timeout time.Duration) (string, error) {
	// Create a channel for the scan result
	done := make(chan bool, 1)
	var line string

	go func() {
		if w.scanner.Scan() {
			line = w.scanner.Text()
			w.addToBuffer(line)
			done <- true
		} else {
			done <- false
		}
	}()

	select {
	case success := <-done:
		if success {
			return line, nil
		}
		if err := w.scanner.Err(); err != nil {
			return "", fmt.Errorf("scanner error: %w", err)
		}
		return "", fmt.Errorf("EOF")
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout reading line")
	}
}

// WaitForAnyPattern waits for any of the provided patterns
// Returns the matched pattern and nil error, or empty string and error on timeout
func (w *OutputWatcher) WaitForAnyPattern(patterns []string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if !w.scanner.Scan() {
			if err := w.scanner.Err(); err != nil {
				return "", fmt.Errorf("scanner error: %w", err)
			}
			return "", fmt.Errorf("EOF reached")
		}

		line := w.scanner.Text()
		w.addToBuffer(line)

		// Check for %output notifications
		var content string
		if strings.HasPrefix(line, "%output") {
			content = ExtractOutputContent(line)
		} else {
			content = line
		}

		// Check all patterns
		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				return pattern, nil
			}
		}
	}

	return "", fmt.Errorf("timeout waiting for any of %d patterns (waited %v)", len(patterns), timeout)
}

// GetRawLine reads next line without buffering to a goroutine
// This is simpler than ReadLine and useful for prompt detection
// Returns the raw line and any error (including timeout)
func (w *OutputWatcher) GetRawLine(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Use non-blocking scan with timeout check
		if w.scanner.Scan() {
			line := w.scanner.Text()
			w.addToBuffer(line)
			return line, nil
		}

		// Check for scanner error
		if err := w.scanner.Err(); err != nil {
			return "", fmt.Errorf("scanner error: %w", err)
		}

		// Small sleep to avoid tight loop
		time.Sleep(10 * time.Millisecond)

		// Check timeout
		if time.Now().After(deadline) {
			break
		}
	}

	return "", fmt.Errorf("timeout reading line (waited %v)", timeout)
}
