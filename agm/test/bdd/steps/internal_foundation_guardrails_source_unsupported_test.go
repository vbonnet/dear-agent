//go:build !darwin && !linux

package steps

import (
	"context"
	"strings"
	"testing"
)

func TestScanBuildAuthorityWaitOwnershipFailsClosedOnUnsupportedPlatform(t *testing.T) {
	_, err := scanBuildAuthorityWaitOwnership(
		context.Background(), t.TempDir(), defaultBuildAuthorityWaitOwnershipLimits(),
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported on this platform") {
		t.Fatalf("unsupported-platform error = %v", err)
	}
}
