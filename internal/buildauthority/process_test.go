package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestProcessPolicyConstantsMatchClosedContract(t *testing.T) {
	if processMaxStructuredOutputBytes != 256<<20 || processDiagnosticPrefixBytes != 1<<20 {
		t.Fatal("process byte ceilings drifted")
	}
	if processGroupMemberLimit != 4_096 || processSurvivorSnapshotLimit != 1_000 {
		t.Fatal("process inventory ceilings drifted")
	}
	if processPollInterval != 5*time.Millisecond || processExitObservationWindow != 5*time.Second ||
		processPipeCompletionWindow != time.Second {
		t.Fatal("process time ceilings drifted")
	}
}

func TestValidateProcessRequestRequiresClosedInputs(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	valid := processRequest{
		ctx:           context.Background(),
		phase:         PhaseSource,
		executable:    "/usr/bin/git",
		directory:     "/private/tmp/task",
		input:         newBoundedProcessInput(nil),
		stdoutLimit:   processMaxStructuredOutputBytes,
		phaseDeadline: deadline,
	}
	if failure := validateProcessRequest(valid); failure != nil {
		t.Fatalf("valid request refused: %+v", failure)
	}

	tests := []struct {
		name   string
		mutate func(*processRequest)
	}{
		{name: "nil context", mutate: func(request *processRequest) { request.ctx = nil }},
		{name: "nil input", mutate: func(request *processRequest) { request.input = nil }},
		{name: "zero input tag", mutate: func(request *processRequest) { request.input = &processInput{} }},
		{name: "unknown input tag", mutate: func(request *processRequest) {
			request.input = &processInput{kind: processInputKind(255)}
		}},
		{name: "missing retained null", mutate: func(request *processRequest) {
			request.input = newRetainedNullProcessInput(nil)
		}},
		{name: "retained null with bounded payload", mutate: func(request *processRequest) {
			request.input = &processInput{
				kind:         processInputRetainedNull,
				retainedNull: &retainedNullDevice{},
				bounded:      []byte{},
			}
		}},
		{name: "bounded input with retained null", mutate: func(request *processRequest) {
			request.input = &processInput{
				kind:         processInputBounded,
				retainedNull: &retainedNullDevice{},
			}
		}},
		{name: "invalid phase", mutate: func(request *processRequest) { request.phase = Phase("other") }},
		{name: "relative executable", mutate: func(request *processRequest) { request.executable = "git" }},
		{name: "unclean executable", mutate: func(request *processRequest) { request.executable = "/usr/bin/../bin/git" }},
		{name: "relative directory", mutate: func(request *processRequest) { request.directory = "task" }},
		{name: "zero stdout limit", mutate: func(request *processRequest) { request.stdoutLimit = 0 }},
		{name: "oversized stdout limit", mutate: func(request *processRequest) { request.stdoutLimit++ }},
		{name: "no deadline", mutate: func(request *processRequest) { request.phaseDeadline = time.Time{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			failure := validateProcessRequest(request)
			if failure == nil || failure.Operation != OperationValidate || len(failure.Causes) != 1 ||
				failure.Causes[0] != CauseInternalInvariant {
				t.Fatalf("failure = %+v, want validate/internal-invariant", failure)
			}
		})
	}
}

func TestProcessStopSamplingUsesFixedPrecedence(t *testing.T) {
	base := time.Unix(1_000, 0)
	request := processRequest{
		ctx:           context.Background(),
		phaseDeadline: base.Add(time.Hour),
	}
	notifications := newProcessNotifications()
	notifications.publish(processStopRead)
	notifications.publish(processStopInput)
	notifications.publish(processStopOutputLimit)
	if got := notifications.sample(&request, base).kind; got != processStopOutputLimit {
		t.Fatalf("worker precedence = %d, want output limit", got)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	request.ctx = canceled
	if got := notifications.sample(&request, base).kind; got != processStopCanceled {
		t.Fatalf("cancel precedence = %d, want canceled", got)
	}
	request.phaseDeadline = base
	if got := notifications.sample(&request, base).kind; got != processStopDeadline {
		t.Fatalf("deadline precedence = %d, want deadline", got)
	}

	workerTests := []struct {
		name string
		kind processStopKind
	}{
		{name: "output limit", kind: processStopOutputLimit},
		{name: "input", kind: processStopInput},
		{name: "read", kind: processStopRead},
	}
	for _, test := range workerTests {
		t.Run(test.name, func(t *testing.T) {
			one := newProcessNotifications()
			one.publish(test.kind)
			if got := one.sampleWorkers().kind; got != test.kind {
				t.Fatalf("sample = %d, want %d", got, test.kind)
			}
		})
	}
}

func TestProcessStopSamplesContextErrorExactlyOnce(t *testing.T) {
	ctx := &changingErrorContext{errors: []error{context.DeadlineExceeded, context.Canceled}}
	request := processRequest{ctx: ctx, phaseDeadline: time.Now().Add(time.Hour)}
	if got := newProcessNotifications().sample(&request, time.Now()).kind; got != processStopDeadline {
		t.Fatalf("stop = %d, want deadline", got)
	}
	if ctx.calls != 1 {
		t.Fatalf("context Err calls = %d, want exactly 1", ctx.calls)
	}
}

func TestProcessPollDelayIsNeverProgrammedPastFiveMilliseconds(t *testing.T) {
	now := time.Unix(10, 0)
	if got := processPollDelay(now); got != 5*time.Millisecond {
		t.Fatalf("unbounded delay = %s", got)
	}
	if got := processPollDelay(now, now.Add(2*time.Millisecond)); got != 2*time.Millisecond {
		t.Fatalf("early deadline delay = %s", got)
	}
	if got := processPollDelay(now, now); got != 0 {
		t.Fatalf("elapsed deadline delay = %s", got)
	}
}

func TestProcessReadersBoundStructuredOutputAndDiagnostics(t *testing.T) {
	notifications := newProcessNotifications()
	stdoutEnd := &memoryProcessEnd{reader: bytes.NewReader([]byte("abcd"))}
	stdout := <-startProcessStdoutReader(
		ownProcessEnd(stdoutEnd),
		3,
		notifications,
	)
	if string(stdout.bytes) != "abc" || !stdout.eof || stdout.readFailed {
		t.Fatalf("stdout = %+v", stdout)
	}
	if got := notifications.sampleWorkers().kind; got != processStopOutputLimit {
		t.Fatalf("limit stop = %d, want output limit", got)
	}
	if stdoutEnd.closeCount != 1 {
		t.Fatalf("stdout closes = %d, want 1", stdoutEnd.closeCount)
	}

	diagnosticBytes := bytes.Repeat([]byte{0xa5}, processDiagnosticPrefixBytes+1)
	stderrEnd := &memoryProcessEnd{reader: bytes.NewReader(diagnosticBytes)}
	stderr := <-startProcessStderrReader(ownProcessEnd(stderrEnd), newProcessNotifications())
	if len(stderr.prefix) != processDiagnosticPrefixBytes || stderr.total != uint64(len(diagnosticBytes)) ||
		!stderr.eof || stderr.readFailed {
		t.Fatalf("stderr = prefix %d total %d eof %v failed %v", len(stderr.prefix), stderr.total, stderr.eof, stderr.readFailed)
	}
	terminal := &processTerminal{class: processTerminalKilled, status: 9}
	diagnostic := childDiagnosticFromProcess(stderr, terminal)
	wantDigest := sha256.Sum256(diagnosticBytes[:processDiagnosticPrefixBytes])
	if !diagnostic.ExitStatusObserved || diagnostic.ExitStatus != -9 || !diagnostic.Truncated ||
		diagnostic.DiagnosticBytes != uint64(len(diagnosticBytes)) || diagnostic.DiagnosticSHA256 != Digest(wantDigest) {
		t.Fatalf("diagnostic = %+v", diagnostic)
	}
}

func TestProcessStdoutPublishesCrossingLimitBeforeSameReadFailure(t *testing.T) {
	notifications := newProcessNotifications()
	result := <-startProcessStdoutReader(
		ownProcessEnd(&memoryProcessEnd{
			reader:          bytes.NewReader([]byte("abcd")),
			readErr:         errors.New("same-read failure"),
			readErrWithData: true,
		}),
		3,
		notifications,
	)
	if result.eof || !result.readFailed || string(result.bytes) != "abc" {
		t.Fatalf("stdout result = %+v", result)
	}
	if got := notifications.sampleWorkers().kind; got != processStopOutputLimit {
		t.Fatalf("same-read limit/read precedence = %d, want output limit", got)
	}
}

func TestProcessInputWriterClonesBeforeAsyncUse(t *testing.T) {
	attempted := make(chan struct{})
	release := make(chan struct{})
	end := &memoryProcessEnd{writeAttempt: attempted, writeRelease: release}
	input := []byte("immutable-input")
	done := startProcessInputWriter(ownProcessEnd(end), input, make(chan struct{}), newProcessNotifications())
	<-attempted
	copy(input, []byte("mutated-input!!"))
	close(release)
	result := <-done
	if result.writeFailed || end.written.String() != "immutable-input" {
		t.Fatalf("writer result=%+v bytes=%q", result, end.written.String())
	}
}

func TestBoundedProcessInputClonesAtConstructionAndResolution(t *testing.T) {
	original := []byte("constructor-clone")
	input := newBoundedProcessInput(original)
	copy(original, []byte("caller-mutates!!"))
	resolved, err := resolveProcessInput(context.Background(), input)
	if err != nil {
		t.Fatal("bounded input did not resolve")
	}
	copy(input.bounded, []byte("plan-mutated!!!!"))
	if string(resolved.bounded) != "constructor-clone" {
		t.Fatalf("resolved bytes = %q", resolved.bounded)
	}
}

func TestProcessProductionSourceHasNoExecutableOutputSeam(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate process test source")
	}
	sourceDirectory := filepath.Dir(testFile)
	for _, name := range []string{"process.go", "process_darwin.go", "process_wait_darwin.go", "process_unsupported.go"} {
		sourcePath := filepath.Join(sourceDirectory, name)
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, forbidden := range []string{"func([]byte)", "callProcessStream", "streamStop"} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("%s contains executable output seam %q", name, forbidden)
			}
		}
	}

	requestType := reflect.TypeFor[processRequest]()
	wantFields := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "ctx", typeOf: reflect.TypeFor[context.Context]()},
		{name: "phase", typeOf: reflect.TypeFor[Phase]()},
		{name: "executable", typeOf: reflect.TypeFor[string]()},
		{name: "arguments", typeOf: reflect.TypeFor[[]string]()},
		{name: "directory", typeOf: reflect.TypeFor[string]()},
		{name: "input", typeOf: reflect.TypeFor[*processInput]()},
		{name: "stdoutLimit", typeOf: reflect.TypeFor[uint64]()},
		{name: "phaseDeadline", typeOf: reflect.TypeFor[time.Time]()},
		{name: "callerDeadline", typeOf: reflect.TypeFor[time.Time]()},
		{name: "transactionDeadline", typeOf: reflect.TypeFor[time.Time]()},
	}
	inputType := reflect.TypeFor[processInput]()
	if inputType.Kind() != reflect.Struct || inputType.NumField() != 4 {
		t.Fatalf("compiled processInput shape = %s with %d fields", inputType, inputType.NumField())
	}
	for _, name := range []string{"kind", "retainedNull", "bracketedNull", "bounded"} {
		field, ok := inputType.FieldByName(name)
		if !ok || field.PkgPath == "" || field.Anonymous || field.Type.Kind() == reflect.Func ||
			field.Type.Kind() == reflect.Interface {
			t.Fatalf("compiled processInput field %q is missing or unsealed", name)
		}
	}
	for _, sealedType := range []reflect.Type{
		reflect.TypeFor[resolvedProcessInput](),
		reflect.TypeFor[retainedNullProcessBorrow](),
	} {
		for field := range sealedType.Fields() {
			if field.Anonymous || field.PkgPath == "" || field.Type.Kind() == reflect.Func ||
				field.Type.Kind() == reflect.Interface {
				t.Fatalf("sealed process-input type %s field %s exposes an open seam", sealedType, field.Name)
			}
		}
	}
	if requestType.Kind() != reflect.Struct || requestType.NumField() != len(wantFields) {
		t.Fatalf("compiled processRequest shape = %s with %d fields, want closed %d-field struct",
			requestType, requestType.NumField(), len(wantFields))
	}
	for index, want := range wantFields {
		field := requestType.Field(index)
		if field.Name != want.name || field.Type != want.typeOf || field.Anonymous || field.PkgPath == "" {
			t.Fatalf("compiled processRequest field %d = %s %s anonymous=%v exported=%v, want %s %s unexported",
				index, field.Name, field.Type, field.Anonymous, field.PkgPath == "", want.name, want.typeOf)
		}
		if field.Type.Kind() == reflect.Func {
			t.Fatalf("compiled processRequest field %q is executable, including through a named alias", field.Name)
		}
		if field.Type.Kind() == reflect.Interface && field.Name != "ctx" {
			t.Fatalf("compiled processRequest field %q is an open interface, including through a named alias", field.Name)
		}
	}

	supervisorType := reflect.TypeFor[processSupervisor]()
	for _, entry := range []struct {
		name        string
		environment reflect.Type
	}{
		{name: "runPreallocation", environment: reflect.TypeFor[preallocationEnvironment]()},
		{name: "runTaskPrivate", environment: reflect.TypeFor[taskPrivateEnvironment]()},
	} {
		method, ok := supervisorType.MethodByName(entry.name)
		if !ok {
			t.Fatalf("process supervisor is missing typed entry %s", entry.name)
		}
		if method.Type.NumIn() != 2 || method.Type.In(0) != reflect.PointerTo(requestType) ||
			method.Type.In(1) != entry.environment || method.Type.NumOut() != 1 ||
			method.Type.Out(0) != reflect.TypeFor[processResult]() {
			t.Fatalf("process supervisor entry %s has type %s", entry.name, method.Type)
		}
	}
	if reflect.TypeFor[preallocationEnvironment]() == reflect.TypeFor[taskPrivateEnvironment]() {
		t.Fatal("preallocation and task-private environments are interchangeable")
	}
	for _, profile := range []reflect.Type{
		reflect.TypeFor[preallocationEnvironment](),
		reflect.TypeFor[taskPrivateEnvironment](),
	} {
		if profile.Kind() != reflect.Struct || profile.Name() == "" {
			t.Fatalf("environment profile %s is not a concrete nominal struct", profile)
		}
		for field := range profile.Fields() {
			if field.Anonymous || field.PkgPath == "" || field.Type.Kind() == reflect.Func ||
				field.Type.Kind() == reflect.Interface || field.Type == reflect.TypeFor[[]string]() {
				t.Fatalf("environment profile %s field %s exposes an unsealed seam", profile, field.Name)
			}
		}
	}

	commandSpecType := reflect.TypeFor[processCommandSpec]()
	wantCommandFields := []string{"executable", "arguments", "directory"}
	if commandSpecType.NumField() != len(wantCommandFields) {
		t.Fatalf("process command spec fields = %d, want %d with no environment field",
			commandSpecType.NumField(), len(wantCommandFields))
	}
	for index, want := range wantCommandFields {
		if field := commandSpecType.Field(index); field.Name != want || field.PkgPath == "" {
			t.Fatalf("process command spec field %d = %s exported=%v, want %s unexported",
				index, field.Name, field.PkgPath == "", want)
		}
	}
	for _, factory := range []struct {
		name        string
		typeOf      reflect.Type
		environment reflect.Type
	}{
		{
			name:        "preallocation",
			typeOf:      reflect.TypeFor[preallocationProcessCommandFactory](),
			environment: reflect.TypeFor[preallocationEnvironment](),
		},
		{
			name:        "task private",
			typeOf:      reflect.TypeFor[taskPrivateProcessCommandFactory](),
			environment: reflect.TypeFor[taskPrivateEnvironment](),
		},
	} {
		if factory.typeOf.Kind() != reflect.Func || factory.typeOf.NumIn() != 2 ||
			factory.typeOf.In(0) != commandSpecType || factory.typeOf.In(1) != factory.environment ||
			factory.typeOf.NumOut() != 1 || factory.typeOf.Out(0) != reflect.TypeFor[processCommand]() {
			t.Fatalf("%s command factory has forgeable shape %s", factory.name, factory.typeOf)
		}
	}
}

func TestProcessDescriptorFailureClosesEveryEndExactlyOnce(t *testing.T) {
	closeFailure := errors.New("close")
	first := &memoryProcessEnd{reader: bytes.NewReader(nil), closeErr: closeFailure}
	second := &memoryProcessEnd{reader: bytes.NewReader(nil)}
	record := processDescriptorFailure(ownProcessEnd(first), ownProcessEnd(second))
	if record == nil || record.Phase != PhaseClose || record.Operation != OperationCloseNonRoot ||
		len(record.Causes) != 1 || record.Causes[0] != CauseDescriptorClose {
		t.Fatalf("record = %+v", record)
	}
	if first.closeCount != 1 || second.closeCount != 1 {
		t.Fatalf("close counts = %d, %d", first.closeCount, second.closeCount)
	}
}

func TestAttachProcessChildUsesEarliestRecordOnly(t *testing.T) {
	result := processResult{
		primary:         &FailureRecord{Phase: PhaseSource, Operation: OperationExecute, Causes: []CauseCode{CauseChildExit}},
		descriptorClose: &FailureRecord{Phase: PhaseClose, Operation: OperationCloseNonRoot, Causes: []CauseCode{CauseDescriptorClose}},
		quiescence:      quiescenceResult{causes: []CauseCode{CauseChildWait}},
	}
	child := &ChildDiagnostic{ExitStatusObserved: true, ExitStatus: 1}
	attachProcessChild(&result, child)
	if result.primary.Child != child || result.descriptorClose.Child != nil || result.quiescence.child != nil {
		t.Fatalf("child ownership = primary %p descriptor %p cleanup %p", result.primary.Child, result.descriptorClose.Child, result.quiescence.child)
	}
}

type memoryProcessEnd struct {
	mu               sync.Mutex
	reader           *bytes.Reader
	written          bytes.Buffer
	readErr          error
	readErrWithData  bool
	readAttempt      chan struct{}
	readAttemptOnce  sync.Once
	readRelease      <-chan struct{}
	readComplete     chan struct{}
	readCompleteOnce sync.Once
	writeErr         error
	writeAttempt     chan struct{}
	writeAttemptOnce sync.Once
	writeRelease     <-chan struct{}
	closeErr         error
	closeCount       int
}

type changingErrorContext struct {
	errors []error
	calls  int
}

func (ctx *changingErrorContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *changingErrorContext) Done() <-chan struct{}       { return nil }

func (ctx *changingErrorContext) Err() error {
	index := ctx.calls
	ctx.calls++
	if index >= len(ctx.errors) {
		return ctx.errors[len(ctx.errors)-1]
	}
	return ctx.errors[index]
}

func (*changingErrorContext) Value(any) any { return nil }

func (end *memoryProcessEnd) Read(buffer []byte) (int, error) {
	end.mu.Lock()
	defer end.mu.Unlock()
	end.readAttemptOnce.Do(func() {
		if end.readAttempt != nil {
			close(end.readAttempt)
		}
	})
	if end.readRelease != nil {
		<-end.readRelease
	}
	if end.reader == nil {
		return 0, io.EOF
	}
	n, err := end.reader.Read(buffer)
	if errors.Is(err, io.EOF) {
		end.readCompleteOnce.Do(func() {
			if end.readComplete != nil {
				close(end.readComplete)
			}
		})
	}
	if n != 0 && end.readErrWithData && end.readErr != nil {
		return n, end.readErr
	}
	if errors.Is(err, io.EOF) && end.readErr != nil {
		return n, end.readErr
	}
	return n, err
}

func (end *memoryProcessEnd) Write(buffer []byte) (int, error) {
	end.mu.Lock()
	defer end.mu.Unlock()
	end.writeAttemptOnce.Do(func() {
		if end.writeAttempt != nil {
			close(end.writeAttempt)
		}
	})
	if end.writeRelease != nil {
		<-end.writeRelease
	}
	if end.writeErr != nil {
		return 0, end.writeErr
	}
	return end.written.Write(buffer)
}

func (end *memoryProcessEnd) Close() error {
	end.mu.Lock()
	defer end.mu.Unlock()
	end.closeCount++
	return end.closeErr
}
