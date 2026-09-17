package harnessexec

import (
	"errors"
	"strings"
	"testing"
)

func TestRunForPlatformRejectsFreeBSDExecutionBeforeDispatch(t *testing.T) {
	t.Parallel()

	for _, protocol := range []string{
		CodexProtocol,
		ClaudeProtocol,
		HarnessProtocol,
		AgyProtocol,
	} {
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()
			err := runForPlatform("freebsd", protocol, nil)
			if !errors.Is(err, errFreeBSDPrivateHarnessExecution) {
				t.Fatalf("runForPlatform() error = %v, want FreeBSD execution refusal", err)
			}
		})
	}
}

func TestRunForPlatformPreservesNonExecutionDispatch(t *testing.T) {
	t.Parallel()

	if err := runForPlatform("freebsd", ExpiryProtocol, nil); err == nil ||
		errors.Is(err, errFreeBSDPrivateHarnessExecution) ||
		!strings.Contains(err.Error(), "expiration request") {
		t.Fatalf("FreeBSD expiry dispatch error = %v, want ordinary request validation", err)
	}
	if err := runForPlatform("freebsd", "new", nil); err == nil ||
		errors.Is(err, errFreeBSDPrivateHarnessExecution) ||
		!strings.Contains(err.Error(), "unsupported private harness protocol") {
		t.Fatalf("FreeBSD unknown protocol error = %v, want ordinary protocol validation", err)
	}
}

func TestFreeBSDPlatformPolicyFailsClosed(t *testing.T) {
	t.Parallel()

	for _, protocol := range []string{
		CodexProtocol,
		ClaudeProtocol,
		HarnessProtocol,
		AgyProtocol,
		"__exec-future-protocol",
	} {
		t.Run(protocol, func(t *testing.T) {
			t.Parallel()
			err := privateHarnessExecutionPlatformError("freebsd", protocol)
			if !errors.Is(err, errFreeBSDPrivateHarnessExecution) {
				t.Fatalf("platform error = %v, want fail-closed FreeBSD refusal", err)
			}
		})
	}

	if err := privateHarnessExecutionPlatformError("freebsd", ExpiryProtocol); err != nil {
		t.Fatalf("FreeBSD expiry platform error = %v, want explicit exception", err)
	}
}
