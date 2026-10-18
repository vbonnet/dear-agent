package deploy

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Conformance: internal/deploy/SPEC.md DEP-30.
func TestLockPulseRegistrySerializesAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "pulses.json")
	ready := filepath.Join(dir, "holder.ready")
	release := filepath.Join(dir, "holder.release")

	var childOutput bytes.Buffer
	child := exec.Command(os.Args[0], "-test.run=^TestLockPulseRegistryHelperProcess$")
	child.Env = append(os.Environ(),
		"DEAR_AGENT_TEST_PULSE_LOCK_HELPER=1",
		"DEAR_AGENT_TEST_PULSE_LOCK_HOST="+host,
		"DEAR_AGENT_TEST_PULSE_LOCK_READY="+ready,
		"DEAR_AGENT_TEST_PULSE_LOCK_RELEASE="+release,
	)
	child.Stdout = &childOutput
	child.Stderr = &childOutput
	if err := child.Start(); err != nil {
		t.Fatalf("start pulse lock holder: %v", err)
	}
	childDone := make(chan error, 1)
	go func() { childDone <- child.Wait() }()
	t.Cleanup(func() {
		_ = child.Process.Kill()
		select {
		case <-childDone:
		default:
		}
	})

	waitForPulseLockTestPath(t, ready)
	lockFile, err := os.OpenFile(host+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open contender lock: %v", err)
	}
	lockErr := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if lockErr == nil {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	}
	if closeErr := lockFile.Close(); closeErr != nil {
		t.Fatalf("close contender lock: %v", closeErr)
	}
	if !errors.Is(lockErr, syscall.EWOULDBLOCK) && !errors.Is(lockErr, syscall.EAGAIN) {
		t.Fatalf("contender lock error = %v, want held by helper process", lockErr)
	}

	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release pulse lock holder: %v", err)
	}
	select {
	case err := <-childDone:
		if err != nil {
			t.Fatalf("pulse lock holder failed: %v\n%s", err, childOutput.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("pulse lock holder did not exit\n%s", childOutput.String())
	}

	lockFile, err = os.OpenFile(host+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("reopen contender lock: %v", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		t.Fatalf("lock remained held after helper exit: %v", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN); err != nil {
		_ = lockFile.Close()
		t.Fatalf("unlock contender: %v", err)
	}
	if err := lockFile.Close(); err != nil {
		t.Fatalf("close reacquired lock: %v", err)
	}
}

func TestLockPulseRegistryHelperProcess(t *testing.T) {
	if os.Getenv("DEAR_AGENT_TEST_PULSE_LOCK_HELPER") != "1" {
		return
	}
	host := os.Getenv("DEAR_AGENT_TEST_PULSE_LOCK_HOST")
	ready := os.Getenv("DEAR_AGENT_TEST_PULSE_LOCK_READY")
	release := os.Getenv("DEAR_AGENT_TEST_PULSE_LOCK_RELEASE")
	if host == "" || ready == "" || release == "" {
		t.Fatal("pulse lock helper environment is incomplete")
	}
	unlock, err := LockPulseRegistry(host)
	if err != nil {
		t.Fatalf("acquire pulse registry lock: %v", err)
	}
	defer unlock()
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatalf("write pulse lock ready marker: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(release)
		switch {
		case err == nil:
			return
		case !os.IsNotExist(err):
			t.Fatalf("inspect pulse lock release marker: %v", err)
		case time.Now().After(deadline):
			t.Fatal("timed out waiting for pulse lock release marker")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func waitForPulseLockTestPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(path)
		switch {
		case err == nil:
			return
		case !os.IsNotExist(err):
			t.Fatalf("inspect %s: %v", path, err)
		case time.Now().After(deadline):
			t.Fatalf("timed out waiting for %s", path)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}
