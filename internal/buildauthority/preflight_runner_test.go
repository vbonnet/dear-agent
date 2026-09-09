package buildauthority

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"testing"
	"time"
)

type runnerTestDeadlineContext struct {
	context.Context
	deadline time.Time
}

func (ctx *runnerTestDeadlineContext) Deadline() (time.Time, bool) {
	return ctx.deadline, true
}

type runnerTestMutableDeadlineContext struct {
	context.Context
	deadline      time.Time
	hasDeadline   bool
	deadlineCalls int
	errCalls      int
}

func (ctx *runnerTestMutableDeadlineContext) Deadline() (time.Time, bool) {
	ctx.deadlineCalls++
	return ctx.deadline, ctx.hasDeadline
}

func (ctx *runnerTestMutableDeadlineContext) Err() error {
	ctx.errCalls++
	return ctx.Context.Err()
}

type runnerTestErrorContext struct {
	context.Context
	err      error
	errCalls int
}

func (ctx *runnerTestErrorContext) Err() error {
	ctx.errCalls++
	return ctx.err
}

type runnerTestTrace struct {
	calls  map[string]int
	events []string
}

func newRunnerTestTrace() *runnerTestTrace {
	return &runnerTestTrace{calls: make(map[string]int)}
}

func (trace *runnerTestTrace) step(base string) string {
	trace.calls[base]++
	event := fmt.Sprintf("%s#%d", base, trace.calls[base])
	trace.events = append(trace.events, event)
	return event
}

type runnerTestScheduler struct {
	base      time.Time
	overrides map[int]time.Time
	hooks     map[int]func()
	samples   []time.Time
	waits     int
}

func (scheduler *runnerTestScheduler) now() time.Time {
	index := len(scheduler.samples)
	sample := scheduler.base.Add(time.Duration(index) * time.Second)
	if override, ok := scheduler.overrides[index]; ok {
		sample = override
	} else if index != 0 {
		monotonicNext := scheduler.samples[index-1].Add(time.Second)
		if monotonicNext.After(sample) {
			sample = monotonicNext
		}
	}
	scheduler.samples = append(scheduler.samples, sample)
	if hook := scheduler.hooks[index]; hook != nil {
		hook()
	}
	return sample
}

func (scheduler *runnerTestScheduler) wait(
	time.Duration,
	<-chan struct{},
	<-chan struct{},
	processWaitPurpose,
) {
	scheduler.waits++
}

type runnerTestAuthorityOwner struct {
	t        *testing.T
	trace    *runnerTestTrace
	ctx      context.Context
	plans    nonSourcePlans
	outcomes map[string]authorityUseOutcome
	contexts map[*nonSourceAuthorityContext]int
}

func (*runnerTestAuthorityOwner) privateNonSourceAuthorityOwner() {}

func (owner *runnerTestAuthorityOwner) next(base string) authorityUseOutcome {
	return owner.outcomes[owner.trace.step(base)]
}

func (owner *runnerTestAuthorityOwner) requireContext(ctx context.Context) {
	owner.t.Helper()
	authorityContext, ok := ctx.(*nonSourceAuthorityContext)
	if !ok || authorityContext.transaction == nil ||
		authorityContext.transaction.ctx != owner.ctx {
		owner.t.Fatal("runner passed foreign authority context")
	}
	owner.contexts[authorityContext]++
}

func (owner *runnerTestAuthorityOwner) requireRoot(
	ctx context.Context,
	root *physicalRootAuthority,
) {
	owner.t.Helper()
	environment := owner.plans.goVersion.environment
	owner.requireContext(ctx)
	if root != environment.authorities.physicalRoot {
		owner.t.Fatalf("runner passed foreign root authority or context")
	}
}

func (owner *runnerTestAuthorityOwner) admitPhysicalRootSentinels(
	ctx context.Context,
	root *physicalRootAuthority,
) (physicalRootSentinelClaim, authorityUseOutcome) {
	owner.requireRoot(ctx, root)
	outcome := owner.next("authority:sentinels-admit")
	if !outcome.proved() {
		return physicalRootSentinelClaim{}, outcome
	}
	return physicalRootSentinelClaim{
		root: root,
		seal: validPhysicalRootSentinelClaim,
	}, outcome
}

func (owner *runnerTestAuthorityOwner) revalidatePhysicalRoot(
	ctx context.Context,
	root *physicalRootAuthority,
) authorityUseOutcome {
	owner.requireRoot(ctx, root)
	return owner.next("authority:root")
}

func (owner *runnerTestAuthorityOwner) revalidateRetainedNull(
	ctx context.Context,
	root *physicalRootAuthority,
	nullDevice *retainedNullDevice,
) authorityUseOutcome {
	owner.requireRoot(ctx, root)
	if nullDevice != owner.plans.goVersion.environment.authorities.nullDevice {
		owner.t.Fatal("runner passed foreign retained-null authority")
	}
	return owner.next("authority:null")
}

func (owner *runnerTestAuthorityOwner) revalidateGOROOT(
	ctx context.Context,
	root *physicalRootAuthority,
	goroot *treeCapture,
) authorityUseOutcome {
	owner.requireRoot(ctx, root)
	if goroot != owner.plans.goVersion.environment.authorities.goroot {
		owner.t.Fatal("runner passed foreign GOROOT authority")
	}
	return owner.next("authority:goroot")
}

func (owner *runnerTestAuthorityOwner) revalidateGoEnvironmentPlan(
	ctx context.Context,
	plan goEnvironmentPlan,
) authorityUseOutcome {
	owner.requireContext(ctx)
	if plan != owner.plans.goEnvironment {
		owner.t.Fatal("runner passed foreign Go environment plan or context")
	}
	return owner.next("authority:go-environment")
}

func (owner *runnerTestAuthorityOwner) revalidateGoAuthority(
	ctx context.Context,
	root *physicalRootAuthority,
	goroot *treeCapture,
	goAuthority *goAuthority,
) authorityUseOutcome {
	owner.requireRoot(ctx, root)
	environment := owner.plans.goVersion.environment
	if goroot != environment.authorities.goroot ||
		goAuthority != environment.authorities.goExecutable {
		owner.t.Fatal("runner passed foreign Go executable authority")
	}
	return owner.next("authority:go")
}

func (owner *runnerTestAuthorityOwner) revalidateCompilerAuthority(
	ctx context.Context,
	root *physicalRootAuthority,
	goroot *treeCapture,
	compiler *compilerAuthority,
) authorityUseOutcome {
	owner.requireRoot(ctx, root)
	environment := owner.plans.goVersion.environment
	if goroot != environment.authorities.goroot ||
		compiler != environment.authorities.compiler {
		owner.t.Fatal("runner passed foreign compiler authority")
	}
	return owner.next("authority:compiler")
}

func (owner *runnerTestAuthorityOwner) revalidateGitAuthority(
	ctx context.Context,
	root *physicalRootAuthority,
	git *gitAuthority,
) authorityUseOutcome {
	owner.requireRoot(ctx, root)
	if git != owner.plans.goVersion.environment.authorities.gitExecutable {
		owner.t.Fatal("runner passed foreign Git authority")
	}
	return owner.next("authority:git")
}

func (owner *runnerTestAuthorityOwner) revalidatePhysicalRootSentinels(
	ctx context.Context,
	claim physicalRootSentinelClaim,
) authorityUseOutcome {
	environment := owner.plans.goVersion.environment
	owner.requireContext(ctx)
	if claim.root != environment.authorities.physicalRoot ||
		claim.seal != validPhysicalRootSentinelClaim {
		owner.t.Fatal("runner passed foreign physical-root sentinel claim")
	}
	return owner.next("authority:sentinels-revalidate")
}

type runnerTestToken struct {
	kind                nonSourceCommandKind
	window              *nonSourceCommandWindow
	phaseSample         time.Time
	phaseDeadline       time.Time
	callerDeadline      time.Time
	transactionDeadline time.Time
}

type runnerTestProcessOwner struct {
	t           *testing.T
	trace       *runnerTestTrace
	issuer      *nonSourceWindowIssuer
	plans       nonSourcePlans
	outputChild *ChildDiagnostic
	results     map[string]processResult
	tokens      []runnerTestToken
}

func (*runnerTestProcessOwner) privateNonSourceProcessOwner() {}

func (owner *runnerTestProcessOwner) run(
	base string,
	kind nonSourceCommandKind,
	environment preallocationEnvironment,
	window *nonSourceCommandWindow,
	output []byte,
) processResult {
	owner.t.Helper()
	event := owner.trace.step(base)
	if window == nil || window.state != nonSourceCommandWindowAuthorized {
		owner.t.Fatal("process owner received an unauthorized command window")
	}
	authority, failure := window.consume(kind, environment, owner.issuer)
	if failure != nil {
		owner.t.Fatalf("consume command window: %+v", failure)
	}
	if authority.seal != validNonSourceCommandAuthority ||
		authority.ctx != window.transaction.ctx ||
		authority.phaseDeadline != window.phaseDeadline ||
		authority.callerDeadline != window.callerDeadline ||
		authority.transactionDeadline != window.transactionDeadline ||
		authority.nullWitness == nil || !authority.nullWitness.validForProcessInput() {
		owner.t.Fatal("consumed command authority was not bound to its window")
	}
	borrow, ok := authority.nullWitness.borrowProcessInput()
	if !ok || borrow.descriptor != environment.authorities.nullDevice.leaf.descriptor {
		owner.t.Fatal("command authority did not retain the admitted null descriptor")
	}
	owner.tokens = append(owner.tokens, runnerTestToken{
		kind:                kind,
		window:              window,
		phaseSample:         window.phaseSample,
		phaseDeadline:       window.phaseDeadline,
		callerDeadline:      window.callerDeadline,
		transactionDeadline: window.transactionDeadline,
	})
	if result, ok := owner.results[event]; ok {
		return result
	}
	return processResult{
		structuredOutput:      append([]byte(nil), output...),
		structuredOutputChild: owner.outputChild,
		quiescence:            quiescenceResult{proven: true},
	}
}

func (owner *runnerTestProcessOwner) runGoVersion(
	plan goVersionPlan,
	window *nonSourceCommandWindow,
) processResult {
	if plan != owner.plans.goVersion {
		owner.t.Fatal("process owner received a foreign Go version plan")
	}
	return owner.run(
		"process:go-version",
		nonSourceCommandGoVersion,
		plan.environment,
		window,
		[]byte(goVersionWire),
	)
}

func (owner *runnerTestProcessOwner) runGoEnvironment(
	plan goEnvironmentPlan,
	window *nonSourceCommandWindow,
) processResult {
	if plan != owner.plans.goEnvironment {
		owner.t.Fatal("process owner received a foreign Go environment plan")
	}
	wire, _, err := expectedGoEnvironmentOutput(plan.environment)
	if err != nil {
		owner.t.Fatalf("render Go environment output: %v", err)
	}
	return owner.run(
		"process:go-environment",
		nonSourceCommandGoEnvironment,
		plan.environment,
		window,
		wire,
	)
}

func (owner *runnerTestProcessOwner) runCompilerVersion(
	plan compilerVersionPlan,
	window *nonSourceCommandWindow,
) processResult {
	if plan != owner.plans.compilerVersion {
		owner.t.Fatal("process owner received a foreign compiler version plan")
	}
	return owner.run(
		"process:compiler-version",
		nonSourceCommandCompilerVersion,
		plan.environment,
		window,
		[]byte(compilerVersionWire),
	)
}

func (owner *runnerTestProcessOwner) runGitVersion(
	plan gitVersionPlan,
	window *nonSourceCommandWindow,
) processResult {
	if plan != owner.plans.gitVersion {
		owner.t.Fatal("process owner received a foreign Git version plan")
	}
	return owner.run(
		"process:git-version",
		nonSourceCommandGitVersion,
		plan.environment,
		window,
		[]byte(gitVersionWire),
	)
}

func (owner *runnerTestProcessOwner) runGitBuiltinInventory(
	plan gitBuiltinInventoryPlan,
	window *nonSourceCommandWindow,
) processResult {
	if plan != owner.plans.gitBuiltins {
		owner.t.Fatal("process owner received a foreign Git builtin plan")
	}
	return owner.run(
		"process:git-builtins",
		nonSourceCommandGitBuiltinInventory,
		plan.environment,
		window,
		pinnedGitBuiltinOutput(),
	)
}

type runnerTestFixture struct {
	ctx       context.Context
	plans     nonSourcePlans
	trace     *runnerTestTrace
	scheduler *runnerTestScheduler
	issuer    *nonSourceWindowIssuer
	authority *runnerTestAuthorityOwner
	processes *runnerTestProcessOwner
	block     *nonSourceBlock
	window    nonSourceTransactionWindow
}

func newRunnerTestFixture(t *testing.T) *runnerTestFixture {
	t.Helper()
	return newRunnerTestFixtureWithContext(t, context.Background())
}

func newRunnerTestFixtureWithContext(
	t *testing.T,
	ctx context.Context,
) *runnerTestFixture {
	t.Helper()
	fixture := newRunnerTestFixtureWithoutTransaction(t, ctx)
	window, failure := fixture.block.beginTransaction(ctx)
	if failure != nil {
		t.Fatalf("begin non-source transaction: %+v", failure)
	}
	fixture.window = window
	return fixture
}

func newRunnerTestFixtureWithoutTransaction(
	t *testing.T,
	ctx context.Context,
) *runnerTestFixture {
	t.Helper()
	environment, claim := testPreallocationPlanInputs(t)
	environment.authorities.nullDevice = newAuthorityBracketFixture(t).nullDevice
	plans, err := newNonSourcePlans(environment, claim)
	if err != nil {
		t.Fatalf("construct runner plans: %v", err)
	}
	trace := newRunnerTestTrace()
	scheduler := &runnerTestScheduler{
		base:      time.Unix(1_700_000_000, 0).UTC(),
		overrides: make(map[int]time.Time),
		hooks:     make(map[int]func()),
	}
	issuer := newNonSourceWindowIssuer(scheduler)
	authority := &runnerTestAuthorityOwner{
		t:        t,
		trace:    trace,
		ctx:      ctx,
		plans:    plans,
		outcomes: make(map[string]authorityUseOutcome),
		contexts: make(map[*nonSourceAuthorityContext]int),
	}
	processes := &runnerTestProcessOwner{
		t:           t,
		trace:       trace,
		issuer:      issuer,
		plans:       plans,
		outputChild: runnerStructuredOutputChild(),
		results:     make(map[string]processResult),
	}
	block, failure := newNonSourceBlockWith(issuer, authority, processes)
	if failure != nil {
		t.Fatalf("construct non-source block: %+v", failure)
	}
	return &runnerTestFixture{
		ctx:       ctx,
		plans:     plans,
		trace:     trace,
		scheduler: scheduler,
		issuer:    issuer,
		authority: authority,
		processes: processes,
		block:     block,
	}
}

func (fixture *runnerTestFixture) run() nonSourceBlockResult {
	return fixture.block.run(fixture.window, fixture.plans)
}

func (fixture *runnerTestFixture) requireSamples(t *testing.T, count int) {
	t.Helper()
	if len(fixture.scheduler.samples) != count {
		t.Fatalf("clock samples = %d, want %d", len(fixture.scheduler.samples), count)
	}
	for index, sample := range fixture.scheduler.samples {
		want := fixture.scheduler.base.Add(time.Duration(index) * time.Second)
		if sample != want {
			t.Fatalf("clock sample %d = %s, want %s", index, sample, want)
		}
	}
	if fixture.scheduler.waits != 0 {
		t.Fatalf("runner invoked scheduler wait %d times", fixture.scheduler.waits)
	}
}

func (fixture *runnerTestFixture) requireMonotonicSamples(t *testing.T, count int) {
	t.Helper()
	if len(fixture.scheduler.samples) != count {
		t.Fatalf("clock samples = %d, want %d", len(fixture.scheduler.samples), count)
	}
	for index := 1; index < len(fixture.scheduler.samples); index++ {
		if !fixture.scheduler.samples[index].After(fixture.scheduler.samples[index-1]) {
			t.Fatalf(
				"clock sample %d = %s, not after %s",
				index,
				fixture.scheduler.samples[index],
				fixture.scheduler.samples[index-1],
			)
		}
	}
	if fixture.scheduler.waits != 0 {
		t.Fatalf("runner invoked scheduler wait %d times", fixture.scheduler.waits)
	}
}

func TestNewNonSourceBlockMintsTransactionBeforePlatformOwners(t *testing.T) {
	source, err := os.ReadFile("preflight_runner.go")
	if err != nil {
		t.Fatalf("read preflight runner: %v", err)
	}
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, "preflight_runner.go", source, 0)
	if err != nil {
		t.Fatalf("parse preflight runner: %v", err)
	}

	var constructor *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == "newNonSourceBlock" {
			constructor = function
			break
		}
	}
	if constructor == nil || constructor.Body == nil {
		t.Fatal("newNonSourceBlock declaration is missing")
	}
	parameters := constructor.Type.Params.List
	if len(parameters) != 1 || len(parameters[0].Names) != 1 ||
		parameters[0].Names[0].Name != "ctx" {
		t.Fatalf("newNonSourceBlock parameters = %#v, want sole ctx", parameters)
	}
	contextType, ok := parameters[0].Type.(*ast.SelectorExpr)
	if !ok {
		t.Fatalf("newNonSourceBlock ctx type = %#v, want context.Context", parameters[0].Type)
	}
	contextPackage, packageOK := contextType.X.(*ast.Ident)
	if !packageOK || contextPackage.Name != "context" ||
		contextType.Sel.Name != "Context" {
		t.Fatalf("newNonSourceBlock ctx type = %#v, want context.Context", parameters[0].Type)
	}

	targets := []string{
		"newNonSourceWindowIssuer",
		"issuer.beginTransaction",
		"newPreflightAuthorityRevalidator",
		"newNonSourceProcessOwner",
		"newNonSourceBlockWith",
	}
	wantBindings := map[string][]string{
		"newNonSourceWindowIssuer":         {"issuer"},
		"issuer.beginTransaction":          {"window", "failure"},
		"newPreflightAuthorityRevalidator": {"authority", "failure"},
		"newNonSourceProcessOwner":         {"processes", "failure"},
		"newNonSourceBlockWith":            {"block", "failure"},
	}
	positions := make(map[string]token.Pos, len(targets))
	counts := make(map[string]int, len(targets))
	ast.Inspect(constructor.Body, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Rhs) != 1 {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch callee := call.Fun.(type) {
		case *ast.Ident:
			name = callee.Name
		case *ast.SelectorExpr:
			receiver, receiverOK := callee.X.(*ast.Ident)
			if receiverOK {
				name = receiver.Name + "." + callee.Sel.Name
			}
		}
		want, tracked := wantBindings[name]
		if !tracked {
			return true
		}
		counts[name]++
		positions[name] = call.Pos()
		got := make([]string, 0, len(assignment.Lhs))
		for _, expression := range assignment.Lhs {
			identifier, identifierOK := expression.(*ast.Ident)
			if !identifierOK {
				t.Fatalf("%s assignment has non-identifier binding", files.Position(call.Pos()))
			}
			got = append(got, identifier.Name)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("%s binds %v, want %v", files.Position(call.Pos()), got, want)
		}
		if name == "issuer.beginTransaction" {
			argument, argumentOK := firstIdentifierArgument(call)
			if !argumentOK || argument != "ctx" {
				t.Fatalf("issuer.beginTransaction arguments do not forward ctx")
			}
		}
		if name == "newNonSourceProcessOwner" {
			argument, argumentOK := firstIdentifierArgument(call)
			if !argumentOK || argument != "issuer" {
				t.Fatalf("newNonSourceProcessOwner arguments do not forward issuer")
			}
		}
		return true
	})

	previous := token.NoPos
	for _, name := range targets {
		if counts[name] != 1 {
			t.Fatalf("newNonSourceBlock %s assignment count = %d, want 1", name, counts[name])
		}
		if previous.IsValid() && positions[name] <= previous {
			t.Fatalf("newNonSourceBlock constructs %s out of order", name)
		}
		previous = positions[name]
	}
	statements := constructor.Body.List
	if len(statements) == 0 {
		t.Fatal("newNonSourceBlock body is empty")
	}
	last, ok := statements[len(statements)-1].(*ast.ReturnStmt)
	if !ok || len(last.Results) != 3 {
		t.Fatalf("newNonSourceBlock final statement = %#v, want block/window return", statements[len(statements)-1])
	}
	wantReturn := []string{"block", "window", "nil"}
	gotReturn := make([]string, 0, len(last.Results))
	for _, expression := range last.Results {
		identifier, identifierOK := expression.(*ast.Ident)
		if !identifierOK {
			t.Fatalf("newNonSourceBlock final return contains non-identifier %#v", expression)
		}
		gotReturn = append(gotReturn, identifier.Name)
	}
	if !slices.Equal(gotReturn, wantReturn) {
		t.Fatalf("newNonSourceBlock final return = %v, want %v", gotReturn, wantReturn)
	}
}

func firstIdentifierArgument(call *ast.CallExpr) (string, bool) {
	if call == nil || len(call.Args) != 1 {
		return "", false
	}
	identifier, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return "", false
	}
	return identifier.Name, true
}

func TestNonSourceBlockIssuesOnlyOneTransaction(t *testing.T) {
	fixture := newRunnerTestFixture(t)
	original := fixture.window.state

	second, failure := fixture.block.beginTransaction(fixture.ctx)
	if second.state != nil || second.seal != 0 {
		t.Fatalf("second transaction = %+v, want zero window", second)
	}
	requireRunnerTestRecord(
		t,
		failure,
		PhaseAuthority,
		OperationValidate,
		CauseInternalInvariant,
	)
	if fixture.issuer.transaction != original || !fixture.window.validFor(fixture.issuer) {
		t.Fatal("rejected second issuance disturbed the original transaction")
	}
	fixture.requireSamples(t, 1)
}

func TestNonSourceBlockRejectsForgedTransactionWindows(t *testing.T) {
	for _, test := range []struct {
		name  string
		forge func(*runnerTestFixture) nonSourceTransactionWindow
	}{
		{
			name: "outer seal",
			forge: func(fixture *runnerTestFixture) nonSourceTransactionWindow {
				return nonSourceTransactionWindow{state: fixture.window.state}
			},
		},
		{
			name: "unissued cloned state",
			forge: func(fixture *runnerTestFixture) nonSourceTransactionWindow {
				cloned := *fixture.window.state
				cloned.authorityContext = newNonSourceAuthorityContext(
					&cloned,
					time.Time{},
					cloned.callerDeadline,
				)
				return nonSourceTransactionWindow{
					state: &cloned,
					seal:  validNonSourceTransactionWindow,
				}
			},
		},
		{
			name: "foreign state issuer",
			forge: func(fixture *runnerTestFixture) nonSourceTransactionWindow {
				cloned := *fixture.window.state
				cloned.issuer = newNonSourceWindowIssuer(fixture.scheduler)
				cloned.authorityContext = newNonSourceAuthorityContext(
					&cloned,
					time.Time{},
					cloned.callerDeadline,
				)
				return nonSourceTransactionWindow{
					state: &cloned,
					seal:  validNonSourceTransactionWindow,
				}
			},
		},
		{
			name: "state deadline",
			forge: func(fixture *runnerTestFixture) nonSourceTransactionWindow {
				cloned := *fixture.window.state
				cloned.transactionDeadline = cloned.transactionDeadline.Add(time.Second)
				return nonSourceTransactionWindow{
					state: &cloned,
					seal:  validNonSourceTransactionWindow,
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRunnerTestFixture(t)
			result := fixture.block.run(test.forge(fixture), fixture.plans)
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				OperationValidate,
				CauseInternalInvariant,
			)
			requireRunnerTestNoOtherSlots(t, result, false)
			requireRunnerTestTrace(t, fixture.trace.events, nil)
			if len(fixture.processes.tokens) != 0 {
				t.Fatalf("forged transaction invoked %d processes", len(fixture.processes.tokens))
			}
			fixture.requireSamples(t, 1)
		})
	}
}

func TestNonSourceBlockRejectsTerminatedEntryContext(t *testing.T) {
	for _, test := range []struct {
		name  string
		ctx   func() context.Context
		cause CauseCode
	}{
		{
			name: "nil",
			ctx: func() context.Context {
				return nil
			},
			cause: CauseInvalidRequest,
		},
		{
			name: "canceled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			cause: CauseCanceled,
		},
		{
			name: "expired",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(
					context.Background(),
					time.Unix(1, 0),
				)
				t.Cleanup(cancel)
				return ctx
			},
			cause: CauseDeadline,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := test.ctx()
			fixture := newRunnerTestFixtureWithoutTransaction(t, ctx)
			window, failure := fixture.block.beginTransaction(ctx)
			if window.state != nil || window.seal != 0 {
				t.Fatalf("terminated entry transaction = %+v, want zero window", window)
			}
			requireRunnerTestRecord(
				t,
				failure,
				PhaseRequest,
				OperationValidate,
				test.cause,
			)
			requireRunnerTestTrace(t, fixture.trace.events, nil)
			if len(fixture.processes.tokens) != 0 {
				t.Fatalf("terminated entry invoked %d processes", len(fixture.processes.tokens))
			}
			fixture.requireSamples(t, 1)
		})
	}
}

func TestNonSourceAuthorityContextUsesEarliestDeadlineAndExactCaller(t *testing.T) {
	for _, test := range []struct {
		name                    string
		callerDeadlineOffset    time.Duration
		wantTransactionDeadline time.Duration
		wantCommandDeadline     time.Duration
	}{
		{
			name:                    "caller",
			callerDeadlineOffset:    10 * time.Second,
			wantTransactionDeadline: 10 * time.Second,
			wantCommandDeadline:     10 * time.Second,
		},
		{
			name:                    "phase",
			callerDeadlineOffset:    10 * time.Minute,
			wantTransactionDeadline: 10 * time.Minute,
			wantCommandDeadline:     time.Second + nonSourceCommandDuration,
		},
		{
			name:                    "transaction",
			callerDeadlineOffset:    2 * time.Hour,
			wantTransactionDeadline: noScratchTransactionDuration,
			wantCommandDeadline:     time.Second + nonSourceCommandDuration,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := time.Unix(1_700_000_000, 0).UTC()
			caller := &runnerTestDeadlineContext{
				Context:  context.Background(),
				deadline: base.Add(test.callerDeadlineOffset),
			}
			fixture := newRunnerTestFixtureWithContext(t, caller)
			transactionDeadline, ok := fixture.window.state.authorityContext.Deadline()
			if !ok || transactionDeadline != base.Add(test.wantTransactionDeadline) {
				t.Fatalf(
					"transaction authority deadline = %s / %t, want %s / true",
					transactionDeadline,
					ok,
					base.Add(test.wantTransactionDeadline),
				)
			}

			environment := fixture.plans.goVersion.environment
			window, failure := fixture.issuer.mintCommand(
				fixture.window,
				nonSourceCommandGoVersion,
				environment,
			)
			if failure != nil {
				t.Fatalf("mint command window: %+v", failure)
			}
			commandDeadline, ok := window.authorityContext.Deadline()
			if !ok || commandDeadline != base.Add(test.wantCommandDeadline) {
				t.Fatalf(
					"command authority deadline = %s / %t, want %s / true",
					commandDeadline,
					ok,
					base.Add(test.wantCommandDeadline),
				)
			}
			if failure := window.admitRetainedNull(environment.authorities.nullDevice); failure != nil {
				t.Fatalf("admit retained null: %+v", failure)
			}
			if failure := window.authorize(); failure != nil {
				t.Fatalf("authorize command window: %+v", failure)
			}
			authority, failure := window.consume(
				nonSourceCommandGoVersion,
				environment,
				fixture.issuer,
			)
			if failure != nil {
				t.Fatalf("consume command window: %+v", failure)
			}
			if authority.ctx != caller || authority.phaseDeadline != window.phaseDeadline ||
				authority.callerDeadline != fixture.window.state.callerDeadline ||
				authority.transactionDeadline != fixture.window.state.transactionDeadline {
				t.Fatalf("process authority = %+v, want exact caller and window deadlines", authority)
			}
			if context.Context(window.authorityContext) == context.Context(caller) {
				t.Fatal("A2 authority wrapper escaped as the process caller context")
			}
			fixture.requireSamples(t, 3)
		})
	}
}

func TestNonSourceEntryCallerDeadlineCannotBeExtendedOrRemoved(t *testing.T) {
	for _, test := range []struct {
		name        string
		mutate      func(*runnerTestMutableDeadlineContext, time.Time)
		wantDynamic bool
	}{
		{
			name: "extended",
			mutate: func(ctx *runnerTestMutableDeadlineContext, base time.Time) {
				ctx.deadline = base.Add(2 * time.Hour)
			},
			wantDynamic: true,
		},
		{
			name: "removed",
			mutate: func(ctx *runnerTestMutableDeadlineContext, _ time.Time) {
				ctx.deadline = time.Time{}
				ctx.hasDeadline = false
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := time.Unix(1_700_000_000, 0).UTC()
			entryDeadline := base.Add(time.Second)
			caller := &runnerTestMutableDeadlineContext{
				Context:     context.Background(),
				deadline:    entryDeadline,
				hasDeadline: true,
			}
			fixture := newRunnerTestFixtureWithContext(t, caller)
			if caller.errCalls != 1 || caller.deadlineCalls != 1 ||
				fixture.window.state.callerDeadline != entryDeadline {
				t.Fatalf(
					"entry samples err=%d deadline=%d sealed=%s",
					caller.errCalls,
					caller.deadlineCalls,
					fixture.window.state.callerDeadline,
				)
			}

			test.mutate(caller, base)
			if deadline, ok := fixture.window.state.authorityContext.Deadline(); !ok || deadline != entryDeadline {
				t.Fatalf("authority deadline = %s / %t, want sealed %s", deadline, ok, entryDeadline)
			}
			if caller.deadlineCalls != 2 {
				t.Fatalf("Deadline samples after projection = %d, want 2", caller.deadlineCalls)
			}

			fixture.scheduler.overrides[1] = base.Add(2 * time.Second)
			beforeErr, beforeDeadline := caller.errCalls, caller.deadlineCalls
			if err := fixture.window.state.authorityContext.Err(); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("authority Err = %v, want deadline", err)
			}
			if caller.errCalls != beforeErr+1 || caller.deadlineCalls != beforeDeadline+1 {
				t.Fatalf(
					"authority boundary samples err=%d deadline=%d, want one each",
					caller.errCalls-beforeErr,
					caller.deadlineCalls-beforeDeadline,
				)
			}

			request := processRequest{
				ctx:                 caller,
				phase:               PhaseAuthority,
				phaseDeadline:       base.Add(nonSourceCommandDuration),
				callerDeadline:      fixture.window.state.callerDeadline,
				transactionDeadline: fixture.window.state.transactionDeadline,
			}
			beforeDeadline = caller.deadlineCalls
			if next := nextProcessDeadline(&request, base); next != entryDeadline {
				t.Fatalf("next process deadline = %s, want sealed %s", next, entryDeadline)
			}
			if caller.deadlineCalls != beforeDeadline+1 {
				t.Fatalf(
					"next process deadline samples = %d, want one",
					caller.deadlineCalls-beforeDeadline,
				)
			}
			beforeErr, beforeDeadline = caller.errCalls, caller.deadlineCalls
			if stop := processRequestStop(&request, base.Add(2*time.Second)); stop.kind != processStopDeadline {
				t.Fatalf("process stop = %d, want deadline", stop.kind)
			}
			if caller.errCalls != beforeErr+1 || caller.deadlineCalls != beforeDeadline+1 {
				t.Fatalf(
					"process boundary samples err=%d deadline=%d, want one each",
					caller.errCalls-beforeErr,
					caller.deadlineCalls-beforeDeadline,
				)
			}
			if test.wantDynamic && caller.deadline != base.Add(2*time.Hour) {
				t.Fatal("extension fixture did not retain its hostile dynamic deadline")
			}
		})
	}
}

func TestNonSourceCommandMintUsesSealedCallerDeadline(t *testing.T) {
	for _, remove := range []bool{false, true} {
		name := "extended"
		if remove {
			name = "removed"
		}
		t.Run(name, func(t *testing.T) {
			base := time.Unix(1_700_000_000, 0).UTC()
			caller := &runnerTestMutableDeadlineContext{
				Context:     context.Background(),
				deadline:    base.Add(time.Second),
				hasDeadline: true,
			}
			fixture := newRunnerTestFixtureWithContext(t, caller)
			if remove {
				caller.deadline = time.Time{}
				caller.hasDeadline = false
			} else {
				caller.deadline = base.Add(2 * time.Hour)
			}
			fixture.scheduler.overrides[1] = base.Add(2 * time.Second)
			beforeErr, beforeDeadline := caller.errCalls, caller.deadlineCalls
			window, failure := fixture.issuer.mintCommand(
				fixture.window,
				nonSourceCommandGoVersion,
				fixture.plans.goVersion.environment,
			)
			if window != nil {
				t.Fatalf("expired sealed caller window = %+v, want nil", window)
			}
			requireRunnerTestRecord(
				t,
				failure,
				PhaseAuthority,
				OperationProbe,
				CauseDeadline,
			)
			if caller.errCalls != beforeErr+1 || caller.deadlineCalls != beforeDeadline+1 {
				t.Fatalf(
					"mint boundary samples err=%d deadline=%d, want one each",
					caller.errCalls-beforeErr,
					caller.deadlineCalls-beforeDeadline,
				)
			}
		})
	}
}

func TestNonSourceCallerDeadlineHonorsLaterShortening(t *testing.T) {
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
			base := time.Unix(1_700_000_000, 0).UTC()
			shortened := base.Add(time.Second)
			caller := &runnerTestMutableDeadlineContext{
				Context:     context.Background(),
				deadline:    base.Add(2 * time.Hour),
				hasDeadline: true,
			}
			fixture := newRunnerTestFixtureWithContext(t, caller)
			caller.deadline = shortened

			if deadline, ok := fixture.window.state.authorityContext.Deadline(); !ok || deadline != shortened {
				t.Fatalf("shortened authority deadline = %s / %t, want %s", deadline, ok, shortened)
			}
			test.mutate(caller, base)
			if deadline, ok := fixture.window.state.authorityContext.Deadline(); !ok || deadline != shortened {
				t.Fatalf("retained authority deadline = %s / %t, want %s", deadline, ok, shortened)
			}
			if deadline, ok := fixture.window.state.callerDeadlineFloor.snapshot(); !ok || deadline != shortened {
				t.Fatalf("transaction deadline floor = %s / %t, want %s", deadline, ok, shortened)
			}
			fixture.scheduler.overrides[1] = base.Add(2 * time.Second)
			if err := fixture.window.state.authorityContext.Err(); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("shortened authority Err = %v, want deadline", err)
			}

			caller.deadline = shortened
			caller.hasDeadline = true
			request := processRequest{
				ctx:                 caller,
				phase:               PhaseAuthority,
				phaseDeadline:       base.Add(nonSourceCommandDuration),
				callerDeadline:      base.Add(2 * time.Hour),
				transactionDeadline: fixture.window.state.transactionDeadline,
			}
			if next := nextProcessDeadline(&request, base); next != shortened {
				t.Fatalf("next shortened process deadline = %s, want %s", next, shortened)
			}
			test.mutate(caller, base)
			if next := nextProcessDeadline(&request, base); next != shortened {
				t.Fatalf("retained process deadline = %s, want %s", next, shortened)
			}
			if stop := processRequestStop(&request, base.Add(2*time.Second)); stop.kind != processStopDeadline {
				t.Fatalf("shortened process stop = %d, want deadline", stop.kind)
			}
		})
	}
}

func TestNonSourceCallerZeroDeadlineStaysExpiredAfterRemoval(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	caller := &runnerTestMutableDeadlineContext{
		Context:     context.Background(),
		deadline:    base.Add(2 * time.Hour),
		hasDeadline: true,
	}
	fixture := newRunnerTestFixtureWithContext(t, caller)
	caller.deadline = time.Time{}

	if deadline, ok := fixture.window.state.authorityContext.Deadline(); !ok || !deadline.IsZero() {
		t.Fatalf("zero caller deadline = %s / %t, want zero / true", deadline, ok)
	}
	caller.hasDeadline = false
	if deadline, ok := fixture.window.state.authorityContext.Deadline(); !ok || !deadline.IsZero() {
		t.Fatalf("retained zero caller deadline = %s / %t, want zero / true", deadline, ok)
	}
	fixture.scheduler.overrides[1] = base
	if err := fixture.window.state.authorityContext.Err(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("zero caller deadline Err = %v, want deadline", err)
	}

	caller.deadline = time.Time{}
	caller.hasDeadline = true
	request := processRequest{
		ctx:                 caller,
		phase:               PhaseAuthority,
		phaseDeadline:       base.Add(nonSourceCommandDuration),
		callerDeadline:      base.Add(2 * time.Hour),
		transactionDeadline: fixture.window.state.transactionDeadline,
	}
	if next := nextProcessDeadline(&request, base); next != base {
		t.Fatalf("zero process deadline = %s, want sampled %s", next, base)
	}
	caller.hasDeadline = false
	if stop := processRequestStop(&request, base); stop.kind != processStopDeadline {
		t.Fatalf("retained zero process stop = %d, want deadline", stop.kind)
	}
}

func TestNonSourceCallerDeadlineTamperInvalidatesIssuedAuthority(t *testing.T) {
	newFixture := func(t *testing.T) *runnerTestFixture {
		t.Helper()
		base := time.Unix(1_700_000_000, 0).UTC()
		return newRunnerTestFixtureWithContext(t, &runnerTestDeadlineContext{
			Context:  context.Background(),
			deadline: base.Add(10 * time.Minute),
		})
	}

	t.Run("transaction state", func(t *testing.T) {
		fixture := newFixture(t)
		fixture.window.state.callerDeadline = fixture.window.state.callerDeadline.Add(time.Second)
		if fixture.window.validFor(fixture.issuer) ||
			fixture.window.state.authorityContext.validFor(
				fixture.window.state,
				time.Time{},
				fixture.window.state.callerDeadline,
			) {
			t.Fatal("tampered entry caller deadline retained transaction authority")
		}
	})

	t.Run("command window", func(t *testing.T) {
		fixture := newFixture(t)
		environment := fixture.plans.goVersion.environment
		window, failure := fixture.issuer.mintCommand(
			fixture.window,
			nonSourceCommandGoVersion,
			environment,
		)
		if failure != nil {
			t.Fatalf("mint command window: %+v", failure)
		}
		window.callerDeadline = window.callerDeadline.Add(time.Second)
		requireRunnerTestRecord(
			t,
			window.boundaryFailure(OperationProbe),
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
	})
}

func TestNonSourceAuthorityContextDeadlineIsStickyAndPrecedesCancel(t *testing.T) {
	base := time.Unix(1_700_000_000, 0).UTC()
	callerBase, cancel := context.WithCancel(context.Background())
	caller := &runnerTestDeadlineContext{
		Context:  callerBase,
		deadline: base.Add(10 * time.Second),
	}
	fixture := newRunnerTestFixtureWithContext(t, caller)
	cancel()
	fixture.scheduler.overrides[1] = caller.deadline

	first := fixture.window.state.authorityContext.Err()
	if !errors.Is(first, context.DeadlineExceeded) {
		t.Fatalf("deadline-plus-cancel boundary = %v, want deadline", first)
	}
	second := fixture.window.state.authorityContext.Err()
	if !errors.Is(second, context.DeadlineExceeded) {
		t.Fatalf("sticky authority boundary = %v, want deadline", second)
	}
	if len(fixture.scheduler.samples) != 2 {
		t.Fatalf(
			"sticky boundary sampled clock %d times, want entry plus first Err",
			len(fixture.scheduler.samples),
		)
	}
	if fixture.scheduler.samples[1] != caller.deadline {
		t.Fatalf(
			"deadline boundary sample = %s, want %s",
			fixture.scheduler.samples[1],
			caller.deadline,
		)
	}
}

func TestProcessNotificationsRejectNonstandardContextError(t *testing.T) {
	base := time.Unix(1_700_100_000, 0).UTC()
	ctx := &runnerTestErrorContext{
		Context: context.Background(),
		err:     errors.New("nonstandard context stop"),
	}
	request := processRequest{
		ctx:                 ctx,
		phase:               PhaseAuthority,
		phaseDeadline:       base.Add(time.Minute),
		transactionDeadline: base.Add(time.Hour),
	}
	notifications := newProcessNotifications()
	notifications.publish(processStopOutputLimit)

	stop := notifications.sample(&request, base)
	if stop.kind != processStopInvalidContext {
		t.Fatalf("hostile context stop = %d, want invalid context", stop.kind)
	}
	requireRunnerTestRecord(
		t,
		stop.primary(request.phase),
		PhaseAuthority,
		OperationValidate,
		CauseInternalInvariant,
	)
	if ctx.errCalls != 1 {
		t.Fatalf("hostile context Err calls = %d, want exactly one", ctx.errCalls)
	}
}

func TestProcessRequestBoundaryRejectsInvalidContextAndClock(t *testing.T) {
	base := time.Unix(1_700_100_000, 0).UTC()
	for _, test := range []struct {
		name         string
		now          time.Time
		wantErrCalls int
	}{
		{name: "nonstandard context error", now: base, wantErrCalls: 1},
		{name: "zero scheduler time", wantErrCalls: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &runnerTestErrorContext{
				Context: context.Background(),
				err:     errors.New("nonstandard context stop"),
			}
			failure := processRequestBoundaryFailure(&processRequest{
				ctx:                 ctx,
				phase:               PhaseAuthority,
				phaseDeadline:       base.Add(time.Minute),
				transactionDeadline: base.Add(time.Hour),
			}, test.now)
			requireRunnerTestRecord(
				t,
				failure,
				PhaseAuthority,
				OperationValidate,
				CauseInternalInvariant,
			)
			if ctx.errCalls != test.wantErrCalls {
				t.Fatalf(
					"process boundary Err calls = %d, want %d",
					ctx.errCalls,
					test.wantErrCalls,
				)
			}
		})
	}
}

func TestNonSourceBlockAllSuccessUsesExactOrderAndMintsProof(t *testing.T) {
	fixture := newRunnerTestFixture(t)
	result := fixture.run()
	if result.failed() || result.proof == nil ||
		!result.proof.validFor(fixture.plans, fixture.window) {
		t.Fatalf(
			"successful non-source result = %+v; primary = %+v; trace = %q",
			result,
			result.primary,
			fixture.trace.events,
		)
	}
	if !result.goVersion || !result.goEnvironment || !result.compilerVersion ||
		!result.gitVersion || !result.gitBuiltinInventory {
		t.Fatalf("successful command proofs = %+v", result)
	}
	requireRunnerTestTrace(t, fixture.trace.events, runnerAllSuccessTrace())
	fixture.requireSamples(t, 76)

	wantKinds := []nonSourceCommandKind{
		nonSourceCommandGoVersion,
		nonSourceCommandGoEnvironment,
		nonSourceCommandCompilerVersion,
		nonSourceCommandGitVersion,
		nonSourceCommandGitBuiltinInventory,
	}
	wantSampleIndexes := []int{4, 19, 34, 47, 58}
	if len(fixture.processes.tokens) != len(wantKinds) {
		t.Fatalf("consumed command tokens = %d, want %d", len(fixture.processes.tokens), len(wantKinds))
	}
	for index, token := range fixture.processes.tokens {
		if token.kind != wantKinds[index] || token.window == nil ||
			!token.window.consumed() ||
			token.phaseSample != fixture.scheduler.samples[wantSampleIndexes[index]] ||
			token.phaseDeadline != token.phaseSample.Add(nonSourceCommandDuration) ||
			token.callerDeadline != fixture.window.state.callerDeadline ||
			token.transactionDeadline != fixture.window.state.transactionDeadline {
			t.Fatalf("command token %d = %+v", index, token)
		}
	}
	wantAuthorityContexts := map[*nonSourceAuthorityContext]int{
		fixture.window.state.authorityContext:               9,
		fixture.processes.tokens[0].window.authorityContext: 10,
		fixture.processes.tokens[1].window.authorityContext: 10,
		fixture.processes.tokens[2].window.authorityContext: 8,
		fixture.processes.tokens[3].window.authorityContext: 6,
		fixture.processes.tokens[4].window.authorityContext: 6,
	}
	if len(fixture.authority.contexts) != len(wantAuthorityContexts) {
		t.Fatalf(
			"authority contexts = %d, want %d",
			len(fixture.authority.contexts),
			len(wantAuthorityContexts),
		)
	}
	for authorityContext, wantCalls := range wantAuthorityContexts {
		if authorityContext == nil ||
			fixture.authority.contexts[authorityContext] != wantCalls {
			t.Fatalf(
				"authority context %+v calls = %d, want %d",
				authorityContext,
				fixture.authority.contexts[authorityContext],
				wantCalls,
			)
		}
	}
	if fixture.window.state.sampledAt != fixture.scheduler.samples[0] ||
		fixture.window.state.transactionDeadline !=
			fixture.scheduler.samples[0].Add(noScratchTransactionDuration) {
		t.Fatalf("transaction window = %+v", fixture.window.state)
	}
}

func TestNonSourceBlockInitialFailureLeavesBlockEndUnarmed(t *testing.T) {
	fixture := newRunnerTestFixture(t)
	fixture.authority.outcomes["authority:sentinels-admit#1"] = runnerAuthorityFailure(
		OperationProbe,
		CausePermission,
	)
	result := fixture.run()
	requireRunnerTestRecord(t, result.primary, PhaseAuthority, OperationProbe, CausePermission)
	requireRunnerTestNoOtherSlots(t, result, false)
	requireRunnerTestTrace(t, fixture.trace.events, []string{
		"authority:root#1",
		"authority:null#1",
		"authority:sentinels-admit#1",
	})
	if len(fixture.processes.tokens) != 0 {
		t.Fatalf("unarmed initial gate invoked %d processes", len(fixture.processes.tokens))
	}
	fixture.requireSamples(t, 4)
}

func TestNonSourceBlockPrecheckFailureSkipsInvocationButRunsBlockEnd(t *testing.T) {
	fixture := newRunnerTestFixture(t)
	fixture.authority.outcomes["authority:root#2"] = runnerAuthorityFailure(
		OperationCompare,
		CauseUnstable,
	)
	result := fixture.run()
	requireRunnerTestRecord(t, result.primary, PhaseAuthority, OperationCompare, CauseUnstable)
	requireRunnerTestNoOtherSlots(t, result, false)
	requireRunnerTestTrace(t, fixture.trace.events, []string{
		"authority:root#1",
		"authority:null#1",
		"authority:sentinels-admit#1",
		"authority:root#2",
		"authority:root#3",
		"authority:null#2",
		"authority:sentinels-revalidate#1",
		"authority:go#1",
		"authority:compiler#1",
		"authority:git#1",
	})
	if len(fixture.processes.tokens) != 0 {
		t.Fatalf("failed precheck invoked %d processes", len(fixture.processes.tokens))
	}
	fixture.requireSamples(t, 13)
}

func TestNonSourceBlockChecksTerminationBeforeAndBetweenPrechecks(t *testing.T) {
	for _, test := range []struct {
		name        string
		stopIndex   int
		cause       CauseCode
		operation   Operation
		wantTrace   []string
		wantSamples int
		wantLater   bool
	}{
		{
			name:        "deadline before first precheck",
			stopIndex:   5,
			cause:       CauseDeadline,
			operation:   OperationProbe,
			wantTrace:   runnerBoundaryBeforeFirstPrecheckTrace(),
			wantSamples: 13,
		},
		{
			name:        "canceled before first precheck",
			stopIndex:   5,
			cause:       CauseCanceled,
			operation:   OperationProbe,
			wantTrace:   runnerBoundaryBeforeFirstPrecheckTrace(),
			wantSamples: 7,
			wantLater:   true,
		},
		{
			name:        "deadline before Go role precheck",
			stopIndex:   9,
			cause:       CauseDeadline,
			operation:   OperationCompare,
			wantTrace:   runnerBoundaryBeforeGoRolePrecheckTrace(),
			wantSamples: 17,
		},
		{
			name:        "canceled before Go role precheck",
			stopIndex:   9,
			cause:       CauseCanceled,
			operation:   OperationCompare,
			wantTrace:   runnerBoundaryBeforeGoRolePrecheckTrace(),
			wantSamples: 11,
			wantLater:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			cancel := func() {}
			if test.cause == CauseCanceled {
				ctx, cancel = context.WithCancel(ctx)
				t.Cleanup(cancel)
			}
			fixture := newRunnerTestFixtureWithContext(t, ctx)
			phaseDeadline := fixture.scheduler.base.Add(4 * time.Second).
				Add(nonSourceCommandDuration)
			if test.cause == CauseDeadline {
				fixture.scheduler.overrides[test.stopIndex] = phaseDeadline
			} else {
				fixture.scheduler.hooks[test.stopIndex] = cancel
			}

			result := fixture.run()
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				test.operation,
				test.cause,
			)
			if test.wantLater {
				requireRunnerTestRecord(
					t,
					result.later,
					PhaseAuthority,
					OperationProbe,
					CauseCanceled,
				)
			} else if result.later != nil {
				t.Fatalf("later = %+v, want nil", result.later)
			}
			if result.descriptorClose != nil || result.cleanup != nil || result.proof != nil {
				t.Fatalf("unexpected termination slots = %+v", result)
			}
			requireRunnerTestTrace(t, fixture.trace.events, test.wantTrace)
			if len(fixture.processes.tokens) != 0 {
				t.Fatalf("precheck termination invoked %d processes", len(fixture.processes.tokens))
			}
			fixture.requireMonotonicSamples(t, test.wantSamples)
			if test.cause == CauseDeadline &&
				fixture.scheduler.samples[test.stopIndex] != phaseDeadline {
				t.Fatalf(
					"deadline sample = %s, want %s",
					fixture.scheduler.samples[test.stopIndex],
					phaseDeadline,
				)
			}
		})
	}
}

func TestNonSourceBlockInvokedFailuresRunPostcheckAndBlockEnd(t *testing.T) {
	descriptorChild := runnerStructuredOutputChild()
	quiescenceChild := runnerStructuredOutputChild()
	childExitFailure := runnerProcessFailure(OperationExecute, CauseChildExit)
	outputChild := runnerStructuredOutputChild()
	closeFailure := &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationCloseNonRoot,
		Causes:    []CauseCode{CauseDescriptorClose},
		Child:     descriptorChild,
	}
	for _, test := range []struct {
		name           string
		process        processResult
		wantPrimary    *FailureRecord
		wantDescriptor bool
		wantCleanup    bool
		wantSamples    int
	}{
		{
			name: "request",
			process: runnerProcessFailure(
				OperationValidate,
				CauseInternalInvariant,
			),
			wantPrimary: authorityFailure(OperationValidate, CauseInternalInvariant),
			wantSamples: 25,
		},
		{
			name:        "setup",
			process:     runnerProcessFailure(OperationOpen, CauseLimit),
			wantPrimary: authorityFailure(OperationOpen, CauseLimit),
			wantSamples: 25,
		},
		{
			name:        "start",
			process:     runnerProcessFailure(OperationExecute, CauseChildStart),
			wantPrimary: authorityFailure(OperationExecute, CauseChildStart),
			wantSamples: 25,
		},
		{
			name:        "execute",
			process:     childExitFailure,
			wantPrimary: childExitFailure.primary,
			wantSamples: 25,
		},
		{
			name: "output",
			process: processResult{
				structuredOutput:      []byte("not a version"),
				structuredOutputChild: outputChild,
				quiescence:            quiescenceResult{proven: true},
			},
			wantPrimary: &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: OperationParse,
				Causes:    []CauseCode{CauseMalformed},
				Child:     outputChild,
			},
			wantSamples: 26,
		},
		{
			name: "descriptor close",
			process: processResult{
				descriptorClose: closeFailure,
				quiescence:      quiescenceResult{proven: true},
			},
			wantDescriptor: true,
			wantSamples:    25,
		},
		{
			name: "quiescence",
			process: processResult{
				quiescence: quiescenceResult{
					causes: []CauseCode{CauseChildWait},
					child:  quiescenceChild,
				},
			},
			wantCleanup: true,
			wantSamples: 25,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRunnerTestFixture(t)
			fixture.processes.results["process:go-version#1"] = test.process
			result := fixture.run()
			if test.wantPrimary == nil {
				if result.primary != nil {
					t.Fatalf("primary = %+v, want nil", result.primary)
				}
			} else {
				requireRunnerTestRecordWithChild(
					t,
					result.primary,
					test.wantPrimary.Phase,
					test.wantPrimary.Operation,
					test.wantPrimary.Causes[0],
					test.wantPrimary.Child,
				)
			}
			if result.later != nil || result.proof != nil {
				t.Fatalf("unexpected later/proof = %+v / %+v", result.later, result.proof)
			}
			if test.wantDescriptor {
				requireRunnerTestRecordWithChild(
					t,
					result.descriptorClose,
					PhaseClose,
					OperationCloseNonRoot,
					CauseDescriptorClose,
					descriptorChild,
				)
			} else if result.descriptorClose != nil {
				t.Fatalf("descriptor close = %+v, want nil", result.descriptorClose)
			}
			if test.wantCleanup {
				requireRunnerTestRecordWithChild(
					t,
					result.cleanup,
					PhaseClose,
					OperationQuiesce,
					CauseChildWait,
					quiescenceChild,
				)
			} else if result.cleanup != nil {
				t.Fatalf("cleanup = %+v, want nil", result.cleanup)
			}
			if !result.failed() {
				t.Fatal("invoked failure was reported as success")
			}
			requireRunnerTestTrace(t, fixture.trace.events, runnerFirstRowFailureTrace())
			if len(fixture.processes.tokens) != 1 ||
				fixture.processes.tokens[0].kind != nonSourceCommandGoVersion {
				t.Fatalf("process tokens = %+v, want only Go version", fixture.processes.tokens)
			}
			fixture.requireSamples(t, test.wantSamples)
		})
	}
}

func TestNonSourceBlockAttributesCandidateOutputBoundaryToParse(t *testing.T) {
	for _, test := range []struct {
		name        string
		cause       CauseCode
		wantSamples int
	}{
		{name: "deadline", cause: CauseDeadline, wantSamples: 21},
		{name: "canceled", cause: CauseCanceled, wantSamples: 15},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			cancel := func() {}
			if test.cause == CauseCanceled {
				ctx, cancel = context.WithCancel(ctx)
				t.Cleanup(cancel)
			}
			fixture := newRunnerTestFixtureWithContext(t, ctx)
			phaseDeadline := fixture.scheduler.base.Add(4 * time.Second).
				Add(nonSourceCommandDuration)
			if test.cause == CauseDeadline {
				fixture.scheduler.overrides[12] = phaseDeadline
			} else {
				fixture.scheduler.hooks[12] = cancel
			}

			result := fixture.run()
			requireRunnerTestRecordWithChild(
				t,
				result.primary,
				PhaseAuthority,
				OperationParse,
				test.cause,
				fixture.processes.outputChild,
			)
			requireRunnerTestRecord(
				t,
				result.later,
				PhaseAuthority,
				OperationProbe,
				test.cause,
			)
			if result.descriptorClose != nil || result.cleanup != nil || result.proof != nil {
				t.Fatalf("unexpected output-boundary slots = %+v", result)
			}
			requireRunnerTestTrace(t, fixture.trace.events, runnerFirstRowFailureTrace())
			if len(fixture.processes.tokens) != 1 ||
				fixture.processes.tokens[0].kind != nonSourceCommandGoVersion {
				t.Fatalf("output-boundary tokens = %+v, want only Go version", fixture.processes.tokens)
			}
			fixture.requireMonotonicSamples(t, test.wantSamples)
			if test.cause == CauseDeadline && fixture.scheduler.samples[12] != phaseDeadline {
				t.Fatalf(
					"output deadline sample = %s, want %s",
					fixture.scheduler.samples[12],
					phaseDeadline,
				)
			}
		})
	}
}

func TestNonSourceOutputValidationFailureRetainsSuccessfulChildPrivately(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation Operation
		cause     CauseCode
	}{
		{name: "parse", operation: OperationParse, cause: CauseMalformed},
		{name: "compare", operation: OperationCompare, cause: CauseIdentity},
	} {
		t.Run(test.name, func(t *testing.T) {
			child := runnerStructuredOutputChild()
			if !validStructuredOutputChild(child) {
				t.Fatal("test output Child is not the exact successful-child witness")
			}
			result := nonSourceBlockResult{}
			result.addOutputValidationFailure(
				authorityFailure(test.operation, test.cause),
				child,
			)
			requireRunnerTestRecordWithChild(
				t,
				result.primary,
				PhaseAuthority,
				test.operation,
				test.cause,
				child,
			)
			if result.later != nil || result.descriptorClose != nil ||
				result.cleanup != nil || result.proof != nil {
				t.Fatalf("output validation failure gained slots: %+v", result)
			}
		})
	}

	t.Run("success", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.addOutputValidationFailure(nil, runnerStructuredOutputChild())
		if result.primary != nil || result.later != nil || result.descriptorClose != nil ||
			result.cleanup != nil || result.proof != nil {
			t.Fatalf("successful output exposed its private Child: %+v", result)
		}
	})
}

func TestNonSourceAccumulatorRejectsInvalidStructuredOutputChildShape(t *testing.T) {
	for _, test := range []struct {
		name   string
		output []byte
		child  *ChildDiagnostic
		orphan bool
	}{
		{
			name:   "orphan child",
			child:  runnerStructuredOutputChild(),
			orphan: true,
		},
		{
			name:   "missing child",
			output: []byte("output"),
		},
		{
			name:   "nonzero status",
			output: []byte("output"),
			child: &ChildDiagnostic{
				ExitStatusObserved: true,
				ExitStatus:         1,
				DiagnosticSHA256:   Digest(sha256.Sum256(nil)),
			},
		},
		{
			name:   "wrong digest",
			output: []byte("output"),
			child: &ChildDiagnostic{
				ExitStatusObserved: true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := processResult{
				structuredOutput:      test.output,
				structuredOutputChild: test.child,
				quiescence:            quiescenceResult{proven: true},
			}
			if test.orphan != (process.structuredOutput == nil && process.structuredOutputChild != nil) {
				t.Fatal("invalid orphan test fixture")
			}
			if validNonSourceProcessOutputShape(process) || validNonSourceProcessResult(process) {
				t.Fatal("invalid structured-output Child shape was admitted")
			}

			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				OperationValidate,
				CauseInternalInvariant,
			)
			if result.later != nil || result.descriptorClose != nil ||
				result.cleanup != nil || result.proof != nil {
				t.Fatalf("invalid output Child escaped canonical slots: %+v", result)
			}
		})
	}
}

func TestNonSourceBlockPreservesPrimaryLaterAndDedicatedSlots(t *testing.T) {
	fixture := newRunnerTestFixture(t)
	primaryChild := &ChildDiagnostic{
		ExitStatusObserved: true,
		ExitStatus:         1,
		DiagnosticSHA256:   Digest(sha256.Sum256(nil)),
	}
	fixture.processes.results["process:go-version#1"] = processResult{
		primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseChildExit},
			Child:     primaryChild,
		},
		descriptorClose: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationCloseNonRoot,
			Causes:    []CauseCode{CauseDescriptorClose},
		},
		quiescence: quiescenceResult{causes: []CauseCode{CauseChildWait}},
	}
	fixture.authority.outcomes["authority:root#3"] = runnerAuthorityFailure(
		OperationCompare,
		CauseIdentity,
	)
	fixture.authority.outcomes["authority:null#3"] = runnerAuthorityFailure(
		OperationProbe,
		CausePermission,
	)
	fixture.authority.outcomes["authority:root#4"] = runnerAuthorityFailure(
		OperationHash,
		CauseUnstable,
	)

	result := fixture.run()
	requireRunnerTestRecordWithChild(
		t,
		result.primary,
		PhaseAuthority,
		OperationExecute,
		CauseChildExit,
		primaryChild,
	)
	requireRunnerTestRecord(
		t,
		result.later,
		PhaseAuthority,
		OperationCompare,
		CauseIdentity,
	)
	requireRunnerTestRecord(
		t,
		result.descriptorClose,
		PhaseClose,
		OperationCloseNonRoot,
		CauseDescriptorClose,
	)
	requireRunnerTestRecord(
		t,
		result.cleanup,
		PhaseClose,
		OperationQuiesce,
		CauseChildWait,
	)
	if result.proof != nil {
		t.Fatalf("failed result minted proof = %+v", result.proof)
	}
	requireRunnerTestTrace(t, fixture.trace.events, runnerFirstRowFailureTrace())
	if len(fixture.processes.tokens) != 1 {
		t.Fatalf("slot failure invoked %d processes, want one", len(fixture.processes.tokens))
	}
	fixture.requireSamples(t, 25)
}

func TestNonSourceBlockRoutesPostAttemptBoundaryToPrivateLater(t *testing.T) {
	fixture := newRunnerTestFixture(t)
	process := runnerProcessFailure(
		OperationExecute,
		CauseChildExit,
	)
	fixture.processes.results["process:go-version#1"] = process
	phaseDeadline := fixture.scheduler.base.Add(4 * time.Second).
		Add(nonSourceCommandDuration)
	fixture.scheduler.overrides[17] = phaseDeadline

	result := fixture.run()
	requireRunnerTestRecordWithChild(
		t,
		result.primary,
		PhaseAuthority,
		OperationExecute,
		CauseChildExit,
		process.primary.Child,
	)
	requireRunnerTestRecord(
		t,
		result.later,
		PhaseAuthority,
		OperationValidate,
		CauseDeadline,
	)
	if result.descriptorClose != nil || result.cleanup != nil || result.proof != nil {
		t.Fatalf("unexpected post-attempt boundary slots = %+v", result)
	}
	requireRunnerTestTrace(t, fixture.trace.events, runnerFirstRowFailureTrace())
	if len(fixture.processes.tokens) != 1 ||
		fixture.processes.tokens[0].kind != nonSourceCommandGoVersion {
		t.Fatalf("post-attempt boundary tokens = %+v, want only Go version", fixture.processes.tokens)
	}
	fixture.requireMonotonicSamples(t, 25)
	if fixture.scheduler.samples[17] != phaseDeadline {
		t.Fatalf(
			"post-attempt deadline sample = %s, want %s",
			fixture.scheduler.samples[17],
			phaseDeadline,
		)
	}
}

func TestValidNonSourceAuthorityPairIsExactClosedUnion(t *testing.T) {
	accepted := map[Operation][]CauseCode{
		OperationValidate: {
			CausePermission,
			CauseUnsupported,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
			CauseIdentity,
			CauseInternalInvariant,
		},
		OperationOpen: {
			CausePermission,
			CauseUnsupported,
			CauseUnstable,
			CauseCanceled,
			CauseDeadline,
		},
		OperationProbe: {
			CausePermission,
			CauseUnsupported,
			CauseUnstable,
			CauseCanceled,
			CauseDeadline,
		},
		OperationWalk: {
			CausePermission,
			CauseUnstable,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
		},
		OperationParse: {
			CauseNotFound,
			CausePermission,
			CauseMalformed,
			CauseUnsupported,
			CauseUnstable,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
		},
		OperationHash: {
			CausePermission,
			CauseUnstable,
			CauseLimit,
			CauseCanceled,
			CauseDeadline,
		},
		OperationCompare: {
			CauseUnsupported,
			CauseUnstable,
			CauseCanceled,
			CauseDeadline,
			CauseIdentity,
		},
	}
	operations := []Operation{
		OperationValidate,
		OperationOpen,
		OperationProbe,
		OperationWalk,
		OperationParse,
		OperationHash,
		OperationCopy,
		OperationExecute,
		OperationCompare,
		OperationQuiesce,
		OperationCloseNonRoot,
		OperationRemove,
		OperationCloseRoot,
	}

	for _, operation := range operations {
		for _, cause := range causeOrder() {
			want := slices.Contains(accepted[operation], cause)
			if got := validNonSourceAuthorityPair(operation, cause); got != want {
				t.Errorf(
					"validNonSourceAuthorityPair(%q, %q) = %t, want %t",
					operation,
					cause,
					got,
					want,
				)
			}
		}
	}

	for _, test := range []struct {
		name      string
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "unknown operation",
			operation: Operation("unknown"),
			cause:     CausePermission,
		},
		{
			name:      "unknown cause",
			operation: OperationValidate,
			cause:     CauseCode("unknown"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if validNonSourceAuthorityPair(test.operation, test.cause) {
				t.Fatalf("unknown authority pair %q/%q was admitted", test.operation, test.cause)
			}
		})
	}
}

func TestNonSourceAccumulatorCanonicalizesInvalidAuthorityPair(t *testing.T) {
	closeRecord := &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationCloseNonRoot,
		Causes:    []CauseCode{CauseDescriptorClose},
	}
	outcome := authorityUseOutcome{
		primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationOpen,
			Causes:    []CauseCode{CauseIdentity},
		},
		descriptorClose: closeRecord,
	}
	if validAuthorityNonCloseOutcome(outcome) {
		t.Fatal("open/identity authority outcome was admitted")
	}

	t.Run("precheck", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.absorbPrecheck(outcome)
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
		requireRunnerTestRecord(
			t,
			result.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		if result.later != nil || result.cleanup != nil || result.proof != nil {
			t.Fatalf("invalid precheck pair escaped canonical slots: %+v", result)
		}
	})

	t.Run("postcheck", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.addPrimary(authorityFailure(OperationExecute, CauseCanceled))
		result.absorbPostcheck(outcome)
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationExecute,
			CauseCanceled,
		)
		requireRunnerTestRecord(
			t,
			result.later,
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
		requireRunnerTestRecord(
			t,
			result.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		if result.cleanup != nil || result.proof != nil {
			t.Fatalf("invalid postcheck pair escaped canonical slots: %+v", result)
		}
	})
}

func TestNonSourceAccumulatorCanonicalizesMalformedProcessAggregate(t *testing.T) {
	for _, test := range []struct {
		name        string
		withPrimary bool
	}{
		{name: "preserves valid primary", withPrimary: true},
		{name: "fills empty primary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			child := runnerStructuredOutputChild()
			process := processResult{
				structuredOutput:      []byte("conflicting structured output"),
				structuredOutputChild: runnerStructuredOutputChild(),
				descriptorClose: &FailureRecord{
					Phase:     PhaseClose,
					Operation: OperationCloseNonRoot,
					Causes:    []CauseCode{CauseDescriptorClose},
				},
				quiescence: quiescenceResult{
					causes: []CauseCode{CauseChildWait},
				},
			}
			if test.withPrimary {
				process.primary = &FailureRecord{
					Phase:     PhaseAuthority,
					Operation: OperationExecute,
					Causes:    []CauseCode{CauseCanceled},
					Child:     child,
				}
			} else {
				process.descriptorClose.Child = child
			}
			otherwiseValid := process
			otherwiseValid.structuredOutput = nil
			otherwiseValid.structuredOutputChild = nil
			if !validNonSourceProcessResult(otherwiseValid) {
				t.Fatal("process fixture is malformed without the conflicting output")
			}
			if validNonSourceProcessResult(process) {
				t.Fatal("conflicting process aggregate was admitted")
			}

			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			if !result.processInvariant {
				t.Fatal("malformed process aggregate did not retain its private invariant marker")
			}
			wantOperation := OperationValidate
			wantCause := CauseInternalInvariant
			if test.withPrimary {
				wantOperation = OperationExecute
				wantCause = CauseCanceled
			}
			var wantPrimaryChild *ChildDiagnostic
			if test.withPrimary {
				wantPrimaryChild = child
			}
			requireRunnerTestRecordWithChild(
				t,
				result.primary,
				PhaseAuthority,
				wantOperation,
				wantCause,
				wantPrimaryChild,
			)
			var wantDescriptorChild *ChildDiagnostic
			if !test.withPrimary {
				wantDescriptorChild = child
			}
			requireRunnerTestRecordWithChild(
				t,
				result.descriptorClose,
				PhaseClose,
				OperationCloseNonRoot,
				CauseDescriptorClose,
				wantDescriptorChild,
			)
			requireRunnerTestRecord(
				t,
				result.cleanup,
				PhaseClose,
				OperationQuiesce,
				CauseChildWait,
			)
			if result.later != nil || result.proof != nil {
				t.Fatalf("malformed process aggregate escaped canonical slots: %+v", result)
			}
		})
	}
}

func TestNonSourceProcessPrimaryRequiresChildOrRelocatesSoleChild(t *testing.T) {
	mandatory := []struct {
		name      string
		operation Operation
		cause     CauseCode
	}{
		{name: "parse", operation: OperationParse, cause: CauseMalformed},
		{name: "child exit", operation: OperationExecute, cause: CauseChildExit},
		{name: "limit", operation: OperationExecute, cause: CauseLimit},
	}
	for _, primaryTest := range mandatory {
		for _, location := range []string{"descriptor close", "cleanup", "absent"} {
			t.Run(primaryTest.name+"/"+location, func(t *testing.T) {
				status := 17
				if primaryTest.operation == OperationParse {
					status = 0
				}
				child := &ChildDiagnostic{
					ExitStatusObserved: true,
					ExitStatus:         status,
					DiagnosticBytes:    7,
				}
				process := processResult{
					primary: &FailureRecord{
						Phase:     PhaseAuthority,
						Operation: primaryTest.operation,
						Causes:    []CauseCode{primaryTest.cause},
					},
					quiescence: quiescenceResult{proven: true},
				}
				switch location {
				case "descriptor close":
					process.descriptorClose = &FailureRecord{
						Phase:     PhaseClose,
						Operation: OperationCloseNonRoot,
						Causes:    []CauseCode{CauseDescriptorClose},
						Child:     child,
					}
				case "cleanup":
					process.quiescence = quiescenceResult{
						causes: []CauseCode{CauseChildWait},
						child:  child,
					}
				}

				if validNonSourceProcessPrimary(process.primary) {
					t.Fatal("mandatory process primary admitted without its Child")
				}
				wantRelocated := location != "absent"
				if got := validNonSourceProcessPrimaryForAbsorption(process); got != wantRelocated {
					t.Fatalf("absorption eligibility = %t, want %t", got, wantRelocated)
				}
				if validNonSourceProcessResult(process) {
					t.Fatal("misplaced or absent mandatory Child passed aggregate validation")
				}

				result := nonSourceBlockResult{}
				result.absorbProcess(process)
				if wantRelocated {
					requireRunnerTestRecordWithChild(
						t,
						result.primary,
						PhaseAuthority,
						primaryTest.operation,
						primaryTest.cause,
						child,
					)
				} else {
					requireRunnerTestRecord(
						t,
						result.primary,
						PhaseAuthority,
						OperationValidate,
						CauseInternalInvariant,
					)
				}
				if location == "descriptor close" {
					requireRunnerTestRecord(
						t,
						result.descriptorClose,
						PhaseClose,
						OperationCloseNonRoot,
						CauseDescriptorClose,
					)
				} else if result.descriptorClose != nil {
					t.Fatalf("unexpected descriptor close = %+v", result.descriptorClose)
				}
				if location == "cleanup" {
					requireRunnerTestRecord(
						t,
						result.cleanup,
						PhaseClose,
						OperationQuiesce,
						CauseChildWait,
					)
				} else if result.cleanup != nil {
					t.Fatalf("unexpected cleanup = %+v", result.cleanup)
				}
				if result.later != nil || result.proof != nil {
					t.Fatalf("mandatory Child result gained later/proof: %+v", result)
				}
			})
		}
	}
}

func TestNonSourceProcessPrimaryAllowsAmbiguousNoChild(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation Operation
		cause     CauseCode
	}{
		{name: "validate internal", operation: OperationValidate, cause: CauseInternalInvariant},
		{name: "canceled", operation: OperationExecute, cause: CauseCanceled},
		{name: "deadline", operation: OperationExecute, cause: CauseDeadline},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := processResult{
				primary:    authorityFailure(test.operation, test.cause),
				quiescence: quiescenceResult{proven: true},
			}
			if !validNonSourceProcessPrimary(process.primary) ||
				!validNonSourceProcessResult(process) {
				t.Fatal("ambiguous process primary was rejected without a Child")
			}
			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				test.operation,
				test.cause,
			)
			if result.later != nil || result.descriptorClose != nil ||
				result.cleanup != nil || result.proof != nil {
				t.Fatalf("ambiguous process primary gained slots: %+v", result)
			}
		})
	}
}

func TestNonSourceAccumulatorNormalizesSingleChildOwnerPriority(t *testing.T) {
	for _, test := range []struct {
		name        string
		primary     *FailureRecord
		descriptor  bool
		wantOwner   string
		wantCleanup CauseCode
	}{
		{
			name:        "descriptor before cleanup",
			descriptor:  true,
			wantOwner:   "descriptor",
			wantCleanup: CauseChildWait,
		},
		{
			name:        "cleanup fallback",
			wantOwner:   "cleanup",
			wantCleanup: CauseChildWait,
		},
		{
			name: "definite pre-start drops child",
			primary: &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: OperationOpen,
				Causes:    []CauseCode{CausePermission},
			},
			wantOwner:   "none",
			wantCleanup: CauseInternalInvariant,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			child := runnerStructuredOutputChild()
			process := processResult{
				primary: test.primary,
				quiescence: quiescenceResult{
					causes: []CauseCode{CauseChildWait},
					child:  child,
				},
			}
			if test.descriptor {
				process.descriptorClose = &FailureRecord{
					Phase:     PhaseClose,
					Operation: OperationCloseNonRoot,
					Causes:    []CauseCode{CauseDescriptorClose},
				}
			}

			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			if test.primary == nil {
				if test.wantOwner == "cleanup" {
					if result.primary != nil {
						t.Fatalf("valid cleanup-only result gained Primary: %+v", result.primary)
					}
				} else {
					requireRunnerTestRecord(
						t,
						result.primary,
						PhaseAuthority,
						OperationValidate,
						CauseInternalInvariant,
					)
				}
			} else {
				requireRunnerTestRecord(
					t,
					result.primary,
					PhaseAuthority,
					OperationOpen,
					CausePermission,
				)
			}
			if test.descriptor {
				wantChild := (*ChildDiagnostic)(nil)
				if test.wantOwner == "descriptor" {
					wantChild = child
				}
				requireRunnerTestRecordWithChild(
					t,
					result.descriptorClose,
					PhaseClose,
					OperationCloseNonRoot,
					CauseDescriptorClose,
					wantChild,
				)
			} else if result.descriptorClose != nil {
				t.Fatalf("unexpected descriptor close = %+v", result.descriptorClose)
			}
			wantCleanupChild := (*ChildDiagnostic)(nil)
			if test.wantOwner == "cleanup" {
				wantCleanupChild = child
			}
			requireRunnerTestRecordWithChild(
				t,
				result.cleanup,
				PhaseClose,
				OperationQuiesce,
				test.wantCleanup,
				wantCleanupChild,
			)
			if result.later != nil || result.proof != nil {
				t.Fatalf("single Child normalization gained later/proof: %+v", result)
			}
		})
	}
}

func TestNonSourceFinishCommandRejectsUnconsumedWindow(t *testing.T) {
	for _, test := range []struct {
		name        string
		withPrimary bool
	}{
		{name: "preserves valid primary", withPrimary: true},
		{name: "fills empty primary"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRunnerTestFixture(t)
			window, failure := fixture.issuer.mintCommand(
				fixture.window,
				nonSourceCommandGoVersion,
				fixture.plans.goVersion.environment,
			)
			if failure != nil {
				t.Fatalf("mint command window: %+v", failure)
			}
			defer window.abort()
			if window.consumed() {
				t.Fatal("new command window is already consumed")
			}

			result := nonSourceBlockResult{
				descriptorClose: &FailureRecord{
					Phase:     PhaseClose,
					Operation: OperationCloseNonRoot,
					Causes:    []CauseCode{CauseDescriptorClose},
				},
				cleanup: &FailureRecord{
					Phase:     PhaseClose,
					Operation: OperationQuiesce,
					Causes:    []CauseCode{CauseChildWait},
				},
			}
			if test.withPrimary {
				result.primary = authorityFailure(OperationExecute, CauseCanceled)
			}
			if fixture.block.finishCommand(&result, window) {
				t.Fatal("unconsumed command window reported success")
			}

			wantOperation := OperationValidate
			wantCause := CauseInternalInvariant
			if test.withPrimary {
				wantOperation = OperationExecute
				wantCause = CauseCanceled
			}
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				wantOperation,
				wantCause,
			)
			requireRunnerTestRecord(
				t,
				result.descriptorClose,
				PhaseClose,
				OperationCloseNonRoot,
				CauseDescriptorClose,
			)
			requireRunnerTestRecord(
				t,
				result.cleanup,
				PhaseClose,
				OperationQuiesce,
				CauseChildWait,
			)
			if result.later != nil || result.proof != nil {
				t.Fatalf("unconsumed command window escaped canonical slots: %+v", result)
			}
			fixture.requireSamples(t, 3)
		})
	}
}

func TestNonSourceAccumulatorDoesNotDuplicateStoppedAuthorityCheck(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := nonSourceBlockResult{}

	boundaryFailed := result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	if !boundaryFailed {
		t.Fatal("canceled command-end boundary reported success")
	}
	result.absorbCommandEndAuthorityCheck(
		runnerAuthorityFailure(OperationProbe, CauseCanceled),
		boundaryFailed,
	)

	requireRunnerTestRecord(
		t,
		result.primary,
		PhaseAuthority,
		OperationProbe,
		CauseCanceled,
	)
	if result.later != nil || result.descriptorClose != nil || result.cleanup != nil {
		t.Fatalf("stopped authority check was counted twice: %+v", result)
	}
}

func TestNonSourceStoppedAuthorityCheckRecordsMalformedAdapterOutcome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := nonSourceBlockResult{}
	boundaryFailed := result.beginCommandEndAuthorityCheck(ctx, OperationProbe)
	result.absorbCommandEndAuthorityCheck(authorityUseOutcome{
		primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationProbe,
			Causes:    []CauseCode{CausePermission, CauseUnstable},
		},
		descriptorClose: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationCloseNonRoot,
			Causes:    []CauseCode{CauseDescriptorClose, CauseInternalInvariant},
		},
	}, boundaryFailed)

	requireRunnerTestRecord(
		t,
		result.primary,
		PhaseAuthority,
		OperationProbe,
		CauseCanceled,
	)
	requireRunnerTestRecord(
		t,
		result.later,
		PhaseAuthority,
		OperationValidate,
		CauseInternalInvariant,
	)
	if result.descriptorClose != nil || result.cleanup != nil {
		t.Fatalf("malformed stopped outcome escaped canonical slots: %+v", result)
	}
}

func TestNonSourcePrecheckLaterDoesNotConsumeBracketSlot(t *testing.T) {
	result := nonSourceBlockResult{}
	result.absorbPrecheck(authorityUseOutcome{
		primary: authorityFailure(OperationOpen, CausePermission),
		later:   authorityFailure(OperationProbe, CauseUnstable),
	})
	requireRunnerTestRecord(
		t,
		result.primary,
		PhaseAuthority,
		OperationOpen,
		CausePermission,
	)
	if result.later != nil {
		t.Fatalf("precheck consumed reserved bracket slot: %+v", result.later)
	}

	result.absorbCommandEndBoundary(authorityFailure(OperationCompare, CauseIdentity))
	requireRunnerTestRecord(
		t,
		result.later,
		PhaseAuthority,
		OperationCompare,
		CauseIdentity,
	)
}

func TestNonSourceAccumulatorPreservesValidCloseFromMalformedAuthority(t *testing.T) {
	malformed := &FailureRecord{
		Phase:     PhaseAuthority,
		Operation: OperationProbe,
		Causes:    []CauseCode{CausePermission, CauseUnstable},
	}
	closeRecord := &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationCloseNonRoot,
		Causes:    []CauseCode{CauseDescriptorClose},
	}
	outcome := authorityUseOutcome{
		primary:         malformed,
		descriptorClose: closeRecord,
	}

	t.Run("precheck", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.absorbPrecheck(outcome)
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
		requireRunnerTestRecord(
			t,
			result.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		if result.later != nil || result.cleanup != nil {
			t.Fatalf("unexpected malformed precheck slots = %+v", result)
		}
	})

	t.Run("postcheck", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.addPrimary(authorityFailure(OperationExecute, CauseChildExit))
		result.absorbPostcheck(outcome)
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationExecute,
			CauseChildExit,
		)
		requireRunnerTestRecord(
			t,
			result.later,
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
		requireRunnerTestRecord(
			t,
			result.descriptorClose,
			PhaseClose,
			OperationCloseNonRoot,
			CauseDescriptorClose,
		)
		if result.cleanup != nil {
			t.Fatalf("unexpected malformed postcheck cleanup = %+v", result.cleanup)
		}
	})
}

func TestNonSourceAccumulatorPreservesNonCloseFromMalformedAuthorityClose(t *testing.T) {
	malformedClose := &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationCloseNonRoot,
		Causes:    []CauseCode{CauseDescriptorClose, CauseInternalInvariant},
	}
	primary := authorityFailure(OperationProbe, CausePermission)
	outcome := authorityUseOutcome{
		primary:         primary,
		descriptorClose: malformedClose,
	}

	t.Run("precheck", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.absorbPrecheck(outcome)
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationProbe,
			CausePermission,
		)
		requireRunnerTestRecord(
			t,
			result.later,
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
		if result.descriptorClose != nil || result.cleanup != nil {
			t.Fatalf("malformed close displaced precheck attribution: %+v", result)
		}
	})

	t.Run("postcheck", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.addPrimary(authorityFailure(OperationExecute, CauseCanceled))
		result.absorbPostcheck(outcome)
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationExecute,
			CauseCanceled,
		)
		requireRunnerTestRecord(
			t,
			result.later,
			PhaseAuthority,
			OperationProbe,
			CausePermission,
		)
		if result.descriptorClose != nil || result.cleanup != nil {
			t.Fatalf("malformed close displaced postcheck attribution: %+v", result)
		}
	})

	t.Run("postcheck records invariant when capacity exists", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.absorbPostcheck(outcome)
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationProbe,
			CausePermission,
		)
		requireRunnerTestRecord(
			t,
			result.later,
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
		if result.descriptorClose != nil || result.cleanup != nil {
			t.Fatalf("malformed close invariant escaped its slot: %+v", result)
		}
	})

	t.Run("close-only still fails closed", func(t *testing.T) {
		result := nonSourceBlockResult{}
		result.absorbPostcheck(authorityUseOutcome{descriptorClose: malformedClose})
		requireRunnerTestRecord(
			t,
			result.primary,
			PhaseAuthority,
			OperationValidate,
			CauseInternalInvariant,
		)
		if result.later != nil || result.descriptorClose != nil || result.cleanup != nil {
			t.Fatalf("malformed close-only outcome escaped: %+v", result)
		}
	})
}

func TestNonSourceAccumulatorCanonicalizesMalformedQuiescenceAndChildren(t *testing.T) {
	primaryChild := &ChildDiagnostic{
		ExitStatusObserved: true,
		ExitStatus:         17,
		DiagnosticBytes:    11,
	}
	descriptorChild := &ChildDiagnostic{
		ExitStatusObserved: true,
		ExitStatus:         18,
		DiagnosticBytes:    12,
	}
	quiescenceChild := &ChildDiagnostic{
		ExitStatusObserved: true,
		ExitStatus:         19,
		DiagnosticBytes:    13,
	}
	result := nonSourceBlockResult{}
	result.absorbProcess(processResult{
		primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseChildExit},
			Child:     primaryChild,
		},
		descriptorClose: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationCloseNonRoot,
			Causes:    []CauseCode{CauseDescriptorClose},
			Child:     descriptorChild,
		},
		quiescence: quiescenceResult{
			causes: []CauseCode{CauseChildWait, CauseChildWait},
			child:  quiescenceChild,
		},
	})

	if result.primary == nil || result.primary.Phase != PhaseAuthority ||
		result.primary.Operation != OperationExecute ||
		!slices.Equal(result.primary.Causes, []CauseCode{CauseChildExit}) ||
		result.primary.Child == nil || result.primary.Child == primaryChild ||
		*result.primary.Child != *primaryChild {
		t.Fatalf("process primary = %+v, want cloned child-exit record", result.primary)
	}
	requireRunnerTestRecord(
		t,
		result.descriptorClose,
		PhaseClose,
		OperationCloseNonRoot,
		CauseDescriptorClose,
	)
	requireRunnerTestRecord(
		t,
		result.cleanup,
		PhaseClose,
		OperationQuiesce,
		CauseInternalInvariant,
	)
	if result.later != nil {
		t.Fatalf("malformed process result created later = %+v", result.later)
	}
	childCount := 0
	for _, record := range []*FailureRecord{
		result.primary,
		result.later,
		result.descriptorClose,
		result.cleanup,
	} {
		if record != nil && record.Child != nil {
			childCount++
		}
	}
	if childCount != 1 {
		t.Fatalf("result child records = %d, want exactly one: %+v", childCount, result)
	}
}

func TestNonSourceAccumulatorRejectsBackgroundWithoutChildWait(t *testing.T) {
	result := nonSourceBlockResult{}
	result.absorbProcess(processResult{
		quiescence: quiescenceResult{
			causes: []CauseCode{CauseChildDrain},
		},
		background: &processBackgroundReap{done: make(chan processReapResult)},
	})

	requireRunnerTestRecord(
		t,
		result.primary,
		PhaseAuthority,
		OperationValidate,
		CauseInternalInvariant,
	)
	requireRunnerTestRecord(
		t,
		result.cleanup,
		PhaseClose,
		OperationQuiesce,
		CauseInternalInvariant,
	)
	if result.later != nil || result.descriptorClose != nil || result.proof != nil {
		t.Fatalf("invalid background handoff escaped canonical slots: %+v", result)
	}
}

func TestNonSourceAccumulatorSanitizesObservedBackgroundChild(t *testing.T) {
	observed := &ChildDiagnostic{
		ExitStatusObserved: true,
		ExitStatus:         23,
		DiagnosticBytes:    7,
	}
	result := nonSourceBlockResult{}
	result.absorbProcess(processResult{
		quiescence: quiescenceResult{
			causes: []CauseCode{CauseChildWait},
			child:  observed,
		},
		background: &processBackgroundReap{done: make(chan processReapResult)},
	})

	requireRunnerTestRecord(
		t,
		result.primary,
		PhaseAuthority,
		OperationValidate,
		CauseInternalInvariant,
	)
	requireRunnerTestRecord(
		t,
		result.cleanup,
		PhaseClose,
		OperationQuiesce,
		CauseInternalInvariant,
	)
	if result.primary.Child != nil || result.cleanup.Child != nil ||
		result.later != nil || result.descriptorClose != nil || result.proof != nil {
		t.Fatalf("observed background child was not sanitized: %+v", result)
	}
}

func TestNonSourceAccumulatorPreservesValidBackgroundAbsentStatusChild(t *testing.T) {
	absentStatus := &ChildDiagnostic{
		DiagnosticBytes: 7,
	}
	result := nonSourceBlockResult{}
	result.absorbProcess(processResult{
		quiescence: quiescenceResult{
			causes: []CauseCode{CauseChildWait},
			child:  absentStatus,
		},
		background: &processBackgroundReap{done: make(chan processReapResult)},
	})

	if result.primary != nil || result.later != nil || result.descriptorClose != nil ||
		result.proof != nil {
		t.Fatalf("valid background handoff was rejected: %+v", result)
	}
	if result.cleanup == nil || result.cleanup.Phase != PhaseClose ||
		result.cleanup.Operation != OperationQuiesce ||
		!slices.Equal(result.cleanup.Causes, []CauseCode{CauseChildWait}) ||
		result.cleanup.Child == nil || result.cleanup.Child == absentStatus ||
		*result.cleanup.Child != *absentStatus {
		t.Fatalf("valid background cleanup = %+v, want cloned absent-status child", result.cleanup)
	}
}

func TestNonSourceAccumulatorRejectsDisallowedBackgroundPrimary(t *testing.T) {
	absentStatus := func() *ChildDiagnostic {
		return &ChildDiagnostic{DiagnosticBytes: 7}
	}
	for _, test := range []struct {
		name          string
		operation     Operation
		cause         CauseCode
		child         *ChildDiagnostic
		wantCanonical bool
	}{
		{
			name:          "open with child",
			operation:     OperationOpen,
			cause:         CausePermission,
			child:         absentStatus(),
			wantCanonical: true,
		},
		{
			name:          "probe with child",
			operation:     OperationProbe,
			cause:         CauseUnstable,
			child:         absentStatus(),
			wantCanonical: true,
		},
		{
			name:          "child start with child",
			operation:     OperationExecute,
			cause:         CauseChildStart,
			child:         absentStatus(),
			wantCanonical: true,
		},
		{
			name:      "open",
			operation: OperationOpen,
			cause:     CausePermission,
		},
		{
			name:      "probe",
			operation: OperationProbe,
			cause:     CauseUnstable,
		},
		{
			name:      "child start",
			operation: OperationExecute,
			cause:     CauseChildStart,
		},
		{
			name:          "child exit",
			operation:     OperationExecute,
			cause:         CauseChildExit,
			wantCanonical: true,
		},
		{
			name:          "parse",
			operation:     OperationParse,
			cause:         CauseMalformed,
			wantCanonical: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primary := &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: test.operation,
				Causes:    []CauseCode{test.cause},
				Child:     test.child,
			}
			process := processResult{
				primary: primary,
				quiescence: quiescenceResult{
					causes: []CauseCode{CauseChildWait},
				},
				background: &processBackgroundReap{done: make(chan processReapResult)},
			}
			if validNonSourceBackgroundPrimary(primary) ||
				validNonSourceProcessResult(process) {
				t.Fatalf("disallowed background primary was admitted: %+v", primary)
			}

			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			if test.wantCanonical {
				requireRunnerTestRecord(
					t,
					result.primary,
					PhaseAuthority,
					OperationValidate,
					CauseInternalInvariant,
				)
			} else {
				requireRunnerTestRecord(
					t,
					result.primary,
					PhaseAuthority,
					test.operation,
					test.cause,
				)
			}
			requireRunnerTestRecord(
				t,
				result.cleanup,
				PhaseClose,
				OperationQuiesce,
				CauseInternalInvariant,
			)
			if result.primary.Child != nil || result.cleanup.Child != nil ||
				result.later != nil || result.descriptorClose != nil || result.proof != nil {
				t.Fatalf("disallowed background primary escaped canonicalization: %+v", result)
			}
		})
	}
}

func TestNonSourceAccumulatorAcceptsAllowedBackgroundPrimary(t *testing.T) {
	absentStatus := &ChildDiagnostic{DiagnosticBytes: 7}
	for _, test := range []struct {
		name      string
		operation Operation
		cause     CauseCode
		child     *ChildDiagnostic
	}{
		{
			name:      "validate internal with absent status",
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
			child:     absentStatus,
		},
		{
			name:      "canceled",
			operation: OperationExecute,
			cause:     CauseCanceled,
			child:     absentStatus,
		},
		{
			name:      "deadline",
			operation: OperationExecute,
			cause:     CauseDeadline,
			child:     absentStatus,
		},
		{
			name:      "limit",
			operation: OperationExecute,
			cause:     CauseLimit,
			child:     absentStatus,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			primary := &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: test.operation,
				Causes:    []CauseCode{test.cause},
				Child:     test.child,
			}
			process := processResult{
				primary: primary,
				quiescence: quiescenceResult{
					causes: []CauseCode{CauseChildWait},
				},
				background: &processBackgroundReap{done: make(chan processReapResult)},
			}
			if !validNonSourceBackgroundPrimary(primary) ||
				!validNonSourceProcessResult(process) {
				t.Fatalf("allowed background primary was rejected: %+v", primary)
			}

			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			if result.primary == nil || result.primary.Phase != PhaseAuthority ||
				result.primary.Operation != test.operation ||
				!slices.Equal(result.primary.Causes, []CauseCode{test.cause}) {
				t.Fatalf("allowed background primary = %+v", result.primary)
			}
			if test.child == nil {
				if result.primary.Child != nil {
					t.Fatalf("allowed background primary gained child = %+v", result.primary.Child)
				}
			} else if result.primary.Child == nil || result.primary.Child == test.child ||
				*result.primary.Child != *test.child || result.primary.Child.ExitStatusObserved {
				t.Fatalf("allowed background child = %+v, want cloned absent status", result.primary.Child)
			}
			requireRunnerTestRecord(
				t,
				result.cleanup,
				PhaseClose,
				OperationQuiesce,
				CauseChildWait,
			)
			if result.later != nil || result.descriptorClose != nil || result.proof != nil {
				t.Fatalf("allowed background primary gained slots: %+v", result)
			}
		})
	}
}

func TestNonSourceAccumulatorAcceptsObservedValidateChildSynchronously(t *testing.T) {
	observed := &ChildDiagnostic{
		ExitStatusObserved: true,
		ExitStatus:         23,
		DiagnosticBytes:    7,
	}
	process := processResult{
		primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseInternalInvariant},
			Child:     observed,
		},
		quiescence: quiescenceResult{proven: true},
	}
	if !validNonSourceProcessResult(process) {
		t.Fatal("synchronous validate/internal child was rejected")
	}

	result := nonSourceBlockResult{}
	result.absorbProcess(process)
	if result.primary == nil || result.primary.Phase != PhaseAuthority ||
		result.primary.Operation != OperationValidate ||
		!slices.Equal(result.primary.Causes, []CauseCode{CauseInternalInvariant}) ||
		result.primary.Child == nil || result.primary.Child == observed ||
		*result.primary.Child != *observed || !result.primary.Child.ExitStatusObserved {
		t.Fatalf("synchronous validate child = %+v, want cloned observed status", result.primary)
	}
	if result.later != nil || result.descriptorClose != nil || result.cleanup != nil ||
		result.proof != nil {
		t.Fatalf("synchronous validate child gained slots: %+v", result)
	}
}

func TestNonSourceAccumulatorRejectsImpossiblePrimaryChildEvidence(t *testing.T) {
	for _, test := range []struct {
		name    string
		primary *FailureRecord
	}{
		{
			name: "child exit status zero",
			primary: &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: OperationExecute,
				Causes:    []CauseCode{CauseChildExit},
				Child:     runnerObservedDiagnosticChild(0, 0),
			},
		},
		{
			name: "parse status nonzero",
			primary: &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: OperationParse,
				Causes:    []CauseCode{CauseMalformed},
				Child:     runnerObservedDiagnosticChild(17, 7),
			},
		},
		{
			name: "parse without diagnostics",
			primary: &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: OperationParse,
				Causes:    []CauseCode{CauseMalformed},
				Child:     runnerObservedDiagnosticChild(0, 0),
			},
		},
		{
			name: "unreachable bracketed input failure",
			primary: &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: OperationExecute,
				Causes:    []CauseCode{CauseUnstable},
			},
		},
		{
			name: "malformed empty causes cannot panic",
			primary: &FailureRecord{
				Phase:     PhaseAuthority,
				Operation: OperationExecute,
				Child:     runnerObservedDiagnosticChild(17, 7),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := processResult{
				primary:    test.primary,
				quiescence: quiescenceResult{proven: true},
			}
			if validNonSourceProcessResult(process) {
				t.Fatal("impossible primary/Child evidence was admitted")
			}
			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				OperationValidate,
				CauseInternalInvariant,
			)
			if result.primary.Child != nil || result.later != nil ||
				result.descriptorClose != nil || result.cleanup != nil {
				t.Fatalf("impossible primary/Child evidence escaped: %+v", result)
			}
		})
	}
}

func TestNonSourceAccumulatorRejectsSynchronousAbsentStatus(t *testing.T) {
	for _, owner := range []string{"primary", "descriptor", "cleanup"} {
		t.Run(owner, func(t *testing.T) {
			child := &ChildDiagnostic{DiagnosticBytes: 7}
			process := processResult{quiescence: quiescenceResult{proven: true}}
			switch owner {
			case "primary":
				process.primary = &FailureRecord{
					Phase:     PhaseAuthority,
					Operation: OperationExecute,
					Causes:    []CauseCode{CauseCanceled},
					Child:     child,
				}
			case "descriptor":
				process.descriptorClose = &FailureRecord{
					Phase:     PhaseClose,
					Operation: OperationCloseNonRoot,
					Causes:    []CauseCode{CauseDescriptorClose},
					Child:     child,
				}
			case "cleanup":
				process.quiescence = quiescenceResult{
					causes: []CauseCode{CauseChildWait},
					child:  child,
				}
			}
			if validNonSourceProcessResult(process) {
				t.Fatal("synchronous absent-status Child was admitted")
			}
			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			for _, record := range []*FailureRecord{
				result.primary,
				result.later,
				result.descriptorClose,
				result.cleanup,
			} {
				if record != nil && record.Child != nil && !record.Child.ExitStatusObserved {
					t.Fatalf("absent-status Child escaped on %s: %+v", owner, result)
				}
			}
		})
	}
}

func TestNonSourceAccumulatorPreservesIntentionalReadStopChild(t *testing.T) {
	for _, owner := range []string{"descriptor", "cleanup"} {
		t.Run(owner, func(t *testing.T) {
			child := runnerObservedDiagnosticChild(nonSourceIntentionalKillExit, 7)
			process := processResult{
				quiescence: quiescenceResult{causes: []CauseCode{CauseChildDrain}},
			}
			if owner == "descriptor" {
				process.descriptorClose = &FailureRecord{
					Phase:     PhaseClose,
					Operation: OperationCloseNonRoot,
					Causes:    []CauseCode{CauseDescriptorClose},
					Child:     child,
				}
			} else {
				process.quiescence.child = child
			}
			if !validNonSourceProcessResult(process) {
				t.Fatal("valid intentional read-stop Child was rejected")
			}
			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			if result.primary != nil {
				t.Fatalf("valid intentional read-stop gained Primary: %+v", result.primary)
			}
			var got *ChildDiagnostic
			if owner == "descriptor" {
				got = result.descriptorClose.Child
			} else {
				got = result.cleanup.Child
			}
			if got == nil || got == child || *got != *child {
				t.Fatalf("intentional read-stop Child = %+v, want clone of %+v", got, child)
			}
		})
	}
}

func TestNonSourceAccumulatorDoesNotLaunderMalformedChildCarrier(t *testing.T) {
	malformed := &ChildDiagnostic{Truncated: true}
	process := processResult{
		primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseCanceled},
		},
		quiescence: quiescenceResult{
			causes: []CauseCode{CauseChildWait},
			child:  malformed,
		},
	}
	result := nonSourceBlockResult{}
	result.absorbProcess(process)
	if !result.processInvariant {
		t.Fatal("malformed Child carrier did not retain its private invariant marker")
	}
	requireRunnerTestRecord(
		t,
		result.primary,
		PhaseAuthority,
		OperationExecute,
		CauseCanceled,
	)
	requireRunnerTestRecord(
		t,
		result.cleanup,
		PhaseClose,
		OperationQuiesce,
		CauseInternalInvariant,
	)
	if result.primary.Child != nil || result.cleanup.Child != nil {
		t.Fatalf("malformed donor Child escaped normalization: %+v", result)
	}
}

func TestNonSourceAccumulatorRequiresValidBackgroundForAbsentStatus(t *testing.T) {
	child := &ChildDiagnostic{DiagnosticBytes: 7}
	process := processResult{
		primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseCanceled},
			Child:     child,
		},
		quiescence: quiescenceResult{causes: []CauseCode{CauseChildWait}},
		background: &processBackgroundReap{},
	}
	if validNonSourceProcessResult(process) {
		t.Fatal("invalid background lent absent-status authority")
	}
	result := nonSourceBlockResult{}
	result.absorbProcess(process)
	requireRunnerTestRecord(
		t,
		result.primary,
		PhaseAuthority,
		OperationExecute,
		CauseCanceled,
	)
	requireRunnerTestRecord(
		t,
		result.cleanup,
		PhaseClose,
		OperationQuiesce,
		CauseInternalInvariant,
	)
	if result.primary.Child != nil || result.cleanup.Child != nil {
		t.Fatalf("invalid background Child escaped: %+v", result)
	}
}

func TestProcessChildDiagnosticRequiresExactEmptyDigest(t *testing.T) {
	wrong := &ChildDiagnostic{ExitStatusObserved: true}
	if validProcessChildDiagnostic(wrong) {
		t.Fatal("zero-byte Child with a non-canonical digest was admitted")
	}
	if !validProcessChildDiagnostic(runnerStructuredOutputChild()) {
		t.Fatal("canonical zero-byte Child was rejected")
	}
}

func TestNonSourceAccumulatorRejectsClosedProcessPrimaryPairs(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "invalid request cause",
			operation: OperationValidate,
			cause:     CauseInvalidRequest,
		},
		{
			name:      "descriptor close cause on open",
			operation: OperationOpen,
			cause:     CauseDescriptorClose,
		},
		{
			name:      "descriptor close primary slot",
			operation: OperationCloseNonRoot,
			cause:     CauseDescriptorClose,
		},
		{
			name:      "bounded input failure unreachable",
			operation: OperationExecute,
			cause:     CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if validNonSourceProcessPrimaryPair(test.operation, test.cause) {
				t.Fatalf("process primary pair %s/%s was admitted", test.operation, test.cause)
			}
			result := nonSourceBlockResult{}
			result.absorbProcess(processResult{
				primary: &FailureRecord{
					Phase:     PhaseAuthority,
					Operation: test.operation,
					Causes:    []CauseCode{test.cause},
				},
				quiescence: quiescenceResult{proven: true},
			})
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				OperationValidate,
				CauseInternalInvariant,
			)
			if result.later != nil || result.descriptorClose != nil || result.cleanup != nil {
				t.Fatalf("invalid process primary escaped its slot: %+v", result)
			}
		})
	}
}

func TestNonSourceAccumulatorRejectsPreStartChildDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation Operation
		cause     CauseCode
	}{
		{name: "pipe setup", operation: OperationOpen, cause: CausePermission},
		{name: "capability probe", operation: OperationProbe, cause: CauseUnstable},
		{name: "start failure", operation: OperationExecute, cause: CauseChildStart},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := processResult{
				primary: &FailureRecord{
					Phase:     PhaseAuthority,
					Operation: test.operation,
					Causes:    []CauseCode{test.cause},
					Child:     &ChildDiagnostic{},
				},
				quiescence: quiescenceResult{proven: true},
			}
			if validNonSourceProcessResult(process) {
				t.Fatal("pre-Start process failure admitted a Child diagnostic")
			}
			result := nonSourceBlockResult{}
			result.absorbProcess(process)
			requireRunnerTestRecord(
				t,
				result.primary,
				PhaseAuthority,
				OperationValidate,
				CauseInternalInvariant,
			)
			if result.later != nil || result.descriptorClose != nil || result.cleanup != nil {
				t.Fatalf("pre-Start child escaped sanitized process result: %+v", result)
			}
		})
	}
}

func runnerAuthorityFailure(operation Operation, cause CauseCode) authorityUseOutcome {
	return authorityUseOutcome{primary: authorityFailure(operation, cause)}
}

func runnerStructuredOutputChild() *ChildDiagnostic {
	return runnerObservedDiagnosticChild(0, 0)
}

func runnerObservedDiagnosticChild(
	status int,
	diagnosticBytes uint64,
) *ChildDiagnostic {
	child := &ChildDiagnostic{
		ExitStatusObserved: true,
		ExitStatus:         status,
		DiagnosticBytes:    diagnosticBytes,
		Truncated:          diagnosticBytes > processDiagnosticPrefixBytes,
	}
	if diagnosticBytes == 0 {
		child.DiagnosticSHA256 = Digest(sha256.Sum256(nil))
	}
	return child
}

func runnerProcessFailure(operation Operation, cause CauseCode) processResult {
	primary := authorityFailure(operation, cause)
	if operation == OperationExecute && cause == CauseChildExit {
		primary.Child = &ChildDiagnostic{
			ExitStatusObserved: true,
			ExitStatus:         1,
			DiagnosticSHA256:   Digest(sha256.Sum256(nil)),
		}
	}
	return processResult{
		primary:    primary,
		quiescence: quiescenceResult{proven: true},
	}
}

func requireRunnerTestRecord(
	t *testing.T,
	record *FailureRecord,
	phase Phase,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	requireRunnerTestRecordWithChild(t, record, phase, operation, cause, nil)
}

func requireRunnerTestRecordWithChild(
	t *testing.T,
	record *FailureRecord,
	phase Phase,
	operation Operation,
	cause CauseCode,
	child *ChildDiagnostic,
) {
	t.Helper()
	if record == nil || record.Phase != phase || record.Operation != operation ||
		len(record.Causes) != 1 || record.Causes[0] != cause {
		t.Fatalf("record = %+v, want %s/%s/%s", record, phase, operation, cause)
	}
	if child == nil {
		if record.Child != nil {
			t.Fatalf("record child = %+v, want nil", record.Child)
		}
		return
	}
	if record.Child == nil || record.Child == child || *record.Child != *child {
		t.Fatalf("record child = %+v, want clone of %+v", record.Child, child)
	}
}

func requireRunnerTestNoOtherSlots(
	t *testing.T,
	result nonSourceBlockResult,
	wantProof bool,
) {
	t.Helper()
	if result.later != nil || result.descriptorClose != nil || result.cleanup != nil ||
		(result.proof != nil) != wantProof {
		t.Fatalf("unexpected runner result slots = %+v", result)
	}
}

func requireRunnerTestTrace(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("runner trace:\n got %q\nwant %q", got, want)
	}
}

func runnerBoundaryBeforeFirstPrecheckTrace() []string {
	return []string{
		"authority:root#1",
		"authority:null#1",
		"authority:sentinels-admit#1",
		"authority:root#2",
		"authority:null#2",
		"authority:sentinels-revalidate#1",
		"authority:go#1",
		"authority:compiler#1",
		"authority:git#1",
	}
}

func runnerBoundaryBeforeGoRolePrecheckTrace() []string {
	return []string{
		"authority:root#1",
		"authority:null#1",
		"authority:sentinels-admit#1",
		"authority:root#2",
		"authority:null#2",
		"authority:goroot#1",
		"authority:go-environment#1",
		"authority:root#3",
		"authority:null#3",
		"authority:sentinels-revalidate#1",
		"authority:go#1",
		"authority:compiler#1",
		"authority:git#1",
	}
}

func runnerFirstRowFailureTrace() []string {
	return []string{
		"authority:root#1",
		"authority:null#1",
		"authority:sentinels-admit#1",
		"authority:root#2",
		"authority:null#2",
		"authority:goroot#1",
		"authority:go-environment#1",
		"authority:go#1",
		"process:go-version#1",
		"authority:root#3",
		"authority:null#3",
		"authority:goroot#2",
		"authority:go-environment#2",
		"authority:go#2",
		"authority:root#4",
		"authority:null#4",
		"authority:sentinels-revalidate#1",
		"authority:go#3",
		"authority:compiler#1",
		"authority:git#1",
	}
}

func runnerAllSuccessTrace() []string {
	return []string{
		"authority:root#1",
		"authority:null#1",
		"authority:sentinels-admit#1",
		"authority:root#2",
		"authority:null#2",
		"authority:goroot#1",
		"authority:go-environment#1",
		"authority:go#1",
		"process:go-version#1",
		"authority:root#3",
		"authority:null#3",
		"authority:goroot#2",
		"authority:go-environment#2",
		"authority:go#2",
		"authority:root#4",
		"authority:null#4",
		"authority:goroot#3",
		"authority:go-environment#3",
		"authority:go#3",
		"process:go-environment#1",
		"authority:root#5",
		"authority:null#5",
		"authority:goroot#4",
		"authority:go-environment#4",
		"authority:go#4",
		"authority:root#6",
		"authority:null#6",
		"authority:goroot#5",
		"authority:compiler#1",
		"process:compiler-version#1",
		"authority:root#7",
		"authority:null#7",
		"authority:goroot#6",
		"authority:compiler#2",
		"authority:root#8",
		"authority:null#8",
		"authority:git#1",
		"process:git-version#1",
		"authority:root#9",
		"authority:null#9",
		"authority:git#2",
		"authority:root#10",
		"authority:null#10",
		"authority:git#3",
		"process:git-builtins#1",
		"authority:root#11",
		"authority:null#11",
		"authority:git#4",
		"authority:root#12",
		"authority:null#12",
		"authority:sentinels-revalidate#1",
		"authority:go#5",
		"authority:compiler#3",
		"authority:git#5",
	}
}
