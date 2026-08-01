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

const legacySpecBDDLinkageFeature = "agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature"

var strictLegacySpecifications = []string{
	"wayfinder/SPEC.md",
	"internal/telemetry/SPEC.md",
	"internal/deploy/SPEC.md",
	"pkg/audit/SPEC.md",
	"pkg/audit/checks/SPEC.md",
	"cmd/fd-pressure/SPEC.md",
	"internal/fsguard/SPEC.md",
	"internal/earsbdd/SPEC.md",
	"cmd/vroom-dispatch-direct/SPEC.md",
	"cmd/agm-job/SPEC.md",
	"cmd/disk-watchdog/SPEC.md",
	"pkg/workflow/SPEC.md",
	"agm/internal/sentinel/daemon/SPEC.md",
	"cmd/dear-deploy/SPEC.md",
	"cmd/routing-guard/SPEC.md",
	"cmd/vroom-mesh/SPEC.md",
	"agm/internal/codexarchive/SPEC.md",
	"agm/internal/circuitbreaker/SPEC.md",
	"agm/cmd/agm/SPEC.md",
	"agm/internal/codexcontrol/SPEC.md",
	"agm/cmd/agm/parity/SPEC.md",
	"agm/internal/reaper/SPEC.md",
	"agm/internal/modelrouter/SPEC.md",
}

type legacySpecBDDLinkageStateKey struct{}

type legacySpecBDDLinkageState struct {
	spec    string
	harness string
	family  string
}

// RegisterLegacySpecBDDLinkageGuardrailSteps registers legacy BDD linkage steps.
func RegisterLegacySpecBDDLinkageGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, legacySpecBDDLinkageStateKey{}, &legacySpecBDDLinkageState{}), nil
	})
	ctx.Step(`^strict legacy specification "([^"]*)" is selected$`, selectStrictLegacySpecification)
	ctx.Step(`^AGM validates strict legacy BDD linkage$`, validateStrictLegacySelection)
	ctx.Step(`^the strict legacy specification should pass EARS and reciprocal linkage$`, validateSelectedStrictLegacySpecification)
	ctx.Step(`^strict legacy linkage runs through "([^"]*)" with "([^"]*)"$`, configureStrictLegacyRoute)
	ctx.Step(`^AGM validates strict legacy linkage route parity$`, validateStrictLegacyRoute)
	ctx.Step(`^every strict legacy specification should retain executable linkage$`, validateAllStrictLegacySpecifications)
}

func selectStrictLegacySpecification(ctx context.Context, spec string) error {
	state, err := getLegacySpecBDDLinkageState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(strictLegacySpecifications, spec) {
		return fmt.Errorf("strict legacy specification %q is not governed", spec)
	}
	state.spec = spec
	return nil
}

func validateStrictLegacySelection(ctx context.Context) error {
	state, err := getLegacySpecBDDLinkageState(ctx)
	if err != nil {
		return err
	}
	if state.spec == "" {
		return fmt.Errorf("strict legacy specification is not selected")
	}
	return nil
}

func validateSelectedStrictLegacySpecification(ctx context.Context) error {
	state, err := getLegacySpecBDDLinkageState(ctx)
	if err != nil {
		return err
	}
	return validateStrictLegacySpec(state.spec)
}

func configureStrictLegacyRoute(ctx context.Context, harness, family string) error {
	state, err := getLegacySpecBDDLinkageState(ctx)
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

func validateStrictLegacyRoute(ctx context.Context) error {
	state, err := getLegacySpecBDDLinkageState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("strict legacy linkage route is not initialized")
	}
	return nil
}

func validateAllStrictLegacySpecifications(ctx context.Context) error {
	if err := validateStrictLegacyRoute(ctx); err != nil {
		return err
	}
	for _, spec := range strictLegacySpecifications {
		if err := validateStrictLegacySpec(spec); err != nil {
			return err
		}
	}
	return nil
}

func validateStrictLegacySpec(spec string) error {
	path := filepath.Join(packageSpecBDDRepoRoot(), filepath.FromSlash(spec))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read strict legacy specification %s: %w", spec, err)
	}
	if !strings.Contains(string(data), legacySpecBDDLinkageFeature) {
		return fmt.Errorf("strict legacy specification %s does not reference %s", spec, legacySpecBDDLinkageFeature)
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
		return fmt.Errorf("strict legacy specification %s fails strict EARS lint: %v", spec, result.Findings)
	}
	return nil
}

func getLegacySpecBDDLinkageState(ctx context.Context) (*legacySpecBDDLinkageState, error) {
	state, ok := ctx.Value(legacySpecBDDLinkageStateKey{}).(*legacySpecBDDLinkageState)
	if !ok || state == nil {
		return nil, fmt.Errorf("strict legacy linkage state not initialized")
	}
	return state, nil
}
