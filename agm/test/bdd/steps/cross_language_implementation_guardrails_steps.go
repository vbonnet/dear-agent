package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	e2etest "github.com/vbonnet/dear-agent/agm/test/e2e"
	"github.com/vbonnet/dear-agent/internal/earslint"
)

const crossLanguageFeaturePath = "agm/test/bdd/features/cross_language_implementation_guardrails.feature"

var crossLanguageImplementationDirs = []string{
	".",
	".agents/hooks",
	".claude/hooks",
	".deepsec",
	".opencode/hooks",
	"agm/.githooks",
	"agm/agm-plugin/channels/agm-bus/src",
	"agm/cmd/agm-bus/contrib",
	"agm/cmd/agm/hooks",
	"agm/docs/hooks",
	"agm/hooks",
	"agm/hooks/cmd",
	"agm/internal/dolt/migrations",
	"agm/migrations",
	"agm/scripts/hooks",
	"agm/test/e2e/docker/scripts",
	"agm/test/e2e/lib",
	"agm/test/e2e/suites",
	"agm/tests",
	"engram/ecphory/diagrams",
	"engram/hooks-bin",
	"engram/hooks-bin/lib",
	"engram/mcp/src",
	"infra",
	"infra/modules/managed-repo",
	"pkg/workspace/dolt/testdata/migrations",
	"scripts",
	"tests/bats",
	"tools/devlog/diagrams",
	"wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/eslint-flat",
}

type crossLanguageGuardrailStateKey struct{}
type crossLanguageRouteStateKey struct{}

type crossLanguageRouteState struct {
	harness string
	family  string
}

// RegisterCrossLanguageImplementationGuardrailSteps registers cross-language coverage steps.
func RegisterCrossLanguageImplementationGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          crossLanguageGuardrailStateKey{},
		label:             "cross-language implementation directory",
		featurePath:       crossLanguageFeaturePath,
		configuredPattern: `^cross-language implementation directory "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates cross-language implementation coverage$`,
		colocatedPattern:  `^cross-language implementation directory "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, crossLanguageRouteStateKey{}, &crossLanguageRouteState{}), nil
	})
	ctx.Step(`^cross-language coverage runs through "([^"]*)" with "([^"]*)"$`, configureCrossLanguageRoute)
	ctx.Step(`^AGM validates cross-language route parity$`, validateCrossLanguageRoute)
	ctx.Step(`^every cross-language implementation should retain strict SPEC and BDD traceability$`, validateCrossLanguageSpecs)
	ctx.Step(`^the AGM end-to-end harness detection helper is configured$`, agmE2EHarnessDetectionHelperIsConfigured)
	ctx.Step(`^AGM validates portable harness command lookup$`, agmValidatesPortableHarnessCommandLookup)
	ctx.Step(`^the exact harness mapping should run under macOS system Bash$`, exactHarnessMappingRunsUnderSystemBash)
}

func agmE2EHarnessDetectionHelperIsConfigured() error {
	helper := filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "e2e", "lib", "harness-detect.sh")
	_, err := os.Stat(helper)
	return err
}

func agmValidatesPortableHarnessCommandLookup() error {
	helper := filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "e2e", "lib", "harness-detect.sh")
	return e2etest.ValidatePortableHarnessDetection(helper)
}

func exactHarnessMappingRunsUnderSystemBash() error {
	return agmValidatesPortableHarnessCommandLookup()
}

func configureCrossLanguageRoute(ctx context.Context, harness, family string) error {
	state, err := getCrossLanguageRouteState(ctx)
	if err != nil {
		return err
	}
	if _, ok := map[string]struct{}{"claude-code": {}, "codex-cli": {}, "agy": {}, "opencode-cli": {}, "pi-cli": {}}[harness]; !ok {
		return fmt.Errorf("unsupported active harness %q", harness)
	}
	if _, ok := map[string]struct{}{"anthropic": {}, "openai": {}, "gemini": {}, "glm": {}, "deepseek": {}, "nemotron": {}, "qwen": {}}[family]; !ok {
		return fmt.Errorf("unsupported model family %q", family)
	}
	state.harness = harness
	state.family = family
	return nil
}

func validateCrossLanguageRoute(ctx context.Context) error {
	state, err := getCrossLanguageRouteState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("cross-language route is not initialized")
	}
	return nil
}

func validateCrossLanguageSpecs(ctx context.Context) error {
	if err := validateCrossLanguageRoute(ctx); err != nil {
		return err
	}
	linter, err := earslint.New(earslint.DefaultConfig())
	if err != nil {
		return fmt.Errorf("create strict EARS linter: %w", err)
	}
	root := packageSpecBDDRepoRoot()
	for _, dir := range crossLanguageImplementationDirs {
		spec := filepath.Join(root, filepath.FromSlash(dir), "SPEC.md")
		data, err := os.ReadFile(spec)
		if err != nil {
			return fmt.Errorf("read cross-language SPEC %s: %w", spec, err)
		}
		if !strings.Contains(string(data), crossLanguageFeaturePath) {
			return fmt.Errorf("cross-language SPEC %s does not reference %s", spec, crossLanguageFeaturePath)
		}
		result, err := linter.LintFile(spec)
		if err != nil {
			return err
		}
		if result.Failed(true) {
			return fmt.Errorf("cross-language SPEC %s fails strict EARS lint: %v", spec, result.Findings)
		}
	}
	return nil
}

func getCrossLanguageRouteState(ctx context.Context) (*crossLanguageRouteState, error) {
	state, ok := ctx.Value(crossLanguageRouteStateKey{}).(*crossLanguageRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("cross-language route state not initialized")
	}
	return state, nil
}
