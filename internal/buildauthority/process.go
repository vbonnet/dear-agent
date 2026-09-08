package buildauthority

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"math"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// These policy values remain private here until the process tranche is folded
// into policy.go. They are the closed limits from the authority contract.
const (
	processMaxStructuredOutputBytes uint64 = 256 << 20
	processDiagnosticPrefixBytes           = 1 << 20
	processGroupMemberLimit                = 4_096
	processSurvivorSnapshotLimit           = 1_000
	processPollInterval                    = 5 * time.Millisecond
	processExitObservationWindow           = 5 * time.Second
	processPipeCompletionWindow            = time.Second
)

// processSupervisor is the transaction-scoped child authority. Its private
// seal prevents an accidental implementation outside this package. The two
// nominal entry points make preallocation and task-private environments
// non-interchangeable at the command seam.
type processSupervisor interface {
	runPreallocation(processRequest, preallocationEnvironment) processResult
	runTaskPrivate(processRequest, taskPrivateEnvironment) processResult
	privateProcessSupervisor()
}

// processRequest contains only already-authenticated command material. The
// environment is supplied only through one of processSupervisor's typed entry
// points and can never be selected as an arbitrary request field.
type processRequest struct {
	ctx                 context.Context
	phase               Phase
	executable          string
	arguments           []string
	directory           string
	input               []byte
	stdoutLimit         uint64
	phaseDeadline       time.Time
	transactionDeadline time.Time
}

// processResult is deliberately package-private. structuredOutput is the only
// raw child data retained by this seam; stderr is reduced to ChildDiagnostic.
type processResult struct {
	structuredOutput []byte
	primary          *FailureRecord
	descriptorClose  *FailureRecord
	quiescence       quiescenceResult
	background       *processBackgroundReap
}

// processBackgroundReap exposes only sanitized liveness state and is never
// joined or consulted to mutate the already-sealed public result.
type processBackgroundReap struct {
	done <-chan processReapResult
}

type processReapResult struct {
	reliable   bool
	pid        int
	exited     bool
	exitStatus int
	signaled   bool
	signal     int
	coreDumped bool
}

type processCommand interface {
	configure(stdin io.Reader, stdout, stderr io.Writer)
	start() (int, error)
	wait() processReapResult
}

type processCommandSpec struct {
	executable string
	arguments  []string
	directory  string
}

type processCommandFactory func(processCommandSpec) processCommand

type preallocationProcessCommandFactory func(
	processCommandSpec,
	preallocationEnvironment,
) processCommand

type taskPrivateProcessCommandFactory func(
	processCommandSpec,
	taskPrivateEnvironment,
) processCommand

type processPipeEnd interface {
	io.Reader
	io.Writer
	io.Closer
}

type processPipe struct {
	read  processPipeEnd
	write processPipeEnd
}

type processPipeFactory func() (processPipe, error)

type processScheduler interface {
	now() time.Time
	wait(time.Duration, <-chan struct{}, <-chan struct{}, processWaitPurpose)
}

type processWaitPurpose uint8

const (
	processWaitObservation processWaitPurpose = iota + 1
	processWaitCompletion
)

type realProcessScheduler struct{}

func (realProcessScheduler) now() time.Time { return time.Now() }

func (realProcessScheduler) wait(
	delay time.Duration,
	wake, canceled <-chan struct{},
	_ processWaitPurpose,
) {
	if delay <= 0 {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-wake:
	case <-canceled:
	}
}

func validateProcessRequest(request processRequest) *FailureRecord {
	phase := request.phase
	if !validPhase(phase) {
		phase = PhaseAuthority
	}
	invalid := request.ctx == nil || !validPhase(request.phase) ||
		request.stdoutLimit == 0 || request.stdoutLimit > processMaxStructuredOutputBytes ||
		!cleanAbsoluteProcessPath(request.executable) ||
		!cleanAbsoluteProcessPath(request.directory)
	if !invalid && request.phaseDeadline.IsZero() && request.transactionDeadline.IsZero() {
		_, hasContextDeadline := request.ctx.Deadline()
		invalid = !hasContextDeadline
	}
	if !invalid {
		return nil
	}
	return &FailureRecord{
		Phase:     phase,
		Operation: OperationValidate,
		Causes:    []CauseCode{CauseInternalInvariant},
	}
}

func cleanAbsoluteProcessPath(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path
}

type processStopKind uint8

const (
	processStopNone processStopKind = iota
	processStopDeadline
	processStopCanceled
	processStopOutputLimit
	processStopInput
	processStopRead
)

type processStop struct {
	kind processStopKind
}

func (stop processStop) primary(phase Phase) *FailureRecord {
	var operation Operation
	var cause CauseCode
	switch stop.kind {
	case processStopDeadline:
		operation, cause = OperationExecute, CauseDeadline
	case processStopCanceled:
		operation, cause = OperationExecute, CauseCanceled
	case processStopOutputLimit:
		operation, cause = OperationExecute, CauseLimit
	case processStopInput:
		operation, cause = OperationExecute, CauseUnstable
	case processStopRead, processStopNone:
		return nil
	default:
		operation, cause = OperationExecute, CauseInternalInvariant
	}
	return &FailureRecord{Phase: phase, Operation: operation, Causes: []CauseCode{cause}}
}

// processNotifications is the only worker-to-owner channel. Workers publish
// facts; only the event owner samples their precedence and chooses an action.
type processNotifications struct {
	mu          sync.Mutex
	wake        chan struct{}
	completion  chan struct{}
	outputLimit bool
	input       bool
	read        bool
}

func newProcessNotifications() *processNotifications {
	return &processNotifications{
		wake:       make(chan struct{}, 1),
		completion: make(chan struct{}, 3),
	}
}

func (notifications *processNotifications) workerCompleted() {
	// One stdout reader, one stderr reader, and at most one input writer each
	// publish exactly once after their buffered result is available.
	notifications.completion <- struct{}{}
}

func (notifications *processNotifications) publish(kind processStopKind) {
	notifications.mu.Lock()
	switch kind {
	case processStopNone, processStopDeadline, processStopCanceled:
		// Only worker-originated facts are publishable through this channel.
	case processStopOutputLimit:
		notifications.outputLimit = true
	case processStopInput:
		notifications.input = true
	case processStopRead:
		notifications.read = true
	}
	notifications.mu.Unlock()
	select {
	case notifications.wake <- struct{}{}:
	default:
	}
}

func (notifications *processNotifications) sample(request processRequest, now time.Time) processStop {
	// Deadline is deliberately sampled before cancellation, including a context
	// whose own timer has already reported DeadlineExceeded.
	contextError := request.ctx.Err()
	if processDeadlineReached(request, now) || errors.Is(contextError, context.DeadlineExceeded) {
		return processStop{kind: processStopDeadline}
	}
	if contextError != nil {
		return processStop{kind: processStopCanceled}
	}
	notifications.mu.Lock()
	defer notifications.mu.Unlock()
	switch {
	case notifications.outputLimit:
		return processStop{kind: processStopOutputLimit}
	case notifications.input:
		return processStop{kind: processStopInput}
	case notifications.read:
		return processStop{kind: processStopRead}
	default:
		return processStop{}
	}
}

func (notifications *processNotifications) sampleWorkers() processStop {
	notifications.mu.Lock()
	defer notifications.mu.Unlock()
	switch {
	case notifications.outputLimit:
		return processStop{kind: processStopOutputLimit}
	case notifications.input:
		return processStop{kind: processStopInput}
	case notifications.read:
		return processStop{kind: processStopRead}
	default:
		return processStop{}
	}
}

func processDeadlineReached(request processRequest, now time.Time) bool {
	for _, deadline := range processDeadlines(request) {
		if !deadline.IsZero() && !now.Before(deadline) {
			return true
		}
	}
	return false
}

func nextProcessDeadline(request processRequest) time.Time {
	var next time.Time
	for _, deadline := range processDeadlines(request) {
		if deadline.IsZero() || (!next.IsZero() && !deadline.Before(next)) {
			continue
		}
		next = deadline
	}
	return next
}

func processDeadlines(request processRequest) [3]time.Time {
	deadlines := [3]time.Time{request.phaseDeadline, request.transactionDeadline}
	if deadline, ok := request.ctx.Deadline(); ok {
		deadlines[2] = deadline
	}
	return deadlines
}

func processPollDelay(now time.Time, deadlines ...time.Time) time.Duration {
	delay := processPollInterval
	for _, deadline := range deadlines {
		if deadline.IsZero() {
			continue
		}
		remaining := deadline.Sub(now)
		if remaining < delay {
			delay = remaining
		}
	}
	if delay < 0 {
		return 0
	}
	return delay
}

type ownedProcessEnd struct {
	end       processPipeEnd
	closeOnce sync.Once
	closeErr  error
}

func ownProcessEnd(end processPipeEnd) *ownedProcessEnd {
	return &ownedProcessEnd{end: end}
}

func (end *ownedProcessEnd) Read(buffer []byte) (int, error) {
	return end.end.Read(buffer)
}

func (end *ownedProcessEnd) Write(buffer []byte) (int, error) {
	return end.end.Write(buffer)
}

func (end *ownedProcessEnd) close() error {
	if end == nil || end.end == nil {
		return nil
	}
	end.closeOnce.Do(func() { end.closeErr = end.end.Close() })
	return end.closeErr
}

func processDescriptorFailure(ends ...*ownedProcessEnd) *FailureRecord {
	failed := false
	for _, end := range ends {
		if end != nil && end.close() != nil {
			failed = true
		}
	}
	if !failed {
		return nil
	}
	return &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationCloseNonRoot,
		Causes:    []CauseCode{CauseDescriptorClose},
	}
}

type processStdoutResult struct {
	bytes         []byte
	eof           bool
	limitExceeded bool
	readFailed    bool
}

type processStderrResult struct {
	prefix     []byte
	total      uint64
	overflow   bool
	eof        bool
	readFailed bool
}

type processWriterResult struct {
	writeFailed bool
}

func startProcessStdoutReader(
	end *ownedProcessEnd,
	limit uint64,
	notifications *processNotifications,
) <-chan processStdoutResult {
	done := make(chan processStdoutResult, 1)
	go func() {
		reader := processStdoutReader{
			result:        processStdoutResult{bytes: make([]byte, 0, processInitialCapacity(limit))},
			limit:         limit,
			notifications: notifications,
		}
		defer func() {
			_ = end.close()
			done <- reader.result
			notifications.workerCompleted()
		}()
		buffer := make([]byte, 32<<10)
		for {
			n, err := end.Read(buffer)
			if n < 0 || n > len(buffer) {
				reader.result.readFailed = true
				notifications.publish(processStopRead)
				return
			}
			if n != 0 {
				reader.consume(buffer[:n])
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					reader.result.eof = true
				} else {
					reader.result.readFailed = true
					notifications.publish(processStopRead)
				}
				return
			}
			if n == 0 {
				reader.result.readFailed = true
				notifications.publish(processStopRead)
				return
			}
		}
	}()
	return done
}

type processStdoutReader struct {
	result        processStdoutResult
	total         uint64
	limit         uint64
	limitFailed   bool
	notifications *processNotifications
}

func (reader *processStdoutReader) consume(chunk []byte) {
	before := reader.total
	reader.total = saturatingProcessByteCount(reader.total, len(chunk))
	if reader.total > reader.limit && !reader.limitFailed {
		// Publish before the caller handles an error returned with this same
		// read, preserving output-limit precedence over the later read failure.
		reader.limitFailed = true
		reader.result.limitExceeded = true
		reader.notifications.publish(processStopOutputLimit)
	}
	keep := retainedProcessChunkBytes(before, reader.limit, len(chunk))
	if keep == 0 {
		return
	}
	reader.result.bytes = append(reader.result.bytes, chunk[:keep]...)
}

func saturatingProcessByteCount(total uint64, added int) uint64 {
	if added < 0 {
		return math.MaxUint64
	}
	addedBytes := uint64(added)
	if total > math.MaxUint64-addedBytes {
		return math.MaxUint64
	}
	return total + addedBytes
}

func retainedProcessChunkBytes(before, limit uint64, available int) int {
	if before >= limit || available <= 0 {
		return 0
	}
	remaining := limit - before
	if remaining >= math.MaxInt || int(remaining) >= available {
		return available
	}
	return int(remaining)
}

func startProcessStderrReader(end *ownedProcessEnd, notifications *processNotifications) <-chan processStderrResult {
	done := make(chan processStderrResult, 1)
	go func() {
		result := processStderrResult{prefix: make([]byte, 0, processDiagnosticPrefixBytes)}
		defer func() {
			_ = end.close()
			done <- result
			notifications.workerCompleted()
		}()
		buffer := make([]byte, 32<<10)
		for {
			n, err := end.Read(buffer)
			if n < 0 || n > len(buffer) {
				result.readFailed = true
				notifications.publish(processStopRead)
				return
			}
			if n != 0 {
				if result.total > math.MaxUint64-uint64(n) {
					result.overflow = true
					result.total = math.MaxUint64
					notifications.publish(processStopOutputLimit)
				} else {
					result.total += uint64(n)
				}
				remaining := min(processDiagnosticPrefixBytes-len(result.prefix), n)
				if remaining != 0 {
					result.prefix = append(result.prefix, buffer[:remaining]...)
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					result.eof = true
				} else {
					result.readFailed = true
					notifications.publish(processStopRead)
				}
				return
			}
			if n == 0 {
				result.readFailed = true
				notifications.publish(processStopRead)
				return
			}
		}
	}()
	return done
}

func startProcessInputWriter(
	end *ownedProcessEnd,
	input []byte,
	stop <-chan struct{},
	notifications *processNotifications,
) <-chan processWriterResult {
	done := make(chan processWriterResult, 1)
	ownedInput := append([]byte(nil), input...)
	go func(input []byte) {
		result := processWriterResult{}
		defer func() {
			_ = end.close()
			done <- result
			notifications.workerCompleted()
		}()
		for len(input) != 0 {
			select {
			case <-stop:
				return
			default:
			}
			n, err := end.Write(input)
			if n < 0 || n > len(input) || (n == 0 && err == nil) {
				result.writeFailed = true
				notifications.publish(processStopInput)
				return
			}
			input = input[n:]
			if err != nil {
				result.writeFailed = true
				notifications.publish(processStopInput)
				return
			}
		}
	}(ownedInput)
	return done
}

func processInitialCapacity(limit uint64) int {
	const eager = 64 << 10
	if limit < eager {
		return int(limit)
	}
	return eager
}

func childDiagnosticFromProcess(stderr processStderrResult, terminal *processTerminal) *ChildDiagnostic {
	diagnostic := &ChildDiagnostic{
		DiagnosticBytes:  stderr.total,
		Truncated:        stderr.total > uint64(len(stderr.prefix)),
		DiagnosticSHA256: Digest(sha256.Sum256(stderr.prefix)),
	}
	if terminal != nil {
		diagnostic.ExitStatusObserved = true
		if terminal.class == processTerminalExited {
			diagnostic.ExitStatus = terminal.status
		} else {
			diagnostic.ExitStatus = -terminal.status
		}
	}
	return diagnostic
}

func attachProcessChild(result *processResult, child *ChildDiagnostic) {
	if result == nil || child == nil {
		return
	}
	switch {
	case result.primary != nil:
		result.primary.Child = child
	case result.descriptorClose != nil:
		result.descriptorClose.Child = child
	case !result.quiescence.proven:
		result.quiescence.child = child
	}
}

func addProcessCleanupCause(result *processResult, cause CauseCode) {
	result.quiescence.proven = false
	if slices.Contains(result.quiescence.causes, cause) {
		return
	}
	result.quiescence.causes = append(result.quiescence.causes, cause)
}

type processTerminalClass uint8

const (
	processTerminalExited processTerminalClass = iota + 1
	processTerminalKilled
	processTerminalDumped
)

type processTerminal struct {
	class  processTerminalClass
	status int
}
