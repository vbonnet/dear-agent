//go:build linux || darwin

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

func TestSysResourceProbe_Snapshot_FreePhysicalMemoryBytes(t *testing.T) {
	t.Parallel()
	p := NewSysResourceProbe()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	// On Linux and Darwin a running machine reports some free RAM; on other
	// platforms the field is 0 ("unknown"). Either is valid — just assert it
	// is consistent with the used-fraction (free > 0 implies not fully used).
	if snap.FreePhysicalMemoryBytes > 0 && snap.MemoryUsedFraction >= 1 {
		t.Errorf("free=%d bytes but MemoryUsedFraction=%f (>=1)", snap.FreePhysicalMemoryBytes, snap.MemoryUsedFraction)
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

func TestSysResourceProbe_Snapshot_OpenFDFraction(t *testing.T) {
	t.Parallel()
	p := NewSysResourceProbe()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	if snap.OpenFDFraction < 0 || snap.OpenFDFraction > 1 {
		t.Errorf("OpenFDFraction = %f, want 0..1", snap.OpenFDFraction)
	}
	// On Linux and Darwin there are always some open FDs.
	if snap.OpenFDFraction == 0 {
		t.Log("OpenFDFraction = 0 (platform may not support this metric)")
	}
}

func TestSysResourceProbe_Snapshot_VnodeUsedFraction(t *testing.T) {
	t.Parallel()
	p := NewSysResourceProbe()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	if snap.VnodeUsedFraction < 0 || snap.VnodeUsedFraction > 1 {
		t.Errorf("VnodeUsedFraction = %f, want 0..1", snap.VnodeUsedFraction)
	}
}

func TestSysResourceProbe_Snapshot_GoplsProcesses(t *testing.T) {
	t.Parallel()
	p := NewSysResourceProbe()
	snap, err := p.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}
	if snap.GoplsProcesses < 0 {
		t.Errorf("GoplsProcesses = %d, want >= 0", snap.GoplsProcesses)
	}
}
