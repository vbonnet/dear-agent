package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
	"gopkg.in/yaml.v3"

	"github.com/vbonnet/dear-agent/internal/earslint"
)

const declarativeRuntimeFeaturePath = "agm/test/bdd/features/declarative_runtime_guardrails.feature"

var declarativeRuntimeDirs = []string{
	".agents/skills/beads/agents",
	".agents/skills/research-pipeline/agents",
	".github",
	".github/act",
	".github/rulesets",
	".github/workflows",
	".opencode/skills/research-pipeline",
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
	"research-pipeline/.claude-plugin",
	"research-pipeline/skills/research-pipeline",
	"wayfinder/.claude-plugin",
}

var declarativeRuntimeAssets = map[string][]string{
	".agents/skills/research-pipeline/agents":    {"openai.yaml"},
	".opencode/skills/research-pipeline":         {"SKILL.md"},
	"research-pipeline/.claude-plugin":           {"plugin.json"},
	"research-pipeline/skills/research-pipeline": {"SKILL.md", "evals.json"},
}

var declarativeRuntimeAssetValidators = map[string]func(string, []byte) error{
	"plugin.json": validatePluginManifestAsset,
	"evals.json":  validateEvalCasesAsset,
	"openai.yaml": validateOpenAIInterfaceAsset,
}

type declarativeRuntimeGuardrailStateKey struct{}
type declarativeRuntimeRouteStateKey struct{}

type declarativeRuntimeRouteState struct {
	harness string
	family  string
	ci      string
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
	ctx.Step(`^the repository CI workflow is configured$`, repositoryCIWorkflowIsConfigured)
	ctx.Step(`^AGM validates the Codex contract CI job$`, agmValidatesCodexContractCIJob)
	ctx.Step(`^CI should run portable active harness parity$`, ciShouldRunPortableActiveHarnessParity)
	ctx.Step(`^CI should run the isolated source-built Codex lifecycle$`, ciShouldRunIsolatedSourceBuiltCodexLifecycle)
	ctx.Step(`^CI should enforce critical lifecycle coverage$`, ciShouldEnforceCriticalLifecycleCoverage)
	ctx.Step(`^scheduled CI should run the credential-free tagged graphs$`, scheduledCIShouldRunCredentialFreeTaggedGraphs)
}

func repositoryCIWorkflowIsConfigured(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), ".github", "workflows", "ci.yml"))
	if err != nil {
		return fmt.Errorf("read CI workflow: %w", err)
	}
	state.ci = string(data)
	return nil
}

func agmValidatesCodexContractCIJob(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.ci, "agm-codex-contracts:") {
		return fmt.Errorf("CI workflow has no AGM Codex contract job")
	}
	return nil
}

func ciShouldRunPortableActiveHarnessParity(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	for _, required := range []string{"TestActiveHarnessParityContract", "TestHarnessPrerequisitesAreScoped"} {
		if !strings.Contains(state.ci, required) {
			return fmt.Errorf("CI Codex contract job does not run %s", required)
		}
	}
	return nil
}

func ciShouldRunIsolatedSourceBuiltCodexLifecycle(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.ci, "TestCodexLifecycleUsesIsolatedSourceEnvironment") {
		return fmt.Errorf("CI Codex contract job does not run the isolated source-built lifecycle")
	}
	return nil
}

func ciShouldEnforceCriticalLifecycleCoverage(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	for _, required := range []string{"go run ./cmd/coverage-ratchet", "agm/test/coverage/critical-lifecycle.json"} {
		if !strings.Contains(state.ci, required) {
			return fmt.Errorf("CI Codex contract job does not enforce %s", required)
		}
	}
	return nil
}

func scheduledCIShouldRunCredentialFreeTaggedGraphs(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	for _, required := range []string{
		"agm-tagged-sweep:",
		"github.event_name == 'schedule' || github.event_name == 'workflow_dispatch'",
		"-tags=contract ./agm/test/contract/...",
		"-tags=integration ./agm/test/integration/...",
	} {
		if !strings.Contains(state.ci, required) {
			return fmt.Errorf("scheduled CI does not retain %s", required)
		}
	}
	sweepStart := strings.Index(state.ci, "  agm-tagged-sweep:")
	if sweepStart < 0 {
		return fmt.Errorf("scheduled CI tagged sweep job is missing")
	}
	sweep, _, found := strings.Cut(state.ci[sweepStart:], "\n  engram-storage-hardening:")
	if !found {
		return fmt.Errorf("scheduled CI tagged sweep job boundary is missing")
	}
	if strings.Contains(sweep, "SKIP_E2E") {
		return fmt.Errorf("scheduled CI full integration graph still sets a package-wide opt-out")
	}
	return nil
}

func configureDeclarativeRuntimeRoute(ctx context.Context, harness, family string) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
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
		for _, asset := range declarativeRuntimeAssets[dir] {
			if err := validateDeclarativeRuntimeAsset(root, dir, asset); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDeclarativeRuntimeAsset(root, dir, asset string) error {
	path := filepath.Join(root, filepath.FromSlash(dir), asset)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat declarative runtime asset %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("declarative runtime asset %s must be a regular discoverable file, not a symlink", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read declarative runtime asset %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return fmt.Errorf("declarative runtime asset %s is empty", path)
	}

	if validate := declarativeRuntimeAssetValidators[asset]; validate != nil {
		return validate(path, data)
	}
	return nil
}

func validatePluginManifestAsset(path string, data []byte) error {
	var manifest struct {
		Name    string   `json:"name"`
		Version string   `json:"version"`
		Skills  []string `json:"skills"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse declarative runtime asset %s: %w", path, err)
	}
	if manifest.Name == "" || manifest.Version == "" || len(manifest.Skills) == 0 {
		return fmt.Errorf("declarative runtime asset %s lacks name, version, or skills", path)
	}
	pluginRoot := filepath.Dir(filepath.Dir(path))
	for _, declared := range manifest.Skills {
		skillDir := filepath.Clean(filepath.Join(pluginRoot, filepath.FromSlash(declared)))
		relative, err := filepath.Rel(pluginRoot, skillDir)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("declarative runtime asset %s declares skill path outside its plugin root: %q", path, declared)
		}
		candidates := []string{
			filepath.Join(skillDir, "SKILL.md"),
			filepath.Join(skillDir, manifest.Name, "SKILL.md"),
		}
		found := false
		for _, candidate := range candidates {
			info, statErr := os.Stat(candidate)
			if statErr == nil && !info.IsDir() {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("declarative runtime asset %s skill path %q does not contain canonical %s/SKILL.md", path, declared, manifest.Name)
		}
	}
	return nil
}

type evalExpectedCheck struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	Pattern string `json:"pattern"`
}

type declarativeEvalCase struct {
	ID             string              `json:"id"`
	Prompt         string              `json:"prompt"`
	Harness        []string            `json:"harness"`
	ShouldTrigger  *bool               `json:"should_trigger"`
	Trials         int                 `json:"trials"`
	ExpectedChecks []evalExpectedCheck `json:"expected_checks"`
}

func validateEvalCasesAsset(path string, data []byte) error {
	var evals struct {
		Cases []declarativeEvalCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &evals); err != nil {
		return fmt.Errorf("parse declarative runtime asset %s: %w", path, err)
	}
	if len(evals.Cases) == 0 {
		return fmt.Errorf("declarative runtime asset %s has no eval cases", path)
	}
	for index, evalCase := range evals.Cases {
		if err := validateEvalCase(path, index, evalCase); err != nil {
			return err
		}
	}
	return nil
}

func validateEvalCase(path string, index int, evalCase declarativeEvalCase) error {
	if evalCase.ID == "" || evalCase.Prompt == "" || len(evalCase.Harness) == 0 ||
		evalCase.ShouldTrigger == nil || evalCase.Trials <= 0 || len(evalCase.ExpectedChecks) == 0 {
		return fmt.Errorf("declarative runtime asset %s eval case %d lacks required fields", path, index)
	}
	for checkIndex, check := range evalCase.ExpectedChecks {
		if check.Type == "" || check.Target == "" || check.Pattern == "" {
			return fmt.Errorf("declarative runtime asset %s eval case %d check %d lacks required fields", path, index, checkIndex)
		}
		if check.Type == "regex" {
			if _, err := regexp.Compile(check.Pattern); err != nil {
				return fmt.Errorf("declarative runtime asset %s eval case %d check %d has invalid regex: %w", path, index, checkIndex, err)
			}
		}
	}
	return nil
}

func validateOpenAIInterfaceAsset(path string, data []byte) error {
	var config struct {
		Interface struct {
			DisplayName      string `yaml:"display_name"`
			ShortDescription string `yaml:"short_description"`
			DefaultPrompt    string `yaml:"default_prompt"`
		} `yaml:"interface"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse declarative runtime asset %s: %w", path, err)
	}
	if config.Interface.DisplayName == "" || config.Interface.ShortDescription == "" || config.Interface.DefaultPrompt == "" {
		return fmt.Errorf("declarative runtime asset %s lacks its published interface fields", path)
	}
	descriptionLength := len([]rune(config.Interface.ShortDescription))
	if descriptionLength < 25 || descriptionLength > 64 {
		return fmt.Errorf("declarative runtime asset %s short_description must contain 25-64 characters, got %d", path, descriptionLength)
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
