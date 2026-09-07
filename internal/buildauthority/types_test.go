package buildauthority

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRefusalReportIsDeepCopiedAndRecoveryIsNotRendered(t *testing.T) {
	t.Parallel()

	digest := Digest{0xab}
	err := newRefusal(FailureReport{
		Primary: &FailureRecord{
			Phase:     PhaseBuildAGM,
			Operation: OperationExecute,
			Causes: []CauseCode{
				CauseChildExit,
				CauseCanceled,
				CauseChildExit,
			},
			Child: &ChildDiagnostic{
				ExitStatusObserved: true,
				ExitStatus:         137,
				DiagnosticBytes:    42,
				Truncated:          true,
				DiagnosticSHA256:   digest,
			},
		},
		DescriptorClose: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationCloseNonRoot,
			Causes:    []CauseCode{CauseDescriptorClose},
		},
		Recovery: &RecoveryInfo{
			TaskPath: "/private/state/.sandbox-gc-build-secret",
			StateRoot: FileIdentity{
				Device:     1,
				Inode:      2,
				UID:        501,
				Mode:       0o40700,
				Filesystem: [2]int32{3, 4},
			},
			TaskRoot: FileIdentity{
				Device:     1,
				Inode:      5,
				UID:        501,
				Mode:       0o40700,
				Filesystem: [2]int32{3, 4},
			},
		},
	})

	const want = "buildauthority: primary=build-agm/execute/canceled,child-exit child(exit=137,bytes=42,truncated=true,sha256=ab00000000000000000000000000000000000000000000000000000000000000); descriptor-close=close/close-nonroot/descriptor-close"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	for _, secret := range []string{"/private/state", "sandbox-gc-build-secret", "501"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("Error() exposed recovery value %q: %q", secret, err)
		}
	}

	var refusal Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("errors.As(%T) did not recover Refusal", err)
	}
	first := refusal.Report()
	if first.Recovery == nil || first.Recovery.TaskPath == "" {
		t.Fatal("typed report omitted recovery information")
	}
	first.Primary.Causes[0] = CauseInternalInvariant
	first.Recovery.TaskPath = "mutated"
	second := refusal.Report()
	if got := second.Primary.Causes; len(got) != 2 || got[0] != CauseCanceled || got[1] != CauseChildExit {
		t.Fatalf("Report causes were aliased or noncanonical: %#v", got)
	}
	if got := second.Recovery.TaskPath; got != "/private/state/.sandbox-gc-build-secret" {
		t.Fatalf("Report recovery was aliased: %q", got)
	}
}

func TestRefusalRendersMissingExitStatusAsNone(t *testing.T) {
	t.Parallel()

	err := newRefusal(FailureReport{
		Primary: &FailureRecord{
			Phase:     PhaseAuthority,
			Operation: OperationProbe,
			Causes:    []CauseCode{CauseChildStart},
			Child: &ChildDiagnostic{
				DiagnosticSHA256: Digest{},
			},
		},
	})
	const want = "buildauthority: primary=authority/probe/child-start child(exit=none,bytes=0,truncated=false,sha256=0000000000000000000000000000000000000000000000000000000000000000)"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCopiedPairSharesOneStableClose(t *testing.T) {
	t.Parallel()

	closeErr := newRefusal(FailureReport{
		Cleanup: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationRemove,
			Causes:    []CauseCode{CauseCleanup},
		},
	})
	var calls atomic.Int32
	receipt := Receipt{
		AuthorityVersion: AuthorityVersion,
		Revision:         strings.Repeat("a", 40),
		Platform:         PlatformDarwinARM64,
		Roles: [2]RoleReceipt{
			{OutputName: "agm", MainPackage: agmMainPackage},
			{OutputName: "disk-watchdog", MainPackage: diskWatchdogMainPackage},
		},
	}
	one := newStagedPair(receipt, func() error {
		calls.Add(1)
		return closeErr
	})
	two := one

	const goroutines = 32
	results := make(chan error, goroutines)
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				results <- one.Close()
				return
			}
			results <- two.Close()
		}(i)
	}
	wg.Wait()
	close(results)

	if got := calls.Load(); got != 1 {
		t.Fatalf("cleanup called %d times, want 1", got)
	}
	for got := range results {
		var gotRefusal *failureError
		if !errors.As(got, &gotRefusal) || gotRefusal != closeErr {
			t.Fatalf("Close() error identity = %p, want %p", got, closeErr)
		}
	}
	if got := one.Receipt(); got != receipt {
		t.Fatalf("Receipt changed after Close: %#v", got)
	}
	if got := two.Receipt(); got != receipt {
		t.Fatalf("copied interface Receipt changed after Close: %#v", got)
	}
}

func TestConcurrentCloseCallersWaitForTheStableResult(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})
	closeErr := newRefusal(FailureReport{
		Cleanup: &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationRemove,
			Causes:    []CauseCode{CauseCleanup},
		},
	})
	pair := newStagedPair(Receipt{}, func() error {
		close(started)
		<-release
		return closeErr
	})

	const callers = 16
	results := make(chan error, callers)
	for range callers {
		go func() { results <- pair.Close() }()
	}
	<-started
	select {
	case got := <-results:
		t.Fatalf("Close returned %v before cleanup completed", got)
	default:
	}
	close(release)
	for i := range callers {
		got := <-results
		var gotRefusal *failureError
		if !errors.As(got, &gotRefusal) || gotRefusal != closeErr {
			t.Fatalf("Close result %d = %p, want %p", i, got, closeErr)
		}
	}
}
