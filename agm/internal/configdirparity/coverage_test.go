package configdirparity

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestValidateActiveDirectories(t *testing.T) {
	if err := ValidateActiveDirectories(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
	got := ActiveSurfaces()
	if len(got) != len(agent.ActiveHarnesses()) {
		t.Fatalf("directory surfaces = %d, want %d: %+v", len(got), len(agent.ActiveHarnesses()), got)
	}
}

func TestActiveHarnessDirectoryMappings(t *testing.T) {
	want := map[string]string{
		"claude-code":  ".claude",
		"codex-cli":    ".codex",
		"agy":          ".agents",
		"opencode-cli": ".opencode",
		"pi-cli":       ".pi",
	}
	for harness, dir := range want {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			t.Fatalf("SurfaceForHarness(%q) not found", harness)
		}
		if surface.Directory != dir {
			t.Errorf("%s directory = %q, want %q", harness, surface.Directory, dir)
		}
		if surface.Deprecated {
			t.Errorf("%s should not be deprecated", harness)
		}
	}
}

func TestValidateDeprecatedCompatibility(t *testing.T) {
	if err := ValidateDeprecatedCompatibility(repoRoot(t)); err != nil {
		t.Fatal(err)
	}
	surface, ok := SurfaceForHarness("gemini-cli")
	if !ok {
		t.Fatal("gemini-cli surface missing")
	}
	if !surface.Deprecated || surface.Directory != ".gemini" {
		t.Fatalf("gemini surface = %+v, want deprecated .gemini", surface)
	}
}
