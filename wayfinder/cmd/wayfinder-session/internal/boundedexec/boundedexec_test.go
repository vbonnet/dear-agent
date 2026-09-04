package boundedexec

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRunReturnsWhenDescendantHoldsPipes is the regression test for the
// `wayfinder session complete-phase` hang: `go test ./...` leaves test binaries
// reparented to init that still hold the inherited output pipes, so killing the
// direct child at the deadline is not enough. Without WaitDelay, Wait blocks
// until the descendant exits on its own and the documented timeout never fires.
func TestRunReturnsWhenDescendantHoldsPipes(t *testing.T) {
	t.Parallel()

	// `sleep 30 &` inherits the output pipe and survives the kill of its
	// parent shell, exactly like an orphaned `go test` binary.
	cmd := Command{
		Label:     "descendant holdover",
		Name:      "sh",
		Args:      []string{"-c", "sleep 30 & sleep 30"},
		Timeout:   300 * time.Millisecond,
		WaitDelay: 300 * time.Millisecond,
		Heartbeat: time.Hour,
		Progress:  &bytes.Buffer{},
	}

	done := make(chan Result, 1)
	go func() { done <- cmd.Run() }()

	select {
	case res := <-done:
		if !res.TimedOut {
			t.Fatalf("expected TimedOut, got %+v", res)
		}
		if res.Err == nil {
			t.Fatal("expected an error for a killed command")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return: the deadline failed to release the wait")
	}
}

// TestRunEmitsProgress proves a slow gate is never silent.
func TestRunEmitsProgress(t *testing.T) {
	t.Parallel()

	var progress bytes.Buffer
	res := Command{
		Label:     "Gate 9 test",
		Name:      "sh",
		Args:      []string{"-c", "sleep 1"},
		Timeout:   30 * time.Second,
		Heartbeat: 100 * time.Millisecond,
		Progress:  &progress,
	}.Run()

	if res.Err != nil {
		t.Fatalf("unexpected error: %v (output %q)", res.Err, res.Output)
	}

	got := progress.String()
	for _, want := range []string{"Gate 9 test", "running", "still running", "completed in"} {
		if !strings.Contains(got, want) {
			t.Errorf("progress output missing %q; got:\n%s", want, got)
		}
	}
}

// TestRunProgressWriterIsSerialized pins that the heartbeat goroutine and the
// terminal line never write to the progress writer at the same time. A fast
// heartbeat over a short command makes the two collide reliably under -race.
func TestRunProgressWriterIsSerialized(t *testing.T) {
	t.Parallel()

	for range 5 {
		var progress bytes.Buffer
		res := Command{
			Label:     "racy heartbeat",
			Name:      "sh",
			Args:      []string{"-c", "sleep 0.2"},
			Timeout:   10 * time.Second,
			Heartbeat: time.Millisecond,
			Progress:  &progress,
		}.Run()
		if res.Err != nil {
			t.Fatalf("unexpected error: %v", res.Err)
		}
		// Reading the buffer here is only safe because Run waits for the
		// heartbeat to stop before returning.
		if !strings.Contains(progress.String(), "completed in") {
			t.Fatalf("missing terminal line; got:\n%s", progress.String())
		}
	}
}

// TestRunDetachesStdin proves a gate never waits on a terminal that a
// non-interactive host session does not have.
func TestRunDetachesStdin(t *testing.T) {
	t.Parallel()

	done := make(chan Result, 1)
	go func() {
		done <- Command{
			Label:    "stdin drain",
			Name:     "sh",
			Args:     []string{"-c", "cat; echo drained"},
			Timeout:  10 * time.Second,
			Progress: &bytes.Buffer{},
		}.Run()
	}()

	select {
	case res := <-done:
		if res.Err != nil {
			t.Fatalf("unexpected error: %v", res.Err)
		}
		if !strings.Contains(res.Output, "drained") {
			t.Errorf("expected stdin to be at EOF immediately; output = %q", res.Output)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("command blocked reading stdin")
	}
}

// TestRunReportsExitCode keeps failure reporting intact: a gate that bounds its
// command must still surface why the command failed.
func TestRunReportsExitCode(t *testing.T) {
	t.Parallel()

	res := Command{
		Label:    "failing gate",
		Name:     "sh",
		Args:     []string{"-c", "echo boom >&2; exit 3"},
		Timeout:  10 * time.Second,
		Progress: &bytes.Buffer{},
	}.Run()

	if res.TimedOut {
		t.Fatal("command failed on its own, it did not time out")
	}
	if got := res.ExitCode(); got != 3 {
		t.Errorf("ExitCode() = %d, want 3", got)
	}
	if !strings.Contains(res.Output, "boom") {
		t.Errorf("stderr not captured; output = %q", res.Output)
	}
}

// TestRunSucceedsWithoutTimeout covers the zero-Timeout path used by callers
// that only want the pipe and stdin guarantees.
func TestRunSucceedsWithoutTimeout(t *testing.T) {
	t.Parallel()

	res := Command{
		Label:    "unbounded",
		Name:     "sh",
		Args:     []string{"-c", "echo hi"},
		Progress: &bytes.Buffer{},
	}.Run()

	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.TimedOut {
		t.Error("TimedOut should be false with no deadline")
	}
	if !strings.Contains(res.Output, "hi") {
		t.Errorf("output = %q", res.Output)
	}
}

// TestCommandLine renders what an operator would retype.
func TestCommandLine(t *testing.T) {
	t.Parallel()

	got := Command{Name: "go", Args: []string{"test", "./..."}}.CommandLine()
	if got != "go test ./..." {
		t.Errorf("CommandLine() = %q", got)
	}
}

// TestCappedWriterTruncates bounds the memory a noisy gate command can consume.
func TestCappedWriterTruncates(t *testing.T) {
	t.Parallel()

	w := &cappedWriter{limit: 10}
	if _, err := w.Write([]byte("0123456789ABCDEF")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := w.String()
	if !strings.HasPrefix(got, "0123456789") {
		t.Errorf("kept prefix = %q", got)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("expected a truncation notice; got %q", got)
	}
}

// panicOnHeartbeat is a caller-supplied progress writer that fails only on the
// heartbeat line. Progress writers belong to callers, so a gate must survive a
// writer that panics on a background goroutine rather than take the whole
// `complete-phase` process down with it.
type panicOnHeartbeat struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *panicOnHeartbeat) Write(p []byte) (int, error) {
	if bytes.Contains(p, []byte("still running")) {
		panic("progress writer failed on the heartbeat line")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *panicOnHeartbeat) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestRunSurvivesPanickingProgressWriter proves the heartbeat goroutine is
// guarded. Without a recover inside the goroutine, this panic is unrecoverable
// and kills the test binary, which is exactly what it would do to a gate: the
// hang fix would have traded an indefinite wait for a crash.
func TestRunSurvivesPanickingProgressWriter(t *testing.T) {
	t.Parallel()

	progress := &panicOnHeartbeat{}
	res := Command{
		Label:     "hostile progress writer",
		Name:      "sh",
		Args:      []string{"-c", "exit 0"},
		Timeout:   10 * time.Second,
		Heartbeat: time.Millisecond,
		Progress:  progress,
	}.Run()

	if res.Err != nil {
		t.Fatalf("command outcome should be unaffected by the progress writer: %v", res.Err)
	}
	if !strings.Contains(progress.String(), "running `sh -c exit 0`") {
		t.Fatalf("missing start line; got:\n%s", progress.String())
	}
}
