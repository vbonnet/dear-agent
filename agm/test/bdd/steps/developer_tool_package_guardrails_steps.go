package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

const developerToolFeaturePath = "agm/test/bdd/features/developer_tool_package_guardrails.feature"

type developerToolPackageStateKey struct{}
type developerToolParityStateKey struct{}

type developerToolParityState struct {
	harness string
	family  string
}

// RegisterDeveloperToolPackageGuardrailSteps registers developer-tool coverage and parity steps.
func RegisterDeveloperToolPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          developerToolPackageStateKey{},
		label:             "developer tool package",
		featurePath:       developerToolFeaturePath,
		configuredPattern: `^developer tool package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates developer tool package coverage$`,
		colocatedPattern:  `^developer tool package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, developerToolParityStateKey{}, &developerToolParityState{}), nil
	})
	ctx.Step(`^developer tooling is invoked by harness "([^"]*)" with model family "([^"]*)"$`, developerToolingIsInvoked)
	ctx.Step(`^AGM validates the developer tooling parity route$`, agmValidatesDeveloperToolingParityRoute)
	ctx.Step(`^the developer tooling contract should remain provider neutral$`, developerToolingContractRemainsProviderNeutral)
	ctx.Step(`^the Markdown link integrity checker is configured$`, markdownLinkIntegrityCheckerIsConfigured)
	ctx.Step(`^AGM validates the Markdown link integrity route$`, agmValidatesMarkdownLinkIntegrityRoute)
	ctx.Step(`^fenced code, tracked hidden files, anchors, and baseline debt should be governed$`, markdownLinkIntegrityContractIsGoverned)
}

func markdownLinkIntegrityCheckerIsConfigured() error {
	root := packageSpecBDDRepoRoot()
	for _, rel := range []string{"tools/dead-links/checker.go", "tools/dead-links/SPEC.md", ".dead-links-baseline.txt"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("markdown link integrity file %s: %w", rel, err)
		}
	}
	return nil
}

func agmValidatesMarkdownLinkIntegrityRoute() error {
	root := packageSpecBDDRepoRoot()
	workflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/health-check.yml"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(workflow), "--baseline .dead-links-baseline.txt") {
		return fmt.Errorf("health check does not enforce the dead-link baseline")
	}
	return nil
}

func markdownLinkIntegrityContractIsGoverned() error {
	root := packageSpecBDDRepoRoot()
	source, err := os.ReadFile(filepath.Join(root, "tools/dead-links/checker.go"))
	if err != nil {
		return err
	}
	text := string(source)
	for _, marker := range []string{"goldmark", "git\", \"-C\"", "explicitAnchorRe", "applyBaseline"} {
		if !strings.Contains(text, marker) {
			return fmt.Errorf("markdown link integrity implementation missing %q", marker)
		}
	}
	return nil
}

func developerToolingIsInvoked(ctx context.Context, harness, family string) error {
	state, err := getDeveloperToolParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = harness
	state.family = family
	return nil
}

func agmValidatesDeveloperToolingParityRoute(ctx context.Context) error {
	state, err := getDeveloperToolParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(agent.ActiveHarnesses(), state.harness) {
		return fmt.Errorf("developer tooling harness %q is not active", state.harness)
	}
	if !agent.IsSupportedModelFamily(state.family) {
		return fmt.Errorf("developer tooling model family %q is not supported", state.family)
	}
	return nil
}

func developerToolingContractRemainsProviderNeutral(ctx context.Context) error {
	state, err := getDeveloperToolParityState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("developer tooling parity route is incomplete")
	}
	return nil
}

func getDeveloperToolParityState(ctx context.Context) (*developerToolParityState, error) {
	state, ok := ctx.Value(developerToolParityStateKey{}).(*developerToolParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("developer tooling parity state not initialized")
	}
	return state, nil
}
