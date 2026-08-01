package steps

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/procguard"
)

const (
	specAuditGoTestDeadline    = 2 * time.Minute
	specAuditGoTestTimeout     = "90s"
	specAuditGoTestOutputLimit = 1 << 20
	specAuditGoTestFileLimit   = 512
	specAuditGoTestFileBytes   = 4 << 20
	specAuditGoTestTotalBytes  = 32 << 20
	specAuditProcessGroupWait  = 2 * time.Second
)

// The nested package is repository implementation test source and must already
// be trusted. These process and environment controls limit accidental leakage
// and residue; they are not a filesystem, syscall, or network sandbox. Run
// untrusted source only in a separate credential-free sandbox or CI job.
type specAuditGoChildRuntime struct {
	goExecutable        trustedSpecAuditExecutable
	gitExecutable       trustedSpecAuditExecutable
	environment         []string
	taskRoot            string
	taskRootIdentity    os.FileInfo
	taskRootOwner       uint32
	taskRootPermissions os.FileMode
}

type trustedSpecAuditExecutable struct {
	path     string
	identity os.FileInfo
}

type specGovernanceToolingStateKey struct{}

type specGovernanceToolingState struct {
	repoRoot string
	command  string
	output   string
	err      error
}

// RegisterSpecGovernanceToolingSteps registers focused specaudit unit checks.
func RegisterSpecGovernanceToolingSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		state := &specGovernanceToolingState{repoRoot: packageSpecBDDRepoRoot()}
		return context.WithValue(ctx, specGovernanceToolingStateKey{}, state), nil
	})
	ctx.Step(`^AGM runs the focused pinned SPEC inventory unit check$`, exercisePinnedSPECInventory)
	ctx.Step(`^AGM runs the focused non-verdict SPEC audit lead unit check$`, exerciseNonVerdictSPECAuditLeads)
	ctx.Step(`^AGM runs the focused reciprocal SPEC and BDD diagnostic unit check$`, exerciseReciprocalSPECBDDDiagnostics)
	ctx.Step(`^AGM runs the focused pinned finding validation unit check$`, exercisePinnedSPECFindingValidation)
	ctx.Step(`^AGM runs the focused bounded offline rendering unit check$`, exerciseBoundedOfflineSPECAuditRendering)
	ctx.Step(`^AGM runs the focused read-only audit boundary unit check$`, exerciseReadOnlySPECAuditBoundary)
	ctx.Step(`^the focused SPEC audit unit check should pass$`, specGovernanceBehaviorShouldPass)
}

func exercisePinnedSPECInventory(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically")
}

func exerciseNonVerdictSPECAuditLeads(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically")
}

func exerciseReciprocalSPECBDDDiagnostics(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryReportsFeatureFirstDiagnosticsFromPinnedObjects")
}

func exercisePinnedSPECFindingValidation(ctx context.Context) error {
	return runSpecAuditGoTests(ctx,
		"TestValidatePinsFindingsToGitResolvedInventory",
		"TestPinnedValidationRejectsForgedEvidenceAndUnsafeVerdicts",
		"TestPositiveOwnershipPlanRejectsDuplicateRetentionAndDivergentApplicability",
		"TestPinnedValidationRejectsIncompleteSharedFeatureAndPreservationTargets",
		"TestPinnedValidationRequiresEveryPinnedReciprocalFeatureInRetirementPlan",
	)
}

func exerciseBoundedOfflineSPECAuditRendering(ctx context.Context) error {
	return runSpecAuditGoTests(ctx,
		"TestRenderIsOfflineAndEscapesEvidence",
		"TestReportInputsAndArtifactsAreBounded",
		"TestEscapedHTMLStopsAtArtifactLimit",
	)
}

func exerciseReadOnlySPECAuditBoundary(ctx context.Context) error {
	return runSpecAuditGoTests(ctx, "TestInventoryValidateRenderPreserveTargetRepositoryState")
}

func runSpecAuditGoTests(ctx context.Context, names ...string) error {
	state, err := getSpecGovernanceToolingState(ctx)
	if err != nil {
		return err
	}
	state.command = ""
	state.output = ""
	state.err = nil
	if _, err := buildExactGoTestRunPattern(names); err != nil {
		state.command = "go test ./tools/specaudit"
		state.err = err
		return nil
	}
	childRuntime, err := newSpecAuditGoChildRuntime()
	if err != nil {
		state.command = "prepare trusted SPEC audit Go test runtime"
		state.err = err
		return nil
	}
	defer func() {
		if cleanupErr := childRuntime.cleanup(); cleanupErr != nil {
			state.err = errors.Join(state.err, cleanupErr)
		}
	}()
	testCtx, cancel := context.WithTimeout(ctx, specAuditGoTestDeadline)
	defer cancel()

	packageInfo, listCommand, listOutput, err := resolveBuildSelectedGoTestPackage(
		testCtx,
		state.repoRoot,
		"./tools/specaudit",
		filepath.Join(state.repoRoot, "tools", "specaudit"),
		childRuntime,
	)
	state.command = strings.Join(listCommand.Args, " ")
	state.output = listOutput.String()
	if err != nil {
		state.err = err
		return nil
	}
	preRunDeclarations, err := observeSelectedGoTestDeclarations(packageInfo, names)
	if err != nil {
		state.err = err
		return nil
	}

	command, err := newSpecAuditGoTestCommand(testCtx, state.repoRoot, childRuntime, names...)
	if err != nil {
		state.err = err
		return nil
	}
	state.command = strings.Join(command.Args, " ")
	output, commandErr := runBoundedSpecAuditCommand(command, specAuditGoTestOutputLimit)
	state.output = output.String()
	switch {
	case testCtx.Err() != nil:
		state.err = fmt.Errorf("%s did not complete within %s: %w", state.command, specAuditGoTestDeadline, testCtx.Err())
	case output.Truncated():
		state.err = fmt.Errorf("%s output exceeded %d-byte safety limit", state.command, specAuditGoTestOutputLimit)
	case commandErr != nil:
		state.err = commandErr
	}
	if state.err != nil {
		return nil
	}
	if err := validateExactGoTestJSON(state.output, packageInfo.ImportPath, names); err != nil {
		state.err = err
		return nil
	}

	postPackageInfo, postListCommand, postListOutput, err := resolveBuildSelectedGoTestPackage(
		testCtx,
		state.repoRoot,
		"./tools/specaudit",
		filepath.Join(state.repoRoot, "tools", "specaudit"),
		childRuntime,
	)
	state.command = strings.Join(postListCommand.Args, " ")
	state.output = postListOutput.String()
	if err != nil {
		state.err = fmt.Errorf("post-test selected-test declaration observation: %w", err)
		return nil
	}
	if err := validatePostRunSelectedTestDeclarations(postPackageInfo, names, preRunDeclarations); err != nil {
		state.err = err
	}
	return nil
}

func newSpecAuditGoTestCommand(ctx context.Context, repoRoot string, childRuntime specAuditGoChildRuntime, names ...string) (*exec.Cmd, error) {
	pattern, err := buildExactGoTestRunPattern(names)
	if err != nil {
		return nil, err
	}
	if err := childRuntime.revalidateExecutables(); err != nil {
		return nil, err
	}
	arguments := []string{"test", "-json", "-mod=readonly", "-count=1", "-timeout=" + specAuditGoTestTimeout, "-run", pattern, "./tools/specaudit"}
	command := exec.CommandContext(ctx, childRuntime.goExecutable.path, arguments...)
	command.Dir = repoRoot
	command.Env = slices.Clone(childRuntime.environment)
	configureSpecAuditChildCommand(command)
	return command, nil
}

func newSpecAuditGoListCommand(ctx context.Context, repoRoot, packagePattern string, childRuntime specAuditGoChildRuntime) (*exec.Cmd, error) {
	if err := childRuntime.revalidateExecutables(); err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, childRuntime.goExecutable.path, "list", "-mod=readonly", "-find", "-json", packagePattern)
	command.Dir = repoRoot
	command.Env = slices.Clone(childRuntime.environment)
	configureSpecAuditChildCommand(command)
	return command, nil
}

func configureSpecAuditChildCommand(command *exec.Cmd) {
	command.SysProcAttr = procguard.ProcessGroupAttr()
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = time.Second
}

// runBoundedSpecAuditCommand keeps only a bounded diagnostic prefix and kills
// the entire isolated process group as soon as that bound is exceeded. Waiting
// for a misbehaving nested test after its output cap is reached would let it
// retain CPU, descendants, or a blocked pipe until the outer deadline.
func runBoundedSpecAuditCommand(command *exec.Cmd, limit int) (*boundedSpecAuditOutput, error) {
	output := &boundedSpecAuditOutput{limit: limit}
	if !specAuditProcessGroupsSupported() {
		return output, fmt.Errorf("trusted SPEC audit Go commands require process-group cleanup on darwin or linux")
	}
	if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid || command.Cancel == nil {
		return output, fmt.Errorf("trusted SPEC audit Go command is missing isolated process-group configuration")
	}
	output.onLimit = func() {
		if err := command.Cancel(); err != nil {
			// The child can win the race to exit after writing its final bytes.
			return
		}
	}
	command.Stdout = output
	command.Stderr = output
	commandErr := command.Run()
	cleanupErr := terminateSpecAuditProcessGroup(command)
	return output, errors.Join(commandErr, cleanupErr)
}

func specAuditProcessGroupsSupported() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "linux"
}

// terminateSpecAuditProcessGroup runs after Cmd.Run has reaped the direct
// child. It kills descendants that remain in the child's process group and
// waits until the kernel no longer reports that group. Descendants that leave
// the group are outside this lifecycle cleanup. This is not a sandbox or a
// claim that trusted test code lacked ambient filesystem access.
func terminateSpecAuditProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	processGroupID := command.Process.Pid
	if err := syscall.Kill(-processGroupID, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("terminate SPEC audit process group %d: %w", processGroupID, err)
	}
	deadline := time.Now().Add(specAuditProcessGroupWait)
	for {
		err := syscall.Kill(-processGroupID, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("probe SPEC audit process group %d: %w", processGroupID, err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("SPEC audit process group %d remained alive after %s", processGroupID, specAuditProcessGroupWait)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// boundedSpecAuditOutput caps nested go-test output and invokes onLimit once
// after retaining the diagnostic prefix. The callback must terminate the
// producer; otherwise an over-limit child could continue running indefinitely.
type boundedSpecAuditOutput struct {
	mu         sync.Mutex
	buffer     bytes.Buffer
	limit      int
	truncated  bool
	onLimit    func()
	cancelOnce sync.Once
}

func (output *boundedSpecAuditOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	originalLen := len(data)
	remaining := output.limit - output.buffer.Len()
	limitExceeded := false
	if remaining <= 0 {
		output.truncated = true
		limitExceeded = true
		output.mu.Unlock()
		output.cancel(limitExceeded)
		return originalLen, nil
	}
	if len(data) > remaining {
		output.truncated = true
		limitExceeded = true
		data = data[:remaining]
	}
	_, err := output.buffer.Write(data)
	output.mu.Unlock()
	output.cancel(limitExceeded)
	return originalLen, err
}

func (output *boundedSpecAuditOutput) cancel(limitExceeded bool) {
	if !limitExceeded || output.onLimit == nil {
		return
	}
	output.cancelOnce.Do(output.onLimit)
}

func (output *boundedSpecAuditOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.String()
}

func (output *boundedSpecAuditOutput) Truncated() bool {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.truncated
}

type goListTestPackage struct {
	Dir          string
	ImportPath   string
	Name         string
	TestGoFiles  []string
	XTestGoFiles []string
}

type selectedGoTestDeclarationFileObservation struct {
	Name   string
	SHA256 [sha256.Size]byte
}

// selectedGoTestDeclarationObservation records only the build-selected
// TestGoFiles and XTestGoFiles used to validate exact named test declarations
// at one observation point. It is deliberately not a build-input integrity
// manifest: it does not cover or make immutable production Go files, module
// files, embed inputs, or dependencies, and a mid-run swap restored before the
// post-test observation is outside this mechanism's detection boundary.
type selectedGoTestDeclarationObservation struct {
	PackageDir   string
	ImportPath   string
	PackageName  string
	TestGoFiles  []string
	XTestGoFiles []string
	Files        []selectedGoTestDeclarationFileObservation
}

const selectedTestDeclarationObservationScope = "selected-test declaration validation covers only build-selected TestGoFiles and XTestGoFiles at the pre-test and post-test observation points; it does not cover or make immutable production Go files, module files, embed inputs, or dependencies, and it cannot detect a mid-run swap restored before the post-test observation"

func resolveBuildSelectedGoTestPackage(
	ctx context.Context,
	repoRoot string,
	packagePattern string,
	expectedPackageDir string,
	childRuntime specAuditGoChildRuntime,
) (goListTestPackage, *exec.Cmd, *boundedSpecAuditOutput, error) {
	command, err := newSpecAuditGoListCommand(ctx, repoRoot, packagePattern, childRuntime)
	if err != nil {
		return goListTestPackage{}, &exec.Cmd{}, &boundedSpecAuditOutput{limit: specAuditGoTestOutputLimit}, err
	}
	output, commandErr := runBoundedSpecAuditCommand(command, specAuditGoTestOutputLimit)
	if ctx.Err() != nil {
		return goListTestPackage{}, command, output, fmt.Errorf("%s: %w", strings.Join(command.Args, " "), ctx.Err())
	}
	if output.Truncated() {
		return goListTestPackage{}, command, output, fmt.Errorf("%s output exceeded %d-byte safety limit", strings.Join(command.Args, " "), specAuditGoTestOutputLimit)
	}
	if commandErr != nil {
		return goListTestPackage{}, command, output, commandErr
	}
	var packageInfo goListTestPackage
	if err := json.Unmarshal([]byte(output.String()), &packageInfo); err != nil {
		return goListTestPackage{}, command, output, fmt.Errorf("decode %s output: %w", strings.Join(command.Args, " "), err)
	}
	if err := validateGoListTestPackage(packageInfo); err != nil {
		return goListTestPackage{}, command, output, err
	}
	if err := validateExpectedPackageDirectory(packageInfo.Dir, expectedPackageDir); err != nil {
		return goListTestPackage{}, command, output, err
	}
	return packageInfo, command, output, nil
}

func validateExpectedPackageDirectory(resolved, expected string) error {
	if expected == "" || !filepath.IsAbs(expected) || filepath.Clean(expected) != expected {
		return fmt.Errorf("invalid expected package directory %q", expected)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("stat resolved package directory %q: %w", resolved, err)
	}
	expectedInfo, err := os.Stat(expected)
	if err != nil {
		return fmt.Errorf("stat expected package directory %q: %w", expected, err)
	}
	if !resolvedInfo.IsDir() || !expectedInfo.IsDir() || !os.SameFile(resolvedInfo, expectedInfo) {
		return fmt.Errorf("go list resolved package directory %q, want exactly %q", resolved, expected)
	}
	return nil
}

func validateGoListTestPackage(packageInfo goListTestPackage) error {
	if packageInfo.Dir == "" || !filepath.IsAbs(packageInfo.Dir) || filepath.Clean(packageInfo.Dir) != packageInfo.Dir {
		return fmt.Errorf("go list returned invalid package directory %q", packageInfo.Dir)
	}
	if packageInfo.ImportPath == "" || packageInfo.Name == "" {
		return fmt.Errorf("go list returned incomplete package identity %q/%q", packageInfo.ImportPath, packageInfo.Name)
	}
	seenFiles := make(map[string]struct{})
	files := append(slicesClone(packageInfo.TestGoFiles), packageInfo.XTestGoFiles...)
	if len(files) > specAuditGoTestFileLimit {
		return fmt.Errorf("go list returned %d test files; limit is %d", len(files), specAuditGoTestFileLimit)
	}
	for _, name := range files {
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return fmt.Errorf("go list returned invalid test filename %q", name)
		}
		if _, exists := seenFiles[name]; exists {
			return fmt.Errorf("go list returned duplicate test filename %q", name)
		}
		seenFiles[name] = struct{}{}
	}
	return nil
}

func slicesClone(values []string) []string {
	return append([]string(nil), values...)
}

func buildExactGoTestRunPattern(names []string) (string, error) {
	if len(names) == 0 {
		return "", fmt.Errorf("at least one SPEC audit test name is required")
	}
	seen := make(map[string]struct{}, len(names))
	escaped := make([]string, 0, len(names))
	for _, name := range names {
		switch {
		case !token.IsIdentifier(name):
			return "", fmt.Errorf("invalid top-level Go test identifier %q", name)
		case name == "TestMain":
			return "", fmt.Errorf("TestMain is a test harness entry point, not a selectable regression")
		case !isCmdGoTestName(name):
			return "", fmt.Errorf("%q is not a cmd/go-style test name", name)
		}
		if _, exists := seen[name]; exists {
			return "", fmt.Errorf("duplicate SPEC audit test name %q", name)
		}
		seen[name] = struct{}{}
		escaped = append(escaped, regexp.QuoteMeta(name))
	}
	return "^(?:" + strings.Join(escaped, "|") + ")$", nil
}

type specAuditGoTestJSONEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

type specAuditGoTestEventState struct {
	run      bool
	paused   bool
	terminal string
}

//nolint:gocyclo // Exact event-stream validation keeps fail-closed ordering and terminal-state checks together.
func validateExactGoTestJSON(output, expectedPackage string, names []string) error {
	wanted, err := selectedGoTestNames(names)
	if err != nil {
		return err
	}
	if expectedPackage == "" {
		return fmt.Errorf("expected SPEC audit package identity is empty")
	}
	states := make(map[string]specAuditGoTestEventState)
	packageStarted := false
	packagePassed := false
	eventCount := 0
	decoder := json.NewDecoder(strings.NewReader(output))
	for {
		var event specAuditGoTestJSONEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("decode bounded go test -json event %d: %w", eventCount+1, err)
		}
		eventCount++
		if event.Action == "" || event.Package != expectedPackage {
			return fmt.Errorf("malformed go test -json event %d: action=%q package=%q, want package %q", eventCount, event.Action, event.Package, expectedPackage)
		}
		if packagePassed {
			return fmt.Errorf("malformed go test -json event %d follows terminal package pass", eventCount)
		}

		if event.Test == "" {
			switch event.Action {
			case "start":
				if packageStarted {
					return fmt.Errorf("duplicate package start in go test -json event %d", eventCount)
				}
				packageStarted = true
			case "output":
				if !packageStarted {
					return fmt.Errorf("package output precedes package start in go test -json event %d", eventCount)
				}
			case "pass":
				if !packageStarted {
					return fmt.Errorf("terminal package pass precedes package start in go test -json event %d", eventCount)
				}
				for _, name := range names {
					state := states[name]
					if !state.run || state.terminal != "pass" {
						return fmt.Errorf("terminal package pass precedes terminal pass for requested test %s", name)
					}
				}
				for testName, state := range states {
					if !state.run || state.terminal != "pass" {
						return fmt.Errorf("terminal package pass precedes terminal pass for selected test event %s", testName)
					}
				}
				packagePassed = true
			case "fail", "skip":
				return fmt.Errorf("go test -json reported terminal package %s", event.Action)
			default:
				return fmt.Errorf("malformed package-level go test -json action %q in event %d", event.Action, eventCount)
			}
			continue
		}

		selectedRoot, selected := selectedGoTestEventRoot(event.Test, wanted)
		if !selected {
			return fmt.Errorf("go test -json reported unrequested test event %q", event.Test)
		}
		if !packageStarted {
			return fmt.Errorf("test event %q precedes package start", event.Test)
		}
		state := states[event.Test]
		switch event.Action {
		case "run":
			if state.run || state.terminal != "" {
				return fmt.Errorf("duplicate or out-of-order run for selected test event %s", event.Test)
			}
			if event.Test != selectedRoot {
				parent := states[selectedRoot]
				if !parent.run || parent.terminal != "" {
					return fmt.Errorf("subtest run %s occurs outside active requested test %s", event.Test, selectedRoot)
				}
			}
			state.run = true
		case "pause":
			if !state.run || state.paused || state.terminal != "" {
				return fmt.Errorf("out-of-order pause for selected test event %s", event.Test)
			}
			state.paused = true
		case "cont":
			if !state.run || !state.paused || state.terminal != "" {
				return fmt.Errorf("out-of-order continuation for selected test event %s", event.Test)
			}
			state.paused = false
		case "output":
			if !state.run || state.terminal != "" {
				return fmt.Errorf("out-of-order output for selected test event %s", event.Test)
			}
		case "pass":
			if !state.run || state.paused || state.terminal != "" {
				return fmt.Errorf("duplicate or out-of-order pass for selected test event %s", event.Test)
			}
			if event.Test == selectedRoot {
				for testName, childState := range states {
					if strings.HasPrefix(testName, selectedRoot+"/") && childState.terminal != "pass" {
						return fmt.Errorf("requested test %s passed before selected subtest %s", selectedRoot, testName)
					}
				}
			}
			state.terminal = "pass"
		case "fail", "skip":
			return fmt.Errorf("go test -json reported selected test %s %s", event.Test, event.Action)
		default:
			return fmt.Errorf("malformed go test -json action %q for selected test event %s", event.Action, event.Test)
		}
		states[event.Test] = state
	}
	if eventCount == 0 {
		return fmt.Errorf("go test -json emitted no events")
	}
	if !packagePassed {
		return fmt.Errorf("go test -json omitted terminal package pass")
	}
	for _, name := range names {
		state := states[name]
		if !state.run || state.terminal != "pass" {
			return fmt.Errorf("go test -json did not report exactly one top-level run and terminal pass for %s", name)
		}
	}
	return nil
}

func selectedGoTestEventRoot(testName string, wanted map[string]struct{}) (string, bool) {
	if _, ok := wanted[testName]; ok {
		return testName, true
	}
	for name := range wanted {
		if strings.HasPrefix(testName, name+"/") {
			return name, true
		}
	}
	return "", false
}

func isCmdGoTestName(name string) bool {
	if !strings.HasPrefix(name, "Test") {
		return false
	}
	if len(name) == len("Test") {
		return true
	}
	next, _ := utf8.DecodeRuneInString(name[len("Test"):])
	return !unicode.IsLower(next)
}

func observeSelectedGoTestDeclarations(packageInfo goListTestPackage, names []string) (selectedGoTestDeclarationObservation, error) {
	limits := goTestDeclarationLimits{
		maxFiles:      specAuditGoTestFileLimit,
		maxFileBytes:  specAuditGoTestFileBytes,
		maxTotalBytes: specAuditGoTestTotalBytes,
	}
	return observeSelectedGoTestDeclarationsWithLimits(packageInfo, names, limits)
}

func validatePostRunSelectedTestDeclarations(packageInfo goListTestPackage, names []string, expected selectedGoTestDeclarationObservation) error {
	actual, err := observeSelectedGoTestDeclarations(packageInfo, names)
	if err != nil {
		return fmt.Errorf("post-test selected-test declaration validation: %w", err)
	}
	if !equalSelectedGoTestDeclarationObservations(expected, actual) {
		return fmt.Errorf("post-test selected-test declaration observation changed: %s", selectedTestDeclarationObservationScope)
	}
	return nil
}

func equalSelectedGoTestDeclarationObservations(first, second selectedGoTestDeclarationObservation) bool {
	return first.PackageDir == second.PackageDir &&
		first.ImportPath == second.ImportPath &&
		first.PackageName == second.PackageName &&
		slices.Equal(first.TestGoFiles, second.TestGoFiles) &&
		slices.Equal(first.XTestGoFiles, second.XTestGoFiles) &&
		slices.Equal(first.Files, second.Files)
}

type goTestDeclarationLimits struct {
	maxFiles      int
	maxFileBytes  int64
	maxTotalBytes int64
}

func observeSelectedGoTestDeclarationsWithLimits(packageInfo goListTestPackage, names []string, limits goTestDeclarationLimits) (selectedGoTestDeclarationObservation, error) {
	if err := validateGoListTestPackage(packageInfo); err != nil {
		return selectedGoTestDeclarationObservation{}, err
	}
	wanted, err := selectedGoTestNames(names)
	if err != nil {
		return selectedGoTestDeclarationObservation{}, err
	}
	observation := selectedGoTestDeclarationObservation{
		PackageDir:   packageInfo.Dir,
		ImportPath:   packageInfo.ImportPath,
		PackageName:  packageInfo.Name,
		TestGoFiles:  slices.Clone(packageInfo.TestGoFiles),
		XTestGoFiles: slices.Clone(packageInfo.XTestGoFiles),
	}
	counts := make(map[string]int, len(names))
	files := append(slicesClone(packageInfo.TestGoFiles), packageInfo.XTestGoFiles...)
	if limits.maxFiles < 1 || limits.maxFileBytes < 1 || limits.maxTotalBytes < 1 {
		return selectedGoTestDeclarationObservation{}, fmt.Errorf("invalid build-selected test declaration limits")
	}
	if len(files) > limits.maxFiles {
		return selectedGoTestDeclarationObservation{}, fmt.Errorf("build-selected test declaration file count %d exceeds limit %d", len(files), limits.maxFiles)
	}
	fileSet := token.NewFileSet()
	var totalBytes int64
	for _, filename := range files {
		path := filepath.Join(packageInfo.Dir, filename)
		source, size, digest, err := readBoundedBuildSelectedTestFile(path, limits.maxFileBytes, limits.maxTotalBytes-totalBytes)
		if err != nil {
			return selectedGoTestDeclarationObservation{}, err
		}
		totalBytes += size
		observation.Files = append(observation.Files, selectedGoTestDeclarationFileObservation{Name: filename, SHA256: digest})
		if err := countSelectedGoTestsInFile(fileSet, path, source, wanted, counts); err != nil {
			return selectedGoTestDeclarationObservation{}, err
		}
	}
	for _, name := range names {
		if counts[name] != 1 {
			return selectedGoTestDeclarationObservation{}, fmt.Errorf("build-selected test %s has %d receiverless cmd/go-style declarations; want exactly 1", name, counts[name])
		}
	}
	return observation, nil
}

func selectedGoTestNames(names []string) (map[string]struct{}, error) {
	if len(names) == 0 {
		return nil, fmt.Errorf("at least one SPEC audit test name is required")
	}
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !token.IsIdentifier(name) || !isCmdGoTestName(name) {
			return nil, fmt.Errorf("invalid top-level Go test identifier %q", name)
		}
		if _, exists := wanted[name]; exists {
			return nil, fmt.Errorf("duplicate SPEC audit test name %q", name)
		}
		wanted[name] = struct{}{}
	}
	return wanted, nil
}

func readBoundedBuildSelectedTestFile(path string, maxFileBytes, remainingTotalBytes int64) ([]byte, int64, [sha256.Size]byte, error) {
	return readBoundedBuildSelectedTestFileWithHook(path, maxFileBytes, remainingTotalBytes, nil)
}

func readBoundedBuildSelectedTestFileWithHook(
	path string,
	maxFileBytes int64,
	remainingTotalBytes int64,
	betweenReads func() error,
) (source []byte, size int64, digest [sha256.Size]byte, err error) {
	file, openedInfo, allowed, err := openBoundedBuildSelectedTestFile(path, maxFileBytes, remainingTotalBytes)
	if err != nil {
		return nil, 0, digest, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close build-selected test file %s: %w", path, closeErr)
		}
	}()
	source, err = readLimitedBuildSelectedTestSource(file, allowed)
	if err != nil {
		return nil, 0, digest, fmt.Errorf("read build-selected test file %s: %w", path, err)
	}
	betweenInfo, err := file.Stat()
	if err != nil {
		return nil, 0, digest, fmt.Errorf("restat build-selected test file %s: %w", path, err)
	}
	if betweenReads != nil {
		if err := betweenReads(); err != nil {
			return nil, 0, digest, fmt.Errorf("mutate build-selected test file %s between reads: %w", path, err)
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, digest, fmt.Errorf("rewind build-selected test file %s: %w", path, err)
	}
	second, err := readLimitedBuildSelectedTestSource(file, allowed)
	if err != nil {
		return nil, 0, digest, fmt.Errorf("reread build-selected test file %s: %w", path, err)
	}
	postInfo, err := file.Stat()
	if err != nil {
		return nil, 0, digest, fmt.Errorf("post-stat build-selected test file %s: %w", path, err)
	}
	postPathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, 0, digest, fmt.Errorf("post-lstat build-selected test file %s: %w", path, err)
	}
	if !stableBuildSelectedTestRead(openedInfo, betweenInfo, postInfo, postPathInfo, source, second) {
		return nil, 0, digest, fmt.Errorf("build-selected test file %s changed while it was read", path)
	}
	digest = sha256.Sum256(source)
	return source, int64(len(source)), digest, nil
}

func openBoundedBuildSelectedTestFile(path string, maxFileBytes, remainingTotalBytes int64) (*os.File, os.FileInfo, int64, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("lstat build-selected test file %s: %w", path, err)
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, nil, 0, fmt.Errorf("build-selected test file %s is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("open build-selected test file %s: %w", path, err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, nil, 0, closeBuildSelectedTestFileAfterError(file, fmt.Errorf("stat build-selected test file %s: %w", path, err))
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(pathInfo, openedInfo) {
		return nil, nil, 0, closeBuildSelectedTestFileAfterError(file, fmt.Errorf("build-selected test file %s changed before it could be read", path))
	}
	if openedInfo.Size() > maxFileBytes {
		return nil, nil, 0, closeBuildSelectedTestFileAfterError(file, fmt.Errorf("build-selected test file %s size %d exceeds per-file limit %d", path, openedInfo.Size(), maxFileBytes))
	}
	if openedInfo.Size() > remainingTotalBytes {
		return nil, nil, 0, closeBuildSelectedTestFileAfterError(file, fmt.Errorf("build-selected test sources exceed aggregate limit"))
	}
	return file, openedInfo, min(maxFileBytes, remainingTotalBytes), nil
}

func closeBuildSelectedTestFileAfterError(file *os.File, primary error) error {
	if err := file.Close(); err != nil {
		return errors.Join(primary, fmt.Errorf("close test file: %w", err))
	}
	return primary
}

func readLimitedBuildSelectedTestSource(file *os.File, allowed int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(file, allowed+1))
}

func stableBuildSelectedTestRead(
	openedInfo os.FileInfo,
	betweenInfo os.FileInfo,
	postInfo os.FileInfo,
	postPathInfo os.FileInfo,
	first []byte,
	second []byte,
) bool {
	return int64(len(first)) == openedInfo.Size() &&
		int64(len(second)) == openedInfo.Size() &&
		stableFileInfo(openedInfo, betweenInfo) &&
		stableFileInfo(openedInfo, postInfo) &&
		postPathInfo.Mode().IsRegular() &&
		os.SameFile(openedInfo, postPathInfo) &&
		bytes.Equal(first, second)
}

func stableFileInfo(first, second os.FileInfo) bool {
	return os.SameFile(first, second) &&
		first.Mode() == second.Mode() &&
		first.Size() == second.Size() &&
		first.ModTime().Equal(second.ModTime())
}

func countSelectedGoTestsInFile(fileSet *token.FileSet, path string, source []byte, wanted map[string]struct{}, counts map[string]int) error {
	parsed, err := parser.ParseFile(fileSet, path, source, parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse build-selected test file %s: %w", path, err)
	}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := function.Name.Name
		if name == "TestMain" && function.Recv == nil {
			return fmt.Errorf("%s declares TestMain, which is not a selectable regression", path)
		}
		if _, selected := wanted[name]; !selected {
			continue
		}
		if function.Recv != nil {
			return fmt.Errorf("%s declares %s as a method, not a top-level test", path, name)
		}
		if !isCmdGoTestFunc(function, "T") {
			return fmt.Errorf("%s declares %s with the wrong test signature", path, name)
		}
		counts[name]++
	}
	return nil
}

func isCmdGoTestFunc(function *ast.FuncDecl, argument string) bool {
	if function.Type.Results != nil && len(function.Type.Results.List) > 0 ||
		function.Type.Params == nil ||
		len(function.Type.Params.List) != 1 ||
		len(function.Type.Params.List[0].Names) > 1 ||
		function.Type.TypeParams != nil && len(function.Type.TypeParams.List) > 0 {
		return false
	}
	pointer, ok := function.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch parameter := pointer.X.(type) {
	case *ast.Ident:
		return parameter.Name == argument
	case *ast.SelectorExpr:
		return parameter.Sel.Name == argument
	default:
		return false
	}
}

func newSpecAuditGoChildRuntime() (specAuditGoChildRuntime, error) {
	if !specAuditProcessGroupsSupported() {
		return specAuditGoChildRuntime{}, fmt.Errorf("trusted SPEC audit Go tests require isolated process-group cleanup on darwin or linux")
	}
	taskRoot, err := os.MkdirTemp("", "dear-agent-specaudit-go-")
	if err != nil {
		return specAuditGoChildRuntime{}, fmt.Errorf("create task-owned SPEC audit Go runtime: %w", err)
	}
	canonicalTaskRoot, err := filepath.EvalSymlinks(taskRoot)
	if err != nil {
		return specAuditGoChildRuntime{}, errors.Join(
			fmt.Errorf("resolve canonical task-owned SPEC audit Go runtime: %w", err),
			os.RemoveAll(taskRoot),
		)
	}
	canonicalTaskRoot, err = filepath.Abs(canonicalTaskRoot)
	if err != nil {
		return specAuditGoChildRuntime{}, errors.Join(
			fmt.Errorf("resolve absolute task-owned SPEC audit Go runtime: %w", err),
			os.RemoveAll(taskRoot),
		)
	}
	taskRootInfo, taskRootOwner, err := validateNewSpecAuditTaskRoot(canonicalTaskRoot)
	if err != nil {
		return specAuditGoChildRuntime{}, errors.Join(err, os.RemoveAll(taskRoot))
	}
	childRuntime := specAuditGoChildRuntime{
		taskRoot:            canonicalTaskRoot,
		taskRootIdentity:    taskRootInfo,
		taskRootOwner:       taskRootOwner,
		taskRootPermissions: taskRootInfo.Mode().Perm(),
	}
	fail := func(primary error) (specAuditGoChildRuntime, error) {
		return specAuditGoChildRuntime{}, errors.Join(primary, childRuntime.cleanup())
	}
	for _, directory := range []string{"home", "tmp", "gopath", "gocache"} {
		if err := os.Mkdir(filepath.Join(canonicalTaskRoot, directory), 0o700); err != nil {
			return fail(fmt.Errorf("create task-owned %s directory: %w", directory, err))
		}
	}
	goExecutable, err := captureTrustedSpecAuditExecutable("go")
	if err != nil {
		return fail(err)
	}
	gitExecutable, err := captureTrustedSpecAuditExecutable("git")
	if err != nil {
		return fail(err)
	}
	goRoot := filepath.Dir(filepath.Dir(goExecutable.path))
	goPaths := filepath.SplitList(build.Default.GOPATH)
	if len(goPaths) == 0 || goPaths[0] == "" {
		return fail(fmt.Errorf("resolve standard Go module cache: GOPATH is empty"))
	}
	moduleCache := filepath.Join(goPaths[0], "pkg", "mod")
	// GOMODCACHE is a narrowly validated shared trusted input, not immutable or
	// sandboxed storage. Network lookup is disabled and -mod=readonly is forced,
	// so a missing cached dependency fails the child instead of being fetched.
	for label, directory := range map[string]string{
		"GOROOT":     goRoot,
		"GOMODCACHE": moduleCache,
	} {
		if err := validateNarrowGoDirectory(label, directory); err != nil {
			return fail(err)
		}
	}
	// Put the captured Git directory first so a same-named executable in the Go
	// tool directory cannot shadow the identity that was validated for children.
	pathEntries := []string{filepath.Dir(gitExecutable.path), filepath.Dir(goExecutable.path)}
	pathEntries = slices.Compact(pathEntries)
	childRuntime.goExecutable = goExecutable
	childRuntime.gitExecutable = gitExecutable
	childRuntime.environment = []string{
		"PATH=" + strings.Join(pathEntries, string(os.PathListSeparator)),
		"HOME=" + filepath.Join(canonicalTaskRoot, "home"),
		"TMPDIR=" + filepath.Join(canonicalTaskRoot, "tmp"),
		"TMP=" + filepath.Join(canonicalTaskRoot, "tmp"),
		"TEMP=" + filepath.Join(canonicalTaskRoot, "tmp"),
		"GOROOT=" + goRoot,
		"GOPATH=" + filepath.Join(canonicalTaskRoot, "gopath"),
		"GOMODCACHE=" + moduleCache,
		"GOCACHE=" + filepath.Join(canonicalTaskRoot, "gocache"),
		"GOENV=off",
		"GOFLAGS=",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOVCS=*:off",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
		"GOTELEMETRY=off",
		"CGO_ENABLED=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ALLOW_PROTOCOL=file",
		"LANG=C",
		"LC_ALL=C",
	}
	return childRuntime, nil
}

func validateNewSpecAuditTaskRoot(path string) (os.FileInfo, uint32, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || !strings.HasPrefix(filepath.Base(path), "dear-agent-specaudit-go-") {
		return nil, 0, fmt.Errorf("invalid canonical SPEC audit task root %q", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve SPEC audit task root %q: %w", path, err)
	}
	if resolved != path {
		return nil, 0, fmt.Errorf("SPEC audit task root %q is not canonical", path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("lstat SPEC audit task root %q: %w", path, err)
	}
	owner, ok := fileOwner(info)
	effectiveUID, effectiveUIDOK := currentEffectiveUID()
	if !ok || !effectiveUIDOK || owner != effectiveUID || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return nil, 0, fmt.Errorf("SPEC audit task root %q must be a current-user-owned 0700 directory", path)
	}
	return info, owner, nil
}

func captureTrustedSpecAuditExecutable(name string) (trustedSpecAuditExecutable, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return trustedSpecAuditExecutable{}, fmt.Errorf("resolve required %s executable: %w", name, err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return trustedSpecAuditExecutable{}, fmt.Errorf("resolve absolute %s executable: %w", name, err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return trustedSpecAuditExecutable{}, fmt.Errorf("resolve required %s executable symlinks: %w", name, err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return trustedSpecAuditExecutable{}, fmt.Errorf("resolve canonical absolute %s executable: %w", name, err)
	}
	identity := trustedSpecAuditExecutable{path: filepath.Clean(path)}
	if identity.path != path {
		return trustedSpecAuditExecutable{}, fmt.Errorf("required %s executable path %q is not clean", name, path)
	}
	info, err := validateTrustedSpecAuditExecutable(name, identity, false)
	if err != nil {
		return trustedSpecAuditExecutable{}, err
	}
	identity.identity = info
	return identity, nil
}

func (childRuntime specAuditGoChildRuntime) revalidateExecutables() error {
	if _, err := validateTrustedSpecAuditExecutable("go", childRuntime.goExecutable, true); err != nil {
		return err
	}
	if _, err := validateTrustedSpecAuditExecutable("git", childRuntime.gitExecutable, true); err != nil {
		return err
	}
	return nil
}

func validateTrustedSpecAuditExecutable(name string, executable trustedSpecAuditExecutable, requireSameIdentity bool) (os.FileInfo, error) {
	if executable.path == "" || !filepath.IsAbs(executable.path) || filepath.Clean(executable.path) != executable.path {
		return nil, fmt.Errorf("required %s executable path %q is not clean and absolute", name, executable.path)
	}
	resolved, err := filepath.EvalSymlinks(executable.path)
	if err != nil {
		return nil, fmt.Errorf("resolve required %s executable %q: %w", name, executable.path, err)
	}
	if resolved != executable.path {
		return nil, fmt.Errorf("required %s executable path %q is no longer canonical", name, executable.path)
	}
	info, err := os.Lstat(executable.path)
	if err != nil {
		return nil, fmt.Errorf("lstat required %s executable: %w", name, err)
	}
	if requireSameIdentity && (executable.identity == nil || !os.SameFile(executable.identity, info)) {
		return nil, fmt.Errorf("required %s executable %q was replaced after validation", name, executable.path)
	}
	if !trustedFileOwner(info) || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("required %s executable %q is not a narrowly trusted executable", name, executable.path)
	}
	if err := validateTrustedCanonicalAncestry(name, filepath.Dir(executable.path)); err != nil {
		return nil, err
	}
	return info, nil
}

func validateTrustedCanonicalAncestry(label, directory string) error {
	effectiveUID, effectiveUIDOK := currentEffectiveUID()
	if !effectiveUIDOK {
		return fmt.Errorf("resolve current effective UID for %s executable ancestry", label)
	}
	for {
		info, err := os.Lstat(directory)
		if err != nil {
			return fmt.Errorf("lstat %s executable ancestry %q: %w", label, directory, err)
		}
		owner, ownerOK := fileOwner(info)
		groupWritableByAnotherOwner := info.Mode().Perm()&0o020 != 0 && owner != effectiveUID
		if !ownerOK || !trustedFileOwner(info) || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o002 != 0 || groupWritableByAnotherOwner {
			return fmt.Errorf("%s executable ancestry %q is not a narrowly trusted directory", label, directory)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return nil
		}
		directory = parent
	}
}

func fileOwner(info os.FileInfo) (uint32, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}

func trustedFileOwner(info os.FileInfo) bool {
	owner, ok := fileOwner(info)
	effectiveUID, effectiveUIDOK := currentEffectiveUID()
	return ok && effectiveUIDOK && (owner == 0 || owner == effectiveUID)
}

func currentEffectiveUID() (uint32, bool) {
	uid := os.Geteuid()
	if uid < 0 || uint64(uid) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(uid), true
}

func validateNarrowGoDirectory(label, path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%s path %q is not clean and absolute", label, path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("lstat %s path %q: %w", label, path, err)
	}
	if !trustedFileOwner(info) || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s path %q is not a narrowly trusted directory", label, path)
	}
	return nil
}

//nolint:gocyclo // All identity, ownership, and permission checks must precede the narrow task-root deletion.
func (childRuntime specAuditGoChildRuntime) cleanup() error {
	if childRuntime.taskRoot == "" {
		return nil
	}
	if !filepath.IsAbs(childRuntime.taskRoot) || filepath.Clean(childRuntime.taskRoot) != childRuntime.taskRoot || !strings.HasPrefix(filepath.Base(childRuntime.taskRoot), "dear-agent-specaudit-go-") {
		return fmt.Errorf("refuse to remove invalid SPEC audit task root %q", childRuntime.taskRoot)
	}
	resolved, err := filepath.EvalSymlinks(childRuntime.taskRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("refuse to remove missing SPEC audit task root %q: %w", childRuntime.taskRoot, err)
		}
		return fmt.Errorf("refuse to remove unresolved SPEC audit task root %q: %w", childRuntime.taskRoot, err)
	}
	if resolved != childRuntime.taskRoot {
		return fmt.Errorf("refuse to remove non-canonical SPEC audit task root %q", childRuntime.taskRoot)
	}
	info, err := os.Lstat(childRuntime.taskRoot)
	if err != nil {
		return fmt.Errorf("refuse to remove missing or unreadable SPEC audit task root %q: %w", childRuntime.taskRoot, err)
	}
	owner, ownerOK := fileOwner(info)
	effectiveUID, effectiveUIDOK := currentEffectiveUID()
	if childRuntime.taskRootIdentity == nil || !os.SameFile(childRuntime.taskRootIdentity, info) {
		return fmt.Errorf("refuse to remove replaced SPEC audit task root %q", childRuntime.taskRoot)
	}
	if !ownerOK || !effectiveUIDOK || owner != effectiveUID || owner != childRuntime.taskRootOwner {
		return fmt.Errorf("refuse to remove wrong-owner SPEC audit task root %q", childRuntime.taskRoot)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 || info.Mode().Perm() != childRuntime.taskRootPermissions {
		return fmt.Errorf("refuse to remove wrong-mode SPEC audit task root %q", childRuntime.taskRoot)
	}
	if err := os.RemoveAll(childRuntime.taskRoot); err != nil {
		return fmt.Errorf("remove SPEC audit task root %q: %w", childRuntime.taskRoot, err)
	}
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
