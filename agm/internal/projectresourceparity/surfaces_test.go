package projectresourceparity

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func TestValidateActiveHarnessSurfaces(t *testing.T) {
	if err := ValidateActiveHarnessSurfaces(); err != nil {
		t.Fatal(err)
	}
	if got, want := len(ActiveHarnessSurfaces()), len(agent.ActiveHarnesses()); got != want {
		t.Fatalf("project resource surfaces = %d, want %d", got, want)
	}
}

func TestSurfaceForHarnessDispositions(t *testing.T) {
	tests := []struct {
		harness                            string
		instructions, skills, config, hook Disposition
	}{
		{"claude-code", DispositionAdapted, DispositionSupported, DispositionSupported, DispositionSupported},
		{"codex-cli", DispositionSupported, DispositionAdapted, DispositionSupported, DispositionAdapted},
		{"agy", DispositionAdapted, DispositionAdapted, DispositionSupported, DispositionSupported},
		{"opencode-cli", DispositionAdapted, DispositionAdapted, DispositionSupported, DispositionAdapted},
		{"pi-cli", DispositionSupported, DispositionAdapted, DispositionSupported, DispositionAdapted},
	}
	for _, test := range tests {
		t.Run(test.harness, func(t *testing.T) {
			surface, ok := SurfaceForHarness(test.harness)
			if !ok {
				t.Fatalf("SurfaceForHarness(%q) not found", test.harness)
			}
			got := []Disposition{
				surface.Instructions.Disposition,
				surface.Skills.Disposition,
				surface.Configuration.Disposition,
				surface.Hooks.Disposition,
			}
			want := []Disposition{test.instructions, test.skills, test.config, test.hook}
			for index := range want {
				if got[index] != want[index] {
					t.Fatalf("SurfaceForHarness(%q) dispositions = %v, want %v", test.harness, got, want)
				}
			}
		})
	}
}

func TestValidatePiInstructionSurface(t *testing.T) {
	if err := ValidatePiInstructionSurface(projectResourceRepoRoot(t)); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePiInstructionSurfaceAcceptsRelativeRoot(t *testing.T) {
	root := piInstructionFixture(t)
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(workingDirectory, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePiInstructionSurface(relative); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePiInstructionSurfaceAcceptsSymlinkRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	root := piInstructionFixture(t)
	alias := filepath.Join(t.TempDir(), "repository")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePiInstructionSurface(alias); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePiInstructionSurfaceRejectsDivergentInstructions(t *testing.T) {
	root := piInstructionFixture(t)
	writeFixtureFile(t, filepath.Join(root, ".pi", "AGENTS.md"), "# Pi-only instructions\n")
	err := ValidatePiInstructionSurface(root)
	if err == nil || !strings.Contains(err.Error(), "divergent") {
		t.Fatalf("ValidatePiInstructionSurface() error = %v, want divergent-instruction rejection", err)
	}
}

func piInstructionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, "AGENTS.md"), "# Shared instructions\n")
	if err := os.MkdirAll(filepath.Join(root, ".pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func projectResourceRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
