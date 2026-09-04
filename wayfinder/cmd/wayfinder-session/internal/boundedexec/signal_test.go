//go:build darwin || linux

package boundedexec

import (
	"os"
	"os/exec"
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
