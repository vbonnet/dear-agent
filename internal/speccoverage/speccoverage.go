// Package speccoverage validates parity-critical SPEC.md and BDD feature
// coverage for dear-agent governance surfaces.
package speccoverage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vbonnet/dear-agent/internal/earslint"
)

// Surface maps one parity-critical implementation area to its executable BDD
// feature and governing SPEC.md.
type Surface struct {
	Name        string
	PackagePath string
	SpecPath    string
	FeaturePath string
}

// Finding describes a SPEC/BDD coverage violation.
type Finding struct {
	Surface string
	Path    string
	Message string
}

func (f Finding) Error() string {
	if f.Path == "" {
		return fmt.Sprintf("%s: %s", f.Surface, f.Message)
	}
	return fmt.Sprintf("%s: %s: %s", f.Surface, f.Path, f.Message)
}

// ParitySurfaces returns the governed parity matrix. Additions to
// agm/test/bdd/features/*_parity.feature must be represented here.
func ParitySurfaces() []Surface {
	return []Surface{
		{
			Name:        "harness and model parity",
			PackagePath: "agm/internal/agent",
			SpecPath:    "agm/internal/agent/SPEC.md",
			FeaturePath: "agm/test/bdd/features/harness_parity.feature",
		},
		{
			Name:        "instruction parity",
			PackagePath: "internal/instructions",
			SpecPath:    "internal/instructions/SPEC.md",
			FeaturePath: "agm/test/bdd/features/instruction_parity.feature",
		},
		{
			Name:        "hook parity",
			PackagePath: "internal/hookparity",
			SpecPath:    "internal/hookparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/hook_parity.feature",
		},
		{
			Name:        "permission parity",
			PackagePath: "agm/internal/permissionparity",
			SpecPath:    "agm/internal/permissionparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/permission_parity.feature",
		},
		{
			Name:        "quota parity",
			PackagePath: "agm/internal/quotaparity",
			SpecPath:    "agm/internal/quotaparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/quota_parity.feature",
		},
		{
			Name:        "MCP parity",
			PackagePath: "agm/internal/mcpparity",
			SpecPath:    "agm/internal/mcpparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/mcp_parity.feature",
		},
		{
			Name:        "marketplace parity",
			PackagePath: "agm/internal/marketplaceparity",
			SpecPath:    "agm/internal/marketplaceparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/marketplace_parity.feature",
		},
		{
			Name:        "Engram parity",
			PackagePath: "agm/internal/engramparity",
			SpecPath:    "agm/internal/engramparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/engram_parity.feature",
		},
		{
			Name:        "Wayfinder parity",
			PackagePath: "agm/internal/wayfinderparity",
			SpecPath:    "agm/internal/wayfinderparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/wayfinder_parity.feature",
		},
		{
			Name:        "configuration directory parity",
			PackagePath: "agm/internal/configdirparity",
			SpecPath:    "agm/internal/configdirparity/SPEC.md",
			FeaturePath: "agm/test/bdd/features/config_directory_parity.feature",
		},
		{
			Name:        "model family provider parity",
			PackagePath: "pkg/llm/provider",
			SpecPath:    "pkg/llm/provider/SPEC.md",
			FeaturePath: "agm/test/bdd/features/model_family_parity.feature",
		},
	}
}

// Validate checks that every registered parity surface has both SPEC and BDD
// coverage, and that every parity feature is registered.
func Validate(root string) ([]Finding, error) {
	var findings []Finding
	surfaces := ParitySurfaces()

	for _, surface := range surfaces {
		findings = append(findings, validateSurface(root, surface)...)
	}

	unregistered, err := UnregisteredParityFeatures(root)
	if err != nil {
		return nil, err
	}
	for _, feature := range unregistered {
		findings = append(findings, Finding{
			Surface: "parity feature registry",
			Path:    feature,
			Message: "parity feature is not registered in speccoverage.ParitySurfaces",
		})
	}

	findings = append(findings, ValidateBDDCatalog(root)...)

	return findings, nil
}

// ValidateBDDCatalog verifies the BDD catalog covers every executable feature
// and does not list feature files that no longer exist.
func ValidateBDDCatalog(root string) []Finding {
	catalogPath := filepath.Join(root, "agm", "docs", "BDD-CATALOG.md")
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		return []Finding{{
			Surface: "BDD catalog",
			Path:    "agm/docs/BDD-CATALOG.md",
			Message: fmt.Sprintf("read BDD catalog: %v", err),
		}}
	}

	features, err := bddFeaturePaths(root)
	if err != nil {
		return []Finding{{
			Surface: "BDD catalog",
			Path:    "agm/test/bdd/features",
			Message: err.Error(),
		}}
	}

	catalogText := string(catalog)
	catalogRefs := catalogFeatureReferences(catalogText)
	catalogSet := map[string]bool{}
	for _, feature := range catalogRefs {
		catalogSet[feature] = true
	}

	var findings []Finding
	for _, feature := range features {
		if !catalogSet[feature] {
			findings = append(findings, Finding{
				Surface: "BDD catalog",
				Path:    feature,
				Message: "BDD feature is not listed in agm/docs/BDD-CATALOG.md",
			})
		}
	}

	featureSet := map[string]bool{}
	for _, feature := range features {
		featureSet[feature] = true
	}
	for _, feature := range catalogRefs {
		if !featureSet[feature] {
			findings = append(findings, Finding{
				Surface: "BDD catalog",
				Path:    feature,
				Message: "BDD catalog references a missing feature file",
			})
		}
	}

	return findings
}

func bddFeaturePaths(root string) ([]string, error) {
	featuresDir := filepath.Join(root, "agm", "test", "bdd", "features")
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return nil, fmt.Errorf("read BDD features directory: %w", err)
	}

	var features []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".feature") {
			continue
		}
		features = append(features, filepath.ToSlash(filepath.Join("agm", "test", "bdd", "features", entry.Name())))
	}
	slices.Sort(features)
	return features, nil
}

func catalogFeatureReferences(catalog string) []string {
	const marker = "../test/bdd/features/"
	seen := map[string]bool{}
	var features []string
	for line := range strings.SplitSeq(catalog, "\n") {
		for {
			_, after, ok := strings.Cut(line, marker)
			if !ok {
				break
			}
			name := after
			if end := strings.IndexAny(name, ")` \t"); end >= 0 {
				name = name[:end]
			}
			line = after
			if !strings.HasSuffix(name, ".feature") || filepath.Base(name) != name {
				continue
			}
			feature := "agm/test/bdd/features/" + name
			if seen[feature] {
				continue
			}
			seen[feature] = true
			features = append(features, feature)
		}
	}
	slices.Sort(features)
	return features
}

// UnregisteredParityFeatures returns every *_parity.feature file that is not
// linked to a governed SPEC surface.
func UnregisteredParityFeatures(root string) ([]string, error) {
	featuresDir := filepath.Join(root, "agm", "test", "bdd", "features")
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return nil, fmt.Errorf("read BDD features directory: %w", err)
	}

	registered := map[string]bool{}
	for _, surface := range ParitySurfaces() {
		registered[filepath.ToSlash(filepath.Clean(surface.FeaturePath))] = true
	}

	var missing []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_parity.feature") {
			continue
		}
		featurePath := filepath.ToSlash(filepath.Join("agm", "test", "bdd", "features", entry.Name()))
		if !registered[featurePath] {
			missing = append(missing, featurePath)
		}
	}
	slices.Sort(missing)
	return missing, nil
}

// ValidateChangedGoPackageSpecs requires changed production Go package
// directories to carry a co-located SPEC.md. It is intentionally diff-based so
// legacy packages can be burned down incrementally without allowing new drift.
func ValidateChangedGoPackageSpecs(ctx context.Context, root, baseRef string) ([]Finding, error) {
	files, err := ChangedGoFiles(ctx, root, baseRef)
	if err != nil {
		return nil, err
	}
	return ValidateGoPackageSpecsForFiles(root, files), nil
}

// ChangedGoFiles returns changed Go files relative to baseRef...HEAD.
func ChangedGoFiles(ctx context.Context, root, baseRef string) ([]string, error) {
	if baseRef == "" {
		baseRef = "origin/main"
	}
	cmd := exec.CommandContext(ctx, "git", "-C", root, "diff", "--name-only", "--diff-filter=ACMR", baseRef+"...HEAD", "--", "*.go")
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("list changed Go files against %s: %w: %s", baseRef, err, strings.TrimSpace(string(out)))
	}

	var files []string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, filepath.ToSlash(filepath.Clean(line)))
	}
	slices.Sort(files)
	return files, nil
}

// ValidateGoPackageSpecsForFiles validates an already-known changed-file set.
func ValidateGoPackageSpecsForFiles(root string, files []string) []Finding {
	seen := map[string]bool{}
	var findings []Finding
	for _, file := range files {
		file = filepath.ToSlash(filepath.Clean(file))
		if !requiresPackageSpec(file) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(file))
		if seen[dir] {
			continue
		}
		seen[dir] = true
		specPath := filepath.Join(root, filepath.FromSlash(dir), "SPEC.md")
		if _, err := os.Stat(specPath); err != nil {
			findings = append(findings, Finding{
				Surface: "changed Go package SPEC coverage",
				Path:    filepath.ToSlash(filepath.Join(dir, "SPEC.md")),
				Message: fmt.Sprintf("changed production Go package %q does not have a co-located SPEC.md", dir),
			})
			continue
		}
		findings = append(findings, validateChangedPackageSpec(root, dir)...)
	}
	return findings
}

func validateChangedPackageSpec(root, dir string) []Finding {
	specPath := filepath.ToSlash(filepath.Join(dir, "SPEC.md"))
	spec, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(specPath)))
	if err != nil {
		return []Finding{{
			Surface: "changed Go package SPEC coverage",
			Path:    specPath,
			Message: fmt.Sprintf("read co-located SPEC.md: %v", err),
		}}
	}

	specText := string(spec)
	if !strings.Contains(specText, "## EARS Requirements") {
		return []Finding{{
			Surface: "changed Go package SPEC coverage",
			Path:    specPath,
			Message: "SPEC.md has invalid EARS syntax: does not declare EARS requirements",
		}}
	}

	return validateSpecEARS(Surface{
		Name:     "changed Go package SPEC coverage",
		SpecPath: specPath,
	}, specText)
}

func requiresPackageSpec(file string) bool {
	if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
		return false
	}
	if strings.HasPrefix(file, "agm/test/") || strings.HasPrefix(file, "tests/") {
		return false
	}
	for part := range strings.SplitSeq(file, "/") {
		switch part {
		case "testdata", "testutil", "integration_test":
			return false
		}
	}
	return true
}

func validateSurface(root string, surface Surface) []Finding {
	var findings []Finding
	var specText string
	var featureText string
	specLoaded := false
	featureLoaded := false

	if surface.PackagePath == "" {
		findings = append(findings, Finding{
			Surface: surface.Name,
			Message: "package path is empty",
		})
	}
	if surface.SpecPath == "" {
		findings = append(findings, Finding{
			Surface: surface.Name,
			Message: "SPEC.md path is empty",
		})
	}
	if surface.FeaturePath == "" {
		findings = append(findings, Finding{
			Surface: surface.Name,
			Message: "BDD feature path is empty",
		})
	}

	if surface.SpecPath != "" {
		spec, err := os.ReadFile(filepath.Join(root, surface.SpecPath))
		if err != nil {
			findings = append(findings, Finding{
				Surface: surface.Name,
				Path:    surface.SpecPath,
				Message: fmt.Sprintf("read SPEC.md: %v", err),
			})
		} else {
			specText = string(spec)
			specLoaded = true
			hasEARS := strings.Contains(specText, "## EARS Requirements")
			if !hasEARS {
				findings = append(findings, Finding{
					Surface: surface.Name,
					Path:    surface.SpecPath,
					Message: "SPEC.md does not declare EARS requirements",
				})
			}
			if !hasCompletedAuditMarker(specText) {
				findings = append(findings, Finding{
					Surface: surface.Name,
					Path:    surface.SpecPath,
					Message: "SPEC.md audit marker is missing or still NEEDS-AUDIT",
				})
			}
			if hasEARS {
				findings = append(findings, validateSpecEARS(surface, specText)...)
			}
		}
	}

	if surface.FeaturePath != "" {
		feature, err := os.ReadFile(filepath.Join(root, surface.FeaturePath))
		if err != nil {
			findings = append(findings, Finding{
				Surface: surface.Name,
				Path:    surface.FeaturePath,
				Message: fmt.Sprintf("read BDD feature: %v", err),
			})
		} else {
			featureText = string(feature)
			featureLoaded = true
			if !strings.Contains(featureText, "Feature:") {
				findings = append(findings, Finding{
					Surface: surface.Name,
					Path:    surface.FeaturePath,
					Message: "BDD feature does not declare a Feature",
				})
			}
		}
	}

	if specLoaded && surface.FeaturePath != "" && !strings.Contains(specText, surface.FeaturePath) {
		findings = append(findings, Finding{
			Surface: surface.Name,
			Path:    surface.SpecPath,
			Message: "SPEC.md does not reference its executable BDD feature",
		})
	}
	if featureLoaded && surface.SpecPath != "" && !strings.Contains(featureText, surface.SpecPath) {
		findings = append(findings, Finding{
			Surface: surface.Name,
			Path:    surface.FeaturePath,
			Message: "BDD feature does not reference its governing SPEC.md",
		})
	}

	return findings
}

func validateSpecEARS(surface Surface, spec string) []Finding {
	linter, err := earslint.New(earslint.Config{})
	if err != nil {
		return []Finding{{
			Surface: surface.Name,
			Path:    surface.SpecPath,
			Message: fmt.Sprintf("initialize EARS linter: %v", err),
		}}
	}

	result, err := linter.Lint(surface.SpecPath, strings.NewReader(spec))
	if err != nil {
		return []Finding{{
			Surface: surface.Name,
			Path:    surface.SpecPath,
			Message: fmt.Sprintf("lint SPEC.md EARS requirements: %v", err),
		}}
	}
	if !result.Failed(true) {
		return nil
	}

	var findings []Finding
	for _, finding := range result.Findings {
		detail := finding.Message
		if finding.Line > 0 {
			detail = fmt.Sprintf("line %d: %s", finding.Line, finding.Message)
		}
		findings = append(findings, Finding{
			Surface: surface.Name,
			Path:    surface.SpecPath,
			Message: "SPEC.md has invalid EARS syntax: " + detail,
		})
	}
	return findings
}

func hasCompletedAuditMarker(spec string) bool {
	for line := range strings.SplitSeq(spec, "\n") {
		line = strings.TrimSpace(line)
		if value, ok := strings.CutPrefix(line, "<!-- Last audited at:"); ok {
			value = strings.TrimSuffix(value, "-->")
			value = strings.TrimSpace(value)
			return value != "" && !strings.EqualFold(value, "NEEDS-AUDIT")
		}
	}
	return false
}
