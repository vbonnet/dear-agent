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
	command               string
	commandSpec           string
	library               string
	librarySpec           string
	traceDir              string
	trace                 safepr.Session
	harness               string
	family                string
	preflightMinutes      int
	localTestTimeout      string
	affectedTestTimeout   string
	ciTestTimeout         string
	localVulnAllowlist    []string
	ciVulnAllowlist       []string
	worktreeBase          string
	worktreeRepo          string
	worktreePath          string
	initialLockReason     string
	transactionOutcome    string
	transactionErr        error
	lockedInPreflight     bool
	lockedInPRCreate      bool
	wayfinderCleanupErr   error
	worktreePreserved     bool
	cleanupRegression     string
	cleanupRegressionErr  error
	childRegression       string
	childRegressionErr    error
	auditRegression       string
	auditRegressionErr    error
	requiredCIRegression  string
	requiredCIError       error
	mergeLoopCIRegression string
	mergeLoopCIError      error
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
	ctx.Step(`^a safe-pr linked worktree with "([^"]*)" lock ownership$`, safePRLinkedWorktreeWithLockOwnership)
	ctx.Step(`^safe-pr protects a "([^"]*)" preflight and PR creation transaction$`, safePRProtectsTransaction)
	ctx.Step(`^the worktree should be protected during preflight and PR creation$`, worktreeShouldBeProtectedDuringTransaction)
	ctx.Step(`^the "([^"]*)" lock ownership should remain after the transaction$`, lockOwnershipShouldRemainAfterTransaction)
	ctx.Step(`^Wayfinder cleanup overlaps a protected safe-pr transaction$`, wayfinderCleanupOverlapsProtectedTransaction)
	ctx.Step(`^Wayfinder should preserve the protected worktree and reject cleanup$`, wayfinderShouldPreserveProtectedWorktree)
	ctx.Step(`^Wayfinder should remove the worktree after the safe-pr transaction$`, wayfinderShouldRemoveWorktreeAfterTransaction)
	ctx.Step(`^AGM runs the protected cleanup regressions$`, agmRunsProtectedCleanupRegressions)
	ctx.Step(`^Wayfinder and AGM cleanup should preserve Git-locked checkouts$`, cleanupShouldPreserveGitLockedCheckouts)
	ctx.Step(`^AGM runs the protected repository cleanup regression$`, agmRunsProtectedRepositoryCleanupRegression)
	ctx.Step(`^repository cleanup should preserve the worktree and its branches$`, repositoryCleanupShouldPreserveWorktreeAndBranches)
	ctx.Step(`^AGM runs the safe-pr abrupt-parent regression$`, agmRunsSafePRAbruptParentRegression)
	ctx.Step(`^the child should retain transaction ownership until it exits$`, childShouldRetainTransactionOwnershipUntilExit)
	ctx.Step(`^AGM runs the safe-pr final transaction audit regression$`, agmRunsSafePRFinalTransactionAuditRegression)
	ctx.Step(`^each safe-pr transaction should have one accurate audit record$`, eachSafePRTransactionShouldHaveOneAccurateAuditRecord)
	ctx.Step(`^AGM runs the effective required-check regressions$`, agmRunsEffectiveRequiredCheckRegressions)
	ctx.Step(`^safe-merge should enforce complete provider-required CI without advisory drift$`, safeMergeShouldEnforceProviderRequiredCI)
	ctx.Step(`^local, affected integration, and required CI Go test timeouts are configured$`, repositoryGoTestTimeoutsAreConfigured)
	ctx.Step(`^AGM validates Go test timeout parity$`, agmValidatesGoTestTimeoutParity)
	ctx.Step(`^all repository Go test timeouts should match$`, repositoryGoTestTimeoutsShouldMatch)
	ctx.Step(`^local and required CI govulncheck allowlists are configured$`, localAndRequiredCIGovulncheckAllowlistsAreConfigured)
	ctx.Step(`^AGM validates govulncheck policy parity$`, agmValidatesGovulncheckPolicyParity)
	ctx.Step(`^the local and required CI govulncheck allowlists should match$`, localAndRequiredCIGovulncheckAllowlistsShouldMatch)
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

func agmRunsProtectedRepositoryCleanupRegression(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.cleanupRegression, state.cleanupRegressionErr = runLocalGuardrailGoTest(ctx,
		`^TestCleanupWorktreesScriptPreservesBranchWhenProtectedWorktreeRemovalFails$`,
		"./internal/safepr",
	)
	return nil
}

func repositoryCleanupShouldPreserveWorktreeAndBranches(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.cleanupRegressionErr != nil {
		return fmt.Errorf("protected repository cleanup regression: %w: %s", state.cleanupRegressionErr, state.cleanupRegression)
	}
	return nil
}

func agmRunsSafePRAbruptParentRegression(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.childRegression, state.childRegressionErr = runLocalGuardrailGoTest(ctx,
		`^TestWorktreeTransactionLockOutlivesKilledParentFor(ProtectedChild|GitHelper)$`,
		"./internal/safepr",
	)
	return nil
}

func childShouldRetainTransactionOwnershipUntilExit(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.childRegressionErr != nil {
		return fmt.Errorf("safe-pr abrupt-parent regression: %w: %s", state.childRegressionErr, state.childRegression)
	}
	return nil
}

func agmRunsSafePRFinalTransactionAuditRegression(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.auditRegression, state.auditRegressionErr = runLocalGuardrailGoTest(ctx,
		`^TestRun_CreateAuditsFinalTransactionOutcome$`,
		"./cmd/safe-pr",
	)
	return nil
}

func eachSafePRTransactionShouldHaveOneAccurateAuditRecord(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.auditRegressionErr != nil {
		return fmt.Errorf("safe-pr final transaction audit regression: %w: %s", state.auditRegressionErr, state.auditRegression)
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
	state.mergeLoopCIRegression, state.mergeLoopCIError = runLocalGuardrailGoTest(ctx,
		`^Test(MergeLoopUsesSharedRequiredProjection|MergeLoopPreservesAuthoritativeEmptyFallback|MergeLoopMapsProjectedRequiredStatuses|MergeLoopDefersOnlyUnavailableProjection|MergeLoopDefersOnlyUnknownProjectedStatus|MergeLoopAbortsWhenParentContextCanceled|MergeLoopSkipsProjectionWhenOpenPRsExceedCap)$`,
		"./cmd/mergeloop",
	)
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
	affectedMatch := regexp.MustCompile(`(?m)^\s*goCommandTimeout\s*=\s*(\d+)\s*\*\s*time\.Minute$`).FindSubmatch(affected)
	if len(affectedMatch) != 2 {
		return fmt.Errorf("affected integration Go test timeout declaration not found")
	}
	state.localTestTimeout = string(localMatch[1])
	state.affectedTestTimeout = string(affectedMatch[1]) + "m"
	state.ciTestTimeout = string(ciMatch[1])
	return nil
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
