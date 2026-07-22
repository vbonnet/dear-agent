package agent

import "testing"

func TestActiveHarnessesCanonicalParitySet(t *testing.T) {
	want := []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}
	got := ActiveHarnesses()
	if len(got) != len(want) {
		t.Fatalf("ActiveHarnesses length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ActiveHarnesses()[%d] = %q, want %q (all: %v)", i, got[i], want[i], got)
		}
		if IsDeprecatedHarness(got[i]) {
			t.Fatalf("active harness %q must not be deprecated", got[i])
		}
	}
}

func TestGeminiCLIIsDeprecatedButKnown(t *testing.T) {
	if !IsDeprecatedHarness("gemini-cli") {
		t.Fatal("gemini-cli should be deprecated")
	}
	if err := ValidateHarnessName("gemini-cli"); err != nil {
		t.Fatalf("deprecated gemini-cli should remain accepted for compatibility: %v", err)
	}
	for _, active := range ActiveHarnesses() {
		if active == "gemini-cli" {
			t.Fatal("gemini-cli must not be in the active parity set")
		}
	}
}

func TestCodexFactoryUsesCLIAdapter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := GetHarness("codex-cli")
	if err != nil {
		t.Fatalf("GetHarness(codex-cli) returned error: %v", err)
	}
	if _, ok := got.(*CodexCLIAdapter); !ok {
		t.Fatalf("GetHarness(codex-cli) = %T, want *CodexCLIAdapter", got)
	}
}

func TestPiFactoryUsesNativeAdapter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := GetHarness("pi")
	if err != nil {
		t.Fatalf("GetHarness(pi) returned error: %v", err)
	}
	if _, ok := got.(*PiAdapter); !ok {
		t.Fatalf("GetHarness(pi) = %T, want *PiAdapter", got)
	}
}

func TestActiveHarnessFactoryNamesMatchCanonicalNames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, harness := range ActiveHarnesses() {
		got, err := GetHarness(harness)
		if err != nil {
			t.Fatalf("GetHarness(%q) returned error: %v", harness, err)
		}
		if got.Name() != harness {
			t.Fatalf("GetHarness(%q).Name() = %q, want %q", harness, got.Name(), harness)
		}
	}
}

func TestAntigravityAliasesNormalizeToAGY(t *testing.T) {
	for _, input := range []string{"agy", "agy-cli", "antigravity"} {
		if got := NormalizeHarnessName(input); got != "agy" {
			t.Fatalf("NormalizeHarnessName(%q) = %q, want agy", input, got)
		}
		if err := ValidateHarnessName(input); err != nil {
			t.Fatalf("ValidateHarnessName(%q) returned error: %v", input, err)
		}
	}
}
