package steps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

const sandboxProviderFeaturePath = "agm/test/bdd/features/sandbox_provider_guardrails.feature"

type sandboxProviderGuardrailStateKey struct{}
type sandboxProviderRuntimeStateKey struct{}

type sandboxProviderRuntimeState struct {
	worktreesBefore string
	worktreesAfter  string
	testOutput      string
	testErr         error
	inventoryErr    error
}

type sandboxProviderCleanupStateKey struct{}

type sandboxProviderCleanupState struct {
	output string
	err    error
}

// RegisterSandboxProviderGuardrailSteps registers BDD steps for sandbox provider packages.
func RegisterSandboxProviderGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(parent context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(parent, sandboxProviderCleanupStateKey{}, &sandboxProviderCleanupState{}), nil
	})
	ctx.Before(func(parent context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(parent, sandboxProviderRuntimeStateKey{}, &sandboxProviderRuntimeState{}), nil
	})
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          sandboxProviderGuardrailStateKey{},
		label:             "sandbox provider package",
		featurePath:       sandboxProviderFeaturePath,
		configuredPattern: `^sandbox provider package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates sandbox provider package coverage$`,
		colocatedPattern:  `^sandbox provider package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Step(`^AGM runs the sandbox provider cleanup retry regressions$`, agmRunsSandboxProviderCleanupRetryRegressions)
	ctx.Step(`^failed destruction should resume at the unfinished cleanup phase$`, failedSandboxDestructionShouldResumeAtUnfinishedPhase)
	ctx.Step(`^AGM runs the sandbox working directory regressions$`, agmRunsSandboxWorkingDirectoryRegressions)
	ctx.Step(`^sandbox providers should preserve the requested project directory$`, sandboxProvidersShouldPreserveRequestedProjectDirectory)
	ctx.Step(`^AGM should route the mapped directory through the shared harness lifecycle$`, agmShouldRouteMappedDirectoryThroughSharedHarnessLifecycle)
	ctx.Step(`^AGM runs the retired sandbox provider regressions$`, agmRunsRetiredSandboxProviderRegressions)
	ctx.Step(`^claudecode-worktree should be rejected before workspace creation$`, claudeCodeWorktreeShouldBeRejectedBeforeWorkspaceCreation)
	ctx.Step(`^the invoking repository worktree inventory is captured$`, captureInvokingRepositoryWorktrees)
	ctx.Step(`^Wayfinder sandbox isolation regressions run$`, runWayfinderSandboxIsolationRegressions)
	ctx.Step(`^the Wayfinder sandbox isolation regressions should pass$`, wayfinderSandboxIsolationRegressionsShouldPass)
	ctx.Step(`^the invoking repository worktree inventory should be unchanged$`, invokingRepositoryWorktreesShouldBeUnchanged)
}

func agmRunsSandboxProviderCleanupRetryRegressions(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	state.output, state.err = runLocalGuardrailGoTest(ctx,
		`^(TestWorktreeCleanupRetriesOnlyIncompletePhase|TestProvider_DestroyPreservesLockedWorktreeForRetry)$`,
		"./internal/sandbox", "./internal/sandbox/bubblewrap", "./internal/sandbox/gvisor",
	)
	return nil
}

func failedSandboxDestructionShouldResumeAtUnfinishedPhase(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	if state.err != nil {
		return fmt.Errorf("sandbox provider cleanup retry regressions: %w: %s", state.err, state.output)
	}
	return nil
}

func agmRunsSandboxWorkingDirectoryRegressions(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	state.output, state.err = runSandboxProviderCommand(ctx, 2*time.Minute,
		"go", "test", "-v", "-count=1", "-timeout=90s", "-run",
		`^(TestMatchWorkingDir.*|TestMapFlatWorkingDir.*|TestProvider_CreateMapsRequestedWorkingDirectoryIntoMatchingClone|TestResolveSandboxLowerDirs_FallsBackToContainingGitRepoForSubdirectory|TestFindPrimaryRepoUsesRequestedDirectoryInsteadOfProcessCWD|TestMaybeProvisionSandboxReturnsProviderMappedWorkingDirectory|TestProvisionSandbox.*)$`,
		"./internal/sandbox", "./internal/sandbox/apfs", "./agm/cmd/agm",
	)
	return nil
}

func sandboxProvidersShouldPreserveRequestedProjectDirectory(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	if state.err != nil {
		return fmt.Errorf("sandbox working directory regressions: %w: %s", state.err, state.output)
	}
	for _, name := range []string{
		"TestMatchWorkingDirPreservesNestedRelativePath",
		"TestMatchWorkingDirSelectsMostSpecificConfiguredRepository",
		"TestMatchWorkingDirResolvesSymlinkAliases",
	} {
		if !strings.Contains(state.output, "--- PASS: "+name) {
			return fmt.Errorf("sandbox working directory output does not show %s passing:\n%s", name, state.output)
		}
	}
	return nil
}

func agmShouldRouteMappedDirectoryThroughSharedHarnessLifecycle(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	for _, name := range []string{
		"TestResolveSandboxLowerDirs_FallsBackToContainingGitRepoForSubdirectory",
		"TestFindPrimaryRepoUsesRequestedDirectoryInsteadOfProcessCWD",
		"TestMaybeProvisionSandboxReturnsProviderMappedWorkingDirectory",
		"TestProvisionSandboxCleansUpProviderThatViolatesWorkingDirectoryContract",
		"TestProvisionSandboxPreservesContractAndCleanupFailures",
	} {
		if !strings.Contains(state.output, "--- PASS: "+name) {
			return fmt.Errorf("AGM sandbox lifecycle output does not show %s passing:\n%s", name, state.output)
		}
	}
	return nil
}

func agmRunsRetiredSandboxProviderRegressions(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	state.output, state.err = runSandboxProviderCommand(ctx, 2*time.Minute,
		"go", "test", "-v", "-count=1", "-timeout=90s", "-run",
		`^(TestProviderLookup_RetiredClaudeCodeWorktree|TestProvisionSandboxRejectsRetiredClaudeCodeProviderBeforeWorkspaceCreation)$`,
		"./internal/sandbox", "./agm/cmd/agm",
	)
	return nil
}

func claudeCodeWorktreeShouldBeRejectedBeforeWorkspaceCreation(ctx context.Context) error {
	state, ok := ctx.Value(sandboxProviderCleanupStateKey{}).(*sandboxProviderCleanupState)
	if !ok || state == nil {
		return fmt.Errorf("sandbox provider cleanup state not initialized")
	}
	if state.err != nil {
		return fmt.Errorf("retired sandbox provider regressions: %w: %s", state.err, state.output)
	}
	for _, name := range []string{
		"TestProviderLookup_RetiredClaudeCodeWorktree",
		"TestProvisionSandboxRejectsRetiredClaudeCodeProviderBeforeWorkspaceCreation",
	} {
		if !strings.Contains(state.output, "--- PASS: "+name) {
			return fmt.Errorf("retired provider output does not show %s passing:\n%s", name, state.output)
		}
	}
	return nil
}

func captureInvokingRepositoryWorktrees(ctx context.Context) error {
	state, err := getSandboxProviderRuntimeState(ctx)
	if err != nil {
		return err
	}
	raw, err := runSandboxProviderCommand(ctx, 10*time.Second, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	state.worktreesBefore, err = scopedWorktreeInventory(packageSpecBDDRepoRoot(), raw)
	return err
}

func runWayfinderSandboxIsolationRegressions(ctx context.Context) error {
	state, err := getSandboxProviderRuntimeState(ctx)
	if err != nil {
		return err
	}
	state.testOutput, state.testErr = runSandboxProviderCommand(
		ctx,
		2*time.Minute,
		"go", "test", "./wayfinder/pkg/sandbox", "-v",
		"-run", `^Test(CreateSandbox|ListSandboxes|CleanupSandbox|BasicSandboxOperationsDoNotMutateHostRepository|SandboxGitWorktreeLifecycleIsIsolated)$`,
		"-count=1", "-timeout=90s",
	)
	raw, err := runSandboxProviderCommand(ctx, 10*time.Second, "git", "worktree", "list", "--porcelain")
	if err != nil {
		state.inventoryErr = err
		return err
	}
	state.worktreesAfter, state.inventoryErr = scopedWorktreeInventory(packageSpecBDDRepoRoot(), raw)
	return nil
}

func wayfinderSandboxIsolationRegressionsShouldPass(ctx context.Context) error {
	state, err := getSandboxProviderRuntimeState(ctx)
	if err != nil {
		return err
	}
	if state.testErr != nil {
		return fmt.Errorf("wayfinder sandbox isolation regressions: %w\n%s", state.testErr, state.testOutput)
	}
	for _, name := range []string{
		"TestCreateSandbox",
		"TestListSandboxes",
		"TestCleanupSandbox",
		"TestBasicSandboxOperationsDoNotMutateHostRepository",
		"TestSandboxGitWorktreeLifecycleIsIsolated",
	} {
		if !strings.Contains(state.testOutput, "--- PASS: "+name) {
			return fmt.Errorf("wayfinder sandbox output does not show %s passing:\n%s", name, state.testOutput)
		}
	}
	return nil
}

func invokingRepositoryWorktreesShouldBeUnchanged(ctx context.Context) error {
	state, err := getSandboxProviderRuntimeState(ctx)
	if err != nil {
		return err
	}
	if state.inventoryErr != nil {
		return fmt.Errorf("capture final worktree inventory: %w", state.inventoryErr)
	}
	if state.worktreesAfter != state.worktreesBefore {
		return fmt.Errorf("invoking repository worktree inventory changed:\nbefore:\n%s\nafter:\n%s", state.worktreesBefore, state.worktreesAfter)
	}
	return nil
}

func scopedWorktreeInventory(root, porcelain string) (string, error) {
	root = filepath.Clean(root)
	var scoped []string
	for record := range strings.SplitSeq(strings.TrimSpace(porcelain), "\n\n") {
		if strings.TrimSpace(record) == "" {
			continue
		}
		firstLine, _, _ := strings.Cut(record, "\n")
		path, ok := strings.CutPrefix(firstLine, "worktree ")
		if !ok || strings.TrimSpace(path) == "" {
			return "", fmt.Errorf("malformed worktree record: %q", record)
		}
		within, err := pathWithinDirectory(root, strings.TrimSpace(path))
		if err != nil {
			return "", err
		}
		if within {
			scoped = append(scoped, record)
		}
	}
	return strings.Join(scoped, "\n\n"), nil
}

func pathWithinDirectory(root, path string) (bool, error) {
	canonicalRoot, err := canonicalPathAllowMissing(root)
	if err != nil {
		return false, fmt.Errorf("resolve invoking repository root %q: %w", root, err)
	}
	canonicalPath, err := canonicalPathAllowMissing(path)
	if err != nil {
		return false, fmt.Errorf("resolve worktree path %q: %w", path, err)
	}
	rel, err := filepath.Rel(canonicalRoot, canonicalPath)
	if err != nil {
		return false, fmt.Errorf("compare worktree path %q with root %q: %w", path, root, err)
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

// canonicalPathAllowMissing resolves every existing prefix while retaining a
// missing suffix. Git can report a concurrently removed, prunable worktree;
// its absent path must still be classified lexically against the physical
// invoking root instead of turning unrelated shared-repository churn into a
// BDD failure.
func canonicalPathAllowMissing(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			parts := append([]string{resolved}, suffix...)
			return filepath.Join(parts...), nil
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		suffix = append([]string{filepath.Base(current)}, suffix...)
		current = parent
	}
}

func getSandboxProviderRuntimeState(ctx context.Context) (*sandboxProviderRuntimeState, error) {
	state, ok := ctx.Value(sandboxProviderRuntimeStateKey{}).(*sandboxProviderRuntimeState)
	if !ok || state == nil {
		return nil, fmt.Errorf("sandbox provider runtime state not initialized")
	}
	return state, nil
}

func runSandboxProviderCommand(parent context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = packageSpecBDDRepoRoot()
	cmd.SysProcAttr = procguard.ProcessGroupAttr()
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return string(output), fmt.Errorf("%s timed out after %s: %w", name, timeout, ctx.Err())
	}
	if err != nil {
		return string(output), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(output), nil
}
