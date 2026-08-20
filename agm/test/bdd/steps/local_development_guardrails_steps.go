package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/procguard"
	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/safepr"
	wayfindersandbox "github.com/vbonnet/dear-agent/wayfinder/pkg/sandbox"
)

type localDevGuardrailState struct {
	command                string
	commandSpec            string
	library                string
	librarySpec            string
	traceDir               string
	trace                  safepr.Session
	harness                string
	family                 string
	preflightMinutes       int
	localTestTimeout       string
	affectedTestTimeout    string
	ciTestTimeout          string
	affectedPackageMins    int
	affectedListMins       int
	affectedStartupMins    int
	affectedCommandMins    int
	affectedJobMins        int
	localVulnAllowlist     []string
	ciVulnAllowlist        []string
	worktreeBase           string
	worktreeRepo           string
	worktreePath           string
	initialLockReason      string
	transactionOutcome     string
	transactionErr         error
	lockedInPreflight      bool
	lockedInPRCreate       bool
	wayfinderCleanupErr    error
	worktreePreserved      bool
	cleanupRegression      string
	cleanupRegressionErr   error
	childRegression        string
	childRegressionErr     error
	auditRegression        string
	auditRegressionErr     error
	noMergeRegression      string
	noMergeRegressionErr   error
	sessionLeakRegression  string
	sessionLeakErr         error
	treeRegression         string
	treeRegressionErr      error
	cleanupWTRegression    string
	cleanupWTRegressionErr error
	requiredCIRegression   string
	requiredCIError        error
	mergeLoopCIRegression  string
	mergeLoopCIError       error
	raceSkippedSLAs        []string
	ordinarySLAPackages    []string
	ordinarySLASanitized   bool
}

type localDevGuardrailStateKey struct{}

// RegisterLocalDevelopmentGuardrailSteps registers BDD steps for audited local development wrappers.
func RegisterLocalDevelopmentGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, localDevGuardrailStateKey{}, &localDevGuardrailState{}), nil
	})
	ctx.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		state, err := getLocalDevGuardrailState(ctx)
		if err == nil && state.traceDir != "" {
			if removeErr := os.RemoveAll(state.traceDir); removeErr != nil && scenarioErr == nil {
				return ctx, fmt.Errorf("remove canonical trace directory: %w", removeErr)
			}
		}
		if err == nil && state.worktreeBase != "" {
			if cleanupErr := cleanupSafePRBDDWorktree(state); cleanupErr != nil && scenarioErr == nil {
				return ctx, cleanupErr
			}
		}
		return ctx, nil
	})

	ctx.Step(`^safe local development command "([^"]*)" is configured$`, safeLocalDevelopmentCommandIsConfigured)
	ctx.Step(`^AGM validates safe local development command coverage$`, agmValidatesSafeLocalDevelopmentCommandCoverage)
	ctx.Step(`^safe local development command "([^"]*)" should have a co-located SPEC$`, safeLocalDevelopmentCommandShouldHaveCoLocatedSPEC)
	ctx.Step(`^safe local development library "([^"]*)" is configured$`, safeLocalDevelopmentLibraryIsConfigured)
	ctx.Step(`^AGM validates safe local development library coverage$`, agmValidatesSafeLocalDevelopmentLibraryCoverage)
	ctx.Step(`^safe local development library "([^"]*)" should have a co-located SPEC$`, safeLocalDevelopmentLibraryShouldHaveCoLocatedSPEC)
	ctx.Step(`^canonical Wayfinder V2 status for harness "([^"]*)" and model family "([^"]*)"$`, canonicalWayfinderV2StatusForRoute)
	ctx.Step(`^safe-pr loads the canonical planning trace$`, safePRLoadsCanonicalPlanningTrace)
	ctx.Step(`^safe-pr should attribute the trace to project "([^"]*)"$`, safePRShouldAttributeTraceToProject)
	ctx.Step(`^the safe-pr full preflight timeout is configured$`, safePRFullPreflightTimeoutIsConfigured)
	ctx.Step(`^AGM validates the safe-pr preflight budget$`, agmValidatesSafePRPreflightBudget)
	ctx.Step(`^safe-pr should allow at least (\d+) minutes for preflight-full$`, safePRShouldAllowAtLeastMinutesForPreflightFull)
	ctx.Step(`^the full preflight ordinary performance gate is configured$`, fullPreflightOrdinaryPerformanceGateIsConfigured)
	ctx.Step(`^AGM validates race-skipped wall-clock SLA coverage$`, agmValidatesRaceSkippedWallClockSLACoverage)
	ctx.Step(`^every race-skipped SLA package should run without inherited test modes or race instrumentation$`, everyRaceSkippedSLAPackageShouldRunWithoutRaceInstrumentation)
	ctx.Step(`^a safe-pr linked worktree with "([^"]*)" lock ownership$`, safePRLinkedWorktreeWithLockOwnership)
	ctx.Step(`^safe-pr protects a "([^"]*)" preflight and PR creation transaction$`, safePRProtectsTransaction)
	ctx.Step(`^the worktree should be protected during preflight and PR creation$`, worktreeShouldBeProtectedDuringTransaction)
	ctx.Step(`^the "([^"]*)" lock ownership should remain after the transaction$`, lockOwnershipShouldRemainAfterTransaction)
	ctx.Step(`^Wayfinder cleanup overlaps a protected safe-pr transaction$`, wayfinderCleanupOverlapsProtectedTransaction)
	ctx.Step(`^Wayfinder should preserve the protected worktree and reject cleanup$`, wayfinderShouldPreserveProtectedWorktree)
	ctx.Step(`^Wayfinder should remove the worktree after the safe-pr transaction$`, wayfinderShouldRemoveWorktreeAfterTransaction)
	ctx.Step(`^AGM runs the protected cleanup regressions$`, agmRunsProtectedCleanupRegressions)
	ctx.Step(`^AGM runs the cleanup-worktrees classification regressions$`, agmRunsCleanupWorktreesRegressions)
	ctx.Step(`^stale-worktree cleanup should refuse dirty and active worktrees and fail closed on probe errors$`, cleanupWorktreesShouldClassifyPreserveAndRemove)
	ctx.Step(`^Wayfinder and AGM cleanup should preserve Git-locked checkouts$`, cleanupShouldPreserveGitLockedCheckouts)
	registerSafePRRegressionGuardrailSteps(ctx)
	ctx.Step(`^AGM runs the affected runner process-tree regressions$`, agmRunsAffectedRunnerProcessTreeRegressions)
	ctx.Step(`^bounded affected runner commands should terminate their descendants$`, boundedAffectedRunnerCommandsShouldTerminateTheirDescendants)
	ctx.Step(`^AGM runs the affected runner fixture regressions$`, agmRunsAffectedRunnerFixtureRegressions)
	ctx.Step(`^partial readiness, early completion, and setup timeout should be distinguished$`, affectedRunnerFixturesShouldDistinguishSetupOutcomes)
	ctx.Step(`^AGM runs the effective required-check regressions$`, agmRunsEffectiveRequiredCheckRegressions)
	ctx.Step(`^safe-merge should enforce complete provider-required CI without advisory drift$`, safeMergeShouldEnforceProviderRequiredCI)
	ctx.Step(`^local, affected integration, and required CI Go test timeouts are configured$`, repositoryGoTestTimeoutsAreConfigured)
	ctx.Step(`^AGM validates Go test timeout parity$`, agmValidatesGoTestTimeoutParity)
	ctx.Step(`^all repository Go test timeouts should match$`, repositoryGoTestTimeoutsShouldMatch)
	ctx.Step(`^affected integration deadline layers should preserve their nested budgets$`, affectedIntegrationDeadlineLayersShouldPreserveTheirNestedBudgets)
	ctx.Step(`^local and required CI govulncheck allowlists are configured$`, localAndRequiredCIGovulncheckAllowlistsAreConfigured)
	ctx.Step(`^AGM validates govulncheck policy parity$`, agmValidatesGovulncheckPolicyParity)
	ctx.Step(`^the local and required CI govulncheck allowlists should match$`, localAndRequiredCIGovulncheckAllowlistsShouldMatch)
	ctx.Step(`^local preflight should resolve configured GOBIN and first-GOPATH Go tool installs outside PATH$`, localPreflightShouldResolveStandardGoToolInstallsOutsidePATH)
}

func safePRLinkedWorktreeWithLockOwnership(ctx context.Context, initial string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	initialReason, err := safePRBDDInitialLockReason(initial)
	if err != nil {
		return err
	}
	base, err := os.MkdirTemp("", "safe-pr-worktree-bdd-*")
	if err != nil {
		return err
	}
	state.worktreeBase = base
	state.worktreeRepo = filepath.Join(base, "repo")
	state.worktreePath = filepath.Join(state.worktreeRepo, ".worktrees", "safe-pr-bdd")
	state.initialLockReason = initialReason
	for _, args := range [][]string{
		{"init", "-q", "-b", "main", state.worktreeRepo},
		{"-C", state.worktreeRepo, "config", "user.name", "Safe PR BDD"},
		{"-C", state.worktreeRepo, "config", "user.email", "safe-pr-bdd@example.invalid"},
	} {
		if _, err := runSafePRBDDGit(ctx, args...); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(state.worktreeRepo, "README.md"), []byte("safe-pr BDD\n"), 0o600); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"-C", state.worktreeRepo, "add", "README.md"},
		{"-C", state.worktreeRepo, "commit", "-q", "-m", "initial"},
		{"-C", state.worktreeRepo, "worktree", "add", "-q", "-b", "safe-pr-bdd", state.worktreePath},
	} {
		if _, err := runSafePRBDDGit(ctx, args...); err != nil {
			return err
		}
	}
	if state.initialLockReason != "" {
		_, err = runSafePRBDDGit(ctx, "-C", state.worktreeRepo, "worktree", "lock", "--reason", state.initialLockReason, state.worktreePath)
	}
	return err
}

func wayfinderCleanupOverlapsProtectedTransaction(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.transactionErr = safepr.WithWorktreeLock(state.worktreePath, "BDD cleanup overlap", func() error {
		state.wayfinderCleanupErr = wayfindersandbox.NewGitWorktreeManager().RemoveWorktree("safe-pr-bdd", state.worktreeRepo)
		_, statErr := os.Stat(state.worktreePath)
		state.worktreePreserved = statErr == nil
		return nil
	})
	return state.transactionErr
}

func wayfinderShouldPreserveProtectedWorktree(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.wayfinderCleanupErr == nil {
		return errors.New("wayfinder cleanup unexpectedly succeeded for a protected worktree")
	}
	if !strings.Contains(state.wayfinderCleanupErr.Error(), "preserving") {
		return fmt.Errorf("wayfinder cleanup: %w; want protected-worktree rejection", state.wayfinderCleanupErr)
	}
	if !state.worktreePreserved {
		return errors.New("wayfinder removed the safe-pr-protected worktree")
	}
	return nil
}

func wayfinderShouldRemoveWorktreeAfterTransaction(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if err := wayfindersandbox.NewGitWorktreeManager().RemoveWorktree("safe-pr-bdd", state.worktreeRepo); err != nil {
		return fmt.Errorf("wayfinder cleanup after safe-pr release: %w", err)
	}
	if _, err := os.Stat(state.worktreePath); err == nil {
		return errors.New("wayfinder worktree still exists after cleanup")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("wayfinder worktree after cleanup: %w; want not exist", err)
	}
	return nil
}

func agmRunsProtectedCleanupRegressions(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.cleanupRegression, state.cleanupRegressionErr = runLocalGuardrailGoTest(ctx,
		`^(TestRemoveOrphanWorktreePreservesGitLockedCheckout|TestCleanupSandboxPreservesLockedWorktreeAndMetadata)$`,
		"./agm/cmd/agm", "./wayfinder/pkg/sandbox",
	)
	return nil
}

func cleanupShouldPreserveGitLockedCheckouts(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.cleanupRegressionErr != nil {
		return fmt.Errorf("protected cleanup regressions: %w: %s", state.cleanupRegressionErr, state.cleanupRegression)
	}
	return nil
}

func agmRunsCleanupWorktreesRegressions(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.cleanupWTRegression, state.cleanupWTRegressionErr = runLocalGuardrailNamedGoTests(ctx,
		"./cmd/cleanup-worktrees",
		"TestParse",
		"TestInspectClassification",
		"TestInspectRefusesDirtyWorktree",
		"TestInspectRefusesActiveSessionWorktree",
		"TestInspectFailsClosedOnProbeFailure",
		"TestInspectFixRemovesStaleWorktreeAndBranch",
		"TestGitIntReportsFailureInsteadOfZero",
		"TestGitEnvScrubsAmbientRepositorySelectors",
		"TestListWorktreesParsesPorcelain",
		"TestParseWorktreesKeepsNewlineBearingPaths",
		"TestParseWorktreesMarksLocked",
		"TestTargetRefPrefersOriginMain",
		"TestRunRejectsNonGitDirectory",
	)
	return nil
}

func cleanupWorktreesShouldClassifyPreserveAndRemove(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.cleanupWTRegressionErr != nil {
		return fmt.Errorf("cleanup-worktrees classification regressions: %w: %s", state.cleanupWTRegressionErr, state.cleanupWTRegression)
	}
	return nil
}

func agmRunsEffectiveRequiredCheckRegressions(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.requiredCIRegression, state.requiredCIError = runLocalGuardrailGoTest(ctx,
		`^Test(ParseAppliedRulesRequiredChecks|ParseAppliedRulesRequiredChecksKnownEmpty|ParseAppliedRulesRequiredChecksFlagsRequiredWorkflows|ParseClassicRequiredChecksPreservesIntegrationScope|MergeRequiredCheckPoliciesUnionsLayeredSources|DiscoverRequiredChecksAcceptsAuthoritativeEmpty|DiscoverRequiredChecksRejectsPartialPolicyOnSourceError|DiscoverRequiredChecksUsesPaginatedSlurp|RulesBranchEndpointEscapesSlashBase|ProviderRequiredClassificationIgnoresAdvisoryFailure|ProviderRequiredClassificationBlocksRequiredFailurePendingAndMissing|ProviderRequiredClassificationRejectsAmbiguousIntegrationIdentity|ProviderRequiredClassificationRejectsDiscoveryDisagreement|ProjectRequiredChecksReconcilesEffectivePolicy|ProjectRequiredChecksSynthesizesMissingContext|ProjectRequiredChecksAcceptsStatusExits|ProjectRequiredChecksAcceptsAuthoritativeEmptyProviderError|ProjectRequiredChecksPreservesAuthoritativeEmptyFallback|ProjectRequiredChecksRejectsNoChecksWhenPolicyNonempty|CheckAllCIAcceptsNoChecksWhenPolicyEmpty|CheckAllCIValidatesAllWhenPolicyEmpty|CheckAllCIIgnoresNonzeroAdvisoryCheckStatus)$`,
		"./internal/safegit",
	)
	cmdOutput, cmdErr := runLocalGuardrailNamedGoTests(ctx, "./cmd/mergeloop",
		"TestMergeLoopUsesSharedRequiredProjection",
		"TestMergeLoopPreservesAuthoritativeEmptyFallback",
		"TestMergeLoopMapsProjectedRequiredStatuses",
		"TestMergeLoopDefersOnlyUnavailableProjection",
		"TestMergeLoopDefersOnlyUnknownProjectedStatus",
		"TestMergeLoopAbortsWhenParentContextCanceled",
		"TestMergeLoopSkipsProjectionWhenOpenPRsExceedCap",
	)
	internalOutput, internalErr := runLocalGuardrailNamedGoTests(ctx, "./internal/mergeloop",
		"TestProjectionErrorPreservesAttemptBudget",
	)
	state.mergeLoopCIRegression = strings.Join([]string{cmdOutput, internalOutput}, "\n")
	state.mergeLoopCIError = errors.Join(cmdErr, internalErr)
	return nil
}

func safeMergeShouldEnforceProviderRequiredCI(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.requiredCIError != nil {
		return fmt.Errorf("safegit effective required-check regressions: %w: %s", state.requiredCIError, state.requiredCIRegression)
	}
	if state.mergeLoopCIError != nil {
		return fmt.Errorf("mergeloop effective required-check regressions: %w: %s", state.mergeLoopCIError, state.mergeLoopCIRegression)
	}
	return nil
}

func runLocalGuardrailGoTest(parent context.Context, pattern string, packages ...string) (string, error) {
	return runLocalGuardrailGoTestWith(parent, nil, pattern, packages...)
}

func runLocalGuardrailNamedGoTests(parent context.Context, packagePath string, testNames ...string) (string, error) {
	patterns := make([]string, 0, len(testNames))
	for _, testName := range testNames {
		patterns = append(patterns, regexp.QuoteMeta(testName))
	}
	pattern := "^(" + strings.Join(patterns, "|") + ")$"
	output, err := runLocalGuardrailGoTest(parent, pattern, packagePath)
	if err != nil {
		return output, err
	}
	if missing := missingNamedGoTestRuns(output, testNames...); len(missing) > 0 {
		return output, fmt.Errorf("go test package %q did not run named regressions: %s", packagePath, strings.Join(missing, ", "))
	}
	return output, nil
}

func missingNamedGoTestRuns(output string, testNames ...string) []string {
	ran := make(map[string]struct{}, len(testNames))
	for line := range strings.SplitSeq(output, "\n") {
		if testName, ok := strings.CutPrefix(strings.TrimSuffix(line, "\r"), "=== RUN   "); ok {
			ran[testName] = struct{}{}
		}
	}

	missing := make([]string, 0, len(testNames))
	for _, testName := range testNames {
		if _, ok := ran[testName]; !ok {
			missing = append(missing, testName)
		}
	}
	return missing
}

// runLocalGuardrailGoTestWith is runLocalGuardrailGoTest with extra `go test`
// flags. Its output is always verbose so BDD assertions can prove that the
// intended named regression ran instead of accepting a vacuous zero-match run.
func runLocalGuardrailGoTestWith(parent context.Context, extraArgs []string, pattern string, packages ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	commandArgs := []string{"test", "-v", "-count=1", "-timeout=90s", "-run", pattern}
	commandArgs = append(commandArgs, extraArgs...)
	commandArgs = append(commandArgs, packages...)
	cmd := exec.CommandContext(ctx, "go", commandArgs...)
	cmd.Dir = localDevBDDRepoRoot()
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return string(out), fmt.Errorf("go test timed out after 2m")
	}
	if err == nil && !strings.Contains(string(out), "=== RUN") {
		return string(out), fmt.Errorf("go test pattern %q did not run a named regression", pattern)
	}
	return string(out), err
}

func safePRBDDInitialLockReason(initial string) (string, error) {
	switch initial {
	case "absent":
		return "", nil
	case "pre-existing":
		return "bdd-existing-owner", nil
	case "stale-safe-pr":
		return "safe-pr-owned:2147483647:0011223344556677:stale BDD transaction", nil
	default:
		return "", fmt.Errorf("unsupported initial worktree lock %q", initial)
	}
}

func safePRProtectsTransaction(ctx context.Context, outcome string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if outcome != "success" && outcome != "failure" {
		return fmt.Errorf("unsupported safe-pr transaction outcome %q", outcome)
	}
	state.transactionOutcome = outcome
	state.transactionErr = safepr.WithWorktreeLock(state.worktreePath, "BDD safe-pr create", func() error {
		locked, _, inspectErr := safePRBDDLockState(ctx, state)
		if inspectErr != nil {
			return inspectErr
		}
		state.lockedInPreflight = locked
		locked, _, inspectErr = safePRBDDLockState(ctx, state)
		if inspectErr != nil {
			return inspectErr
		}
		state.lockedInPRCreate = locked
		if outcome == "failure" {
			return errors.New("simulated PR creation failure")
		}
		return nil
	})
	return nil
}

func worktreeShouldBeProtectedDuringTransaction(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if !state.lockedInPreflight || !state.lockedInPRCreate {
		return fmt.Errorf("worktree protection = preflight:%t PR-create:%t", state.lockedInPreflight, state.lockedInPRCreate)
	}
	if state.transactionOutcome == "success" && state.transactionErr != nil {
		return fmt.Errorf("successful transaction returned %w", state.transactionErr)
	}
	if state.transactionOutcome == "failure" && state.transactionErr == nil {
		return errors.New("failed transaction returned no error")
	}
	if state.transactionOutcome == "failure" && !strings.Contains(state.transactionErr.Error(), "simulated PR creation failure") {
		return fmt.Errorf("failed transaction error: %w", state.transactionErr)
	}
	return nil
}

func lockOwnershipShouldRemainAfterTransaction(ctx context.Context, final string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	locked, reason, err := safePRBDDLockState(ctx, state)
	if err != nil {
		return err
	}
	switch final {
	case "absent":
		if locked {
			return fmt.Errorf("safe-pr owned lock survived transaction with reason %q", reason)
		}
	case "pre-existing":
		if !locked || reason != "bdd-existing-owner" {
			return fmt.Errorf("pre-existing lock after transaction = locked:%t reason:%q", locked, reason)
		}
	default:
		return fmt.Errorf("unsupported final worktree lock %q", final)
	}
	return nil
}

func safePRBDDLockState(ctx context.Context, state *localDevGuardrailState) (bool, string, error) {
	out, err := runSafePRBDDGit(ctx, "-C", state.worktreeRepo, "worktree", "list", "--porcelain")
	if err != nil {
		return false, "", err
	}
	return parseSafePRBDDLockState(state.worktreePath, out)
}

func parseSafePRBDDLockState(worktreePath, out string) (bool, string, error) {
	out = strings.ReplaceAll(out, "\r", "")
	want := canonicalSafePRBDDPath(worktreePath)
	for record := range strings.SplitSeq(strings.TrimSpace(out), "\n\n") {
		path := ""
		locked := false
		reason := ""
		for line := range strings.SplitSeq(record, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				path = canonicalSafePRBDDPath(strings.TrimPrefix(line, "worktree "))
			case line == "locked":
				locked = true
			case strings.HasPrefix(line, "locked "):
				locked = true
				reason = strings.TrimPrefix(line, "locked ")
			}
		}
		if path == want {
			return locked, reason, nil
		}
	}
	return false, "", fmt.Errorf("BDD worktree %s is not registered", worktreePath)
}

func cleanupSafePRBDDWorktree(state *localDevGuardrailState) error {
	if _, err := os.Stat(state.worktreePath); os.IsNotExist(err) {
		return os.RemoveAll(state.worktreeBase)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	locked, _, inspectErr := safePRBDDLockState(ctx, state)
	var unlockErr error
	if inspectErr == nil && locked {
		_, unlockErr = runSafePRBDDGit(ctx, "-C", state.worktreeRepo, "worktree", "unlock", state.worktreePath)
	}
	_, removeErr := runSafePRBDDGit(ctx, "-C", state.worktreeRepo, "worktree", "remove", "--force", state.worktreePath)
	return errors.Join(inspectErr, unlockErr, removeErr, os.RemoveAll(state.worktreeBase))
}

func canonicalSafePRBDDPath(path string) string {
	path = filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(resolved)
	}
	return path
}

func runSafePRBDDGit(parent context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	cmd := gittest.CommandContext(ctx, "", args...)
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func repositoryGoTestTimeoutsAreConfigured(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	root := localDevBDDRepoRoot()
	local, err := os.ReadFile(filepath.Join(root, "scripts", "preflight.sh"))
	if err != nil {
		return fmt.Errorf("read local preflight: %w", err)
	}
	ci, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		return fmt.Errorf("read required CI workflow: %w", err)
	}
	affected, err := os.ReadFile(filepath.Join(root, "cmd", "test-affected", "main.go"))
	if err != nil {
		return fmt.Errorf("read affected integration test runner: %w", err)
	}
	localMatch := regexp.MustCompile(`(?m)^TEST_TIMEOUT="([^"]+)"$`).FindSubmatch(local)
	if len(localMatch) != 2 {
		return fmt.Errorf("local Go test timeout declaration not found")
	}
	ciMatch := regexp.MustCompile(`go test[^\n]*-timeout=([^\s]+)`).FindSubmatch(ci)
	if len(ciMatch) != 2 {
		return fmt.Errorf("required CI Go test timeout declaration not found")
	}
	affectedMatch := regexp.MustCompile(`(?m)^\s*goTestTimeout\s*=\s*(\d+)\s*\*\s*time\.Minute$`).FindSubmatch(affected)
	if len(affectedMatch) != 2 {
		return fmt.Errorf("affected integration Go test timeout declaration not found")
	}
	commandMatch := regexp.MustCompile(`(?m)^\s*goCommandTimeout\s*=\s*(\d+)\s*\*\s*time\.Minute$`).FindSubmatch(affected)
	if len(commandMatch) != 2 {
		return fmt.Errorf("affected integration process timeout declaration not found")
	}
	listMatch := regexp.MustCompile(`(?m)^\s*goListCommandTimeout\s*=\s*(\d+)\s*\*\s*time\.Minute$`).FindSubmatch(affected)
	if len(listMatch) != 2 {
		return fmt.Errorf("affected integration package-discovery timeout declaration not found")
	}
	jobMinutes, err := workflowJobTimeoutMinutes(ci, "integration-tests")
	if err != nil {
		return fmt.Errorf("affected integration CI job timeout: %w", err)
	}
	nativeMinutes, err := strconv.Atoi(string(affectedMatch[1]))
	if err != nil {
		return fmt.Errorf("parse affected integration native timeout: %w", err)
	}
	commandMinutes, err := strconv.Atoi(string(commandMatch[1]))
	if err != nil {
		return fmt.Errorf("parse affected integration process timeout: %w", err)
	}
	listMinutes, err := strconv.Atoi(string(listMatch[1]))
	if err != nil {
		return fmt.Errorf("parse affected integration package-discovery timeout: %w", err)
	}
	if commandMinutes < 2*nativeMinutes {
		return fmt.Errorf("affected integration process timeout lacks one native interval of build headroom")
	}
	if jobMinutes < listMinutes+commandMinutes+nativeMinutes {
		return fmt.Errorf("affected integration CI job timeout lacks one native interval beyond all bounded runner phases")
	}
	if !strings.Contains(string(affected), `"-timeout=" + goTestTimeout.String()`) {
		return fmt.Errorf("affected integration runner does not pass its timeout to the Go test binary")
	}
	if !strings.Contains(string(affected), `goCommandTimeoutExitCode = 124`) {
		return fmt.Errorf("affected integration runner does not preserve the timeout exit-code contract")
	}
	state.localTestTimeout = string(localMatch[1])
	state.affectedTestTimeout = string(affectedMatch[1]) + "m"
	state.ciTestTimeout = string(ciMatch[1])
	return nil
}

func workflowJobTimeoutMinutes(workflow []byte, jobName string) (int, error) {
	lines := strings.Split(string(workflow), "\n")
	header := "  " + jobName + ":"
	start := -1
	for i, line := range lines {
		if line == header {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return 0, fmt.Errorf("workflow job %q not found", jobName)
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") &&
			strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "#") {
			end = i
			break
		}
	}

	block := strings.Join(lines[start:end], "\n")
	match := regexp.MustCompile(`(?m)^    timeout-minutes:\s*(\d+)\s*$`).FindStringSubmatch(block)
	if len(match) != 2 {
		return 0, fmt.Errorf("workflow job %q has no explicit timeout-minutes", jobName)
	}
	minutes, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("parse workflow job %q timeout: %w", jobName, err)
	}
	return minutes, nil
}

func agmValidatesGoTestTimeoutParity(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.localTestTimeout == "" || state.affectedTestTimeout == "" || state.ciTestTimeout == "" {
		return fmt.Errorf("repository Go test timeouts are not configured")
	}
	return nil
}

func repositoryGoTestTimeoutsShouldMatch(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.localTestTimeout != state.ciTestTimeout || state.affectedTestTimeout != state.ciTestTimeout {
		return fmt.Errorf(
			"repository Go test timeouts differ: local=%q affected-integration=%q required-CI=%q",
			state.localTestTimeout,
			state.affectedTestTimeout,
			state.ciTestTimeout,
		)
	}
	return nil
}

func affectedIntegrationDeadlineLayersShouldPreserveTheirNestedBudgets(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.affectedTestTimeout == "" || state.ciTestTimeout == "" {
		return fmt.Errorf("affected integration timeout state is not initialized")
	}
	affected, err := os.ReadFile(filepath.Join(localDevBDDRepoRoot(), "cmd", "test-affected", "main.go"))
	if err != nil {
		return fmt.Errorf("read affected integration runner: %w", err)
	}
	source := string(affected)
	if !regexp.MustCompile(`context\.WithTimeout\((?:context\.Background\(\)|ctx|parent),\s*goCommandTimeout\)`).MatchString(source) {
		return fmt.Errorf("affected test command timeout is not wired through context.WithTimeout")
	}
	if !regexp.MustCompile(`context\.WithTimeout\((?:context\.Background\(\)|ctx|parent),\s*goListCommandTimeout\)`).MatchString(source) {
		return fmt.Errorf("affected package discovery timeout is not wired through context.WithTimeout")
	}
	return nil
}

func localAndRequiredCIGovulncheckAllowlistsAreConfigured(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	root := localDevBDDRepoRoot()
	state.localVulnAllowlist, err = readGovulncheckAllowlist(filepath.Join(root, "scripts", "preflight.sh"))
	if err != nil {
		return fmt.Errorf("local govulncheck allowlist: %w", err)
	}
	state.ciVulnAllowlist, err = readGovulncheckAllowlist(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		return fmt.Errorf("required CI govulncheck allowlist: %w", err)
	}
	return nil
}

func readGovulncheckAllowlist(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	match := regexp.MustCompile(`--argjson allow '(\[[^']+\])'`).FindSubmatch(content)
	if len(match) != 2 {
		return nil, fmt.Errorf("allowlist declaration not found in %s", path)
	}
	var ids []string
	if err := json.Unmarshal(match[1], &ids); err != nil {
		return nil, fmt.Errorf("parse allowlist in %s: %w", path, err)
	}
	return ids, nil
}

func agmValidatesGovulncheckPolicyParity(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if len(state.localVulnAllowlist) == 0 || len(state.ciVulnAllowlist) == 0 {
		return fmt.Errorf("govulncheck allowlists are not configured")
	}
	return nil
}

func localAndRequiredCIGovulncheckAllowlistsShouldMatch(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if !slices.Equal(state.localVulnAllowlist, state.ciVulnAllowlist) {
		return fmt.Errorf("local govulncheck allowlist %v does not match required CI %v", state.localVulnAllowlist, state.ciVulnAllowlist)
	}
	return nil
}

func localPreflightShouldResolveStandardGoToolInstallsOutsidePATH(ctx context.Context) error {
	if _, err := getLocalDevGuardrailState(ctx); err != nil {
		return err
	}
	sourcePath := filepath.Join(localDevBDDRepoRoot(), "scripts", "preflight.sh")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read local preflight: %w", err)
	}
	return validateGoToolInstallResolution(string(source))
}

func validateGoToolInstallResolution(source string) error {
	requiredInOrder := []string{
		`GOVULNCHECK_BIN="$(command -v govulncheck || true)"`,
		`GO_TOOL_INSTALL_BIN="$(go env GOBIN)"`,
		`if [[ -z "$GO_TOOL_INSTALL_BIN" ]]; then`,
		`GOPATH_VALUE="$(go env GOPATH)"`,
		`GO_TOOL_INSTALL_BIN="${GOPATH_VALUE%%:*}/bin"`,
		`GOVULNCHECK_CANDIDATE="$GO_TOOL_INSTALL_BIN/govulncheck"`,
		`[[ -x "$GOVULNCHECK_CANDIDATE" ]]`,
		`GOVULNCHECK_BIN="$GOVULNCHECK_CANDIDATE"`,
		`"$GOVULNCHECK_BIN" -format json -scan package ./...`,
	}
	previous := -1
	for _, required := range requiredInOrder {
		index := strings.Index(source, required)
		if index < 0 {
			return fmt.Errorf("local preflight is missing govulncheck resolution contract %q", required)
		}
		if index <= previous {
			return fmt.Errorf("local preflight govulncheck resolution contract is out of order at %q", required)
		}
		previous = index
	}
	return nil
}

func safePRFullPreflightTimeoutIsConfigured(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	sourcePath := filepath.Join(localDevBDDRepoRoot(), "cmd", "safe-pr", "main.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read safe-pr source: %w", err)
	}
	match := regexp.MustCompile(`preflightTimeout\s*=\s*(\d+)\s*\*\s*time\.Minute`).FindSubmatch(source)
	if len(match) != 2 {
		return fmt.Errorf("safe-pr preflight timeout declaration not found")
	}
	state.preflightMinutes, err = strconv.Atoi(string(match[1]))
	if err != nil {
		return fmt.Errorf("parse safe-pr preflight timeout: %w", err)
	}
	return nil
}

func agmValidatesSafePRPreflightBudget(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.preflightMinutes <= 0 {
		return fmt.Errorf("safe-pr preflight timeout is not configured")
	}
	return nil
}

func safePRShouldAllowAtLeastMinutesForPreflightFull(ctx context.Context, minimum int) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.preflightMinutes < minimum {
		return fmt.Errorf("safe-pr preflight timeout = %dm, want at least %dm", state.preflightMinutes, minimum)
	}
	return nil
}

func fullPreflightOrdinaryPerformanceGateIsConfigured(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	root := localDevBDDRepoRoot()
	preflight, err := os.ReadFile(filepath.Join(root, "scripts", "preflight.sh"))
	if err != nil {
		return fmt.Errorf("read local preflight: %w", err)
	}
	if !strings.Contains(string(preflight), "ordinary performance SLA packages") {
		return fmt.Errorf("ordinary performance SLA publication gate is not configured")
	}
	state.ordinarySLASanitized = strings.Contains(
		string(preflight),
		"GOFLAGS='' CI='' go test -race=false -short=false",
	)
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open repository root: %w", err)
	}
	defer func() {
		_ = rootFS.Close()
	}()
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		relativeFile, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		source, readErr := rootFS.ReadFile(relativeFile)
		if readErr != nil {
			return readErr
		}
		if !sourceHasRaceSuppressedSLA(string(source)) {
			return nil
		}
		relativeDir, relativeErr := filepath.Rel(root, filepath.Dir(path))
		if relativeErr != nil {
			return relativeErr
		}
		packagePath := "./" + filepath.ToSlash(relativeDir)
		if !slices.Contains(state.raceSkippedSLAs, packagePath) {
			state.raceSkippedSLAs = append(state.raceSkippedSLAs, packagePath)
		}
		return nil
	})
}

// raceSkippedSLAMarker is the structural token a wall-clock SLA assertion
// must carry at its `raceEnabled` skip/downgrade site (see
// docs/policies/harness-hygiene.ai.md#L31-L32: "encode a binary requirement
// as prose the model self-checks" is a documented anti-pattern here).
// sourceHasRaceSuppressedSLA keys discovery off this exact token instead of
// matching one of several known skip/log sentences, so rewording a
// human-readable skip message can't silently drop a package from "every
// race-skipped SLA published to preflight.sh" coverage.
const raceSkippedSLAMarker = "SLA-RACE-SKIP"

func sourceHasRaceSuppressedSLA(source string) bool {
	return strings.Contains(source, "raceEnabled") && strings.Contains(source, raceSkippedSLAMarker)
}

func agmValidatesRaceSkippedWallClockSLACoverage(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if len(state.raceSkippedSLAs) == 0 {
		return fmt.Errorf("no race-skipped wall-clock SLA packages discovered")
	}
	preflight, err := os.ReadFile(filepath.Join(localDevBDDRepoRoot(), "scripts", "preflight.sh"))
	if err != nil {
		return fmt.Errorf("read local preflight: %w", err)
	}
	for _, packagePath := range state.raceSkippedSLAs {
		if strings.Contains(string(preflight), packagePath) {
			state.ordinarySLAPackages = append(state.ordinarySLAPackages, packagePath)
		}
	}
	return nil
}

func everyRaceSkippedSLAPackageShouldRunWithoutRaceInstrumentation(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	missing := make([]string, 0)
	for _, packagePath := range state.raceSkippedSLAs {
		if !slices.Contains(state.ordinarySLAPackages, packagePath) {
			missing = append(missing, packagePath)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("ordinary performance SLA gate omits race-skipped packages: %s", strings.Join(missing, ", "))
	}
	if !state.ordinarySLASanitized {
		return fmt.Errorf("ordinary performance SLA gate does not neutralize inherited Go test modes")
	}
	return nil
}

func canonicalWayfinderV2StatusForRoute(ctx context.Context, harness, family string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "safe-pr-v2-trace-*")
	if err != nil {
		return fmt.Errorf("create canonical trace directory: %w", err)
	}
	state.traceDir = dir
	state.harness = harness
	state.family = family
	content := "---\nschema_version: \"2.0\"\nproject_name: parity-audit\nproject_type: infrastructure\nrisk_level: M\ncurrent_waypoint: CHARTER\nstatus: planning\ncreated_at: 2026-07-20T00:00:00Z\nupdated_at: 2026-07-20T00:00:00Z\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("write canonical trace: %w", err)
	}
	return nil
}

func safePRLoadsCanonicalPlanningTrace(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.traceDir == "" {
		return fmt.Errorf("trace directory is not initialized in scenario state")
	}
	trace, err := safepr.LoadSession(state.traceDir)
	if err != nil {
		return fmt.Errorf("load canonical %s/%s trace: %w", state.harness, state.family, err)
	}
	state.trace = trace
	return nil
}

func safePRShouldAttributeTraceToProject(ctx context.Context, project string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.trace.ID != project {
		return fmt.Errorf("safe-pr trace attribution = %q, want %q", state.trace.ID, project)
	}
	return nil
}

func safeLocalDevelopmentCommandIsConfigured(ctx context.Context, command string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.command = command
	state.commandSpec = filepath.Join(localDevBDDRepoRoot(), "cmd", command, "SPEC.md")
	return nil
}

func agmValidatesSafeLocalDevelopmentCommandCoverage(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.commandSpec == "" {
		return fmt.Errorf("no safe local development command configured")
	}
	if _, err := os.Stat(state.commandSpec); err != nil {
		return fmt.Errorf("safe local development command SPEC %s: %w", state.commandSpec, err)
	}
	return nil
}

func safeLocalDevelopmentCommandShouldHaveCoLocatedSPEC(ctx context.Context, command string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if command != state.command {
		return fmt.Errorf("configured safe local development command = %q, want %q", state.command, command)
	}
	wantSuffix := filepath.Join("cmd", command, "SPEC.md")
	if !strings.HasSuffix(state.commandSpec, wantSuffix) {
		return fmt.Errorf("safe local development command SPEC = %q, want suffix %q", state.commandSpec, wantSuffix)
	}
	return nil
}

func safeLocalDevelopmentLibraryIsConfigured(ctx context.Context, pkg string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.library = pkg
	state.librarySpec = filepath.Join(localDevBDDRepoRoot(), "internal", pkg, "SPEC.md")
	return nil
}

func agmValidatesSafeLocalDevelopmentLibraryCoverage(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.librarySpec == "" {
		return fmt.Errorf("no safe local development library configured")
	}
	if _, err := os.Stat(state.librarySpec); err != nil {
		return fmt.Errorf("safe local development library SPEC %s: %w", state.librarySpec, err)
	}
	return nil
}

func safeLocalDevelopmentLibraryShouldHaveCoLocatedSPEC(ctx context.Context, pkg string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if pkg != state.library {
		return fmt.Errorf("configured safe local development library = %q, want %q", state.library, pkg)
	}
	wantSuffix := filepath.Join("internal", pkg, "SPEC.md")
	if !strings.HasSuffix(state.librarySpec, wantSuffix) {
		return fmt.Errorf("safe local development library SPEC = %q, want suffix %q", state.librarySpec, wantSuffix)
	}
	return nil
}

func getLocalDevGuardrailState(ctx context.Context) (*localDevGuardrailState, error) {
	state, ok := ctx.Value(localDevGuardrailStateKey{}).(*localDevGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("local development guardrail state not initialized")
	}
	return state, nil
}

func localDevBDDRepoRoot() string {
	return packageSpecBDDRepoRoot()
}

func minuteConstant(source, name string) (int, error) {
	pattern := fmt.Sprintf(`(?m)^\s*%s\s*=\s*(\d+)\s*\*\s*time\.Minute$`, regexp.QuoteMeta(name))
	match := regexp.MustCompile(pattern).FindStringSubmatch(source)
	if len(match) != 2 {
		return 0, fmt.Errorf("affected integration %s declaration not found", name)
	}
	minutes, err := strconv.Atoi(match[1])
	if err != nil || minutes <= 0 {
		return 0, fmt.Errorf("affected integration %s must be positive minutes", name)
	}
	return minutes, nil
}
