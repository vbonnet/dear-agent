//go:build !darwin || !arm64

package buildauthority

func newProcessSupervisor() (processSupervisor, *FailureRecord) {
	return nil, &FailureRecord{
		Phase:     PhaseAuthority,
		Operation: OperationValidate,
		Causes:    []CauseCode{CauseUnsupported},
	}
}
