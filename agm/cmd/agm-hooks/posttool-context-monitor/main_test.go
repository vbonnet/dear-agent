package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTokenUsageFromReminder(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantUsed  int
		wantTotal int
		wantNil   bool
	}{
		{
			name:      "basic reminder",
			text:      "<system-reminder>Token usage: 45000/200000; 155000 remaining</system-reminder>",
			wantUsed:  45000,
			wantTotal: 200000,
		},
		{
			name:      "reminder with noise",
			text:      "Some output\n<system-reminder>Token usage: 12345/200000; 187655 remaining</system-reminder>\nMore output",
			wantUsed:  12345,
			wantTotal: 200000,
		},
		{
			name:    "no reminder",
			text:    "Just some regular output with no token info",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{}
			got := monitor.extractTokenUsageFromReminder(tt.text)

			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantUsed, got.Used)
				assert.Equal(t, tt.wantTotal, got.Total)
			}
		})
	}
}

func TestExtractTokenUsageFromJSON(t *testing.T) {
	tests := []struct {
		name      string
		data      map[string]interface{}
		wantUsed  int
		wantTotal int
		wantNil   bool
	}{
		{
			name: "complete JSON",
			data: map[string]interface{}{
				"token_usage": map[string]interface{}{
					"input_tokens":  float64(10000),
					"output_tokens": float64(2345),
					"total_tokens":  float64(12345),
				},
				"max_context_tokens": float64(200000),
			},
			wantUsed:  12345,
			wantTotal: 200000,
		},
		{
			name: "JSON with default max",
			data: map[string]interface{}{
				"token_usage": map[string]interface{}{
					"total_tokens": float64(50000),
				},
			},
			wantUsed:  50000,
			wantTotal: 200000, // default
		},
		{
			name: "no token usage",
			data: map[string]interface{}{
				"some": "other",
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{}
			got := monitor.extractTokenUsageFromJSON(tt.data)

			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				require.NotNil(t, got)
				assert.Equal(t, tt.wantUsed, got.Used)
				assert.Equal(t, tt.wantTotal, got.Total)
			}
		})
	}
}

func TestCalculatePercentage(t *testing.T) {
	tests := []struct {
		name  string
		used  int
		total int
		want  float64
	}{
		{
			name:  "basic percentage",
			used:  50000,
			total: 200000,
			want:  25.0,
		},
		{
			name:  "with rounding",
			used:  12345,
			total: 200000,
			want:  6.2, // Rounded to 1 decimal
		},
		{
			name:  "zero total",
			used:  100,
			total: 0,
			want:  0.0,
		},
		{
			name:  "high usage",
			used:  198000,
			total: 200000,
			want:  99.0,
		},
		{
			name:  "full usage",
			used:  200000,
			total: 200000,
			want:  100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{}
			got := monitor.calculatePercentage(tt.used, tt.total)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShouldUpdate(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name       string
		setupCache func(string) // setup function to create cache
		percentage float64
		wantUpdate bool
	}{
		{
			name:       "no cache exists",
			setupCache: nil,
			percentage: 50.0,
			wantUpdate: true,
		},
		{
			name: "interval not elapsed",
			setupCache: func(cacheFile string) {
				cache := CacheEntry{
					Percentage: 50.0,
					Timestamp:  time.Now(),
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(cacheFile, data, 0644)
			},
			percentage: 51.0,
			wantUpdate: false,
		},
		{
			name: "interval elapsed with significant change",
			setupCache: func(cacheFile string) {
				cache := CacheEntry{
					Percentage: 50.0,
					Timestamp:  time.Now().Add(-15 * time.Second),
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(cacheFile, data, 0644)
			},
			percentage: 55.0,
			wantUpdate: true,
		},
		{
			name: "interval elapsed but change too small",
			setupCache: func(cacheFile string) {
				cache := CacheEntry{
					Percentage: 50.0,
					Timestamp:  time.Now().Add(-15 * time.Second),
				}
				data, _ := json.Marshal(cache)
				os.WriteFile(cacheFile, data, 0644)
			},
			percentage: 50.5,
			wantUpdate: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor := &ContextMonitor{
				sessionID: "test-session-123",
				cacheDir:  tempDir,
			}

			cacheFile := monitor.getCacheFile()
			if tt.setupCache != nil {
				tt.setupCache(cacheFile)
			}

			got := monitor.shouldUpdate(tt.percentage)
			assert.Equal(t, tt.wantUpdate, got)
		})
	}
}

func TestUpdateCache(t *testing.T) {
	tempDir := t.TempDir()

	monitor := &ContextMonitor{
		sessionID: "test-session-456",
		cacheDir:  tempDir,
	}

	monitor.updateCache(75.5)

	cacheFile := monitor.getCacheFile()
	require.FileExists(t, cacheFile)

	data, err := os.ReadFile(cacheFile)
	require.NoError(t, err)

	var cache CacheEntry
	err = json.Unmarshal(data, &cache)
	require.NoError(t, err)

	assert.Equal(t, 75.5, cache.Percentage)
	assert.WithinDuration(t, time.Now(), cache.Timestamp, time.Second)
}

func TestFindAGMSession(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name          string
		setupManifest func(string) // setup function to create manifest
		wantSession   string
	}{
		{
			name: "AGM session exists",
			setupManifest: func(manifestPath string) {
				os.MkdirAll(filepath.Dir(manifestPath), 0755)
				content := `session_id: test-session-456
agm_session_name: my-agm-session
workspace: oss
`
				os.WriteFile(manifestPath, []byte(content), 0644)
			},
			wantSession: "my-agm-session",
		},
		{
			name: "not AGM-managed",
			setupManifest: func(manifestPath string) {
				os.MkdirAll(filepath.Dir(manifestPath), 0755)
				content := `session_id: test-session-456
workspace: personal
`
				os.WriteFile(manifestPath, []byte(content), 0644)
			},
			wantSession: "",
		},
		{
			name:          "no manifest",
			setupManifest: nil,
			wantSession:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp home directory structure
			sessionID := "test-session-456"
			homeDir := tempDir
			manifestPath := filepath.Join(homeDir, ".claude", "sessions", sessionID, "manifest.yaml")

			if tt.setupManifest != nil {
				tt.setupManifest(manifestPath)
			}

			// Override home dir for testing
			t.Setenv("HOME", homeDir)

			monitor := &ContextMonitor{
				sessionID: sessionID,
			}

			got := monitor.findAGMSession()
			assert.Equal(t, tt.wantSession, got)
		})
	}
}

func TestRun_NoTokenUsage(t *testing.T) {
	monitor := &ContextMonitor{
		toolResult: "Just some output without tokens",
	}

	exitCode := monitor.Run()
	assert.Equal(t, 0, exitCode)
}

func TestRun_NonBashTool(t *testing.T) {
	// Context monitor doesn't filter by tool name, but let's test basic flow
	monitor := &ContextMonitor{
		toolResult: "<system-reminder>Token usage: 50000/200000; 150000 remaining</system-reminder>",
		sessionID:  "test-session",
		cacheDir:   t.TempDir(),
	}

	// Without AGM session, should succeed but do nothing
	exitCode := monitor.Run()
	assert.Equal(t, 0, exitCode)
}

// fakeAGMOnPath drops a stub `agm` binary on PATH that records every call into
// the returned log file and exits with the given code. This lets us drive the
// hook's `exec.Command("agm", …)` paths without needing the real CLI or any
// network/state — both arguments and the count of invocations are observable.
func fakeAGMOnPath(t *testing.T, exitCode int) string {
	t.Helper()
	bin := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logFile + "\nexit " + fmt.Sprint(exitCode) + "\n"
	if err := os.WriteFile(filepath.Join(bin, "agm"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake agm: %v", err)
	}
	// Prepend our bin dir to PATH so exec.LookPath("agm") resolves to the stub.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

func writeManifest(t *testing.T, home, sessionID, agmSessionName string) {
	t.Helper()
	dir := filepath.Join(home, ".claude", "sessions", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	body := "session_id: " + sessionID + "\n"
	if agmSessionName != "" {
		body += "agm_session_name: " + agmSessionName + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestNewContextMonitor(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "abc-123")
	t.Setenv("CLAUDE_TOOL_NAME", "Bash")
	t.Setenv("CLAUDE_TOOL_RESULT", "hello")
	t.Setenv("CLAUDE_WORKING_DIR", "/tmp/wd")
	t.Setenv("AGM_HOOK_DEBUG", "1")

	m := NewContextMonitor()

	require.NotNil(t, m)
	assert.Equal(t, "abc-123", m.sessionID)
	assert.Equal(t, "Bash", m.toolName)
	assert.Equal(t, "hello", m.toolResult)
	assert.Equal(t, "/tmp/wd", m.workingDir)
	assert.True(t, m.debug)
	assert.DirExists(t, m.cacheDir, "cache dir must exist after construction")
}

func TestUpdateAGMContext_Success(t *testing.T) {
	logFile := fakeAGMOnPath(t, 0)
	m := &ContextMonitor{debug: true}

	if ok := m.updateAGMContext("my-session", 42.7); !ok {
		t.Fatalf("updateAGMContext returned false on exit-0 fake")
	}

	// Verify the fake was invoked with the integer percentage and the right flags.
	data, err := os.ReadFile(logFile)
	require.NoError(t, err)
	got := string(data)
	if !contains(got, "session set-context-usage") || !contains(got, " 43 ") || !contains(got, "--session my-session") {
		t.Errorf("fake agm not invoked correctly: %q", got)
	}
}

func TestUpdateAGMContext_Failure(t *testing.T) {
	_ = fakeAGMOnPath(t, 1)
	m := &ContextMonitor{debug: false}

	if ok := m.updateAGMContext("my-session", 80.0); ok {
		t.Error("updateAGMContext returned true on exit-1 fake; want false")
	}
}

func TestUpdateAGMState(t *testing.T) {
	logFile := fakeAGMOnPath(t, 0)
	m := &ContextMonitor{debug: true}

	// updateAGMState has no return; we observe through the fake's log + the
	// fact it never panics. We exercise both branches by also calling with a
	// failing fake.
	m.updateAGMState("sess", "THINKING")
	data, _ := os.ReadFile(logFile)
	if !contains(string(data), "session state set sess THINKING --source posttool-hook") {
		t.Errorf("state set call missing: %q", string(data))
	}

	// Failing call: hook must still return quietly (best-effort).
	_ = fakeAGMOnPath(t, 7)
	m2 := &ContextMonitor{debug: true}
	m2.updateAGMState("sess", "BUSY") // must not panic
}

func TestRun_EndToEnd_WithAGMSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	logFile := fakeAGMOnPath(t, 0)
	sessionID := "claude-xyz"
	writeManifest(t, home, sessionID, "agm-real")

	m := &ContextMonitor{
		sessionID:  sessionID,
		toolResult: "<system-reminder>Token usage: 100000/200000; 100000 remaining</system-reminder>",
		cacheDir:   t.TempDir(),
		debug:      true,
	}

	if code := m.Run(); code != 0 {
		t.Errorf("Run with AGM session = %d, want 0", code)
	}

	calls, _ := os.ReadFile(logFile)
	s := string(calls)
	if !contains(s, "session state set agm-real THINKING") {
		t.Errorf("expected THINKING state set; got: %q", s)
	}
	if !contains(s, "session set-context-usage") || !contains(s, " 50 ") {
		t.Errorf("expected set-context-usage 50; got: %q", s)
	}

	// Cache must have been written.
	cachePath := filepath.Join(m.cacheDir, sessionID+".json")
	assert.FileExists(t, cachePath)
}

func TestRun_EndToEnd_AGMContextFails_ReturnsOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = fakeAGMOnPath(t, 0) // first call (state) succeeds
	sessionID := "claude-fail"
	writeManifest(t, home, sessionID, "agm-fail")

	// Re-stub PATH so that the *context-usage* call fails — easiest is to make
	// every agm invocation fail; the state call's error is swallowed
	// (best-effort) so the only signal is the return code of Run.
	_ = fakeAGMOnPath(t, 1)

	m := &ContextMonitor{
		sessionID:  sessionID,
		toolResult: "<system-reminder>Token usage: 100000/200000; 100000 remaining</system-reminder>",
		cacheDir:   t.TempDir(),
	}

	if code := m.Run(); code != 1 {
		t.Errorf("Run when AGM context-set fails = %d, want 1", code)
	}
}

func TestRun_FromStdinJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = fakeAGMOnPath(t, 0)
	sessionID := "claude-stdin"
	writeManifest(t, home, sessionID, "agm-stdin")

	// Replace stdin with a pipe that delivers a JSON payload. Run() only reads
	// stdin when there's no system-reminder in toolResult.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	go func() {
		_, _ = w.Write([]byte(`{"token_usage":{"total_tokens":80000},"max_context_tokens":200000}`))
		_ = w.Close()
	}()

	m := &ContextMonitor{
		sessionID: sessionID,
		cacheDir:  t.TempDir(),
		// toolResult deliberately empty so Run falls through to the stdin branch.
	}

	if code := m.Run(); code != 0 {
		t.Errorf("Run with JSON stdin = %d, want 0", code)
	}
}

func TestRun_AGMSessionNotManaged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = fakeAGMOnPath(t, 0)
	// Manifest exists but does NOT contain agm_session_name → not AGM-managed.
	sessionID := "claude-bare"
	writeManifest(t, home, sessionID, "")

	m := &ContextMonitor{
		sessionID:  sessionID,
		toolResult: "<system-reminder>Token usage: 100000/200000; 100000 remaining</system-reminder>",
		cacheDir:   t.TempDir(),
	}

	if code := m.Run(); code != 0 {
		t.Errorf("Run on non-AGM session = %d, want 0", code)
	}
}

func TestShouldUpdate_CorruptCache(t *testing.T) {
	dir := t.TempDir()
	m := &ContextMonitor{sessionID: "corrupt", cacheDir: dir, debug: true}

	// Garbage cache → shouldUpdate logs a WARN and returns true (treat as no cache).
	if err := os.WriteFile(m.getCacheFile(), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt cache: %v", err)
	}
	if !m.shouldUpdate(50.0) {
		t.Error("shouldUpdate with corrupt cache returned false; want true")
	}
}

func TestUpdateCache_NoSessionID(t *testing.T) {
	// Defensive: with no sessionID the cache file path is empty and updateCache
	// must noop (no panic, no file written).
	m := &ContextMonitor{cacheDir: t.TempDir()}
	m.updateCache(10.0)
	entries, _ := os.ReadDir(m.cacheDir)
	if len(entries) != 0 {
		t.Errorf("updateCache with no sessionID created files: %v", entries)
	}
}

func TestLog_DebugAndError(t *testing.T) {
	// debug=false + INFO → silent
	out := captureStderr(t, func() {
		(&ContextMonitor{debug: false}).log("INFO", "quiet")
	})
	if out != "" {
		t.Errorf("INFO with debug=false leaked to stderr: %q", out)
	}

	// debug=false + ERROR → still logged
	out = captureStderr(t, func() {
		(&ContextMonitor{debug: false}).log("ERROR", "loud")
	})
	if !contains(out, "ERROR: loud") {
		t.Errorf("ERROR not logged: %q", out)
	}

	// debug=true + INFO → logged
	out = captureStderr(t, func() {
		(&ContextMonitor{debug: true}).log("INFO", "verbose")
	})
	if !contains(out, "INFO: verbose") {
		t.Errorf("INFO with debug=true not logged: %q", out)
	}
}

// --- helpers below this line -------------------------------------------

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	done := make(chan string)
	go func() {
		var buf [4096]byte
		n, _ := r.Read(buf[:])
		done <- string(buf[:n])
	}()

	fn()
	_ = w.Close()
	return <-done
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
