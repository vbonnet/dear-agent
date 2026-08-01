//go:build darwin

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessCodeIdentityReadsRunningCDHash(t *testing.T) {
	identity, err := processCodeIdentity(os.Getpid())
	if err != nil {
		t.Fatalf("inspect current code identity: %v", err)
	}
	prefix := codeIdentityAlgorithm() + ":"
	if !strings.HasPrefix(identity, prefix) ||
		len(strings.TrimPrefix(identity, prefix)) != codeIdentityHexLength() {
		t.Fatalf("code identity = %q, want %s plus %d hex characters",
			identity, prefix, codeIdentityHexLength())
	}

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("/usr/bin/codesign", "-dvvv", executable).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect executable code signature: %v: %s", err, output)
	}
	var installedIdentity string
	for line := range strings.SplitSeq(string(output), "\n") {
		if digest, ok := strings.CutPrefix(line, "CDHash="); ok {
			installedIdentity = codeIdentityAlgorithm() + ":" + digest
			break
		}
	}
	if identity != installedIdentity {
		t.Fatalf("kernel identity = %q, codesign identity = %q", identity, installedIdentity)
	}
}

func TestValidateDarwinCodeStatusRequiresUnmodifiedHardenedRuntime(t *testing.T) {
	if err := validateDarwinCodeStatus(csValid | csRuntime); err != nil {
		t.Fatalf("valid hardened runtime status rejected: %v", err)
	}
	for name, status := range map[string]uint32{
		"not valid":           csRuntime,
		"not hardened":        csValid,
		"get task allow":      csValid | csRuntime | csGetTaskAllow,
		"invalid allowed":     csValid | csRuntime | csInvalidAllowed,
		"previously debugged": csValid | csRuntime | csDebugged,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDarwinCodeStatus(status); err == nil {
				t.Fatal("injectable process status was accepted")
			}
		})
	}
}

func TestValidateProcessImageAcceptsHardenedRuntimeChild(t *testing.T) {
	sourcePath, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read test executable: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "hardened-test-child")
	if err := os.WriteFile(executable, source, 0o600); err != nil {
		t.Fatalf("write test executable: %v", err)
	}
	if err := os.Chmod(executable, 0o700); err != nil {
		t.Fatalf("make test executable executable: %v", err)
	}
	output, err := exec.Command(
		"/usr/bin/codesign",
		"-f", "-s", "-", "--options", "runtime",
		executable,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("harden test executable: %v: %s", err, output)
	}
	child := exec.Command(executable, "-test.run=^TestHardenedRuntimeChildHelper$")
	child.Env = append(os.Environ(), "AGM_HARDENED_RUNTIME_TEST_CHILD=1")
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatalf("capture hardened child readiness: %v", err)
	}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatalf("start hardened test child: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = child.Wait()
		t.Fatalf("wait for hardened test child: %v: %s", err, stderr.String())
	}
	if strings.TrimSpace(ready) != "ready" {
		t.Fatalf("hardened test child readiness = %q", ready)
	}
	if err := validateProcessImage(child.Process.Pid); err != nil {
		t.Fatalf("hardened runtime child rejected: %v", err)
	}
}

func TestHardenedRuntimeChildHelper(t *testing.T) {
	if os.Getenv("AGM_HARDENED_RUNTIME_TEST_CHILD") != "1" {
		return
	}
	fmt.Println("ready")
	for {
		time.Sleep(time.Hour)
	}
}
