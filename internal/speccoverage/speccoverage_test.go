package speccoverage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestBDDCatalogCoversExecutableFeatures(t *testing.T) {
	t.Parallel()
	for _, finding := range ValidateBDDCatalog(repoRoot()) {
		t.Error(finding.Error())
	}
}

func TestValidateBDDCatalogReportsUnlistedFeature(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/listed.feature", "Feature: Listed\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/unlisted.feature", "Feature: Unlisted\n")
	writeCoverageFile(t, root, "agm/docs/BDD-CATALOG.md", "**File:** [`listed.feature`](../test/bdd/features/listed.feature)\n")

	findings := ValidateBDDCatalog(root)
	if len(findings) != 1 {
		t.Fatalf("expected one uncataloged feature finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "agm/test/bdd/features/unlisted.feature" {
		t.Fatalf("finding path = %q", findings[0].Path)
	}
	if findings[0].Message != "BDD feature is not listed in agm/docs/BDD-CATALOG.md" {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestValidateBDDCatalogReportsMissingReferencedFeature(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/listed.feature", "Feature: Listed\n")
	writeCoverageFile(t, root, "agm/docs/BDD-CATALOG.md", strings.Join([]string{
		"**File:** [`listed.feature`](../test/bdd/features/listed.feature)",
		"**File:** [`missing.feature`](../test/bdd/features/missing.feature)",
	}, "\n"))

	findings := ValidateBDDCatalog(root)
	if len(findings) != 1 {
		t.Fatalf("expected one missing feature finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "agm/test/bdd/features/missing.feature" {
		t.Fatalf("finding path = %q", findings[0].Path)
	}
	if findings[0].Message != "BDD catalog references a missing feature file" {
		t.Fatalf("finding message = %q", findings[0].Message)
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

func TestValidateSurfaceRejectsNeedsAuditMarker(t *testing.T) {
	t.Parallel()
	for _, marker := range []string{"NEEDS-AUDIT", "needs-audit", ""} {
		t.Run(marker, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n<!-- Last audited at: "+marker+" -->\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall report stale audits.\n")
			writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "Feature: Example parity\n")

			findings := validateSurface(root, Surface{
				Name:        "example parity",
				PackagePath: "internal/example",
				SpecPath:    "internal/example/SPEC.md",
				FeaturePath: "agm/test/bdd/features/example_parity.feature",
			})
			if len(findings) != 1 {
				t.Fatalf("expected one stale audit finding, got %d: %v", len(findings), findings)
			}
			if findings[0].Message != "SPEC.md audit marker is missing or still NEEDS-AUDIT" {
				t.Fatalf("finding message = %q", findings[0].Message)
			}
		})
	}
}

func TestValidateSurfaceAcceptsCompletedAuditMarker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n<!-- Last audited at: 2026-07-01 -->\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall accept completed audits.\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "Feature: Example parity\n")

	findings := validateSurface(root, Surface{
		Name:        "example parity",
		PackagePath: "internal/example",
		SpecPath:    "internal/example/SPEC.md",
		FeaturePath: "agm/test/bdd/features/example_parity.feature",
	})
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
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

func writeCoverageFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
