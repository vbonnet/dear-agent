// Package stophook provides shared utilities for harness stop and completion hooks.
package stophook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Input is the normalized JSON structure received from a harness hook on stdin.
type Input struct {
	Harness        string `json:"harness,omitempty"`
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	StopReason     string `json:"stop_reason"`
	Cwd            string `json:"cwd"`
}

type wireInput struct {
	Harness        string `json:"harness"`
	SessionID      string `json:"session_id"`
	ThreadID       string `json:"thread_id"`
	ConversationID string `json:"conversation_id"`
	TranscriptPath string `json:"transcript_path"`
	Transcript     string `json:"transcript"`
	StopReason     string `json:"stop_reason"`
	Reason         string `json:"reason"`
	Cwd            string `json:"cwd"`
	WorkspaceDir   string `json:"workspace_dir"`
	ProjectDir     string `json:"project_dir"`
}

// ReadInput reads and parses the Stop hook JSON input from stdin.
func ReadInput(r io.Reader) (*Input, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}
	var wire wireInput
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}
	return &Input{
		Harness:        wire.Harness,
		SessionID:      firstHookValue(wire.SessionID, wire.ThreadID, wire.ConversationID),
		TranscriptPath: firstHookValue(wire.TranscriptPath, wire.Transcript),
		StopReason:     firstHookValue(wire.StopReason, wire.Reason),
		Cwd:            firstHookValue(wire.Cwd, wire.WorkspaceDir, wire.ProjectDir),
	}, nil
}

func firstHookValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// RunWithTimeout executes a hook function with a timeout.
// Returns exit code 0 on timeout (fail open).
func RunWithTimeout(timeout time.Duration, fn func() int) int {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan int, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "[stop-hook] panic in hook function: %v\n", r)
				done <- 1
			}
		}()
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
