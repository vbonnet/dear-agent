//go:build darwin && arm64

package buildauthority

import (
	"context"
	"testing"
)

func TestPhysicalRootSentinelProductionPathUsesRetainedRoot(t *testing.T) {
	root, err := retainPhysicalRoot(context.Background())
	if err != nil {
		t.Fatalf("retain physical root: %v", err)
	}
	defer func() {
		if err := root.close(); err != nil {
			t.Errorf("close physical root: %v", err)
		}
	}()
	claim, failure := admitPhysicalRootSentinels(context.Background(), root)
	if failure != nil {
		t.Fatalf("admit physical-root sentinels: %+v", failure)
	}
	if failure := claim.revalidate(context.Background()); failure != nil {
		t.Fatalf("revalidate physical-root sentinels: %+v", failure)
	}
}
