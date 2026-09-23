package boundedexec

import (
	"strings"
	"testing"
	"time"
)

// TestRunInterleavedStdoutStderrHasNoRace probes the reviewer claim that using
// one cappedWriter for both streams races. os/exec returns the same pipe for
// Stderr when it is the same value as Stdout, so there is one copy goroutine.
func TestRunInterleavedStdoutStderrHasNoRace(t *testing.T) {
	t.Parallel()
	res := Command{
		Label:   "interleaved streams",
		Name:    "sh",
		Args:    []string{"-c", "i=0; while [ $i -lt 400 ]; do echo out-$i; echo err-$i 1>&2; i=$((i+1)); done"},
		Timeout: 30 * time.Second,
	}.Run()
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if !strings.Contains(res.Output, "out-399") || !strings.Contains(res.Output, "err-399") {
		t.Fatal("lost output from one of the two streams")
	}
}
