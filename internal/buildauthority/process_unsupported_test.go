//go:build !darwin || !arm64

package buildauthority

import "testing"

func TestProcessSupervisorRefusesUnsupportedPlatform(t *testing.T) {
	supervisor, failure := newProcessSupervisor()
	if supervisor != nil {
		t.Fatal("unsupported platform returned a supervisor")
	}
	if failure == nil || failure.Phase != PhaseAuthority || failure.Operation != OperationValidate ||
		len(failure.Causes) != 1 || failure.Causes[0] != CauseUnsupported {
		t.Fatalf("failure = %+v", failure)
	}
}
