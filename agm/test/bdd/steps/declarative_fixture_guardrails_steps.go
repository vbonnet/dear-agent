package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/spec-governance/earslint"
)

const declarativeFixtureFeaturePath = "agm/test/bdd/features/declarative_fixture_guardrails.feature"

var declarativeFixtureDirs = []string{
	"agm/internal/testdata/file-provenance",
	"agm/internal/testdata/mock-manifests",
	"agm/internal/testdata/orphan-recovery",
	"agm/test/golden",
	"agm/test/golden/agent-interactions",
	"agm/tests/e2e-install/Dockerfiles",
	"benchmarks/baselines",
	"engram/internal/health/testdata",
	"pkg/config-loader/testdata",
	"pkg/workspace/testdata",
	"tools/dod-enforcer/examples",
	"wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/eslint",
	"wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/golangci",
	"wayfinder/cmd/wayfinder-session/internal/lintcontext/testdata/python",
	"wayfinder/cmd/wayfinder-session/internal/status/testdata",
}

type declarativeFixtureGuardrailStateKey struct{}
type declarativeFixtureRouteStateKey struct{}

type declarativeFixtureRouteState struct {
	harness string
	family  string
}

// RegisterDeclarativeFixtureGuardrailSteps registers fixture coverage steps.
func RegisterDeclarativeFixtureGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          declarativeFixtureGuardrailStateKey{},
		label:             "declarative fixture directory",
		featurePath:       declarativeFixtureFeaturePath,
		configuredPattern: `^declarative fixture directory "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates declarative fixture coverage$`,
		colocatedPattern:  `^declarative fixture directory "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, declarativeFixtureRouteStateKey{}, &declarativeFixtureRouteState{}), nil
	})
	ctx.Step(`^declarative fixture coverage runs through "([^"]*)" with "([^"]*)"$`, configureDeclarativeFixtureRoute)
	ctx.Step(`^AGM validates declarative fixture route parity$`, validateDeclarativeFixtureRoute)
	ctx.Step(`^every declarative fixture contract should retain strict SPEC and BDD traceability$`, validateDeclarativeFixtureSpecs)
}

func configureDeclarativeFixtureRoute(ctx context.Context, harness, family string) error {
	state, err := getDeclarativeFixtureRouteState(ctx)
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

func validateDeclarativeFixtureRoute(ctx context.Context) error {
	state, err := getDeclarativeFixtureRouteState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("declarative fixture route is not initialized")
	}
	return nil
}

func validateDeclarativeFixtureSpecs(ctx context.Context) error {
	if err := validateDeclarativeFixtureRoute(ctx); err != nil {
		return err
	}
	linter, err := earslint.New(earslint.DefaultConfig())
	if err != nil {
		return fmt.Errorf("create strict EARS linter: %w", err)
	}
	root := packageSpecBDDRepoRoot()
	for _, dir := range declarativeFixtureDirs {
		spec := filepath.Join(root, filepath.FromSlash(dir), "SPEC.md")
		data, err := os.ReadFile(spec)
		if err != nil {
			return fmt.Errorf("read declarative fixture SPEC %s: %w", spec, err)
		}
		if !strings.Contains(string(data), declarativeFixtureFeaturePath) {
			return fmt.Errorf("declarative fixture SPEC %s does not reference %s", spec, declarativeFixtureFeaturePath)
		}
		result, err := linter.LintFile(spec)
		if err != nil {
			return err
		}
		if result.Failed(true) {
			return fmt.Errorf("declarative fixture SPEC %s fails strict EARS lint: %v", spec, result.Findings)
		}
	}
	return nil
}

func getDeclarativeFixtureRouteState(ctx context.Context) (*declarativeFixtureRouteState, error) {
	state, ok := ctx.Value(declarativeFixtureRouteStateKey{}).(*declarativeFixtureRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("declarative fixture route state not initialized")
	}
	return state, nil
}
