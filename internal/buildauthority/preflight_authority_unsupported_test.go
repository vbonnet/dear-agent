//go:build !darwin || !arm64

package buildauthority

import "testing"

func TestPreflightAuthorityRevalidatorRefusesUnsupportedPlatform(t *testing.T) {
	revalidator, failure := newPreflightAuthorityRevalidator()
	if revalidator != nil {
		t.Fatal("unsupported platform returned a live authority revalidator")
	}
	requireAuthorityRecord(t, failure, OperationValidate, CauseUnsupported)
}
