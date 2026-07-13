package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

const qualityCommandFeaturePath = "agm/test/bdd/features/quality_command_guardrails.feature"

type qualityCommandGuardrailStateKey struct{}

// RegisterQualityCommandGuardrailSteps registers BDD steps for repo quality command packages.
func RegisterQualityCommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:           qualityCommandGuardrailStateKey{},
		label:              "quality command package",
		featurePath:        qualityCommandFeaturePath,
		configuredPattern:  `^quality command package "([^"]*)" is configured$`,
		validatePattern:    `^AGM validates quality command package coverage$`,
		colocatedPattern:   `^quality command package "([^"]*)" should have a co-located SPEC$`,
		requirementPattern: `^quality command package SPEC should declare requirement "([^"]*)" containing "([^"]*)"$`,
	})
	ctx.Step(`^repo health measures executable BDD discovery$`, repoHealthMeasuresExecutableBDDDiscovery)
	ctx.Step(`^repo health should follow the tag-free BDD enforcement policy$`, repoHealthShouldFollowTagFreeBDDPolicy)
	ctx.Step(`^repo health measures implementation source coverage$`, repoHealthMeasuresImplementationSourceCoverage)
	ctx.Step(`^repo health should recognize canonical Dockerfile and Makefile names$`, repoHealthShouldRecognizeCanonicalBuildFiles)
}

func repoHealthMeasuresExecutableBDDDiscovery() error {
	return nil
}

func repoHealthShouldFollowTagFreeBDDPolicy() error {
	root := packageSpecBDDRepoRoot()
	checks := map[string][]string{
		"cmd/repo-health/agenthealth.go": {"canonical directory", "ADR-027 retired @implemented tags", "isHealthImplementationSource"},
		"cmd/repo-health/evaluate.go":    {"features are in the canonical executable suite"},
		"cmd/repo-health/render.go":      {"Executable BDD features"},
		"cmd/repo-health/types.go":       {"features_executable", "implementation_dirs_with_spec"},
	}
	for rel, markers := range checks {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		for _, marker := range markers {
			if !strings.Contains(string(data), marker) {
				return fmt.Errorf("%s does not enforce %q", rel, marker)
			}
		}
	}
	return nil
}

func repoHealthMeasuresImplementationSourceCoverage() error {
	return nil
}

func repoHealthShouldRecognizeCanonicalBuildFiles() error {
	root := packageSpecBDDRepoRoot()
	checks := map[string][]string{
		"cmd/repo-health/agenthealth.go":      {`"dockerfile"`, `"makefile"`},
		"cmd/repo-health/agenthealth_test.go": {"containers/image/Dockerfile", "automation/Makefile"},
	}
	for rel, markers := range checks {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		for _, marker := range markers {
			if !strings.Contains(string(data), marker) {
				return fmt.Errorf("%s does not enforce %q", rel, marker)
			}
		}
	}
	return nil
}
