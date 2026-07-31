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
	ctx.Step(`^AGM exercises dynamic SPEC skill projection discovery$`, exerciseDynamicSPECProjectionDiscovery)
	ctx.Step(`^AGM exercises safe SPEC skill projection replacement$`, exerciseSafeSPECProjectionReplacement)
	ctx.Step(`^AGM exercises strict SPEC governance package metadata$`, exerciseStrictSPECMetadata)
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

func exerciseDynamicSPECProjectionDiscovery(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/cmd/sync-skill-projections", "TestSyncWritesAndChecksProjections|TestSyncDiscoversNewAndObsoleteGeneratedSkills")
}

func exerciseSafeSPECProjectionReplacement(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/cmd/sync-skill-projections", "TestSyncRefuses|TestSyncValidatesAllMetadataBeforeWriting|TestReadMetadataUsesStrictYAMLFrontmatter|TestRunWriteRejectsExplicitRoot|TestRequireLinkedWorktreeRoot")
}

func exerciseStrictSPECMetadata(ctx context.Context) error {
	return runSpecGovernanceGoTests(ctx, "./spec-governance/cmd/sync-skill-projections", "TestSyncRejectsInvalidPackageMetadataBeforeWriting")
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
	state.command = strings.Join([]string{"go", "test", packagePath, "-run", pattern, "-count=1"}, " ")
	command := exec.Command("go", "test", packagePath, "-run", pattern, "-count=1")
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
