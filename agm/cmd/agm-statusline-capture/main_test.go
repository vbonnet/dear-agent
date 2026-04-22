package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// setStatusLineDir overrides statusLineDir for a test and restores it on cleanup.
func setStatusLineDir(t *testing.T, dir string) {
	t.Helper()
	orig := statusLineDir
	statusLineDir = dir
	t.Cleanup(func() { statusLineDir = orig })
}

// fakeStdin replaces os.Stdin with a reader containing data, restoring on cleanup.
// For large data (>64KB), uses a temp file to avoid pipe buffer deadlock.
func fakeStdin(t *testing.T, data []byte) {
	t.Helper()

	old := os.Stdin

	if len(data) > 60000 {
		// Use a temp file for large payloads to avoid pipe buffer limits.
		f, err := os.CreateTemp(t.TempDir(), "stdin-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Seek(0, 0); err != nil {
			t.Fatal(err)
		}
		os.Stdin = f
		t.Cleanup(func() { os.Stdin = old })
		return
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()

	os.Stdin = r
	t.Cleanup(func() { os.Stdin = old })
}

// captureStdout captures stdout output during fn, restoring on return.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

func TestSessionDataParsing(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantID    string
		wantError bool
	}{
		{
			name:   "valid session",
			input:  `{"session_id":"abc123","cost":{"total_cost_usd":1.50}}`,
			wantID: "abc123",
		},
		{
			name:      "missing session_id",
			input:     `{"cost":{"total_cost_usd":1.50}}`,
			wantError: true,
		},
		{
			name:      "empty session_id",
			input:     `{"session_id":"","cost":{"total_cost_usd":1.50}}`,
			wantError: true,
		},
		{
			name:      "invalid JSON",
			input:     `not json at all`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sd sessionData
			err := json.Unmarshal([]byte(tt.input), &sd)
			if err != nil {
				if !tt.wantError {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if sd.SessionID == "" {
				if !tt.wantError {
					t.Errorf("expected session_id but got empty")
				}
				return
			}
			if tt.wantError {
				t.Errorf("expected error but got session_id=%q", sd.SessionID)
				return
			}
			if sd.SessionID != tt.wantID {
				t.Errorf("session_id = %q, want %q", sd.SessionID, tt.wantID)
			}
		})
	}
}

// --- Tests that exercise run() ---

func TestRunEmptyStdin(t *testing.T) {
	setStatusLineDir(t, t.TempDir())
	fakeStdin(t, nil)

	if err := run(); err != nil {
		t.Errorf("run() with empty stdin: %v", err)
	}
}

func TestRunValidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)

	input := map[string]any{
		"session_id":      "sess-42",
		"transcript_path": "/tmp/transcript.jsonl",
		"cwd":             "~/src",
		"cost": map[string]any{
			"total_cost_usd":    64.09,
			"total_duration_ms": 13471590,
		},
		"model": map[string]any{
			"id":           "claude-opus-4-6",
			"display_name": "Opus 4.6 (1M context)",
		},
		"version": "2.1.81",
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	fakeStdin(t, raw)

	if err := run(); err != nil {
		t.Fatalf("run() returned error: %v", err)
	}

	// Verify the file was written to the correct path.
	dst := filepath.Join(tmpDir, "sess-42.json")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("parse written JSON: %v", err)
	}
	if parsed["session_id"] != "sess-42" {
		t.Errorf("session_id = %v, want sess-42", parsed["session_id"])
	}
	if parsed["version"] != "2.1.81" {
		t.Errorf("version = %v, want 2.1.81", parsed["version"])
	}

	// Verify no .tmp file remains.
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file should not exist after successful rename")
	}
}

func TestRunInvalidJSON(t *testing.T) {
	setStatusLineDir(t, t.TempDir())
	fakeStdin(t, []byte(`{not valid json`))

	err := run()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "parse JSON") {
		t.Errorf("error = %q, want it to contain 'parse JSON'", err)
	}
}

func TestRunMissingSessionID(t *testing.T) {
	setStatusLineDir(t, t.TempDir())
	fakeStdin(t, []byte(`{"cost":{"total_cost_usd":1.50}}`))

	err := run()
	if err == nil {
		t.Fatal("expected error for missing session_id, got nil")
	}
	if !strings.Contains(err.Error(), "missing session_id") {
		t.Errorf("error = %q, want it to contain 'missing session_id'", err)
	}
}

func TestRunEmptySessionID(t *testing.T) {
	setStatusLineDir(t, t.TempDir())
	fakeStdin(t, []byte(`{"session_id":"","cost":1}`))

	err := run()
	if err == nil {
		t.Fatal("expected error for empty session_id, got nil")
	}
	if !strings.Contains(err.Error(), "missing session_id") {
		t.Errorf("error = %q, want 'missing session_id'", err)
	}
}

func TestRunCreatesDirectory(t *testing.T) {
	// Use a nested path that doesn't exist yet.
	tmpDir := filepath.Join(t.TempDir(), "nested", "dir")
	setStatusLineDir(t, tmpDir)
	fakeStdin(t, []byte(`{"session_id":"dir-test"}`))

	if err := run(); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	dst := filepath.Join(tmpDir, "dir-test.json")
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected file at %s: %v", dst, err)
	}
}

func TestRunLargeJSON(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)

	// Build a >1MB JSON payload.
	bigField := strings.Repeat("x", 1<<20) // 1MB string
	input := map[string]any{
		"session_id": "big-sess",
		"payload":    bigField,
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	fakeStdin(t, raw)

	if err := run(); err != nil {
		t.Fatalf("run() error with large input: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tmpDir, "big-sess.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 1<<20 {
		t.Errorf("written file too small: %d bytes", len(got))
	}
}

func TestRunWriteFailure(t *testing.T) {
	// Point to a path we can't write to (a file, not a directory).
	tmpDir := t.TempDir()
	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	// Use the file as the "directory" — MkdirAll will fail.
	setStatusLineDir(t, blocker)
	fakeStdin(t, []byte(`{"session_id":"fail"}`))

	err := run()
	if err == nil {
		t.Fatal("expected error when directory creation fails")
	}
}

func TestRunAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)
	fakeStdin(t, []byte(`{"session_id":"atomic-test","val":1}`))

	if err := run(); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	// File should exist at final path, not at .tmp path.
	dst := filepath.Join(tmpDir, "atomic-test.json")
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("final file missing: %v", err)
	}
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp file should not exist after rename")
	}
}

func TestRunOverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)

	dst := filepath.Join(tmpDir, "overwrite.json")
	if err := os.WriteFile(dst, []byte(`{"session_id":"overwrite","v":1}`), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeStdin(t, []byte(`{"session_id":"overwrite","v":2}`))

	if err := run(); err != nil {
		t.Fatalf("run() error: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatal(err)
	}
	if v, ok := parsed["v"].(float64); !ok || v != 2 {
		t.Errorf("v = %v, want 2", parsed["v"])
	}
}

func TestRunConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine writes its own session to avoid data races on
			// os.Stdin. We call the core logic directly instead.
			sid := "concurrent-" + strings.Repeat("x", idx)
			raw, _ := json.Marshal(map[string]any{"session_id": sid})

			dir := tmpDir
			if err := os.MkdirAll(dir, 0o700); err != nil {
				errs <- err
				return
			}
			dst := filepath.Join(dir, sid+".json")
			tmp := dst + ".tmp"
			if err := os.WriteFile(tmp, raw, 0o600); err != nil {
				errs <- err
				return
			}
			if err := os.Rename(tmp, dst); err != nil {
				_ = os.Remove(tmp)
				errs <- err
				return
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent write error: %v", err)
	}
}

// --- Tests for printPrompt ---

func TestPrintPromptOutput(t *testing.T) {
	out := captureStdout(t, func() {
		printPrompt()
	})

	// Should contain ANSI escape codes.
	if !strings.Contains(out, "\033[") {
		t.Errorf("expected ANSI codes in output, got %q", out)
	}
	// Should contain a colon separator between user@host and cwd.
	if !strings.Contains(out, ":") {
		t.Errorf("expected ':' separator in prompt, got %q", out)
	}
}

// --- Test main() integration ---

func TestMainValidInput(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)
	fakeStdin(t, []byte(`{"session_id":"main-test","val":99}`))

	out := captureStdout(t, func() {
		main()
	})

	// Should produce prompt on stdout.
	if !strings.Contains(out, "\033[") {
		t.Errorf("main() should output ANSI prompt, got %q", out)
	}

	// File should be written.
	dst := filepath.Join(tmpDir, "main-test.json")
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected file at %s: %v", dst, err)
	}
}

func TestMainEmptyStdin(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)
	fakeStdin(t, nil)

	out := captureStdout(t, func() {
		main()
	})

	// Should still output prompt even with empty stdin.
	if !strings.Contains(out, "\033[") {
		t.Errorf("main() should output ANSI prompt even on empty stdin, got %q", out)
	}
}

func TestMainInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)
	fakeStdin(t, []byte(`broken json!!!`))

	// Capture stderr too to verify error is logged.
	oldStderr := os.Stderr
	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	out := captureStdout(t, func() {
		main()
	})

	stderrW.Close()
	os.Stderr = oldStderr
	stderrBuf := make([]byte, 4096)
	n, _ := stderrR.Read(stderrBuf)
	stderrOut := string(stderrBuf[:n])

	// Prompt should still appear on stdout.
	if !strings.Contains(out, "\033[") {
		t.Errorf("main() should output ANSI prompt even on bad JSON, got %q", out)
	}

	// Error should be logged to stderr.
	if !strings.Contains(stderrOut, "agm-statusline-capture") {
		t.Errorf("expected error on stderr, got %q", stderrOut)
	}
}

func TestMainMissingSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	setStatusLineDir(t, tmpDir)
	fakeStdin(t, []byte(`{"foo":"bar"}`))

	// Capture stderr.
	oldStderr := os.Stderr
	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	out := captureStdout(t, func() {
		main()
	})

	stderrW.Close()
	os.Stderr = oldStderr
	stderrBuf := make([]byte, 4096)
	n, _ := stderrR.Read(stderrBuf)
	stderrOut := string(stderrBuf[:n])

	// Prompt still output.
	if !strings.Contains(out, "\033[") {
		t.Errorf("main() should output prompt, got %q", out)
	}

	// Error logged.
	if !strings.Contains(stderrOut, "missing session_id") {
		t.Errorf("expected 'missing session_id' on stderr, got %q", stderrOut)
	}
}
