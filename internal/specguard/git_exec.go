package specguard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

var errCommandOutputLimit = errors.New("git command output exceeded its safety limit")

func (git gitClient) run(ctx context.Context, root string, input []byte, outputLimit int64, args ...string) ([]byte, *guardFailure) {
	if int64(len(input)) > git.limits.maxGitInput {
		return nil, fail("git-input-limit", "", "Git command input exceeded the safety limit")
	}
	if outputLimit <= 0 || outputLimit > git.limits.maxGitOutput {
		return nil, fail("git-output-limit", "", "Git command output limit is invalid")
	}
	if git.repository != nil {
		if failure := git.repository.validate(root); failure != nil {
			return nil, failure
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, git.limits.gitTime)
	defer cancel()

	commandArgs := []string{
		"--no-replace-objects",
		"-c", "protocol.allow=never",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.trustctime=true",
		"-c", "core.checkStat=default",
		"-c", "diff.external=",
	}
	if root != "" {
		commandArgs = append(commandArgs, "-C", root)
	}
	commandArgs = append(commandArgs, args...)
	command := exec.Command(git.executable, commandArgs...)
	command.Env = cleanGitEnvironment(git.executable)
	command.Stdin = bytes.NewReader(input)
	configureProcessGroup(command)
	command.WaitDelay = 250 * time.Millisecond
	capture := newCommandCapture(outputLimit)
	command.Stdout = capture.stdout()
	command.Stderr = capture.stderr()
	execution := runGitCommand(commandCtx, command, capture)
	stdout, stderr, overflow := capture.result()
	if git.afterCommand != nil {
		git.afterCommand()
	}
	if git.repository != nil {
		if failure := git.repository.validate(root); failure != nil {
			return nil, failure
		}
	}
	return classifyGitCommandExecution(stdout, stderr, overflow, execution, git.limits.gitTime)
}

func classifyGitCommandExecution(
	stdout []byte,
	stderr []byte,
	overflow bool,
	execution gitCommandExecution,
	gitTime time.Duration,
) ([]byte, *guardFailure) {
	cancellationSignalErr := normalizeGitCancellationSignalError(
		execution.cancellationSignalErr,
		execution.observedExitErr,
		execution.cleanupErr,
	)
	switch {
	case execution.observedExitErr != nil || execution.cleanupErr != nil ||
		cancellationSignalErr != nil && !errors.Is(cancellationSignalErr, os.ErrProcessDone):
		return nil, fail("git-descendant-termination", "", "Git descendant processes could not be terminated")
	case overflow:
		return nil, fail("git-output-limit", "", errCommandOutputLimit.Error())
	case execution.contextCancellationObserved:
		return nil, fail("git-time-limit", "", fmt.Sprintf("Git command exceeded the %s wall-time limit", gitTime))
	case execution.commandErr != nil:
		message := "Git command failed"
		if detail := boundedDiagnostic(stderr, 512); detail != "" {
			message += ": " + detail
		}
		return nil, fail("git-command", "", message)
	default:
		return stdout, nil
	}
}

func normalizeGitCancellationSignalError(signalErr, observedExitErr, cleanupErr error) error {
	if errors.Is(signalErr, syscall.EPERM) && observedExitErr == nil && cleanupErr == nil {
		// On Darwin a context wake can race with direct-child exit and observe
		// EPERM from a zombie-only group. Successful pinned final cleanup proves
		// that no descendant remained, so the earlier signal is process-done.
		return os.ErrProcessDone
	}
	return signalErr
}

type gitCommandExecution struct {
	commandErr                  error
	observedExitErr             error
	cleanupErr                  error
	cancellationSignalErr       error
	contextCancellationObserved bool
}

// runGitCommand owns cancellation and process-group cleanup. waitid leaves the
// direct child unreaped until final cleanup has killed and sealed its isolated
// group, so neither an output callback nor a late context wake can signal a
// different process after numeric PID/PGID reuse.
func runGitCommand(ctx context.Context, command *exec.Cmd, capture *commandCapture) gitCommandExecution {
	if err := ctx.Err(); err != nil {
		return gitCommandExecution{commandErr: err, contextCancellationObserved: true}
	}
	lifecycle := newGitProcessGroupLifecycle(command)
	capture.onLimit = func() {
		_ = lifecycle.cancel()
	}
	if err := command.Start(); err != nil {
		lifecycle.disable()
		return gitCommandExecution{commandErr: err}
	}

	lifecycleDone := make(chan struct{})
	contextResult := make(chan gitContextCancellationResult, 1)
	go func() {
		result := gitContextCancellationResult{}
		defer func() {
			if recovered := recover(); recovered != nil {
				result = gitContextCancellationResult{
					signalErr: errors.New("git context watcher panicked"),
				}
			}
			contextResult <- result
		}()
		select {
		case <-ctx.Done():
			observed, err := lifecycle.cancelObserved()
			result = gitContextCancellationResult{signalErr: err, contextObserved: observed}
		case <-lifecycleDone:
		}
	}()

	observedExitErr := waitForGitCommandExitWithoutReaping(command.Process.Pid)
	cleanupErr := lifecycle.complete(observedExitErr == nil, errors.Is(observedExitErr, syscall.ECHILD))
	close(lifecycleDone)
	cancellation := <-contextResult
	commandErr := command.Wait()
	return gitCommandExecution{
		commandErr:                  commandErr,
		observedExitErr:             observedExitErr,
		cleanupErr:                  cleanupErr,
		cancellationSignalErr:       cancellation.signalErr,
		contextCancellationObserved: cancellation.contextObserved,
	}
}

type gitContextCancellationResult struct {
	signalErr       error
	contextObserved bool
}

// gitProcessGroupLifecycle serializes every group signal with final cleanup.
// complete seals the signal path while the unreaped leader still pins the
// numeric process-group ID; Cmd.Wait performs the one subsequent reap.
type gitProcessGroupLifecycle struct {
	mu                      sync.Mutex
	command                 *exec.Cmd
	enabled                 bool
	directChildExitObserved bool
	terminationSignaled     bool
}

func newGitProcessGroupLifecycle(command *exec.Cmd) *gitProcessGroupLifecycle {
	return &gitProcessGroupLifecycle{command: command, enabled: true}
}

func (lifecycle *gitProcessGroupLifecycle) cancel() error {
	_, err := lifecycle.cancelObserved()
	return err
}

// cancelObserved reports whether cancellation acquired the lifecycle before
// final cleanup sealed it. A raw ProcessDone result can therefore still count
// as a timeout when the context won the ordering race while the child exited.
func (lifecycle *gitProcessGroupLifecycle) cancelObserved() (bool, error) {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.enabled || lifecycle.command.Process == nil {
		return false, os.ErrProcessDone
	}
	err := killProcessGroup(lifecycle.command.Process)
	if err == nil {
		lifecycle.terminationSignaled = true
	}
	return true, err
}

func (lifecycle *gitProcessGroupLifecycle) complete(directChildExitObserved, skipKill bool) error {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if !lifecycle.enabled {
		return nil
	}
	lifecycle.directChildExitObserved = directChildExitObserved
	var err error
	if !skipKill {
		err = lifecycle.terminateLocked()
	}
	lifecycle.enabled = false
	return err
}

func (lifecycle *gitProcessGroupLifecycle) disable() {
	lifecycle.mu.Lock()
	lifecycle.enabled = false
	lifecycle.mu.Unlock()
}

func (lifecycle *gitProcessGroupLifecycle) terminateLocked() error {
	if lifecycle.command.Process == nil {
		return nil
	}
	processGroupID := lifecycle.command.Process.Pid
	if err := killProcessGroup(lifecycle.command.Process); errors.Is(err, os.ErrProcessDone) {
		return nil
	} else if errors.Is(err, syscall.EPERM) {
		complete, classificationErr := gitProcessGroupEPERMComplete(
			processGroupID,
			lifecycle.directChildExitObserved,
			lifecycle.terminationSignaled,
		)
		if classificationErr != nil {
			return fmt.Errorf("classify EPERM for Git process group %d: %w", processGroupID, classificationErr)
		}
		if complete {
			return nil
		}
		return fmt.Errorf("terminate Git process group %d: %w", processGroupID, err)
	} else if err != nil {
		return fmt.Errorf("terminate Git process group %d: %w", processGroupID, err)
	}
	return nil
}

func cleanGitEnvironment(executable string) []string {
	pathEntries := slices.Compact([]string{filepath.Dir(executable), "/usr/bin", "/bin"})
	environment := []string{
		"HOME=/var/empty",
		"LANG=C",
		"LC_ALL=C",
		"PATH=" + strings.Join(pathEntries, string(os.PathListSeparator)),
		"PAGER=cat",
		"XDG_CONFIG_HOME=/var/empty",
		"GIT_ATTR_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_SYSTEM=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	}
	for _, name := range []string{"COMSPEC", "PATHEXT", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func boundedDiagnostic(data []byte, limit int) string {
	if len(data) > limit {
		data = data[:limit]
	}
	text := strings.Map(func(value rune) rune {
		if value == '\n' || value == '\r' || value == '\t' {
			return ' '
		}
		if value < 0x20 || value == 0x7f {
			return -1
		}
		return value
	}, string(data))
	return strings.Join(strings.Fields(text), " ")
}

func (failure *guardFailure) withCode(code string) *guardFailure {
	if failure == nil {
		return nil
	}
	if failure.code == "git-input-limit" || failure.code == "git-output-limit" || failure.code == "git-time-limit" ||
		failure.code == "git-descendant-termination" || failure.code == "repository-identity-changed" {
		return failure
	}
	return fail(code, failure.path, failure.message)
}

type commandCapture struct {
	mu           sync.Mutex
	stdoutBuffer bytes.Buffer
	stderrBuffer bytes.Buffer
	limit        int64
	used         int64
	overflow     bool
	onLimit      func()
	cancelOnce   sync.Once
}

type captureStream struct {
	capture *commandCapture
	stderr  bool
}

func newCommandCapture(limit int64) *commandCapture {
	return &commandCapture{limit: limit}
}

func (capture *commandCapture) stdout() io.Writer { return captureStream{capture: capture} }
func (capture *commandCapture) stderr() io.Writer {
	return captureStream{capture: capture, stderr: true}
}

func (stream captureStream) Write(data []byte) (int, error) {
	stream.capture.mu.Lock()
	remaining := stream.capture.limit - stream.capture.used
	if int64(len(data)) > remaining {
		if remaining > 0 {
			prefix := data[:remaining]
			stream.capture.used += int64(len(prefix))
			if stream.stderr {
				_, _ = stream.capture.stderrBuffer.Write(prefix)
			} else {
				_, _ = stream.capture.stdoutBuffer.Write(prefix)
			}
		}
		stream.capture.overflow = true
		stream.capture.mu.Unlock()
		stream.capture.cancelOnce.Do(func() {
			if stream.capture.onLimit != nil {
				stream.capture.onLimit()
			}
		})
		return 0, errCommandOutputLimit
	}
	stream.capture.used += int64(len(data))
	defer stream.capture.mu.Unlock()
	if stream.stderr {
		return stream.capture.stderrBuffer.Write(data)
	}
	return stream.capture.stdoutBuffer.Write(data)
}

func (capture *commandCapture) result() (stdout, stderr []byte, overflow bool) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return bytes.Clone(capture.stdoutBuffer.Bytes()), bytes.Clone(capture.stderrBuffer.Bytes()), capture.overflow
}
