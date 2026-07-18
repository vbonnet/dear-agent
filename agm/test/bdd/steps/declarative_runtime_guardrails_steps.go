package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/earslint"
)

const declarativeRuntimeFeaturePath = "agm/test/bdd/features/declarative_runtime_guardrails.feature"

var declarativeRuntimeDirs = []string{
	".agents/skills/beads/agents",
	".github",
	".github/act",
	".github/rulesets",
	".github/workflows",
	"agm/.claude-plugin",
	"agm/.github/workflows",
	"agm/agm-plugin/.claude-plugin",
	"agm/agm-plugin/channels/agm-bus",
	"agm/cmd/agm/schedules",
	"agm/contracts",
	"agm/schemas",
	"agm/systemd",
	"agm/test/e2e/docker",
	"agm/youtube-plugin/.claude-plugin",
	"cmd/dear-agent-bumblebee/templates",
	"config",
	"configs/workflows",
	"deploy",
	"deploy/launchd",
	"pkg/codeintel/rules/go",
	"pkg/codeintel/rules/python",
	"pkg/codeintel/rules/typescript",
	"wayfinder/.claude-plugin",
}

type declarativeRuntimeGuardrailStateKey struct{}
type declarativeRuntimeRouteStateKey struct{}

type declarativeRuntimeRouteState struct {
	harness string
	family  string
}

// RegisterDeclarativeRuntimeGuardrailSteps registers runtime configuration coverage steps.
func RegisterDeclarativeRuntimeGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          declarativeRuntimeGuardrailStateKey{},
		label:             "declarative runtime directory",
		featurePath:       declarativeRuntimeFeaturePath,
		configuredPattern: `^declarative runtime directory "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates declarative runtime coverage$`,
		colocatedPattern:  `^declarative runtime directory "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, declarativeRuntimeRouteStateKey{}, &declarativeRuntimeRouteState{}), nil
	})
	ctx.Step(`^declarative runtime coverage runs through "([^"]*)" with "([^"]*)"$`, configureDeclarativeRuntimeRoute)
	ctx.Step(`^AGM validates declarative runtime route parity$`, validateDeclarativeRuntimeRoute)
	ctx.Step(`^every declarative runtime contract should retain strict SPEC and BDD traceability$`, validateDeclarativeRuntimeSpecs)
}

func configureDeclarativeRuntimeRoute(ctx context.Context, harness, family string) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if _, ok := map[string]struct{}{"claude-code": {}, "codex-cli": {}, "agy": {}, "opencode-cli": {}}[harness]; !ok {
		return fmt.Errorf("unsupported active harness %q", harness)
	}
	if _, ok := map[string]struct{}{"anthropic": {}, "openai": {}, "gemini": {}, "glm": {}, "deepseek": {}, "nemotron": {}, "qwen": {}}[family]; !ok {
		return fmt.Errorf("unsupported model family %q", family)
	}
	state.harness = harness
	state.family = family
	return nil
}

func validateDeclarativeRuntimeRoute(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("declarative runtime route is not initialized")
	}
	return nil
}

func validateDeclarativeRuntimeSpecs(ctx context.Context) error {
	if err := validateDeclarativeRuntimeRoute(ctx); err != nil {
		return err
	}
	linter, err := earslint.New(earslint.DefaultConfig())
	if err != nil {
		return fmt.Errorf("create strict EARS linter: %w", err)
	}
	root := packageSpecBDDRepoRoot()
	for _, dir := range declarativeRuntimeDirs {
		spec := filepath.Join(root, filepath.FromSlash(dir), "SPEC.md")
		data, err := os.ReadFile(spec)
		if err != nil {
			return fmt.Errorf("read declarative runtime SPEC %s: %w", spec, err)
		}
		if !strings.Contains(string(data), declarativeRuntimeFeaturePath) {
			return fmt.Errorf("declarative runtime SPEC %s does not reference %s", spec, declarativeRuntimeFeaturePath)
		}
		result, err := linter.LintFile(spec)
		if err != nil {
			return err
		}
		if result.Failed(true) {
			return fmt.Errorf("declarative runtime SPEC %s fails strict EARS lint: %v", spec, result.Findings)
		}
	}
	return nil
}

func getDeclarativeRuntimeRouteState(ctx context.Context) (*declarativeRuntimeRouteState, error) {
	state, ok := ctx.Value(declarativeRuntimeRouteStateKey{}).(*declarativeRuntimeRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("declarative runtime route state not initialized")
	}
	return state, nil
}
