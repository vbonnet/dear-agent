// Package stophook provides shared utilities for Claude Code Stop hooks.
package stophook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Input is the JSON structure received from Claude Code Stop hooks on stdin.
type Input struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	StopReason     string `json:"stop_reason"`
	Cwd            string `json:"cwd"`
}

// ReadInput reads and parses the Stop hook JSON input from stdin.
func ReadInput(r io.Reader) (*Input, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}
	return &input, nil
}

// RunWithTimeout executes a hook function with a timeout.
// Returns exit code 0 on timeout (fail open).
func RunWithTimeout(timeout time.Duration, fn func() int) int {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan int, 1)
	go func() {
		done <- fn()
	}()

	select {
	case code := <-done:
		return code
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr, "[stop-hook] timed out, allowing exit")
		return 0
	}
}
