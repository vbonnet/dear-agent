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
		`$(call install-go-bin-hardened,bin/agm)`,
		`$(call install-go-bin-hardened,bin/agm-mcp-server)`,
		`expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"`,
		`IFS= read -r confirmed_hash`,
		`test "$$confirmed_hash" = "$$expected_hash"`,
		`IFS= read -r confirmed_identity`,
		`test "$$confirmed_identity" = "$$caller_identity"`,
		`IFS= read -r confirmed_companion_identity`,
		`test "$$confirmed_companion_identity" = "$$companion_caller_identity"`,
		`/usr/bin/sudo /usr/bin/true`,
		`identity_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-ledger-agm.identity.XXXXXX)"`,
		`companion_identity_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-ledger-agm-mcp-server.identity.XXXXXX)"`,
		`helper_staging="$$(/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-override-ledger-append.XXXXXX)"`,
		`/usr/bin/sudo /usr/bin/install -o root -g "$$root_group" -m 0755 "$$artifact" "$$helper_staging"`,
		`staged_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$helper_staging")"`,
		`test "$$staged_hash" = "$$expected_hash"`,
		`/usr/bin/sudo /bin/mv -f "$$identity_staging" "$$identity"`,
		`/usr/bin/sudo /bin/mv -f "$$companion_identity_staging" "$$companion_policy"`,
		`/usr/bin/sudo /bin/mv -f "$$helper_staging" "$$helper"`,
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
	installMacro, err := os.ReadFile("../../mk/install-go-bin.mk")
	if err != nil {
		t.Fatalf("read shared install macro: %v", err)
	}
	for _, required := range []string{
		"define install-go-bin-hardened",
		`codesign -f -s - --options runtime "$$stage"`,
	} {
		if !strings.Contains(string(installMacro), required) {
			t.Fatalf("hardened launcher install macro lacks %q", required)
		}
	}
}
