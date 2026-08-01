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

	var (
		stdinCloseOnce sync.Once
		stdinCloseErr  error
	)
	closeStdin := func() error {
		stdinCloseOnce.Do(func() { stdinCloseErr = stdinWriter.Close() })
		return stdinCloseErr
	}
	inputDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(stdinWriter, bytes.NewReader(input))
		inputDone <- errors.Join(copyErr, closeStdin())
	}()

	var output bytes.Buffer
	readDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&output, stdoutReader)
		readDone <- copyErr
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	var readErr error
	for waitDone != nil || readDone != nil {
		select {
		case waitErr := <-waitDone:
			waitDone = nil
			inputCloseErr := closeStdin()
			if inputDone != nil {
				<-inputDone
				inputDone = nil
			}
			if waitErr != nil {
				closeErr := stdoutReader.Close()
				if readDone != nil {
					readErr = <-readDone
				}
				return output.Bytes(), errors.Join(waitErr, inputCloseErr, closeErr, readErr)
			}
		case readErr = <-readDone:
			readDone = nil
		case <-ctx.Done():
			closeErr := errors.Join(closeStdin(), stdoutReader.Close())
			if inputDone != nil {
				<-inputDone
				inputDone = nil
			}
			if waitDone != nil {
				<-waitDone
			}
			if readDone != nil {
				<-readDone
			}
			return output.Bytes(), errors.Join(ctx.Err(), closeErr)
		}
	}

	closeErr := stdoutReader.Close()
	if readErr != nil {
		return output.Bytes(), errors.Join(
			fmt.Errorf("read provider stdout: %w", readErr),
			closeErr,
		)
	}
	return output.Bytes(), closeErr
}
