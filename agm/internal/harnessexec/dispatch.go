package harnessexec

import (
	"errors"
	"fmt"
	"runtime"
)

var errFreeBSDPrivateHarnessExecution = errors.New("private harness execution is unsupported on freebsd")

// Run validates a private protocol request and replaces the current AGM
// process with the fixed harness executable. A successful call does not return.
func Run(protocol string, args []string) error {
	return runForPlatform(runtime.GOOS, protocol, args)
}

func runForPlatform(goos, protocol string, args []string) error {
	if !IsProtocol(protocol) {
		return fmt.Errorf("unsupported private harness protocol %q", protocol)
	}
	if err := privateHarnessExecutionPlatformError(goos, protocol); err != nil {
		return err
	}

	switch protocol {
	case CodexProtocol:
		return runCodex(args)
	case ClaudeProtocol:
		return runClaude(args)
	case HarnessProtocol:
		return runHarness(args)
	case AgyProtocol:
		return runAgy(args)
	case ExpiryProtocol:
		return runExpiry(args)
	default:
		return fmt.Errorf("unsupported private harness protocol %q", protocol)
	}
}

func privateHarnessExecutionPlatformError(goos, protocol string) error {
	if goos != "freebsd" || protocol == ExpiryProtocol {
		return nil
	}
	return fmt.Errorf("%w: protocol %q", errFreeBSDPrivateHarnessExecution, protocol)
}
