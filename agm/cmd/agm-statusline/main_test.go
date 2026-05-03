package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistJSON(t *testing.T) {
	dir := t.TempDir()
	statusLineDir = dir

	raw := []byte(`{"session_id":"test-123","session_name":"test"}`)
	if err := persistJSON("test-123", raw); err != nil {
		t.Fatalf("persistJSON: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "test-123.json"))
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if string(got) != string(raw) {
		t.Errorf("got %q, want %q", got, raw)
	}
}

func TestReadAndPersist(t *testing.T) {
	dir := t.TempDir()
	statusLineDir = dir

	input := map[string]interface{}{
		"session_id":   "abc-456",
		"session_name": "my-session",
		"workspace":    map[string]interface{}{"current_dir": "/tmp/ws"},
	}
	raw, _ := json.Marshal(input)

	// Redirect stdin.
	r, w, _ := os.Pipe()
	w.Write(raw)
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	gotRaw, sd, err := readAndPersist()
	if err != nil {
		t.Fatalf("readAndPersist: %v", err)
	}
	if sd.SessionID != "abc-456" {
		t.Errorf("SessionID = %q, want %q", sd.SessionID, "abc-456")
	}
	if sd.SessionName != "my-session" {
		t.Errorf("SessionName = %q, want %q", sd.SessionName, "my-session")
	}
	if len(gotRaw) == 0 {
		t.Error("raw should not be empty")
	}

	// Verify persisted.
	persisted, err := os.ReadFile(filepath.Join(dir, "abc-456.json"))
	if err != nil {
		t.Fatalf("read persisted: %v", err)
	}
	if len(persisted) == 0 {
		t.Error("persisted file should not be empty")
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"10-orchestrator", "orchestrator"},
		{"20-wayfinder", "wayfinder"},
		{"99-custom-thing", "custom-thing"},
		{"noprefixfile", "noprefixfile"},
		{"0-zero", "zero"},
	}
	for _, tt := range tests {
		got := stripPrefix(tt.input)
		if got != tt.want {
			t.Errorf("stripPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// withGenerousProviderTimeout extends providerTimeout for the duration of t.
// The 500ms production default exists to keep the statusline responsive in
// the editor; under heavy parallel `go test ./...` load (especially in CI)
// fork+exec of the test's /bin/sh providers can exceed that, producing a
// flaky 0-segments result that has nothing to do with the behavior under
// test. Bumping the timeout for tests that exercise actual subprocesses
// makes the assertion test correctness, not contention.
func withGenerousProviderTimeout(t *testing.T) {
	t.Helper()
	prev := providerTimeout
	providerTimeout = 10 * time.Second
	t.Cleanup(func() { providerTimeout = prev })
}

func TestRunProviders(t *testing.T) {
	withGenerousProviderTimeout(t)
	dir := t.TempDir()
	providersDir = dir

	// Create two test provider scripts.
	script1 := filepath.Join(dir, "10-hello")
	os.WriteFile(script1, []byte("#!/bin/sh\necho 'Hello'"), 0o755)

	script2 := filepath.Join(dir, "20-world")
	os.WriteFile(script2, []byte("#!/bin/sh\necho 'World'"), 0o755)

	// Create a failing provider (should be skipped).
	script3 := filepath.Join(dir, "30-fail")
	os.WriteFile(script3, []byte("#!/bin/sh\nexit 1"), 0o755)

	// Create an empty-output provider (should be skipped).
	script4 := filepath.Join(dir, "40-empty")
	os.WriteFile(script4, []byte("#!/bin/sh\necho ''"), 0o755)

	cfg := config{Separator: " │ "}
	raw := []byte(`{"session_id":"test","session_name":"test"}`)
	sd := sessionData{SessionID: "test", SessionName: "test"}

	segments := runProviders(cfg, raw, sd)

	if len(segments) != 2 {
		t.Fatalf("got %d segments, want 2: %v", len(segments), segments)
	}
	if segments[0] != "Hello" {
		t.Errorf("segment[0] = %q, want %q", segments[0], "Hello")
	}
	if segments[1] != "World" {
		t.Errorf("segment[1] = %q, want %q", segments[1], "World")
	}
}

func TestRunProviders_Disabled(t *testing.T) {
	withGenerousProviderTimeout(t)
	dir := t.TempDir()
	providersDir = dir

	script1 := filepath.Join(dir, "10-hello")
	os.WriteFile(script1, []byte("#!/bin/sh\necho 'Hello'"), 0o755)

	script2 := filepath.Join(dir, "20-world")
	os.WriteFile(script2, []byte("#!/bin/sh\necho 'World'"), 0o755)

	cfg := config{Separator: " │ ", Disable: []string{"hello"}}
	raw := []byte(`{"session_id":"test"}`)
	sd := sessionData{SessionID: "test"}

	segments := runProviders(cfg, raw, sd)

	if len(segments) != 1 {
		t.Fatalf("got %d segments, want 1: %v", len(segments), segments)
	}
	if segments[0] != "World" {
		t.Errorf("segment[0] = %q, want %q", segments[0], "World")
	}
}

func TestRunProviders_StdinPassthrough(t *testing.T) {
	withGenerousProviderTimeout(t)
	dir := t.TempDir()
	providersDir = dir

	// Provider that reads session_name from stdin JSON.
	script := filepath.Join(dir, "10-echo-name")
	os.WriteFile(script, []byte(`#!/bin/sh
jq -r '.session_name // "unknown"'
`), 0o755)

	cfg := config{Separator: " │ "}
	raw := []byte(`{"session_id":"test","session_name":"my-session"}`)
	sd := sessionData{SessionID: "test", SessionName: "my-session"}

	segments := runProviders(cfg, raw, sd)

	if len(segments) != 1 {
		t.Fatalf("got %d segments, want 1: %v", len(segments), segments)
	}
	if segments[0] != "my-session" {
		t.Errorf("segment[0] = %q, want %q", segments[0], "my-session")
	}
}

func TestRunProviders_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	providersDir = dir

	cfg := config{Separator: " │ "}
	segments := runProviders(cfg, nil, sessionData{})

	if len(segments) != 0 {
		t.Errorf("expected no segments, got %v", segments)
	}
}

func TestRunProviders_NoDir(t *testing.T) {
	providersDir = "/nonexistent/path"

	cfg := config{Separator: " │ "}
	segments := runProviders(cfg, nil, sessionData{})

	if len(segments) != 0 {
		t.Errorf("expected no segments, got %v", segments)
	}
}

func TestReadCache_Fresh(t *testing.T) {
	dir := t.TempDir()
	statusLineDir = dir

	// Write a cache file.
	writeCache("sess-1", "Hello │ World")

	got, ok := readCache("sess-1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "Hello │ World" {
		t.Errorf("got %q, want %q", got, "Hello │ World")
	}
}

func TestReadCache_Stale(t *testing.T) {
	dir := t.TempDir()
	statusLineDir = dir

	// Write a cache file.
	writeCache("sess-2", "old output")

	// Simulate time passing beyond TTL.
	oldTimeNow := timeNow
	timeNow = func() time.Time { return time.Now().Add(cacheTTL + time.Second) }
	defer func() { timeNow = oldTimeNow }()

	_, ok := readCache("sess-2")
	if ok {
		t.Error("expected cache miss for stale entry")
	}
}

func TestReadCache_Missing(t *testing.T) {
	dir := t.TempDir()
	statusLineDir = dir

	_, ok := readCache("nonexistent")
	if ok {
		t.Error("expected cache miss for missing file")
	}
}

func TestWriteCache_EmptyOutput(t *testing.T) {
	dir := t.TempDir()
	statusLineDir = dir

	// Empty output should still be cached (prevents re-running providers).
	writeCache("sess-3", "")

	got, ok := readCache("sess-3")
	if !ok {
		t.Fatal("expected cache hit for empty output")
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
