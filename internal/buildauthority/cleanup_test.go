package buildauthority

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCleanupTransactionSuccessOrder(t *testing.T) {
	t.Parallel()

	var actions []string
	transaction := cleanupTransaction{
		quiesce: func() quiescenceResult {
			actions = append(actions, "quiesce")
			return quiescenceResult{proven: true}
		},
		nonRootClosers: []func() error{
			func() error { actions = append(actions, "close-a"); return nil },
			func() error { actions = append(actions, "close-b"); return nil },
		},
		remove: func() removalResult {
			actions = append(actions, "remove")
			return removalResult{disposition: dispositionRemoved}
		},
		closeTaskRoot: func() error {
			actions = append(actions, "close-task-root")
			return nil
		},
		closeStateRoot: func() error {
			actions = append(actions, "close-state-root")
			return nil
		},
		recovery: testRecoveryInfo(),
	}

	if err := runCleanup(transaction); err != nil {
		t.Fatalf("runCleanup() = %v, want nil", err)
	}
	want := []string{
		"quiesce",
		"close-a",
		"close-b",
		"remove",
		"close-task-root",
		"close-state-root",
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("actions = %#v, want %#v", actions, want)
	}
}

func TestCleanupUncertainQuiescencePreservesRootButStillClosesDescriptors(t *testing.T) {
	t.Parallel()

	var actions []string
	recovery := testRecoveryInfo()
	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult {
			actions = append(actions, "quiesce")
			return quiescenceResult{
				causes: []CauseCode{CauseChildSurvivor},
				child:  &ChildDiagnostic{DiagnosticBytes: 17},
			}
		},
		nonRootClosers: []func() error{
			func() error { actions = append(actions, "close-a"); return nil },
			func() error { actions = append(actions, "close-b"); return nil },
		},
		observe: func() removalResult {
			actions = append(actions, "observe")
			return removalResult{disposition: dispositionPresent}
		},
		closeTaskRoot: func() error {
			actions = append(actions, "close-task-root")
			return nil
		},
		closeStateRoot: func() error {
			actions = append(actions, "close-state-root")
			return nil
		},
		recovery: recovery,
	})

	report := refusalReport(t, err)
	wantActions := []string{
		"quiesce",
		"close-a",
		"close-b",
		"observe",
		"close-task-root",
		"close-state-root",
	}
	if !reflect.DeepEqual(actions, wantActions) {
		t.Fatalf("actions = %#v, want %#v", actions, wantActions)
	}
	if report.Primary != nil {
		t.Fatalf("Primary = %#v, want nil", report.Primary)
	}
	if report.Cleanup == nil || report.Cleanup.Phase != PhaseClose || report.Cleanup.Operation != OperationQuiesce {
		t.Fatalf("Cleanup = %#v, want close/quiesce", report.Cleanup)
	}
	if got := report.Cleanup.Causes; !reflect.DeepEqual(got, []CauseCode{CauseChildSurvivor}) {
		t.Fatalf("Cleanup causes = %#v", got)
	}
	if report.Cleanup.Child == nil || report.Cleanup.Child.DiagnosticBytes != 17 {
		t.Fatalf("Cleanup child = %#v", report.Cleanup.Child)
	}
	if report.Recovery == nil || !reflect.DeepEqual(*report.Recovery, recovery) {
		t.Fatalf("Recovery = %#v, want %#v", report.Recovery, recovery)
	}
}

func TestCleanupUnobservedTaskIdentityCanOnlyRemainUnknown(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		disposition taskDisposition
		causes      []CauseCode
	}{
		{name: "present", disposition: dispositionPresent, causes: []CauseCode{CauseCleanup, CauseInternalInvariant}},
		{name: "removed", disposition: dispositionRemoved, causes: []CauseCode{CauseInternalInvariant}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stateClosed := false
			err := runCleanup(cleanupTransaction{
				quiesce: func() quiescenceResult { return quiescenceResult{proven: true} },
				remove: func() removalResult {
					return removalResult{disposition: test.disposition}
				},
				closeStateRoot: func() error {
					stateClosed = true
					return nil
				},
				recovery: RecoveryInfo{
					TaskPath:  "/private/state/.sandbox-gc-build-unobserved",
					StateRoot: FileIdentity{Device: 1, Inode: 2},
				},
			})
			report := refusalReport(t, err)
			if !stateClosed {
				t.Fatal("StateRoot was not closed")
			}
			if report.DescriptorClose != nil {
				t.Fatalf("DescriptorClose = %#v, want nil for absent task-root handle", report.DescriptorClose)
			}
			if report.Cleanup == nil || !reflect.DeepEqual(report.Cleanup.Causes, test.causes) {
				t.Fatalf("Cleanup = %#v", report.Cleanup)
			}
			if report.Recovery == nil || report.Recovery.TaskRoot != nil {
				t.Fatalf("Recovery = %#v, want unobserved task root", report.Recovery)
			}
		})
	}
}

func TestCleanupNonRootCloseFailureBlocksRemovalAndClosesEveryRoot(t *testing.T) {
	t.Parallel()

	secret := errors.New("secret descriptor /private/path")
	var actions []string
	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult {
			actions = append(actions, "quiesce")
			return quiescenceResult{proven: true}
		},
		nonRootClosers: []func() error{
			func() error { actions = append(actions, "close-a"); return secret },
			func() error { actions = append(actions, "close-b"); return nil },
		},
		observe: func() removalResult {
			actions = append(actions, "observe")
			return removalResult{disposition: dispositionPresent}
		},
		closeTaskRoot: func() error {
			actions = append(actions, "close-task-root")
			return nil
		},
		closeStateRoot: func() error {
			actions = append(actions, "close-state-root")
			return nil
		},
		recovery: testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	wantActions := []string{
		"quiesce",
		"close-a",
		"close-b",
		"observe",
		"close-task-root",
		"close-state-root",
	}
	if !reflect.DeepEqual(actions, wantActions) {
		t.Fatalf("actions = %#v, want %#v", actions, wantActions)
	}
	if report.DescriptorClose == nil || report.DescriptorClose.Operation != OperationCloseNonRoot {
		t.Fatalf("DescriptorClose = %#v", report.DescriptorClose)
	}
	if report.Cleanup == nil || report.Cleanup.Operation != OperationRemove {
		t.Fatalf("Cleanup = %#v, want retained-root disposition refusal", report.Cleanup)
	}
	if got := report.Cleanup.Causes; !reflect.DeepEqual(got, []CauseCode{CauseCleanup}) {
		t.Fatalf("Cleanup causes = %#v, want cleanup", got)
	}
	if report.Recovery == nil {
		t.Fatal("Recovery missing for retained task root")
	}
	if strings.Contains(err.Error(), secret.Error()) {
		t.Fatalf("Error() exposed raw close cause: %q", err)
	}
}

func TestCleanupRemovalFailureAndRootCloseFailuresOccupyDeterministicSlots(t *testing.T) {
	t.Parallel()

	primary := &FailureRecord{
		Phase:     PhaseVerifyAGM,
		Operation: OperationCompare,
		Causes:    []CauseCode{CauseUnstable},
	}
	var actions []string
	err := runCleanup(cleanupTransaction{
		primary: primary,
		quiesce: func() quiescenceResult {
			actions = append(actions, "quiesce")
			return quiescenceResult{proven: true}
		},
		nonRootClosers: []func() error{
			func() error { actions = append(actions, "close-a"); return nil },
		},
		remove: func() removalResult {
			actions = append(actions, "remove")
			return removalResult{
				disposition: dispositionPresent,
				causes:      []CauseCode{CauseIdentity},
			}
		},
		closeTaskRoot: func() error {
			actions = append(actions, "close-task-root")
			return errors.New("task-root close")
		},
		closeStateRoot: func() error {
			actions = append(actions, "close-state-root")
			return errors.New("state-root close")
		},
		recovery: testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	wantActions := []string{
		"quiesce",
		"close-a",
		"remove",
		"close-task-root",
		"close-state-root",
	}
	if !reflect.DeepEqual(actions, wantActions) {
		t.Fatalf("actions = %#v, want %#v", actions, wantActions)
	}
	if report.Primary == nil || report.Primary.Phase != PhaseVerifyAGM {
		t.Fatalf("Primary = %#v", report.Primary)
	}
	if report.DescriptorClose == nil || report.DescriptorClose.Operation != OperationCloseRoot {
		t.Fatalf("DescriptorClose = %#v", report.DescriptorClose)
	}
	if report.Cleanup == nil || report.Cleanup.Operation != OperationRemove {
		t.Fatalf("Cleanup = %#v", report.Cleanup)
	}
	if report.Recovery == nil {
		t.Fatal("Recovery missing for failed removal")
	}
	const want = "buildauthority: primary=verify-agm/compare/unstable; descriptor-close=close/close-root/descriptor-close; cleanup=close/remove/identity"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCleanupSuccessfulRemovalHasNoRecoveryEvenWhenRootCloseFails(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult { return quiescenceResult{proven: true} },
		remove:  func() removalResult { return removalResult{disposition: dispositionRemoved} },
		closeTaskRoot: func() error {
			return errors.New("closed task handle failed")
		},
		closeStateRoot: func() error { return nil },
		recovery:       testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	if report.DescriptorClose == nil || report.DescriptorClose.Operation != OperationCloseRoot {
		t.Fatalf("DescriptorClose = %#v", report.DescriptorClose)
	}
	if report.Recovery != nil {
		t.Fatalf("Recovery = %#v after successful task-root removal", report.Recovery)
	}
}

func TestCleanupQuiescenceFailurePreservesAnExistingPrimary(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		primary: &FailureRecord{
			Phase:     PhaseBuildAGM,
			Operation: OperationExecute,
			Causes:    []CauseCode{CauseChildExit},
		},
		quiesce: func() quiescenceResult {
			return quiescenceResult{causes: []CauseCode{CauseChildSurvivor}}
		},
		observe:        func() removalResult { return removalResult{disposition: dispositionPresent} },
		closeTaskRoot:  func() error { return nil },
		closeStateRoot: func() error { return nil },
		recovery:       testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	if report.Primary == nil || report.Primary.Phase != PhaseBuildAGM || report.Primary.Operation != OperationExecute {
		t.Fatalf("Primary replaced: %#v", report.Primary)
	}
	wantCauses := []CauseCode{CauseChildExit}
	if !reflect.DeepEqual(report.Primary.Causes, wantCauses) {
		t.Fatalf("Primary causes = %#v, want %#v", report.Primary.Causes, wantCauses)
	}
	if report.Cleanup == nil || report.Cleanup.Operation != OperationQuiesce {
		t.Fatalf("Cleanup = %#v, want quiescence refusal", report.Cleanup)
	}
	if got := report.Cleanup.Causes; !reflect.DeepEqual(got, []CauseCode{CauseChildSurvivor}) {
		t.Fatalf("Cleanup causes = %#v", got)
	}
	if report.Recovery == nil {
		t.Fatal("Recovery missing after quiescence uncertainty")
	}
}

func TestCleanupDescriptorSlotUsesEarliestFailingClosureCategory(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult { return quiescenceResult{proven: true} },
		nonRootClosers: []func() error{
			func() error { return errors.New("non-root") },
		},
		observe:        func() removalResult { return removalResult{disposition: dispositionPresent} },
		closeTaskRoot:  func() error { return errors.New("task-root") },
		closeStateRoot: func() error { return errors.New("state-root") },
		recovery:       testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	if report.DescriptorClose == nil || report.DescriptorClose.Operation != OperationCloseNonRoot {
		t.Fatalf("DescriptorClose = %#v, want earliest close-nonroot category", report.DescriptorClose)
	}
	if report.Cleanup == nil || report.Cleanup.Operation != OperationRemove {
		t.Fatalf("Cleanup = %#v, want retained-root disposition refusal", report.Cleanup)
	}
}

func TestCleanupRendersDescriptorBeforeEarlierQuiescenceFailure(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult {
			return quiescenceResult{causes: []CauseCode{CauseChildSurvivor}}
		},
		nonRootClosers: []func() error{
			func() error { return errors.New("non-root") },
		},
		observe:        func() removalResult { return removalResult{disposition: dispositionPresent} },
		closeTaskRoot:  func() error { return nil },
		closeStateRoot: func() error { return nil },
		recovery:       testRecoveryInfo(),
	})

	const want = "buildauthority: descriptor-close=close/close-nonroot/descriptor-close; cleanup=close/quiesce/child-survivor"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestCleanupMissingFinalObservationSealsUnknownDisposition(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult {
			return quiescenceResult{causes: []CauseCode{CauseChildWait}}
		},
		closeTaskRoot:  func() error { return nil },
		closeStateRoot: func() error { return nil },
		recovery:       testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	if report.Cleanup == nil || report.Cleanup.Operation != OperationQuiesce {
		t.Fatalf("Cleanup = %#v, want close/quiesce", report.Cleanup)
	}
	wantCauses := []CauseCode{CauseChildWait, CauseInternalInvariant}
	if got := report.Cleanup.Causes; !reflect.DeepEqual(got, wantCauses) {
		t.Fatalf("Cleanup causes = %#v, want %#v", got, wantCauses)
	}
	if report.Recovery == nil {
		t.Fatal("Recovery missing for unknown disposition")
	}
}

func TestCleanupRejectsRemovedFinalObservation(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult {
			return quiescenceResult{causes: []CauseCode{CauseChildSurvivor}}
		},
		observe: func() removalResult {
			return removalResult{disposition: dispositionRemoved}
		},
		closeTaskRoot:  func() error { return nil },
		closeStateRoot: func() error { return nil },
		recovery:       testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	if report.Cleanup == nil || report.Cleanup.Operation != OperationQuiesce {
		t.Fatalf("Cleanup = %#v, want close/quiesce", report.Cleanup)
	}
	wantCauses := []CauseCode{CauseChildSurvivor, CauseInternalInvariant}
	if got := report.Cleanup.Causes; !reflect.DeepEqual(got, wantCauses) {
		t.Fatalf("Cleanup causes = %#v, want %#v", got, wantCauses)
	}
	if report.Recovery == nil {
		t.Fatal("Recovery missing after invalid removed observation")
	}
}

func TestCleanupRejectsRemovedDispositionWithFailureCause(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult { return quiescenceResult{proven: true} },
		remove: func() removalResult {
			return removalResult{
				disposition: dispositionRemoved,
				causes:      []CauseCode{CauseIdentity},
			}
		},
		closeTaskRoot:  func() error { return nil },
		closeStateRoot: func() error { return nil },
		recovery:       testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	if report.Cleanup == nil || report.Cleanup.Operation != OperationRemove {
		t.Fatalf("Cleanup = %#v, want close/remove", report.Cleanup)
	}
	wantCauses := []CauseCode{CauseIdentity, CauseInternalInvariant}
	if got := report.Cleanup.Causes; !reflect.DeepEqual(got, wantCauses) {
		t.Fatalf("Cleanup causes = %#v, want %#v", got, wantCauses)
	}
	if report.Recovery == nil {
		t.Fatal("Recovery missing after contradictory removal result")
	}
}

func TestCleanupUnknownObservationWithoutCauseRecordsCleanup(t *testing.T) {
	t.Parallel()

	err := runCleanup(cleanupTransaction{
		quiesce: func() quiescenceResult { return quiescenceResult{proven: true} },
		nonRootClosers: []func() error{
			func() error { return errors.New("non-root") },
		},
		observe: func() removalResult {
			return removalResult{disposition: dispositionUnknown}
		},
		closeTaskRoot:  func() error { return nil },
		closeStateRoot: func() error { return nil },
		recovery:       testRecoveryInfo(),
	})

	report := refusalReport(t, err)
	if report.Cleanup == nil || report.Cleanup.Operation != OperationRemove {
		t.Fatalf("Cleanup = %#v, want close/remove", report.Cleanup)
	}
	if got := report.Cleanup.Causes; !reflect.DeepEqual(got, []CauseCode{CauseCleanup}) {
		t.Fatalf("Cleanup causes = %#v, want cleanup", got)
	}
	if report.Recovery == nil {
		t.Fatal("Recovery missing for unknown disposition")
	}
}

func refusalReport(t *testing.T, err error) FailureReport {
	t.Helper()
	if err == nil {
		t.Fatal("runCleanup() = nil, want typed refusal")
	}
	var refusal Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("runCleanup() error %T does not implement Refusal", err)
	}
	return refusal.Report()
}

func testRecoveryInfo() RecoveryInfo {
	return RecoveryInfo{
		TaskPath: "/private/state/.sandbox-gc-build-task",
		StateRoot: FileIdentity{
			Device:     1,
			Inode:      2,
			UID:        501,
			Mode:       0o40700,
			Filesystem: [2]int32{3, 4},
		},
		TaskRoot: &FileIdentity{
			Device:     1,
			Inode:      5,
			UID:        501,
			Mode:       0o40700,
			Filesystem: [2]int32{3, 4},
		},
	}
}
