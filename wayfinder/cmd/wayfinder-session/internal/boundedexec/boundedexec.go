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
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

	// progressQueue bounds how many progress lines may be in flight. A gate
	// emits a handful, so exceeding this means the writer is not draining.
	progressQueue = 64

	// progressDrainGrace bounds how long Run waits for queued progress lines to
	// reach the writer before returning without them.
	progressDrainGrace = 2 * time.Second
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

	// Setpgid takes the command out of Wayfinder's foreground process group, so
	// a terminal Ctrl-C no longer reaches it. Without this, interrupting a gate
	// would kill Wayfinder and leave the build or test running orphaned. Making
	// the context signal-aware routes the interrupt back through Cancel, which
	// kills the whole group.
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	// NotifyContext reports that a signal arrived but not which one, and the
	// difference is visible to whoever supervises Wayfinder: SIGTERM must not
	// come back as SIGINT. A second registration on the same signals records it.
	arrived := make(chan os.Signal, 1)
	signal.Notify(arrived, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(arrived)
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
	// Kill the whole process group on cancellation, not just the launcher we
	// started, so descendants do not outlive the gate that gave up on them.
	configureProcessGroup(cmd)
	// cancelled records that cancellation actually reached the process. Without
	// it, an ordinary non-zero exit that happens to land next to an expiring
	// deadline reads as a timeout, and the gate then advises raising the
	// timeout instead of showing the real build or test failure.
	var cancelled atomic.Bool
	cmd.Cancel = func() error {
		err := killProcessTree(cmd)
		// Only a kill that actually reached a live process counts. A process
		// that had already exited reports ErrProcessDone, and treating that as
		// proof of cancellation relabels an ordinary failure as a timeout on
		// platforms where the process state cannot tell the two apart.
		if err == nil {
			cancelled.Store(true)
		}
		return err
	}
	// Release Wait once the child is killed, even if a descendant still holds
	// the pipes. This is the difference between a bounded gate and a hang.
	cmd.WaitDelay = waitDelay

	out := &cappedWriter{limit: MaxOutputBytes}
	cmd.Stdout = out
	cmd.Stderr = out

	sink := newProgressSink(progress)
	defer sink.close()

	sink.emit("▶  %s: running `%s`%s\n", c.Label, c.commandLine(), c.timeoutSuffix())

	start := time.Now()
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		// A panic here must not take the gate down: this package exists to make
		// a gate bounded, and trading an indefinite wait for a crash is no fix.
		defer func() {
			if r := recover(); r != nil {
				sink.emit("!  %s: progress reporting stopped after a panic: %v\n", c.Label, r)
			}
		}()
		c.beat(done, sink, start, heartbeat)
	}()

	err := cmd.Run()
	// Joining the heartbeat is safe because the sink never blocks on the
	// caller's writer; a wedged writer costs dropped lines, never a stuck join.
	close(done)
	<-stopped
	elapsed := time.Since(start)

	timedOut, err := classifyOutcome(cmd.ProcessState, err, cancelled.Load(), ctx.Err())

	switch {
	case timedOut:
		sink.emit("⏱  %s: timed out after %s\n", c.Label, c.Timeout)
	case err == nil:
		sink.emit("✓  %s: completed in %s\n", c.Label, elapsed.Round(time.Second))
	default:
		sink.emit("✗  %s: failed after %s\n", c.Label, elapsed.Round(time.Second))
	}

	// An interrupt must end Wayfinder, not just this command. Consuming the
	// signal here and returning would let complete-phase treat the cancelled
	// review as an abstention and carry on to complete the phase, which is the
	// opposite of what the operator asked for. Restore the default disposition
	// and re-raise the signal that actually arrived.
	//
	// This runs after the terminal progress line, and re-raising is not assumed
	// to be fatal: a signal the process inherited as ignored comes straight back
	// here, and the caller still gets a well-formed result instead of a panic.
	if errors.Is(ctx.Err(), context.Canceled) {
		sink.close()
		stopSignals()
		signal.Stop(arrived)
		reraise(receivedSignal(arrived))
	}

	return Result{Output: out.String(), Elapsed: elapsed, TimedOut: timedOut, Err: err}
}

// receivedSignal reports the signal that cancelled the run, defaulting to
// os.Interrupt when the delivery was observed only through the context.
func receivedSignal(arrived <-chan os.Signal) os.Signal {
	select {
	case sig := <-arrived:
		return sig
	default:
		return os.Interrupt
	}
}

// progressSink serializes progress lines onto the caller's writer from a single
// goroutine and never lets that writer delay the command.
//
// A caller's writer can block indefinitely: Wayfinder's stderr redirected into
// a full, undrained pipe is enough. Writing progress inline would then hold the
// gate open through the very code added to keep it bounded, so lines are queued
// and dropped rather than waited on.
type progressSink struct {
	lines    chan string
	done     chan struct{}
	closeOne sync.Once
}

func newProgressSink(w io.Writer) *progressSink {
	s := &progressSink{lines: make(chan string, progressQueue), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		// The writer belongs to the caller. If it panics, drop the rest rather
		// than take the process down.
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "boundedexec: progress writer panicked: %v\n", r)
			}
		}()
		for line := range s.lines {
			fmt.Fprint(w, line)
		}
	}()
	return s
}

// emit queues a line, or drops it when the writer is not keeping up. Dropping
// is deliberate: a progress line is never worth blocking a gate on.
func (s *progressSink) emit(format string, args ...any) {
	select {
	case s.lines <- fmt.Sprintf(format, args...):
	default:
	}
}

// close stops the sink and waits a bounded time for queued lines to land. A
// wedged writer simply loses the tail.
// close is idempotent: the interrupt path closes the sink before re-raising,
// and Run's deferred close still runs afterwards.
func (s *progressSink) close() {
	s.closeOne.Do(func() {
		close(s.lines)
		select {
		case <-s.done:
		case <-time.After(progressDrainGrace):
		}
	})
}

// classifyOutcome recovers the command's real outcome and decides whether the
// deadline is what ended it.
//
// os/exec can replace a finished command's result with the context error: when
// cancellation races the exit, Cancel returns nil for a process that is already
// dead (killing its zombie group succeeds), and Wait then prefers ctx.Err() to
// the real status. So the returned error cannot be trusted to describe how the
// process ended. A recorded ProcessState reporting a normal exit can: a
// successful build stays successful, and a real failure keeps its exit code and
// its diagnostic instead of being relabelled a timeout.
func classifyOutcome(state *os.ProcessState, err error, cancelled bool, ctxErr error) (timedOut bool, outcome error) {
	// A launcher can exit cleanly while a descendant still holds the output
	// pipes, and Wait then gives up after WaitDelay. The command's own status
	// says success, but the gate never received its output and the descendant is
	// still running, so this must not be laundered into a pass.
	//
	// The process group is deliberately not killed here: Wait has already reaped
	// the child, so its PID no longer reliably names that group and could name
	// somebody else's.
	if errors.Is(err, exec.ErrWaitDelay) {
		return false, err
	}
	if state != nil && !endedByCancellation(state, cancelled) {
		if state.Success() {
			return false, nil
		}
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			return false, exitErr
		}
		return false, &exec.ExitError{ProcessState: state}
	}
	// No normal exit: the process was signalled, never started, or Wait gave up
	// on descendants holding the pipes. Only now can the deadline own the result.
	return err != nil && cancelled && errors.Is(ctxErr, context.DeadlineExceeded), err
}

// beat writes a liveness line every interval until done is closed, so a gate
// that legitimately takes minutes still shows that it is making progress.
func (c Command) beat(done <-chan struct{}, sink *progressSink, start time.Time, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			sink.emit("⏳ %s: still running (%s elapsed%s)\n",
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
