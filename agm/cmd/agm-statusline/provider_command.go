package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// outputProviderCommand returns stdout only when the command and its inherited
// stdout writers finish before ctx. The pipe is caller-owned rather than an
// os/exec copying pipe, so ctx can close the reader at the configured deadline
// without imposing an earlier post-process WaitDelay. CommandContext still
// terminates only the direct process; this does not claim descendant cleanup.
func outputProviderCommand(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create provider stdout pipe: %w", err)
	}
	cmd.Stdout = stdoutWriter
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("start provider: %w", err),
			stdoutReader.Close(),
			stdoutWriter.Close(),
		)
	}
	if err := stdoutWriter.Close(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("close parent provider stdout: %w", err),
			cmd.Process.Kill(),
			cmd.Wait(),
			stdoutReader.Close(),
		)
	}

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
			if waitErr != nil {
				closeErr := stdoutReader.Close()
				if readDone != nil {
					readErr = <-readDone
				}
				return output.Bytes(), errors.Join(waitErr, closeErr, readErr)
			}
		case readErr = <-readDone:
			readDone = nil
		case <-ctx.Done():
			closeErr := stdoutReader.Close()
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
