//go:build !darwin || !arm64

package buildauthority

import "testing"

func TestUnsupportedRetainedTaskRemovalPrimitives(t *testing.T) {
	t.Parallel()

	requirePrivateCauses(t, removeRetainedDirectoryAt(0, "task"), CauseUnsupported)
	_, err := retainedDescriptorPath(nil)
	requirePrivateCauses(t, err, CauseUnsupported)
}
