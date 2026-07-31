package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexHookJSONHelperUsesDigestBoundOperatorInstall(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	makefileBytes, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(makefileBytes)
	start := strings.Index(makefile, "install-codex-hook-json: build-codex-hook-json")
	end := strings.Index(makefile, "\n# Enforces Definition of Done")
	if start < 0 || end <= start {
		t.Fatal("Makefile does not retain a bounded Codex hook JSON helper install target")
	}
	install := makefile[start:end]
	for _, required := range []string{
		"test -t 0",
		`expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"`,
		`root_installer="$$(/bin/cat "$$root_installer_path")"`,
		`expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"`,
		"IFS= read -r confirmed_hash",
		"IFS= read -r confirmed_installer_hash",
		`printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer"`,
		`printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`,
		"/usr/local/libexec/dear-agent-codex-hook-json",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("Codex hook JSON helper installer does not retain %q", required)
		}
	}
	if strings.Contains(install, "bin/codex-hook-json /usr/local/libexec/dear-agent-codex-hook-json") {
		t.Fatal("Codex hook JSON helper installer copies mutable build output directly to the privileged path")
	}
	if got := strings.Count(install, "/usr/bin/sudo"); got != 2 {
		t.Fatalf("Codex hook JSON helper installer uses %d sudo calls, want one probe and one transaction", got)
	}
	for _, forbidden := range []string{
		"/usr/bin/sudo /usr/bin/true",
		"/usr/bin/sudo -n /usr/bin/true",
		"/usr/bin/sudo /usr/bin/install",
		"/usr/bin/sudo /bin/mv",
	} {
		if strings.Contains(install, forbidden) {
			t.Errorf("Codex hook JSON helper installer retains reusable sudo flow %q", forbidden)
		}
	}

	rootInstallerBytes, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "install-root-artifact.sh"))
	if err != nil {
		t.Fatal(err)
	}
	rootInstaller := string(rootInstallerBytes)
	for _, required := range []string{
		`test "$mode" != PROBE || exit 42`,
		`trusted_parent=/private/var/root`,
		`trusted "$dir"`,
		`test "$staged_hash" = "$expected_hash"`,
		`/bin/mv -f "$staging" "$destination"`,
	} {
		if !strings.Contains(rootInstaller, required) {
			t.Errorf("fixed root artifact installer lacks %q", required)
		}
	}

	for _, name := range []string{
		"pretool-bead-close-guard",
		"pretool-beads-dir-block",
		"pretool-bypass-guard",
		"pretool-pr-guard",
		"pretool-spawn-routing",
	} {
		hookBytes, readErr := os.ReadFile(filepath.Join(repoRoot, ".codex", "hooks", name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		hook := string(hookBytes)
		if !strings.Contains(hook, `hook_json() { /usr/local/libexec/dear-agent-codex-hook-json "$@"; }`) {
			t.Errorf("%s does not select the operator-owned JSON helper for bypassed hooks", name)
		}
		if !strings.Contains(hook, `if [ -n "${AGM_CODEX_HOOK_ROOT:-}" ]`) ||
			!strings.Contains(hook, "command -v jq") ||
			!strings.Contains(hook, `hook_json() { jq "$@"; }`) {
			t.Errorf("%s does not preserve ordinary reviewed-session jq lookup", name)
		}
	}
}
