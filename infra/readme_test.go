package infra

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadmePostApplyRulesetIDAssertion(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	infraDir := filepath.Dir(file)
	readme, err := os.ReadFile(filepath.Join(infraDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	var assertions []string
	for line := range strings.SplitSeq(string(readme), "\n") {
		if strings.Contains(line, `printf '%s\n' "$state"`) && strings.Contains(line, "18061003") {
			assertions = append(assertions, line)
		}
	}
	// Both the provider's ruleset_id attribute and the resource id the module
	// output reads must be asserted, so neither can drift unnoticed.
	if len(assertions) != 2 {
		t.Fatalf("found %d documented ruleset ID assertions, want 2", len(assertions))
	}

	fixture := filepath.Join(infraDir, "testdata", "github_repository_ruleset_state.txt")
	script := `set -euo pipefail
state="$(<"$1")"
` + strings.Join(assertions, "\n")
	cmd := exec.Command("bash", "-c", script, "post-apply-verification", fixture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("documented ruleset ID assertions rejected pinned-provider state: %v: %s", err, output)
	}

	// The assertions must actually discriminate: a state bound to a different
	// ruleset has to fail, or the documented procedure proves nothing.
	wrongFixture := filepath.Join(t.TempDir(), "wrong_ruleset_state.txt")
	pinned, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	wrong := strings.ReplaceAll(string(pinned), "18061003", "99")
	if err := os.WriteFile(wrongFixture, []byte(wrong), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("bash", "-c", script, "post-apply-verification", wrongFixture)
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("documented ruleset ID assertions accepted a foreign ruleset binding: %s", output)
	}
}

func TestReadmeRequiresIndependentExactPlanAttestation(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	readmeBytes, err := os.ReadFile(filepath.Join(filepath.Dir(file), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(readmeBytes)

	if got := strings.Count(readme, `printf 'plan_sha256=%s\n'`); got != 3 {
		t.Fatalf("documented plan digest emissions = %d, want 3", got)
	}
	if got := strings.Count(readme, `: "${ATTESTED_PLAN_SHA256:?independent exact-plan attestation required}"`); got != 3 {
		t.Fatalf("documented independent attestation gates = %d, want 3", got)
	}
	for _, required := range []string{
		"without human approval",
		"plan contains a destroy, replacement, state migration, irreversible change",
		"ambiguous effect",
	} {
		if !strings.Contains(readme, required) {
			t.Fatalf("dark-factory saved-plan guidance does not retain %q", required)
		}
	}
	for _, prohibited := range []string{"human reviews exact plan", "plan has been human-reviewed", "human must review a saved plan"} {
		if strings.Contains(readme, prohibited) {
			t.Fatalf("routine saved-plan guidance still requires a human gate via %q", prohibited)
		}
	}
}
