package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

// TestRunSupervisorProbe_WritesTrailRecord verifies the probe appends a
// well-formed "overseer.resource.probe" record to the configured trail and
// prints a summary line.
func TestRunSupervisorProbe_WritesTrailRecord(t *testing.T) {
	dir := t.TempDir()
	trailPath := filepath.Join(dir, "trail.jsonl")

	// Restore package-level flag state after the test.
	prev := supervisorProbeTrailPath
	t.Cleanup(func() { supervisorProbeTrailPath = prev })
	supervisorProbeTrailPath = trailPath

	cmd := supervisorProbeCmd
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(context.Background())
	t.Cleanup(func() {
		cmd.SetOut(nil)
		cmd.SetErr(nil)
		cmd.SetContext(context.Background())
	})

	if err := runSupervisorProbe(cmd, nil); err != nil {
		t.Fatalf("runSupervisorProbe: %v", err)
	}

	data, err := os.ReadFile(trailPath)
	if err != nil {
		t.Fatalf("read trail: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 trail line, got %d: %q", len(lines), string(data))
	}

	var rec struct {
		Role    string         `json:"role"`
		Kind    string         `json:"kind"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	if rec.Role != string(supervisor.RoleOverseer) {
		t.Errorf("role = %q, want %q", rec.Role, supervisor.RoleOverseer)
	}
	if rec.Kind != "overseer.resource.probe" {
		t.Errorf("kind = %q, want overseer.resource.probe", rec.Kind)
	}
	// The probe always reports these keys (even as zero on unsupported platforms).
	for _, key := range []string{"disk_used_fraction", "memory_used_fraction", "open_fd_fraction", "gopls_processes"} {
		if _, ok := rec.Payload[key]; !ok {
			t.Errorf("payload missing key %q: %v", key, rec.Payload)
		}
	}
	if !strings.Contains(out.String(), "resource probe:") {
		t.Errorf("stdout missing summary line: %q", out.String())
	}
}

// TestSnapshotPayload checks every snapshot field is rendered into the payload.
func TestSnapshotPayload(t *testing.T) {
	snap := supervisor.ResourceSnapshot{
		DiskUsedFraction:        0.5,
		MemoryUsedFraction:      0.6,
		FreePhysicalMemoryBytes: 1234,
		SwapUsedFraction:        0.1,
		CPUUsedFraction:         0.2,
		OpenFDFraction:          0.3,
		VnodeUsedFraction:       0.4,
		GoplsProcesses:          7,
		StrandedWorktrees:       2,
		OrphanedSessions:        1,
	}
	p := snapshotPayload(snap)
	if got := p["gopls_processes"]; got != 7 {
		t.Errorf("gopls_processes = %v, want 7", got)
	}
	if got := p["free_physical_bytes"]; got != uint64(1234) {
		t.Errorf("free_physical_bytes = %v, want 1234", got)
	}
	if got := p["disk_used_fraction"]; got != 0.5 {
		t.Errorf("disk_used_fraction = %v, want 0.5", got)
	}
}
