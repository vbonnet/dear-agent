//go:build !darwin || !arm64

package buildauthority

import (
	"context"
	"testing"
)

func TestUnsupportedAuthorityCapabilitiesRefuse(t *testing.T) {
	t.Parallel()

	_, err := retainPhysicalRoot(context.Background())
	requirePrivateCauses(t, err, CauseUnsupported)
	_, err = retainNullDevice(context.Background())
	requirePrivateCauses(t, err, CauseUnsupported)
	_, err = openFixedNullDeviceNoFollow(0)
	requirePrivateCauses(t, err, CauseUnsupported)
	requirePrivateCauses(t, validateNullDeviceSnapshot(fileSnapshot{}), CauseUnsupported)
}
