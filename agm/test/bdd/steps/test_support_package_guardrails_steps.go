package steps

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
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
	ctx.Step(`^AGM performance workload sources are configured$`, agmPerformanceWorkloadSourcesAreConfigured)
	ctx.Step(`^AGM validates performance client readiness$`, agmValidatesPerformanceClientReadiness)
	ctx.Step(`^performance workloads should use bounded hub client readiness$`, performanceWorkloadsUseBoundedHubClientReadiness)
	ctx.Step(`^churn cleanup should be observed before stable clients disconnect$`, churnCleanupIsObservedBeforeStableClientsDisconnect)
	ctx.Step(`^isolated Codex lifecycle test sources are configured$`, isolatedCodexLifecycleTestSourcesAreConfigured)
	ctx.Step(`^AGM validates real lifecycle isolation$`, agmValidatesRealLifecycleIsolation)
	ctx.Step(`^the lifecycle should use a source-built AGM and unique tmux socket$`, lifecycleUsesSourceBuiltAGMAndUniqueTmuxSocket)
	ctx.Step(`^cleanup should target only owned test resources$`, cleanupTargetsOnlyOwnedTestResources)
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
	cmd := newTrustIsolationTestCommand(testCtx)
	cmd.Dir = packageSpecBDDRepoRoot()
	output, runErr := cmd.CombinedOutput()
	state.trustIsolationOutput = string(output)
	state.trustIsolationErr = runErr
	return nil
}

func newTrustIsolationTestCommand(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "go", "test", "./agm/test/bdd/steps", "-run", `^TestTrustProtocol`, "-count=1", "-timeout=90s", "-v")
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	return cmd
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

func agmPerformanceWorkloadSourcesAreConfigured() error {
	_, err := os.Stat(filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "performance", "eventbus_load_test.go"))
	return err
}

func agmValidatesPerformanceClientReadiness() error {
	return nil
}

func performanceWorkloadsUseBoundedHubClientReadiness() error {
	data, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "performance", "eventbus_load_test.go"))
	if err != nil {
		return err
	}
	source := string(data)
	for _, required := range []string{
		"func waitForClientCount(",
		"deadline := time.Now().Add(5 * time.Second)",
		"waitForClientCount(t, hub, 1)",
		"waitForClientCount(t, hub, numClients)",
	} {
		if !strings.Contains(source, required) {
			return fmt.Errorf("performance workload lacks bounded client readiness %q", required)
		}
	}
	return nil
}

func churnCleanupIsObservedBeforeStableClientsDisconnect() error {
	data, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "agm", "test", "performance", "eventbus_load_test.go"))
	if err != nil {
		return err
	}
	source := string(data)
	churnStart := strings.Index(source, "func TestConnectionChurn(")
	if churnStart < 0 {
		return fmt.Errorf("connection churn workload is missing")
	}
	churn := source[churnStart:]
	registered := strings.Index(churn, "waitForClientCount(t, hub, stableClients+1)")
	closed := strings.Index(churn, "ephConn.Close()")
	unregistered := -1
	if closed >= 0 {
		if relative := strings.Index(churn[closed:], "waitForClientCount(t, hub, stableClients)"); relative >= 0 {
			unregistered = closed + relative
		}
	}
	stableClosed := strings.Index(churn, "// Close stable connections to unblock ReadMessage in receiver goroutines.")
	if registered < 0 || closed < 0 || unregistered < 0 || stableClosed < 0 ||
		registered >= closed || closed >= unregistered || unregistered >= stableClosed {
		return fmt.Errorf("connection churn does not observe ephemeral registration and cleanup before stable disconnect")
	}
	return nil
}

func isolatedCodexLifecycleTestSourcesAreConfigured() error {
	root := packageSpecBDDRepoRoot()
	for _, path := range []string{
		"agm/test/integration/helpers/isolated_environment.go",
		"agm/test/integration/lifecycle/codex_isolated_lifecycle_test.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			return fmt.Errorf("isolated lifecycle source %s: %w", path, err)
		}
	}
	return nil
}

func agmValidatesRealLifecycleIsolation() error {
	return nil
}

func lifecycleUsesSourceBuiltAGMAndUniqueTmuxSocket() error {
	root := packageSpecBDDRepoRoot()
	helperData, err := os.ReadFile(filepath.Join(root, "agm", "test", "integration", "helpers", "isolated_environment.go"))
	if err != nil {
		return err
	}
	helper := string(helperData)
	for _, required := range []string{
		`testcontext.New()`, `"go", "build"`, `e.AGMBinary`,
		`"-S", e.TmuxSocket`, `SessionPrefix`,
	} {
		if !strings.Contains(helper, required) {
			return fmt.Errorf("isolated environment lacks source/socket guard %s", required)
		}
	}

	lifecycleData, err := os.ReadFile(filepath.Join(root, "agm", "test", "integration", "lifecycle", "codex_isolated_lifecycle_test.go"))
	if err != nil {
		return err
	}
	lifecycle := string(lifecycleData)
	for _, required := range []string{`NewIsolatedEnvironment(t)`, `env.Command(`, `env.StartTmuxServer(`} {
		if !strings.Contains(lifecycle, required) {
			return fmt.Errorf("codex lifecycle bypasses isolated environment guard %s", required)
		}
	}
	return nil
}

func cleanupTargetsOnlyOwnedTestResources() error {
	root := packageSpecBDDRepoRoot()
	data, err := os.ReadFile(filepath.Join(root, "agm", "test", "integration", "helpers", "isolated_environment.go"))
	if err != nil {
		return err
	}
	source := string(data)
	for _, required := range []string{`RegisterSession`, `e.owned`, `"kill-session", "-t", name`, `"kill-server"`} {
		if !strings.Contains(source, required) {
			return fmt.Errorf("isolated cleanup lacks exact ownership guard %s", required)
		}
	}
	for _, banned := range []string{`ListTmuxSessions(`, `"test-"`, `"agm-test-*"`} {
		if strings.Contains(source, banned) {
			return fmt.Errorf("isolated cleanup retains broad target %s", banned)
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
