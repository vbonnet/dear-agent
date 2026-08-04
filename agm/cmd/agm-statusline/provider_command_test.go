package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func newProviderCommandRunForTest(t *testing.T, output string) *providerCommandRun {
	t.Helper()
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		if closeErr := errors.Join(stdinReader.Close(), stdinWriter.Close()); closeErr != nil {
			t.Errorf("close stdin pipe after setup failure: %v", closeErr)
		}
		t.Fatalf("create stdout pipe: %v", err)
	}
	t.Cleanup(func() {
		for _, file := range []*os.File{stdinReader, stdinWriter, stdoutReader, stdoutWriter} {
			if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
				t.Errorf("close test pipe: %v", closeErr)
			}
		}
	})
	return &providerCommandRun{
		stdinWriter:  stdinWriter,
		stdoutReader: stdoutReader,
		outputBuffer: *bytes.NewBufferString(output),
		inputDone:    make(chan error, 1),
		readDone:     make(chan error, 1),
		waitDone:     make(chan error, 1),
	}
}

func TestProviderCommandRunContextFailureDoesNotWaitForWorkers(t *testing.T) {
	run := newProviderCommandRunForTest(t, "partial")
	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, runErr := run.contextFailure(context.DeadlineExceeded)
		done <- result{output: output, err: runErr}
	}()

	select {
	case got := <-done:
		if got.output != nil {
			t.Fatalf("timeout output = %q, want nil while stdout worker may still be exiting", got.output)
		}
		if !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("timeout error = %v, want context deadline", got.err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("context failure waited for worker completion channels")
	}
}

func TestProviderCommandRunPrioritizesExpiredContextOverReadyWorkers(t *testing.T) {
	for i := range 64 {
		t.Run(fmt.Sprintf("ready-workers-%02d", i), func(t *testing.T) {
			run := newProviderCommandRunForTest(t, "late output")
			run.inputDone <- nil
			run.readDone <- nil
			run.waitDone <- nil
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()

			output, runErr := run.output(ctx)
			if output != nil {
				t.Fatalf("expired-context output = %q, want nil", output)
			}
			if !errors.Is(runErr, context.DeadlineExceeded) {
				t.Fatalf("expired-context error = %v, want deadline exceeded", runErr)
			}
		})
	}
}

func TestProviderCommandRunChecksDeadlineBeforeFinalSuccess(t *testing.T) {
	run := newProviderCommandRunForTest(t, "late output")
	run.inputDone = nil
	run.readDone = nil
	run.waitDone = nil
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	output, runErr := run.output(ctx)
	if output != nil {
		t.Fatalf("expired-context output = %q, want nil", output)
	}
	if !errors.Is(runErr, context.DeadlineExceeded) {
		t.Fatalf("expired-context error = %v, want deadline exceeded", runErr)
	}
}
