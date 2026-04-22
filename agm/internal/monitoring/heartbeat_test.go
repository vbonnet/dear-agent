package monitoring

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeartbeatWriter_Write(t *testing.T) {
	dir := t.TempDir()

	writer, err := NewHeartbeatWriter(dir)
	if err != nil {
		t.Fatalf("NewHeartbeatWriter: %v", err)
	}

	err = writer.Write("test-session", 300, 5, true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, "loop-test-session.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("heartbeat file not created")
	}
}

func TestReadHeartbeat(t *testing.T) {
	dir := t.TempDir()

	writer, err := NewHeartbeatWriter(dir)
	if err != nil {
		t.Fatalf("NewHeartbeatWriter: %v", err)
	}

	err = writer.Write("my-session", 300, 10, true)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	hb, err := ReadHeartbeat(dir, "my-session")
	if err != nil {
		t.Fatalf("ReadHeartbeat: %v", err)
	}

	if hb.Session != "my-session" {
		t.Errorf("expected session 'my-session', got %q", hb.Session)
	}
	if hb.IntervalSecs != 300 {
		t.Errorf("expected interval 300, got %d", hb.IntervalSecs)
	}
	if hb.CycleNumber != 10 {
		t.Errorf("expected cycle 10, got %d", hb.CycleNumber)
	}
	if !hb.OK {
		t.Error("expected OK=true")
	}
}

func TestReadHeartbeat_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadHeartbeat(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent heartbeat")
	}
}

func TestListHeartbeats(t *testing.T) {
	dir := t.TempDir()

	writer, err := NewHeartbeatWriter(dir)
	if err != nil {
		t.Fatalf("NewHeartbeatWriter: %v", err)
	}

	writer.Write("session-a", 300, 1, true)
	writer.Write("session-b", 600, 2, true)

	heartbeats, err := ListHeartbeats(dir)
	if err != nil {
		t.Fatalf("ListHeartbeats: %v", err)
	}

	if len(heartbeats) != 2 {
		t.Fatalf("expected 2 heartbeats, got %d", len(heartbeats))
	}
}

func TestCheckStaleness_OK(t *testing.T) {
	hb := &LoopHeartbeat{
		Timestamp:    time.Now(),
		IntervalSecs: 300,
	}
	status := CheckStaleness(hb)
	if status != "ok" {
		t.Errorf("expected 'ok', got %q", status)
	}
}

func TestCheckStaleness_Stale(t *testing.T) {
	hb := &LoopHeartbeat{
		Timestamp:    time.Now().Add(-10 * time.Minute),
		IntervalSecs: 300,
	}
	status := CheckStaleness(hb)
	if status != "stale" {
		t.Errorf("expected 'stale', got %q", status)
	}
}

func TestCheckStaleness_Nil(t *testing.T) {
	status := CheckStaleness(nil)
	if status != "stale" {
		t.Errorf("expected 'stale' for nil, got %q", status)
	}
}

func TestCheckStaleness_Warn(t *testing.T) {
	// Set timestamp to 80% of threshold (interval + 60s)
	// For 300s interval: threshold = 360s, 80% = 288s
	hb := &LoopHeartbeat{
		Timestamp:    time.Now().Add(-290 * time.Second),
		IntervalSecs: 300,
	}
	status := CheckStaleness(hb)
	if status != "warn" {
		t.Errorf("expected 'warn', got %q", status)
	}
}

func TestCheckStalenessWithMaxAge_OK(t *testing.T) {
	hb := &LoopHeartbeat{
		Timestamp:    time.Now(),
		IntervalSecs: 300,
	}
	status := CheckStalenessWithMaxAge(hb, 20*time.Minute)
	if status != "ok" {
		t.Errorf("expected 'ok', got %q", status)
	}
}

func TestCheckStalenessWithMaxAge_Stale(t *testing.T) {
	hb := &LoopHeartbeat{
		Timestamp:    time.Now().Add(-25 * time.Minute),
		IntervalSecs: 300,
	}
	status := CheckStalenessWithMaxAge(hb, 20*time.Minute)
	if status != "stale" {
		t.Errorf("expected 'stale', got %q", status)
	}
}

func TestCheckStalenessWithMaxAge_Warn(t *testing.T) {
	hb := &LoopHeartbeat{
		Timestamp:    time.Now().Add(-17 * time.Minute),
		IntervalSecs: 300,
	}
	status := CheckStalenessWithMaxAge(hb, 20*time.Minute)
	if status != "warn" {
		t.Errorf("expected 'warn', got %q", status)
	}
}

func TestCheckStalenessWithMaxAge_Nil(t *testing.T) {
	status := CheckStalenessWithMaxAge(nil, 20*time.Minute)
	if status != "stale" {
		t.Errorf("expected 'stale' for nil, got %q", status)
	}
}

func TestRemoveHeartbeat(t *testing.T) {
	dir := t.TempDir()

	writer, err := NewHeartbeatWriter(dir)
	if err != nil {
		t.Fatalf("NewHeartbeatWriter: %v", err)
	}

	writer.Write("to-remove", 300, 1, true)

	err = RemoveHeartbeat(dir, "to-remove")
	if err != nil {
		t.Fatalf("RemoveHeartbeat: %v", err)
	}

	// Verify file removed
	path := filepath.Join(dir, "loop-to-remove.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("heartbeat file should be removed")
	}
}
