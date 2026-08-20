package steps

import (
	"context"
	"encoding/json"
	"errors"
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
	// shellLint and terraformLint hold the two non-Go source gates, read
	// together so a scenario can assert on both without re-reading files.
	shellLint     string
	terraformLint string
	// jqLint is the jq fixture gate workflow.
	jqLint string
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
	ctx.Step(`^the non-Go source gates are configured$`, nonGoSourceGatesAreConfigured)
	ctx.Step(`^AGM validates non-Go source coverage$`, agmValidatesNonGoSourceCoverage)
	ctx.Step(`^shell changes should be gated on the lines they introduce$`, shellChangesGatedOnIntroducedLines)
	ctx.Step(`^the whole repository should stay ShellCheck clean at error severity$`, repositoryStaysShellCheckCleanAtError)
	ctx.Step(`^the changed-line verdict should come from a tested command$`, changedLineVerdictComesFromTestedCommand)
	ctx.Step(`^OpenTofu sources should be formatted, validated and linted$`, openTofuSourcesFormattedValidatedLinted)
	ctx.Step(`^the OpenTofu gates should require no credentials$`, openTofuGatesRequireNoCredentials)
	ctx.Step(`^the jq policy gate is configured$`, jqPolicyGateIsConfigured)
	ctx.Step(`^AGM validates jq policy coverage$`, agmValidatesJQPolicyCoverage)
	ctx.Step(`^every checked-in jq program should have a fixture case$`, everyJQProgramHasAFixtureCase)
	ctx.Step(`^jq fixtures should assert output and refusal alike$`, jqFixturesAssertOutputAndRefusal)
	ctx.Step(`^the jq gate should fail rather than skip when jq is absent from CI$`, jqGateFailsWhenJQAbsentInCI)
}

func jqPolicyGateIsConfigured(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), ".github", "workflows", "jq-lint.yml"))
	if err != nil {
		return fmt.Errorf("read jq gate workflow: %w", err)
	}
	state.jqLint = string(data)
	return nil
}

func agmValidatesJQPolicyCoverage(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if state.jqLint == "" {
		return fmt.Errorf("jq gate workflow was not loaded")
	}
	return nil
}

// everyJQProgramHasAFixtureCase walks the real tree rather than trusting the
// runner's own coverage assertion, so deleting that assertion is caught here
// too.
func everyJQProgramHasAFixtureCase(ctx context.Context) error {
	root := packageSpecBDDRepoRoot()
	fixtures, err := os.ReadDir(filepath.Join(root, "tests", "jq", "testdata"))
	if err != nil {
		return fmt.Errorf("read jq fixtures: %w", err)
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("jq gate has no fixture suites; it would pass vacuously")
	}

	var programs []string
	walkErr := filepath.WalkDir(filepath.Join(root, ".github"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".jq" {
			programs = append(programs, path)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk jq programs: %w", walkErr)
	}
	if len(programs) == 0 {
		return fmt.Errorf("no checked-in jq programs found")
	}

	covered, err := jqProgramsNamedByFixtures(filepath.Join(root, "tests", "jq", "testdata"))
	if err != nil {
		return err
	}
	for _, program := range programs {
		rel, relErr := filepath.Rel(root, program)
		if relErr != nil {
			return relErr
		}
		if !covered[filepath.ToSlash(rel)] {
			return fmt.Errorf("jq program %s has no fixture case", rel)
		}
	}
	return nil
}

// jqProgramsNamedByFixtures reads every case.json and returns the set of
// programs they exercise.
func jqProgramsNamedByFixtures(fixtureRoot string) (map[string]bool, error) {
	covered := map[string]bool{}
	err := filepath.WalkDir(fixtureRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "case.json" {
			return nil
		}
		// #nosec G122 -- path comes from a read-only walk rooted in the
		// repository's own checked-in fixture tree.
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var spec struct {
			Program string `json:"program"`
		}
		if unmarshalErr := json.Unmarshal(data, &spec); unmarshalErr != nil {
			return fmt.Errorf("%s: %w", path, unmarshalErr)
		}
		covered[spec.Program] = true
		return nil
	})
	return covered, err
}

// jqFixturesAssertOutputAndRefusal keeps the suite from decaying into
// happy-path-only coverage: a policy validator that is never shown a malformed
// document proves nothing about how it fails.
func jqFixturesAssertOutputAndRefusal(ctx context.Context) error {
	fixtureRoot := filepath.Join(packageSpecBDDRepoRoot(), "tests", "jq", "testdata")
	var outputs, refusals int
	err := filepath.WalkDir(fixtureRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch d.Name() {
		case "expected.json", "expected.txt":
			outputs++
		case "expected-error.txt":
			refusals++
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk jq fixtures: %w", err)
	}
	if outputs == 0 {
		return fmt.Errorf("no jq fixture asserts an expected output")
	}
	if refusals == 0 {
		return fmt.Errorf("no jq fixture asserts an expected refusal")
	}
	return nil
}

// jqGateFailsWhenJQAbsentInCI closes the loop on the runner's local skip: the
// skip keeps a developer machine without jq from reporting a false red, and CI
// asserts jq is present so the skip can never hide a real regression.
func jqGateFailsWhenJQAbsentInCI(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.jqLint, "command -v jq") {
		return fmt.Errorf("jq gate does not assert jq is installed, so a missing jq would silently skip")
	}
	if !strings.Contains(state.jqLint, "go test -count=1 -v ./tests/jq/") {
		return fmt.Errorf("jq gate does not replay the fixtures")
	}
	return nil
}

func nonGoSourceGatesAreConfigured(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	for _, gate := range []struct {
		name string
		into *string
	}{
		{"shell-lint.yml", &state.shellLint},
		{"terraform-lint.yml", &state.terraformLint},
	} {
		data, readErr := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), ".github", "workflows", gate.name))
		if readErr != nil {
			return fmt.Errorf("read %s: %w", gate.name, readErr)
		}
		*gate.into = string(data)
	}
	return nil
}

func agmValidatesNonGoSourceCoverage(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if state.shellLint == "" || state.terraformLint == "" {
		return fmt.Errorf("non-Go source gates were not loaded")
	}
	return nil
}

// shellChangesGatedOnIntroducedLines pins the scoping decision itself. A gate
// scoped to whole changed files would fail a one-line edit to a legacy script
// for findings its author did not write, and would be routed around instead of
// used.
func shellChangesGatedOnIntroducedLines(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	// -U0 is what makes each hunk's destination range exactly the added lines;
	// any larger context would attribute untouched code to the change.
	for _, required := range []string{"git diff -U0", "shellcheck-diff", "--min-severity style"} {
		if !strings.Contains(state.shellLint, required) {
			return fmt.Errorf("shell gate does not scope findings to introduced lines: missing %q", required)
		}
	}
	return nil
}

func repositoryStaysShellCheckCleanAtError(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	// Diff scoping alone would let a regression land in a script no pull
	// request touches; the repository-wide error-severity job is what keeps
	// that visible.
	if !strings.Contains(state.shellLint, "shellcheck -S error") {
		return fmt.Errorf("shell gate has no repository-wide error-severity baseline")
	}
	return nil
}

func changedLineVerdictComesFromTestedCommand(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	// The verdict must not live in an untestable run: block.
	if !strings.Contains(state.shellLint, "go test ./cmd/shellcheck-diff/") {
		return fmt.Errorf("shell gate does not verify its own decision logic")
	}
	specPath := filepath.Join(packageSpecBDDRepoRoot(), "cmd", "shellcheck-diff", "SPEC.md")
	if _, err := os.Stat(specPath); err != nil {
		return fmt.Errorf("shell gate decision command has no co-located SPEC: %w", err)
	}
	return nil
}

func openTofuSourcesFormattedValidatedLinted(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	for _, required := range []string{"tofu fmt -check -recursive", "tofu validate", "tflint --recursive"} {
		if !strings.Contains(state.terraformLint, required) {
			return fmt.Errorf("OpenTofu gate is missing %q", required)
		}
	}
	return nil
}

// openTofuGatesRequireNoCredentials keeps the gate safe to run on a pull
// request from any source: it must never reach the real state backend, and a
// plan would need provider credentials this workflow deliberately lacks.
func openTofuGatesRequireNoCredentials(ctx context.Context) error {
	state, err := getDeclarativeRuntimeRouteState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.terraformLint, "-backend=false") {
		return fmt.Errorf("OpenTofu gate does not initialise without a backend")
	}
	if strings.Contains(state.terraformLint, "tofu plan") {
		return fmt.Errorf("OpenTofu lint gate must not plan; planning needs credentials it deliberately lacks")
	}
	if strings.Contains(state.terraformLint, "secrets.") {
		return fmt.Errorf("OpenTofu lint gate must not consume repository secrets")
	}
	return nil
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
	if !info.Mode().IsRegular() {
		return fmt.Errorf("declarative runtime asset %s must be a regular discoverable file", path)
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
	resolvedPluginRoot, err := filepath.EvalSymlinks(pluginRoot)
	if err != nil {
		return fmt.Errorf("resolve declarative runtime plugin root %s: %w", pluginRoot, err)
	}
	for _, declared := range manifest.Skills {
		if err := validatePluginSkillDeclaration(path, pluginRoot, resolvedPluginRoot, manifest.Name, declared); err != nil {
			return err
		}
	}
	return nil
}

func validatePluginSkillDeclaration(path, pluginRoot, resolvedPluginRoot, pluginName, declared string) error {
	skillDir := filepath.Clean(filepath.Join(pluginRoot, filepath.FromSlash(declared)))
	if pathEscapesRoot(pluginRoot, skillDir) {
		return fmt.Errorf("declarative runtime asset %s declares skill path outside its plugin root: %q", path, declared)
	}
	candidates := []string{
		filepath.Join(skillDir, "SKILL.md"),
		filepath.Join(skillDir, pluginName, "SKILL.md"),
	}
	for _, candidate := range candidates {
		found, err := validatePluginSkillEntrypoint(path, resolvedPluginRoot, candidate)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
	}
	return fmt.Errorf("declarative runtime asset %s skill path %q does not contain canonical %s/SKILL.md", path, declared, pluginName)
}

func validatePluginSkillEntrypoint(path, resolvedPluginRoot, candidate string) (bool, error) {
	info, err := os.Lstat(candidate)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat declarative runtime skill entrypoint %s: %w", candidate, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("declarative runtime asset %s skill entrypoint %s must be a regular file, not a symlink or special file", path, candidate)
	}
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false, fmt.Errorf("resolve declarative runtime skill entrypoint %s: %w", candidate, err)
	}
	if pathEscapesRoot(resolvedPluginRoot, resolvedCandidate) {
		return false, fmt.Errorf("declarative runtime asset %s skill entrypoint resolves outside its plugin root: %s", path, candidate)
	}
	return true, nil
}

func pathEscapesRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))
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

var supportedDeclarativeEvalHarnesses = map[string]struct{}{
	"claude": {},
	"codex":  {},
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
	seenIDs := make(map[string]int, len(evals.Cases))
	for index, evalCase := range evals.Cases {
		if err := validateEvalCase(path, index, evalCase); err != nil {
			return err
		}
		if previous, exists := seenIDs[evalCase.ID]; exists {
			return fmt.Errorf("declarative runtime asset %s eval case %d duplicates id %q from case %d", path, index, evalCase.ID, previous)
		}
		seenIDs[evalCase.ID] = index
	}
	return nil
}

func validateEvalCase(path string, index int, evalCase declarativeEvalCase) error {
	if strings.TrimSpace(evalCase.ID) == "" || strings.TrimSpace(evalCase.Prompt) == "" || len(evalCase.Harness) == 0 ||
		evalCase.ShouldTrigger == nil || evalCase.Trials <= 0 || len(evalCase.ExpectedChecks) == 0 {
		return fmt.Errorf("declarative runtime asset %s eval case %d lacks required fields", path, index)
	}
	if err := validateEvalHarnesses(path, index, evalCase.Harness); err != nil {
		return err
	}
	for checkIndex, check := range evalCase.ExpectedChecks {
		if strings.TrimSpace(check.Type) == "" || strings.TrimSpace(check.Target) == "" || strings.TrimSpace(check.Pattern) == "" {
			return fmt.Errorf("declarative runtime asset %s eval case %d check %d lacks required fields", path, index, checkIndex)
		}
		if check.Type != "regex" {
			return fmt.Errorf("declarative runtime asset %s eval case %d check %d has unsupported type %q", path, index, checkIndex, check.Type)
		}
		if check.Target != "trace" {
			return fmt.Errorf("declarative runtime asset %s eval case %d check %d has unsupported target %q", path, index, checkIndex, check.Target)
		}
		if _, err := regexp.Compile(check.Pattern); err != nil {
			return fmt.Errorf("declarative runtime asset %s eval case %d check %d has invalid regex: %w", path, index, checkIndex, err)
		}
	}
	return nil
}

func validateEvalHarnesses(path string, caseIndex int, harnesses []string) error {
	for harnessIndex, harness := range harnesses {
		if strings.TrimSpace(harness) == "" {
			return fmt.Errorf("declarative runtime asset %s eval case %d harness %d lacks a value", path, caseIndex, harnessIndex)
		}
		if _, ok := supportedDeclarativeEvalHarnesses[harness]; !ok {
			return fmt.Errorf("declarative runtime asset %s eval case %d harness %d has unsupported value %q", path, caseIndex, harnessIndex, harness)
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
