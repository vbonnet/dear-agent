package buildauthority

import (
	"context"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

type materializerTestScheduler struct {
	mu      sync.Mutex
	current time.Time
	calls   int
}

func (scheduler *materializerTestScheduler) now() time.Time {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	scheduler.calls++
	return scheduler.current
}

func (*materializerTestScheduler) wait(
	time.Duration,
	<-chan struct{},
	<-chan struct{},
	processWaitPurpose,
) {
}

func (scheduler *materializerTestScheduler) set(current time.Time) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	scheduler.current = current
}

func (scheduler *materializerTestScheduler) callCount() int {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return scheduler.calls
}

type recordedMaterializerCall struct {
	request         processRequest
	environment     preallocationEnvironment
	environmentRows []string
	resolved        resolvedProcessInput
	resolveErr      error
}

type recordingMaterializerSupervisor struct {
	calls              []recordedMaterializerCall
	observePreallocate func(*processRequest)
	taskPrivate        int
}

func (*recordingMaterializerSupervisor) privateProcessSupervisor() {}

func (supervisor *recordingMaterializerSupervisor) runPreallocation(
	request *processRequest,
	environment preallocationEnvironment,
) processResult {
	if request == nil {
		return invalidNonSourceProcessResult(nil)
	}
	if supervisor.observePreallocate != nil {
		supervisor.observePreallocate(request)
	}
	resolved, err := resolveProcessInput(request.ctx, request.input)
	supervisor.calls = append(supervisor.calls, recordedMaterializerCall{
		request:         *request,
		environment:     environment,
		environmentRows: environment.clone(),
		resolved:        resolved,
		resolveErr:      err,
	})
	return processResult{quiescence: quiescenceResult{proven: true}}
}

func (supervisor *recordingMaterializerSupervisor) runTaskPrivate(
	*processRequest,
	taskPrivateEnvironment,
) processResult {
	supervisor.taskPrivate++
	return invalidNonSourceProcessResult(nil)
}

type materializerTestContextKey struct{}

type materializerTestDeadlineContext struct {
	context.Context
	deadline time.Time
}

func (ctx *materializerTestDeadlineContext) Deadline() (time.Time, bool) {
	return ctx.deadline, true
}

type materializerTestCase struct {
	name       string
	kind       nonSourceCommandKind
	executable string
	arguments  []string
	run        func(nonSourceProcessOwner, *nonSourceCommandWindow) processResult
}

func TestNonSourceMaterializersConstructExactBracketedRequests(t *testing.T) {
	requireMaterializerTestPlatform(t)

	plans, nullDevice := materializerTestPlans(t)
	base := time.Unix(1_800_000_000, 123_000_000)
	scheduler := &materializerTestScheduler{current: base}
	issuer := newNonSourceWindowIssuer(scheduler)
	ctx := &materializerTestDeadlineContext{
		Context:  context.WithValue(context.Background(), materializerTestContextKey{}, "exact-context"),
		deadline: base.Add(30 * time.Minute),
	}
	transaction, failure := issuer.beginTransaction(ctx)
	if failure != nil {
		t.Fatalf("begin transaction: %+v", failure)
	}
	supervisor := new(recordingMaterializerSupervisor)
	owner := &preallocationNonSourceProcessOwner{supervisor: supervisor, issuer: issuer}
	if owner.issuer != issuer || owner.issuer.scheduler != scheduler {
		t.Fatal("process owner did not retain the exact window issuer and scheduler")
	}

	tests := materializerTestCases(plans)
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			phaseSample := base.Add(time.Duration(index+1) * time.Minute)
			scheduler.set(phaseSample)
			window := authorizedMaterializerWindow(
				t,
				issuer,
				transaction,
				test.kind,
				plans.goVersion.environment,
				nullDevice,
			)
			scheduler.set(phaseSample.Add(time.Second))
			before := len(supervisor.calls)
			result := test.run(owner, window)
			requireSuccessfulMaterializerResult(t, result)
			if len(supervisor.calls) != before+1 {
				t.Fatalf("preallocation calls = %d, want %d", len(supervisor.calls), before+1)
			}
			call := supervisor.calls[before]
			request := call.request
			if request.ctx != ctx {
				t.Fatal("request did not retain the exact caller context")
			}
			if request.phase != PhaseAuthority || request.executable != test.executable ||
				request.directory != physicalRootPath || request.stdoutLimit != processMaxStructuredOutputBytes {
				t.Fatalf(
					"request authority fields = phase=%q executable=%q cwd=%q limit=%d",
					request.phase,
					request.executable,
					request.directory,
					request.stdoutLimit,
				)
			}
			if !reflect.DeepEqual(request.arguments, test.arguments) {
				t.Fatalf("arguments = %q, want %q", request.arguments, test.arguments)
			}
			if !request.phaseDeadline.Equal(phaseSample.Add(nonSourceCommandDuration)) ||
				!request.callerDeadline.Equal(ctx.deadline) ||
				!request.transactionDeadline.Equal(base.Add(noScratchTransactionDuration)) {
				t.Fatalf(
					"deadlines = phase %s caller %s transaction %s",
					request.phaseDeadline,
					request.callerDeadline,
					request.transactionDeadline,
				)
			}
			if call.environment != plans.goVersion.environment || !call.environment.valid() ||
				len(call.environmentRows) != directEnvironmentRowCount ||
				!reflect.DeepEqual(call.environmentRows, plans.goVersion.environment.clone()) {
				t.Fatalf("materialized environment is not the exact D39 profile")
			}
			if call.resolveErr != nil {
				t.Fatalf("resolve bracketed input: %v", call.resolveErr)
			}
			if request.input == nil || request.input.kind != processInputBracketedRetainedNull ||
				request.input.retainedNull != nil || request.input.bounded != nil ||
				request.input.bracketedNull == nil {
				t.Fatalf("input = %+v, want only a bracketed retained-null witness", request.input)
			}
			if call.resolved.kind != processInputBracketedRetainedNull ||
				call.resolved.retainedNull.device != nullDevice ||
				call.resolved.retainedNull.descriptor != nullDevice.leaf.descriptor {
				t.Fatal("bracketed input did not retain the exact captured null descriptor")
			}
			if !window.consumed() {
				t.Fatal("successful materialization did not consume its one-shot window")
			}
		})
	}
	if supervisor.taskPrivate != 0 {
		t.Fatalf("task-private calls = %d, want zero", supervisor.taskPrivate)
	}
	if scheduler.callCount() != 1+2*len(tests) {
		t.Fatalf("shared scheduler samples = %d, want %d", scheduler.callCount(), 1+2*len(tests))
	}

	// The fixture descriptor has no live os.File backing. resolveProcessInput
	// above can therefore succeed only through the bracket-captured witness;
	// the legacy retained-null path would attempt an operating-system recheck.
}

func TestNonSourceMaterializersReturnFreshArgumentsAndEnvironmentRows(t *testing.T) {
	requireMaterializerTestPlatform(t)

	plans, nullDevice := materializerTestPlans(t)
	base := time.Unix(1_800_100_000, 0)
	scheduler := &materializerTestScheduler{current: base}
	issuer := newNonSourceWindowIssuer(scheduler)
	transaction, failure := issuer.beginTransaction(context.Background())
	if failure != nil {
		t.Fatalf("begin transaction: %+v", failure)
	}
	supervisor := new(recordingMaterializerSupervisor)
	owner := &preallocationNonSourceProcessOwner{supervisor: supervisor, issuer: issuer}

	for _, test := range materializerTestCases(plans) {
		t.Run(test.name, func(t *testing.T) {
			first := authorizedMaterializerWindow(
				t, issuer, transaction, test.kind, plans.goVersion.environment, nullDevice,
			)
			requireSuccessfulMaterializerResult(t, test.run(owner, first))
			firstCall := supervisor.calls[len(supervisor.calls)-1]
			firstCall.request.arguments[0] = "tampered"
			firstCall.environmentRows[0] = "TAMPERED=1"

			second := authorizedMaterializerWindow(
				t, issuer, transaction, test.kind, plans.goVersion.environment, nullDevice,
			)
			requireSuccessfulMaterializerResult(t, test.run(owner, second))
			secondCall := supervisor.calls[len(supervisor.calls)-1]
			if !reflect.DeepEqual(secondCall.request.arguments, test.arguments) {
				t.Fatalf("rematerialized arguments = %q, want %q", secondCall.request.arguments, test.arguments)
			}
			if !reflect.DeepEqual(secondCall.environmentRows, plans.goVersion.environment.clone()) {
				t.Fatal("rematerialized environment inherited caller mutation")
			}
		})
	}
}

func TestNonSourceMaterializerWindowsAreIssuerBoundAndOneShot(t *testing.T) {
	requireMaterializerTestPlatform(t)

	plans, nullDevice := materializerTestPlans(t)
	base := time.Unix(1_800_200_000, 0)
	scheduler := &materializerTestScheduler{current: base}
	issuer := newNonSourceWindowIssuer(scheduler)
	transaction, failure := issuer.beginTransaction(context.Background())
	if failure != nil {
		t.Fatalf("begin transaction: %+v", failure)
	}
	supervisor := new(recordingMaterializerSupervisor)
	owner := &preallocationNonSourceProcessOwner{supervisor: supervisor, issuer: issuer}

	assertRefusedWithoutCall := func(
		t *testing.T,
		before int,
		result processResult,
		operation Operation,
		cause CauseCode,
	) {
		t.Helper()
		requireAuthorityRecord(t, result.primary, operation, cause)
		if result.structuredOutput != nil || result.descriptorClose != nil ||
			!result.quiescence.proven || result.background != nil {
			t.Fatalf("invalid materializer result = %+v", result)
		}
		if len(supervisor.calls) != before {
			t.Fatalf("generic supervisor calls = %d, want %d", len(supervisor.calls), before)
		}
	}

	t.Run("nil window", func(t *testing.T) {
		before := len(supervisor.calls)
		assertRefusedWithoutCall(
			t, before, owner.runGoVersion(plans.goVersion, nil),
			OperationValidate, CauseInternalInvariant,
		)
	})

	t.Run("zero window", func(t *testing.T) {
		before := len(supervisor.calls)
		assertRefusedWithoutCall(
			t, before, owner.runGoVersion(plans.goVersion, new(nonSourceCommandWindow)),
			OperationValidate, CauseInternalInvariant,
		)
	})

	t.Run("wrong command", func(t *testing.T) {
		window := authorizedMaterializerWindow(
			t, issuer, transaction, nonSourceCommandGitVersion,
			plans.goVersion.environment, nullDevice,
		)
		before := len(supervisor.calls)
		assertRefusedWithoutCall(
			t, before, owner.runGoVersion(plans.goVersion, window),
			OperationValidate, CauseInternalInvariant,
		)
	})

	t.Run("foreign issuer", func(t *testing.T) {
		// Sharing a scheduler is insufficient: the window must belong to the
		// exact issuer retained by the process owner.
		foreignIssuer := newNonSourceWindowIssuer(scheduler)
		foreignTransaction, foreignFailure := foreignIssuer.beginTransaction(context.Background())
		if foreignFailure != nil {
			t.Fatalf("begin foreign transaction: %+v", foreignFailure)
		}
		window := authorizedMaterializerWindow(
			t, foreignIssuer, foreignTransaction, nonSourceCommandGoVersion,
			plans.goVersion.environment, nullDevice,
		)
		before := len(supervisor.calls)
		assertRefusedWithoutCall(
			t, before, owner.runGoVersion(plans.goVersion, window),
			OperationValidate, CauseInternalInvariant,
		)
	})

	t.Run("captured descriptor replaced", func(t *testing.T) {
		window := authorizedMaterializerWindow(
			t, issuer, transaction, nonSourceCommandGoVersion,
			plans.goVersion.environment, nullDevice,
		)
		retainedDescriptor := nullDevice.leaf.descriptor
		nullDevice.leaf.descriptor = plans.goVersion.executable.retained.leaf.descriptor
		t.Cleanup(func() { nullDevice.leaf.descriptor = retainedDescriptor })
		before := len(supervisor.calls)
		assertRefusedWithoutCall(
			t, before, owner.runGoVersion(plans.goVersion, window),
			OperationValidate, CauseInternalInvariant,
		)
		nullDevice.leaf.descriptor = retainedDescriptor
	})

	t.Run("replay", func(t *testing.T) {
		window := authorizedMaterializerWindow(
			t, issuer, transaction, nonSourceCommandGoVersion,
			plans.goVersion.environment, nullDevice,
		)
		requireSuccessfulMaterializerResult(t, owner.runGoVersion(plans.goVersion, window))
		before := len(supervisor.calls)
		assertRefusedWithoutCall(
			t, before, owner.runGoVersion(plans.goVersion, window),
			OperationValidate, CauseInternalInvariant,
		)
	})
}

func TestNonSourceMaterializersRejectInvalidPlansBeforeSupervisorCall(t *testing.T) {
	requireMaterializerTestPlatform(t)

	plans, nullDevice := materializerTestPlans(t)
	base := time.Unix(1_800_300_000, 0)
	scheduler := &materializerTestScheduler{current: base}
	issuer := newNonSourceWindowIssuer(scheduler)
	transaction, failure := issuer.beginTransaction(context.Background())
	if failure != nil {
		t.Fatalf("begin transaction: %+v", failure)
	}
	supervisor := new(recordingMaterializerSupervisor)
	owner := &preallocationNonSourceProcessOwner{supervisor: supervisor, issuer: issuer}

	tests := []struct {
		name string
		kind nonSourceCommandKind
		run  func(*nonSourceCommandWindow) processResult
	}{
		{name: "go version", kind: nonSourceCommandGoVersion, run: func(window *nonSourceCommandWindow) processResult {
			return owner.runGoVersion(goVersionPlan{}, window)
		}},
		{name: "go environment", kind: nonSourceCommandGoEnvironment, run: func(window *nonSourceCommandWindow) processResult {
			return owner.runGoEnvironment(goEnvironmentPlan{}, window)
		}},
		{name: "compiler version", kind: nonSourceCommandCompilerVersion, run: func(window *nonSourceCommandWindow) processResult {
			return owner.runCompilerVersion(compilerVersionPlan{}, window)
		}},
		{name: "git version", kind: nonSourceCommandGitVersion, run: func(window *nonSourceCommandWindow) processResult {
			return owner.runGitVersion(gitVersionPlan{}, window)
		}},
		{name: "git builtin inventory", kind: nonSourceCommandGitBuiltinInventory, run: func(window *nonSourceCommandWindow) processResult {
			return owner.runGitBuiltinInventory(gitBuiltinInventoryPlan{}, window)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			window := authorizedMaterializerWindow(
				t, issuer, transaction, test.kind, plans.goVersion.environment, nullDevice,
			)
			before := len(supervisor.calls)
			result := test.run(window)
			requireAuthorityRecord(t, result.primary, OperationValidate, CauseInternalInvariant)
			if len(supervisor.calls) != before {
				t.Fatalf("generic supervisor calls = %d, want %d", len(supervisor.calls), before)
			}
			if window.consumed() {
				t.Fatal("invalid plan consumed its command window")
			}
		})
	}
}

func TestNonSourceMaterializerRetainsProcessObservedCallerDeadline(t *testing.T) {
	requireMaterializerTestPlatform(t)

	plans, nullDevice := materializerTestPlans(t)
	for _, test := range []struct {
		name   string
		mutate func(*runnerTestMutableDeadlineContext, time.Time)
	}{
		{
			name: "then extended",
			mutate: func(ctx *runnerTestMutableDeadlineContext, base time.Time) {
				ctx.deadline = base.Add(3 * time.Hour)
			},
		},
		{
			name: "then removed",
			mutate: func(ctx *runnerTestMutableDeadlineContext, _ time.Time) {
				ctx.deadline = time.Time{}
				ctx.hasDeadline = false
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := time.Unix(1_800_350_000, 0)
			shortened := base.Add(time.Second)
			caller := &runnerTestMutableDeadlineContext{
				Context:     context.Background(),
				deadline:    base.Add(2 * time.Hour),
				hasDeadline: true,
			}
			scheduler := &materializerTestScheduler{current: base}
			issuer := newNonSourceWindowIssuer(scheduler)
			transaction, failure := issuer.beginTransaction(caller)
			if failure != nil {
				t.Fatalf("begin transaction: %+v", failure)
			}
			window := authorizedMaterializerWindow(
				t,
				issuer,
				transaction,
				nonSourceCommandGoVersion,
				plans.goVersion.environment,
				nullDevice,
			)
			supervisor := &recordingMaterializerSupervisor{
				observePreallocate: func(request *processRequest) {
					caller.deadline = shortened
					if next := nextProcessDeadline(request, base); next != shortened {
						t.Fatalf("process-observed deadline = %s, want %s", next, shortened)
					}
					test.mutate(caller, base)
					if next := nextProcessDeadline(request, base); next != shortened {
						t.Fatalf("retained process deadline = %s, want %s", next, shortened)
					}
				},
			}
			owner := &preallocationNonSourceProcessOwner{supervisor: supervisor, issuer: issuer}
			requireSuccessfulMaterializerResult(
				t,
				owner.runGoVersion(plans.goVersion, window),
			)
			if len(supervisor.calls) != 1 ||
				supervisor.calls[0].request.callerDeadline != shortened {
				t.Fatalf("recorded request = %+v, want retained %s", supervisor.calls, shortened)
			}
			if deadline, ok := transaction.state.callerDeadlineFloor.snapshot(); !ok || deadline != shortened {
				t.Fatalf("transaction deadline floor = %s / %t, want %s", deadline, ok, shortened)
			}
			if deadline, ok := transaction.state.authorityContext.Deadline(); !ok || deadline != shortened {
				t.Fatalf("post-process authority deadline = %s / %t, want %s", deadline, ok, shortened)
			}
		})
	}
}

func TestNonSourceMaterializerDeadlineAndCancellationRefuseBeforeSupervisor(t *testing.T) {
	requireMaterializerTestPlatform(t)

	plans, nullDevice := materializerTestPlans(t)
	base := time.Unix(1_800_400_000, 0)

	t.Run("deadline", func(t *testing.T) {
		scheduler := &materializerTestScheduler{current: base}
		issuer := newNonSourceWindowIssuer(scheduler)
		transaction, failure := issuer.beginTransaction(context.Background())
		if failure != nil {
			t.Fatalf("begin transaction: %+v", failure)
		}
		window := authorizedMaterializerWindow(
			t, issuer, transaction, nonSourceCommandGoVersion,
			plans.goVersion.environment, nullDevice,
		)
		scheduler.set(base.Add(nonSourceCommandDuration))
		supervisor := new(recordingMaterializerSupervisor)
		owner := &preallocationNonSourceProcessOwner{supervisor: supervisor, issuer: issuer}
		result := owner.runGoVersion(plans.goVersion, window)
		requireAuthorityRecord(t, result.primary, OperationExecute, CauseDeadline)
		if len(supervisor.calls) != 0 {
			t.Fatalf("generic supervisor calls = %d, want zero", len(supervisor.calls))
		}
	})

	t.Run("canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		scheduler := &materializerTestScheduler{current: base}
		issuer := newNonSourceWindowIssuer(scheduler)
		transaction, failure := issuer.beginTransaction(ctx)
		if failure != nil {
			t.Fatalf("begin transaction: %+v", failure)
		}
		window := authorizedMaterializerWindow(
			t, issuer, transaction, nonSourceCommandGoVersion,
			plans.goVersion.environment, nullDevice,
		)
		cancel()
		supervisor := new(recordingMaterializerSupervisor)
		owner := &preallocationNonSourceProcessOwner{supervisor: supervisor, issuer: issuer}
		result := owner.runGoVersion(plans.goVersion, window)
		requireAuthorityRecord(t, result.primary, OperationExecute, CauseCanceled)
		if len(supervisor.calls) != 0 {
			t.Fatalf("generic supervisor calls = %d, want zero", len(supervisor.calls))
		}
	})
}

func materializerTestPlans(
	t *testing.T,
) (nonSourcePlans, *retainedNullDevice) {
	t.Helper()
	environment, claim := testPreallocationPlanInputs(t)
	nullDevice := newAuthorityBracketFixture(t).nullDevice
	environment.authorities.nullDevice = nullDevice
	if !environment.valid() {
		t.Fatal("materializer test environment is invalid")
	}
	plans, err := newNonSourcePlans(environment, claim)
	if err != nil {
		t.Fatalf("construct materializer plans: %v", err)
	}
	return plans, nullDevice
}

func materializerTestCases(plans nonSourcePlans) []materializerTestCase {
	return []materializerTestCase{
		{
			name:       "go version",
			kind:       nonSourceCommandGoVersion,
			executable: plans.goVersion.executable.retained.leaf.path,
			arguments:  []string{"version"},
			run: func(owner nonSourceProcessOwner, window *nonSourceCommandWindow) processResult {
				return owner.runGoVersion(plans.goVersion, window)
			},
		},
		{
			name:       "go environment",
			kind:       nonSourceCommandGoEnvironment,
			executable: plans.goEnvironment.executable.retained.leaf.path,
			arguments:  goEnvironmentArguments(),
			run: func(owner nonSourceProcessOwner, window *nonSourceCommandWindow) processResult {
				return owner.runGoEnvironment(plans.goEnvironment, window)
			},
		},
		{
			name:       "compiler version",
			kind:       nonSourceCommandCompilerVersion,
			executable: plans.compilerVersion.executable.retained.leaf.path,
			arguments:  []string{"-V=full"},
			run: func(owner nonSourceProcessOwner, window *nonSourceCommandWindow) processResult {
				return owner.runCompilerVersion(plans.compilerVersion, window)
			},
		},
		{
			name:       "git version",
			kind:       nonSourceCommandGitVersion,
			executable: plans.gitVersion.executable.retained.leaf.path,
			arguments:  []string{"version", "--build-options"},
			run: func(owner nonSourceProcessOwner, window *nonSourceCommandWindow) processResult {
				return owner.runGitVersion(plans.gitVersion, window)
			},
		},
		{
			name:       "git builtin inventory",
			kind:       nonSourceCommandGitBuiltinInventory,
			executable: plans.gitBuiltins.executable.retained.leaf.path,
			arguments: []string{
				"--git-dir=/dev/null",
				"--list-cmds=builtins",
			},
			run: func(owner nonSourceProcessOwner, window *nonSourceCommandWindow) processResult {
				return owner.runGitBuiltinInventory(plans.gitBuiltins, window)
			},
		},
	}
}

func authorizedMaterializerWindow(
	t *testing.T,
	issuer *nonSourceWindowIssuer,
	transaction nonSourceTransactionWindow,
	kind nonSourceCommandKind,
	environment preallocationEnvironment,
	nullDevice *retainedNullDevice,
) *nonSourceCommandWindow {
	t.Helper()
	window, failure := issuer.mintCommand(transaction, kind, environment)
	if failure != nil {
		t.Fatalf("mint %d window: %+v", kind, failure)
	}
	if failure := window.admitRetainedNull(nullDevice); failure != nil {
		t.Fatalf("admit retained null for %d: %+v", kind, failure)
	}
	if failure := window.authorize(); failure != nil {
		t.Fatalf("authorize %d window: %+v", kind, failure)
	}
	return window
}

func requireSuccessfulMaterializerResult(t *testing.T, result processResult) {
	t.Helper()
	if result.primary != nil || result.descriptorClose != nil ||
		!result.quiescence.proven || len(result.quiescence.causes) != 0 ||
		result.quiescence.child != nil || result.background != nil {
		t.Fatalf("materializer result = %+v, want proved process result", result)
	}
}

func requireMaterializerTestPlatform(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("retained-null command witnesses are admitted only on darwin/arm64")
	}
}
