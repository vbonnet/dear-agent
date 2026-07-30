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
		"IFS= read -r confirmed_hash",
		"/usr/bin/sudo -k",
		"/usr/bin/sudo -n -v",
		"/usr/bin/sudo -v",
		"/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-codex-hook-json.XXXXXX",
		`test "$$staged_hash" = "$$expected_hash"`,
		`/usr/bin/sudo /bin/mv -f "$$helper_staging" "$$helper"`,
		"/usr/local/libexec/dear-agent-codex-hook-json",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("Codex hook JSON helper installer does not retain %q", required)
		}
	}
	if strings.Contains(install, "bin/codex-hook-json /usr/local/libexec/dear-agent-codex-hook-json") {
		t.Fatal("Codex hook JSON helper installer copies mutable build output directly to the privileged path")
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
