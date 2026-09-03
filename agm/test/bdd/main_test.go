package bdd

import (
	"testing"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/test/bdd/steps"
)

// TestFeatures runs every Gherkin scenario in features/. There is no tag
// filter on purpose: every direct-child feature file in this directory MUST
// run, while a separate invariant rejects nested feature files that Godog would
// otherwise discover recursively outside the flat catalog contract.
// A scenario whose steps are not implemented fails as "undefined" rather than
// being silently skipped, so dead/aspirational specs cannot accumulate. If you
// add a feature file, add its step definitions in the same change.
func TestFeatures(t *testing.T) {
	suite := featureTestSuite(t)
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

func featureTestSuite(t *testing.T) godog.TestSuite {
	return godog.TestSuite{
		TestSuiteInitializer: InitializeTestSuite,
		Options:              featureTestOptions(t),
	}
}

func featureTestOptions(t *testing.T) *godog.Options {
	return &godog.Options{
		Format:   "pretty",
		Paths:    []string{"features"},
		Strict:   true,
		TestingT: t,
	}
}

func TestFeatureRunnerUsesStrictUndefinedStepPolicy(t *testing.T) {
	if !featureTestOptions(t).Strict {
		t.Fatal("BDD runner must fail pending, undefined, and ambiguous steps")
	}
}

func TestFeatureRunnerRegistersDefinitionsAtSuiteScope(t *testing.T) {
	suite := featureTestSuite(t)
	if suite.TestSuiteInitializer == nil {
		t.Fatal("BDD runner must register definitions once on the base suite")
	}
	if suite.ScenarioInitializer != nil {
		t.Fatal("BDD runner must not rebuild every step regular expression per scenario")
	}
}

func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	// Godog copies this base suite for every scenario. Registering definitions
	// and Before hooks here preserves per-scenario state setup while compiling
	// the step expressions once instead of once for every scenario.
	RegisterScenarioDefinitions(ctx.ScenarioContext())
}

func RegisterScenarioDefinitions(ctx *godog.ScenarioContext) {
	// Each step group registers its own per-scenario Before hook to set up
	// state. Godog copies those hooks with the base suite and invokes them for
	// each scenario.
	steps.RegisterAGMControlSurfaceGuardrailSteps(ctx)
	steps.RegisterAGMConversationDiscoveryGuardrailSteps(ctx)
	steps.RegisterAGMCapacityPlatformSteps(ctx)
	steps.RegisterAGMRuntimePackageGuardrailSteps(ctx)
	steps.RegisterAgentSelectionGuardrailSteps(ctx)
	steps.RegisterAgentUtilityParitySteps(ctx)
	steps.RegisterAPIGatewayPackageGuardrailSteps(ctx)
	steps.RegisterAuditPackageGuardrailSteps(ctx)
	steps.RegisterCrossLanguageImplementationGuardrailSteps(ctx)
	steps.RegisterDangerousOverrideGovernanceSteps(ctx)
	steps.RegisterBDDRootPortabilitySteps(ctx)
	steps.RegisterAgenticReviewGateSteps(ctx)
	steps.RegisterAGMDiagnosticsPackageGuardrailSteps(ctx)
	steps.RegisterDBPersistenceGuardrailSteps(ctx)
	steps.RegisterDeveloperToolPackageGuardrailSteps(ctx)
	steps.RegisterDeclarativeRuntimeGuardrailSteps(ctx)
	steps.RegisterDeclarativeFixtureGuardrailSteps(ctx)
	steps.RegisterEngramAnalysisConfigurationGuardrailSteps(ctx)
	steps.RegisterEngramCoreContextGuardrailSteps(ctx)
	steps.RegisterEngramCLISupportGuardrailSteps(ctx)
	steps.RegisterEngramHookGuardrailSteps(ctx)
	steps.RegisterEngramGovernanceRuntimeGuardrailSteps(ctx)
	steps.RegisterEngramKnowledgeGuardrailSteps(ctx)
	steps.RegisterEngramObservabilityGuardrailSteps(ctx)
	steps.RegisterEvaluationControlParitySteps(ctx)
	steps.RegisterEngramSecurityTokenGuardrailSteps(ctx)
	steps.RegisterEngramReflectionStorageGuardrailSteps(ctx)
	steps.RegisterTrustProtocolSteps(ctx)
	steps.RegisterValidationWorkspaceParitySteps(ctx)
	steps.RegisterScanLoopSteps(ctx)
	steps.RegisterStallDetectionSteps(ctx)
	steps.RegisterTestSupportPackageGuardrailSteps(ctx)
	steps.RegisterAgySavedSessionDiscoverySteps(ctx)
	steps.RegisterHarnessParitySteps(ctx)
	steps.RegisterHarnessSpecGuardrailSteps(ctx)
	steps.RegisterHarnessConfigSurfaceGuardrailSteps(ctx)
	steps.RegisterInstructionParitySteps(ctx)
	steps.RegisterContextManagementParitySteps(ctx)
	steps.RegisterPiCustomContextSteps(ctx)
	steps.RegisterInternalFoundationGuardrailSteps(ctx)
	steps.RegisterHookParitySteps(ctx)
	steps.RegisterLLMRuntimeGuardrailSteps(ctx)
	steps.RegisterLegacySpecStrictnessGuardrailSteps(ctx)
	steps.RegisterMarkdownVisibilitySteps(ctx)
	steps.RegisterLegacyNFREARSGuardrailSteps(ctx)
	steps.RegisterLegacySpecBDDLinkageGuardrailSteps(ctx)
	steps.RegisterLocalDevelopmentGuardrailSteps(ctx)
	steps.RegisterCIHealthEscapeAnalysisSteps(ctx)
	steps.RegisterDocRefLintSteps(ctx)
	steps.RegisterModelFamilyParitySteps(ctx)
	steps.RegisterMCPCommandGuardrailSteps(ctx)
	steps.RegisterObservabilityPackageGuardrailSteps(ctx)
	steps.RegisterPluginSkillPackageGuardrailSteps(ctx)
	steps.RegisterAGMProductSurfaceGuardrailSteps(ctx)
	steps.RegisterQuotaMonitoringGuardrailSteps(ctx)
	steps.RegisterQualityCommandGuardrailSteps(ctx)
	steps.RegisterRootLifecycleCommandGuardrailSteps(ctx)
	steps.RegisterRootMaintenanceCommandGuardrailSteps(ctx)
	steps.RegisterRootSafetyCommandGuardrailSteps(ctx)
	steps.RegisterRootIntelligenceCommandGuardrailSteps(ctx)
	steps.RegisterRootOperationsCommandGuardrailSteps(ctx)
	steps.RegisterSandboxProviderGuardrailSteps(ctx)
	steps.RegisterSharedRuntimePolicyGuardrailSteps(ctx)
	steps.RegisterSessionProtocolGuardrailSteps(ctx)
	steps.RegisterAGMSupervisionRecoveryGuardrailSteps(ctx)
	steps.RegisterSpecCoverageSteps(ctx)
	steps.RegisterSpecGuardSteps(ctx)
	steps.RegisterSpecGovernanceToolingSteps(ctx)
	steps.RegisterSourceKnowledgePackageGuardrailSteps(ctx)
	steps.RegisterWorkflowCommandGuardrailSteps(ctx)
	steps.RegisterWorkflowPackageGuardrailSteps(ctx)
	steps.RegisterWorkflowToolingGuardrailSteps(ctx)
	steps.RegisterWayfinderLifecycleGuardrailSteps(ctx)
	steps.RegisterWayfinderInternalPackageGuardrailSteps(ctx)
	steps.RegisterWayfinderStatusGuardrailSteps(ctx)
	steps.RegisterWayfinderV2CommandGuardrailSteps(ctx)
	steps.RegisterVROOMRuntimeGuardrailSteps(ctx)
}
