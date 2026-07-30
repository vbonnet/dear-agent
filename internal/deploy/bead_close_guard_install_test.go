package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexBeadCloseGuardUsesOperatorOwnedInstall(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(repoRoot, path))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	hook := read(filepath.Join(".codex", "hooks", "pretool-bead-close-guard"))
	if !strings.Contains(hook, `guard="/usr/local/libexec/dear-agent-bead-close-guard"`) {
		t.Fatal("attested Codex hook does not prefer the operator-owned guard path")
	}
	bypassDeny := strings.Index(hook, `if [ -n "${AGM_CODEX_HOOK_ROOT:-}" ]`)
	guardResolve := strings.Index(hook, `guard="/usr/local/libexec/dear-agent-bead-close-guard"`)
	if bypassDeny < 0 || guardResolve < 0 || bypassDeny >= guardResolve {
		t.Fatal("attested Codex hook does not deny closure before resolving the guard and its CLI dependencies")
	}
	if !strings.Contains(hook[bypassDeny:guardResolve], `--force does not bypass this direct hook decision`) {
		t.Fatal("attested Codex hook does not explicitly keep force-close behind the reviewed-session boundary")
	}

	makefile := read("Makefile")
	start := strings.Index(makefile, "install-bead-close-guard: build-bead-close-guard")
	end := strings.Index(makefile, "\n# Detects deployment drift:")
	if start < 0 || end <= start {
		t.Fatal("Makefile does not retain a bounded bead-close guard install target")
	}
	install := makefile[start:end]
	for _, required := range []string{
		"test -t 0",
		`expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"`,
		"IFS= read -r confirmed_hash",
		"/usr/bin/sudo -k",
		"/usr/bin/sudo -n -v",
		"/usr/bin/sudo -v",
		"/usr/bin/sudo /usr/bin/mktemp /usr/local/libexec/.dear-agent-bead-close-guard.XXXXXX",
		`test "$$staged_hash" = "$$expected_hash"`,
		`/usr/bin/sudo /bin/mv -f "$$guard_staging" "$$guard"`,
		"/usr/local/libexec/dear-agent-bead-close-guard",
		"$(call install-go-bin,bin/bead-close-guard)",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("bead-close guard installer does not retain %q", required)
		}
	}
	if strings.Contains(install, "bin/bead-close-guard /usr/local/libexec/dear-agent-bead-close-guard") {
		t.Fatal("bead-close guard installer copies mutable build output directly to the privileged path")
	}
}
