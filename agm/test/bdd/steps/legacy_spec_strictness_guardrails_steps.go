package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/earslint"
)

const legacySpecStrictnessFeature = "agm/test/bdd/features/legacy_spec_strictness_guardrails.feature"

var selectedLegacySpecifications = []string{
	"agm/SPEC.md",
	"agm/cmd/agm/workspace/SPEC.md",
	"agm/cmd/agm-daemon/SPEC.md",
	"agm/internal/agent/gemini/SPEC.md",
	"agm/internal/dolt/SPEC.md",
	"agm/internal/evaluation/SPEC.md",
	"cmd/vroom-governor/SPEC.md",
	"engram/SPEC.md",
	"engram/cmd/engram/SPEC.md",
	"engram/cmd/engram/cmd/SPEC.md",
	"engram/errormemory/SPEC.md",
	"engram/hooks-bin/cmd/generate-patterns/SPEC.md",
	"engram/internal/health/SPEC.md",
	"engram/mcp/SPEC.md",
	"engram/retrieval/SPEC.md",
	"internal/ci/SPEC.md",
	"internal/sandbox/SPEC.md",
	"pkg/cliframe/SPEC.md",
	"pkg/engram/SPEC.md",
	"pkg/hash/SPEC.md",
	"pkg/llm/SPEC.md",
	"pkg/progress/SPEC.md",
	"pkg/table/SPEC.md",
	"tools/benchmark-query/SPEC.md",
	"tools/devlog/SPEC.md",
	"tools/schema-registry/SPEC.md",
	"wayfinder/cmd/wayfinder-session/SPEC.md",
}

type legacySpecStrictnessStateKey struct{}

type legacySpecStrictnessState struct {
	spec    string
	harness string
	family  string
}

// RegisterLegacySpecStrictnessGuardrailSteps registers legacy SPEC convergence steps.
func RegisterLegacySpecStrictnessGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, legacySpecStrictnessStateKey{}, &legacySpecStrictnessState{}), nil
	})
	ctx.Step(`^legacy specification "([^"]*)" is selected$`, selectLegacySpecification)
	ctx.Step(`^AGM validates the selected legacy specification$`, validateSelectedLegacySpecification)
	ctx.Step(`^the legacy specification should pass strict EARS lint$`, legacySpecificationPassesStrictEARS)
	ctx.Step(`^the legacy specification should reference its executable guardrail$`, legacySpecificationReferencesGuardrail)
	ctx.Step(`^legacy specification coverage runs through "([^"]*)" with "([^"]*)"$`, configureLegacySpecificationRoute)
	ctx.Step(`^AGM validates legacy specification route parity$`, validateLegacySpecificationRoute)
	ctx.Step(`^every selected legacy specification should retain strict executable coverage$`, validateAllSelectedLegacySpecifications)
}

func selectLegacySpecification(ctx context.Context, spec string) error {
	state, err := getLegacySpecStrictnessState(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(selectedLegacySpecifications, spec) {
		state.spec = spec
		return nil
	}
	return fmt.Errorf("legacy specification %q is not governed", spec)
}

func validateSelectedLegacySpecification(ctx context.Context) error {
	state, err := getLegacySpecStrictnessState(ctx)
	if err != nil {
		return err
	}
	if state.spec == "" {
		return fmt.Errorf("legacy specification is not selected")
	}
	return nil
}

func legacySpecificationPassesStrictEARS(ctx context.Context) error {
	state, err := getLegacySpecStrictnessState(ctx)
	if err != nil {
		return err
	}
	return validateLegacySpecFile(state.spec, true)
}

func legacySpecificationReferencesGuardrail(ctx context.Context) error {
	state, err := getLegacySpecStrictnessState(ctx)
	if err != nil {
		return err
	}
	return validateLegacySpecFile(state.spec, false)
}

func configureLegacySpecificationRoute(ctx context.Context, harness, family string) error {
	state, err := getLegacySpecStrictnessState(ctx)
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

func validateLegacySpecificationRoute(ctx context.Context) error {
	state, err := getLegacySpecStrictnessState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("legacy specification route is not initialized")
	}
	return nil
}

func validateAllSelectedLegacySpecifications(ctx context.Context) error {
	if err := validateLegacySpecificationRoute(ctx); err != nil {
		return err
	}
	for _, spec := range selectedLegacySpecifications {
		if err := validateLegacySpecFile(spec, true); err != nil {
			return err
		}
	}
	return nil
}

func validateLegacySpecFile(spec string, strict bool) error {
	path := filepath.Join(packageSpecBDDRepoRoot(), filepath.FromSlash(spec))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read legacy specification %s: %w", spec, err)
	}
	if !strings.Contains(string(data), legacySpecStrictnessFeature) {
		return fmt.Errorf("legacy specification %s does not reference %s", spec, legacySpecStrictnessFeature)
	}
	if !strict {
		return nil
	}
	linter, err := earslint.New(earslint.DefaultConfig())
	if err != nil {
		return fmt.Errorf("create strict EARS linter: %w", err)
	}
	result, err := linter.LintFile(path)
	if err != nil {
		return err
	}
	if result.Failed(true) {
		return fmt.Errorf("legacy specification %s fails strict EARS lint: %v", spec, result.Findings)
	}
	return nil
}

func getLegacySpecStrictnessState(ctx context.Context) (*legacySpecStrictnessState, error) {
	state, ok := ctx.Value(legacySpecStrictnessStateKey{}).(*legacySpecStrictnessState)
	if !ok || state == nil {
		return nil, fmt.Errorf("legacy specification state not initialized")
	}
	return state, nil
}
