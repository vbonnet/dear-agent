package speccoverage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
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
