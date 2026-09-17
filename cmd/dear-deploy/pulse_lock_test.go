package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Conformance: internal/deploy/SPEC.md DEP-30.
func TestNormalRegistryPairHoldsPulseLockThroughJobsPublication(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	transitionPulses := `{"pulses":[
  {"name":"live-tick","type":"file_mtime","path":"~/live","window":"1h"},
  {"name":"next-tick","type":"file_mtime","path":"~/next","window":"1h"}
]}`
	liveJobs := `{"jobs":[{"name":"live-job","pulse":"live-tick"}]}`
	nextJobs := `{"jobs":[{"name":"next-job","pulse":"next-tick"}]}`
	mustWrite(t, filepath.Join(repo, "custom/jobs.json"), nextJobs)
	mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
`)

	pulseSource := filepath.Join(repo, "deploy/absence-alarm/pulses.json")
	if err := os.MkdirAll(filepath.Dir(pulseSource), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(pulseSource, 0o600); err != nil {
		t.Fatalf("mkfifo pulse source: %v", err)
	}
	pulsePath := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
	jobsPath := filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
	mustWrite(t, pulsePath,
		`{"pulses":[{"name":"live-tick","type":"file_mtime","path":"~/live","window":"1h"}]}`)
	mustWrite(t, jobsPath, liveJobs)

	type runResult struct {
		code      int
		out, errs string
	}
	done := make(chan runResult, 1)
	go func() {
		code, out, errs := invoke(t, repo, home, "sync")
		done <- runResult{code: code, out: out, errs: errs}
	}()

	// Opening the source writer succeeds only after the deploy has rendered the
	// prospective jobs, observed the live jobs, and reached pulse rendering.
	pulseWriter := openFIFOWriterForReader(t, pulseSource)
	if err := os.Remove(jobsPath); err != nil {
		t.Fatalf("replace live jobs with publication barrier: %v", err)
	}
	if err := unix.Mkfifo(jobsPath, 0o600); err != nil {
		t.Fatalf("mkfifo jobs target: %v", err)
	}
	if _, err := io.WriteString(pulseWriter, transitionPulses); err != nil {
		t.Fatalf("feed pulse source: %v", err)
	}
	if err := pulseWriter.Close(); err != nil {
		t.Fatalf("close pulse source: %v", err)
	}

	// DeployRendered reads the target before replacing it. Keep that read open
	// so the test can probe the publication lock after pulse activation but
	// before jobs activation.
	jobsWriter := openFIFOWriterForReader(t, jobsPath)
	deployedPulse, pulseReadErr := os.ReadFile(pulsePath)
	lockFile, lockOpenErr := os.OpenFile(pulsePath+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	var lockErr error
	if lockOpenErr == nil {
		lockErr = unix.Flock(int(lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if lockErr == nil {
			_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
		}
		_ = lockFile.Close()
	}

	var writeErr error
	if _, err := io.WriteString(jobsWriter, liveJobs); err != nil {
		writeErr = err
	}
	closeErr := jobsWriter.Close()
	var result runResult
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("normal registry pair did not finish after releasing jobs barrier")
	}

	if pulseReadErr != nil || string(deployedPulse) != transitionPulses {
		t.Fatalf("pulse at jobs barrier: got=%s err=%v", deployedPulse, pulseReadErr)
	}
	if lockOpenErr != nil {
		t.Fatalf("open publication lock: %v", lockOpenErr)
	}
	if !errors.Is(lockErr, unix.EWOULDBLOCK) && !errors.Is(lockErr, unix.EAGAIN) {
		t.Fatalf("publication lock probe = %v, want held through jobs activation", lockErr)
	}
	if writeErr != nil || closeErr != nil {
		t.Fatalf("release jobs barrier: write=%v close=%v", writeErr, closeErr)
	}
	if result.code != 0 {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", result.code, result.out, result.errs)
	}
	if got, err := os.ReadFile(jobsPath); err != nil || string(got) != nextJobs {
		t.Fatalf("deployed jobs after barrier: got=%s err=%v want=%s", got, err, nextJobs)
	}
}

func openFIFOWriterForReader(t *testing.T, path string) *os.File {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK, 0)
		if err == nil {
			writer := os.NewFile(uintptr(fd), path)
			if writer == nil {
				_ = unix.Close(fd)
				t.Fatalf("wrap FIFO writer %s", path)
			}
			return writer
		}
		if !errors.Is(err, unix.ENXIO) && !errors.Is(err, unix.ENOENT) {
			t.Fatalf("open FIFO writer %s: %v", path, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for FIFO reader %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
