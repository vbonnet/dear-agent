//go:build darwin || linux

package boundedexec

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const helperEnv = "BOUNDEDEXEC_SIGNAL_HELPER_PIDFILE"

// TestHelperProcessRunsBoundedCommand is not a test. It is the child half of
// TestInterruptEndsWayfinderAndTheProcessGroup, re-executed through the test
// binary so the signal path runs in a real process rather than in-process.
func TestHelperProcessRunsBoundedCommand(t *testing.T) {
	pidFile := os.Getenv(helperEnv)
	if pidFile == "" {
		t.Skip("not the helper process")
	}
	Command{
		Label:     "helper",
		Name:      "sh",
		Args:      []string{"-c", "sleep 60 & echo $! > " + pidFile + "; sleep 60"},
		Timeout:   60 * time.Second,
		Heartbeat: time.Hour,
		Progress:  os.Stderr,
	}.Run()
	// Stand in for the rest of complete-phase: more work after the interrupted
	// command. If Run swallowed the signal, the helper is still alive here and
	// the parent's wait times out, which is exactly the defect under test.
	time.Sleep(60 * time.Second)
	os.Exit(0)
}

// TestInterruptEndsWayfinderAndTheProcessGroup proves both halves of the
// interrupt contract that this package has to honour. Setpgid takes the command
// out of the caller's foreground process group, so the package owns what Ctrl-C
// now means: the toolchain must die with its group, and the interrupt must
// still end the caller instead of being consumed so that complete-phase carries
// on to complete the phase.
func TestInterruptEndsWayfinderAndTheProcessGroup(t *testing.T) {
	pidFile := t.TempDir() + "/descendant.pid"

	helper := exec.Command(os.Args[0], "-test.run=TestHelperProcessRunsBoundedCommand", "-test.timeout=120s")
	helper.Env = append(os.Environ(), helperEnv+"="+pidFile)
	helper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // isolate from our own group
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL) })

	descendant := waitForPid(t, pidFile)

	if err := helper.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt helper: %v", err)
	}

	waited := make(chan error, 1)
	go func() { waited <- helper.Wait() }()
	select {
	case <-waited:
	case <-time.After(20 * time.Second):
		t.Fatal("the interrupt was swallowed: the caller kept running after Ctrl-C")
	}

	deadline := time.Now().Add(10 * time.Second)
	for descendantIsRunning(t, descendant) {
		if time.Now().After(deadline) {
			t.Fatalf("descendant pid %d survived the interrupt: the group was not killed", descendant)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForPid(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("helper never recorded a descendant pid in %s", pidFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestInterruptPreservesTheSignal checks that a supervisor's SIGTERM comes back
// as SIGTERM. Wayfinder runs under supervisors that read the exit status, so
// collapsing every signal into SIGINT/130 misreports why it stopped.
func TestInterruptPreservesTheSignal(t *testing.T) {
	pidFile := t.TempDir() + "/descendant.pid"

	helper := exec.Command(os.Args[0], "-test.run=TestHelperProcessRunsBoundedCommand", "-test.timeout=120s")
	helper.Env = append(os.Environ(), helperEnv+"="+pidFile)
	helper.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-helper.Process.Pid, syscall.SIGKILL) })

	waitForPid(t, pidFile)
	if err := helper.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("terminate helper: %v", err)
	}

	waited := make(chan error, 1)
	go func() { waited <- helper.Wait() }()
	select {
	case <-waited:
	case <-time.After(20 * time.Second):
		t.Fatal("SIGTERM was swallowed: the caller kept running")
	}

	status, ok := helper.ProcessState.Sys().(syscall.WaitStatus)
	if !ok {
		t.Fatal("no wait status recorded")
	}
	if !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("SIGTERM was not preserved: signaled=%v signal=%v exit=%d",
			status.Signaled(), status.Signal(), status.ExitStatus())
	}
}

// TestRunSurvivesAnIgnoredInterrupt covers the fall-through that re-raising
// cannot rule out: a signal inherited as ignored is delivered and discarded, so
// Run keeps executing past the re-raise. It must still return a well-formed
// result rather than writing to the sink it already closed.
func TestRunSurvivesAnIgnoredInterrupt(t *testing.T) {
	signal.Ignore(syscall.SIGUSR1)
	t.Cleanup(func() { signal.Reset(syscall.SIGUSR1) })

	sink := newProgressSink(&bytes.Buffer{})
	sink.close()
	sink.close() // idempotent: the interrupt path closes early, the defer closes again

	reraise(syscall.SIGUSR1) // ignored, so this returns instead of terminating
}
