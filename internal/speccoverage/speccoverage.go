// Package speccoverage validates parity-critical SPEC.md and BDD feature
// coverage for dear-agent governance surfaces.
package speccoverage

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/vbonnet/dear-agent/internal/earslint"
	"github.com/vbonnet/dear-agent/internal/repoinventory"
	"github.com/vbonnet/dear-agent/internal/specguard"
)

// Surface maps one parity-critical implementation area to its executable BDD
// feature and governing SPEC.md.
type Surface struct {
	Name        string
	PackagePath string
	SpecPath    string
	FeaturePath string
}

// FindingKind identifies a stable SPEC/BDD coverage violation category.
// Callers must classify findings by Kind rather than parsing human-readable
// paths or messages.
type FindingKind string

// Finding kinds are stable policy categories consumed by BDD enforcement.
const (
	FindingKindUnregisteredParityFeature FindingKind = "unregistered-parity-feature"
	FindingKindCatalogRead               FindingKind = "catalog-read"
	FindingKindFeatureDiscovery          FindingKind = "feature-discovery"
	FindingKindCatalogUnlistedFeature    FindingKind = "catalog-unlisted-feature"
	FindingKindCatalogMissingFeature     FindingKind = "catalog-missing-feature"
	FindingKindFeatureRead               FindingKind = "feature-read"
	FindingKindFeatureMissingSpec        FindingKind = "feature-missing-spec"
	FindingKindFeatureMissingSpecRef     FindingKind = "feature-missing-spec-reference"
	FindingKindSpecMissingFeatureRef     FindingKind = "spec-missing-feature-reference"
	FindingKindMissingCoLocatedSpec      FindingKind = "missing-co-located-spec"
	FindingKindSpecMissingBDDRef         FindingKind = "spec-missing-bdd-reference"
	FindingKindMissingReferencedBDD      FindingKind = "missing-referenced-bdd"
	FindingKindFeatureMissingDeclaration FindingKind = "feature-missing-declaration"
	FindingKindMissingPackagePath        FindingKind = "missing-package-path"
	FindingKindMissingSpecPath           FindingKind = "missing-spec-path"
	FindingKindMissingFeaturePath        FindingKind = "missing-feature-path"
	FindingKindInvalidSpecOwner          FindingKind = "invalid-spec-owner"
	FindingKindSpecRead                  FindingKind = "spec-read"
	FindingKindMissingEARS               FindingKind = "missing-ears-requirements"
	FindingKindMissingAudit              FindingKind = "missing-audit-marker"
	FindingKindInvalidEARS               FindingKind = "invalid-ears"
)

// Finding describes a SPEC/BDD coverage violation.
type Finding struct {
	Kind    FindingKind
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
		{
			Name:        "context management parity",
			PackagePath: "pkg/context",
			SpecPath:    "pkg/context/SPEC.md",
			FeaturePath: "agm/test/bdd/features/context_management_parity.feature",
		},
		{
			Name:        "agent utility parity",
			PackagePath: "pkg/promptcache",
			SpecPath:    "pkg/promptcache/SPEC.md",
			FeaturePath: "agm/test/bdd/features/agent_utility_parity.feature",
		},
		{
			Name:        "validation and workspace parity",
			PackagePath: "pkg/security",
			SpecPath:    "pkg/security/SPEC.md",
			FeaturePath: "agm/test/bdd/features/validation_workspace_parity.feature",
		},
		{
			Name:        "evaluation and control parity",
			PackagePath: "pkg/benchmarks",
			SpecPath:    "pkg/benchmarks/SPEC.md",
			FeaturePath: "agm/test/bdd/features/evaluation_control_parity.feature",
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
			Kind:    FindingKindUnregisteredParityFeature,
			Surface: "parity feature registry",
			Path:    feature,
			Message: "parity feature is not registered in speccoverage.ParitySurfaces",
		})
	}

	findings = append(findings, ValidateBDDCatalog(root)...)
	findings = append(findings, ValidateBDDFeatureTraceability(root)...)

	return findings, nil
}

// ValidateBDDCatalog verifies the BDD catalog covers every executable feature
// and does not list feature files that no longer exist.
func ValidateBDDCatalog(root string) []Finding {
	catalogPath := filepath.Join(root, "agm", "docs", "BDD-CATALOG.md")
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		return []Finding{{
			Kind:    FindingKindCatalogRead,
			Surface: "BDD catalog",
			Path:    "agm/docs/BDD-CATALOG.md",
			Message: fmt.Sprintf("read BDD catalog: %v", err),
		}}
	}

	features, err := bddFeaturePaths(root)
	if err != nil {
		return []Finding{{
			Kind:    FindingKindFeatureDiscovery,
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
				Kind:    FindingKindCatalogUnlistedFeature,
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
				Kind:    FindingKindCatalogMissingFeature,
				Surface: "BDD catalog",
				Path:    feature,
				Message: "BDD catalog references a missing feature file",
			})
		}
	}

	return findings
}

// ValidateBDDFeatureTraceability verifies every executable BDD feature declares
// a governing SPEC.md and that the referenced SPEC points back to the feature.
func ValidateBDDFeatureTraceability(root string) []Finding {
	features, err := bddFeaturePaths(root)
	if err != nil {
		return []Finding{{
			Kind:    FindingKindFeatureDiscovery,
			Surface: "BDD feature traceability",
			Path:    "agm/test/bdd/features",
			Message: err.Error(),
		}}
	}

	var findings []Finding
	for _, feature := range features {
		featureData, err := os.ReadFile(filepath.Join(root, feature))
		if err != nil {
			findings = append(findings, Finding{
				Kind:    FindingKindFeatureRead,
				Surface: "BDD feature traceability",
				Path:    feature,
				Message: fmt.Sprintf("read BDD feature: %v", err),
			})
			continue
		}
		specPaths, ok := featureSpecPaths(string(featureData))
		if !ok {
			findings = append(findings, Finding{
				Kind:    FindingKindFeatureMissingSpecRef,
				Surface: "BDD feature traceability",
				Path:    feature,
				Message: "BDD feature does not declare governing SPEC.md",
			})
			continue
		}
		for _, specPath := range specPaths {
			specData, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(specPath)))
			if err != nil {
				findings = append(findings, Finding{
					Kind:    FindingKindFeatureMissingSpec,
					Surface: "BDD feature traceability",
					Path:    specPath,
					Message: fmt.Sprintf("BDD feature references a missing SPEC.md: %v", err),
				})
				continue
			}
			if !strings.Contains(string(specData), feature) {
				findings = append(findings, Finding{
					Kind:    FindingKindSpecMissingFeatureRef,
					Surface: "BDD feature traceability",
					Path:    specPath,
					Message: fmt.Sprintf("governing or related SPEC.md does not reference executable BDD feature: %s", feature),
				})
			}
		}
	}
	return findings
}

func featureSpecPath(featureText string) (string, bool) {
	paths, ok := featureSpecPaths(featureText)
	if !ok {
		return "", false
	}
	return paths[0], true
}

func featureSpecPaths(featureText string) ([]string, bool) {
	var paths []string
	hasPrimary := false
	for line := range strings.SplitSeq(featureText, "\n") {
		line = strings.TrimSpace(line)
		value, primary := strings.CutPrefix(line, "# SPEC:")
		if !primary {
			var related bool
			value, related = strings.CutPrefix(line, "# RELATED-SPEC:")
			if !related {
				continue
			}
		}
		value = strings.TrimSpace(value)
		if value == "" || filepath.Base(value) != "SPEC.md" {
			return nil, false
		}
		if primary {
			if hasPrimary {
				return nil, false
			}
			hasPrimary = true
			paths = append([]string{filepath.ToSlash(filepath.Clean(value))}, paths...)
			continue
		}
		paths = append(paths, filepath.ToSlash(filepath.Clean(value)))
	}
	return paths, hasPrimary
}

func bddFeaturePaths(root string) ([]string, error) {
	featuresDir := filepath.Join(root, "agm", "test", "bdd", "features")
	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return nil, fmt.Errorf("read BDD features directory: %w", err)
	}

	var features []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		feature := filepath.ToSlash(filepath.Join(repoinventory.ExecutableBDDFeatureRoot, entry.Name()))
		if repoinventory.IsExecutableBDDFeaturePath(feature) {
			features = append(features, feature)
		}
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

// ValidateChangedGoPackageSpecs provides a fast, diff-scoped diagnostic for
// changed production Go packages. ValidateAllImplementationSpecs remains the
// authoritative repository-wide gate.
func ValidateChangedGoPackageSpecs(ctx context.Context, root, baseRef string) ([]Finding, error) {
	files, err := ChangedGoFiles(ctx, root, baseRef)
	if err != nil {
		return nil, err
	}
	return ValidateGoPackageSpecsForFiles(root, files), nil
}

var specBDDFeaturePattern = regexp.MustCompile(`agm/test/bdd/features/[A-Za-z0-9._-]+\.feature`)

const sharedSpecOwnerName = "SPEC.owner"

// ValidateAllGoPackageSpecs requires every Go package, including test-only
// packages, to have strict SPEC.md and reciprocal executable BDD coverage.
func ValidateAllGoPackageSpecs(root string) ([]Finding, error) {
	inventory, err := scanCoverageInventory(root)
	if err != nil {
		return nil, err
	}
	packageDirs, err := inventory.dirs(goSourceFile)
	if err != nil {
		return nil, err
	}
	return validateAllImplementationDirs(root, packageDirs, inventory.byPath)
}

// ValidateAllImplementationSpecs requires every implementation directory,
// regardless of source language, to have strict owned SPEC and BDD coverage.
// A directory may point at one canonical shared contract with SPEC.owner when
// a co-located SPEC.md would duplicate behavior owned elsewhere.
func ValidateAllImplementationSpecs(root string) ([]Finding, error) {
	inventory, err := scanCoverageInventory(root)
	if err != nil {
		return nil, err
	}
	implementationDirs, err := inventory.dirs(implementationSourceFile)
	if err != nil {
		return nil, err
	}
	return validateAllImplementationDirs(root, implementationDirs, inventory.byPath)
}

// ValidateAllRepositorySpecs requires every SPEC.md artifact, including specs
// in doc-only and hidden policy directories, to retain strict EARS and
// reciprocal executable BDD traceability.
func ValidateAllRepositorySpecs(root string) ([]Finding, error) {
	inventory, err := scanCoverageInventory(root)
	if err != nil {
		return nil, err
	}
	specDirs, err := inventory.dirs(func(file repoinventory.File) (bool, error) {
		return file.Name() == "SPEC.md", nil
	})
	if err != nil {
		return nil, err
	}
	findings, err := validateAllImplementationDirs(root, specDirs, inventory.byPath)
	if err != nil {
		return nil, err
	}
	ownerDirs, err := inventory.dirs(func(file repoinventory.File) (bool, error) {
		return file.Name() == sharedSpecOwnerName, nil
	})
	if err != nil {
		return nil, err
	}
	implementationDirs, err := inventory.dirs(implementationSourceFile)
	if err != nil {
		return nil, err
	}
	implementationSet := make(map[string]bool, len(implementationDirs))
	for _, dir := range implementationDirs {
		implementationSet[dir] = true
	}
	specSet := make(map[string]bool, len(specDirs))
	for _, dir := range specDirs {
		specSet[dir] = true
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open repository root: %w", err)
	}
	defer func() { _ = rootFS.Close() }()
	for _, dir := range ownerDirs {
		ownerPath := filepath.ToSlash(filepath.Join(dir, sharedSpecOwnerName))
		if !implementationSet[dir] {
			findings = append(findings, Finding{
				Kind: FindingKindInvalidSpecOwner, Surface: "repository SPEC owner coverage", Path: ownerPath,
				Message: "SPEC.owner does not belong to a discovered implementation directory",
			})
			continue
		}
		if specSet[dir] {
			continue
		}
		findings = append(findings, validateRepositoryImplementationSpec(rootFS, inventory.byPath, dir)...)
	}
	return findings, nil
}

func validateAllImplementationDirs(root string, dirs []string, files map[string]repoinventory.File) ([]Finding, error) {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open repository root: %w", err)
	}
	defer func() { _ = rootFS.Close() }()

	var findings []Finding
	for _, dir := range dirs {
		findings = append(findings, validateRepositoryImplementationSpec(rootFS, files, dir)...)
	}
	return findings, nil
}

type coverageInventory struct {
	files  []repoinventory.File
	byPath map[string]repoinventory.File
}

func scanCoverageInventory(root string) (coverageInventory, error) {
	files, err := repoinventory.Scan(root)
	if err != nil {
		return coverageInventory{}, fmt.Errorf("discover repository implementation files: %w", err)
	}
	byPath := make(map[string]repoinventory.File, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	return coverageInventory{files: files, byPath: byPath}, nil
}

func (inventory coverageInventory) dirs(isSource func(repoinventory.File) (bool, error)) ([]string, error) {
	seen := map[string]bool{}
	for _, file := range inventory.files {
		include, err := isSource(file)
		if err != nil {
			return nil, err
		}
		if include {
			seen[filepath.ToSlash(filepath.Dir(file.Path))] = true
		}
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	slices.Sort(dirs)
	return dirs, nil
}

func goSourceFile(file repoinventory.File) (bool, error) {
	return strings.HasSuffix(file.Name(), ".go"), nil
}

func implementationSourceFile(file repoinventory.File) (bool, error) {
	return repoinventory.IsImplementationSource(file.Path, file.Mode.IsRegular(), file.Executable()), nil
}

func validateRepositoryImplementationSpec(root *os.Root, files map[string]repoinventory.File, dir string) []Finding {
	resolved, findings := resolveImplementationSpec(root, files, dir, "repository implementation SPEC coverage")
	if len(findings) > 0 {
		return findings
	}
	specPath := resolved.path
	specData := resolved.data

	specText := string(specData)
	findings = validateSpecEARS(Surface{
		Name:     "repository implementation SPEC coverage",
		SpecPath: specPath,
	}, specText)

	featurePaths := specBDDFeaturePattern.FindAllString(specText, -1)
	slices.Sort(featurePaths)
	featurePaths = slices.Compact(featurePaths)
	if len(featurePaths) == 0 {
		return append(findings, Finding{
			Kind:    FindingKindSpecMissingBDDRef,
			Surface: "repository implementation BDD coverage",
			Path:    specPath,
			Message: "SPEC.md does not reference an executable BDD feature",
		})
	}

	for _, featurePath := range featurePaths {
		featureData, err := readGovernedInventoryFile(root, files, featurePath, 0)
		if err != nil {
			findings = append(findings, Finding{
				Kind:    FindingKindMissingReferencedBDD,
				Surface: "repository implementation BDD coverage",
				Path:    featurePath,
				Message: fmt.Sprintf("SPEC.md references a missing executable BDD feature: %v", err),
			})
			continue
		}
		featureText := string(featureData)
		if !strings.Contains(featureText, "Feature:") {
			findings = append(findings, Finding{
				Kind:    FindingKindFeatureMissingDeclaration,
				Surface: "repository implementation BDD coverage",
				Path:    featurePath,
				Message: "referenced BDD feature does not declare a Feature",
			})
		}
		if !bddFeatureReferencesSpec(featureText, specPath) {
			findings = append(findings, Finding{
				Kind:    FindingKindFeatureMissingSpecRef,
				Surface: "repository implementation BDD coverage",
				Path:    featurePath,
				Message: fmt.Sprintf("referenced BDD feature does not declare reciprocal SPEC traceability to %s", specPath),
			})
		}
	}
	return findings
}

type implementationSpec struct {
	path string
	data []byte
}

func resolveImplementationSpec(root *os.Root, files map[string]repoinventory.File, dir, surface string) (implementationSpec, []Finding) {
	localPath := filepath.ToSlash(filepath.Join(dir, "SPEC.md"))
	ownerPath := filepath.ToSlash(filepath.Join(dir, sharedSpecOwnerName))
	_, hasLocal := files[localPath]
	_, hasOwner := files[ownerPath]
	if hasLocal && hasOwner {
		return implementationSpec{}, []Finding{{
			Kind: FindingKindInvalidSpecOwner, Surface: surface, Path: ownerPath,
			Message: "implementation directory declares both a co-located SPEC.md and SPEC.owner; choose exactly one normative owner",
		}}
	}
	if hasLocal {
		localData, err := readGovernedInventoryFile(root, files, localPath, 0)
		if err != nil {
			return implementationSpec{}, []Finding{{
				Kind: FindingKindSpecRead, Surface: surface, Path: localPath,
				Message: fmt.Sprintf("read governed co-located SPEC.md: %v", err),
			}}
		}
		return implementationSpec{path: localPath, data: localData}, nil
	}
	if !hasOwner {
		return implementationSpec{}, []Finding{{
			Kind: FindingKindMissingCoLocatedSpec, Surface: surface, Path: localPath,
			Message: fmt.Sprintf("implementation directory %q has neither a co-located SPEC.md nor an explicit %s", dir, sharedSpecOwnerName),
		}}
	}

	ownerData, err := readGovernedInventoryFile(root, files, ownerPath, specguard.MaxSpecOwnerBytes)
	if err != nil {
		return implementationSpec{}, []Finding{{
			Kind: FindingKindInvalidSpecOwner, Surface: surface, Path: ownerPath,
			Message: fmt.Sprintf("read governed %s: %v", sharedSpecOwnerName, err),
		}}
	}
	target, err := specguard.ParseSpecOwner(ownerData, localPath)
	if err != nil {
		return implementationSpec{}, []Finding{{
			Kind: FindingKindInvalidSpecOwner, Surface: surface, Path: ownerPath,
			Message: err.Error(),
		}}
	}
	targetData, err := readGovernedInventoryFile(root, files, target, 0)
	if err != nil {
		return implementationSpec{}, []Finding{{
			Kind: FindingKindSpecRead, Surface: surface, Path: target,
			Message: fmt.Sprintf("read canonical shared SPEC.md declared by %s: %v", ownerPath, err),
		}}
	}
	if chainedTarget, chainErr := specguard.ParseSpecOwner(targetData, target); chainErr == nil {
		return implementationSpec{}, []Finding{{
			Kind: FindingKindInvalidSpecOwner, Surface: surface, Path: ownerPath,
			Message: fmt.Sprintf("SPEC.owner target %q is another ownership pointer to %q", target, chainedTarget),
		}}
	}
	return implementationSpec{path: target, data: targetData}, nil
}

func readGovernedInventoryFile(root *os.Root, files map[string]repoinventory.File, filePath string, limit int64) ([]byte, error) {
	file, ok := files[filePath]
	if !ok {
		return nil, fmt.Errorf("path is absent from the governed repository inventory")
	}
	if !file.Mode.IsRegular() || file.Mode&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("governed path is not a regular non-symlink file")
	}
	handle, err := root.Open(filepath.FromSlash(filePath))
	if err != nil {
		return nil, err
	}
	defer func() { _ = handle.Close() }()
	if limit <= 0 {
		return io.ReadAll(handle)
	}
	data, err := io.ReadAll(io.LimitReader(handle, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("governed file exceeds the %d-byte limit", limit)
	}
	return data, nil
}

func bddFeatureReferencesSpec(featureText, specPath string) bool {
	for line := range strings.SplitSeq(featureText, "\n") {
		line = strings.TrimSpace(line)
		for _, marker := range []string{"# SPEC:", "# RELATED-SPEC:"} {
			value, ok := strings.CutPrefix(line, marker)
			if ok && filepath.ToSlash(filepath.Clean(strings.TrimSpace(value))) == specPath {
				return true
			}
		}
	}
	return false
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
	inventory, err := scanCoverageInventory(root)
	if err != nil {
		return []Finding{{
			Kind: FindingKindSpecRead, Surface: "changed Go package SPEC coverage",
			Message: err.Error(),
		}}
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return []Finding{{
			Kind: FindingKindSpecRead, Surface: "changed Go package SPEC coverage",
			Message: fmt.Sprintf("open repository root: %v", err),
		}}
	}
	defer func() { _ = rootFS.Close() }()

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
		resolved, resolutionFindings := resolveImplementationSpec(rootFS, inventory.byPath, dir, "changed Go package SPEC coverage")
		if len(resolutionFindings) > 0 {
			findings = append(findings, resolutionFindings...)
			continue
		}
		findings = append(findings, validateSpecEARS(Surface{
			Name:     "changed Go package SPEC coverage",
			SpecPath: resolved.path,
		}, string(resolved.data))...)
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
	var specText string
	var featureText string
	specLoaded := false
	featureLoaded := false

	if surface.PackagePath == "" {
		findings = append(findings, Finding{
			Kind:    FindingKindMissingPackagePath,
			Surface: surface.Name,
			Message: "package path is empty",
		})
	}
	if surface.SpecPath == "" {
		findings = append(findings, Finding{
			Kind:    FindingKindMissingSpecPath,
			Surface: surface.Name,
			Message: "SPEC.md path is empty",
		})
	}
	if surface.FeaturePath == "" {
		findings = append(findings, Finding{
			Kind:    FindingKindMissingFeaturePath,
			Surface: surface.Name,
			Message: "BDD feature path is empty",
		})
	}

	if surface.SpecPath != "" {
		spec, err := os.ReadFile(filepath.Join(root, surface.SpecPath))
		if err != nil {
			findings = append(findings, Finding{
				Kind:    FindingKindSpecRead,
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
					Kind:    FindingKindMissingEARS,
					Surface: surface.Name,
					Path:    surface.SpecPath,
					Message: "SPEC.md does not declare EARS requirements",
				})
			}
			if !hasCompletedAuditMarker(specText) {
				findings = append(findings, Finding{
					Kind:    FindingKindMissingAudit,
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
				Kind:    FindingKindFeatureRead,
				Surface: surface.Name,
				Path:    surface.FeaturePath,
				Message: fmt.Sprintf("read BDD feature: %v", err),
			})
		} else {
			featureText = string(feature)
			featureLoaded = true
			if !strings.Contains(featureText, "Feature:") {
				findings = append(findings, Finding{
					Kind:    FindingKindFeatureMissingDeclaration,
					Surface: surface.Name,
					Path:    surface.FeaturePath,
					Message: "BDD feature does not declare a Feature",
				})
			}
		}
	}

	if specLoaded && surface.FeaturePath != "" && !strings.Contains(specText, surface.FeaturePath) {
		findings = append(findings, Finding{
			Kind:    FindingKindSpecMissingBDDRef,
			Surface: surface.Name,
			Path:    surface.SpecPath,
			Message: "SPEC.md does not reference its executable BDD feature",
		})
	}
	if featureLoaded && surface.SpecPath != "" && !strings.Contains(featureText, surface.SpecPath) {
		findings = append(findings, Finding{
			Kind:    FindingKindFeatureMissingSpecRef,
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
			Kind:    FindingKindInvalidEARS,
			Surface: surface.Name,
			Path:    surface.SpecPath,
			Message: fmt.Sprintf("initialize EARS linter: %v", err),
		}}
	}

	result, err := linter.Lint(surface.SpecPath, strings.NewReader(spec))
	if err != nil {
		return []Finding{{
			Kind:    FindingKindInvalidEARS,
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
			Kind:    FindingKindInvalidEARS,
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
