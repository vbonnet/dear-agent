package scratchpad

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These runtime capability tests run in the ordinary suite and skip explicitly
// when Docker is absent, stopped, or cannot expose the chosen host temp root.

func skipWithoutDocker(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping Docker integration test in short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Skipping: Docker not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput(); err != nil {
		t.Skipf("Skipping: Docker daemon unavailable: %v\n%s", err, out)
	}
}

func scratchpadTestConfig(maxExecutions int) *SandboxConfig {
	return &SandboxConfig{
		Image:         "python:3.11-slim",
		MaxExecutions: maxExecutions,
		MemoryLimit:   "512m",
		Timeout:       30 * time.Second,
	}
}

func requireDockerSandbox(t *testing.T, ctx context.Context, config *SandboxConfig) *Sandbox {
	t.Helper()
	skipWithoutDocker(t)

	sandbox, err := NewSandbox(ctx, config)
	if isSkippableBindCapabilityError(err) {
		t.Skipf("Skipping: Docker daemon cannot expose the scratchpad bind source: %v", err)
	}
	if err != nil {
		t.Fatalf("NewSandbox failed: %v", err)
	}
	return sandbox
}

func isSkippableBindCapabilityError(err error) bool {
	return errors.Is(err, errBindMountUnavailable) && !errors.Is(err, errSandboxCleanup)
}

func newDockerSandbox(t *testing.T, ctx context.Context, config *SandboxConfig) *Sandbox {
	t.Helper()
	sandbox := requireDockerSandbox(t, ctx, config)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := sandbox.Cleanup(cleanupCtx); err != nil {
			t.Errorf("Cleanup failed: %v", err)
		}
	})
	return sandbox
}

type fakeDockerOptions struct {
	mountVisible          bool
	probeCommandAvailable bool
	probeBlocks           bool
	runBlocks             bool
	wrongProbeBytes       bool
	removeBlocks          bool
	removeFails           bool
}

type fakeDockerEnvironment struct {
	tempRoot            string
	logPath             string
	containerNamePath   string
	containerExistsPath string
}

func installFakeDocker(t *testing.T, options fakeDockerOptions) fakeDockerEnvironment {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake Docker fixture requires a POSIX shell")
	}
	fixtureRoot := t.TempDir()
	binDir := filepath.Join(fixtureRoot, "bin")
	tempRoot := filepath.Join(fixtureRoot, "tmp")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("create fake Docker bin dir: %v", err)
	}
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		t.Fatalf("create fake Docker temp root: %v", err)
	}

	dockerPath := filepath.Join(binDir, "docker")
	const fakeDockerScript = `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
case "$1" in
  run)
	shift
	while [ "$#" -gt 0 ]; do
	  case "$1" in
		--name)
		  shift
		  printf '%s\n' "$1" > "$FAKE_DOCKER_CONTAINER_NAME"
		  ;;
		-v)
		  shift
		  printf '%s\n' "${1%:/workspace:ro}" > "$FAKE_DOCKER_SOURCE"
		  ;;
	  esac
	  shift
	done
	touch "$FAKE_DOCKER_CONTAINER_EXISTS"
	if [ "$FAKE_DOCKER_RUN_BLOCK" = "block" ]; then
	  while :; do :; done
	fi
	printf 'fake-container\n'
	;;
  exec)
    if [ "$FAKE_DOCKER_PROBE_COMMAND" != "available" ]; then
      printf 'exec: cat: executable file not found\n' >&2
      exit 127
    fi
    cat
    if [ "$FAKE_DOCKER_PROBE_BLOCK" = "block" ]; then
      while :; do :; done
    fi
    if [ "$FAKE_DOCKER_MOUNT_VISIBLE" != "visible" ]; then
      printf 'cat: /workspace/.engram-mount-probe: No such file or directory\n' >&2
      exit 1
    fi
    if [ "$FAKE_DOCKER_WRONG_BYTES" = "wrong" ]; then
      printf 'wrong-probe-bytes\n'
      exit 0
    fi
    source_path=$(cat "$FAKE_DOCKER_SOURCE")
    cat "$source_path/.engram-mount-probe"
    ;;
  rm)
	if [ "$FAKE_DOCKER_REMOVE_BLOCK" = "block" ]; then
	  while :; do :; done
	fi
	if [ "$FAKE_DOCKER_REMOVE" = "fail" ]; then
	  printf 'fake container removal failed\n' >&2
	  exit 1
	fi
	rm -f "$FAKE_DOCKER_CONTAINER_EXISTS"
	;;
  info)
    ;;
  *)
    printf 'unexpected fake docker command: %s\n' "$*" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(dockerPath, []byte(fakeDockerScript), 0o700); err != nil {
		t.Fatalf("write fake Docker executable: %v", err)
	}

	logPath := filepath.Join(fixtureRoot, "docker.log")
	containerNamePath := filepath.Join(fixtureRoot, "container-name")
	containerExistsPath := filepath.Join(fixtureRoot, "container-exists")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TMPDIR", tempRoot)
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	t.Setenv("FAKE_DOCKER_SOURCE", filepath.Join(fixtureRoot, "mount-source"))
	t.Setenv("FAKE_DOCKER_CONTAINER_NAME", containerNamePath)
	t.Setenv("FAKE_DOCKER_CONTAINER_EXISTS", containerExistsPath)
	t.Setenv("FAKE_DOCKER_MOUNT_VISIBLE", boolWord(options.mountVisible, "visible", "hidden"))
	t.Setenv("FAKE_DOCKER_PROBE_COMMAND", boolWord(options.probeCommandAvailable, "available", "missing"))
	t.Setenv("FAKE_DOCKER_PROBE_BLOCK", boolWord(options.probeBlocks, "block", "continue"))
	t.Setenv("FAKE_DOCKER_RUN_BLOCK", boolWord(options.runBlocks, "block", "continue"))
	t.Setenv("FAKE_DOCKER_WRONG_BYTES", boolWord(options.wrongProbeBytes, "wrong", "exact"))
	t.Setenv("FAKE_DOCKER_REMOVE_BLOCK", boolWord(options.removeBlocks, "block", "continue"))
	t.Setenv("FAKE_DOCKER_REMOVE", boolWord(options.removeFails, "fail", "succeed"))

	return fakeDockerEnvironment{
		tempRoot:            tempRoot,
		logPath:             logPath,
		containerNamePath:   containerNamePath,
		containerExistsPath: containerExistsPath,
	}
}

func boolWord(condition bool, trueWord, falseWord string) string {
	if condition {
		return trueWord
	}
	return falseWord
}

func readFakeDockerLog(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fake Docker log: %v", err)
	}
	return string(contents)
}

func TestNewSandboxAcceptsVisibleBindUnderNonDefaultTempRoot(t *testing.T) {
	fake := installFakeDocker(t, fakeDockerOptions{
		mountVisible:          true,
		probeCommandAvailable: true,
	})

	sandbox, err := NewSandbox(context.Background(), scratchpadTestConfig(3))
	if err != nil {
		t.Fatalf("NewSandbox failed with a visible bind: %v", err)
	}
	cleanupNeeded := true
	t.Cleanup(func() {
		if !cleanupNeeded {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := sandbox.Cleanup(cleanupCtx); err != nil {
			t.Errorf("emergency Cleanup failed: %v", err)
		}
	})

	tempPrefix := filepath.Clean(fake.tempRoot) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(sandbox.workdir)+string(os.PathSeparator), tempPrefix) {
		t.Fatalf("workdir %q is not beneath test temp root %q", sandbox.workdir, fake.tempRoot)
	}
	if _, err := os.Stat(filepath.Join(sandbox.workdir, mountProbeFilename)); !os.IsNotExist(err) {
		t.Fatalf("private mount probe still exists after verification: %v", err)
	}

	log := readFakeDockerLog(t, fake.logPath)
	if !strings.Contains(log, sandbox.workdir+":/workspace:ro") {
		t.Fatalf("docker run did not retain a read-only workspace bind:\n%s", log)
	}
	wantProbe := "exec -i -u 0 fake-container cat - " + mountProbeContainerPath
	if !strings.Contains(log, wantProbe) {
		t.Fatalf("docker log does not contain exact visibility probe %q:\n%s", wantProbe, log)
	}

	if err := sandbox.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	cleanupNeeded = false
	if _, err := os.Stat(sandbox.workdir); !os.IsNotExist(err) {
		t.Fatalf("workdir still exists after cleanup: %v", err)
	}
	log = readFakeDockerLog(t, fake.logPath)
	if strings.Count(log, "rm -f fake-container") != 1 {
		t.Fatalf("container cleanup count = %d, want 1:\n%s", strings.Count(log, "rm -f fake-container"), log)
	}
}

func TestNewMountProbePayloadIsUnique(t *testing.T) {
	first, err := newMountProbePayload()
	if err != nil {
		t.Fatalf("newMountProbePayload first call: %v", err)
	}
	second, err := newMountProbePayload()
	if err != nil {
		t.Fatalf("newMountProbePayload second call: %v", err)
	}
	if string(first) == string(second) {
		t.Fatal("two bind visibility probes used the same nonce")
	}
	for _, payload := range [][]byte{first, second} {
		if !strings.HasPrefix(string(payload), mountProbePayloadPrefix) || !strings.HasSuffix(string(payload), "\n") {
			t.Fatalf("probe payload does not have the expected framing: %q", payload)
		}
		wantLength := len(mountProbePayloadPrefix) + 2*mountProbeNonceBytes + 1
		if len(payload) != wantLength {
			t.Fatalf("probe payload length = %d, want %d", len(payload), wantLength)
		}
		nonce, err := hex.DecodeString(string(payload[len(mountProbePayloadPrefix) : len(payload)-1]))
		if err != nil {
			t.Fatalf("probe nonce is not hex: %v", err)
		}
		if len(nonce) != mountProbeNonceBytes {
			t.Fatalf("decoded probe nonce length = %d, want %d", len(nonce), mountProbeNonceBytes)
		}
	}
}

func TestNewSandboxLaunchCancellationRollsBackPreassignedContainer(t *testing.T) {
	tests := []struct {
		name             string
		removeFails      bool
		wantCleanupError bool
	}{
		{name: "cleanup succeeds"},
		{name: "cleanup failure is joined", removeFails: true, wantCleanupError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := installFakeDocker(t, fakeDockerOptions{
				runBlocks:   true,
				removeFails: test.removeFails,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			creationObserved := make(chan struct{})
			go func() {
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					if _, err := os.Stat(fake.containerExistsPath); err == nil {
						close(creationObserved)
						cancel()
						return
					}
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
			}()

			sandbox, err := NewSandbox(ctx, scratchpadTestConfig(3))
			if sandbox != nil {
				t.Fatalf("NewSandbox returned sandbox %#v after interrupted launch", sandbox)
			}
			if err == nil {
				t.Fatal("NewSandbox succeeded after interrupted launch")
			}
			select {
			case <-creationObserved:
			default:
				t.Fatal("fake container creation was not observed before launch returned")
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("launch error %v does not wrap context cancellation", err)
			}
			if got := errors.Is(err, errSandboxCleanup); got != test.wantCleanupError {
				t.Fatalf("errors.Is(err, errSandboxCleanup) = %v, want %v: %v", got, test.wantCleanupError, err)
			}

			entries, readErr := os.ReadDir(fake.tempRoot)
			if readErr != nil {
				t.Fatalf("read test temp root after rollback: %v", readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("rollback left %d workdir entries under %s", len(entries), fake.tempRoot)
			}

			containerNameBytes, readErr := os.ReadFile(fake.containerNamePath)
			if readErr != nil {
				t.Fatalf("read preassigned container name: %v", readErr)
			}
			containerName := strings.TrimSpace(string(containerNameBytes))
			if !strings.HasPrefix(containerName, sandboxContainerPrefix) {
				t.Fatalf("preassigned container name %q lacks prefix %q", containerName, sandboxContainerPrefix)
			}
			nonce, decodeErr := hex.DecodeString(strings.TrimPrefix(containerName, sandboxContainerPrefix))
			if decodeErr != nil || len(nonce) != sandboxContainerNonceBytes {
				t.Fatalf("preassigned container name %q has invalid nonce: bytes=%d err=%v", containerName, len(nonce), decodeErr)
			}

			log := readFakeDockerLog(t, fake.logPath)
			if !strings.Contains(log, "--name "+containerName) {
				t.Fatalf("docker run did not use preassigned container name %q:\n%s", containerName, log)
			}
			if strings.Count(log, "rm -f "+containerName) != 1 {
				t.Fatalf("ambiguous launch rollback count = %d, want 1:\n%s", strings.Count(log, "rm -f "+containerName), log)
			}
			if strings.Contains(log, "exec ") {
				t.Fatalf("docker exec ran after interrupted launch:\n%s", log)
			}
			_, statErr := os.Stat(fake.containerExistsPath)
			if test.removeFails {
				if statErr != nil {
					t.Fatalf("fake container marker unexpectedly absent after removal failure: %v", statErr)
				}
			} else if !os.IsNotExist(statErr) {
				t.Fatalf("fake container marker remains after rollback: %v", statErr)
			}
		})
	}
}

func TestNewSandboxRejectsUnverifiedBindAndRollsBack(t *testing.T) {
	tests := []struct {
		name             string
		options          fakeDockerOptions
		wantUnavailable  bool
		wantCleanupError bool
		wantSkippable    bool
		wantErrorDetails string
		contextTimeout   time.Duration
	}{
		{
			name: "invisible bind source",
			options: fakeDockerOptions{
				probeCommandAvailable: true,
			},
			wantUnavailable:  true,
			wantSkippable:    true,
			wantErrorDetails: "No such file or directory",
		},
		{
			name: "different probe bytes",
			options: fakeDockerOptions{
				mountVisible:          true,
				probeCommandAvailable: true,
				wrongProbeBytes:       true,
			},
			wantUnavailable:  true,
			wantSkippable:    true,
			wantErrorDetails: "mount probe bytes differed",
		},
		{
			name: "probe command unavailable",
			options: fakeDockerOptions{
				mountVisible: true,
			},
			wantUnavailable:  false,
			wantSkippable:    false,
			wantErrorDetails: "probe command failed before reading its input",
		},
		{
			name: "probe context expires after command starts",
			options: fakeDockerOptions{
				mountVisible:          true,
				probeCommandAvailable: true,
				probeBlocks:           true,
			},
			wantUnavailable:  false,
			wantSkippable:    false,
			wantErrorDetails: "bind visibility probe interrupted: context deadline exceeded",
			contextTimeout:   750 * time.Millisecond,
		},
		{
			name: "container rollback failure is joined",
			options: fakeDockerOptions{
				probeCommandAvailable: true,
				removeFails:           true,
			},
			wantUnavailable:  true,
			wantCleanupError: true,
			wantSkippable:    false,
			wantErrorDetails: "failed to remove container",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := installFakeDocker(t, test.options)
			ctx := context.Background()
			var sandbox *Sandbox
			var err error
			if test.contextTimeout > 0 {
				timedCtx, cancel := context.WithTimeout(ctx, test.contextTimeout)
				defer cancel()
				sandbox, err = NewSandbox(timedCtx, scratchpadTestConfig(3))
			} else {
				sandbox, err = NewSandbox(ctx, scratchpadTestConfig(3))
			}
			if sandbox != nil {
				t.Fatalf("NewSandbox returned sandbox %#v after failed visibility proof", sandbox)
			}
			if err == nil {
				t.Fatal("NewSandbox succeeded without a valid visibility proof")
			}
			if got := errors.Is(err, errBindMountUnavailable); got != test.wantUnavailable {
				t.Fatalf("errors.Is(err, errBindMountUnavailable) = %v, want %v: %v", got, test.wantUnavailable, err)
			}
			if got := errors.Is(err, errSandboxCleanup); got != test.wantCleanupError {
				t.Fatalf("errors.Is(err, errSandboxCleanup) = %v, want %v: %v", got, test.wantCleanupError, err)
			}
			if got := isSkippableBindCapabilityError(err); got != test.wantSkippable {
				t.Fatalf("isSkippableBindCapabilityError(err) = %v, want %v: %v", got, test.wantSkippable, err)
			}
			if !strings.Contains(err.Error(), test.wantErrorDetails) {
				t.Fatalf("error %q does not contain %q", err, test.wantErrorDetails)
			}

			entries, readErr := os.ReadDir(fake.tempRoot)
			if readErr != nil {
				t.Fatalf("read test temp root after rollback: %v", readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("rollback left %d workdir entries under %s", len(entries), fake.tempRoot)
			}

			log := readFakeDockerLog(t, fake.logPath)
			if strings.Count(log, "rm -f fake-container") != 1 {
				t.Fatalf("container rollback count = %d, want 1:\n%s", strings.Count(log, "rm -f fake-container"), log)
			}
			for _, interpreter := range []string{" python3 ", " bash ", " node "} {
				if strings.Contains(log, interpreter) {
					t.Fatalf("interpreter %q ran before bind visibility was proven:\n%s", strings.TrimSpace(interpreter), log)
				}
			}
		})
	}
}

func TestRollbackSandboxCreationIgnoresCanceledCreationContext(t *testing.T) {
	fake := installFakeDocker(t, fakeDockerOptions{})
	workdir := filepath.Join(fake.tempRoot, "created-workdir")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatalf("create rollback workdir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rollbackSandboxCreation(ctx, "fake-container", workdir); err != nil {
		t.Fatalf("rollbackSandboxCreation with canceled creation context: %v", err)
	}

	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Fatalf("rollback left workdir behind: %v", err)
	}
	log := readFakeDockerLog(t, fake.logPath)
	if strings.Count(log, "rm -f fake-container") != 1 {
		t.Fatalf("rollback container cleanup count = %d, want 1:\n%s", strings.Count(log, "rm -f fake-container"), log)
	}
}

func TestRollbackSandboxCreationStopsAtCleanupLimit(t *testing.T) {
	fake := installFakeDocker(t, fakeDockerOptions{removeBlocks: true})
	workdir := filepath.Join(fake.tempRoot, "created-workdir")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatalf("create rollback workdir: %v", err)
	}

	const cleanupLimit = time.Second
	started := time.Now()
	err := rollbackSandboxCreationWithin(context.Background(), "fake-container", workdir, cleanupLimit)
	elapsed := time.Since(started)

	if !errors.Is(err, errSandboxCleanup) {
		t.Fatalf("rollback error %v does not wrap errSandboxCleanup", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("rollback error %v does not wrap context deadline", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("cleanup took %s after %s limit", elapsed, cleanupLimit)
	}
	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Fatalf("rollback left workdir behind: %v", err)
	}
	log := readFakeDockerLog(t, fake.logPath)
	if strings.Count(log, "rm -f fake-container") != 1 {
		t.Fatalf("rollback container cleanup count = %d, want 1:\n%s", strings.Count(log, "rm -f fake-container"), log)
	}
}

func TestNewSandbox(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(10))

	if sandbox.containerID == "" {
		t.Error("Container ID is empty")
	}

	if sandbox.workdir == "" {
		t.Error("Workdir is empty")
	}

	if sandbox.maxExecutions != 10 {
		t.Errorf("MaxExecutions = %d, want 10", sandbox.maxExecutions)
	}
}

func TestExecute_Python(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(10))

	// Test: Execute simple Python script
	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     "print('Hello from sandbox')",
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", response.ExitCode)
	}

	if !strings.Contains(response.Stdout, "Hello from sandbox") {
		t.Errorf("Stdout = %q, want to contain 'Hello from sandbox'", response.Stdout)
	}

	if response.Error != "" {
		t.Errorf("Unexpected error: %s", response.Error)
	}
}

func TestExecute_Python_JSON(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(10))

	code := `
import json
data = {"status": "working", "version": "1.0"}
print(json.dumps(data))
`

	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     code,
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", response.ExitCode)
	}

	if !strings.Contains(response.Stdout, `"status": "working"`) {
		t.Errorf("Stdout = %q, want JSON output", response.Stdout)
	}
}

func TestExecute_Bash(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(10))

	code := `#!/bin/bash
echo "Testing bash"
echo "Exit code test"
exit 0
`

	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "bash",
		Code:     code,
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", response.ExitCode)
	}

	if !strings.Contains(response.Stdout, "Testing bash") {
		t.Errorf("Stdout = %q, want to contain 'Testing bash'", response.Stdout)
	}
}

func TestExecute_Error(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(10))

	// Test: Python script with error
	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     "import nonexistent_module",
		Timeout:  5 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if response.ExitCode == 0 {
		t.Error("Expected non-zero exit code for error")
	}

	if !strings.Contains(response.Stderr, "ModuleNotFoundError") && !strings.Contains(response.Stderr, "No module named") {
		t.Errorf("Stderr = %q, want to contain module error", response.Stderr)
	}
}

func TestExecute_Timeout(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(10))

	// Test: Script that sleeps longer than timeout
	code := `
import time
time.sleep(10)
print("Should not reach here")
`

	response, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     code,
		Timeout:  1 * time.Second,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should timeout
	if response.ExitCode == 0 {
		t.Error("Expected non-zero exit code for timeout")
	}

	if response.Duration < 1*time.Second {
		t.Errorf("Duration = %v, expected at least 1 second", response.Duration)
	}
}

func TestExecute_ExecutionLimit(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(3))

	// Execute 3 times successfully
	for i := 0; i < 3; i++ {
		response, err := sandbox.Execute(ctx, &ExecuteRequest{
			Language: "python",
			Code:     "print('test')",
			Timeout:  5 * time.Second,
		})
		if err != nil {
			t.Fatalf("Execute %d failed: %v", i+1, err)
		}
		if response.ExitCode != 0 || !strings.Contains(response.Stdout, "test") {
			t.Fatalf("Execute %d did not run the probe successfully: exit=%d stdout=%q stderr=%q", i+1, response.ExitCode, response.Stdout, response.Stderr)
		}
	}

	// 4th execution should fail
	_, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "python",
		Code:     "print('should fail')",
		Timeout:  5 * time.Second,
	})

	if err == nil {
		t.Error("Expected error for execution limit exceeded")
	}

	if !strings.Contains(err.Error(), "execution limit reached") {
		t.Errorf("Error = %v, want execution limit error", err)
	}
}

func TestExecute_UnsupportedLanguage(t *testing.T) {
	ctx := context.Background()
	sandbox := newDockerSandbox(t, ctx, scratchpadTestConfig(10))

	// Test: Unsupported language
	_, err := sandbox.Execute(ctx, &ExecuteRequest{
		Language: "ruby",
		Code:     "puts 'hello'",
		Timeout:  5 * time.Second,
	})

	if err == nil {
		t.Fatal("Expected error for unsupported language, got nil")
		return
	}

	if !strings.Contains(err.Error(), "unsupported language") {
		t.Errorf("Error = %v, want unsupported language error", err)
	}
}

func TestCleanup(t *testing.T) {
	ctx := context.Background()
	sandbox := requireDockerSandbox(t, ctx, scratchpadTestConfig(10))

	containerID := sandbox.containerID
	workdir := sandbox.workdir

	// Cleanup
	if err := sandbox.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify container removed (docker ps -a should not list it)
	// This is hard to verify without docker CLI, so we just check no error

	// Verify workdir removed
	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Errorf("Workdir still exists after cleanup: %s", workdir)
	}

	// Verify containerID was set
	if containerID == "" {
		t.Error("ContainerID was empty before cleanup")
	}
}

func TestGetFileExtension(t *testing.T) {
	tests := []struct {
		language string
		want     string
	}{
		{"python", ".py"},
		{"bash", ".sh"},
		{"node", ".js"},
		{"unknown", ".txt"},
	}

	for _, tt := range tests {
		got := getFileExtension(tt.language)
		if got != tt.want {
			t.Errorf("getFileExtension(%q) = %q, want %q", tt.language, got, tt.want)
		}
	}
}
