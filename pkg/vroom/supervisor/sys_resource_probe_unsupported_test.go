//go:build !linux && !darwin

package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestSysResourceProbeRejectsUnsupportedPlatform(t *testing.T) {
	snapshot, err := NewSysResourceProbe().Snapshot(context.Background())
	if snapshot != (ResourceSnapshot{}) {
		t.Fatalf("snapshot = %+v, want empty", snapshot)
	}
	if !errors.Is(err, errSysResourceProbeUnsupported) {
		t.Fatalf("error = %v, want unsupported resource probe", err)
	}
}
