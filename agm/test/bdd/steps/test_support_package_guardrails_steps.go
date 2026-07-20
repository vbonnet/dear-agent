package steps

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/internal/earslint"
)

const testSupportFeaturePath = "agm/test/bdd/features/test_support_package_guardrails.feature"

var residualTestSupportPackages = []string{
	"agm/examples",
	"agm/internal/testcontext",
	"agm/internal/testutil",
	"agm/scripts",
	"agm/test",
	"agm/test/bdd",
	"agm/test/bdd/steps",
	"agm/test/contract",
	"agm/test/contracts",
	"agm/test/e2e",
	"agm/test/helpers",
	"agm/test/integration",
	"agm/test/integration/helpers",
	"agm/test/integration/lifecycle",
	"agm/test/performance",
	"agm/test/regression",
	"agm/test/unit",
	"engram/hooks-bin/cmd/integration_test",
	"engram/hooks-bin/internal/integration_test",
	"engram/internal/testutil",
	"internal/testutil",
	"wayfinder/cmd/wayfinder-session/internal/integration",
}

type testSupportPackageGuardrailStateKey struct{}
type testSupportRouteStateKey struct{}

type testSupportRouteState struct {
	harness              string
	family               string
	trustIsolationOutput string
	trustIsolationErr    error
}

// RegisterTestSupportPackageGuardrailSteps registers residual package coverage steps.
func RegisterTestSupportPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          testSupportPackageGuardrailStateKey{},
		label:             "test support package",
		featurePath:       testSupportFeaturePath,
		configuredPattern: `^test support package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates test support package coverage$`,
		colocatedPattern:  `^test support package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, testSupportRouteStateKey{}, &testSupportRouteState{}), nil
	})
	ctx.Step(`^test support coverage runs through "([^"]*)" with "([^"]*)"$`, configureTestSupportRoute)
	ctx.Step(`^AGM validates residual support package parity$`, validateTestSupportRoute)
	ctx.Step(`^every residual support package should retain strict SPEC and BDD traceability$`, validateResidualTestSupportSpecs)
	ctx.Step(`^live harness contract sources are configured$`, liveHarnessContractSourcesAreConfigured)
	ctx.Step(`^AGM validates live harness contract command construction$`, agmValidatesLiveHarnessContractCommands)
	ctx.Step(`^live harness contracts should use canonical session and harness arguments$`, liveHarnessContractsUseCanonicalArguments)
	ctx.Step(`^unavailable live harness dependencies should be skipped explicitly$`, unavailableLiveHarnessDependenciesAreSkipped)
	ctx.Step(`^AGM validates trust protocol scenario isolation$`, agmValidatesTrustProtocolScenarioIsolation)
	ctx.Step(`^trust protocol setup should run only for trust scenarios$`, trustProtocolSetupShouldBeScoped)
	ctx.Step(`^trust protocol hooks should restore HOME and shared Go cache variables$`, trustProtocolHooksShouldRestoreEnvironment)
	ctx.Step(`^trust protocol cleanup should remove read-only owned module trees$`, trustProtocolCleanupShouldRemoveReadOnlyModuleTrees)
}

func trustProtocolSetupShouldBeScoped(ctx context.Context) error {
	return requireTrustProtocolIsolationTests(ctx, "TestTrustProtocolHookScope")
}

func agmValidatesTrustProtocolScenarioIsolation(ctx context.Context) error {
	state, err := getTestSupportRouteState(ctx)
	if err != nil {
		return err
	}
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(testCtx, "go", "test", "./agm/test/bdd/steps", "-run", `^TestTrustProtocol`, "-count=1", "-v")
	cmd.Dir = packageSpecBDDRepoRoot()
	output, runErr := cmd.CombinedOutput()
	state.trustIsolationOutput = string(output)
	state.trustIsolationErr = runErr
	return nil
}

func trustProtocolHooksShouldRestoreEnvironment(ctx context.Context) error {
	return requireTrustProtocolIsolationTests(ctx,
		"TestTrustProtocolEnvironmentRoundTrip",
		"TestTrustProtocolResolveGoCachesUsesExplicitEnvironment",
	)
}

func trustProtocolCleanupShouldRemoveReadOnlyModuleTrees(ctx context.Context) error {
	return requireTrustProtocolIsolationTests(ctx,
		"TestTrustProtocolCleanupRemovesReadOnlyModuleTree",
		"TestTrustProtocolCleanupRejectsUnownedDirectory",
	)
}

func requireTrustProtocolIsolationTests(ctx context.Context, tests ...string) error {
	state, err := getTestSupportRouteState(ctx)
	if err != nil {
		return err
	}
	if state.trustIsolationErr != nil {
		return fmt.Errorf("trust protocol isolation tests failed: %w\n%s", state.trustIsolationErr, state.trustIsolationOutput)
	}
	for _, test := range tests {
		if !strings.Contains(state.trustIsolationOutput, "--- PASS: "+test) {
			return fmt.Errorf("trust protocol isolation test %s did not pass:\n%s", test, state.trustIsolationOutput)
		}
	}
	return nil
}

func liveHarnessContractSourcesAreConfigured() error {
	root := filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "contract")
	for _, name := range []string{"claude_contract_test.go", "gemini_contract_test.go", "opencode_contract_test.go", "cli_helpers_test.go"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			return fmt.Errorf("live contract source %s: %w", name, err)
		}
	}
	return nil
}

func agmValidatesLiveHarnessContractCommands() error {
	return nil
}

func liveHarnessContractsUseCanonicalArguments() error {
	root := filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "contract")
	helperData, err := os.ReadFile(filepath.Join(root, "cli_helpers_test.go"))
	if err != nil {
		return err
	}
	helper := string(helperData)
	for _, required := range []string{`"session", "new"`, `"--harness"`, `"send", "msg"`} {
		if !strings.Contains(helper, required) {
			return fmt.Errorf("live contract CLI helper lacks canonical argument %s", required)
		}
	}
	for _, name := range []string{"claude_contract_test.go", "gemini_contract_test.go", "opencode_contract_test.go"} {
		data, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			return readErr
		}
		source := string(data)
		for _, retired := range []string{`helpers.RunCLI(t, "new"`, `"--agent"`} {
			if strings.Contains(source, retired) {
				return fmt.Errorf("live contract %s retains retired CLI form %s", name, retired)
			}
		}
	}
	return nil
}

func unavailableLiveHarnessDependenciesAreSkipped() error {
	root := filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "contract")
	helperData, err := os.ReadFile(filepath.Join(root, "cli_helpers_test.go"))
	if err != nil {
		return err
	}
	helper := string(helperData)
	for _, required := range []string{"OPENCODE_SERVER_URL", "Skip", "/health", "2 * time.Second"} {
		if !strings.Contains(helper, required) {
			return fmt.Errorf("OpenCode live contract guard lacks %q", required)
		}
	}
	opencodeData, err := os.ReadFile(filepath.Join(root, "opencode_contract_test.go"))
	if err != nil {
		return err
	}
	if count := strings.Count(string(opencodeData), "requireOpenCodeServer(t)"); count != 5 {
		return fmt.Errorf("OpenCode live contracts guard %d tests, want 5", count)
	}
	return nil
}

func configureTestSupportRoute(ctx context.Context, harness, family string) error {
	state, err := getTestSupportRouteState(ctx)
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

func validateTestSupportRoute(ctx context.Context) error {
	state, err := getTestSupportRouteState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("test support route is not initialized")
	}
	return nil
}

func validateResidualTestSupportSpecs(ctx context.Context) error {
	if err := validateTestSupportRoute(ctx); err != nil {
		return err
	}
	linter, err := earslint.New(earslint.DefaultConfig())
	if err != nil {
		return fmt.Errorf("create strict EARS linter: %w", err)
	}
	root := packageSpecBDDRepoRoot()
	for _, pkg := range residualTestSupportPackages {
		spec := filepath.Join(root, filepath.FromSlash(pkg), "SPEC.md")
		data, err := os.ReadFile(spec)
		if err != nil {
			return fmt.Errorf("read residual SPEC %s: %w", spec, err)
		}
		if !strings.Contains(string(data), testSupportFeaturePath) {
			return fmt.Errorf("residual SPEC %s does not reference %s", spec, testSupportFeaturePath)
		}
		result, err := linter.LintFile(spec)
		if err != nil {
			return err
		}
		if result.Failed(true) {
			return fmt.Errorf("residual SPEC %s fails strict EARS lint: %v", spec, result.Findings)
		}
	}
	return nil
}

func getTestSupportRouteState(ctx context.Context) (*testSupportRouteState, error) {
	state, ok := ctx.Value(testSupportRouteStateKey{}).(*testSupportRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("test support route state not initialized")
	}
	return state, nil
}
