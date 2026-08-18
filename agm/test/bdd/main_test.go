package bdd

import (
	"testing"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/test/bdd/steps"
)

// TestFeatures runs every Gherkin scenario under features/. There is no tag
// filter on purpose: any feature file that exists in this directory MUST run.
// A scenario whose steps are not implemented fails as "undefined" rather than
// being silently skipped, so dead/aspirational specs cannot accumulate. If you
// add a feature file, add its step definitions in the same change.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options:             featureTestOptions(t),
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
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

func InitializeScenario(ctx *godog.ScenarioContext) {
	// Each step group registers its own per-scenario Before hook to set up
	// state, so there is no shared environment to wire here.
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
