//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
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
	source, err := os.ReadFile("/bin/sleep")
	if err != nil {
		t.Fatalf("read sleep fixture: %v", err)
	}
	executable := filepath.Join(t.TempDir(), "hardened-sleep")
	if err := os.WriteFile(executable, source, 0o600); err != nil {
		t.Fatalf("write sleep fixture: %v", err)
	}
	if err := syscall.Chmod(executable, 0o700); err != nil {
		t.Fatalf("make sleep fixture executable: %v", err)
	}
	output, err := exec.Command(
		"/usr/bin/codesign",
		"-f", "-s", "-", "--options", "runtime",
		executable,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("harden sleep fixture: %v: %s", err, output)
	}
	child := exec.Command(executable, "30")
	if err := child.Start(); err != nil {
		t.Fatalf("start hardened sleep fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	if err := validateProcessImage(child.Process.Pid); err != nil {
		t.Fatalf("hardened runtime child rejected: %v", err)
	}
}
