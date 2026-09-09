//go:build darwin && arm64

package buildauthority

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestDarwinSupervisorInitialAndPreStartSigactionRows(t *testing.T) {
	admitted := darwinOldSigaction{handler: 0x1234, mask: 0x55}
	for _, test := range []struct {
		name      string
		actions   []darwinActionStep
		wantPhase Phase
		wantOp    Operation
		wantCause CauseCode
		wantStart int
	}{
		{
			name:      "initial query failure",
			actions:   []darwinActionStep{{err: syscall.EIO}},
			wantPhase: PhaseAuthority,
			wantOp:    OperationProbe,
			wantCause: CauseInternalInvariant,
		},
		{
			name:      "initial ignored",
			actions:   []darwinActionStep{{action: darwinOldSigaction{handler: darwinSIGIgnored}}},
			wantPhase: PhaseAuthority,
			wantOp:    OperationValidate,
			wantCause: CauseUnsupported,
		},
		{
			name:      "initial no child wait",
			actions:   []darwinActionStep{{action: darwinOldSigaction{flags: darwinSANoChildWait}}},
			wantPhase: PhaseAuthority,
			wantOp:    OperationValidate,
			wantCause: CauseUnsupported,
		},
		{
			name:      "pre-start query failure",
			actions:   []darwinActionStep{{action: admitted}, {err: syscall.EIO}},
			wantPhase: PhaseSource,
			wantOp:    OperationProbe,
			wantCause: CauseInternalInvariant,
		},
		{
			name:      "pre-start drift",
			actions:   []darwinActionStep{{action: admitted}, {action: darwinOldSigaction{handler: 0x5678}}},
			wantPhase: PhaseSource,
			wantOp:    OperationProbe,
			wantCause: CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(admitted)
			harness.actions = test.actions
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if test.wantPhase == PhaseAuthority {
				requireProcessRecord(t, initial, test.wantPhase, test.wantOp, test.wantCause)
				if supervisor != nil {
					t.Fatal("unsafe initial action returned a supervisor")
				}
				return
			}
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, harness.request())
			requireProcessRecord(t, result.primary, test.wantPhase, test.wantOp, test.wantCause)
			if harness.command.startCount != test.wantStart || harness.waitCalls() != 0 || harness.waitIDCalls != 0 ||
				harness.signalCalls != 0 || harness.snapshotCalls != 0 {
				t.Fatalf("calls: start=%d Wait=%d waitid=%d signal=%d snapshot=%d", harness.command.startCount, harness.waitCalls(), harness.waitIDCalls, harness.signalCalls, harness.snapshotCalls)
			}
			if !result.quiescence.proven || result.quiescence.child != nil {
				t.Fatalf("pre-start quiescence = %+v", result.quiescence)
			}
		})
	}
}

func TestDarwinSupervisorPipeSetupAndStartFailureCloseExactlyOnce(t *testing.T) {
	for _, failureAt := range []int{1, 2, 3} {
		t.Run(string(rune('0'+failureAt)), func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.pipeFailureAt = failureAt
			request := harness.request()
			request.input = newBoundedProcessInput([]byte("input"))
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			requireProcessRecord(t, result.primary, PhaseSource, OperationOpen, CauseLimit)
			if harness.command.startCount != 0 || !result.quiescence.proven {
				t.Fatalf("start=%d quiescence=%+v", harness.command.startCount, result.quiescence)
			}
			harness.requireEveryCreatedEndClosedOnce(t)
		})
	}

	harness := newDarwinHarness(darwinOldSigaction{})
	harness.command.startErr = errors.New("start")
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, harness.request())
	requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseChildStart)
	if harness.waitCalls() != 0 || harness.waitIDCalls != 0 || harness.signalCalls != 0 || harness.snapshotCalls != 0 {
		t.Fatalf("post-start calls were invented")
	}
	if result.primary.Child != nil || !result.quiescence.proven {
		t.Fatalf("start failure child/quiescence = %+v / %+v", result.primary.Child, result.quiescence)
	}
	harness.requireEveryCreatedEndClosedOnce(t)
}

func TestRealProcessCommandsUseOnlyTypedClosedEnvironmentAndCreateNewGroup(t *testing.T) {
	preallocation, err := newPreallocationEnvironment("/private/go", "/private/mod")
	if err != nil {
		t.Fatalf("preallocation environment: %v", err)
	}
	workspace, err := newWorkspacePaths("/private/state/task")
	if err != nil {
		t.Fatalf("workspace paths: %v", err)
	}
	taskPrivate, err := newTaskPrivateEnvironment(workspace, "/private/go", "/private/mod")
	if err != nil {
		t.Fatalf("task-private environment: %v", err)
	}
	spec := processCommandSpec{executable: "/bin/sh", directory: "/private/tmp"}
	for _, test := range []struct {
		name    string
		want    []string
		command processCommand
	}{
		{
			name:    "preallocation",
			want:    preallocation.clone(),
			command: newRealPreallocationProcessCommand(spec, preallocation),
		},
		{
			name:    "task private",
			want:    taskPrivate.clone(),
			command: newRealTaskPrivateProcessCommand(spec, taskPrivate),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := test.command.(*realProcessCommand).command
			if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid || command.SysProcAttr.Pgid != 0 {
				t.Fatalf("SysProcAttr = %+v", command.SysProcAttr)
			}
			if !reflect.DeepEqual(command.Env, test.want) {
				t.Fatalf("closed environment = %v", command.Env)
			}
		})
	}
}

func TestDarwinSupervisorTypedEntriesForwardOnlyTheirClosedProfiles(t *testing.T) {
	workspace, err := newWorkspacePaths("/private/state/task")
	if err != nil {
		t.Fatalf("workspace paths: %v", err)
	}
	preallocation, err := newPreallocationEnvironment("/private/go", "/private/mod")
	if err != nil {
		t.Fatalf("preallocation environment: %v", err)
	}
	taskPrivate, err := newTaskPrivateEnvironment(workspace, "/private/go", "/private/mod")
	if err != nil {
		t.Fatalf("task-private environment: %v", err)
	}

	for _, test := range []struct {
		name string
		want []string
		run  func(processSupervisor, processRequest) processResult
	}{
		{
			name: "preallocation",
			want: preallocation.clone(),
			run: func(supervisor processSupervisor, request processRequest) processResult {
				return supervisor.runPreallocation(request, preallocation)
			},
		},
		{
			name: "task private",
			want: taskPrivate.clone(),
			run: func(supervisor processSupervisor, request processRequest) processResult {
				return supervisor.runTaskPrivate(request, taskPrivate)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := test.run(supervisor, harness.request())
			if result.primary != nil || len(harness.commandSpecs) != 1 ||
				len(harness.commandProfiles) != 1 || harness.commandProfiles[0] != test.name ||
				len(harness.commandEnvs) != 1 {
				t.Fatalf("result=%+v command specs=%d profiles=%q environments=%d",
					result, len(harness.commandSpecs), harness.commandProfiles, len(harness.commandEnvs))
			}
			if got := harness.commandEnvs[0]; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("forwarded environment:\n got: %q\nwant: %q", got, test.want)
			}
		})
	}
}

func TestDarwinSupervisorRejectsZeroEnvironmentProfilesBeforeAllocation(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(processSupervisor, processRequest) processResult
	}{
		{
			name: "preallocation",
			run: func(supervisor processSupervisor, request processRequest) processResult {
				return supervisor.runPreallocation(request, preallocationEnvironment{})
			},
		},
		{
			name: "task private",
			run: func(supervisor processSupervisor, request processRequest) processResult {
				return supervisor.runTaskPrivate(request, taskPrivateEnvironment{})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := test.run(supervisor, harness.request())
			requireProcessRecord(t, result.primary, PhaseSource, OperationValidate, CauseInternalInvariant)
			if harness.pipeCalls != 0 || len(harness.commandSpecs) != 0 || harness.command.startCount != 0 {
				t.Fatalf("invalid profile crossed allocation seam: pipes=%d specs=%d starts=%d",
					harness.pipeCalls, len(harness.commandSpecs), harness.command.startCount)
			}
		})
	}
}

func TestDarwinSupervisorRejectsInvalidInputBeforeAllocation(t *testing.T) {
	for _, test := range []struct {
		name  string
		input *processInput
	}{
		{name: "nil"},
		{name: "zero tag", input: &processInput{}},
		{name: "unknown tag", input: &processInput{kind: processInputKind(255)}},
		{name: "missing retained null", input: newRetainedNullProcessInput(nil)},
		{
			name: "retained null with bounded payload",
			input: &processInput{
				kind:         processInputRetainedNull,
				retainedNull: &retainedNullDevice{},
				bounded:      []byte{},
			},
		},
		{
			name: "bounded with retained null",
			input: &processInput{
				kind:         processInputBounded,
				retainedNull: &retainedNullDevice{},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			request := harness.request()
			request.input = test.input
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			requireProcessRecord(t, result.primary, PhaseSource, OperationValidate, CauseInternalInvariant)
			if result.structuredOutput != nil || harness.pipeCalls != 0 ||
				len(harness.commandSpecs) != 0 || harness.command.startCount != 0 {
				t.Fatalf("invalid input crossed allocation seam: output=%v pipes=%d specs=%d starts=%d",
					result.structuredOutput != nil, harness.pipeCalls, len(harness.commandSpecs), harness.command.startCount)
			}
		})
	}
}

func TestDarwinSupervisorInputVariantsHaveDisjointOwnership(t *testing.T) {
	for _, test := range []struct {
		name      string
		input     func(*testing.T) (*processInput, *retainedNullDevice)
		wantPipes int
	}{
		{
			name: "bounded empty",
			input: func(*testing.T) (*processInput, *retainedNullDevice) {
				return newBoundedProcessInput(nil), nil
			},
			wantPipes: 3,
		},
		{
			name: "retained null",
			input: func(t *testing.T) (*processInput, *retainedNullDevice) {
				device, err := retainNullDevice(context.Background())
				if err != nil {
					t.Fatalf("retain null device: %v", err)
				}
				return newRetainedNullProcessInput(device), device
			},
			wantPipes: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
			harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
			input, retained := test.input(t)
			if retained != nil {
				defer func() {
					if err := retained.close(); err != nil {
						t.Errorf("close retained null device: %v", err)
					}
				}()
			}
			request := harness.request()
			request.input = input
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			if result.primary != nil || result.descriptorClose != nil || !result.quiescence.proven ||
				result.structuredOutput == nil || harness.pipeCalls != test.wantPipes {
				t.Fatalf("primary=%+v close=%+v quiescence=%+v output-nil=%v pipe calls=%d",
					result.primary, result.descriptorClose, result.quiescence,
					result.structuredOutput == nil, harness.pipeCalls)
			}
			if retained == nil {
				if harness.command.stdin != harness.ends[4] {
					t.Fatal("bounded input did not configure the child side of its private pipe")
				}
			} else {
				if harness.command.stdin != retained.leaf.descriptor {
					t.Fatal("retained null input did not borrow the retained descriptor directly")
				}
				if _, err := retained.leaf.descriptor.Stat(); err != nil {
					t.Fatalf("supervisor closed retained null descriptor: %v", err)
				}
			}
			harness.requireEveryCreatedEndClosedOnce(t)
		})
	}
}

func TestDarwinSupervisorRejectsUnusableRetainedNullBeforeAllocation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *retainedNullDevice) func()
	}{
		{
			name: "closed descriptor",
			mutate: func(t *testing.T, device *retainedNullDevice) func() {
				t.Helper()
				if err := device.close(); err != nil {
					t.Fatalf("close retained null device: %v", err)
				}
				return func() {}
			},
		},
		{
			name: "descriptor drift",
			mutate: func(t *testing.T, device *retainedNullDevice) func() {
				t.Helper()
				original := device.leaf.descriptor
				wrong, err := os.Open(physicalRootPath)
				if err != nil {
					t.Fatalf("open wrong descriptor: %v", err)
				}
				device.leaf.descriptor = wrong
				return func() {
					_ = wrong.Close()
					device.leaf.descriptor = original
					_ = device.close()
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			device, err := retainNullDevice(context.Background())
			if err != nil {
				t.Fatalf("retain null device: %v", err)
			}
			cleanup := test.mutate(t, device)
			defer cleanup()
			harness := newDarwinHarness(darwinOldSigaction{})
			request := harness.request()
			request.input = newRetainedNullProcessInput(device)
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			if result.primary == nil || result.primary.Phase != PhaseAuthority ||
				result.primary.Operation != OperationProbe || result.structuredOutput != nil ||
				harness.pipeCalls != 0 || len(harness.commandSpecs) != 0 || harness.command.startCount != 0 {
				t.Fatalf("unusable retained null crossed start boundary: result=%+v pipes=%d specs=%d starts=%d",
					result, harness.pipeCalls, len(harness.commandSpecs), harness.command.startCount)
			}
		})
	}
}

func TestDarwinSupervisorPostCommandRetainedNullRevalidationGatesOutput(t *testing.T) {
	device, err := retainNullDevice(context.Background())
	if err != nil {
		t.Fatalf("retain null device: %v", err)
	}
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.stdoutData = []byte("withheld")
	harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
	harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
	dependencies := harness.dependencies()
	originalWaitID := dependencies.waitID
	closed := false
	dependencies.waitID = func(pid int) (darwinSiginfo, error) {
		if !closed {
			closed = true
			if err := device.close(); err != nil {
				t.Fatalf("close retained null device after Start: %v", err)
			}
		}
		return originalWaitID(pid)
	}
	request := harness.request()
	request.input = newRetainedNullProcessInput(device)
	supervisor, initial := newDarwinProcessSupervisor(dependencies)
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, request)
	if !closed || result.primary == nil || result.primary.Phase != PhaseAuthority ||
		result.primary.Operation != OperationProbe || result.structuredOutput != nil ||
		!result.quiescence.proven || result.descriptorClose != nil || harness.pipeCalls != 2 {
		t.Fatalf("post-command retained-null gate = %+v closed=%v pipes=%d", result, closed, harness.pipeCalls)
	}
}

func TestDarwinSupervisorWaitAnomalySealsAbsentStatusAndHandsOff(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.waitSteps = []darwinWaitStep{{err: syscall.ECHILD}}
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, harness.request())
	if result.background == nil {
		t.Fatal("wait anomaly did not hand off")
	}
	requireProcessCause(t, result.quiescence.causes, CauseChildWait)
	if result.quiescence.child == nil || result.quiescence.child.ExitStatusObserved {
		t.Fatalf("handoff child = %+v", result.quiescence.child)
	}
	select {
	case <-result.background.done:
	case <-time.After(time.Second):
		t.Fatal("background reaper did not return")
	}
	if harness.waitCalls() != 1 || harness.waitIDCalls != 1 || harness.signalCalls != 0 || harness.snapshotCalls != 0 {
		t.Fatalf("calls: Wait=%d waitid=%d signal=%d snapshot=%d", harness.waitCalls(), harness.waitIDCalls, harness.signalCalls, harness.snapshotCalls)
	}
}

func TestDarwinSupervisorPreSignalDriftNeverSignalsOrProbes(t *testing.T) {
	admitted := darwinOldSigaction{handler: 0x1234}
	harness := newDarwinHarness(admitted)
	harness.actions = []darwinActionStep{{action: admitted}, {action: admitted}, {action: darwinOldSigaction{handler: 0x9999}}}
	harness.waitSteps = []darwinWaitStep{{info: darwinSiginfo{}}}
	request := harness.request()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	request.ctx = canceled
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, request)
	requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseCanceled)
	requireProcessCause(t, result.quiescence.causes, CauseChildWait)
	if result.background == nil || harness.signalCalls != 0 || harness.snapshotCalls != 0 || harness.waitIDCalls != 1 {
		t.Fatalf("handoff=%v signal=%d snapshot=%d waitid=%d", result.background != nil, harness.signalCalls, harness.snapshotCalls, harness.waitIDCalls)
	}
	<-result.background.done
}

func TestDarwinSupervisorPreSignalQueryErrorAndUnsafeActionHandOff(t *testing.T) {
	admitted := darwinOldSigaction{handler: 0x1234}
	for _, test := range []struct {
		name string
		step darwinActionStep
	}{
		{name: "query error", step: darwinActionStep{err: syscall.EIO}},
		{name: "ignored", step: darwinActionStep{action: darwinOldSigaction{handler: darwinSIGIgnored}}},
		{name: "no child wait", step: darwinActionStep{action: darwinOldSigaction{flags: darwinSANoChildWait}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(admitted)
			harness.actions = []darwinActionStep{{action: admitted}, {action: admitted}, test.step}
			harness.waitSteps = []darwinWaitStep{{info: darwinSiginfo{}}}
			request := harness.request()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			request.ctx = ctx
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseCanceled)
			requireProcessCause(t, result.quiescence.causes, CauseChildWait)
			if result.background == nil || harness.signalCalls != 0 || harness.snapshotCalls != 0 || harness.waitIDCalls != 1 {
				t.Fatalf("background=%v signal=%d snapshot=%d waitid=%d", result.background != nil, harness.signalCalls, harness.snapshotCalls, harness.waitIDCalls)
			}
			<-result.background.done
		})
	}
}

func TestDarwinSupervisorProvisionalSignalRequiresLeaderZombie(t *testing.T) {
	for _, test := range []struct {
		name      string
		signalErr error
		members   []darwinTestMember
		wantCause CauseCode
		wantProof bool
	}{
		{
			name:      "leader zombie resolves ESRCH",
			signalErr: syscall.ESRCH,
			members:   []darwinTestMember{{pid: 101, pgid: 101, state: darwinProcessZombie}},
			wantProof: true,
		},
		{
			name:      "leader zombie resolves EPERM",
			signalErr: syscall.EPERM,
			members:   []darwinTestMember{{pid: 101, pgid: 101, state: darwinProcessZombie}},
			wantProof: true,
		},
		{
			name:      "missing leader does not resolve ESRCH",
			signalErr: syscall.ESRCH,
			members:   nil,
			wantCause: CauseChildTerminate,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.waitSteps = []darwinWaitStep{
				{info: darwinSiginfo{}},
				{info: validDarwinInfo(101, darwinChildKilled, 9)},
			}
			harness.signalErrors = []error{test.signalErr}
			harness.snapshots = [][]darwinTestMember{test.members}
			harness.command.reap = processReapResult{reliable: true, pid: 101, signaled: true, signal: 9}
			request := harness.request()
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			request.ctx = canceled
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			if result.quiescence.proven != test.wantProof {
				t.Fatalf("quiescence = %+v", result.quiescence)
			}
			if test.wantCause != "" {
				requireProcessCause(t, result.quiescence.causes, test.wantCause)
			}
			if result.primary == nil || result.primary.Child == nil || !result.primary.Child.ExitStatusObserved || result.primary.Child.ExitStatus != -9 {
				t.Fatalf("primary/child = %+v", result.primary)
			}
			if harness.signalCalls != 1 || harness.snapshotCalls != 1 || harness.waitCalls() != 1 {
				t.Fatalf("calls: signal=%d snapshot=%d Wait=%d", harness.signalCalls, harness.snapshotCalls, harness.waitCalls())
			}
		})
	}
}

func TestDarwinSupervisorPostTerminalSurvivorDriftPreservesStatus(t *testing.T) {
	admitted := darwinOldSigaction{handler: 0x1234}
	harness := newDarwinHarness(admitted)
	harness.actions = []darwinActionStep{{action: admitted}, {action: admitted}, {action: darwinOldSigaction{handler: 0x5678}}}
	harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
	harness.snapshots = [][]darwinTestMember{{
		{pid: 101, pgid: 101, state: darwinProcessZombie},
		{pid: 102, pgid: 101, state: 2},
	}}
	harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, harness.request())
	if result.background != nil || harness.signalCalls != 0 || harness.snapshotCalls != 1 || harness.waitCalls() != 1 {
		t.Fatalf("background=%v signal=%d snapshot=%d Wait=%d", result.background != nil, harness.signalCalls, harness.snapshotCalls, harness.waitCalls())
	}
	requireProcessCause(t, result.quiescence.causes, CauseChildSurvivor)
	requireProcessCause(t, result.quiescence.causes, CauseChildWait)
	if result.quiescence.child == nil || !result.quiescence.child.ExitStatusObserved || result.quiescence.child.ExitStatus != 0 {
		t.Fatalf("preserved child status = %+v", result.quiescence.child)
	}
}

func TestDarwinSupervisorPostTerminalQueryErrorAndUnsafeActionReapSynchronously(t *testing.T) {
	admitted := darwinOldSigaction{handler: 0x1234}
	for _, test := range []struct {
		name string
		step darwinActionStep
	}{
		{name: "query error", step: darwinActionStep{err: syscall.EIO}},
		{name: "ignored", step: darwinActionStep{action: darwinOldSigaction{handler: darwinSIGIgnored}}},
		{name: "no child wait", step: darwinActionStep{action: darwinOldSigaction{flags: darwinSANoChildWait}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(admitted)
			harness.actions = []darwinActionStep{{action: admitted}, {action: admitted}, test.step}
			harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
			harness.snapshots = [][]darwinTestMember{{
				{pid: 101, pgid: 101, state: darwinProcessZombie},
				{pid: 102, pgid: 101, state: 2},
			}}
			harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, harness.request())
			requireProcessCause(t, result.quiescence.causes, CauseChildSurvivor)
			requireProcessCause(t, result.quiescence.causes, CauseChildWait)
			if result.background != nil || harness.signalCalls != 0 || harness.snapshotCalls != 1 || harness.waitCalls() != 1 {
				t.Fatalf("background=%v signal=%d snapshot=%d Wait=%d", result.background != nil, harness.signalCalls, harness.snapshotCalls, harness.waitCalls())
			}
			if result.quiescence.child == nil || !result.quiescence.child.ExitStatusObserved {
				t.Fatalf("terminal status not preserved: %+v", result.quiescence.child)
			}
		})
	}
}

func TestDarwinSupervisorBoundsExitObservationAndSurvivorFollowup(t *testing.T) {
	t.Run("post signal exit observation", func(t *testing.T) {
		harness := newDarwinHarness(darwinOldSigaction{})
		harness.signalErrors = []error{nil}
		request := harness.request()
		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		request.ctx = canceled
		supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
		if initial != nil {
			t.Fatalf("initial failure = %+v", initial)
		}
		result := harness.run(supervisor, request)
		if result.background == nil || harness.signalCalls != 1 || harness.snapshotCalls != 0 {
			t.Fatalf("handoff=%v signal=%d snapshot=%d", result.background != nil, harness.signalCalls, harness.snapshotCalls)
		}
		requireProcessCause(t, result.quiescence.causes, CauseChildWait)
		if elapsed := harness.scheduler.now().Sub(harness.base); elapsed != processExitObservationWindow {
			t.Fatalf("exit observation elapsed = %s", elapsed)
		}
		harness.scheduler.requireMaxWait(t, processPollInterval)
		<-result.background.done
	})

	t.Run("survivor snapshots", func(t *testing.T) {
		harness := newDarwinHarness(darwinOldSigaction{})
		harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
		harness.snapshots = [][]darwinTestMember{{
			{pid: 101, pgid: 101, state: darwinProcessZombie},
			{pid: 102, pgid: 101, state: 2},
		}}
		harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
		supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
		if initial != nil {
			t.Fatalf("initial failure = %+v", initial)
		}
		result := harness.run(supervisor, harness.request())
		requireProcessCause(t, result.quiescence.causes, CauseChildSurvivor)
		if harness.snapshotCalls != 1+processSurvivorSnapshotLimit || harness.signalCalls != 1 || harness.waitCalls() != 1 {
			t.Fatalf("calls: snapshot=%d signal=%d Wait=%d", harness.snapshotCalls, harness.signalCalls, harness.waitCalls())
		}
		if elapsed := harness.scheduler.now().Sub(harness.base); elapsed != processExitObservationWindow {
			t.Fatalf("survivor followup elapsed = %s", elapsed)
		}
		harness.scheduler.requireMaxWait(t, processPollInterval)
	})
}

func TestDarwinSupervisorWaitStateAgreementIsExact(t *testing.T) {
	terminals := []*processTerminal{
		{class: processTerminalExited, status: 7},
		{class: processTerminalKilled, status: 9},
		{class: processTerminalDumped, status: 11},
	}
	matching := []processReapResult{
		{reliable: true, pid: 10, exited: true, exitStatus: 7},
		{reliable: true, pid: 10, signaled: true, signal: 9},
		{reliable: true, pid: 10, signaled: true, signal: 11, coreDumped: true},
	}
	for index := range terminals {
		if !processReapMatches(10, terminals[index], matching[index]) {
			t.Fatalf("matching pair %d refused", index)
		}
		mismatch := matching[index]
		mismatch.pid++
		if processReapMatches(10, terminals[index], mismatch) {
			t.Fatalf("wrong PID pair %d accepted", index)
		}
		mismatch = matching[index]
		mismatch.coreDumped = !mismatch.coreDumped
		if terminals[index].class != processTerminalExited && processReapMatches(10, terminals[index], mismatch) {
			t.Fatalf("core class pair %d was not exact", index)
		}
	}
}

func TestDarwinSupervisorResamplesDeadlineAfterWaitidEINTR(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.waitSteps = []darwinWaitStep{
		{err: syscall.EINTR},
		{info: darwinSiginfo{}},
		{info: validDarwinInfo(101, darwinChildKilled, 9)},
	}
	harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
	harness.command.reap = processReapResult{reliable: true, pid: 101, signaled: true, signal: 9}
	request := harness.request()
	request.phaseDeadline = harness.base.Add(time.Second)
	canceled, cancel := context.WithCancel(context.Background())
	request.ctx = canceled
	deps := harness.dependencies()
	originalWaitID := deps.waitID
	deps.waitID = func(pid int) (darwinSiginfo, error) {
		info, err := originalWaitID(pid)
		if errors.Is(err, syscall.EINTR) {
			harness.scheduler.mu.Lock()
			harness.scheduler.current = request.phaseDeadline
			harness.scheduler.mu.Unlock()
			cancel()
		}
		return info, err
	}
	supervisor, initial := newDarwinProcessSupervisor(deps)
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, request)
	requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseDeadline)
	if harness.waitIDCalls != 3 || harness.signalCalls != 1 || !result.quiescence.proven {
		t.Fatalf("waitid=%d signal=%d quiescence=%+v", harness.waitIDCalls, harness.signalCalls, result.quiescence)
	}
}

func TestDarwinSupervisorBoundsPersistentEINTRAfterStop(t *testing.T) {
	for _, test := range []struct {
		name        string
		prepare     func(*darwinHarness, *processRequest)
		wantPrimary CauseCode
		wantWait    time.Duration
	}{
		{
			name: "cancel",
			prepare: func(_ *darwinHarness, request *processRequest) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				request.ctx = ctx
			},
			wantPrimary: CauseCanceled,
			wantWait:    processPollInterval,
		},
		{
			name: "deadline",
			prepare: func(harness *darwinHarness, request *processRequest) {
				request.phaseDeadline = harness.base
			},
			wantPrimary: CauseDeadline,
			wantWait:    processPollInterval,
		},
		{
			name: "output limit",
			prepare: func(harness *darwinHarness, request *processRequest) {
				harness.stdoutData = []byte("xx")
				harness.stdoutComplete = make(chan struct{})
				harness.command.startGate = harness.stdoutComplete
				request.stdoutLimit = 1
			},
			wantPrimary: CauseLimit,
			wantWait:    processPollInterval,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.waitSteps = []darwinWaitStep{{err: syscall.EINTR}, {err: syscall.EINTR}}
			request := harness.request()
			test.prepare(harness, &request)
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, test.wantPrimary)
			requireProcessCause(t, result.quiescence.causes, CauseChildWait)
			if result.background == nil || harness.waitIDCalls != 2 || harness.signalCalls != 0 || harness.snapshotCalls != 0 {
				t.Fatalf("background=%v waitid=%d signal=%d snapshot=%d", result.background != nil, harness.waitIDCalls, harness.signalCalls, harness.snapshotCalls)
			}
			harness.scheduler.mu.Lock()
			waits := append([]time.Duration(nil), harness.scheduler.waits...)
			wakeEnabled := append([]bool(nil), harness.scheduler.wakeEnabled...)
			cancelEnabled := append([]bool(nil), harness.scheduler.cancelEnabled...)
			harness.scheduler.mu.Unlock()
			channelFreeWaits := make([]time.Duration, 0, len(waits))
			for index, wait := range waits {
				if !wakeEnabled[index] && !cancelEnabled[index] {
					channelFreeWaits = append(channelFreeWaits, wait)
				}
			}
			if len(channelFreeWaits) != 1 || channelFreeWaits[0] != test.wantWait || channelFreeWaits[0] != processPollInterval {
				t.Fatalf("channel-free EINTR waits = %v; all waits=%v", channelFreeWaits, waits)
			}
			<-result.background.done
		})
	}
}

func TestDarwinSupervisorExpiredExitDeadlineSkipsEINTRRetry(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.waitSteps = []darwinWaitStep{{info: darwinSiginfo{}}, {err: syscall.EINTR}}
	request := harness.request()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request.ctx = ctx
	dependencies := harness.dependencies()
	originalWaitID := dependencies.waitID
	dependencies.waitID = func(pid int) (darwinSiginfo, error) {
		info, err := originalWaitID(pid)
		if errors.Is(err, syscall.EINTR) {
			harness.scheduler.mu.Lock()
			harness.scheduler.current = harness.base.Add(processExitObservationWindow)
			harness.scheduler.mu.Unlock()
		}
		return info, err
	}
	supervisor, initial := newDarwinProcessSupervisor(dependencies)
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, request)
	requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseCanceled)
	requireProcessCause(t, result.quiescence.causes, CauseChildWait)
	if result.background == nil || harness.waitIDCalls != 2 || harness.signalCalls != 1 || harness.snapshotCalls != 0 {
		t.Fatalf("background=%v waitid=%d signal=%d snapshot=%d", result.background != nil, harness.waitIDCalls, harness.signalCalls, harness.snapshotCalls)
	}
	<-result.background.done
	harness.scheduler.mu.Lock()
	defer harness.scheduler.mu.Unlock()
	for index, wait := range harness.scheduler.waits {
		if !harness.scheduler.wakeEnabled[index] && !harness.scheduler.cancelEnabled[index] {
			t.Fatalf("expired observation deadline programmed channel-free retry %s", wait)
		}
	}
}

func TestDarwinSupervisorPostSignalEINTRUsesExactPositiveChannelFreeWait(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.waitSteps = []darwinWaitStep{
		{info: darwinSiginfo{}},
		{err: syscall.EINTR},
		{info: validDarwinInfo(101, darwinChildKilled, 9)},
	}
	harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
	harness.command.reap = processReapResult{reliable: true, pid: 101, signaled: true, signal: 9}
	request := harness.request()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request.ctx = ctx
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, request)
	requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseCanceled)
	if !result.quiescence.proven || harness.waitIDCalls != 3 || harness.signalCalls != 1 || harness.snapshotCalls != 1 {
		t.Fatalf("result=%+v waitid=%d signal=%d snapshot=%d", result, harness.waitIDCalls, harness.signalCalls, harness.snapshotCalls)
	}
	harness.scheduler.mu.Lock()
	defer harness.scheduler.mu.Unlock()
	channelFree := make([]time.Duration, 0, 1)
	for index, wait := range harness.scheduler.waits {
		if harness.scheduler.purposes[index] == processWaitObservation &&
			!harness.scheduler.wakeEnabled[index] && !harness.scheduler.cancelEnabled[index] {
			channelFree = append(channelFree, wait)
		}
	}
	if len(channelFree) != 1 || channelFree[0] != processPollInterval || channelFree[0] <= 0 {
		t.Fatalf("post-signal channel-free waits = %v", channelFree)
	}
}

func TestDarwinSupervisorValidWaitidResetsStoppedEINTRBound(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.waitSteps = []darwinWaitStep{
		{err: syscall.EINTR},
		{info: darwinSiginfo{}},
		{err: syscall.EINTR},
		{err: syscall.EINTR},
	}
	request := harness.request()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request.ctx = ctx
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, request)
	if result.background == nil || harness.waitIDCalls != 4 || harness.signalCalls != 1 || harness.snapshotCalls != 0 {
		t.Fatalf("background=%v waitid=%d signal=%d snapshot=%d", result.background != nil, harness.waitIDCalls, harness.signalCalls, harness.snapshotCalls)
	}
	requireProcessCause(t, result.quiescence.causes, CauseChildWait)
	harness.scheduler.requireMaxWait(t, processPollInterval)
	<-result.background.done
}

func TestDarwinSupervisorReadStopSuppressesOnlyAcceptedExactSIGKILL(t *testing.T) {
	for _, test := range []struct {
		name               string
		attemptSignal      bool
		signalErr          error
		terminalCode       int
		terminalStatus     int
		reap               processReapResult
		wantIntentional    bool
		wantTerminateCause bool
	}{
		{
			name:            "accepted exact SIGKILL",
			attemptSignal:   true,
			terminalCode:    darwinChildKilled,
			terminalStatus:  int(syscall.SIGKILL),
			reap:            processReapResult{reliable: true, pid: 101, signaled: true, signal: int(syscall.SIGKILL)},
			wantIntentional: true,
		},
		{
			name:           "unattempted SIGKILL",
			terminalCode:   darwinChildKilled,
			terminalStatus: int(syscall.SIGKILL),
			reap:           processReapResult{reliable: true, pid: 101, signaled: true, signal: int(syscall.SIGKILL)},
		},
		{
			name:           "ESRCH SIGKILL",
			attemptSignal:  true,
			signalErr:      syscall.ESRCH,
			terminalCode:   darwinChildKilled,
			terminalStatus: int(syscall.SIGKILL),
			reap:           processReapResult{reliable: true, pid: 101, signaled: true, signal: int(syscall.SIGKILL)},
		},
		{
			name:           "EPERM SIGKILL",
			attemptSignal:  true,
			signalErr:      syscall.EPERM,
			terminalCode:   darwinChildKilled,
			terminalStatus: int(syscall.SIGKILL),
			reap:           processReapResult{reliable: true, pid: 101, signaled: true, signal: int(syscall.SIGKILL)},
		},
		{
			name:               "hard signal failure SIGKILL",
			attemptSignal:      true,
			signalErr:          syscall.EIO,
			terminalCode:       darwinChildKilled,
			terminalStatus:     int(syscall.SIGKILL),
			reap:               processReapResult{reliable: true, pid: 101, signaled: true, signal: int(syscall.SIGKILL)},
			wantTerminateCause: true,
		},
		{
			name:           "accepted wrong signal",
			attemptSignal:  true,
			terminalCode:   darwinChildKilled,
			terminalStatus: int(syscall.SIGTERM),
			reap:           processReapResult{reliable: true, pid: 101, signaled: true, signal: int(syscall.SIGTERM)},
		},
		{
			name:           "accepted natural nonzero exit",
			attemptSignal:  true,
			terminalCode:   darwinChildExited,
			terminalStatus: 7,
			reap:           processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 7},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.stdoutReadErr = syscall.EIO
			harness.stdoutReadAttempt = make(chan struct{})
			harness.stdoutReadRelease = make(chan struct{})
			harness.scheduler.requiredWake = 1
			harness.waitSteps = []darwinWaitStep{{info: darwinSiginfo{}}}
			if test.attemptSignal {
				// The second no-result sample occurs after the read stop is
				// latched, making the following group signal causally exact.
				harness.waitSteps = append(harness.waitSteps, darwinWaitStep{info: darwinSiginfo{}})
				harness.signalErrors = []error{test.signalErr}
			}
			harness.waitSteps = append(harness.waitSteps, darwinWaitStep{
				info: validDarwinInfo(101, test.terminalCode, test.terminalStatus),
			})
			harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
			harness.command.reap = test.reap
			dependencies := harness.dependencies()
			originalWaitID := dependencies.waitID
			firstWait := true
			dependencies.waitID = func(pid int) (darwinSiginfo, error) {
				if firstWait {
					firstWait = false
					<-harness.stdoutReadAttempt
					close(harness.stdoutReadRelease)
				}
				return originalWaitID(pid)
			}
			supervisor, initial := newDarwinProcessSupervisor(dependencies)
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, harness.request())
			if test.wantIntentional {
				if result.primary != nil {
					t.Fatalf("accepted exact SIGKILL invented Primary: %+v", result.primary)
				}
				if result.quiescence.child == nil || !result.quiescence.child.ExitStatusObserved ||
					result.quiescence.child.ExitStatus != -int(syscall.SIGKILL) {
					t.Fatalf("cleanup child = %+v", result.quiescence.child)
				}
			} else {
				requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseChildExit)
				if result.primary.Child == nil || !result.primary.Child.ExitStatusObserved {
					t.Fatalf("unintentional abnormal exit child = %+v", result.primary.Child)
				}
			}
			requireProcessCause(t, result.quiescence.causes, CauseChildDrain)
			if test.wantTerminateCause {
				requireProcessCause(t, result.quiescence.causes, CauseChildTerminate)
			}
			wantSignals := 0
			if test.attemptSignal {
				wantSignals = 1
			}
			if harness.signalCalls != wantSignals || harness.waitIDCalls != len(harness.waitSteps) ||
				harness.snapshotCalls != 1 || harness.waitCalls() != 1 {
				t.Fatalf("calls: signal=%d/%d waitid=%d/%d snapshot=%d Wait=%d",
					harness.signalCalls, wantSignals, harness.waitIDCalls, len(harness.waitSteps),
					harness.snapshotCalls, harness.waitCalls())
			}
			harness.scheduler.requireCompletionConsumed(t)
		})
	}
}

func TestDarwinSetPrimaryDoesNotTreatDumpedSIGKILLAsIntentional(t *testing.T) {
	run := darwinProcessRun{request: processRequest{phase: PhaseSource}}
	run.setPrimary(
		processStop{kind: processStopRead},
		&processTerminal{class: processTerminalDumped, status: int(syscall.SIGKILL)},
		true,
	)
	requireProcessRecord(t, run.result.primary, PhaseSource, OperationExecute, CauseChildExit)
}

func TestDarwinStructuredOutputPublishesOnlyAfterEverySuccessGate(t *testing.T) {
	ready := func() darwinProcessRun {
		return darwinProcessRun{
			request:         processRequest{phase: PhaseSource},
			result:          processResult{quiescence: quiescenceResult{proven: true}},
			publicTerminal:  &processTerminal{class: processTerminalExited, status: 0},
			candidateOutput: []byte("structured"),
			ioComplete:      true,
			stderr:          processStderrResult{eof: true},
		}
	}
	for _, test := range []struct {
		name   string
		mutate func(*darwinProcessRun)
	}{
		{name: "primary", mutate: func(run *darwinProcessRun) {
			run.result.primary = &FailureRecord{Phase: PhaseSource, Operation: OperationExecute}
		}},
		{name: "descriptor close", mutate: func(run *darwinProcessRun) {
			run.result.descriptorClose = &FailureRecord{Phase: PhaseClose, Operation: OperationCloseNonRoot}
		}},
		{name: "quiescence gap", mutate: func(run *darwinProcessRun) { run.result.quiescence.proven = false }},
		{name: "background handoff", mutate: func(run *darwinProcessRun) {
			run.result.background = &processBackgroundReap{done: make(chan processReapResult)}
		}},
		{name: "io gap", mutate: func(run *darwinProcessRun) { run.ioComplete = false }},
		{name: "missing terminal", mutate: func(run *darwinProcessRun) { run.publicTerminal = nil }},
		{name: "nonzero terminal", mutate: func(run *darwinProcessRun) { run.publicTerminal.status = 1 }},
		{name: "successful stderr", mutate: func(run *darwinProcessRun) { run.stderr.total = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := ready()
			test.mutate(&run)
			run.publishStructuredOutput()
			if run.result.structuredOutput != nil {
				t.Fatalf("gate published %q", run.result.structuredOutput)
			}
		})
	}
	run := ready()
	run.publishStructuredOutput()
	if string(run.result.structuredOutput) != "structured" {
		t.Fatalf("proved result = %q", run.result.structuredOutput)
	}
}

func TestDarwinSuccessfulStderrGateIsParseMalformedWithStatusZeroChild(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.stdoutData = []byte("withheld")
	harness.stderrData = []byte("diagnostic")
	harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
	harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
	harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, harness.request())
	requireProcessRecord(t, result.primary, PhaseSource, OperationParse, CauseMalformed)
	if result.structuredOutput != nil || result.primary.Child == nil ||
		!result.primary.Child.ExitStatusObserved || result.primary.Child.ExitStatus != 0 ||
		result.primary.Child.DiagnosticBytes != uint64(len(harness.stderrData)) ||
		!result.quiescence.proven || result.descriptorClose != nil {
		t.Fatalf("stderr-gated result = %+v", result)
	}
}

func TestDarwinSupervisorCompletionDrivenSchedulingStress(t *testing.T) {
	for iteration := range 250 {
		harness := newDarwinHarness(darwinOldSigaction{})
		harness.stdoutReadErr = syscall.EIO
		harness.stdoutReadAttempt = make(chan struct{})
		harness.stdoutReadRelease = make(chan struct{})
		harness.scheduler.requiredWake = 1
		harness.waitSteps = []darwinWaitStep{
			{info: darwinSiginfo{}},
			{info: darwinSiginfo{}},
			{info: validDarwinInfo(101, darwinChildKilled, 9)},
		}
		harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
		harness.command.reap = processReapResult{reliable: true, pid: 101, signaled: true, signal: 9}
		dependencies := harness.dependencies()
		originalWaitID := dependencies.waitID
		firstWait := true
		dependencies.waitID = func(pid int) (darwinSiginfo, error) {
			if firstWait {
				firstWait = false
				<-harness.stdoutReadAttempt
				close(harness.stdoutReadRelease)
			}
			return originalWaitID(pid)
		}
		supervisor, initial := newDarwinProcessSupervisor(dependencies)
		if initial != nil {
			t.Fatalf("iteration %d initial failure = %+v", iteration, initial)
		}
		result := harness.run(supervisor, harness.request())
		if result.primary != nil || harness.signalCalls != 1 {
			t.Fatalf("iteration %d primary = %+v", iteration, result.primary)
		}
		requireProcessCause(t, result.quiescence.causes, CauseChildDrain)
		harness.scheduler.requireCompletionConsumed(t)
	}
}

func TestDarwinSupervisorDeliberatelyDelayedFastExitObservation(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.waitSteps = []darwinWaitStep{
		{info: darwinSiginfo{}},
		{info: darwinSiginfo{}},
		{info: darwinSiginfo{}},
		{info: validDarwinInfo(101, darwinChildExited, 0)},
	}
	harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
	harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, harness.request())
	if result.primary != nil || !result.quiescence.proven || harness.waitIDCalls != 4 || harness.signalCalls != 0 {
		t.Fatalf("result=%+v waitid=%d signal=%d", result, harness.waitIDCalls, harness.signalCalls)
	}
	harness.scheduler.requireMaxWait(t, processPollInterval)
}

func TestDarwinSupervisorMapsSignalProbeAndReapFaults(t *testing.T) {
	for _, test := range []struct {
		name          string
		signalErr     error
		snapshot      []darwinTestMember
		snapshotError error
		overflow      bool
		reap          processReapResult
		wantCause     CauseCode
	}{
		{
			name:      "group signal failure",
			signalErr: syscall.EIO,
			snapshot:  []darwinTestMember{{pid: 101, pgid: 101, state: darwinProcessZombie}},
			reap:      processReapResult{reliable: true, pid: 101, signaled: true, signal: 9},
			wantCause: CauseChildTerminate,
		},
		{
			name:          "snapshot syscall failure",
			snapshotError: syscall.EIO,
			reap:          processReapResult{reliable: true, pid: 101, signaled: true, signal: 9},
			wantCause:     CauseChildProbe,
		},
		{
			name:      "snapshot overflow",
			overflow:  true,
			reap:      processReapResult{reliable: true, pid: 101, signaled: true, signal: 9},
			wantCause: CauseChildProbe,
		},
		{
			name:      "Wait mismatch",
			snapshot:  []darwinTestMember{{pid: 101, pgid: 101, state: darwinProcessZombie}},
			reap:      processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0},
			wantCause: CauseChildWait,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.waitSteps = []darwinWaitStep{
				{info: darwinSiginfo{}},
				{info: validDarwinInfo(101, darwinChildKilled, 9)},
			}
			harness.signalErrors = []error{test.signalErr}
			harness.snapshots = [][]darwinTestMember{test.snapshot}
			harness.snapshotErr = test.snapshotError
			harness.snapshotOverflow = test.overflow
			harness.command.reap = test.reap
			request := harness.request()
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			request.ctx = canceled
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			requireProcessCause(t, result.quiescence.causes, test.wantCause)
			if result.quiescence.proven || harness.waitCalls() != 1 {
				t.Fatalf("quiescence=%+v Wait=%d", result.quiescence, harness.waitCalls())
			}
		})
	}
}

func TestDarwinSupervisorMapsWorkerFailuresByFixedPriority(t *testing.T) {
	for _, test := range []struct {
		name          string
		stdout        string
		stdoutLimit   uint64
		input         []byte
		inputError    error
		stdoutReadErr error
		wantPrimary   CauseCode
		wantOperation Operation
		wantCleanup   CauseCode
	}{
		{name: "stdout limit", stdout: "1234", stdoutLimit: 3, wantPrimary: CauseLimit, wantOperation: OperationExecute},
		{name: "input", input: []byte("input"), inputError: syscall.EPIPE, wantPrimary: CauseUnstable, wantOperation: OperationExecute},
		{name: "read", stdoutReadErr: syscall.EIO, wantCleanup: CauseChildDrain},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.stdoutData = []byte(test.stdout)
			harness.stdoutReadErr = test.stdoutReadErr
			harness.inputWriteErr = test.inputError
			if test.stdoutLimit != 0 {
				harness.stdoutComplete = make(chan struct{})
				harness.command.startGate = harness.stdoutComplete
			}
			if test.stdoutReadErr != nil {
				harness.stdoutReadAttempt = make(chan struct{})
				harness.stdoutReadRelease = make(chan struct{})
				harness.scheduler.requiredWake = 1
			}
			harness.waitSteps = []darwinWaitStep{
				{info: darwinSiginfo{}},
				{info: validDarwinInfo(101, darwinChildExited, 0)},
			}
			harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
			harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
			request := harness.request()
			request.input = newBoundedProcessInput(test.input)
			if test.stdoutLimit != 0 {
				request.stdoutLimit = test.stdoutLimit
			}
			dependencies := harness.dependencies()
			waitReleasedAfterInput := false
			if test.inputError != nil {
				harness.inputWriteAttempt = make(chan struct{})
				harness.scheduler.requiredWake = 1
				originalWaitID := dependencies.waitID
				dependencies.waitID = func(pid int) (darwinSiginfo, error) {
					<-harness.inputWriteAttempt
					waitReleasedAfterInput = true
					return originalWaitID(pid)
				}
			}
			if test.stdoutReadErr != nil {
				originalWaitID := dependencies.waitID
				firstWait := true
				dependencies.waitID = func(pid int) (darwinSiginfo, error) {
					if firstWait {
						firstWait = false
						<-harness.stdoutReadAttempt
						close(harness.stdoutReadRelease)
					}
					return originalWaitID(pid)
				}
			}
			supervisor, initial := newDarwinProcessSupervisor(dependencies)
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, request)
			if test.wantPrimary != "" {
				requireProcessRecord(t, result.primary, PhaseSource, test.wantOperation, test.wantPrimary)
			} else if result.primary != nil {
				t.Fatalf("primary = %+v", result.primary)
			}
			if test.wantCleanup != "" {
				requireProcessCause(t, result.quiescence.causes, test.wantCleanup)
			}
			if test.inputError != nil && !waitReleasedAfterInput {
				t.Fatal("terminal observation was not causally gated on the injected stdin failure")
			}
			if test.stdoutReadErr != nil {
				harness.scheduler.requireCompletionConsumed(t)
			} else if test.inputError != nil {
				harness.scheduler.requireCompletionSafe(t)
			}
		})
	}
}

func TestDarwinSupervisorTerminalLatchSurvivesLateInputFailure(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.inputWriteErr = syscall.EPIPE
	harness.inputWriteAttempt = make(chan struct{})
	harness.inputWriteRelease = make(chan struct{})
	harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 7)}}
	harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
	harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 7}
	request := harness.request()
	request.input = newBoundedProcessInput([]byte("input"))
	dependencies := harness.dependencies()
	originalWaitID := dependencies.waitID
	dependencies.waitID = func(pid int) (darwinSiginfo, error) {
		<-harness.inputWriteAttempt
		return originalWaitID(pid)
	}
	originalSnapshot := dependencies.snapshot
	released := false
	dependencies.snapshot = func(pid int, buffer *darwinGroupBuffer) (darwinSnapshotResult, error) {
		if !released {
			released = true
			close(harness.inputWriteRelease)
		}
		return originalSnapshot(pid, buffer)
	}
	supervisor, initial := newDarwinProcessSupervisor(dependencies)
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, request)
	requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseChildExit)
	requireProcessCause(t, result.quiescence.causes, CauseChildDrain)
	if !released || result.background != nil || harness.signalCalls != 0 || harness.waitCalls() != 1 ||
		result.primary.Child == nil || !result.primary.Child.ExitStatusObserved || result.primary.Child.ExitStatus != 7 {
		t.Fatalf("terminal latch=%+v released=%v background=%v signal=%d Wait=%d",
			result.primary, released, result.background != nil, harness.signalCalls, harness.waitCalls())
	}
}

func TestDarwinSupervisorDescriptorFailureOwnsOnlyChildDiagnostic(t *testing.T) {
	for failureAt := 1; failureAt <= 6; failureAt++ {
		t.Run(string(rune('0'+failureAt)), func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.stdoutData = []byte("withheld")
			harness.endCloseFailureAt = failureAt
			harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
			harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
			harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, harness.request())
			if result.primary != nil || result.descriptorClose == nil || result.descriptorClose.Child == nil ||
				result.quiescence.child != nil || !result.quiescence.proven || result.structuredOutput != nil {
				t.Fatalf("result = primary %+v descriptor %+v quiescence %+v output=%q",
					result.primary, result.descriptorClose, result.quiescence, result.structuredOutput)
			}
			harness.requireEveryCreatedEndClosedOnce(t)
		})
	}
}

func TestDarwinSupervisorLateBackgroundWaitCannotMutateSealedResult(t *testing.T) {
	harness := newDarwinHarness(darwinOldSigaction{})
	harness.waitSteps = []darwinWaitStep{{err: syscall.ECHILD}}
	harness.command.waitBlock = make(chan struct{})
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, harness.request())
	if result.background == nil || result.quiescence.child == nil || result.quiescence.child.ExitStatusObserved {
		t.Fatalf("sealed result = %+v", result)
	}
	before := *result.quiescence.child
	close(harness.command.waitBlock)
	<-result.background.done
	if *result.quiescence.child != before || result.quiescence.child.ExitStatusObserved {
		t.Fatalf("private Wait mutated public child: before=%+v after=%+v", before, result.quiescence.child)
	}
}

func TestDarwinBackgroundHandoffTransfersOnlySealedReaperCapability(t *testing.T) {
	reaperType := reflect.TypeFor[darwinBackgroundReaper]()
	waitType := reflect.TypeFor[func() processReapResult]()
	if reaperType.Kind() != reflect.Struct || reaperType.NumField() != 1 {
		t.Fatalf("background reaper capability = %s with %d fields", reaperType, reaperType.NumField())
	}
	waitField := reaperType.Field(0)
	if waitField.Name != "waitCommand" || waitField.Type != waitType || waitField.PkgPath == "" || waitField.Anonymous {
		t.Fatalf("background reaper field = %s %s exported=%v anonymous=%v",
			waitField.Name, waitField.Type, waitField.PkgPath == "", waitField.Anonymous)
	}
	if got, want := reflect.TypeOf(sealDarwinBackgroundReaper),
		reflect.TypeFor[func(processCommand) darwinBackgroundReaper](); got != want {
		t.Fatalf("reaper seal type = %s, want %s", got, want)
	}
	if got, want := reflect.TypeOf(reapDarwinProcessInBackground),
		reflect.TypeFor[func(darwinBackgroundReaper, chan<- processReapResult)](); got != want {
		t.Fatalf("background reaper entry type = %s, want %s", got, want)
	}

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate Darwin process test source")
	}
	parsed, err := parser.ParseFile(
		token.NewFileSet(),
		filepath.Join(filepath.Dir(testFile), "process_darwin.go"),
		nil,
		0,
	)
	if err != nil {
		t.Fatalf("parse process_darwin.go: %v", err)
	}
	var handoff *ast.FuncDecl
	var seal *ast.FuncDecl
	var backgroundEntry *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch function.Name.Name {
		case "backgroundHandoff":
			handoff = function
		case "sealDarwinBackgroundReaper":
			seal = function
		case "reapDarwinProcessInBackground":
			backgroundEntry = function
		}
	}
	if handoff == nil || seal == nil || backgroundEntry == nil {
		t.Fatal("background handoff, reaper seal, or reaper entry declaration not found")
	}
	goCalls := 0
	reaperAssignments := 0
	unsafeLaunch := ""
	ast.Inspect(handoff.Body, func(node ast.Node) bool {
		if assignment, ok := node.(*ast.AssignStmt); ok {
			for _, target := range assignment.Lhs {
				identifier, isIdentifier := target.(*ast.Ident)
				if !isIdentifier || identifier.Name != "reaper" {
					continue
				}
				reaperAssignments++
				if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || assignment.Tok != token.DEFINE {
					unsafeLaunch = "sealed reaper assignment shape drifted"
					continue
				}
				call, isCall := assignment.Rhs[0].(*ast.CallExpr)
				if !isCall {
					unsafeLaunch = "background reaper no longer comes directly from its seal"
					continue
				}
				callee, direct := call.Fun.(*ast.Ident)
				if !direct || callee.Name != "sealDarwinBackgroundReaper" || len(call.Args) != 1 {
					unsafeLaunch = "background reaper no longer comes directly from its seal"
					continue
				}
				selector, isSelector := call.Args[0].(*ast.SelectorExpr)
				if !isSelector {
					unsafeLaunch = "background reaper seal input drifted from the command"
					continue
				}
				receiver, isReceiver := selector.X.(*ast.Ident)
				if !isReceiver || receiver.Name != "run" || selector.Sel.Name != "command" {
					unsafeLaunch = "background reaper seal input drifted from the command"
				}
			}
		}
		statement, ok := node.(*ast.GoStmt)
		if !ok {
			return true
		}
		goCalls++
		callee, direct := statement.Call.Fun.(*ast.Ident)
		if !direct || callee.Name != "reapDarwinProcessInBackground" {
			unsafeLaunch = "background launch is not the fixed reaper entry"
			return false
		}
		if len(statement.Call.Args) != 2 {
			unsafeLaunch = "background launch argument count drifted"
			return false
		}
		wantArguments := []string{"reaper", "reapDone"}
		for index, argument := range statement.Call.Args {
			identifier, identifierOnly := argument.(*ast.Ident)
			if !identifierOnly || identifier.Name != wantArguments[index] {
				unsafeLaunch = "background launch captures an expression instead of sealed values"
				return false
			}
		}
		return false
	})
	if unsafeLaunch != "" || goCalls != 1 || reaperAssignments != 1 {
		t.Fatalf("background launch structure: calls=%d reaper assignments=%d problem=%s",
			goCalls, reaperAssignments, unsafeLaunch)
	}
	if len(seal.Body.List) != 1 {
		t.Fatal("background reaper seal must be a single direct return")
	}
	returned, isReturn := seal.Body.List[0].(*ast.ReturnStmt)
	if !isReturn || len(returned.Results) != 1 {
		t.Fatal("background reaper seal must return exactly one capability")
	}
	capability, isComposite := returned.Results[0].(*ast.CompositeLit)
	if !isComposite || len(capability.Elts) != 1 {
		t.Fatal("background reaper seal must construct exactly one-field capability")
	}
	field, isField := capability.Elts[0].(*ast.KeyValueExpr)
	if !isField {
		t.Fatal("background reaper seal must name its only authority field")
	}
	fieldName, isFieldName := field.Key.(*ast.Ident)
	waitSelector, isWaitSelector := field.Value.(*ast.SelectorExpr)
	if !isWaitSelector {
		t.Fatal("background reaper seal must retain only the command wait method")
	}
	command, isCommand := waitSelector.X.(*ast.Ident)
	if !isFieldName || fieldName.Name != "waitCommand" ||
		!isCommand || command.Name != "command" || waitSelector.Sel.Name != "wait" {
		t.Fatal("background reaper seal no longer narrows command authority to wait")
	}
	if len(backgroundEntry.Body.List) != 1 {
		t.Fatal("background reaper entry must contain only the result send")
	}
	send, isSend := backgroundEntry.Body.List[0].(*ast.SendStmt)
	if !isSend {
		t.Fatal("background reaper entry must only send the wait result")
	}
	done, isDone := send.Chan.(*ast.Ident)
	waitCall, isWaitCall := send.Value.(*ast.CallExpr)
	if !isWaitCall {
		t.Fatal("background reaper entry must call the sealed wait capability")
	}
	backgroundWait, isBackgroundWait := waitCall.Fun.(*ast.SelectorExpr)
	if !isBackgroundWait {
		t.Fatal("background reaper entry must call the sealed wait capability directly")
	}
	backgroundReaper, isBackgroundReaper := backgroundWait.X.(*ast.Ident)
	if !isDone || done.Name != "done" || len(waitCall.Args) != 0 || !isBackgroundReaper ||
		backgroundReaper.Name != "reaper" || backgroundWait.Sel.Name != "waitCommand" {
		t.Fatal("background reaper entry gained authority beyond wait and its result channel")
	}
}

func TestDarwinSupervisorAcceptsExact4096RowSnapshotAndRefusesGrowth(t *testing.T) {
	admitted := darwinOldSigaction{handler: 0x1234}
	harness := newDarwinHarness(admitted)
	harness.actions = []darwinActionStep{
		{action: admitted},
		{action: admitted},
		// Stop after the admitted initial snapshot rather than taking survivor
		// follow-ups; this test owns the exact inventory ceiling itself.
		{action: darwinOldSigaction{handler: 0x5678}},
	}
	harness.waitSteps = []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}
	members := make([]darwinTestMember, processGroupMemberLimit)
	members[0] = darwinTestMember{pid: 101, pgid: 101, state: darwinProcessZombie}
	for index := 1; index < len(members); index++ {
		members[index] = darwinTestMember{pid: 101 + index, pgid: 101, state: 2}
	}
	harness.snapshots = [][]darwinTestMember{members}
	harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
	supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
	if initial != nil {
		t.Fatalf("initial failure = %+v", initial)
	}
	result := harness.run(supervisor, harness.request())
	requireProcessCause(t, result.quiescence.causes, CauseChildSurvivor)
	requireProcessCause(t, result.quiescence.causes, CauseChildWait)
	if harness.snapshotCalls != 1 || harness.signalCalls != 0 || harness.waitCalls() != 1 {
		t.Fatalf("calls: snapshot=%d signal=%d Wait=%d", harness.snapshotCalls, harness.signalCalls, harness.waitCalls())
	}
}

func TestDarwinSupervisorUsesExactOneSecondPipeDeadlines(t *testing.T) {
	for _, test := range []struct {
		name       string
		waitSteps  []darwinWaitStep
		background bool
	}{
		{name: "synchronous", waitSteps: []darwinWaitStep{{info: validDarwinInfo(101, darwinChildExited, 0)}}},
		{name: "handoff", waitSteps: []darwinWaitStep{{err: syscall.ECHILD}}, background: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			harness := newDarwinHarness(darwinOldSigaction{})
			harness.waitSteps = test.waitSteps
			harness.heldReaders = true
			harness.snapshots = [][]darwinTestMember{{{pid: 101, pgid: 101, state: darwinProcessZombie}}}
			harness.command.reap = processReapResult{reliable: true, pid: 101, exited: true, exitStatus: 0}
			supervisor, initial := newDarwinProcessSupervisor(harness.dependencies())
			if initial != nil {
				t.Fatalf("initial failure = %+v", initial)
			}
			result := harness.run(supervisor, harness.request())
			if (result.background != nil) != test.background {
				t.Fatalf("background = %v", result.background != nil)
			}
			requireProcessCause(t, result.quiescence.causes, CauseChildDrain)
			if elapsed := harness.scheduler.now().Sub(harness.base); elapsed != processPipeCompletionWindow {
				t.Fatalf("pipe completion elapsed = %s", elapsed)
			}
			harness.scheduler.requireMaxWait(t, processPollInterval)
			if result.background != nil {
				<-result.background.done
			}
		})
	}
}

func TestRealDarwinSupervisorSuccessAndFailure(t *testing.T) {
	supervisor, initial := newProcessSupervisor()
	if initial != nil {
		t.Fatalf("initial supervisor refusal = %+v", initial)
	}
	workspace, err := newWorkspacePaths(t.TempDir())
	if err != nil {
		t.Fatalf("workspace paths: %v", err)
	}
	environment, err := newTaskPrivateEnvironment(workspace, workspace.child("goroot-authority"), t.TempDir())
	if err != nil {
		t.Fatalf("task-private environment: %v", err)
	}
	for _, test := range []struct {
		name       string
		script     string
		wantOutput string
		wantOp     Operation
		wantCause  CauseCode
		wantStatus int
	}{
		{name: "success", script: "printf stdout", wantOutput: "stdout"},
		{name: "status zero stderr", script: "printf stdout; printf diagnostic >&2", wantOp: OperationParse, wantCause: CauseMalformed},
		{name: "nonzero", script: "printf failure >&2; exit 7", wantOp: OperationExecute, wantCause: CauseChildExit, wantStatus: 7},
		{name: "unrequested signal", script: "kill -TERM $$", wantOp: OperationExecute, wantCause: CauseChildExit, wantStatus: -int(syscall.SIGTERM)},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := supervisor.runTaskPrivate(processRequest{
				ctx:           context.Background(),
				phase:         PhaseSource,
				executable:    "/bin/sh",
				arguments:     []string{"-c", test.script},
				directory:     workspace.taskRoot,
				input:         newBoundedProcessInput(nil),
				stdoutLimit:   1 << 20,
				phaseDeadline: time.Now().Add(5 * time.Second),
			}, environment)
			if string(result.structuredOutput) != test.wantOutput ||
				(test.wantCause != "" && result.structuredOutput != nil) {
				t.Fatalf("stdout = %q; primary=%+v cleanup=%+v close=%+v", result.structuredOutput, result.primary, result.quiescence, result.descriptorClose)
			}
			if !result.quiescence.proven || result.background != nil || result.descriptorClose != nil {
				t.Fatalf("result proof/background/close = %+v / %v / %+v", result.quiescence, result.background != nil, result.descriptorClose)
			}
			if test.wantCause == "" {
				if result.primary != nil {
					t.Fatalf("success primary = %+v", result.primary)
				}
				return
			}
			requireProcessRecord(t, result.primary, PhaseSource, test.wantOp, test.wantCause)
			if result.primary.Child == nil || !result.primary.Child.ExitStatusObserved || result.primary.Child.ExitStatus != test.wantStatus {
				t.Fatalf("child = %+v", result.primary.Child)
			}
		})
	}
}

func TestRealDarwinSupervisorCancellationKillsOnlyOwnedGroup(t *testing.T) {
	supervisor, initial := newProcessSupervisor()
	if initial != nil {
		t.Fatalf("initial supervisor refusal = %+v", initial)
	}
	workspace, err := newWorkspacePaths(t.TempDir())
	if err != nil {
		t.Fatalf("workspace paths: %v", err)
	}
	environment, err := newTaskPrivateEnvironment(workspace, workspace.child("goroot-authority"), t.TempDir())
	if err != nil {
		t.Fatalf("task-private environment: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	result := supervisor.runTaskPrivate(processRequest{
		ctx:           ctx,
		phase:         PhaseSource,
		executable:    "/bin/sh",
		arguments:     []string{"-c", "exec /bin/sleep 30"},
		directory:     workspace.taskRoot,
		input:         newBoundedProcessInput(nil),
		stdoutLimit:   1 << 20,
		phaseDeadline: time.Now().Add(5 * time.Second),
	}, environment)
	if time.Since(started) > 2*time.Second {
		t.Fatal("canceled child cleanup exceeded focused test bound")
	}
	requireProcessRecord(t, result.primary, PhaseSource, OperationExecute, CauseCanceled)
	if result.structuredOutput != nil || !result.quiescence.proven || result.background != nil || result.primary.Child == nil ||
		!result.primary.Child.ExitStatusObserved || result.primary.Child.ExitStatus != -int(syscall.SIGKILL) {
		t.Fatalf("canceled result = %+v", result)
	}
}

type darwinActionStep struct {
	action darwinOldSigaction
	err    error
}

type darwinWaitStep struct {
	info darwinSiginfo
	err  error
}

type darwinHarness struct {
	base              time.Time
	admitted          darwinOldSigaction
	actions           []darwinActionStep
	actionCalls       int
	waitSteps         []darwinWaitStep
	waitIDCalls       int
	signalErrors      []error
	signalCalls       int
	snapshots         [][]darwinTestMember
	snapshotCalls     int
	snapshotErr       error
	snapshotOverflow  bool
	pipeFailureAt     int
	pipeCalls         int
	heldReaders       bool
	stdoutData        []byte
	stderrData        []byte
	commandSpecs      []processCommandSpec
	commandProfiles   []string
	commandEnvs       [][]string
	stdoutReadErr     error
	inputWriteErr     error
	inputWriteAttempt chan struct{}
	inputWriteRelease chan struct{}
	stdoutComplete    chan struct{}
	stdoutReadAttempt chan struct{}
	stdoutReadRelease chan struct{}
	endCloseFailureAt int
	ends              []processPipeEnd
	command           *fakeProcessCommand
	scheduler         *fakeProcessScheduler
}

func newDarwinHarness(admitted darwinOldSigaction) *darwinHarness {
	base := time.Unix(10_000, 0)
	return &darwinHarness{
		base:      base,
		admitted:  admitted,
		command:   &fakeProcessCommand{pid: 101, reap: processReapResult{reliable: true, pid: 101, exited: true}},
		scheduler: &fakeProcessScheduler{current: base},
	}
}

func (harness *darwinHarness) dependencies() darwinProcessDependencies {
	harness.scheduler.waitForWorkerCompletion = !harness.heldReaders
	return darwinProcessDependencies{
		queryAction: func() (darwinOldSigaction, error) {
			call := harness.actionCalls
			harness.actionCalls++
			if call < len(harness.actions) {
				return harness.actions[call].action, harness.actions[call].err
			}
			return harness.admitted, nil
		},
		waitID: func(int) (darwinSiginfo, error) {
			call := harness.waitIDCalls
			harness.waitIDCalls++
			if call < len(harness.waitSteps) {
				return harness.waitSteps[call].info, harness.waitSteps[call].err
			}
			return darwinSiginfo{}, nil
		},
		signalGroup: func(int) error {
			call := harness.signalCalls
			harness.signalCalls++
			if call < len(harness.signalErrors) {
				return harness.signalErrors[call]
			}
			return nil
		},
		snapshot: func(_ int, buffer *darwinGroupBuffer) (darwinSnapshotResult, error) {
			call := harness.snapshotCalls
			harness.snapshotCalls++
			var members []darwinTestMember
			if len(harness.snapshots) != 0 {
				index := call
				if index >= len(harness.snapshots) {
					index = len(harness.snapshots) - 1
				}
				members = harness.snapshots[index]
			}
			for index, member := range members {
				buffer.rows[index] = unix.KinfoProc{
					Proc:  unix.ExternProc{P_pid: int32(member.pid), P_stat: member.state},
					Eproc: unix.Eproc{Pgid: int32(member.pgid)},
				}
			}
			result := darwinSnapshotResult{
				byteLength: uintptr(len(members)) * unsafe.Sizeof(unix.KinfoProc{}),
				overflow:   harness.snapshotOverflow,
			}
			return result, harness.snapshotErr
		},
		newPreallocationCommand: func(
			spec processCommandSpec,
			environment preallocationEnvironment,
		) processCommand {
			spec.arguments = append([]string(nil), spec.arguments...)
			harness.commandSpecs = append(harness.commandSpecs, spec)
			harness.commandProfiles = append(harness.commandProfiles, "preallocation")
			harness.commandEnvs = append(harness.commandEnvs, environment.clone())
			return harness.command
		},
		newTaskPrivateCommand: func(
			spec processCommandSpec,
			environment taskPrivateEnvironment,
		) processCommand {
			spec.arguments = append([]string(nil), spec.arguments...)
			harness.commandSpecs = append(harness.commandSpecs, spec)
			harness.commandProfiles = append(harness.commandProfiles, "task private")
			harness.commandEnvs = append(harness.commandEnvs, environment.clone())
			return harness.command
		},
		newPipe:   harness.newPipe,
		scheduler: harness.scheduler,
	}
}

func (harness *darwinHarness) request() processRequest {
	return processRequest{
		ctx:           context.Background(),
		phase:         PhaseSource,
		executable:    "/bin/echo",
		directory:     "/private/tmp",
		input:         newBoundedProcessInput(nil),
		stdoutLimit:   1 << 20,
		phaseDeadline: harness.base.Add(time.Hour),
	}
}

func (harness *darwinHarness) run(supervisor processSupervisor, request processRequest) processResult {
	workspace, err := newWorkspacePaths("/private/state/task")
	if err != nil {
		panic(err)
	}
	environment, err := newTaskPrivateEnvironment(workspace, "/private/go", "/private/mod")
	if err != nil {
		panic(err)
	}
	return supervisor.runTaskPrivate(request, environment)
}

func (harness *darwinHarness) newPipe() (processPipe, error) {
	harness.pipeCalls++
	if harness.pipeCalls == harness.pipeFailureAt {
		return processPipe{}, syscall.EMFILE
	}
	var read processPipeEnd
	if harness.heldReaders && harness.pipeCalls <= 2 {
		read = newBlockingProcessEnd()
	} else {
		data := []byte(nil)
		readErr := error(nil)
		switch harness.pipeCalls {
		case 1:
			data = harness.stdoutData
			readErr = harness.stdoutReadErr
		case 2:
			data = harness.stderrData
		}
		read = &memoryProcessEnd{reader: bytes.NewReader(data), readErr: readErr}
		if harness.pipeCalls == 1 {
			stdout := read.(*memoryProcessEnd)
			stdout.readComplete = harness.stdoutComplete
			stdout.readAttempt = harness.stdoutReadAttempt
			stdout.readRelease = harness.stdoutReadRelease
		}
	}
	write := &memoryProcessEnd{reader: bytes.NewReader(nil)}
	if harness.pipeCalls == 3 {
		write.writeErr = harness.inputWriteErr
		write.writeAttempt = harness.inputWriteAttempt
		write.writeRelease = harness.inputWriteRelease
	}
	firstIndex := len(harness.ends) + 1
	harness.ends = append(harness.ends, read, write)
	if harness.endCloseFailureAt == firstIndex {
		if memory, ok := read.(*memoryProcessEnd); ok {
			memory.closeErr = syscall.EIO
		}
	}
	if harness.endCloseFailureAt == firstIndex+1 {
		write.closeErr = syscall.EIO
	}
	return processPipe{read: read, write: write}, nil
}

func (harness *darwinHarness) waitCalls() int {
	harness.command.mu.Lock()
	defer harness.command.mu.Unlock()
	return harness.command.waitCount
}

func (harness *darwinHarness) requireEveryCreatedEndClosedOnce(t *testing.T) {
	t.Helper()
	for index, end := range harness.ends {
		switch typed := end.(type) {
		case *memoryProcessEnd:
			typed.mu.Lock()
			count := typed.closeCount
			typed.mu.Unlock()
			if count != 1 {
				t.Fatalf("end %d close count = %d", index, count)
			}
		case *blockingProcessEnd:
			typed.mu.Lock()
			count := typed.closeCount
			typed.mu.Unlock()
			if count != 1 {
				t.Fatalf("end %d close count = %d", index, count)
			}
		}
	}
}

type fakeProcessCommand struct {
	mu         sync.Mutex
	pid        int
	startErr   error
	reap       processReapResult
	startCount int
	waitCount  int
	waitBlock  chan struct{}
	startGate  <-chan struct{}
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
}

func (command *fakeProcessCommand) configure(stdin io.Reader, stdout, stderr io.Writer) {
	command.stdin, command.stdout, command.stderr = stdin, stdout, stderr
}

func (command *fakeProcessCommand) start() (int, error) {
	if command.startGate != nil {
		<-command.startGate
	}
	command.startCount++
	return command.pid, command.startErr
}

func (command *fakeProcessCommand) wait() processReapResult {
	if command.waitBlock != nil {
		<-command.waitBlock
	}
	command.mu.Lock()
	defer command.mu.Unlock()
	command.waitCount++
	return command.reap
}

type fakeProcessScheduler struct {
	mu                      sync.Mutex
	current                 time.Time
	waits                   []time.Duration
	wakeEnabled             []bool
	cancelEnabled           []bool
	purposes                []processWaitPurpose
	requiredWake            int
	missingWake             bool
	waitForWorkerCompletion bool
}

func (scheduler *fakeProcessScheduler) now() time.Time {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return scheduler.current
}

func (scheduler *fakeProcessScheduler) wait(
	delay time.Duration,
	wake, canceled <-chan struct{},
	purpose processWaitPurpose,
) {
	scheduler.mu.Lock()
	scheduler.waits = append(scheduler.waits, delay)
	scheduler.wakeEnabled = append(scheduler.wakeEnabled, wake != nil)
	scheduler.cancelEnabled = append(scheduler.cancelEnabled, canceled != nil)
	scheduler.purposes = append(scheduler.purposes, purpose)
	waitForCompletion := purpose == processWaitCompletion && scheduler.waitForWorkerCompletion
	if !waitForCompletion {
		scheduler.current = scheduler.current.Add(delay)
	}
	requireWake := scheduler.requiredWake > 0
	if requireWake {
		scheduler.requiredWake--
		if wake == nil {
			scheduler.missingWake = true
		}
	}
	scheduler.mu.Unlock()
	if waitForCompletion && wake != nil {
		<-wake
		return
	}
	if requireWake && wake != nil {
		<-wake
	}
}

func (scheduler *fakeProcessScheduler) requireMaxWait(t *testing.T, maximum time.Duration) {
	t.Helper()
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	for index, delay := range scheduler.waits {
		if delay < 0 || delay > maximum {
			t.Fatalf("wait %d = %s, maximum %s", index, delay, maximum)
		}
	}
}

func (scheduler *fakeProcessScheduler) requireCompletionSafe(t *testing.T) {
	t.Helper()
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.missingWake {
		t.Fatal("completion scheduling requested a missing wake channel")
	}
}

func (scheduler *fakeProcessScheduler) requireCompletionConsumed(t *testing.T) {
	t.Helper()
	scheduler.requireCompletionSafe(t)
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.requiredWake != 0 {
		t.Fatalf("completion scheduling left %d required wakes", scheduler.requiredWake)
	}
}

type blockingProcessEnd struct {
	mu         sync.Mutex
	closed     chan struct{}
	closeOnce  sync.Once
	closeCount int
}

func newBlockingProcessEnd() *blockingProcessEnd {
	return &blockingProcessEnd{closed: make(chan struct{})}
}

func (end *blockingProcessEnd) Read([]byte) (int, error) {
	<-end.closed
	return 0, errors.New("closed")
}

func (end *blockingProcessEnd) Write([]byte) (int, error) {
	<-end.closed
	return 0, errors.New("closed")
}

func (end *blockingProcessEnd) Close() error {
	end.mu.Lock()
	end.closeCount++
	end.mu.Unlock()
	end.closeOnce.Do(func() { close(end.closed) })
	return nil
}

func requireProcessRecord(t *testing.T, record *FailureRecord, phase Phase, operation Operation, cause CauseCode) {
	t.Helper()
	if record == nil || record.Phase != phase || record.Operation != operation || len(record.Causes) != 1 || record.Causes[0] != cause {
		t.Fatalf("record = %+v, want %s/%s/%s", record, phase, operation, cause)
	}
}

func requireProcessCause(t *testing.T, causes []CauseCode, want CauseCode) {
	t.Helper()
	if !slices.Contains(causes, want) {
		t.Fatalf("causes = %v, missing %s", causes, want)
	}
}
