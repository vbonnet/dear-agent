// Package speccoverage validates parity-critical SPEC.md and BDD feature
// coverage for dear-agent governance surfaces.
package speccoverage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
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

	return findings, nil
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
func ValidateChangedGoPackageSpecs(root, baseRef string) ([]Finding, error) {
	files, err := ChangedGoFiles(root, baseRef)
	if err != nil {
		return nil, err
	}
	return ValidateGoPackageSpecsForFiles(root, files), nil
}

// ChangedGoFiles returns changed Go files relative to baseRef...HEAD.
func ChangedGoFiles(root, baseRef string) ([]string, error) {
	if baseRef == "" {
		baseRef = "origin/main"
	}
	cmd := exec.Command("git", "-C", root, "diff", "--name-only", "--diff-filter=ACMR", baseRef+"...HEAD", "--", "*.go")
	out, err := cmd.CombinedOutput()
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
		}
	}
	return findings
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
		} else if !strings.Contains(string(spec), "## EARS Requirements") {
			findings = append(findings, Finding{
				Surface: surface.Name,
				Path:    surface.SpecPath,
				Message: "SPEC.md does not declare EARS requirements",
			})
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
		} else if !strings.Contains(string(feature), "Feature:") {
			findings = append(findings, Finding{
				Surface: surface.Name,
				Path:    surface.FeaturePath,
				Message: "BDD feature does not declare a Feature",
			})
		}
	}

	return findings
}
