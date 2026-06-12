package supervisor

import (
	"context"
	"testing"
)

func TestSysResourceProbe_Snapshot_DiskFraction(t *testing.T) {
	t.Parallel()
	p := NewSysResourceProbe()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	if snap.DiskUsedFraction < 0 || snap.DiskUsedFraction > 1 {
		t.Errorf("DiskUsedFraction = %f, want 0..1", snap.DiskUsedFraction)
	}
	// Expect some disk to be in use on any real filesystem.
	if snap.DiskUsedFraction == 0 {
		t.Error("DiskUsedFraction = 0, expected >0 on a real filesystem")
	}
}

func TestSysResourceProbe_Snapshot_MemoryFraction(t *testing.T) {
	t.Parallel()
	p := NewSysResourceProbe()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	if snap.MemoryUsedFraction < 0 || snap.MemoryUsedFraction > 1 {
		t.Errorf("MemoryUsedFraction = %f, want 0..1", snap.MemoryUsedFraction)
	}
}

func TestSysResourceProbe_Snapshot_CustomPath(t *testing.T) {
	t.Parallel()
	p := &SysResourceProbe{DiskPath: t.TempDir()}
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot(TempDir) error: %v", err)
	}
	// A tmpfs/tmpdir is still on a real filesystem.
	if snap.DiskUsedFraction < 0 || snap.DiskUsedFraction > 1 {
		t.Errorf("DiskUsedFraction = %f, want 0..1", snap.DiskUsedFraction)
	}
}

func TestSysResourceProbe_Snapshot_EmptyPath(t *testing.T) {
	t.Parallel()
	// Empty DiskPath should default to "/" without error.
	p := &SysResourceProbe{DiskPath: ""}
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot(empty path) error: %v", err)
	}
	if snap.DiskUsedFraction <= 0 {
		t.Errorf("DiskUsedFraction = %f, expected >0 for '/'", snap.DiskUsedFraction)
	}
}

func TestSysResourceProbe_ImplementsResourceProbe(t *testing.T) {
	t.Parallel()
	var _ ResourceProbe = (*SysResourceProbe)(nil)
}

func TestSysResourceProbe_CPUAlwaysZero(t *testing.T) {
	t.Parallel()
	p := NewSysResourceProbe()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	// CPU sampling not implemented — always 0 in this version.
	if snap.CPUUsedFraction != 0 {
		t.Errorf("CPUUsedFraction = %f, want 0 (not yet implemented)", snap.CPUUsedFraction)
	}
}
