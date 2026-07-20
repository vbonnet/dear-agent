package main

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestExecuteWithSignalContextPropagatesCancellation(t *testing.T) {
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- executeWithSignalContext(context.Background(), func(ctx context.Context) error {
			close(ready)
			<-ctx.Done()
			return ctx.Err()
		}, syscall.SIGUSR1)
	}()
	<-ready
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find current process: %v", err)
	}
	if err := process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatalf("send test signal: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("executeWithSignalContext error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("signal did not cancel root execution context")
	}
}
