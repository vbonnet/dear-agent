package speccoverage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParitySurfacesHaveSpecAndBDD(t *testing.T) {
	t.Parallel()
	findings, err := Validate(repoRoot())
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		t.Error(finding.Error())
	}
}

func TestParitySurfaceRegistryIsNotEmpty(t *testing.T) {
	t.Parallel()
	if len(ParitySurfaces()) == 0 {
		t.Fatal("parity surface registry is empty")
	}
}

func TestUnregisteredParityFeaturesIsEmpty(t *testing.T) {
	t.Parallel()
	missing, err := UnregisteredParityFeatures(repoRoot())
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("unregistered parity features: %v", missing)
	}
}

func TestValidateSurfaceDoesNotReadEmptyPaths(t *testing.T) {
	t.Parallel()
	findings := validateSurface(repoRoot(), Surface{Name: "empty surface"})
	if len(findings) != 3 {
		t.Fatalf("expected three structural findings, got %d: %v", len(findings), findings)
	}
	for _, finding := range findings {
		if finding.Path != "" {
			t.Fatalf("empty-path finding should not attempt file read: %v", finding)
		}
	}
}

func TestValidateGoPackageSpecsForFilesRequiresSpecForProductionGo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "newfeature"), 0o755); err != nil {
		t.Fatal(err)
	}

	findings := ValidateGoPackageSpecsForFiles(root, []string{"internal/newfeature/newfeature.go"})
	if len(findings) != 1 {
		t.Fatalf("expected one missing SPEC finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "internal/newfeature/SPEC.md" {
		t.Fatalf("finding path = %q, want internal/newfeature/SPEC.md", findings[0].Path)
	}
}

func TestValidateGoPackageSpecsForFilesAcceptsCoLocatedSpec(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "newfeature")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("# SPEC\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := ValidateGoPackageSpecsForFiles(root, []string{"internal/newfeature/newfeature.go"})
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestValidateGoPackageSpecsForFilesIgnoresTestOnlyChanges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	findings := ValidateGoPackageSpecsForFiles(root, []string{
		"internal/newfeature/newfeature_test.go",
		"agm/test/bdd/main.go",
		"tests/githooks/hook.go",
		"internal/newfeature/testdata/fixture.go",
	})
	if len(findings) != 0 {
		t.Fatalf("expected no findings for test-only changes, got %v", findings)
	}
}

func TestChangedGoFilesHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ChangedGoFiles(ctx, repoRoot(), "origin/main")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ChangedGoFiles error = %v, want context.Canceled", err)
	}
}

func repoRoot() string {
	return filepath.Clean(filepath.Join("..", ".."))
}
