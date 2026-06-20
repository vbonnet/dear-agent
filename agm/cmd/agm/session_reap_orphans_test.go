package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/orphan"
)

func TestWriteOrphanJSON(t *testing.T) {
	res := orphan.Result{
		Targets: []string{"gopls", "agm-mcp-server"},
		Orphans: []orphan.Proc{{PID: 10}, {PID: 20}, {PID: 30}},
		Killed:  []orphan.Proc{{PID: 10}, {PID: 20}},
		Failed:  []orphan.Proc{{PID: 30}},
		DryRun:  false,
		KillError: map[int]string{
			30: "operation not permitted",
		},
	}

	var buf bytes.Buffer
	if err := writeOrphanJSON(&buf, res); err != nil {
		t.Fatalf("writeOrphanJSON: %v", err)
	}

	var got orphanReapJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	if got.OrphansFound != 3 {
		t.Errorf("orphans_found = %d, want 3", got.OrphansFound)
	}
	if got.Killed != 2 {
		t.Errorf("killed = %d, want 2", got.Killed)
	}
	if got.Failed != 1 {
		t.Errorf("failed = %d, want 1", got.Failed)
	}
	if len(got.KilledPIDs) != 2 || got.KilledPIDs[0] != 10 || got.KilledPIDs[1] != 20 {
		t.Errorf("killed_pids = %v, want [10 20]", got.KilledPIDs)
	}
	if len(got.FailedPIDs) != 1 || got.FailedPIDs[0] != 30 {
		t.Errorf("failed_pids = %v, want [30]", got.FailedPIDs)
	}
	if len(got.Targets) != 2 {
		t.Errorf("targets = %v, want 2 entries", got.Targets)
	}
}

func TestWriteOrphanJSON_DryRunEmpty(t *testing.T) {
	res := orphan.Result{
		Targets: []string{"gopls"},
		DryRun:  true,
	}
	var buf bytes.Buffer
	if err := writeOrphanJSON(&buf, res); err != nil {
		t.Fatalf("writeOrphanJSON: %v", err)
	}
	var got orphanReapJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !got.DryRun {
		t.Error("dry_run = false, want true")
	}
	if got.OrphansFound != 0 || got.Killed != 0 || got.Failed != 0 {
		t.Errorf("expected all-zero counts, got %+v", got)
	}
	// Empty slices must serialize as [], never null, so consumers can range
	// without a nil check.
	if got.KilledPIDs == nil || got.FailedPIDs == nil {
		t.Errorf("killed_pids/failed_pids should be [] not null: %s", buf.String())
	}
}
