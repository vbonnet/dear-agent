package speccoverage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/specguard"
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

func TestRepositoryImplementationSpecsAndBDD(t *testing.T) {
	root := repoRoot()
	findings, err := ValidateAllImplementationSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	repositorySpecFindings, err := ValidateAllRepositorySpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	findings = append(findings, repositorySpecFindings...)
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
	writeCoverageFile(t, root, "agm/docs/BDD-CATALOG.md", strings.Join([]string{
		"unlisted.feature was mentioned in prose but is not cataloged.",
		"**File:** [`listed.feature`](../test/bdd/features/listed.feature)",
	}, "\n"))

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

func TestValidateBDDFeatureTraceabilityReportsMissingSpecMarker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/untraced.feature", "Feature: Untraced\n")

	findings := ValidateBDDFeatureTraceability(root)
	if len(findings) != 1 {
		t.Fatalf("expected one missing SPEC marker finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "agm/test/bdd/features/untraced.feature" {
		t.Fatalf("finding path = %q", findings[0].Path)
	}
	if findings[0].Message != "BDD feature does not declare governing SPEC.md" {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestValidateBDDFeatureTraceabilityReportsMissingSpec(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/missing_spec.feature", "# SPEC: internal/missing/SPEC.md\nFeature: Missing SPEC\n")

	findings := ValidateBDDFeatureTraceability(root)
	if len(findings) != 1 {
		t.Fatalf("expected one missing SPEC finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "internal/missing/SPEC.md" {
		t.Fatalf("finding path = %q", findings[0].Path)
	}
	if !strings.Contains(findings[0].Message, "BDD feature references a missing SPEC.md") {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestValidateBDDFeatureTraceabilityReportsMissingBackReference(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/missing_backref.feature", "# SPEC: internal/example/SPEC.md\nFeature: Missing backref\n")
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall keep BDD traceability.\n")

	findings := ValidateBDDFeatureTraceability(root)
	if len(findings) != 1 {
		t.Fatalf("expected one missing back-reference finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Path != "internal/example/SPEC.md" {
		t.Fatalf("finding path = %q", findings[0].Path)
	}
	if findings[0].Message != "governing or related SPEC.md does not reference executable BDD feature: agm/test/bdd/features/missing_backref.feature" {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestValidateBDDFeatureTraceabilityReportsMissingRelatedSpec(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/related.feature", "# SPEC: internal/primary/SPEC.md\n# RELATED-SPEC: internal/missing/SPEC.md\nFeature: Related\n")
	writeCoverageFile(t, root, "internal/primary/SPEC.md", "- Feature: `agm/test/bdd/features/related.feature`\n")

	findings := ValidateBDDFeatureTraceability(root)
	if len(findings) != 1 || findings[0].Path != "internal/missing/SPEC.md" {
		t.Fatalf("expected missing related SPEC finding, got %v", findings)
	}
}

func TestValidateBDDFeatureTraceabilityReportsMissingRelatedBackReference(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/related.feature", "# SPEC: internal/primary/SPEC.md\n# RELATED-SPEC: internal/related/SPEC.md\nFeature: Related\n")
	writeCoverageFile(t, root, "internal/primary/SPEC.md", "- Feature: `agm/test/bdd/features/related.feature`\n")
	writeCoverageFile(t, root, "internal/related/SPEC.md", "# Related\n")

	findings := ValidateBDDFeatureTraceability(root)
	if len(findings) != 1 || findings[0].Path != "internal/related/SPEC.md" {
		t.Fatalf("expected missing related back-reference finding, got %v", findings)
	}
}

func TestValidateBDDFeatureTraceabilityAcceptsReciprocalTrace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/traced.feature", "# SPEC: internal/example/SPEC.md\nFeature: Traced\n")
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall keep BDD traceability.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/traced.feature`\n")

	findings := ValidateBDDFeatureTraceability(root)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestValidateBDDFeatureTraceabilityAcceptsReciprocalRelatedTrace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/traced.feature", "# SPEC: internal/primary/SPEC.md\n# RELATED-SPEC: internal/related/SPEC.md\nFeature: Traced\n")
	writeCoverageFile(t, root, "internal/primary/SPEC.md", "- Feature: `agm/test/bdd/features/traced.feature`\n")
	writeCoverageFile(t, root, "internal/related/SPEC.md", "- Feature: `agm/test/bdd/features/traced.feature`\n")

	findings := ValidateBDDFeatureTraceability(root)
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
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
			writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n<!-- Last audited at: "+marker+" -->\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall report stale audits.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example_parity.feature`\n")
			writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "# SPEC: internal/example/SPEC.md\nFeature: Example parity\n")

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
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n<!-- Last audited at: 2026-07-01 -->\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall accept completed audits.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example_parity.feature`\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "# SPEC: internal/example/SPEC.md\nFeature: Example parity\n")

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

func TestValidateSurfaceRejectsInvalidEARSRequirement(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/SPEC.md", strings.Join([]string{
		"# SPEC",
		"",
		"<!-- Last audited at: 2026-07-01 -->",
		"",
		"## EARS Requirements",
		"",
		"**EX-01** Eventually the system shall maybe work.",
		"**EX-02** Somehow the system shall become correct.",
		"",
		"## BDD Traceability",
		"",
		"- Feature: `agm/test/bdd/features/example_parity.feature`",
	}, "\n"))
	writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "# SPEC: internal/example/SPEC.md\nFeature: Example parity\n")

	findings := validateSurface(root, Surface{
		Name:        "example parity",
		PackagePath: "internal/example",
		SpecPath:    "internal/example/SPEC.md",
		FeaturePath: "agm/test/bdd/features/example_parity.feature",
	})
	if len(findings) != 3 {
		t.Fatalf("expected all invalid EARS findings, got %d: %v", len(findings), findings)
	}
	want := []string{
		"SPEC.md has invalid EARS syntax: line 7: requirement does not match any EARS pattern",
		"SPEC.md has invalid EARS syntax: line 8: requirement does not match any EARS pattern",
		"SPEC.md has invalid EARS syntax: no valid EARS requirements found",
	}
	for i, message := range want {
		if findings[i].Message != message {
			t.Fatalf("finding[%d] message = %q, want %q", i, findings[i].Message, message)
		}
	}
}

func TestValidateSurfaceSkipsEARSLintWithoutEARSSection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n<!-- Last audited at: 2026-07-01 -->\n\n**EX-01** Eventually the system shall maybe work.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example_parity.feature`\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "# SPEC: internal/example/SPEC.md\nFeature: Example parity\n")

	findings := validateSurface(root, Surface{
		Name:        "example parity",
		PackagePath: "internal/example",
		SpecPath:    "internal/example/SPEC.md",
		FeaturePath: "agm/test/bdd/features/example_parity.feature",
	})
	if len(findings) != 1 {
		t.Fatalf("expected one missing EARS section finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Message != "SPEC.md does not declare EARS requirements" {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestValidateSurfaceRequiresSpecToReferenceFeature(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n<!-- Last audited at: 2026-07-01 -->\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall reject missing feature traceability.\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "# SPEC: internal/example/SPEC.md\nFeature: Example parity\n")

	findings := validateSurface(root, Surface{
		Name:        "example parity",
		PackagePath: "internal/example",
		SpecPath:    "internal/example/SPEC.md",
		FeaturePath: "agm/test/bdd/features/example_parity.feature",
	})
	if len(findings) != 1 {
		t.Fatalf("expected one missing feature reference finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Message != "SPEC.md does not reference its executable BDD feature" {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestValidateSurfaceRequiresFeatureToReferenceSpec(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# SPEC\n\n<!-- Last audited at: 2026-07-01 -->\n\n## EARS Requirements\n\n**EX-01** When an example is validated, the system shall reject missing SPEC traceability.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example_parity.feature`\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example_parity.feature", "Feature: Example parity\n")

	findings := validateSurface(root, Surface{
		Name:        "example parity",
		PackagePath: "internal/example",
		SpecPath:    "internal/example/SPEC.md",
		FeaturePath: "agm/test/bdd/features/example_parity.feature",
	})
	if len(findings) != 1 {
		t.Fatalf("expected one missing SPEC reference finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Message != "BDD feature does not reference its governing SPEC.md" {
		t.Fatalf("finding message = %q", findings[0].Message)
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
	spec := "# SPEC\n\n## EARS Requirements\n\n**NEW-01** When a new feature changes, the system shall keep a valid SPEC.\n"
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := ValidateGoPackageSpecsForFiles(root, []string{"internal/newfeature/newfeature.go"})
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %v", findings)
	}
}

func TestValidateGoPackageSpecsForFilesRequiresEARSSection(t *testing.T) {
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
	if len(findings) != 1 {
		t.Fatalf("expected one missing EARS section finding, got %d: %v", len(findings), findings)
	}
	if findings[0].Message != "SPEC.md has invalid EARS syntax: no valid EARS requirements found" {
		t.Fatalf("finding message = %q", findings[0].Message)
	}
}

func TestValidateGoPackageSpecsForFilesAcceptsEstablishedRequirementsHeading(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "newfeature")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := "# SPEC\n\n## Requirements\n\n**NEW-01** When coverage is checked, the system shall validate EARS semantics.\n"
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := ValidateGoPackageSpecsForFiles(root, []string{"internal/newfeature/newfeature.go"})
	if len(findings) != 0 {
		t.Fatalf("expected semantic EARS validation independent of heading, got %v", findings)
	}
}

func TestValidateGoPackageSpecsForFilesRequiresStrictEARSLint(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "internal", "newfeature")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	spec := strings.Join([]string{
		"# SPEC",
		"",
		"## EARS Requirements",
		"",
		"**NEW-01** Eventually the system shall maybe work.",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := ValidateGoPackageSpecsForFiles(root, []string{"internal/newfeature/newfeature.go"})
	if len(findings) != 2 {
		t.Fatalf("expected strict EARS lint findings, got %d: %v", len(findings), findings)
	}
	if findings[0].Message != "SPEC.md has invalid EARS syntax: line 5: requirement does not match any EARS pattern" {
		t.Fatalf("finding[0] message = %q", findings[0].Message)
	}
	if findings[1].Message != "SPEC.md has invalid EARS syntax: no valid EARS requirements found" {
		t.Fatalf("finding[1] message = %q", findings[1].Message)
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

func TestValidateAllGoPackageSpecsAcceptsProductionAndTestOnlyPackages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRepositoryPackageCoverage(t, root, "internal/production", "production.go")
	writeRepositoryPackageCoverage(t, root, "testutil/assertions", "assertions_test.go")

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected complete repository coverage, got %v", findings)
	}
}

func TestValidateAllGoPackageSpecsRequiresSpecForTestOnlyPackage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "integration_test/example_test.go", "package integration_test\n")

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Path != "integration_test/SPEC.md" {
		t.Fatalf("expected missing test-only package SPEC finding, got %v", findings)
	}
}

func TestValidateAllGoPackageSpecsRequiresStrictEARS(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/example.go", "package example\n")
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# Example\n\n## EARS Requirements\n\n**EX-01** Eventually this maybe works.\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example.feature", "# SPEC: internal/example/SPEC.md\nFeature: Example\n")

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 || !strings.Contains(findings[0].Message, "invalid EARS syntax") {
		t.Fatalf("expected strict EARS findings, got %v", findings)
	}
}

func TestValidateAllGoPackageSpecsAcceptsStrictEARSUnderEstablishedHeading(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/example.go", "package example\n")
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# Example\n\n## Requirements\n\n**EX-01** When coverage is checked, the system shall validate EARS semantics.\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example.feature", "# SPEC: internal/example/SPEC.md\nFeature: Example\n")

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected semantic EARS validation independent of heading, got %v", findings)
	}
}

func TestValidateAllGoPackageSpecsRequiresBDDReference(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/example.go", "package example\n")
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# Example\n\n## EARS Requirements\n\n**EX-01** When coverage is checked, the system shall require BDD coverage.\n")

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Message != "SPEC.md does not reference an executable BDD feature" {
		t.Fatalf("expected missing BDD reference finding, got %v", findings)
	}
}

func TestValidateAllGoPackageSpecsRequiresReciprocalBDDTraceability(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/example.go", "package example\n")
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# Example\n\n## EARS Requirements\n\n**EX-01** When coverage is checked, the system shall require reciprocal traceability.\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/example.feature", "# SPEC: internal/other/SPEC.md\nFeature: Example\n")

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "reciprocal SPEC traceability") {
		t.Fatalf("expected reciprocal traceability finding, got %v", findings)
	}
}

func TestValidateAllGoPackageSpecsRejectsMissingBDDFeature(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "internal/example/example.go", "package example\n")
	writeCoverageFile(t, root, "internal/example/SPEC.md", "# Example\n\n## EARS Requirements\n\n**EX-01** When coverage is checked, the system shall reject stale BDD references.\n\n- Feature: `agm/test/bdd/features/missing.feature`\n")

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "missing executable BDD feature") {
		t.Fatalf("expected missing BDD feature finding, got %v", findings)
	}
}

func TestValidateAllGoPackageSpecsSkipsNestedWorktreesAndOutputs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRepositoryPackageCoverage(t, root, "internal/example", "example.go")
	for _, path := range []string{
		".worktrees/branch/uncovered.go",
		"vendor/dependency/uncovered.go",
		"node_modules/tool/uncovered.go",
		"build/generated/uncovered.go",
		"bin/generated/uncovered.go",
	} {
		writeCoverageFile(t, root, path, "package ignored\n")
	}

	findings, err := ValidateAllGoPackageSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected excluded directories to be skipped, got %v", findings)
	}
}

func TestRepositoryCoverageExcludesIgnoredAndGeneratedImplementationPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if out, err := gittest.Output(t, root, "init", "-q"); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	writeCoverageFile(t, root, ".gitignore", ".beads/\nagm/agm-plugin/channels/agm-bus/dist/\n*~\n")
	writeRepositoryPackageCoverage(t, root, "internal/example", "example.go")
	for _, path := range []string{
		".beads/hooks/uncovered.go",
		"agm/agm-plugin/channels/agm-bus/dist/uncovered.js",
		"~/beads/context-engine/.beads/embeddeddolt/beads/.dolt/uncovered.go",
		"~/beads/context-engine/embeddeddolt/beads/.dolt/uncovered.go",
	} {
		writeCoverageFile(t, root, path, "package ignored\n")
	}

	findings, err := ValidateAllImplementationSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	repositoryFindings, err := ValidateAllRepositorySpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	findings = append(findings, repositoryFindings...)
	if len(findings) != 0 {
		t.Fatalf("ignored repository paths produced coverage findings: %v", findings)
	}
}

func TestValidateAllImplementationSpecsIncludesNonGoAndExecutableSources(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRepositoryPackageCoverage(t, root, "services/typescript", "index.ts")
	writeRepositoryPackageCoverage(t, root, "tools/shell", "deploy.sh")
	writeRepositoryPackageCoverage(t, root, "tools/rust", "main.rs")
	writeRepositoryPackageCoverage(t, root, "config/harness", "settings.yaml")
	writeRepositoryPackageCoverage(t, root, "containers/image", "Dockerfile")
	writeRepositoryPackageCoverage(t, root, "automation", "Makefile")
	writeRepositoryPackageCoverage(t, root, "tools/executable", "hook")
	if err := os.Chmod(filepath.Join(root, "tools", "executable", "hook"), 0o755); err != nil {
		t.Fatal(err)
	}

	findings, err := ValidateAllImplementationSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected complete cross-language coverage, got %v", findings)
	}
}

func TestValidateAllImplementationSpecsRequiresNonGoSpec(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "services/typescript/index.ts", "export const value = true;\n")

	findings, err := ValidateAllImplementationSpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Path != "services/typescript/SPEC.md" {
		t.Fatalf("expected missing TypeScript SPEC finding, got %v", findings)
	}
}

func TestValidateAllImplementationSpecsAcceptsCanonicalSharedSpecOwner(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"SPEC.md", ".dear-agent/SPEC.md", "internal/shared/SPEC.md"} {
		t.Run(strings.ReplaceAll(target, "/", "_"), func(t *testing.T) {
			root := t.TempDir()
			writeRepositorySharedCoverage(t, root, ".opencode/plugins", "adapter.mjs", target)
			findings, err := ValidateAllImplementationSpecs(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 0 {
				t.Fatalf("canonical shared SPEC owner produced findings: %v", findings)
			}
		})
	}
}

func TestValidateChangedGoPackageSpecsAcceptsCanonicalSharedSpecOwner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRepositorySharedCoverage(t, root, "adapters/provider", "adapter.go", "internal/shared/SPEC.md")

	findings := ValidateGoPackageSpecsForFiles(root, []string{"adapters/provider/adapter.go"})
	if len(findings) != 0 {
		t.Fatalf("changed package shared SPEC owner produced findings: %v", findings)
	}
}

func TestValidateChangedGoPackageSpecsRejectsInvalidSharedSpecOwner(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		owner string
		local bool
		kind  FindingKind
	}{
		{name: "malformed", owner: "../shared/SPEC.md\n", kind: FindingKindInvalidSpecOwner},
		{name: "missing target", owner: "internal/missing/SPEC.md\n", kind: FindingKindSpecRead},
		{name: "ambiguous local", owner: "internal/shared/SPEC.md\n", local: true, kind: FindingKindInvalidSpecOwner},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeCoverageFile(t, root, "adapters/provider/adapter.go", "package provider\n")
			writeCoverageFile(t, root, "adapters/provider/SPEC.owner", test.owner)
			if test.local {
				writeRepositoryPackageCoverage(t, root, "adapters/provider", "support.go")
			}
			findings := ValidateGoPackageSpecsForFiles(root, []string{"adapters/provider/adapter.go"})
			if len(findings) != 1 || findings[0].Kind != test.kind {
				t.Fatalf("changed-Go invalid owner findings = %v, want one %s", findings, test.kind)
			}
		})
	}
}

func TestValidateAllImplementationSpecsRejectsInvalidSharedSpecOwners(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		owner string
	}{
		{name: "empty", owner: ""},
		{name: "traversal", owner: "../shared/SPEC.md\n"},
		{name: "noncanonical", owner: "internal/../shared/SPEC.md\n"},
		{name: "absolute", owner: "/internal/shared/SPEC.md\n"},
		{name: "backslash", owner: "internal\\shared\\SPEC.md\n"},
		{name: "multiple lines", owner: "internal/one/SPEC.md\ninternal/two/SPEC.md\n"},
		{name: "not a spec", owner: "internal/shared/README.md\n"},
		{name: "dotted harness owned", owner: ".opencode/shared/SPEC.md\n"},
		{name: "bare harness owned", owner: "codex-cli/SPEC.md\n"},
		{name: "plugin collection", owner: "internal/plugins/SPEC.md\n"},
		{name: "colon", owner: "internal/shared:variant/SPEC.md\n"},
		{name: "fragment", owner: "internal/shared/SPEC.md#fragment\n"},
		{name: "glob", owner: "internal/*/SPEC.md\n"},
		{name: "tilde", owner: "~/shared/SPEC.md\n"},
		{name: "crlf", owner: "internal/shared/SPEC.md\r\n"},
		{name: "control", owner: "internal/\x01shared/SPEC.md\n"},
		{name: "invalid utf8", owner: string([]byte{'i', 'n', 't', 'e', 'r', 'n', 'a', 'l', '/', 0xff, '/', 'S', 'P', 'E', 'C', '.', 'm', 'd', '\n'})},
		{name: "oversized", owner: strings.Repeat("a", specguard.MaxSpecOwnerBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeCoverageFile(t, root, "adapters/provider/adapter.mjs", "export default {};\n")
			writeCoverageFile(t, root, "adapters/provider/SPEC.owner", test.owner)
			findings, err := ValidateAllImplementationSpecs(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 1 || findings[0].Kind != FindingKindInvalidSpecOwner || findings[0].Path != "adapters/provider/SPEC.owner" {
				t.Fatalf("invalid owner findings = %v", findings)
			}
		})
	}
}

func TestValidateAllRepositorySpecsRejectsOrphanSharedSpecOwner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRepositorySharedCoverage(t, root, "adapters/provider", "adapter.mjs", "internal/shared/SPEC.md")
	if err := os.Remove(filepath.Join(root, "adapters", "provider", "adapter.mjs")); err != nil {
		t.Fatal(err)
	}

	findings, err := ValidateAllRepositorySpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Kind != FindingKindInvalidSpecOwner || !strings.Contains(findings[0].Message, "does not belong") {
		t.Fatalf("orphan SPEC.owner findings = %v", findings)
	}
}

func TestValidateAllImplementationSpecsRejectsSpecOwnerSymlinksAndIgnoredTargets(t *testing.T) {
	t.Parallel()
	t.Run("owner symlink", func(t *testing.T) {
		root := t.TempDir()
		writeCoverageFile(t, root, "adapters/provider/adapter.mjs", "export default {};\n")
		writeCoverageFile(t, root, "owner-target", "internal/shared/SPEC.md\n")
		if err := os.Symlink(filepath.Join("..", "..", "owner-target"), filepath.Join(root, "adapters", "provider", "SPEC.owner")); err != nil {
			t.Fatal(err)
		}
		findings, err := ValidateAllImplementationSpecs(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Kind != FindingKindInvalidSpecOwner || !strings.Contains(findings[0].Message, "non-symlink") {
			t.Fatalf("owner symlink findings = %v", findings)
		}
	})

	t.Run("target symlink", func(t *testing.T) {
		root := t.TempDir()
		writeRepositorySharedCoverage(t, root, "adapters/provider", "adapter.mjs", "internal/shared/SPEC.md")
		target := filepath.Join(root, "internal", "shared", "SPEC.md")
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		writeCoverageFile(t, root, "internal/canonical/SPEC.md", validRepositoryCoverageSpec("agm/test/bdd/features/shared_owner.feature"))
		if err := os.Symlink(filepath.Join("..", "canonical", "SPEC.md"), target); err != nil {
			t.Fatal(err)
		}
		findings, err := ValidateAllImplementationSpecs(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Kind != FindingKindSpecRead || !strings.Contains(findings[0].Message, "non-symlink") {
			t.Fatalf("target symlink findings = %v", findings)
		}
	})

	t.Run("ignored target", func(t *testing.T) {
		root := t.TempDir()
		if out, err := gittest.Output(t, root, "init", "-q"); err != nil {
			t.Fatalf("git init: %v: %s", err, out)
		}
		writeCoverageFile(t, root, ".gitignore", "internal/shared/\n")
		writeRepositorySharedCoverage(t, root, "adapters/provider", "adapter.mjs", "internal/shared/SPEC.md")
		findings, err := ValidateAllImplementationSpecs(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Kind != FindingKindSpecRead || !strings.Contains(findings[0].Message, "governed repository inventory") {
			t.Fatalf("ignored target findings = %v", findings)
		}
	})
}

func TestValidateAllImplementationSpecsRejectsPointerChainsAndNoncanonicalFeatureBacklinks(t *testing.T) {
	t.Parallel()
	t.Run("pointer chain", func(t *testing.T) {
		root := t.TempDir()
		writeCoverageFile(t, root, "adapters/provider/adapter.mjs", "export default {};\n")
		writeCoverageFile(t, root, "adapters/provider/SPEC.owner", "internal/shared/SPEC.md\n")
		writeCoverageFile(t, root, "internal/shared/SPEC.md", "internal/canonical/SPEC.md\n")
		findings, err := ValidateAllImplementationSpecs(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Kind != FindingKindInvalidSpecOwner || !strings.Contains(findings[0].Message, "another ownership pointer") {
			t.Fatalf("pointer-chain findings = %v", findings)
		}
	})

	t.Run("feature references pointer", func(t *testing.T) {
		root := t.TempDir()
		writeRepositorySharedCoverage(t, root, "adapters/provider", "adapter.mjs", "internal/shared/SPEC.md")
		writeCoverageFile(t, root, "agm/test/bdd/features/shared_owner.feature", "# RELATED-SPEC: adapters/provider/SPEC.owner\nFeature: Shared owner\n")
		findings, err := ValidateAllImplementationSpecs(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Kind != FindingKindFeatureMissingSpecRef || !strings.Contains(findings[0].Message, "internal/shared/SPEC.md") {
			t.Fatalf("noncanonical feature backlink findings = %v", findings)
		}
	})
}

func TestValidateAllImplementationSpecsRejectsMissingAndAmbiguousSharedOwners(t *testing.T) {
	t.Parallel()
	t.Run("missing target", func(t *testing.T) {
		root := t.TempDir()
		writeCoverageFile(t, root, "adapters/provider/adapter.mjs", "export default {};\n")
		writeCoverageFile(t, root, "adapters/provider/SPEC.owner", "internal/missing/SPEC.md\n")
		findings, err := ValidateAllImplementationSpecs(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Kind != FindingKindSpecRead || findings[0].Path != "internal/missing/SPEC.md" {
			t.Fatalf("missing owner target findings = %v", findings)
		}
	})

	t.Run("co-located and shared", func(t *testing.T) {
		root := t.TempDir()
		writeRepositoryPackageCoverage(t, root, "adapters/provider", "adapter.mjs")
		writeCoverageFile(t, root, "adapters/provider/SPEC.owner", "internal/shared/SPEC.md\n")
		findings, err := ValidateAllImplementationSpecs(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(findings) != 1 || findings[0].Kind != FindingKindInvalidSpecOwner || !strings.Contains(findings[0].Message, "both") {
			t.Fatalf("ambiguous owner findings = %v", findings)
		}
	})
}

func TestValidateAllRepositorySpecsIncludesDocOnlyContracts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "docs/policy/SPEC.md", "# Policy\n\n## EARS Requirements\n\n**POL-01** When policy is checked, the system shall require BDD traceability.\n")

	findings, err := ValidateAllRepositorySpecs(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Path != "docs/policy/SPEC.md" || !strings.Contains(findings[0].Message, "executable BDD feature") {
		t.Fatalf("expected missing doc-only BDD finding, got %v", findings)
	}
}

func TestBDDFeaturePathsIgnoresNestedFeaturesForbiddenBySuitePolicy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeCoverageFile(t, root, "agm/test/bdd/features/top.feature", "Feature: Top\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/nested/child.feature", "Feature: Child\n")
	writeCoverageFile(t, root, "agm/test/bdd/features/nested/ignored.md", "not executable\n")

	paths, err := bddFeaturePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agm/test/bdd/features/top.feature",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("BDD feature paths = %#v, want %#v", paths, want)
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

func writeRepositoryPackageCoverage(t *testing.T, root, dir, goFile string) {
	t.Helper()
	specPath := filepath.ToSlash(filepath.Join(dir, "SPEC.md"))
	featureName := strings.ReplaceAll(filepath.ToSlash(dir), "/", "_") + ".feature"
	featurePath := "agm/test/bdd/features/" + featureName
	writeCoverageFile(t, root, filepath.ToSlash(filepath.Join(dir, goFile)), "package example\n")
	writeCoverageFile(t, root, specPath, "# Example\n\n## EARS Requirements\n\n**EX-01** When coverage is checked, the system shall retain executable BDD traceability.\n\n- Feature: `"+featurePath+"`\n")
	writeCoverageFile(t, root, featurePath, "# RELATED-SPEC: "+specPath+"\nFeature: Example\n")
}

func writeRepositorySharedCoverage(t *testing.T, root, dir, sourceFile, specPath string) {
	t.Helper()
	featurePath := "agm/test/bdd/features/shared_owner.feature"
	writeCoverageFile(t, root, filepath.ToSlash(filepath.Join(dir, sourceFile)), "package example\n")
	writeCoverageFile(t, root, filepath.ToSlash(filepath.Join(dir, "SPEC.owner")), specPath+"\n")
	writeCoverageFile(t, root, specPath, validRepositoryCoverageSpec(featurePath))
	writeCoverageFile(t, root, featurePath, "# RELATED-SPEC: "+specPath+"\nFeature: Shared owner\n")
}

func validRepositoryCoverageSpec(featurePath string) string {
	return "# Shared\n\n## EARS Requirements\n\n**SHARED-01** When an adapter reuses shared behavior, the system shall retain one canonical contract owner.\n\n- Feature: `" + featurePath + "`\n"
}
