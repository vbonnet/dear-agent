//go:build darwin && arm64

package buildauthority

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type darwinProcessDependencies struct {
	queryAction             func() (darwinOldSigaction, error)
	waitID                  func(int) (darwinSiginfo, error)
	signalGroup             func(int) error
	snapshot                func(int, *darwinGroupBuffer) (darwinSnapshotResult, error)
	newPreallocationCommand preallocationProcessCommandFactory
	newTaskPrivateCommand   taskPrivateProcessCommandFactory
	newPipe                 processPipeFactory
	scheduler               processScheduler
}

type darwinProcessSupervisor struct {
	retained darwinOldSigaction
	deps     darwinProcessDependencies
}

func (*darwinProcessSupervisor) privateProcessSupervisor() {}

func newProcessSupervisor() (processSupervisor, *FailureRecord) {
	return newProcessSupervisorWithScheduler(&realProcessScheduler{})
}

func newProcessSupervisorWithScheduler(
	scheduler processScheduler,
) (processSupervisor, *FailureRecord) {
	if scheduler == nil {
		return nil, &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
	}
	return newDarwinProcessSupervisor(darwinProcessDependencies{
		queryAction:             realDarwinSigaction,
		waitID:                  realDarwinWaitID,
		signalGroup:             realDarwinSignalGroup,
		snapshot:                realDarwinGroupSnapshot,
		newPreallocationCommand: newRealPreallocationProcessCommand,
		newTaskPrivateCommand:   newRealTaskPrivateProcessCommand,
		newPipe:                 newRealProcessPipe,
		scheduler:               scheduler,
	})
}

func newDarwinProcessSupervisor(deps darwinProcessDependencies) (processSupervisor, *FailureRecord) {
	if deps.queryAction == nil || deps.waitID == nil || deps.signalGroup == nil ||
		deps.snapshot == nil || deps.newPreallocationCommand == nil ||
		deps.newTaskPrivateCommand == nil || deps.newPipe == nil || deps.scheduler == nil {
		return nil, &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationProbe,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
	}
	action, err := deps.queryAction()
	if err != nil {
		return nil, &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationProbe,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
	}
	if !action.admitted() {
		return nil, &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseUnsupported},
		}
	}
	return &darwinProcessSupervisor{retained: action, deps: deps}, nil
}

func (supervisor *darwinProcessSupervisor) runPreallocation(
	request *processRequest,
	environment preallocationEnvironment,
) processResult {
	return supervisor.runClosed(request, environment.valid(), func(spec processCommandSpec) processCommand {
		return supervisor.deps.newPreallocationCommand(spec, environment)
	})
}

func (supervisor *darwinProcessSupervisor) runTaskPrivate(
	request *processRequest,
	environment taskPrivateEnvironment,
) processResult {
	return supervisor.runClosed(request, environment.valid(), func(spec processCommandSpec) processCommand {
		return supervisor.deps.newTaskPrivateCommand(spec, environment)
	})
}

//nolint:gocyclo // The fixed setup/start/ownership order is the audited process state machine.
func (supervisor *darwinProcessSupervisor) runClosed(
	request *processRequest,
	validEnvironment bool,
	newCommand processCommandFactory,
) processResult {
	input, result, valid := supervisor.validateClosedRun(request, validEnvironment)
	if !valid {
		return result
	}
	if input.kind == processInputBracketedRetainedNull {
		if failure := processRequestBoundaryFailure(request, supervisor.deps.scheduler.now()); failure != nil {
			result.primary = failure
			return result
		}
	}

	pipes, setupErr := setupProcessPipes(supervisor.deps.newPipe, input.kind == processInputBounded)
	if setupErr != nil {
		result.primary = &FailureRecord{
			Phase:     request.phase,
			Operation: OperationOpen,
			Causes:    []CauseCode{classifyProcessPipeError(setupErr)},
		}
		result.descriptorClose = processDescriptorFailure(pipes.allEnds()...)
		return result
	}

	action, actionErr := supervisor.deps.queryAction()
	if actionErr != nil || action != supervisor.retained || !action.admitted() {
		cause := CauseUnstable
		if actionErr != nil {
			cause = CauseInternalInvariant
		}
		result.primary = &FailureRecord{
			Phase:     request.phase,
			Operation: OperationProbe,
			Causes:    []CauseCode{cause},
		}
		result.descriptorClose = processDescriptorFailure(pipes.allEnds()...)
		return result
	}
	if input.kind == processInputBracketedRetainedNull {
		if failure := processRequestBoundaryFailure(request, supervisor.deps.scheduler.now()); failure != nil {
			result.primary = failure
			result.descriptorClose = processDescriptorFailure(pipes.allEnds()...)
			return result
		}
	}

	command := newCommand(processCommandSpec{
		executable: request.executable,
		arguments:  append([]string(nil), request.arguments...),
		directory:  request.directory,
	})
	if command == nil {
		result.primary = &FailureRecord{
			Phase:     request.phase,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseChildStart},
		}
		result.descriptorClose = processDescriptorFailure(pipes.allEnds()...)
		return result
	}

	stdin := input.retainedNull.processReader()
	if input.kind == processInputBounded {
		stdin = pipes.stdinChildReader()
	}
	command.configure(stdin, pipes.stdoutChildWriter(), pipes.stderrChildWriter())
	notifications := newProcessNotifications()
	workerStop := make(chan struct{})
	stdoutDone := startProcessStdoutReader(
		pipes.stdoutRead,
		request.stdoutLimit,
		notifications,
	)
	stderrDone := startProcessStderrReader(pipes.stderrRead, notifications)
	if input.kind == processInputBracketedRetainedNull {
		if failure := processRequestBoundaryFailure(request, supervisor.deps.scheduler.now()); failure != nil {
			_ = pipes.closeAll()
			<-stdoutDone
			<-stderrDone
			result.primary = failure
			result.descriptorClose = processDescriptorFailure(pipes.allEnds()...)
			return result
		}
	}

	pid, startErr := command.start()
	if startErr != nil {
		_ = pipes.closeAll()
		<-stdoutDone
		<-stderrDone
		result.primary = &FailureRecord{
			Phase:     request.phase,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseChildStart},
		}
		result.descriptorClose = processDescriptorFailure(pipes.allEnds()...)
		return result
	}

	// Successful Start is the process-group establishment proof. Only the
	// parent's duplicates of child-side ends are closed here. Record the child
	// in its owning run before any later close or worker failure can occur.
	run := &darwinProcessRun{
		supervisor:    supervisor,
		request:       request,
		command:       command,
		pid:           pid,
		pipes:         pipes,
		notifications: notifications,
		workerStop:    workerStop,
		stdoutDone:    stdoutDone,
		stderrDone:    stderrDone,
	}
	_ = pipes.closeChildEnds()
	if pipes.stdinWrite != nil {
		run.writerDone = startProcessInputWriter(pipes.stdinWrite, input.bounded, workerStop, notifications)
	}
	if pid <= 0 || pid > math.MaxInt32 {
		addProcessCleanupCause(&run.result, CauseChildWait)
		run.backgroundHandoff(processStop{})
	} else {
		run.execute()
	}
	run.result.descriptorClose = processDescriptorFailure(pipes.allEnds()...)
	run.applyRetainedNullGate(request.ctx, input)
	run.applySuccessfulStderrGate()
	attachProcessChild(&run.result, childDiagnosticFromProcess(run.stderr, run.publicTerminal))
	run.publishStructuredOutput()
	return run.result
}

func (supervisor *darwinProcessSupervisor) validateClosedRun(
	request *processRequest,
	validEnvironment bool,
) (resolvedProcessInput, processResult, bool) {
	result := processResult{quiescence: quiescenceResult{proven: true}}
	if request == nil {
		result.primary = &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
		return resolvedProcessInput{}, result, false
	}
	if failure := validateProcessRequest(*request); failure != nil {
		result.primary = failure
		return resolvedProcessInput{}, result, false
	}
	if supervisor == nil || !validEnvironment {
		result.primary = &FailureRecord{
			Phase:     request.phase,
			Operation: OperationValidate,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
		return resolvedProcessInput{}, result, false
	}
	input, inputErr := resolveProcessInput(request.ctx, request.input)
	if inputErr != nil {
		result.primary = &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationProbe,
			Causes:    privateCauses(inputErr, CauseInternalInvariant),
		}
		return resolvedProcessInput{}, result, false
	}
	return input, result, true
}

type darwinProcessRun struct {
	supervisor    *darwinProcessSupervisor
	request       *processRequest
	command       processCommand
	pid           int
	pipes         processRunPipes
	notifications *processNotifications
	workerStop    chan struct{}
	stopOnce      sync.Once
	stdoutDone    <-chan processStdoutResult
	stderrDone    <-chan processStderrResult
	writerDone    <-chan processWriterResult
	groupBuffer   darwinGroupBuffer
	groupSeen     [processGroupMemberLimit * 2]int32

	result          processResult
	stderr          processStderrResult
	publicTerminal  *processTerminal
	candidateOutput []byte
	ioComplete      bool
}

func (run *darwinProcessRun) stopWorkers() {
	run.stopOnce.Do(func() { close(run.workerStop) })
}

type darwinProcessObservation struct {
	stop                     processStop
	terminal                 *processTerminal
	signalAttempted          bool
	initialGroupKillAccepted bool
	signalResolved           bool
	signalProvisional        bool
	consecutiveStoppedEINTR  int
	exitDeadline             time.Time
}

func (run *darwinProcessRun) execute() {
	observation, handoff := run.observeTerminal()
	if handoff {
		run.backgroundHandoff(observation.stop)
		return
	}
	run.finishTerminal(&observation)
}

func (run *darwinProcessRun) observeTerminal() (darwinProcessObservation, bool) {
	observation := darwinProcessObservation{signalResolved: true}
	for {
		now := run.supervisor.deps.scheduler.now()
		run.sampleObservationStop(&observation, now)
		if observation.signalAttempted && !now.Before(observation.exitDeadline) {
			addProcessCleanupCause(&run.result, CauseChildWait)
			return observation, true
		}
		info, err := run.supervisor.deps.waitID(run.pid)
		if err != nil {
			if !errors.Is(err, syscall.EINTR) {
				addProcessCleanupCause(&run.result, CauseChildWait)
				return observation, true
			}
			if run.handleInterruptedWait(&observation) {
				return observation, true
			}
			continue
		}
		decoded, noResult, valid := decodeDarwinWait(info, run.pid)
		if !valid {
			addProcessCleanupCause(&run.result, CauseChildWait)
			return observation, true
		}
		observation.consecutiveStoppedEINTR = 0
		if decoded != nil {
			run.sampleObservationStop(&observation, run.supervisor.deps.scheduler.now())
			observation.terminal = decoded
			return observation, false
		}
		if !noResult {
			addProcessCleanupCause(&run.result, CauseChildWait)
			return observation, true
		}
		if observation.stop.kind != processStopNone && !observation.signalAttempted {
			if !run.attemptInitialSignal(&observation) {
				return observation, true
			}
			continue
		}
		run.waitForNextObservation(&observation)
	}
}

func (run *darwinProcessRun) sampleObservationStop(observation *darwinProcessObservation, now time.Time) {
	sampled := run.notifications.sample(run.request, now)
	if observation.stop.kind != processStopNone || sampled.kind == processStopNone {
		return
	}
	observation.stop = sampled
	run.stopWorkers()
	if sampled.kind == processStopRead {
		addProcessCleanupCause(&run.result, CauseChildDrain)
	}
}

func (run *darwinProcessRun) handleInterruptedWait(observation *darwinProcessObservation) bool {
	if observation.stop.kind == processStopNone {
		now := run.supervisor.deps.scheduler.now()
		delay := processPollDelay(
			now,
			nextProcessDeadline(run.request, now),
			observation.exitDeadline,
		)
		run.supervisor.deps.scheduler.wait(
			delay,
			run.notifications.wake,
			run.request.ctx.Done(),
			processWaitObservation,
		)
		return false
	}
	observation.consecutiveStoppedEINTR++
	if observation.consecutiveStoppedEINTR >= 2 ||
		observation.signalAttempted && !run.supervisor.deps.scheduler.now().Before(observation.exitDeadline) {
		addProcessCleanupCause(&run.result, CauseChildWait)
		return true
	}
	run.supervisor.deps.scheduler.wait(processPollInterval, nil, nil, processWaitObservation)
	return false
}

func (run *darwinProcessRun) attemptInitialSignal(observation *darwinProcessObservation) bool {
	action, err := run.supervisor.deps.queryAction()
	if err != nil || action != run.supervisor.retained || !action.admitted() {
		addProcessCleanupCause(&run.result, CauseChildWait)
		return false
	}
	attemptInstant := run.supervisor.deps.scheduler.now()
	observation.signalAttempted = true
	observation.exitDeadline = attemptInstant.Add(processExitObservationWindow)
	signalErr := run.supervisor.deps.signalGroup(run.pid)
	switch {
	case signalErr == nil:
		observation.initialGroupKillAccepted = true
		observation.signalResolved = true
	case errors.Is(signalErr, syscall.ESRCH), errors.Is(signalErr, syscall.EPERM):
		observation.signalResolved = false
		observation.signalProvisional = true
	default:
		observation.signalResolved = false
		addProcessCleanupCause(&run.result, CauseChildTerminate)
	}
	return true
}

func (run *darwinProcessRun) waitForNextObservation(observation *darwinProcessObservation) {
	now := run.supervisor.deps.scheduler.now()
	delay := processPollDelay(
		now,
		nextProcessDeadline(run.request, now),
		observation.exitDeadline,
	)
	wake := run.notifications.wake
	cancelWake := run.request.ctx.Done()
	if observation.stop.kind != processStopNone {
		wake = nil
		cancelWake = nil
	}
	run.supervisor.deps.scheduler.wait(delay, wake, cancelWake, processWaitObservation)
}

func (run *darwinProcessRun) finishTerminal(observation *darwinProcessObservation) {
	run.publicTerminal = observation.terminal
	// The stop-or-terminal outcome is sealed at the waitid sampling boundary.
	// Later I/O faults may make cleanup uncertain but cannot replace it.
	run.setPrimary(observation.stop, observation.terminal, observation.initialGroupKillAccepted)
	leaderOnlyProof := run.handleTerminalGroup(observation)
	reap := run.command.wait()
	reapReliable := processReapMatches(run.pid, observation.terminal, reap)
	if !reapReliable {
		addProcessCleanupCause(&run.result, CauseChildWait)
	}
	run.stopWorkers()
	completion := run.finishIO(run.supervisor.deps.scheduler.now().Add(processPipeCompletionWindow))
	run.applyIOCompletion(completion, observation.stop)
	proof := leaderOnlyProof && observation.signalResolved && reapReliable && completion.writerJoined &&
		completion.stdoutJoined && completion.stdout.eof && !completion.stdout.readFailed &&
		completion.stderrJoined && completion.stderr.eof && !completion.stderr.readFailed
	run.result.quiescence.proven = proof && len(run.result.quiescence.causes) == 0
}

func (run *darwinProcessRun) handleTerminalGroup(observation *darwinProcessObservation) bool {
	classification, snapshotOK := run.snapshotGroup()
	if !snapshotOK || !classification.valid {
		addProcessCleanupCause(&run.result, CauseChildProbe)
	}
	if classification.hasSurvivor {
		addProcessCleanupCause(&run.result, CauseChildSurvivor)
	}
	leaderOnlyProof := snapshotOK && classification.valid && classification.leaderOnly
	if leaderOnlyProof && observation.signalProvisional {
		observation.signalResolved = true
	}
	if snapshotOK && classification.valid && classification.hasSurvivor {
		resolved := run.handleSurvivors()
		if observation.signalProvisional && resolved {
			observation.signalResolved = true
		}
	}
	if observation.signalProvisional && !observation.signalResolved {
		addProcessCleanupCause(&run.result, CauseChildTerminate)
	}
	return leaderOnlyProof
}

func (run *darwinProcessRun) handleSurvivors() bool {
	action, err := run.supervisor.deps.queryAction()
	if err != nil || action != run.supervisor.retained || !action.admitted() {
		addProcessCleanupCause(&run.result, CauseChildWait)
		return false
	}
	attemptInstant := run.supervisor.deps.scheduler.now()
	signalErr := run.supervisor.deps.signalGroup(run.pid)
	provisional := errors.Is(signalErr, syscall.ESRCH) || errors.Is(signalErr, syscall.EPERM)
	if signalErr != nil && !provisional {
		addProcessCleanupCause(&run.result, CauseChildTerminate)
	}
	resolved := run.followSurvivors(attemptInstant.Add(processExitObservationWindow))
	if provisional && !resolved {
		addProcessCleanupCause(&run.result, CauseChildTerminate)
	}
	return resolved
}

func (run *darwinProcessRun) backgroundHandoff(stop processStop) {
	handoffInstant := run.supervisor.deps.scheduler.now()
	run.stopWorkers()
	reapDone := make(chan processReapResult, 1)
	reaper := sealDarwinBackgroundReaper(run.command)
	go reapDarwinProcessInBackground(reaper, reapDone)
	run.result.background = &processBackgroundReap{done: reapDone}
	completion := run.finishIO(handoffInstant.Add(processPipeCompletionWindow))
	if stop.kind == processStopNone {
		stop = run.notifications.sampleWorkers()
		if stop.kind == processStopRead {
			addProcessCleanupCause(&run.result, CauseChildDrain)
		}
	}
	run.applyIOCompletion(completion, stop)
	run.setPrimary(stop, nil, false)
	run.result.quiescence.proven = false
}

// darwinBackgroundReaper is the complete capability transferred to the
// background goroutine. The method value retains only the command receiver and
// exposes no run, request, result, pipe, supervisor, signal, or probe authority.
type darwinBackgroundReaper struct {
	waitCommand func() processReapResult
}

func sealDarwinBackgroundReaper(command processCommand) darwinBackgroundReaper {
	return darwinBackgroundReaper{waitCommand: command.wait}
}

func reapDarwinProcessInBackground(reaper darwinBackgroundReaper, done chan<- processReapResult) {
	done <- reaper.waitCommand()
}

func (run *darwinProcessRun) setPrimary(
	stop processStop,
	terminal *processTerminal,
	initialGroupKillAccepted bool,
) {
	if run.result.primary == nil {
		run.result.primary = stop.primary(run.request.phase)
	}
	if run.result.primary != nil || terminal == nil ||
		terminal.class == processTerminalExited && terminal.status == 0 {
		return
	}
	intentionalKill := stop.kind != processStopNone && initialGroupKillAccepted &&
		terminal.class == processTerminalKilled && terminal.status == int(syscall.SIGKILL)
	if !intentionalKill {
		run.result.primary = &FailureRecord{
			Phase:     run.request.phase,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseChildExit},
		}
	}
}

func (run *darwinProcessRun) snapshotGroup() (darwinGroupClassification, bool) {
	result, err := run.supervisor.deps.snapshot(run.pid, &run.groupBuffer)
	if err != nil {
		return darwinGroupClassification{}, false
	}
	count, ok := result.count()
	if !ok {
		return darwinGroupClassification{}, false
	}
	return classifyDarwinGroupWithSeen(&run.groupBuffer, count, run.pid, &run.groupSeen), true
}

func (run *darwinProcessRun) followSurvivors(deadline time.Time) bool {
	for range processSurvivorSnapshotLimit {
		now := run.supervisor.deps.scheduler.now()
		if !now.Before(deadline) {
			return false
		}
		classification, ok := run.snapshotGroup()
		if !ok || !classification.valid {
			addProcessCleanupCause(&run.result, CauseChildProbe)
			return false
		}
		if classification.leaderOnly {
			return true
		}
		if !classification.hasSurvivor {
			addProcessCleanupCause(&run.result, CauseChildProbe)
			return false
		}
		delay := processPollDelay(run.supervisor.deps.scheduler.now(), deadline)
		run.supervisor.deps.scheduler.wait(delay, run.notifications.wake, nil, processWaitObservation)
	}
	return false
}

type darwinGroupClassification struct {
	valid       bool
	leaderOnly  bool
	hasSurvivor bool
}

func classifyDarwinGroup(buffer *darwinGroupBuffer, count, processGroupID int) darwinGroupClassification {
	var seen [processGroupMemberLimit * 2]int32
	return classifyDarwinGroupWithSeen(buffer, count, processGroupID, &seen)
}

func classifyDarwinGroupWithSeen(
	buffer *darwinGroupBuffer,
	count, processGroupID int,
	seen *[processGroupMemberLimit * 2]int32,
) darwinGroupClassification {
	classification := darwinGroupClassification{}
	if buffer == nil || seen == nil || count <= 0 || count > processGroupMemberLimit {
		return classification
	}
	*seen = [processGroupMemberLimit * 2]int32{}
	leaderFound := false
	valid := true
	for index := range count {
		pid, pgid, state := buffer.member(index)
		if pid <= 0 || int(pgid) != processGroupID {
			valid = false
		}
		if pid != 0 && !insertDarwinPID(seen, pid) {
			valid = false
		}
		if int(pid) == processGroupID {
			leaderFound = true
			if state != darwinProcessZombie {
				valid = false
			}
		} else {
			classification.hasSurvivor = true
		}
	}
	classification.valid = valid && leaderFound
	classification.leaderOnly = classification.valid && count == 1 && !classification.hasSurvivor
	return classification
}

func insertDarwinPID(seen *[processGroupMemberLimit * 2]int32, pid int32) bool {
	// This file is Darwin/arm64-only. Widening the positive int32 PID before
	// multiplication makes the Knuth hash exact in the 64-bit int domain.
	slot := int(pid) * 2_654_435_761 & (len(seen) - 1)
	for range len(seen) {
		switch seen[slot] {
		case 0:
			seen[slot] = pid
			return true
		case pid:
			return false
		default:
			slot = (slot + 1) & (len(seen) - 1)
		}
	}
	return false
}

func decodeDarwinWait(info darwinSiginfo, pid int) (*processTerminal, bool, bool) {
	if info.pid == 0 {
		return nil, true, true
	}
	if info.signo != darwinSIGCHLD || info.errno != 0 || int(info.pid) != pid {
		return nil, false, false
	}
	status := int(info.status)
	switch info.code {
	case darwinChildExited:
		if status < 0 || status > 255 {
			return nil, false, false
		}
		return &processTerminal{class: processTerminalExited, status: status}, false, true
	case darwinChildKilled:
		if !darwinKilledStatus(status) {
			return nil, false, false
		}
		return &processTerminal{class: processTerminalKilled, status: status}, false, true
	case darwinChildDumped:
		if !darwinDumpedStatus(status) {
			return nil, false, false
		}
		return &processTerminal{class: processTerminalDumped, status: status}, false, true
	default:
		return nil, false, false
	}
}

func darwinKilledStatus(status int) bool {
	return status >= 1 && status <= 15 || status >= 24 && status <= 27 || status == 30 || status == 31
}

func darwinDumpedStatus(status int) bool {
	switch status {
	case 3, 4, 5, 6, 7, 8, 10, 11, 12:
		return true
	default:
		return false
	}
}

func processReapMatches(pid int, terminal *processTerminal, reap processReapResult) bool {
	if terminal == nil || !reap.reliable || reap.pid != pid {
		return false
	}
	switch terminal.class {
	case processTerminalExited:
		return reap.exited && !reap.signaled && reap.exitStatus == terminal.status
	case processTerminalKilled:
		return reap.signaled && !reap.exited && reap.signal == terminal.status && !reap.coreDumped
	case processTerminalDumped:
		return reap.signaled && !reap.exited && reap.signal == terminal.status && reap.coreDumped
	default:
		return false
	}
}

type processIOCompletion struct {
	stdout       processStdoutResult
	stderr       processStderrResult
	writer       processWriterResult
	stdoutJoined bool
	stderrJoined bool
	writerJoined bool
	missed       bool
}

func (run *darwinProcessRun) finishIO(deadline time.Time) processIOCompletion {
	completion := processIOCompletion{writerJoined: run.writerDone == nil}
	for !completion.stdoutJoined || !completion.stderrJoined || !completion.writerJoined {
		pollProcessIO(&completion, run.stdoutDone, run.stderrDone, run.writerDone)
		if completion.stdoutJoined && completion.stderrJoined && completion.writerJoined {
			break
		}
		now := run.supervisor.deps.scheduler.now()
		if !now.Before(deadline) {
			completion.missed = true
			if !completion.stdoutJoined {
				_ = run.pipes.stdoutRead.close()
			}
			if !completion.stderrJoined {
				_ = run.pipes.stderrRead.close()
			}
			if !completion.writerJoined && run.pipes.stdinWrite != nil {
				_ = run.pipes.stdinWrite.close()
			}
			if !completion.stdoutJoined {
				completion.stdout = <-run.stdoutDone
				completion.stdoutJoined = true
			}
			if !completion.stderrJoined {
				completion.stderr = <-run.stderrDone
				completion.stderrJoined = true
			}
			if !completion.writerJoined {
				completion.writer = <-run.writerDone
				completion.writerJoined = true
			}
			break
		}
		delay := processPollDelay(now, deadline)
		run.supervisor.deps.scheduler.wait(delay, run.notifications.completion, nil, processWaitCompletion)
	}
	return completion
}

func pollProcessIO(
	completion *processIOCompletion,
	stdout <-chan processStdoutResult,
	stderr <-chan processStderrResult,
	writer <-chan processWriterResult,
) {
	if !completion.stdoutJoined {
		select {
		case completion.stdout = <-stdout:
			completion.stdoutJoined = true
		default:
		}
	}
	if !completion.stderrJoined {
		select {
		case completion.stderr = <-stderr:
			completion.stderrJoined = true
		default:
		}
	}
	if !completion.writerJoined && writer != nil {
		select {
		case completion.writer = <-writer:
			completion.writerJoined = true
		default:
		}
	}
}

func (run *darwinProcessRun) applyIOCompletion(completion processIOCompletion, latched processStop) {
	run.candidateOutput = completion.stdout.bytes
	run.stderr = completion.stderr
	if processIOHasCleanupGap(completion, latched) {
		addProcessCleanupCause(&run.result, CauseChildDrain)
	}
	run.ioComplete = processIOComplete(completion)
}

func processIOHasCleanupGap(completion processIOCompletion, latched processStop) bool {
	lateOutputLimit := (completion.stdout.limitExceeded || completion.stderr.overflow) &&
		latched.kind != processStopOutputLimit
	lateInputFailure := completion.writer.writeFailed && latched.kind != processStopInput
	return completion.missed || !completion.stdout.eof || completion.stdout.readFailed ||
		!completion.stderr.eof || completion.stderr.readFailed || !completion.writerJoined ||
		lateOutputLimit || lateInputFailure
}

func processIOComplete(completion processIOCompletion) bool {
	return !completion.missed && completion.stdoutJoined && completion.stdout.eof &&
		!completion.stdout.readFailed && !completion.stdout.limitExceeded &&
		completion.stderrJoined && completion.stderr.eof && !completion.stderr.readFailed &&
		!completion.stderr.overflow && completion.writerJoined && !completion.writer.writeFailed
}

func (run *darwinProcessRun) applyRetainedNullGate(ctx context.Context, input resolvedProcessInput) {
	if input.kind != processInputRetainedNull || run.result.primary != nil {
		return
	}
	if err := input.retainedNull.revalidate(ctx); err != nil {
		run.result.primary = &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationProbe,
			Causes:    privateCauses(err, CauseInternalInvariant),
		}
	}
}

func (run *darwinProcessRun) applySuccessfulStderrGate() {
	if run.result.primary != nil || run.publicTerminal == nil ||
		run.publicTerminal.class != processTerminalExited || run.publicTerminal.status != 0 ||
		run.stderr.total == 0 {
		return
	}
	run.result.primary = &FailureRecord{
		Phase:     run.request.phase,
		Operation: OperationParse,
		Causes:    []CauseCode{CauseMalformed},
	}
}

func (run *darwinProcessRun) publishStructuredOutput() {
	run.result.structuredOutput = nil
	run.result.structuredOutputChild = nil
	if run.result.primary != nil || run.result.descriptorClose != nil ||
		!run.result.quiescence.proven || run.result.background != nil || !run.ioComplete ||
		run.publicTerminal == nil || run.publicTerminal.class != processTerminalExited ||
		run.publicTerminal.status != 0 || run.stderr.total != 0 {
		return
	}
	child := childDiagnosticFromProcess(run.stderr, run.publicTerminal)
	if !validStructuredOutputChild(child) {
		return
	}
	run.result.structuredOutput = run.candidateOutput
	run.result.structuredOutputChild = child
}

type processRunPipes struct {
	stdoutRead  *ownedProcessEnd
	stdoutWrite *ownedProcessEnd
	stderrRead  *ownedProcessEnd
	stderrWrite *ownedProcessEnd
	stdinRead   *ownedProcessEnd
	stdinWrite  *ownedProcessEnd
}

func setupProcessPipes(factory processPipeFactory, withInput bool) (processRunPipes, error) {
	var pipes processRunPipes
	stdout, err := factory()
	if err != nil {
		return pipes, err
	}
	pipes.stdoutRead, pipes.stdoutWrite = ownProcessEnd(stdout.read), ownProcessEnd(stdout.write)
	if stdout.read == nil || stdout.write == nil {
		return pipes, syscall.EINVAL
	}
	stderr, err := factory()
	if err != nil {
		return pipes, err
	}
	pipes.stderrRead, pipes.stderrWrite = ownProcessEnd(stderr.read), ownProcessEnd(stderr.write)
	if stderr.read == nil || stderr.write == nil {
		return pipes, syscall.EINVAL
	}
	if !withInput {
		return pipes, nil
	}
	stdin, err := factory()
	if err != nil {
		return pipes, err
	}
	pipes.stdinRead, pipes.stdinWrite = ownProcessEnd(stdin.read), ownProcessEnd(stdin.write)
	if stdin.read == nil || stdin.write == nil {
		return pipes, syscall.EINVAL
	}
	return pipes, nil
}

// Pass the concrete child-side *os.File through the exec seam. Wrapping it in
// ownedProcessEnd would make os/exec allocate a second pipe and perform hidden
// copying from Cmd.Wait, defeating this supervisor's explicit-pipe ownership.
func (pipes processRunPipes) stdoutChildWriter() io.Writer { return pipes.stdoutWrite.end }
func (pipes processRunPipes) stderrChildWriter() io.Writer { return pipes.stderrWrite.end }

func (pipes processRunPipes) stdinChildReader() io.Reader {
	if pipes.stdinRead == nil {
		return nil
	}
	return pipes.stdinRead.end
}

func (pipes processRunPipes) closeChildEnds() error {
	var first error
	for _, end := range []*ownedProcessEnd{pipes.stdoutWrite, pipes.stderrWrite, pipes.stdinRead} {
		if err := end.close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (pipes processRunPipes) closeAll() error {
	var first error
	for _, end := range pipes.allEnds() {
		if err := end.close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (pipes processRunPipes) allEnds() []*ownedProcessEnd {
	return []*ownedProcessEnd{
		pipes.stdoutRead,
		pipes.stdoutWrite,
		pipes.stderrRead,
		pipes.stderrWrite,
		pipes.stdinRead,
		pipes.stdinWrite,
	}
}

func classifyProcessPipeError(err error) CauseCode {
	switch {
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return CausePermission
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE),
		errors.Is(err, syscall.ENOMEM), errors.Is(err, syscall.ENOBUFS):
		return CauseLimit
	default:
		return CauseInternalInvariant
	}
}

func newRealProcessPipe() (processPipe, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return processPipe{}, err
	}
	return processPipe{read: read, write: write}, nil
}

type realProcessCommand struct {
	command *exec.Cmd
}

func newRealPreallocationProcessCommand(
	spec processCommandSpec,
	environment preallocationEnvironment,
) processCommand {
	command := exec.Command(spec.executable, spec.arguments...)
	command.Dir = spec.directory
	command.Env = environment.clone()
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}
	return &realProcessCommand{command: command}
}

func newRealTaskPrivateProcessCommand(
	spec processCommandSpec,
	environment taskPrivateEnvironment,
) processCommand {
	command := exec.Command(spec.executable, spec.arguments...)
	command.Dir = spec.directory
	command.Env = environment.clone()
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: 0}
	return &realProcessCommand{command: command}
}

func (command *realProcessCommand) configure(stdin io.Reader, stdout, stderr io.Writer) {
	command.command.Stdin = stdin
	command.command.Stdout = stdout
	command.command.Stderr = stderr
}

func (command *realProcessCommand) start() (int, error) {
	if err := command.command.Start(); err != nil {
		return 0, err
	}
	if command.command.Process == nil {
		return 0, nil
	}
	return command.command.Process.Pid, nil
}

func (command *realProcessCommand) wait() processReapResult {
	err := command.command.Wait()
	state := command.command.ProcessState
	if state == nil {
		return processReapResult{}
	}
	result := processReapResult{pid: state.Pid()}
	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		return result
	}
	waitStatus, ok := state.Sys().(syscall.WaitStatus)
	if !ok {
		return result
	}
	result.reliable = true
	result.exited = waitStatus.Exited()
	result.signaled = waitStatus.Signaled()
	if result.exited {
		result.exitStatus = waitStatus.ExitStatus()
	}
	if result.signaled {
		result.signal = int(waitStatus.Signal())
		result.coreDumped = waitStatus.CoreDump()
	}
	return result
}
