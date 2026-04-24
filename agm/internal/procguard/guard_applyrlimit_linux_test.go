//go:build linux

package procguard

import (
	"os"
	"os/exec"
	"testing"
)

func TestApplyNprocLimit(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep process: %v", err)
	}
	defer cmd.Process.Kill()

	pid := cmd.Process.Pid

	err := ApplyNprocLimit(pid, 128)
	if err != nil {
		t.Fatalf("ApplyNprocLimit failed: %v", err)
	}
}

func TestApplyNprocLimit_InvalidPID(t *testing.T) {
	err := ApplyNprocLimit(999999, 128)
	if err == nil {
		t.Fatal("expected error for invalid PID")
	}
}

func TestApplyNprocLimit_CurrentProcess(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("skipping: requires root/CAP_SYS_RESOURCE to set RLIMIT_NPROC on self")
	}
	pid := os.Getpid()
	err := ApplyNprocLimit(pid, 65535)
	if err != nil {
		t.Fatalf("ApplyNprocLimit on self failed: %v", err)
	}
}
