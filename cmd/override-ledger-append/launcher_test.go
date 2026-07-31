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
		"companion_caller_identity",
		`agm_staging="$$(/usr/bin/mktemp "$$agm_executable.XXXXXX")"`,
		`companion_staging="$$(/usr/bin/mktemp "$$companion_executable.XXXXXX")"`,
		`/usr/bin/codesign -f -s - --options runtime "$$agm_staging"`,
		`root_installer_path="$$repo_root/scripts/install-override-ledger-root.sh"`,
		`test "$$confirmed_installer_hash" = "$$expected_installer_hash"`,
		"CGO_ENABLED=0 go build $(GOFLAGS) -o bin/agm ",
		"CGO_ENABLED=0 go build $(GOFLAGS) -o bin/agm-mcp-server ",
		"darwin-cdhash:",
		"linux-sha256:",
		"confirmed_installer_hash",
		"dear-agent-override-ledger-installer",
		`agm_policy_artifact="$$(/usr/bin/mktemp "$$agm_executable.policy.XXXXXX")"`,
		`transaction_nonce="$$(/usr/bin/openssl rand -hex 32)"`,
		`install_lock_file="/tmp/dear-agent-override-ledger-install.lock"`,
		`umask 000; : >>"$$install_lock_file"`,
		`DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED=1 "$$install_lock_tool" -t 0`,
		`DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED=1 "$$install_lock_tool" -n`,
		`install-override-ledger-helper-locked`,
		`test "$${DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED:-}" = 1`,
		`launcher_txdir="$$(/usr/bin/mktemp -d "$(HOME)/go/bin/.dear-agent-launchers.XXXXXX")"`,
		`restore_launcher()`,
		`cleanup_launchers()`,
		`root_transaction_committed()`,
		`dear-agent-override-ledger-install.receipt`,
		`launcher_set_active=1`,
		`launcher_activation_complete=1`,
		`/bin/mv -f "$$agm_staging" "$$agm_executable"`,
		`/bin/mv -f "$$companion_staging" "$$companion_executable"`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("helper installer is missing %q", required)
		}
	}
	rootInstaller, err := os.ReadFile("../../scripts/install-override-ledger-root.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"dear-agent-override-ledger-agm.identity",
		"dear-agent-override-ledger-agm-mcp-server.identity",
		`"$txdir/agm-mcp-server"`,
		"dear-agent-override-ledger-install.receipt",
		`test "darwin-cdhash:$companion_digest" = "$companion_identity"`,
	} {
		if !strings.Contains(string(rootInstaller), required) {
			t.Errorf("fixed root installer is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"agm_executable",
		"companion_executable",
		"agm_stage=",
		"agm_mcp_stage=",
		"operator_uid",
		"operator_gid",
		`mktemp "$agm`,
	} {
		if strings.Contains(string(rootInstaller), forbidden) {
			t.Errorf("fixed root installer must not resolve a replaceable user path: found %q", forbidden)
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
		`root_installer="$$(/bin/cat "$$root_installer_path")"`,
		`printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer"`,
		`printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`,
	} {
		if !strings.Contains(installer, required) {
			t.Errorf("helper installer is missing one-shot privileged boundary %q", required)
		}
	}
	if strings.Contains(installer, "/usr/bin/sudo -n -v") ||
		strings.Contains(installer, "/usr/bin/sudo -v") ||
		strings.Contains(installer, "/usr/bin/sudo /usr/bin/true") {
		t.Fatal("helper installer still uses reusable sudo validation")
	}
	if got := strings.Count(installer, "/usr/bin/sudo"); got != 2 {
		t.Fatalf("helper installer uses %d sudo invocations, want one probe and one transaction", got)
	}
	backup := strings.Index(installer, `launcher_txdir="$$(/usr/bin/mktemp -d`)
	activateLauncher := strings.Index(installer, `/bin/mv -f "$$agm_staging" "$$agm_executable"`)
	activateRootSet := strings.Index(installer, `printf 'INSTALL\n' | /usr/bin/sudo`)
	if backup < 0 || activateLauncher < 0 || activateRootSet < 0 ||
		backup > activateLauncher || activateLauncher > activateRootSet {
		t.Fatal("helper installer does not back up and activate the caller launchers before the rollback-protected root transaction")
	}
}

func TestLaunchDaemonInstallerRollsBackPartialActivation(t *testing.T) {
	transactionBytes, err := os.ReadFile("../../agm/internal/launchdaudit/install.go")
	if err != nil {
		t.Fatal(err)
	}
	transaction := string(transactionBytes)
	for _, required := range []string{
		`auditLive = "/usr/local/libexec/dear-agent-override-audit"`,
		`plistLive = "/Library/LaunchDaemons/com.dear-agent.override-audit.plist"`,
		`auditBackup string`,
		`plistBackup string`,
		`tx.activationStarted = true`,
		`tx.activationComplete = true`,
		`restoreArtifact(tx.config.auditLive, &tx.auditBackup, tx.auditExisted)`,
		`restoreArtifact(tx.config.plistLive, &tx.plistBackup, tx.plistExisted)`,
		`rollbackErrors = append(rollbackErrors, tx.cleanup(!tx.activationStarted))`,
	} {
		if !strings.Contains(transaction, required) {
			t.Errorf("LaunchDaemon installer is missing transactional activation fragment %q", required)
		}
	}
	backup := strings.Index(transaction, "tx.backupLiveSet()")
	activate := strings.Index(transaction, "tx.activateSet(ctx)")
	if backup < 0 || activate < 0 || backup > activate {
		t.Fatal("LaunchDaemon installer does not back up live artifacts before activation")
	}
}
