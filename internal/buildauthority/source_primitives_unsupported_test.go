//go:build !darwin || !arm64

package buildauthority

import "testing"

func TestUnsupportedSourcePrimitiveFactory(t *testing.T) {
	primitives, failure := platformSourcePrimitives()
	if primitives != nil {
		t.Fatalf("unsupported source primitives = %T, want nil", primitives)
	}
	if failure == nil || failure.operation != OperationValidate || failure.cause != CauseUnsupported {
		t.Fatalf("unsupported source primitive failure = %+v, want Validate / Unsupported", failure)
	}
}
