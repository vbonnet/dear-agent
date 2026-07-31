package deploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestOverrideLedgerHelperInstallBindsApprovedBytesBeforeActivation(t *testing.T) {
	data, err := os.ReadFile("../../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(data)
	start := strings.Index(makefile, "install-override-ledger-helper: build-override-ledger-helper")
	if start < 0 {
		t.Fatal("install-override-ledger-helper target is missing")
	}
	end := strings.Index(makefile[start:], "\n# Install the macOS audit")
	if end < 0 {
		t.Fatal("cannot find end of install-override-ledger-helper target")
	}
	target := makefile[start : start+end]

	requiredInOrder := []string{
		`Darwin) install_lock_path="/private/var/run"`,
		`Linux) install_lock_path="/run"`,
		`test ! -L "$$install_lock_ancestor"`,
		`test ! -w "$$install_lock_ancestor"`,
		`test "$$install_lock_uid" = 0`,
		`DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED=1 /usr/bin/lockf -k -t 0`,
		`DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED=1 /usr/bin/flock -n`,
		`install-override-ledger-helper-locked:`,
		`test "$${DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED:-}" = 1`,
		`agm_staging="$$(/usr/bin/mktemp "$$agm_executable.XXXXXX")"`,
		`companion_staging="$$(/usr/bin/mktemp "$$companion_executable.XXXXXX")"`,
		`/bin/cp "$$agm_artifact" "$$agm_staging"`,
		`/bin/cp "$$companion_artifact" "$$companion_staging"`,
		`agm_policy_artifact="$$(/usr/bin/mktemp "$$agm_executable.policy.XXXXXX")"`,
		`/bin/cp "$$agm_staging" "$$agm_policy_artifact"`,
		`transaction_nonce="$$(/usr/bin/openssl rand -hex 32)"`,
		`expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"`,
		`expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"`,
		`IFS= read -r confirmed_hash`,
		`test "$$confirmed_hash" = "$$expected_hash"`,
		`IFS= read -r confirmed_identity`,
		`test "$$confirmed_identity" = "$$caller_identity"`,
		`IFS= read -r confirmed_companion_identity`,
		`test "$$confirmed_companion_identity" = "$$companion_caller_identity"`,
		`IFS= read -r confirmed_installer_hash`,
		`test "$$confirmed_installer_hash" = "$$expected_installer_hash"`,
		`forward_privileged() { signal=$$1; status=$$2;`,
		`printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer"`,
		`launcher_txdir="$$(/usr/bin/mktemp -d "$(HOME)/go/bin/.dear-agent-launchers.XXXXXX")"`,
		`/bin/mv -f "$$agm_staging" "$$agm_executable"`,
		`/bin/mv -f "$$companion_staging" "$$companion_executable"`,
		`launcher_set_active=1`,
		`printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`,
		`root_transaction_committed && launcher_activation_complete=1`,
	}
	offset := 0
	for _, want := range requiredInOrder {
		next := strings.Index(target[offset:], want)
		if next < 0 {
			t.Fatalf("install target lacks ordered immutable-byte boundary %q", want)
		}
		offset += next + len(want)
	}
	if strings.Contains(target, `-m 0755 bin/dear-agent-override-ledger-append "$$helper"`) {
		t.Fatal("install target still copies mutable same-user bytes directly to the privileged helper")
	}
	if strings.Contains(target, "/tmp/dear-agent-override-ledger-install.lock") ||
		strings.Contains(target, `: >>"$$install_lock`) {
		t.Fatal("installer still uses a same-user-replaceable regular-file lock")
	}
	if strings.Contains(target, "command -v lockf") ||
		strings.Contains(target, "command -v flock") {
		t.Fatal("installer still resolves a same-user-selectable lock utility from PATH")
	}
	if !strings.Contains(target, `if test "$$launcher_set_active" = 1 && root_transaction_committed; then launcher_activation_complete=1; fi`) {
		t.Fatal("signal recovery does not distinguish a complete launcher set from a partial user activation")
	}
	if strings.Contains(target, `install_lock_held`) || strings.Contains(target, `/bin/mkdir "$$install_lock_dir"`) {
		t.Fatal("installer still uses a crash-persistent directory lock")
	}
	if !strings.Contains(target, `exec /usr/bin/env DEAR_AGENT_OVERRIDE_LEDGER_INSTALL_LOCKED=1`) {
		t.Fatal("installer does not hold the automatically released operating-system lock across the recursive transaction")
	}
	if got := strings.Count(target, "/usr/bin/sudo"); got != 2 {
		t.Fatalf("install target uses %d sudo calls, want one probe and one transaction", got)
	}
	confirmed := strings.Index(target, `test "$$confirmed_companion_identity" = "$$companion_caller_identity"`)
	transaction := strings.Index(target, `printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`)
	if confirmed < 0 || transaction < 0 || transaction <= confirmed {
		t.Fatal("privileged transaction is not delayed until all operator confirmations pass")
	}

	rootInstallerBytes, err := os.ReadFile("../../scripts/install-override-ledger-root.sh")
	if err != nil {
		t.Fatalf("read fixed root installer: %v", err)
	}
	rootInstaller := string(rootInstallerBytes)
	requiredRootOrder := []string{
		`test "$mode" != PROBE || exit 42`,
		`trusted_parent=/private/var/root`,
		`txdir=$(/usr/bin/mktemp -d`,
		`"$helper_artifact" "$txdir/helper"`,
		`test "$staged_hash" = "$expected_hash"`,
		`test "darwin-cdhash:$agm_digest" = "$caller_identity"`,
		`printf '%s\n' "$receipt_nonce" >"$txdir/receipt"`,
		`/usr/sbin/visudo -cf "$txdir/sudoers"`,
		`helper_existed=0`,
		`helper_stage=$(/usr/bin/mktemp`,
		`receipt_stage=$(/usr/bin/mktemp`,
		`activation_started=1`,
		`/bin/mv -f "$receipt_stage" "$receipt"`,
		`activation_complete=1`,
	}
	offset = 0
	for _, want := range requiredRootOrder {
		next := strings.Index(rootInstaller[offset:], want)
		if next < 0 {
			t.Fatalf("fixed root installer lacks ordered transaction boundary %q", want)
		}
		offset += next + len(want)
	}
	if strings.Contains(rootInstaller, "/usr/bin/sudo ") {
		t.Fatal("fixed root installer recursively invokes sudo")
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
		if strings.Contains(rootInstaller, forbidden) {
			t.Errorf("fixed root installer must not resolve a replaceable user path: found %q", forbidden)
		}
	}
	if !strings.Contains(rootInstaller, `test "$activation_started" = 1 && test "$activation_complete" != 1`) {
		t.Fatal("fixed root installer lacks rollback for a partially activated root-owned artifact set")
	}
	if !strings.Contains(rootInstaller, `restore "$receipt" "$txdir/receipt.backup" "$receipt_existed"`) {
		t.Fatal("fixed root installer does not roll back its transaction-specific commit receipt")
	}
	executableLines := 0
	for line := range strings.SplitSeq(rootInstaller, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			executableLines++
		}
	}
	if executableLines > 20 {
		t.Fatalf("fixed root installer has %d executable lines, want at most 20", executableLines)
	}
}
