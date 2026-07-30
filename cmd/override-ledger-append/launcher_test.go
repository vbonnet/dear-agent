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

func TestLauncherIdentityMatchesOnlyApprovedAGMOrCompanion(t *testing.T) {
	agm := codeIdentityAlgorithm() + ":" + strings.Repeat("a", codeIdentityHexLength())
	companion := codeIdentityAlgorithm() + ":" + strings.Repeat("b", codeIdentityHexLength())
	unapproved := codeIdentityAlgorithm() + ":" + strings.Repeat("c", codeIdentityHexLength())
	if !launcherIdentityMatches(agm, []string{agm}) {
		t.Fatal("approved AGM identity did not match")
	}
	if !launcherIdentityMatches(companion, []string{agm, companion}) {
		t.Fatal("approved companion identity did not match issuance policy")
	}
	if launcherIdentityMatches(companion, []string{agm}) {
		t.Fatal("companion identity matched AGM-only policy")
	}
	if launcherIdentityMatches(unapproved, []string{agm, companion}) {
		t.Fatal("unapproved identity matched launcher policy")
	}
}

func TestInstallerStagesApprovedAGMIdentity(t *testing.T) {
	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	text := string(makefile)
	for _, required := range []string{
		"install-override-ledger-helper: build-override-ledger-helper build-agm build-agm-mcp-server",
		"dear-agent-override-ledger-agm.identity",
		"dear-agent-override-ledger-agm-mcp-server.identity",
		"companion_caller_identity",
		"staged_companion_identity",
		`agm_staging="$$(/usr/bin/mktemp "$$agm_executable.XXXXXX")"`,
		`companion_staging="$$(/usr/bin/mktemp "$$companion_executable.XXXXXX")"`,
		`/usr/bin/codesign -f -s - --options runtime "$$agm_staging"`,
		`activation_started=1`,
		`activation_complete=1`,
		"CGO_ENABLED=0 go build $(GOFLAGS) -o bin/agm ",
		"CGO_ENABLED=0 go build $(GOFLAGS) -o bin/agm-mcp-server ",
		"darwin-cdhash:",
		"linux-sha256:",
		"staged_identity",
		"NOPASSWD: sha256:$$expected_hash $$helper",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("helper installer is missing %q", required)
		}
	}
	start := strings.Index(text, "install-override-ledger-helper:")
	if start < 0 {
		t.Fatal("could not find helper installer target")
	}
	end := strings.Index(text[start:], "\ninstall-override-audit-launchdaemon:")
	if end < 0 {
		t.Fatal("could not find the target after helper installation")
	}
	installer := text[start : start+end]
	for _, required := range []string{
		"/usr/bin/sudo -n /usr/bin/true",
		"/usr/bin/sudo /usr/bin/true",
	} {
		if !strings.Contains(installer, required) {
			t.Errorf("helper installer is missing command-specific authentication %q", required)
		}
	}
	if strings.Contains(installer, "/usr/bin/sudo -n -v") ||
		strings.Contains(installer, "/usr/bin/sudo -v") {
		t.Fatal("helper installer still uses sudo's generic validation pseudocommand")
	}
}

func TestLaunchDaemonInstallerRollsBackPartialActivation(t *testing.T) {
	makefile, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	text := string(makefile)
	start := strings.Index(text, "install-override-audit-launchdaemon:")
	if start < 0 {
		t.Fatal("could not find LaunchDaemon installer target")
	}
	end := strings.Index(text[start:], "\nuninstall-override-audit-launchdaemon:")
	if end < 0 {
		t.Fatal("could not find target after LaunchDaemon installation")
	}
	installer := text[start : start+end]
	for _, required := range []string{
		`audit_live="/usr/local/libexec/dear-agent-override-audit"`,
		`plist_live="/Library/LaunchDaemons/com.dear-agent.override-audit.plist"`,
		`audit_backup=`,
		`plist_backup=`,
		`status=$$1`,
		`trap - EXIT HUP INT TERM`,
		`activation_started=1`,
		`activation_complete=1`,
		`/bin/mv -f "$$audit_backup" "$$audit_live"`,
		`/bin/mv -f "$$plist_backup" "$$plist_live"`,
		`/bin/rm -f "$$audit_live"`,
		`/bin/rm -f "$$plist_live"`,
		`exit "$$status"`,
	} {
		if !strings.Contains(installer, required) {
			t.Errorf("LaunchDaemon installer is missing transactional activation fragment %q", required)
		}
	}
	backup := strings.Index(installer, `audit_backup="$$(/usr/bin/sudo /usr/bin/mktemp`)
	activate := strings.Index(installer, "activation_started=1")
	if backup < 0 || activate < 0 || backup > activate {
		t.Fatal("LaunchDaemon installer does not back up live artifacts before activation")
	}
}
