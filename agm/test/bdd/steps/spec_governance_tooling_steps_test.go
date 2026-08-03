package steps

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestSpecAuditGoTestCommandIsBoundedAndGroupCancelable(t *testing.T) {
	t.Setenv("GOFLAGS", "-race")
	childRuntime := newSpecAuditRunnerTestRuntime(t)
	command, err := newSpecAuditGoTestCommand(packageSpecBDDRepoRoot(), childRuntime, "TestExample")
	if err != nil {
		t.Fatalf("newSpecAuditGoTestCommand() error = %v", err)
	}

	if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
		t.Fatal("nested SPEC audit test must run in an isolated process group")
	}
	if command.Cancel != nil {
		t.Fatal("nested SPEC audit test must delegate cancellation to the bounded runner")
	}
	if command.WaitDelay != time.Second {
		t.Fatalf("nested SPEC audit test WaitDelay = %v, want %v", command.WaitDelay, time.Second)
	}
	if !slices.Contains(command.Args, "-timeout="+specAuditGoTestTimeout) {
		t.Fatalf("nested SPEC audit test args %q omit the inner test timeout", command.Args)
	}
	if !slices.Contains(command.Args, "-mod=readonly") {
		t.Fatalf("nested SPEC audit test args %q omit module-readonly enforcement", command.Args)
	}
	if !slices.Contains(command.Args, "-json") || slices.Contains(command.Args, "-v") {
		t.Fatalf("nested SPEC audit test args %q must use machine-readable JSON without redundant verbose output", command.Args)
	}
	if !slices.Contains(command.Args, "^(?:TestExample)$") {
		t.Fatalf("nested SPEC audit test args %q omit the exact selected test", command.Args)
	}
	if !slices.Contains(command.Env, "GOFLAGS=") || slices.Contains(command.Env, "GOFLAGS=-race") {
		t.Fatalf("nested SPEC audit test environment did not clear inherited GOFLAGS: %q", command.Env)
	}
}

func TestSpecAuditGoTestRunnerRejectsZeroMatch(t *testing.T) {
	state := &specGovernanceToolingState{repoRoot: packageSpecBDDRepoRoot()}
	ctx := context.WithValue(context.Background(), specGovernanceToolingStateKey{}, state)
	if err := runSpecAuditGoTests(ctx, "TestSpecAuditGoTestRunnerMustNotExist"); err != nil {
		t.Fatalf("runSpecAuditGoTests() error = %v, want deferred BDD state failure", err)
	}
	if state.err == nil {
		t.Fatalf("zero-match run unexpectedly succeeded; output:\n%s", state.output)
	}
	if !strings.Contains(state.err.Error(), "want exactly 1") {
		t.Fatalf("zero-match error = %v, want exact declaration rejection", state.err)
	}
}

func TestSpecAuditGoTestRunnerAcceptsGroupedCurrentSelection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	packageInfo, listCommand, output, err := resolveBuildSelectedGoTestPackage(
		ctx,
		packageSpecBDDRepoRoot(),
		"./tools/specaudit",
		filepath.Join(packageSpecBDDRepoRoot(), "tools", "specaudit"),
		newSpecAuditRunnerTestRuntime(t),
	)
	if err != nil {
		t.Fatalf("resolveBuildSelectedGoTestPackage() error = %v\n%s", err, output.String())
	}
	if !slices.Contains(listCommand.Args, "-mod=readonly") {
		t.Fatalf("SPEC audit go list args %q omit module-readonly enforcement", listCommand.Args)
	}
	if _, err := observeSelectedGoTestDeclarations(packageInfo, []string{
		"TestInventoryReadsPinnedRevisionAndProducesSeedsDeterministically",
		"TestInventoryReportsFeatureFirstDiagnosticsFromPinnedObjects",
	}); err != nil {
		t.Fatalf("grouped current selection was rejected: %v", err)
	}
}

func TestSpecAuditGoTestRunnerEndToEnd(t *testing.T) {
	state := &specGovernanceToolingState{repoRoot: packageSpecBDDRepoRoot()}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, specGovernanceToolingStateKey{}, state)
	if err := runSpecAuditGoTests(ctx, "TestGitOutputIsBounded"); err != nil {
		t.Fatalf("runSpecAuditGoTests() error = %v", err)
	}
	if state.err != nil {
		t.Fatalf("end-to-end SPEC audit test runner failed: %v\n%s", state.err, state.output)
	}
}

func TestBoundedSpecAuditOutputCapEdges(t *testing.T) {
	t.Run("exact limit", func(t *testing.T) {
		var callbacks atomic.Int32
		output := &boundedSpecAuditOutput{limit: 3, onLimit: func() { callbacks.Add(1) }}
		if written, err := output.Write([]byte("abc")); err != nil || written != 3 {
			t.Fatalf("Write() = (%d, %v), want (3, nil)", written, err)
		}
		if output.Truncated() || callbacks.Load() != 0 {
			t.Fatalf("exact-limit write truncated=%v callbacks=%d, want false/0", output.Truncated(), callbacks.Load())
		}
		if got := output.String(); got != "abc" {
			t.Fatalf("String() = %q, want exact retained output", got)
		}
	})

	t.Run("one over", func(t *testing.T) {
		var callbacks atomic.Int32
		output := &boundedSpecAuditOutput{limit: 3, onLimit: func() { callbacks.Add(1) }}
		if written, err := output.Write([]byte("abcd")); err != nil || written != 4 {
			t.Fatalf("Write() = (%d, %v), want (4, nil)", written, err)
		}
		if got := output.String(); got != "abc" {
			t.Fatalf("String() = %q, want capped output", got)
		}
		if !output.Truncated() || callbacks.Load() != 1 {
			t.Fatalf("one-over write truncated=%v callbacks=%d, want true/1", output.Truncated(), callbacks.Load())
		}
	})

	t.Run("zero", func(t *testing.T) {
		var callbacks atomic.Int32
		output := &boundedSpecAuditOutput{limit: 0, onLimit: func() { callbacks.Add(1) }}
		if written, err := output.Write([]byte("x")); err != nil || written != 1 {
			t.Fatalf("Write() = (%d, %v), want (1, nil)", written, err)
		}
		if got := output.String(); got != "" {
			t.Fatalf("String() = %q, want empty bounded prefix", got)
		}
		if !output.Truncated() || callbacks.Load() != 1 {
			t.Fatalf("zero-limit write truncated=%v callbacks=%d, want true/1", output.Truncated(), callbacks.Load())
		}
	})

	t.Run("concurrent callback once", func(t *testing.T) {
		var callbacks atomic.Int32
		output := &boundedSpecAuditOutput{limit: 1, onLimit: func() { callbacks.Add(1) }}
		var writers sync.WaitGroup
		for range 32 {
			writers.Go(func() {
				if _, err := output.Write([]byte("xx")); err != nil {
					t.Errorf("Write() error = %v", err)
				}
			})
		}
		writers.Wait()
		if got := len(output.String()); got != 1 {
			t.Fatalf("retained output length = %d, want 1", got)
		}
		if !output.Truncated() || callbacks.Load() != 1 {
			t.Fatalf("concurrent writes truncated=%v callbacks=%d, want true/1", output.Truncated(), callbacks.Load())
		}
	})
}

func TestBoundedSpecAuditOutputCancelsNoisyChildPromptly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.Command(os.Args[0], "-test.run=^TestSpecAuditGoTestRunnerNoisyHelper$")
	command.Env = append(os.Environ(), "DEAR_AGENT_SPECAUDIT_NOISY_HELPER=1")
	configureSpecAuditChildCommand(command)

	started := time.Now()
	output, err := runBoundedSpecAuditCommand(ctx, command, 256)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("noisy child unexpectedly completed without process-group cancellation")
	}
	if ctx.Err() != nil {
		t.Fatalf("output cap did not stop noisy child before context deadline: %v", ctx.Err())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("output cap cancellation took %v, want prompt termination", elapsed)
	}
	if !output.Truncated() {
		t.Fatal("noisy child output did not trip the output cap")
	}
	if got := output.String(); len(got) != 256 {
		t.Fatalf("retained output length = %d, want bounded 256-byte prefix", len(got))
	}
}

func TestSpecAuditGoTestRunnerNoisyHelper(t *testing.T) {
	if os.Getenv("DEAR_AGENT_SPECAUDIT_NOISY_HELPER") != "1" {
		return
	}
	payload := bytes.Repeat([]byte("x"), 4096)
	for {
		if _, err := os.Stdout.Write(payload); err != nil {
			return
		}
	}
}

func TestSuccessfulCommandCleansSilentDescendant(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.Command(os.Args[0], "-test.run=^TestSpecAuditSuccessfulDescendantHelper$")
	command.Env = append(os.Environ(),
		"DEAR_AGENT_SPECAUDIT_DESCENDANT_HELPER=1",
		"DEAR_AGENT_SPECAUDIT_DESCENDANT_PID_FILE="+pidFile,
	)
	configureSpecAuditChildCommand(command)
	output, err := runBoundedSpecAuditCommand(ctx, command, 4096)
	if err != nil {
		t.Fatalf("successful descendant helper failed: %v\n%s", err, output.String())
	}
	payload, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("ReadFile(descendant PID) error = %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		t.Fatalf("parse descendant PID %q: %v", payload, err)
	}
	deadline := time.Now().Add(specAuditProcessGroupWait)
	for {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) || (errors.Is(err, syscall.EPERM) && runtime.GOOS == "darwin") {
			break
		}
		if err != nil {
			t.Fatalf("probe silent background descendant %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("silent background descendant %d remains after successful command cleanup", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBoundedSpecAuditCommandPreservesDirectChildExitStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.Command(os.Args[0], "-test.run=^TestSpecAuditExitStatusHelper$")
	command.Env = append(os.Environ(), "DEAR_AGENT_SPECAUDIT_EXIT_STATUS_HELPER=1")
	configureSpecAuditChildCommand(command)

	output, err := runBoundedSpecAuditCommand(ctx, command, 4096)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("runBoundedSpecAuditCommand() error = %v, want *exec.ExitError\n%s", err, output.String())
	}
	if got := exitErr.ExitCode(); got != 23 {
		t.Fatalf("direct child exit code = %d, want 23", got)
	}
}

func TestBoundedSpecAuditCommandAcceptsSuccessfulChildWithoutDescendants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.Command("/bin/sh", "-c", "exit 0")
	configureSpecAuditChildCommand(command)

	output, err := runBoundedSpecAuditCommand(ctx, command, 4096)
	if err != nil {
		t.Fatalf("runBoundedSpecAuditCommand() error = %v\n%s", err, output.String())
	}
	if command.Cancel != nil {
		t.Fatal("bounded runner installed exec.Cmd cancellation outside its owned lifecycle")
	}
}

func TestSpecAuditProcessGroupLifecycleRejectsLateCancellation(t *testing.T) {
	lifecycle := newSpecAuditProcessGroupLifecycle(&exec.Cmd{})
	if err := lifecycle.complete(false, true); err != nil {
		t.Fatalf("complete sealed lifecycle: %v", err)
	}
	var cancellations sync.WaitGroup
	for range 64 {
		cancellations.Go(func() {
			for range 100 {
				if err := lifecycle.cancel(); !errors.Is(err, os.ErrProcessDone) {
					t.Errorf("late lifecycle cancel error = %v, want os.ErrProcessDone", err)
				}
			}
		})
	}
	cancellations.Wait()
}

func TestBoundedSpecAuditCommandCancelsOnContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	command := exec.Command(os.Args[0], "-test.run=^TestSpecAuditSilentGrandchild$")
	command.Env = append(os.Environ(), "DEAR_AGENT_SPECAUDIT_SILENT_GRANDCHILD=1")
	configureSpecAuditChildCommand(command)

	started := time.Now()
	_, err := runBoundedSpecAuditCommand(ctx, command, 4096)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context-canceled command error = %v, want context deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("context cancellation took %v, want prompt process-group termination", elapsed)
	}
}

func TestBoundedSpecAuditCommandRejectsNilContext(t *testing.T) {
	command := exec.Command("/bin/sh", "-c", "exit 0")
	configureSpecAuditChildCommand(command)
	var nilContext context.Context
	_, err := runBoundedSpecAuditCommand(nilContext, command, 4096)
	if err == nil || !strings.Contains(err.Error(), "non-nil context") {
		t.Fatalf("nil-context error = %v, want explicit rejection", err)
	}
}

func TestSpecAuditExitStatusHelper(t *testing.T) {
	if os.Getenv("DEAR_AGENT_SPECAUDIT_EXIT_STATUS_HELPER") != "1" {
		return
	}
	os.Exit(23)
}

func TestSpecAuditSuccessfulDescendantHelper(t *testing.T) {
	if os.Getenv("DEAR_AGENT_SPECAUDIT_DESCENDANT_HELPER") != "1" {
		return
	}
	device, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("OpenFile(os.DevNull) error = %v", err)
	}
	defer func() {
		if err := device.Close(); err != nil {
			t.Errorf("close os.DevNull: %v", err)
		}
	}()
	grandchild := exec.Command(os.Args[0], "-test.run=^TestSpecAuditSilentGrandchild$")
	grandchild.Env = append(os.Environ(), "DEAR_AGENT_SPECAUDIT_SILENT_GRANDCHILD=1")
	grandchild.Stdin = device
	grandchild.Stdout = device
	grandchild.Stderr = device
	if err := grandchild.Start(); err != nil {
		t.Fatalf("start silent background descendant: %v", err)
	}
	pidFile := os.Getenv("DEAR_AGENT_SPECAUDIT_DESCENDANT_PID_FILE")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write silent background descendant PID: %v", err)
	}
}

func TestSpecAuditSilentGrandchild(t *testing.T) {
	if os.Getenv("DEAR_AGENT_SPECAUDIT_SILENT_GRANDCHILD") != "1" {
		return
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestBuildExactGoTestRunPattern(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		want    string
		wantErr bool
	}{
		{
			name:  "grouped exact names",
			names: []string{"TestOne", "TestTwo"},
			want:  "^(?:TestOne|TestTwo)$",
		},
		{
			name:    "empty selection",
			wantErr: true,
		},
		{
			name:    "duplicate",
			names:   []string{"TestOne", "TestOne"},
			wantErr: true,
		},
		{
			name:    "TestMain",
			names:   []string{"TestMain"},
			wantErr: true,
		},
		{
			name:    "subtest expression",
			names:   []string{"TestOne/subtest"},
			wantErr: true,
		},
		{
			name:    "lowercase suffix",
			names:   []string{"Testtarget"},
			wantErr: true,
		},
		{
			name:    "non-test identifier",
			names:   []string{"BenchmarkTarget"},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := buildExactGoTestRunPattern(test.names)
			if (err != nil) != test.wantErr {
				t.Fatalf("buildExactGoTestRunPattern() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("buildExactGoTestRunPattern() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidateExactGoTestJSONRequiresOneRunAndTerminalPassPerName(t *testing.T) {
	const packagePath = "github.com/vbonnet/dear-agent/tools/specaudit"
	validOne := strings.Join([]string{
		`{"Action":"start","Package":"` + packagePath + `"}`,
		`{"Action":"run","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"output","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"pass","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"output","Package":"` + packagePath + `"}`,
		`{"Action":"pass","Package":"` + packagePath + `"}`,
	}, "\n") + "\n"
	validTwo := strings.Join([]string{
		`{"Action":"start","Package":"` + packagePath + `"}`,
		`{"Action":"run","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"pass","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"run","Package":"` + packagePath + `","Test":"TestTwo"}`,
		`{"Action":"pass","Package":"` + packagePath + `","Test":"TestTwo"}`,
		`{"Action":"pass","Package":"` + packagePath + `"}`,
	}, "\n") + "\n"
	validParallelSubtest := strings.Join([]string{
		`{"Action":"start","Package":"` + packagePath + `"}`,
		`{"Action":"run","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"pause","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"cont","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"run","Package":"` + packagePath + `","Test":"TestOne/subtest"}`,
		`{"Action":"pass","Package":"` + packagePath + `","Test":"TestOne/subtest"}`,
		`{"Action":"pass","Package":"` + packagePath + `","Test":"TestOne"}`,
		`{"Action":"pass","Package":"` + packagePath + `"}`,
	}, "\n") + "\n"

	tests := []struct {
		name        string
		output      string
		selected    []string
		wantErrPart string
	}{
		{name: "one exact pass", output: validOne, selected: []string{"TestOne"}},
		{name: "two exact passes", output: validTwo, selected: []string{"TestOne", "TestTwo"}},
		{name: "parallel test and subtest pass", output: validParallelSubtest, selected: []string{"TestOne"}},
		{
			name:        "missing requested test",
			output:      validOne,
			selected:    []string{"TestOne", "TestTwo"},
			wantErrPart: "precedes terminal pass for requested test TestTwo",
		},
		{
			name: "duplicate top-level run",
			output: strings.Replace(validOne,
				`{"Action":"run","Package":"`+packagePath+`","Test":"TestOne"}`,
				strings.Join([]string{
					`{"Action":"run","Package":"` + packagePath + `","Test":"TestOne"}`,
					`{"Action":"run","Package":"` + packagePath + `","Test":"TestOne"}`,
				}, "\n"), 1),
			selected: []string{"TestOne"}, wantErrPart: "duplicate or out-of-order run",
		},
		{
			name: "skip despite zero process error",
			output: strings.Replace(validOne,
				`{"Action":"pass","Package":"`+packagePath+`","Test":"TestOne"}`,
				`{"Action":"skip","Package":"`+packagePath+`","Test":"TestOne"}`, 1),
			selected: []string{"TestOne"}, wantErrPart: "TestOne skip",
		},
		{
			name: "fail despite zero process error",
			output: strings.Replace(validOne,
				`{"Action":"pass","Package":"`+packagePath+`","Test":"TestOne"}`,
				`{"Action":"fail","Package":"`+packagePath+`","Test":"TestOne"}`, 1),
			selected: []string{"TestOne"}, wantErrPart: "TestOne fail",
		},
		{
			name: "duplicate terminal pass",
			output: strings.Replace(validOne,
				`{"Action":"pass","Package":"`+packagePath+`","Test":"TestOne"}`,
				strings.Join([]string{
					`{"Action":"pass","Package":"` + packagePath + `","Test":"TestOne"}`,
					`{"Action":"pass","Package":"` + packagePath + `","Test":"TestOne"}`,
				}, "\n"), 1),
			selected: []string{"TestOne"}, wantErrPart: "duplicate or out-of-order pass",
		},
		{
			name: "terminal pass before run",
			output: strings.Join([]string{
				`{"Action":"start","Package":"` + packagePath + `"}`,
				`{"Action":"pass","Package":"` + packagePath + `","Test":"TestOne"}`,
				`{"Action":"pass","Package":"` + packagePath + `"}`,
			}, "\n") + "\n",
			selected: []string{"TestOne"}, wantErrPart: "duplicate or out-of-order pass",
		},
		{
			name:     "unrequested test",
			output:   strings.ReplaceAll(validOne, "TestOne", "TestOther"),
			selected: []string{"TestOne"}, wantErrPart: "unrequested test event",
		},
		{
			name:     "malformed JSON",
			output:   validOne + `{not-json}`,
			selected: []string{"TestOne"}, wantErrPart: "decode bounded go test -json event",
		},
		{
			name: "missing package terminal",
			output: strings.TrimSuffix(validOne,
				`{"Action":"pass","Package":"`+packagePath+`"}`+"\n"),
			selected: []string{"TestOne"}, wantErrPart: "omitted terminal package pass",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateExactGoTestJSON(test.output, packagePath, test.selected)
			if test.wantErrPart == "" {
				if err != nil {
					t.Fatalf("validateExactGoTestJSON() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErrPart) {
				t.Fatalf("validateExactGoTestJSON() error = %v, want containing %q", err, test.wantErrPart)
			}
		})
	}
}

func TestValidateGoListTestPackage(t *testing.T) {
	absolute := t.TempDir()
	tests := []struct {
		name        string
		packageInfo goListTestPackage
		wantErr     bool
	}{
		{name: "valid", packageInfo: goListTestPackage{Dir: absolute, ImportPath: "example.com/target", Name: "target", TestGoFiles: []string{"main_test.go"}}},
		{name: "relative directory", packageInfo: goListTestPackage{Dir: "relative", ImportPath: "example.com/target", Name: "target"}, wantErr: true},
		{name: "traversal filename", packageInfo: goListTestPackage{Dir: absolute, ImportPath: "example.com/target", Name: "target", TestGoFiles: []string{"../main_test.go"}}, wantErr: true},
		{name: "duplicate filename", packageInfo: goListTestPackage{Dir: absolute, ImportPath: "example.com/target", Name: "target", TestGoFiles: []string{"main_test.go"}, XTestGoFiles: []string{"main_test.go"}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateGoListTestPackage(test.packageInfo); (err != nil) != test.wantErr {
				t.Fatalf("validateGoListTestPackage() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestObserveSelectedGoTestDeclarationsRejectsInvalidDeclarations(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		selected []string
		wantErr  bool
	}{
		{
			name:     "valid",
			files:    map[string]string{"valid_test.go": "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) {}\n"},
			selected: []string{"TestTarget"},
		},
		{
			name:     "missing",
			files:    map[string]string{"missing_test.go": "package target\nimport \"testing\"\nfunc TestOther(t *testing.T) {}\n"},
			selected: []string{"TestTarget"},
			wantErr:  true,
		},
		{
			name:     "near match",
			files:    map[string]string{"near_test.go": "package target\nimport \"testing\"\nfunc TestTargetExtra(t *testing.T) {}\n"},
			selected: []string{"TestTarget"},
			wantErr:  true,
		},
		{
			name:     "method",
			files:    map[string]string{"method_test.go": "package target\nimport \"testing\"\ntype suite struct{}\nfunc (suite) TestTarget(t *testing.T) {}\n"},
			selected: []string{"TestTarget"},
			wantErr:  true,
		},
		{
			name:     "wrong signature",
			files:    map[string]string{"wrong_test.go": "package target\nfunc TestTarget() {}\n"},
			selected: []string{"TestTarget"},
			wantErr:  true,
		},
		{
			name: "duplicate",
			files: map[string]string{
				"first_test.go":  "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) {}\n",
				"second_test.go": "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) {}\n",
			},
			selected: []string{"TestTarget"},
			wantErr:  true,
		},
		{
			name:     "TestMain",
			files:    map[string]string{"main_test.go": "package target\nimport \"testing\"\nfunc TestMain(m *testing.M) {}\n"},
			selected: []string{"TestMain"},
			wantErr:  true,
		},
		{
			name:     "coexisting TestMain",
			files:    map[string]string{"main_test.go": "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) {}\nfunc TestMain(m *testing.M) {}\n"},
			selected: []string{"TestTarget"},
			wantErr:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			filenames := make([]string, 0, len(test.files))
			for filename, source := range test.files {
				writeSpecAuditRunnerTestFile(t, directory, filename, source)
				filenames = append(filenames, filename)
			}
			packageInfo := goListTestPackage{Dir: directory, ImportPath: "example.com/target", Name: "target", TestGoFiles: filenames}
			if _, err := observeSelectedGoTestDeclarations(packageInfo, test.selected); (err != nil) != test.wantErr {
				t.Fatalf("observeSelectedGoTestDeclarations() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestBuildSelectedIgnoredAndTaggedFilesCannotSatisfySelection(t *testing.T) {
	repoRoot := t.TempDir()
	writeSpecAuditRunnerTestFile(t, repoRoot, "go.mod", "module example.com/specaudit-selection\n\ngo 1.26.0\n")
	writeSpecAuditRunnerTestFile(t, repoRoot, "target/target.go", "package target\n")
	writeSpecAuditRunnerTestFile(t, repoRoot, "target/active_test.go", "package target\nimport \"testing\"\nfunc TestActive(t *testing.T) {}\n")
	writeSpecAuditRunnerTestFile(t, repoRoot, "target/_ignored_test.go", "package target\nimport \"testing\"\nfunc TestIgnored(t *testing.T) {}\n")
	writeSpecAuditRunnerTestFile(t, repoRoot, "target/tagged_test.go", "//go:build never\n\npackage target\nimport \"testing\"\nfunc TestTagged(t *testing.T) {}\n")
	t.Setenv("GOFLAGS", "-tags=never")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	expectedDirectory := filepath.Join(repoRoot, "target")
	childRuntime := newSpecAuditRunnerTestRuntime(t)
	packageInfo, _, output, err := resolveBuildSelectedGoTestPackage(ctx, repoRoot, "./target", expectedDirectory, childRuntime)
	if err != nil {
		t.Fatalf("resolveBuildSelectedGoTestPackage() error = %v\n%s", err, output.String())
	}
	resolvedDirectory, statErr := os.Stat(packageInfo.Dir)
	if statErr != nil {
		t.Fatalf("Stat(resolved package directory) error = %v", statErr)
	}
	wantedDirectory, statErr := os.Stat(filepath.Join(repoRoot, "target"))
	if statErr != nil {
		t.Fatalf("Stat(wanted package directory) error = %v", statErr)
	}
	if !os.SameFile(resolvedDirectory, wantedDirectory) {
		t.Fatalf("resolved package directory = %q, want same directory as %q", packageInfo.Dir, filepath.Join(repoRoot, "target"))
	}
	if _, err := observeSelectedGoTestDeclarations(packageInfo, []string{"TestActive"}); err != nil {
		t.Fatalf("active build-selected test rejected: %v", err)
	}
	for _, hidden := range []string{"TestIgnored", "TestTagged"} {
		if _, err := observeSelectedGoTestDeclarations(packageInfo, []string{hidden}); err == nil {
			t.Fatalf("non-build-selected declaration %s unexpectedly satisfied selection; files=%q", hidden, packageInfo.TestGoFiles)
		}
	}
	unexpectedDirectory := filepath.Join(repoRoot, "other")
	if err := os.MkdirAll(unexpectedDirectory, 0o755); err != nil {
		t.Fatalf("MkdirAll(unexpected package directory) error = %v", err)
	}
	if _, _, _, err := resolveBuildSelectedGoTestPackage(ctx, repoRoot, "./target", unexpectedDirectory, childRuntime); err == nil {
		t.Fatal("go list package directory mismatch unexpectedly succeeded")
	}
}

func TestObserveSelectedGoTestDeclarationsBoundsDeclarationFiles(t *testing.T) {
	t.Run("finite file count", func(t *testing.T) {
		directory := t.TempDir()
		writeSpecAuditRunnerTestFile(t, directory, "one_test.go", "package target\nimport \"testing\"\nfunc TestOne(t *testing.T) {}\n")
		writeSpecAuditRunnerTestFile(t, directory, "two_test.go", "package target\nimport \"testing\"\nfunc TestTwo(t *testing.T) {}\n")
		packageInfo := goListTestPackage{Dir: directory, ImportPath: "example.com/target", Name: "target", TestGoFiles: []string{"one_test.go", "two_test.go"}}
		limits := goTestDeclarationLimits{maxFiles: 1, maxFileBytes: 1024, maxTotalBytes: 2048}
		if _, err := observeSelectedGoTestDeclarationsWithLimits(packageInfo, []string{"TestOne"}, limits); err == nil {
			t.Fatal("over-limit build-selected file count unexpectedly succeeded")
		}
	})

	t.Run("per file bytes", func(t *testing.T) {
		directory := t.TempDir()
		source := "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) {}\n"
		writeSpecAuditRunnerTestFile(t, directory, "target_test.go", source)
		packageInfo := goListTestPackage{Dir: directory, ImportPath: "example.com/target", Name: "target", TestGoFiles: []string{"target_test.go"}}
		limits := goTestDeclarationLimits{maxFiles: 1, maxFileBytes: int64(len(source) - 1), maxTotalBytes: 1024}
		if _, err := observeSelectedGoTestDeclarationsWithLimits(packageInfo, []string{"TestTarget"}, limits); err == nil {
			t.Fatal("over-limit build-selected test declaration file unexpectedly succeeded")
		}
	})

	t.Run("aggregate bytes", func(t *testing.T) {
		directory := t.TempDir()
		one := "package target\nimport \"testing\"\nfunc TestOne(t *testing.T) {}\n"
		two := "package target\nimport \"testing\"\nfunc TestTwo(t *testing.T) {}\n"
		writeSpecAuditRunnerTestFile(t, directory, "one_test.go", one)
		writeSpecAuditRunnerTestFile(t, directory, "two_test.go", two)
		packageInfo := goListTestPackage{Dir: directory, ImportPath: "example.com/target", Name: "target", TestGoFiles: []string{"one_test.go", "two_test.go"}}
		limits := goTestDeclarationLimits{maxFiles: 2, maxFileBytes: 1024, maxTotalBytes: int64(len(one) + len(two) - 1)}
		if _, err := observeSelectedGoTestDeclarationsWithLimits(packageInfo, []string{"TestOne", "TestTwo"}, limits); err == nil {
			t.Fatal("over-limit aggregate build-selected test declaration files unexpectedly succeeded")
		}
	})

	t.Run("regular files only", func(t *testing.T) {
		directory := t.TempDir()
		target := filepath.Join(directory, "target.go")
		writeSpecAuditRunnerTestFile(t, directory, "target.go", "package target\n")
		if err := os.Symlink(target, filepath.Join(directory, "target_test.go")); err != nil {
			t.Fatalf("Symlink() error = %v", err)
		}
		packageInfo := goListTestPackage{Dir: directory, ImportPath: "example.com/target", Name: "target", TestGoFiles: []string{"target_test.go"}}
		limits := goTestDeclarationLimits{maxFiles: 1, maxFileBytes: 1024, maxTotalBytes: 1024}
		if _, err := observeSelectedGoTestDeclarationsWithLimits(packageInfo, []string{"TestTarget"}, limits); err == nil {
			t.Fatal("symlinked build-selected test declaration file unexpectedly succeeded")
		}
	})
}

func TestStableDeclarationFileReadRejectsSameLengthInPlaceMutation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "target_test.go")
	original := "package target\n"
	mutated := "package targeu\n"
	if len(original) != len(mutated) {
		t.Fatal("test fixture mutation must preserve length")
	}
	writeSpecAuditRunnerTestFile(t, directory, "target_test.go", original)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(before mutation) error = %v", err)
	}
	_, _, _, err = readBoundedBuildSelectedTestFileWithHook(path, 1024, 1024, func() error {
		return os.WriteFile(path, []byte(mutated), 0o600)
	})
	if err == nil {
		t.Fatal("same-length in-place mutation unexpectedly passed stable read")
	}
	after, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("Stat(after mutation) error = %v", statErr)
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() {
		t.Fatal("test fixture did not perform a same-inode, same-length mutation")
	}
}

func TestPostRunSelectedTestDeclarationObservationRejectsVisibleChanges(t *testing.T) {
	tests := []struct {
		name    string
		mutated string
	}{
		{
			name:    "renamed requested declaration",
			mutated: "package target\nimport \"testing\"\nfunc TestRenamed(t *testing.T) {}\n",
		},
		{
			name:    "added TestMain",
			mutated: "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) {}\nfunc TestMain(m *testing.M) {}\n",
		},
		{
			name:    "valid declaration body changed",
			mutated: "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) { t.Log(\"changed\") }\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			initial := "package target\nimport \"testing\"\nfunc TestTarget(t *testing.T) {}\n"
			writeSpecAuditRunnerTestFile(t, directory, "target_test.go", initial)
			packageInfo := goListTestPackage{
				Dir:         directory,
				ImportPath:  "example.com/target",
				Name:        "target",
				TestGoFiles: []string{"target_test.go"},
			}
			preRun, err := observeSelectedGoTestDeclarations(packageInfo, []string{"TestTarget"})
			if err != nil {
				t.Fatalf("pre-run selected-test declaration observation error = %v", err)
			}
			writeSpecAuditRunnerTestFile(t, directory, "target_test.go", test.mutated)
			if err := validatePostRunSelectedTestDeclarations(packageInfo, []string{"TestTarget"}, preRun); err == nil {
				t.Fatal("changed post-run selected-test declaration observation unexpectedly matched")
			}
		})
	}
}

func TestSelectedTestDeclarationObservationScopeIsNarrow(t *testing.T) {
	for _, required := range []string{
		"build-selected TestGoFiles and XTestGoFiles",
		"pre-test and post-test observation points",
		"does not cover or make immutable production Go files",
		"module files",
		"embed inputs",
		"dependencies",
		"cannot detect a mid-run swap restored before the post-test observation",
	} {
		if !strings.Contains(selectedTestDeclarationObservationScope, required) {
			t.Fatalf("selected-test declaration scope omits %q: %q", required, selectedTestDeclarationObservationScope)
		}
	}

	catalogPath := filepath.Join(packageSpecBDDRepoRoot(), "agm", "docs", "BDD-CATALOG.md")
	catalog, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read BDD catalog disclosure: %v", err)
	}
	for _, required := range []string{
		"build-selected `TestGoFiles` and `XTestGoFiles`",
		"pre-test and post-test observation points",
		"does not cover or make immutable production Go files",
		"module files",
		"embed inputs",
		"dependencies",
		"mid-run swap that is restored before the post-test observation",
	} {
		if !bytes.Contains(catalog, []byte(required)) {
			t.Fatalf("BDD catalog selected-test declaration disclosure omits %q", required)
		}
	}
}

func TestTrustedImplementationTestRuntimeScrubsSecretsAndForcesOfflineGo(t *testing.T) {
	t.Setenv("SPEC_AUDIT_TEST_SECRET", "must-not-reach-child")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "must-not-reach-child")
	t.Setenv("GOFLAGS", "-race -tags=hostile")
	childRuntime := newSpecAuditRunnerTestRuntime(t)
	for _, entry := range childRuntime.environment {
		key, _, _ := strings.Cut(entry, "=")
		if key == "SPEC_AUDIT_TEST_SECRET" || key == "AWS_SECRET_ACCESS_KEY" {
			t.Fatalf("secret environment variable reached child: %q", entry)
		}
	}
	for _, required := range []string{
		"GOFLAGS=",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOVCS=*:off",
		"GOTOOLCHAIN=local",
		"GOENV=off",
	} {
		if !slices.Contains(childRuntime.environment, required) {
			t.Fatalf("minimal child environment omits %q: %q", required, childRuntime.environment)
		}
	}
	taskBuildCache := "GOCACHE=" + filepath.Join(childRuntime.taskRoot, "gocache")
	if !slices.Contains(childRuntime.environment, taskBuildCache) {
		t.Fatalf("child GOCACHE is not task-owned: want %q in %q", taskBuildCache, childRuntime.environment)
	}
	cacheInfo, err := os.Lstat(filepath.Join(childRuntime.taskRoot, "gocache"))
	if err != nil {
		t.Fatalf("Lstat(task-owned GOCACHE) error = %v", err)
	}
	if !cacheInfo.IsDir() || cacheInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("task-owned GOCACHE is not a real directory: mode=%v", cacheInfo.Mode())
	}
	if !filepath.IsAbs(childRuntime.taskRoot) || filepath.Clean(childRuntime.taskRoot) != childRuntime.taskRoot {
		t.Fatalf("task root is not canonical and absolute: %q", childRuntime.taskRoot)
	}
	if childRuntime.taskRootIdentity == nil || childRuntime.taskRootPermissions != 0o700 {
		t.Fatalf("task root identity was not retained with 0700 permissions: identity=%v permissions=%#o", childRuntime.taskRootIdentity, childRuntime.taskRootPermissions)
	}
	if err := childRuntime.revalidateExecutables(); err != nil {
		t.Fatalf("trusted executable identity revalidation failed: %v", err)
	}
}

func TestSpecAuditTaskRootCleanupRefusesIdentityAndPermissionDrift(t *testing.T) {
	newRuntime := func(t *testing.T) specAuditGoChildRuntime {
		t.Helper()
		childRuntime, err := newSpecAuditGoChildRuntime()
		if err != nil {
			t.Fatalf("newSpecAuditGoChildRuntime() error = %v", err)
		}
		return childRuntime
	}

	t.Run("missing root", func(t *testing.T) {
		childRuntime := newRuntime(t)
		if err := os.RemoveAll(childRuntime.taskRoot); err != nil {
			t.Fatalf("RemoveAll(task root) error = %v", err)
		}
		if err := childRuntime.cleanup(); err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("cleanup() error = %v, want missing-root refusal", err)
		}
	})

	t.Run("replaced root", func(t *testing.T) {
		childRuntime := newRuntime(t)
		original := childRuntime.taskRoot + "-original"
		defer func() {
			_ = os.RemoveAll(childRuntime.taskRoot)
			_ = os.RemoveAll(original)
		}()
		if err := os.Rename(childRuntime.taskRoot, original); err != nil {
			t.Fatalf("Rename(task root) error = %v", err)
		}
		if err := os.Mkdir(childRuntime.taskRoot, 0o700); err != nil {
			t.Fatalf("Mkdir(replacement task root) error = %v", err)
		}
		if err := childRuntime.cleanup(); err == nil || !strings.Contains(err.Error(), "replaced") {
			t.Fatalf("cleanup() error = %v, want replaced-root refusal", err)
		}
		if err := os.Remove(childRuntime.taskRoot); err != nil {
			t.Fatalf("Remove(replacement task root) error = %v", err)
		}
		if err := os.Rename(original, childRuntime.taskRoot); err != nil {
			t.Fatalf("restore original task root error = %v", err)
		}
		if err := childRuntime.cleanup(); err != nil {
			t.Fatalf("cleanup(restored task root) error = %v", err)
		}
	})

	t.Run("wrong mode", func(t *testing.T) {
		childRuntime := newRuntime(t)
		defer func() { _ = os.RemoveAll(childRuntime.taskRoot) }()
		if err := os.Chmod(childRuntime.taskRoot, 0o750); err != nil {
			t.Fatalf("Chmod(task root) error = %v", err)
		}
		if err := childRuntime.cleanup(); err == nil || !strings.Contains(err.Error(), "wrong-mode") {
			t.Fatalf("cleanup() error = %v, want wrong-mode refusal", err)
		}
		if err := os.Chmod(childRuntime.taskRoot, 0o700); err != nil {
			t.Fatalf("restore task root mode error = %v", err)
		}
		if err := childRuntime.cleanup(); err != nil {
			t.Fatalf("cleanup(restored task root) error = %v", err)
		}
	})

	t.Run("wrong retained owner", func(t *testing.T) {
		childRuntime := newRuntime(t)
		actualOwner := childRuntime.taskRootOwner
		childRuntime.taskRootOwner++
		if err := childRuntime.cleanup(); err == nil || !strings.Contains(err.Error(), "wrong-owner") {
			t.Fatalf("cleanup() error = %v, want wrong-owner refusal", err)
		}
		childRuntime.taskRootOwner = actualOwner
		if err := childRuntime.cleanup(); err != nil {
			t.Fatalf("cleanup(restored owner identity) error = %v", err)
		}
	})
}

func TestTrustedExecutableValidationRejectsReplacementAndUnsafeAncestry(t *testing.T) {
	//nolint:usetesting // t.TempDir cannot select the trusted repository parent required to test executable ancestry.
	fixtureRoot, err := os.MkdirTemp(packageSpecBDDRepoRoot(), ".specaudit-exec-test-")
	if err != nil {
		t.Fatalf("MkdirTemp(trusted executable fixture root) error = %v", err)
	}
	fixtureRootInfo, err := os.Lstat(fixtureRoot)
	if err != nil {
		t.Fatalf("Lstat(trusted executable fixture root) error = %v", err)
	}
	t.Cleanup(func() {
		currentInfo, err := os.Lstat(fixtureRoot)
		if err != nil {
			t.Errorf("Lstat(trusted executable fixture root during cleanup) error = %v", err)
			return
		}
		if !currentInfo.IsDir() || currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(fixtureRootInfo, currentInfo) {
			t.Errorf("trusted executable fixture root identity changed before cleanup: mode=%v", currentInfo.Mode())
			return
		}
		if err := os.RemoveAll(fixtureRoot); err != nil {
			t.Errorf("RemoveAll(trusted executable fixture root) error = %v", err)
		}
	})

	executableDirectory := filepath.Join(fixtureRoot, "trusted-bin")
	if err := os.Mkdir(executableDirectory, 0o700); err != nil {
		t.Fatalf("Mkdir(trusted executable directory) error = %v", err)
	}
	executablePath := filepath.Join(executableDirectory, "tool")
	if err := os.WriteFile(executablePath, []byte("trusted fixture"), 0o700); err != nil {
		t.Fatalf("WriteFile(trusted executable) error = %v", err)
	}
	identity := trustedSpecAuditExecutable{path: executablePath}
	info, err := validateTrustedSpecAuditExecutable("fixture", identity, false)
	if err != nil {
		t.Fatalf("validate initial trusted executable error = %v", err)
	}
	identity.identity = info
	original := executablePath + "-original"
	if err := os.Rename(executablePath, original); err != nil {
		t.Fatalf("Rename(trusted executable) error = %v", err)
	}
	if err := os.WriteFile(executablePath, []byte("replacement fixture"), 0o700); err != nil {
		t.Fatalf("WriteFile(replacement executable) error = %v", err)
	}
	if _, err := validateTrustedSpecAuditExecutable("fixture", identity, true); err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("replacement revalidation error = %v, want identity rejection", err)
	}

	groupWritableDirectory := filepath.Join(fixtureRoot, "group-writable-bin")
	if err := os.Mkdir(groupWritableDirectory, 0o700); err != nil {
		t.Fatalf("Mkdir(group-writable executable directory) error = %v", err)
	}
	if err := os.Chmod(groupWritableDirectory, 0o770); err != nil {
		t.Fatalf("Chmod(group-writable executable directory) error = %v", err)
	}
	groupWritableExecutable := filepath.Join(groupWritableDirectory, "tool")
	if err := os.WriteFile(groupWritableExecutable, []byte("group-writable ancestry fixture"), 0o700); err != nil {
		t.Fatalf("WriteFile(group-writable ancestry executable) error = %v", err)
	}
	if _, err := validateTrustedSpecAuditExecutable("fixture", trustedSpecAuditExecutable{path: groupWritableExecutable}, false); err != nil {
		t.Fatalf("current-user-owned group-writable ancestry should remain portable: %v", err)
	}

	worldWritableDirectory := filepath.Join(fixtureRoot, "world-writable-bin")
	if err := os.Mkdir(worldWritableDirectory, 0o700); err != nil {
		t.Fatalf("Mkdir(world-writable executable directory) error = %v", err)
	}
	if err := os.Chmod(worldWritableDirectory, 0o702); err != nil {
		t.Fatalf("Chmod(world-writable executable directory) error = %v", err)
	}
	worldWritableExecutable := filepath.Join(worldWritableDirectory, "tool")
	if err := os.WriteFile(worldWritableExecutable, []byte("world-writable ancestry fixture"), 0o700); err != nil {
		t.Fatalf("WriteFile(world-writable ancestry executable) error = %v", err)
	}
	if _, err := validateTrustedSpecAuditExecutable("fixture", trustedSpecAuditExecutable{path: worldWritableExecutable}, false); err == nil || !strings.Contains(err.Error(), "ancestry") {
		t.Fatalf("world-writable-ancestry validation error = %v, want ancestry rejection", err)
	}
}

func TestGitHubHostedGoToolcacheTrustIsNarrowAndDigestBound(t *testing.T) {
	toolCache, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks(GitHub-hosted tool-cache fixture) error = %v", err)
	}
	goRoot := filepath.Join(toolCache, "go", "1.26.5", "x64")
	goExecutable := filepath.Join(goRoot, "bin", "go")
	if err := os.MkdirAll(filepath.Dir(goExecutable), 0o777); err != nil {
		t.Fatalf("MkdirAll(GitHub-hosted Go fixture) error = %v", err)
	}
	for _, directory := range []string{toolCache, filepath.Join(toolCache, "go"), filepath.Join(toolCache, "go", "1.26.5"), goRoot, filepath.Dir(goExecutable)} {
		if err := os.Chmod(directory, 0o777); err != nil {
			t.Fatalf("Chmod(%q) error = %v", directory, err)
		}
	}
	if err := os.WriteFile(goExecutable, []byte("github-hosted-go-fixture"), 0o777); err != nil {
		t.Fatalf("WriteFile(GitHub-hosted Go fixture) error = %v", err)
	}
	executable := trustedSpecAuditExecutable{
		path:      goExecutable,
		trustMode: specAuditGitHubHostedGoTrust,
	}
	if _, err := validateTrustedSpecAuditExecutable("go", executable, false); err == nil {
		t.Fatal("strict executable trust unexpectedly accepted a world-writable Go toolchain")
	}
	runnerContext := specAuditGitHubHostedGoContext{
		GitHubActions:     "true",
		RunnerEnvironment: "github-hosted",
		RunnerOS:          "Linux",
		RunnerArch:        "X64",
		RunnerToolCache:   toolCache,
		ImageOS:           "ubuntu24",
		ImageVersion:      "20260720.247.2",
		RuntimeGOOS:       "linux",
		RuntimeGOARCH:     "amd64",
		RuntimeVersion:    "go1.26.5",
		RuntimeGOROOT:     goRoot,
	}
	info, digest, capturedGoRoot, goRootInfo, err := validateGitHubHostedGoExecutable(
		executable,
		runnerContext,
		toolCache,
		false,
	)
	if err != nil {
		t.Fatalf("validateGitHubHostedGoExecutable() error = %v", err)
	}
	executable.identity = info
	executable.digest = digest
	executable.githubHostedContext = runnerContext
	executable.githubHostedGoRoot = capturedGoRoot
	executable.githubHostedGoRootID = goRootInfo
	if _, _, _, _, err := validateGitHubHostedGoExecutable(executable, runnerContext, toolCache, true); err != nil {
		t.Fatalf("GitHub-hosted Go identity revalidation error = %v", err)
	}

	selfHosted := runnerContext
	selfHosted.RunnerEnvironment = "self-hosted"
	if _, _, _, _, err := validateGitHubHostedGoExecutable(executable, selfHosted, toolCache, true); err == nil || !strings.Contains(err.Error(), "exact GitHub-hosted Ubuntu runner context") {
		t.Fatalf("self-hosted runner context error = %v, want exact-context rejection", err)
	}
	wrongArchitecture := runnerContext
	wrongArchitecture.RunnerArch = "ARM64"
	if _, _, _, _, err := validateGitHubHostedGoExecutable(executable, wrongArchitecture, toolCache, true); err == nil || !strings.Contains(err.Error(), "exact GitHub-hosted Ubuntu runner context") {
		t.Fatalf("wrong-architecture runner context error = %v, want exact-context rejection", err)
	}
	overriddenGoRoot := runnerContext
	overriddenGoRoot.GOROOTOverride = goRoot
	if _, _, _, _, err := validateGitHubHostedGoExecutable(executable, overriddenGoRoot, toolCache, true); err == nil || !strings.Contains(err.Error(), "exact GitHub-hosted Ubuntu runner context") {
		t.Fatalf("overridden-GOROOT runner context error = %v, want exact-context rejection", err)
	}
	otherCache := filepath.Join(toolCache, "other")
	if err := os.Mkdir(otherCache, 0o777); err != nil {
		t.Fatalf("Mkdir(other tool cache) error = %v", err)
	}
	if _, _, _, _, err := validateGitHubHostedGoExecutable(executable, runnerContext, otherCache, true); err == nil || !strings.Contains(err.Error(), "is not the required") {
		t.Fatalf("alternate tool-cache root error = %v, want exact-root rejection", err)
	}
	if err := os.WriteFile(goExecutable, []byte("mutated-hosted-go-fixture"), 0o777); err != nil {
		t.Fatalf("mutate GitHub-hosted Go fixture error = %v", err)
	}
	if _, _, _, _, err := validateGitHubHostedGoExecutable(executable, runnerContext, toolCache, true); err == nil || !strings.Contains(err.Error(), "changed after validation") {
		t.Fatalf("mutated GitHub-hosted Go error = %v, want digest rejection", err)
	}
}

func newSpecAuditRunnerTestRuntime(t *testing.T) specAuditGoChildRuntime {
	t.Helper()
	childRuntime, err := newSpecAuditGoChildRuntime()
	if err != nil {
		t.Fatalf("newSpecAuditGoChildRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := childRuntime.cleanup(); err != nil {
			t.Errorf("SPEC audit test runtime cleanup error = %v", err)
		}
	})
	return childRuntime
}

func writeSpecAuditRunnerTestFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
