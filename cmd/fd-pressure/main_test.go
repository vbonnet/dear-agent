package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunTrailAppendsProbeRecord verifies the --trail flag wires the
// SysResourceProbe snapshot into the decision trail: a single
// "overseer.resource.probe" record with the canonical role and a populated
// payload (bead ce-mbgq).
func TestRunTrailAppendsProbeRecord(t *testing.T) {
	dir := t.TempDir()
	trailPath := filepath.Join(dir, "trail.jsonl")

	var out bytes.Buffer
	// --json keeps stdout parseable; the probe runs against the real OS but we
	// only assert on record shape, not specific metric values.
	if _, err := run([]string{"--json", "--trail", trailPath}, &out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	data, err := os.ReadFile(trailPath)
	if err != nil {
		t.Fatalf("reading trail: %v", err)
	}

	// Exactly one record should have been appended.
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			lines = append(lines, sc.Text())
		}
	}
	if len(lines) != 1 {
		t.Fatalf("expected exactly 1 trail record, got %d: %q", len(lines), string(data))
	}

	var rec struct {
		EventID   string         `json:"event_id"`
		Timestamp string         `json:"timestamp"`
		Role      string         `json:"role"`
		Kind      string         `json:"kind"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}

	if rec.Kind != "overseer.resource.probe" {
		t.Errorf("kind = %q, want overseer.resource.probe", rec.Kind)
	}
	if rec.Role != "overseer" {
		t.Errorf("role = %q, want overseer", rec.Role)
	}
	if rec.EventID == "" {
		t.Error("event_id should be populated by the trail")
	}
	if rec.Timestamp == "" {
		t.Error("timestamp should be populated by the trail")
	}
	// The payload must carry the snapshot fields, including the breached count.
	for _, key := range []string{"disk_used_fraction", "memory_used_fraction", "gopls_processes", "breached"} {
		if _, ok := rec.Payload[key]; !ok {
			t.Errorf("payload missing key %q; payload=%v", key, rec.Payload)
		}
	}
}

// TestRunNoTrailWritesNothing confirms the probe stays silent on the trail
// when --trail is not supplied (the default fd-pressure behavior is unchanged).
func TestRunNoTrailWritesNothing(t *testing.T) {
	dir := t.TempDir()
	trailPath := filepath.Join(dir, "trail.jsonl")

	var out bytes.Buffer
	if _, err := run([]string{"--json"}, &out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if _, err := os.Stat(trailPath); !os.IsNotExist(err) {
		t.Errorf("trail file should not exist without --trail; stat err = %v", err)
	}
}

// TestRunTrailFailureDoesNotFail confirms a bad trail path is best-effort: the
// run still succeeds (exit code reflects metrics, not the trail-write error).
func TestRunTrailFailureDoesNotFail(t *testing.T) {
	// A path under a non-existent, unwritable parent that cannot be created.
	badPath := filepath.Join(string([]byte{0}), "trail.jsonl")

	var out bytes.Buffer
	code, err := run([]string{"--json", "--trail", badPath}, &out)
	if err != nil {
		t.Fatalf("run should not return an error on trail failure: %v", err)
	}
	if code != 0 && code != 1 {
		t.Errorf("exit code = %d, want 0 or 1 (metric-driven, not trail-driven)", code)
	}
}
