package steps

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cucumber/godog"
)

type specGovernanceToolingStateKey struct{}

type specGovernanceToolingState struct {
	repoRoot string
	command  string
	output   string
	err      error
}

// RegisterSpecGovernanceToolingSteps registers executable SPEC tooling checks.
func RegisterSpecGovernanceToolingSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, specGovernanceToolingStateKey{}, &specGovernanceToolingState{repoRoot: packageSpecBDDRepoRoot()}), nil
	})
	ctx.Step(`^AGM exercises pinned SPEC inventory and strict extraction$`, exercisePinnedSPECInventory)
	ctx.Step(`^AGM exercises forged SPEC finding rejection$`, exerciseForgedSPECFindingRejection)
	ctx.Step(`^AGM exercises complete offline SPEC audit rendering$`, exerciseOfflineSPECAuditRendering)
	ctx.Step(`^AGM exercises the fixed SPEC skill export boundary$`, exerciseFixedSPECSkillExportBoundary)
	ctx.Step(`^AGM exercises fail-closed SPEC skill projection mutation$`, exerciseFailClosedSPECProjectionMutation)
	ctx.Step(`^AGM exercises strict SPEC governance package metadata$`, exerciseStrictSPECMetadata)
	ctx.Step(`^AGM exercises the installed SPEC audit execution seam$`, exerciseInstalledSPECAuditExecution)
	ctx.Step(`^AGM exercises bounded SPEC audit Git collection$`, exerciseBoundedSPECAuditGitCollection)
	ctx.Step(`^AGM runs the repository SPEC skill drift gate$`, runRepositorySPECDriftGate)
	ctx.Step(`^the SPEC governance behavioral contract should pass$`, specGovernanceBehaviorShouldPass)
}

func exercisePinnedSPECInventory(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/skills/audit-specs/scripts/specaudit", "TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically|TestParseSpec")
}

func exerciseForgedSPECFindingRejection(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/skills/audit-specs/scripts/specaudit", "TestValidateAuthenticatesFindingsAgainstPinnedGitInventory|TestAuthenticatedValidationRejectsForgedEvidenceAndUnsafeVerdicts")
}

func exerciseOfflineSPECAuditRendering(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/skills/audit-specs/scripts/specaudit", "TestRenderIsOfflineAndEscapesEvidence|TestCommandsRejectFilesystemOutputFlags")
}

func exerciseFixedSPECSkillExportBoundary(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/cmd/sync-skill-projections", "TestSyncWritesAndChecksProjections|TestSyncRejectsFullyDeclaredAdditionalCanonicalSkill|TestSyncFindsObsoleteGeneratedProjection")
}

func exerciseFailClosedSPECProjectionMutation(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/cmd/sync-skill-projections", "TestSyncRefuses|TestSyncNoClobber|TestSyncFailsClosed|TestSyncRequiresExplicitRemoval|TestSyncRejectsHardLinkedProjection|TestSyncRejectsUnexpectedProjectionPermissions|TestSyncRejectsOversizedProjectionTarget|TestSyncValidatesAllMetadataBeforeWriting|TestReadMetadataUsesStrictYAMLFrontmatter|TestRunWriteRejectsExplicitRoot|TestRequireLinkedWorktreeRoot")
}

func exerciseStrictSPECMetadata(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/cmd/sync-skill-projections", "TestSyncRejectsInvalidPackageMetadataBeforeWriting")
}

func exerciseInstalledSPECAuditExecution(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/skills/audit-specs/scripts/specaudit", "TestInstalledPluginRunsFromUnrelatedWorkingDirectory")
}

func exerciseBoundedSPECAuditGitCollection(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/skills/audit-specs/scripts/specaudit", "TestGitOutputIsBounded")
}

func runRepositorySPECDriftGate(ctx context.Context) error {
	state, err := getSpecGovernanceToolingState(ctx)
	if err != nil {
		return err
	}
	state.command = "make lint-skills"
	command := exec.Command("make", "lint-skills")
	command.Dir = state.repoRoot
	output, commandErr := command.CombinedOutput()
	state.output = string(output)
	state.err = commandErr
	return nil
}

func runSpecGovernanceGoTests(ctx context.Context, packagePath, pattern string) error {
	state, err := getSpecGovernanceToolingState(ctx)
	if err != nil {
		return err
	}
	const modulePrefix = "./spec-governance/"
	if !strings.HasPrefix(packagePath, modulePrefix) {
		return fmt.Errorf("SPEC governance package %q is outside its isolated module", packagePath)
	}
	relativePackage := "./" + strings.TrimPrefix(packagePath, modulePrefix)
	arguments := []string{"-C", "spec-governance", "test", relativePackage, "-run", pattern, "-count=1"}
	state.command = "go " + strings.Join(arguments, " ")
	command := exec.Command("go", arguments...)
	command.Dir = state.repoRoot
	output, commandErr := command.CombinedOutput()
	state.output = string(output)
	state.err = commandErr
	return nil
}

func specGovernanceBehaviorShouldPass(ctx context.Context) error {
	state, err := getSpecGovernanceToolingState(ctx)
	if err != nil {
		return err
	}
	if state.command == "" {
		return fmt.Errorf("SPEC governance command was not selected")
	}
	if state.err != nil {
		return fmt.Errorf("%s failed: %w\n%s", state.command, state.err, state.output)
	}
	return nil
}

func getSpecGovernanceToolingState(ctx context.Context) (*specGovernanceToolingState, error) {
	state, ok := ctx.Value(specGovernanceToolingStateKey{}).(*specGovernanceToolingState)
	if !ok || state == nil {
		return nil, fmt.Errorf("SPEC governance tooling state not initialized")
	}
	return state, nil
}
