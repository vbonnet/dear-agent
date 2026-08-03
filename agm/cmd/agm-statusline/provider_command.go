package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// outputProviderCommand passes input and captures stdout through caller-owned
// pipes. This lets direct-process exit or ctx unblock both directions without
// os/exec waiting on descendants that retain inherited descriptors. stdout is
// accepted until EOF or ctx; CommandContext still terminates only the direct
// process, so this does not claim descendant cleanup.
func outputProviderCommand(ctx context.Context, cmd *exec.Cmd, input []byte) ([]byte, error) {
	run, err := startProviderCommand(cmd, input)
	if err != nil {
		return nil, err
	}
	return run.output(ctx)
}

type providerCommandRun struct {
	stdinWriter  *os.File
	stdoutReader *os.File
	outputBuffer bytes.Buffer
	inputDone    chan error
	readDone     chan error
	waitDone     chan error
	readErr      error

	stdinCloseOnce sync.Once
	stdinCloseErr  error
}

func startProviderCommand(cmd *exec.Cmd, input []byte) (*providerCommandRun, error) {
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create provider stdin pipe: %w", err)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("create provider stdout pipe: %w", err),
			stdinReader.Close(),
			stdinWriter.Close(),
		)
	}
	cmd.Stdin = stdinReader
	cmd.Stdout = stdoutWriter
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("start provider: %w", err),
			stdinReader.Close(),
			stdinWriter.Close(),
			stdoutReader.Close(),
			stdoutWriter.Close(),
		)
	}
	if err := errors.Join(stdinReader.Close(), stdoutWriter.Close()); err != nil {
		return nil, errors.Join(
			fmt.Errorf("close parent provider pipes: %w", err),
			stdinWriter.Close(),
			stdoutReader.Close(),
			cmd.Process.Kill(),
			cmd.Wait(),
		)
	}

	run := &providerCommandRun{
		stdinWriter:  stdinWriter,
		stdoutReader: stdoutReader,
		inputDone:    make(chan error, 1),
		readDone:     make(chan error, 1),
		waitDone:     make(chan error, 1),
	}
	go func() {
		_, copyErr := io.Copy(stdinWriter, bytes.NewReader(input))
		run.inputDone <- errors.Join(copyErr, run.closeStdin())
	}()
	go func() {
		_, copyErr := io.Copy(&run.outputBuffer, stdoutReader)
		run.readDone <- copyErr
	}()
	go func() { run.waitDone <- cmd.Wait() }()
	return run, nil
}

func (run *providerCommandRun) output(ctx context.Context) ([]byte, error) {
	for run.active() {
		select {
		case waitErr := <-run.waitDone:
			run.waitDone = nil
			if waitErr != nil {
				return run.waitFailure(waitErr)
			}
		case <-run.inputDone:
			// A successful provider may deliberately close or ignore stdin.
			// Treat its side of the pipe closing like os/exec does: input came
			// from an infallible byte reader, so the copy error is not a
			// provider failure.
			run.inputDone = nil
		case run.readErr = <-run.readDone:
			run.readDone = nil
		case <-ctx.Done():
			return run.contextFailure(ctx.Err())
		}
		if err := run.finishInputAfterOutput(); err != nil {
			return run.outputBuffer.Bytes(), err
		}
	}
	return run.finish()
}

func (run *providerCommandRun) active() bool {
	return run.waitDone != nil || run.readDone != nil || run.inputDone != nil
}

func (run *providerCommandRun) closeStdin() error {
	run.stdinCloseOnce.Do(func() { run.stdinCloseErr = run.stdinWriter.Close() })
	return run.stdinCloseErr
}

func (run *providerCommandRun) waitFailure(waitErr error) ([]byte, error) {
	inputCloseErr := run.closeStdin()
	if run.inputDone != nil {
		<-run.inputDone
	}
	closeErr := run.stdoutReader.Close()
	if run.readDone != nil {
		run.readErr = <-run.readDone
	}
	return run.outputBuffer.Bytes(), errors.Join(waitErr, inputCloseErr, closeErr, run.readErr)
}

func (run *providerCommandRun) contextFailure(contextErr error) ([]byte, error) {
	closeErr := errors.Join(run.closeStdin(), run.stdoutReader.Close())
	// Every worker publishes to a buffered channel, so owned-descriptor closure
	// lets it finish without a synchronous drain here. Do not expose partial
	// output: the stdout worker may still be returning from its interrupted copy.
	return nil, errors.Join(contextErr, closeErr)
}

func (run *providerCommandRun) finishInputAfterOutput() error {
	if run.waitDone != nil || run.readDone != nil || run.inputDone == nil {
		return nil
	}
	closeErr := run.closeStdin()
	<-run.inputDone
	run.inputDone = nil
	return closeErr
}

func (run *providerCommandRun) finish() ([]byte, error) {
	closeErr := run.stdoutReader.Close()
	if run.readErr == nil {
		return run.outputBuffer.Bytes(), closeErr
	}
	return run.outputBuffer.Bytes(), errors.Join(
		fmt.Errorf("read provider stdout: %w", run.readErr),
		closeErr,
	)
}
