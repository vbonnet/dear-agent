package eventbus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegration_MultiSink(t *testing.T) {
	tmpDir := t.TempDir()

	bus := NewLocalBus(WithDurableDir(tmpDir))
	defer bus.Close()

	// Add JSONL sink
	jsonlSink, err := NewJSONLSink(filepath.Join(tmpDir, "sink"))
	if err != nil {
		t.Fatalf("NewJSONLSink: %v", err)
	}
	bus.AddSink(jsonlSink, nil)

	// Add log sink (no filter — receives all)
	logSink := NewLogSink(bus.logger)
	bus.AddSink(logSink, nil)

	// Emit events across channels
	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", map[string]interface{}{"k": "v"}))
	bus.Emit(context.Background(), NewEvent("audit.session.start", "pub", map[string]interface{}{"user": "test"}))
	bus.Emit(context.Background(), NewEvent("notification.phase.complete", "pub", nil))
	bus.Emit(context.Background(), NewEvent("heartbeat.agent", "pub", nil))

	time.Sleep(100 * time.Millisecond)

	// Close sinks to flush
	jsonlSink.Close()

	// Verify JSONL sink wrote files per channel
	for _, ch := range []string{"telemetry", "audit", "notification", "heartbeat"} {
		path := filepath.Join(tmpDir, "sink", ch+".jsonl")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("expected %s.jsonl to exist: %v", ch, err)
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 1 {
			t.Errorf("%s.jsonl: expected 1 line, got %d", ch, len(lines))
		}
	}
}

func TestIntegration_DurablePersistence(t *testing.T) {
	tmpDir := t.TempDir()

	bus := NewLocalBus(WithDurableDir(tmpDir))

	// Emit durable events (audit, notification)
	bus.Emit(context.Background(), NewEvent("audit.test.one", "pub", map[string]interface{}{"n": 1}))
	bus.Emit(context.Background(), NewEvent("audit.test.two", "pub", map[string]interface{}{"n": 2}))
	bus.Emit(context.Background(), NewEvent("notification.alert", "pub", map[string]interface{}{"msg": "hi"}))

	// Emit non-durable events (should NOT create durable files)
	bus.Emit(context.Background(), NewEvent("telemetry.test", "pub", nil))
	bus.Emit(context.Background(), NewEvent("heartbeat.agent", "pub", nil))

	time.Sleep(50 * time.Millisecond)
	bus.Close()

	// Verify audit durable file
	auditPath := filepath.Join(tmpDir, "audit.jsonl")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("audit.jsonl not found: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("audit.jsonl: expected 2 lines, got %d", len(lines))
	}

	// Verify notification durable file
	notifPath := filepath.Join(tmpDir, "notification.jsonl")
	data, err = os.ReadFile(notifPath)
	if err != nil {
		t.Fatalf("notification.jsonl not found: %v", err)
	}
	lines = strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("notification.jsonl: expected 1 line, got %d", len(lines))
	}

	// Verify non-durable channels did NOT create durable files
	if _, err := os.Stat(filepath.Join(tmpDir, "telemetry.jsonl")); err == nil {
		t.Error("telemetry.jsonl should not exist (not durable)")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "heartbeat.jsonl")); err == nil {
		t.Error("heartbeat.jsonl should not exist (not durable)")
	}
}

func TestIntegration_JSONLRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	bus := NewLocalBus(WithDurableDir(tmpDir))

	original := NewEvent("audit.roundtrip", "test-source", map[string]interface{}{
		"key1": "value1",
		"key2": float64(42),
	})
	bus.Emit(context.Background(), original)
	time.Sleep(50 * time.Millisecond)
	bus.Close()

	// Read back the JSONL
	data, err := os.ReadFile(filepath.Join(tmpDir, "audit.jsonl"))
	if err != nil {
		t.Fatalf("read audit.jsonl: %v", err)
	}

	var restored Event
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", restored.ID, original.ID)
	}
	if restored.Type != original.Type {
		t.Errorf("Type mismatch: %q vs %q", restored.Type, original.Type)
	}
	if restored.Source != original.Source {
		t.Errorf("Source mismatch: %q vs %q", restored.Source, original.Source)
	}
	if string(restored.Channel) != string(original.Channel) {
		t.Errorf("Channel mismatch: %q vs %q", restored.Channel, original.Channel)
	}
	if restored.Data["key1"] != "value1" {
		t.Errorf("Data[key1] = %v, want %q", restored.Data["key1"], "value1")
	}
	if restored.Data["key2"] != float64(42) {
		t.Errorf("Data[key2] = %v, want 42", restored.Data["key2"])
	}
}
