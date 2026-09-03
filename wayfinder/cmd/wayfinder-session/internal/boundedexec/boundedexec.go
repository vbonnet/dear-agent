// Package boundedexec runs the external commands that Wayfinder gates shell
// out to, under a wall-clock bound that actually holds.
//
// It exists because a context deadline is not by itself a bound.
// exec.CommandContext signals only the direct child; a descendant that
// inherited the output pipes keeps the write end open, so Cmd.Wait blocks
// until that descendant exits. A `go test ./...` run leaves reparented test
// binaries behind, which is how a documented ten-minute gate timeout turned
// into an indefinite hang in `wayfinder session complete-phase`.
//
// Every command run through this package is bounded, detached from stdin, and
// reports progress while it runs. A gate may be slow; it may never be silent.
package boundedexec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// DefaultWaitDelay bounds how long a killed command may keep the output
	// pipes open before Wait gives up on them. This is what converts a
	// deadline into a real bound.
	DefaultWaitDelay = 5 * time.Second

	// DefaultHeartbeat is how often a still-running command reports that it is
	// alive, so an operator never watches a blank terminal.
	DefaultHeartbeat = 30 * time.Second

	// MaxOutputBytes bounds what a single command may buffer, so a noisy build
	// or test run cannot exhaust the caller.
	MaxOutputBytes = 8 << 20
)

// Command describes one bounded external command.
type Command struct {
	Dir     string        // working directory
	Label   string        // human-readable name for progress lines, e.g. "Gate 9 build"
	Name    string        // executable
	Args    []string      // arguments, excluding argv[0]
	Timeout time.Duration // wall-clock bound; zero means no deadline

	// WaitDelay, Heartbeat and Progress are overridable for tests; zero values
	// select the package defaults, and a nil Progress writes to stderr.
	WaitDelay time.Duration
	Heartbeat time.Duration
	Progress  io.Writer
}

// Result is the outcome of one Command.
type Result struct {
	Output   string        // combined stdout and stderr, truncated at MaxOutputBytes
	Elapsed  time.Duration // how long the command actually ran
	TimedOut bool          // the wall-clock bound expired
	Err      error         // non-nil if the command failed, was killed, or could not start
}

// Run executes the command. It always returns: either the command finished, or
// the deadline expired and WaitDelay released the wait.
func (c Command) Run() Result {
	progress := c.Progress
	if progress == nil {
		progress = os.Stderr
	}
	heartbeat := c.Heartbeat
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeat
	}
	waitDelay := c.WaitDelay
	if waitDelay <= 0 {
		waitDelay = DefaultWaitDelay
	}

	ctx := context.Background()
	cancel := context.CancelFunc(func() {})
	if c.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
	}
	defer cancel()

	// Callers pass fixed per-language command tables, never user input.
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	// Explicitly detach stdin. A gate must never block waiting on a terminal
	// that a non-interactive host session does not have.
	cmd.Stdin = nil
	// Release Wait once the child is killed, even if a descendant still holds
	// the pipes. This is the difference between a bounded gate and a hang.
	cmd.WaitDelay = waitDelay

	out := &cappedWriter{limit: MaxOutputBytes}
	cmd.Stdout = out
	cmd.Stderr = out

	fmt.Fprintf(progress, "▶  %s: running `%s`%s\n", c.Label, c.commandLine(), c.timeoutSuffix())

	start := time.Now()
	done := make(chan struct{})
	go c.beat(done, progress, start, heartbeat)

	err := cmd.Run()
	close(done)
	elapsed := time.Since(start)

	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
	switch {
	case timedOut:
		fmt.Fprintf(progress, "⏱  %s: timed out after %s\n", c.Label, c.Timeout)
	case err == nil:
		fmt.Fprintf(progress, "✓  %s: completed in %s\n", c.Label, elapsed.Round(time.Second))
	default:
		fmt.Fprintf(progress, "✗  %s: failed after %s\n", c.Label, elapsed.Round(time.Second))
	}

	return Result{Output: out.String(), Elapsed: elapsed, TimedOut: timedOut, Err: err}
}

// beat writes a liveness line every interval until done is closed, so a gate
// that legitimately takes minutes still shows that it is making progress.
func (c Command) beat(done <-chan struct{}, progress io.Writer, start time.Time, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			fmt.Fprintf(progress, "⏳ %s: still running (%s elapsed%s)\n",
				c.Label, time.Since(start).Round(time.Second), c.timeoutRemainder())
		}
	}
}

// CommandLine renders the command the way an operator would retype it.
func (c Command) CommandLine() string { return c.commandLine() }

func (c Command) commandLine() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

func (c Command) timeoutSuffix() string {
	if c.Timeout <= 0 {
		return ""
	}
	return fmt.Sprintf(" (timeout %s)", c.Timeout)
}

func (c Command) timeoutRemainder() string {
	if c.Timeout <= 0 {
		return ""
	}
	return fmt.Sprintf(", timeout %s", c.Timeout)
}

// ExitCode reports the child's exit status, or -1 when it never exited
// normally (killed by the deadline, or failed to start).
func (r Result) ExitCode() int {
	if exitErr, ok := errors.AsType[*exec.ExitError](r.Err); ok {
		return exitErr.ExitCode()
	}
	if r.Err != nil {
		return -1
	}
	return 0
}

// cappedWriter accumulates output up to limit and drops the rest, reporting
// that it truncated. Gate commands can be very noisy and callers only ever
// quote the output back to an operator.
type cappedWriter struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if remaining := w.limit - w.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			w.buf.Write(p[:remaining])
			w.truncated = true
		} else {
			w.buf.Write(p)
		}
	} else if len(p) > 0 {
		w.truncated = true
	}
	return len(p), nil
}

func (w *cappedWriter) String() string {
	if w.truncated {
		return w.buf.String() + fmt.Sprintf("\n[output truncated at %d bytes]\n", w.limit)
	}
	return w.buf.String()
}
