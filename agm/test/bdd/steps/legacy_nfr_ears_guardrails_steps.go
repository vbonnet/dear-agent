package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/spec-governance/earslint"
)

const legacyNFREARSFeature = "agm/test/bdd/features/legacy_nfr_ears_guardrails.feature"

var convertedLegacySpecifications = []string{
	"engram/ecphory/SPEC.md",
	"engram/ecphory/ranking/SPEC.md",
	"pkg/monitoring/SPEC.md",
	"pkg/telemetry/SPEC.md",
	"tools/dod-enforcer/SPEC.md",
}

type legacyNFREARSStateKey struct{}

type legacyNFREARSState struct {
	spec    string
	harness string
	family  string
}

// RegisterLegacyNFREARSGuardrailSteps registers converted legacy requirement steps.
func RegisterLegacyNFREARSGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, legacyNFREARSStateKey{}, &legacyNFREARSState{}), nil
	})
	ctx.Step(`^converted legacy specification "([^"]*)" is selected$`, selectConvertedLegacySpecification)
	ctx.Step(`^AGM validates converted legacy requirements$`, validateConvertedLegacySelection)
	ctx.Step(`^the converted legacy specification should pass strict EARS lint$`, convertedLegacySpecificationPassesStrictEARS)
	ctx.Step(`^the converted legacy specification should reference its executable guardrail$`, convertedLegacySpecificationReferencesGuardrail)
	ctx.Step(`^converted legacy coverage runs through "([^"]*)" with "([^"]*)"$`, configureConvertedLegacyRoute)
	ctx.Step(`^AGM validates converted legacy route parity$`, validateConvertedLegacyRoute)
	ctx.Step(`^every converted legacy specification should retain strict executable coverage$`, validateAllConvertedLegacySpecifications)
}

func selectConvertedLegacySpecification(ctx context.Context, spec string) error {
	state, err := getLegacyNFREARSState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(convertedLegacySpecifications, spec) {
		return fmt.Errorf("converted legacy specification %q is not governed", spec)
	}
	state.spec = spec
	return nil
}

func validateConvertedLegacySelection(ctx context.Context) error {
	state, err := getLegacyNFREARSState(ctx)
	if err != nil {
		return err
	}
	if state.spec == "" {
		return fmt.Errorf("converted legacy specification is not selected")
	}
	return nil
}

func convertedLegacySpecificationPassesStrictEARS(ctx context.Context) error {
	state, err := getLegacyNFREARSState(ctx)
	if err != nil {
		return err
	}
	return validateConvertedLegacySpec(state.spec, true)
}

func convertedLegacySpecificationReferencesGuardrail(ctx context.Context) error {
	state, err := getLegacyNFREARSState(ctx)
	if err != nil {
		return err
	}
	return validateConvertedLegacySpec(state.spec, false)
}

func configureConvertedLegacyRoute(ctx context.Context, harness, family string) error {
	state, err := getLegacyNFREARSState(ctx)
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

func validateConvertedLegacyRoute(ctx context.Context) error {
	state, err := getLegacyNFREARSState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("converted legacy route is not initialized")
	}
	return nil
}

func validateAllConvertedLegacySpecifications(ctx context.Context) error {
	if err := validateConvertedLegacyRoute(ctx); err != nil {
		return err
	}
	for _, spec := range convertedLegacySpecifications {
		if err := validateConvertedLegacySpec(spec, true); err != nil {
			return err
		}
	}
	return nil
}

func validateConvertedLegacySpec(spec string, strict bool) error {
	path := filepath.Join(packageSpecBDDRepoRoot(), filepath.FromSlash(spec))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read converted legacy specification %s: %w", spec, err)
	}
	if !strings.Contains(string(data), legacyNFREARSFeature) {
		return fmt.Errorf("converted legacy specification %s does not reference %s", spec, legacyNFREARSFeature)
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
		return fmt.Errorf("converted legacy specification %s fails strict EARS lint: %v", spec, result.Findings)
	}
	return nil
}

func getLegacyNFREARSState(ctx context.Context) (*legacyNFREARSState, error) {
	state, ok := ctx.Value(legacyNFREARSStateKey{}).(*legacyNFREARSState)
	if !ok || state == nil {
		return nil, fmt.Errorf("converted legacy specification state not initialized")
	}
	return state, nil
}
