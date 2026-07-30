//go:build !windows

package main

import (
	"os"
	"strings"
	"testing"
)

func TestValidateLauncherChainAllowsOnlySudoBetweenHelperAndAGM(t *testing.T) {
	parents := map[int]int{40: 30, 30: 20}
	paths := map[int]string{40: installedSudoPath, 30: installedSudoPath}
	parent := func(pid int) (int, error) { return parents[pid], nil }
	path := func(pid int) (string, error) { return paths[pid], nil }

	if err := validateLauncherChain(40, 20, parent, path); err != nil {
		t.Fatalf("direct AGM caller behind sudo rejected: %v", err)
	}

	paths[30] = "/bin/zsh"
	if err := validateLauncherChain(40, 20, parent, path); err == nil ||
		!strings.Contains(err.Error(), "between the privileged helper") {
		t.Fatalf("same-user intermediary error = %v", err)
	}
}

func TestValidateLauncherChainRejectsUnrelatedApprovedProcess(t *testing.T) {
	parent := func(pid int) (int, error) {
		if pid == 40 {
			return 1, nil
		}
		return 0, nil
	}
	path := func(int) (string, error) { return installedSudoPath, nil }
	if err := validateLauncherChain(40, 99, parent, path); err == nil {
		t.Fatal("unrelated launcher PID was accepted")
	}
}

func TestValidateLauncherChainAllowsDirectParentWhenSudoExecsHelper(t *testing.T) {
	if err := validateLauncherChain(
		20,
		20,
		func(int) (int, error) { return 0, nil },
		func(int) (string, error) { return "", nil },
	); err != nil {
		t.Fatalf("direct authenticated AGM parent rejected: %v", err)
	}
}

func TestParseLauncherIdentityRequiresCurrentPlatformDigest(t *testing.T) {
	valid := codeIdentityAlgorithm() + ":" + strings.Repeat("a", codeIdentityHexLength())
	if got, err := parseLauncherIdentity([]byte(valid + "\n")); err != nil || got != valid {
		t.Fatalf("valid identity = %q, %v", got, err)
	}
	for _, invalid := range []string{
		"",
		"sha256:" + strings.Repeat("a", codeIdentityHexLength()),
		codeIdentityAlgorithm() + ":abcd",
		codeIdentityAlgorithm() + ":" + strings.Repeat("z", codeIdentityHexLength()),
		valid + " extra",
		" " + valid,
		valid + "\n\n",
	} {
		if _, err := parseLauncherIdentity([]byte(invalid)); err == nil {
			t.Fatalf("invalid identity %q was accepted", invalid)
		}
	}
}

func TestInstallerStagesApprovedAGMIdentity(t *testing.T) {
	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	text := string(makefile)
	for _, required := range []string{
		"install-override-ledger-helper: build-override-ledger-helper install-agm",
		"dear-agent-override-ledger-agm.identity",
		"darwin-cdhash:",
		"linux-sha256:",
		"staged_identity",
		"NOPASSWD: sha256:$$expected_hash $$helper",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("helper installer is missing %q", required)
		}
	}
}
