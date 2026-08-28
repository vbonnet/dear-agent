package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/internal/supervisorheartbeat"
	vroomsupervisor "github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func TestHeartbeatRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec := supervisorheartbeat.Record{
		ID:          "test-sup",
		PrimaryFor:  "peer-a",
		TertiaryFor: "peer-b",
		LastBeatUTC: time.Now().UTC().Round(time.Millisecond),
		PID:         12345,
	}
	root, err := supervisorHeartbeatRoot()
	if err != nil {
		t.Fatal(err)
	}
	store := supervisorheartbeat.New(root)
	if err := store.Write(rec); err != nil {
		t.Fatalf("Store.Write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agm", "supervisors", "test-sup")); err != nil {
		t.Errorf("expected supervisor state directory: %v", err)
	}

	got, err := store.Read("test-sup")
	if err != nil {
		t.Fatalf("Store.Read: %v", err)
	}
	if got == nil {
		t.Fatal("Store.Read returned nil for just-written record")
	}
	if got.ID != rec.ID || got.PrimaryFor != rec.PrimaryFor ||
		got.TertiaryFor != rec.TertiaryFor || got.PID != rec.PID {
		t.Errorf("roundtrip mismatch: got %+v want %+v", got, rec)
	}
}

func TestReadHeartbeatMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root, err := supervisorHeartbeatRoot()
	if err != nil {
		t.Fatal(err)
	}
	got, err := supervisorheartbeat.New(root).Read("never-heartbeated")
	if err != nil {
		t.Errorf("Store.Read(missing): %v", err)
	}
	if got != nil {
		t.Errorf("Store.Read(missing) = %+v, want nil", got)
	}
}

func TestRunSupervisorHeartbeatWritesCanonicalStoreAndPreservesMirrorProjection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	originalID := supervisorID
	originalPrimary := supervisorPrimaryFor
	originalTertiary := supervisorTertiaryFor
	t.Cleanup(func() {
		supervisorID = originalID
		supervisorPrimaryFor = originalPrimary
		supervisorTertiaryFor = originalTertiary
	})
	supervisorID = vroomsupervisor.AliasOrchestrator
	supervisorPrimaryFor = ""
	supervisorTertiaryFor = ""

	started := time.Now().UTC()
	if err := runSupervisorHeartbeat(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runSupervisorHeartbeat: %v", err)
	}
	finished := time.Now().UTC()

	root, err := supervisorHeartbeatRoot()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := supervisorheartbeat.New(root).Read(vroomsupervisor.IDOrchestrator)
	if err != nil {
		t.Fatalf("read authoritative heartbeat: %v", err)
	}
	if rec == nil {
		t.Fatal("authoritative heartbeat is missing")
	}
	if rec.ID != vroomsupervisor.IDOrchestrator ||
		rec.PrimaryFor != vroomsupervisor.IDOverseer ||
		rec.TertiaryFor != vroomsupervisor.IDMetaOrchestrator {
		t.Fatalf("authoritative heartbeat topology = %+v", rec)
	}
	if rec.LastBeatUTC.Before(started) || rec.LastBeatUTC.After(finished) {
		t.Fatalf("authoritative heartbeat time = %s, want within [%s, %s]", rec.LastBeatUTC, started, finished)
	}

	mirrorPath := filepath.Join(home, ".agm", "vroom", "heartbeat", vroomsupervisor.AliasOrchestrator+".json")
	data, err := os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("read legacy mirror projection: %v", err)
	}
	var mirror vroomHeartbeatFile
	if err := json.Unmarshal(data, &mirror); err != nil {
		t.Fatalf("decode legacy mirror projection: %v", err)
	}
	if mirror.Role != string(vroomsupervisor.RoleOrchestrator) || mirror.ISO != rec.LastBeatUTC.Format(time.RFC3339) {
		t.Fatalf("legacy mirror projection = %+v, authoritative heartbeat = %+v", mirror, rec)
	}
}
