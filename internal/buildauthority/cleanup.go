package buildauthority

// quiescenceResult is a sanitized proof result. A false proven value prevents
// task-root removal; causes and child metadata are copied into the refusal.
type quiescenceResult struct {
	proven bool
	causes []CauseCode
	child  *ChildDiagnostic
}

// removalResult distinguishes a fully removed task child from every
// preservation outcome. Causes are public-safe classifications, never raw
// operating-system errors.
type removalResult struct {
	removed bool
	causes  []CauseCode
}

// cleanupTransaction owns the complete post-allocation teardown sequence.
// The two root handles are intentionally separate from nonRootClosers so they
// remain available to authorize removal and always close in their fixed order.
type cleanupTransaction struct {
	primary        *FailureRecord
	quiesce        func() quiescenceResult
	nonRootClosers []func() error
	remove         func() removalResult
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
	removed := false
	if quiescenceProven && !nonRootCloseFailed {
		removed = attemptRemoval(&report, transaction.remove)
	}
	closeRoot(&report, transaction.closeTaskRoot)
	closeRoot(&report, transaction.closeStateRoot)

	if !removed {
		recovery := transaction.recovery
		report.Recovery = &recovery
	}
	if report.Primary == nil && report.DescriptorClose == nil && report.Cleanup == nil {
		return nil
	}
	return newRefusal(report)
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

func attemptRemoval(report *FailureReport, remove func() removalResult) bool {
	if remove == nil {
		report.Cleanup = &FailureRecord{
			Phase:     PhaseClose,
			Operation: OperationRemove,
			Causes:    []CauseCode{CauseInternalInvariant},
		}
		return false
	}
	removal := remove()
	if removal.removed && len(removal.causes) == 0 {
		return true
	}
	causes := append([]CauseCode(nil), removal.causes...)
	if len(causes) == 0 {
		causes = append(causes, CauseCleanup)
	}
	if removal.removed {
		causes = append(causes, CauseInternalInvariant)
	}
	report.Cleanup = &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationRemove,
		Causes:    causes,
	}
	return removal.removed
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
