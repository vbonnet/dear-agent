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
		`agm_staging="$$(/usr/bin/mktemp "$$agm_executable.XXXXXX")"`,
		`companion_staging="$$(/usr/bin/mktemp "$$companion_executable.XXXXXX")"`,
		`/bin/cp "$$agm_artifact" "$$agm_staging"`,
		`/bin/cp "$$companion_artifact" "$$companion_staging"`,
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
		`printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`,
		`/bin/rm -f "$$agm_staging" "$$companion_staging"`,
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
		`/usr/sbin/visudo -cf "$txdir/sudoers"`,
		`helper_existed=0`,
		`helper_stage=$(/usr/bin/mktemp`,
		`activation_started=1`,
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
	if !strings.Contains(rootInstaller, `test "$activation_started" = 1 && test "$activation_complete" != 1`) {
		t.Fatal("fixed root installer lacks rollback for a partially activated artifact set")
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
