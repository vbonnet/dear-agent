//go:build darwin

package main

import (
	"os"
	"os/exec"
	"strings"
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
