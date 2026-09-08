package buildauthority

// quiescenceResult is a sanitized proof result. A false proven value prevents
// task-root removal; causes and child metadata are copied into the refusal.
type quiescenceResult struct {
	proven bool
	causes []CauseCode
	child  *ChildDiagnostic
}

type taskDisposition uint8

const (
	dispositionUnknown taskDisposition = iota
	dispositionPresent
	dispositionRemoved
)

// removalResult carries one of the three proof states produced by owned
// removal or the final no-follow name/identity observation. Causes are
// public-safe classifications, never raw operating-system errors.
type removalResult struct {
	disposition taskDisposition
	causes      []CauseCode
}

// cleanupTransaction owns the complete post-allocation teardown sequence.
// The two root handles are intentionally separate from nonRootClosers so they
// remain available to authorize removal and always close in their fixed order.
type cleanupTransaction struct {
	primary        *FailureRecord
	quiesce        func() quiescenceResult
	nonRootClosers []func() error
	remove         func() removalResult
	observe        func() removalResult
	closeTaskRoot  func() error
	closeStateRoot func() error
	recovery       RecoveryInfo
}

// runCleanup executes the one normative post-allocation transaction. Raw
// callback errors are deliberately reduced to fixed cause codes at this seam.
func runCleanup(transaction cleanupTransaction) error {
	report := FailureReport{Primary: cloneFailureRecord(transaction.primary)}
	quiescenceProven := attemptQuiescence(&report, transaction.quiesce)
	nonRootCloseFailed := closeNonRoots(&report, transaction.nonRootClosers)
	var disposition taskDisposition
	if quiescenceProven && !nonRootCloseFailed {
		disposition = attemptRemoval(&report, transaction.remove)
	} else {
		disposition = observePreservedRoot(&report, transaction.observe)
	}
	disposition = requireObservedIdentityForDisposition(
		&report,
		disposition,
		transaction.recovery.TaskRoot != nil,
	)
	closeOptionalTaskRoot(
		&report,
		transaction.closeTaskRoot,
		transaction.recovery.TaskRoot != nil,
	)
	closeRoot(&report, transaction.closeStateRoot)

	if disposition != dispositionRemoved {
		recovery := transaction.recovery
		report.Recovery = &recovery
	}
	if report.Primary == nil && report.DescriptorClose == nil && report.Cleanup == nil {
		return nil
	}
	return newRefusal(report)
}

func requireObservedIdentityForDisposition(
	report *FailureReport,
	disposition taskDisposition,
	taskRootObserved bool,
) taskDisposition {
	if taskRootObserved || disposition == dispositionUnknown {
		return disposition
	}
	if report.Cleanup == nil {
		report.Cleanup = &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationRemove,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
	} else {
		report.Cleanup.Causes = append(report.Cleanup.Causes, CauseInternalInvariant)
	}
	return dispositionUnknown
}

func closeOptionalTaskRoot(
	report *FailureReport,
	closeDescriptor func() error,
	taskRootObserved bool,
) {
	if closeDescriptor != nil {
		closeRoot(report, closeDescriptor)
		return
	}
	if taskRootObserved {
		closeRoot(report, nil)
	}
}

func attemptQuiescence(report *FailureReport, quiesce func() quiescenceResult) bool {
	quiescence := quiescenceResult{causes: []CauseCode{CauseInternalInvariant}}
	if quiesce != nil {
		quiescence = quiesce()
	}
	if quiescence.proven && (len(quiescence.causes) != 0 || quiescence.child != nil) {
		quiescence.proven = false
		quiescence.causes = append(quiescence.causes, CauseInternalInvariant)
	}
	if quiescence.proven {
		return true
	}
	if len(quiescence.causes) == 0 {
		quiescence.causes = []CauseCode{CauseInternalInvariant}
	}
	report.Cleanup = &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationQuiesce,
		Causes:    append([]CauseCode(nil), quiescence.causes...),
		Child:     cloneChildDiagnostic(quiescence.child),
	}
	return false
}

func closeNonRoots(report *FailureReport, closers []func() error) bool {
	nonRootCloseFailed := false
	for _, closeDescriptor := range closers {
		if closeDescriptor == nil {
			nonRootCloseFailed = true
			addDescriptorFailure(report, OperationCloseNonRoot, CauseDescriptorClose, CauseInternalInvariant)
			continue
		}
		if err := closeDescriptor(); err != nil {
			nonRootCloseFailed = true
			addDescriptorFailure(report, OperationCloseNonRoot, CauseDescriptorClose)
		}
	}
	return nonRootCloseFailed
}

func attemptRemoval(report *FailureReport, remove func() removalResult) taskDisposition {
	if remove == nil {
		report.Cleanup = &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationRemove,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
		return dispositionUnknown
	}
	removal := remove()
	if removal.disposition == dispositionRemoved && len(removal.causes) == 0 {
		return dispositionRemoved
	}
	causes := append([]CauseCode(nil), removal.causes...)
	if len(causes) == 0 {
		causes = append(causes, CauseCleanup)
	}
	if removal.disposition != dispositionPresent && removal.disposition != dispositionUnknown {
		causes = append(causes, CauseInternalInvariant)
		removal.disposition = dispositionUnknown
	}
	report.Cleanup = &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationRemove,
		Causes:    causes,
	}
	return removal.disposition
}

func observePreservedRoot(report *FailureReport, observe func() removalResult) taskDisposition {
	observation := removalResult{
		disposition: dispositionUnknown,
		causes:      []CauseCode{CauseInternalInvariant},
	}
	if observe != nil {
		observation = observe()
	}
	if observation.disposition != dispositionPresent && observation.disposition != dispositionUnknown {
		observation.disposition = dispositionUnknown
		observation.causes = append(observation.causes, CauseInternalInvariant)
	}
	causes := append([]CauseCode(nil), observation.causes...)
	switch {
	case report.Cleanup == nil:
		if len(causes) == 0 {
			causes = append(causes, CauseCleanup)
		}
		report.Cleanup = &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationRemove,
			Causes:    causes,
		}
	case len(causes) != 0:
		report.Cleanup.Causes = append(report.Cleanup.Causes, causes...)
	case observation.disposition == dispositionUnknown:
		report.Cleanup.Causes = append(report.Cleanup.Causes, CauseCleanup)
	}
	return observation.disposition
}

func addDescriptorFailure(report *FailureReport, operation Operation, causes ...CauseCode) {
	if report.DescriptorClose == nil {
		report.DescriptorClose = &FailureRecord{
			Phase:     PhaseClose,
			Operation: operation,
			Causes:    append([]CauseCode(nil), causes...),
		}
		return
	}
	report.DescriptorClose.Causes = append(report.DescriptorClose.Causes, causes...)
}

func closeRoot(report *FailureReport, closeDescriptor func() error) {
	if closeDescriptor == nil {
		addDescriptorFailure(report, OperationCloseRoot, CauseDescriptorClose, CauseInternalInvariant)
		return
	}
	if err := closeDescriptor(); err != nil {
		addDescriptorFailure(report, OperationCloseRoot, CauseDescriptorClose)
	}
}

func cloneChildDiagnostic(child *ChildDiagnostic) *ChildDiagnostic {
	if child == nil {
		return nil
	}
	cloned := *child
	return &cloned
}
