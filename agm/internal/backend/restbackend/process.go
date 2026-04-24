package restbackend

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// ProcessState represents the lifecycle state of a managed process.
type ProcessState string

const (
	ProcessStateStarting ProcessState = "starting"
	ProcessStateRunning  ProcessState = "running"
	ProcessStateStopped  ProcessState = "stopped"
	ProcessStateCrashed  ProcessState = "crashed"
)

// managedProcess wraps an os/exec.Cmd with stdin/stdout pipes and state tracking.
type managedProcess struct {
	mu     sync.Mutex
	id     string
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	state  atomic.Value // ProcessState
	output *ringBuffer
	cancel context.CancelFunc
	done   chan struct{} // closed when process exits
	err    error        // exit error, if any
}

// streamMessage is the JSON format for sending messages to Claude via stdin.
type streamMessage struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// streamEvent is the JSON format for output events from Claude's stdout.
type streamEvent struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	Text    string `json:"text,omitempty"`
	Tool    string `json:"tool,omitempty"`
}

// spawnProcess creates and starts a new managed process.
func spawnProcess(ctx context.Context, id, name, claudePath, workdir, model string, env []string) (*managedProcess, error) {
	args := []string{
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
	}
	if workdir != "" {
		args = append(args, "-C", workdir)
	}
	if model != "" {
		args = append(args, "--model", model)
	}

	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, claudePath, args...)
	if len(env) > 0 {
		cmd.Env = env
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	proc := &managedProcess{
		id:     id,
		name:   name,
		cmd:    cmd,
		stdin:  stdinPipe,
		stdout: stdoutPipe,
		output: newRingBuffer(1000),
		cancel: cancel,
		done:   make(chan struct{}),
	}
	proc.state.Store(ProcessStateStarting)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start process: %w", err)
	}

	proc.state.Store(ProcessStateRunning)

	// Read stdout in background
	go proc.readOutputLoop()

	// Wait for process exit in background
	go func() {
		waitErr := cmd.Wait()
		proc.mu.Lock()
		proc.err = waitErr
		proc.mu.Unlock()
		if waitErr != nil {
			proc.state.Store(ProcessStateCrashed)
		} else {
			proc.state.Store(ProcessStateStopped)
		}
		close(proc.done)
	}()

	return proc, nil
}

// sendMessage writes a JSON message to the process's stdin.
func (p *managedProcess) sendMessage(msg string) error {
	data, err := json.Marshal(streamMessage{
		Type:    "user_message",
		Content: msg,
	})
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	data = append(data, '\n')

	p.mu.Lock()
	defer p.mu.Unlock()

	if _, err := p.stdin.Write(data); err != nil {
		return fmt.Errorf("write to stdin: %w", err)
	}
	return nil
}

// readOutput returns the last n lines of output.
func (p *managedProcess) readOutput(lines int) string {
	items := p.output.ReadLast(lines)
	result := ""
	for i, item := range items {
		if i > 0 {
			result += "\n"
		}
		result += item
	}
	return result
}

// isAlive returns true if the process is still running.
func (p *managedProcess) isAlive() bool {
	s := p.state.Load().(ProcessState)
	return s == ProcessStateRunning || s == ProcessStateStarting
}

// stop terminates the process.
func (p *managedProcess) stop() error {
	p.cancel()
	select {
	case <-p.done:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for process %s to exit", p.id)
	}
}

// readOutputLoop reads JSON events from stdout and stores them.
func (p *managedProcess) readOutputLoop() {
	scanner := bufio.NewScanner(p.stdout)
	// Allow larger lines (1MB)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		var event streamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Store raw line if not valid JSON
			p.output.Write(line)
			continue
		}

		// Store a human-readable version
		switch event.Type {
		case "assistant":
			if event.Text != "" {
				p.output.Write(event.Text)
			}
		case "result":
			p.output.Write("[result] " + event.Subtype)
		default:
			p.output.Write(line)
		}
	}
}
