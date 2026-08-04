package main

import (
	"os"
	"syscall"
	"testing"
)

func TestExitCodeForSignal(t *testing.T) {
	for _, test := range []struct {
		signal os.Signal
		want   int32
	}{
		{syscall.SIGHUP, 129},
		{os.Interrupt, 130},
		{syscall.SIGTERM, 143},
	} {
		if got := exitCodeForSignal(test.signal); got != test.want {
			t.Fatalf("exitCodeForSignal(%v) = %d, want %d", test.signal, got, test.want)
		}
	}
}

func TestRunRejectsInvalidArgumentsBeforeInstallation(t *testing.T) {
	if got := run(nil); got != 2 {
		t.Fatalf("run(nil) = %d, want 2", got)
	}
}
