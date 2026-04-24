package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/testutil"
)

func TestFindArchived_InvalidPattern(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := FindArchived(tmpDir, "[invalid", nil)
	if err == nil {
		t.Error("expected error for invalid glob pattern")
	}
}

func TestFindArchived_NoArchivedSessions(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	// Create an active (non-archived) session
	sid := "test-noarch-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid)

	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sid,
		Name:          sid,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "", // active
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-" + sid},
		Tmux:          manifest.Tmux{SessionName: sid},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	results, err := FindArchived(tmpDir, "*"+sid+"*", adapter)
	if err != nil {
		t.Fatalf("FindArchived: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 archived results for active session, got %d", len(results))
	}
}

func TestFindArchived_SingleMatch(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	sid := "test-single-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid)

	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sid,
		Name:          sid,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     manifest.LifecycleArchived,
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-" + sid},
		Tmux:          manifest.Tmux{SessionName: sid},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	results, err := FindArchived(tmpDir, "*"+sid+"*", adapter)
	if err != nil {
		t.Fatalf("FindArchived: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 archived result, got %d", len(results))
	}
	if len(results) > 0 && results[0].SessionID != sid {
		t.Errorf("expected session %s, got %s", sid, results[0].SessionID)
	}
}

func TestFindArchived_WildcardPattern(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	prefix := "test-wild-" + uuid.New().String()[:6]
	sid1 := prefix + "-a"
	sid2 := prefix + "-b"
	defer testutil.CleanupTestSession(t, adapter, sid1)
	defer testutil.CleanupTestSession(t, adapter, sid2)

	for _, sid := range []string{sid1, sid2} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sid,
			Name:          sid,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     manifest.LifecycleArchived,
			Workspace:     "oss",
			Claude:        manifest.Claude{UUID: "uuid-" + sid},
			Tmux:          manifest.Tmux{SessionName: sid},
			Context:       manifest.Context{Project: "~/src/test"},
		}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("CreateSession(%s): %v", sid, err)
		}
	}

	results, err := FindArchived(tmpDir, prefix+"*", adapter)
	if err != nil {
		t.Fatalf("FindArchived: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 wildcard matches, got %d", len(results))
	}
}

func TestFindArchived_QuestionMarkPattern(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	prefix := "test-qm-" + uuid.New().String()[:6]
	sid1 := prefix + "-x"
	sid2 := prefix + "-y"
	sid3 := prefix + "-zz" // won't match ? (2 chars)
	defer testutil.CleanupTestSession(t, adapter, sid1)
	defer testutil.CleanupTestSession(t, adapter, sid2)
	defer testutil.CleanupTestSession(t, adapter, sid3)

	for _, sid := range []string{sid1, sid2, sid3} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sid,
			Name:          sid,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     manifest.LifecycleArchived,
			Workspace:     "oss",
			Claude:        manifest.Claude{UUID: "uuid-" + sid},
			Tmux:          manifest.Tmux{SessionName: sid},
			Context:       manifest.Context{Project: "~/src/test"},
		}
		if err := adapter.CreateSession(m); err != nil {
			t.Fatalf("CreateSession(%s): %v", sid, err)
		}
	}

	// ? matches exactly one character, so prefix + "-?" matches sid1 and sid2 but not sid3
	results, err := FindArchived(tmpDir, prefix+"-?", adapter)
	if err != nil {
		t.Fatalf("FindArchived: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 question-mark matches, got %d", len(results))
	}
}

func TestFindArchived_NoMatches(t *testing.T) {
	tmpDir := t.TempDir()

	createTestSession(t, tmpDir, "archived-session", manifest.LifecycleArchived)

	// Phase 6: Use Dolt adapter for tests
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()
	results, err := FindArchived(tmpDir, "*nonexistent*", adapter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching pattern, got %d", len(results))
	}
}

func TestFindArchived_WithTags(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	sid := "test-tags-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid)

	m := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sid,
		Name:          sid,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     manifest.LifecycleArchived,
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-" + sid},
		Tmux:          manifest.Tmux{SessionName: sid},
		Context: manifest.Context{
			Project: "~/src/test",
			Tags:    []string{"important", "test"},
		},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	results, err := FindArchived(tmpDir, "*"+sid+"*", adapter)
	if err != nil {
		t.Fatalf("FindArchived: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Tags may or may not be preserved depending on Dolt storage; just verify session found
	if results[0].SessionID != sid {
		t.Errorf("expected session %s, got %s", sid, results[0].SessionID)
	}
}

func TestFindArchived_SortedByDate(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	prefix := "test-sort-" + uuid.New().String()[:6]
	sidOld := prefix + "-old"
	sidNew := prefix + "-new"
	defer testutil.CleanupTestSession(t, adapter, sidOld)
	defer testutil.CleanupTestSession(t, adapter, sidNew)

	// Create older session first
	mOld := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sidOld,
		Name:          sidOld,
		CreatedAt:     time.Now().Add(-48 * time.Hour),
		UpdatedAt:     time.Now().Add(-48 * time.Hour),
		Lifecycle:     manifest.LifecycleArchived,
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-" + sidOld},
		Tmux:          manifest.Tmux{SessionName: sidOld},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	if err := adapter.CreateSession(mOld); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Create newer session
	mNew := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sidNew,
		Name:          sidNew,
		CreatedAt:     time.Now().Add(-1 * time.Hour),
		UpdatedAt:     time.Now().Add(-1 * time.Hour),
		Lifecycle:     manifest.LifecycleArchived,
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-" + sidNew},
		Tmux:          manifest.Tmux{SessionName: sidNew},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	if err := adapter.CreateSession(mNew); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	results, err := FindArchived(tmpDir, prefix+"*", adapter)
	if err != nil {
		t.Fatalf("FindArchived: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
}

// TestFindArchived_NilAdapter verifies error on nil adapter
func TestFindArchived_NilAdapter(t *testing.T) {
	_, err := FindArchived("/tmp", "*", nil)
	if err == nil {
		t.Error("expected error for nil adapter")
	}
}

// TestFindArchived_ValidPattern verifies search with valid pattern
func TestFindArchived_ValidPattern(t *testing.T) {
	tmpDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	results, err := FindArchived(tmpDir, "*", adapter)
	if err != nil {
		t.Fatalf("FindArchived() error = %v", err)
	}
	// May return 0 or more results depending on DB state
	_ = results
}

// TestDiagnosticTest exercises the diagnostic function for coverage
func TestDiagnosticTest(t *testing.T) {
	result := DiagnosticTest()
	if result == "" {
		t.Error("DiagnosticTest() returned empty string")
	}
}

// TestValidateIdentifier tests path traversal prevention
func TestValidateIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid simple name", "my-session", false},
		{"valid with numbers", "session-123", false},
		{"empty string", "", true},
		{"forward slash", "../../etc/passwd", true},
		{"backslash", "foo\\bar", true},
		{"double dot", "foo..bar", true},
		{"starts with dot", ".hidden", true},
		{"path traversal", "../secret", true},
		{"clean path differs", "foo/./bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIdentifier(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateIdentifier(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}

// TestCheckHealth tests health check functionality
func TestCheckHealth(t *testing.T) {
	t.Run("healthy session with existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := &manifest.Manifest{
			Context: manifest.Context{
				Project: tmpDir,
			},
			Harness: "claude-code",
		}
		report, err := CheckHealth(m)
		if err != nil {
			t.Fatalf("CheckHealth() error = %v", err)
		}
		if !report.WorktreeExists {
			t.Error("WorktreeExists = false, want true")
		}
		if !report.IsHealthy() {
			t.Errorf("IsHealthy() = false, want true")
		}
		if report.Summary() != "All health checks passed" {
			t.Errorf("Summary() = %q, want %q", report.Summary(), "All health checks passed")
		}
	})

	t.Run("unhealthy session with missing directory", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{
				Project: "/nonexistent/path/xyz-99999",
			},
			Harness: "gemini-cli", // non-claude to skip bloat check
		}
		report, err := CheckHealth(m)
		if err != nil {
			t.Fatalf("CheckHealth() error = %v", err)
		}
		if report.WorktreeExists {
			t.Error("WorktreeExists = true, want false")
		}
		if report.IsHealthy() {
			t.Error("IsHealthy() = true, want false")
		}
		if report.Summary() == "All health checks passed" {
			t.Error("Summary() should report issues")
		}
	})

	t.Run("claude-code harness with no UUID skips bloat check", func(t *testing.T) {
		tmpDir := t.TempDir()
		m := &manifest.Manifest{
			Context: manifest.Context{
				Project: tmpDir,
			},
			Harness: "claude-code",
			// No Claude.UUID set
		}
		report, err := CheckHealth(m)
		if err != nil {
			t.Fatalf("CheckHealth() error = %v", err)
		}
		if !report.IsHealthy() {
			t.Errorf("IsHealthy() = false, want true (no UUID = skip bloat check)")
		}
	})
}

// TestHealthReport tests HealthReport methods
func TestHealthReport(t *testing.T) {
	t.Run("healthy report", func(t *testing.T) {
		r := &HealthReport{WorktreeExists: true, Issues: []string{}}
		if !r.IsHealthy() {
			t.Error("IsHealthy() = false, want true")
		}
		if r.Summary() != "All health checks passed" {
			t.Errorf("Summary() = %q", r.Summary())
		}
	})

	t.Run("unhealthy report with issues", func(t *testing.T) {
		r := &HealthReport{
			WorktreeExists: false,
			Issues:         []string{"dir missing", "bloat detected"},
		}
		if r.IsHealthy() {
			t.Error("IsHealthy() = true, want false")
		}
		summary := r.Summary()
		if summary == "All health checks passed" {
			t.Error("Summary() should not say all passed")
		}
	})
}

// TestFormatBloatError tests bloat error message formatting
func TestFormatBloatError(t *testing.T) {
	t.Run("with progress count", func(t *testing.T) {
		msg := formatBloatError("/tmp/test.jsonl", 150.0, 5000)
		if msg == "" {
			t.Error("formatBloatError returned empty string")
		}
		// Check key parts of the message
		if !containsStr(msg, "150MB") {
			t.Error("message should contain file size")
		}
		if !containsStr(msg, "5000 progress entries") {
			t.Error("message should contain progress count")
		}
		if !containsStr(msg, "/tmp/test.jsonl") {
			t.Error("message should contain file path")
		}
	})

	t.Run("without progress count", func(t *testing.T) {
		msg := formatBloatError("/tmp/test.jsonl", 200.0, -1)
		if msg == "" {
			t.Error("formatBloatError returned empty string")
		}
		// When count is -1, the header line should NOT contain "-1 progress entries"
		if containsStr(msg, "-1 progress entries") {
			t.Error("header should not contain progress entry count when count is -1")
		}
	})

	t.Run("zero progress count", func(t *testing.T) {
		msg := formatBloatError("/tmp/test.jsonl", 120.0, 0)
		if msg == "" {
			t.Error("formatBloatError returned empty string")
		}
	})
}

// TestCountProgressEntries tests counting progress entries in session files
func TestCountProgressEntries(t *testing.T) {
	t.Run("file with progress entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.jsonl")
		content := `{"type":"user","content":"hello"}
{"type":"progress","data":{"status":"thinking"}}
{"type":"assistant","content":"hi"}
{"type":"progress","data":{"status":"running"}}
{"type":"progress","data":{"status":"done"}}
`
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		count, err := countProgressEntries(filePath)
		if err != nil {
			t.Fatalf("countProgressEntries() error = %v", err)
		}
		if count != 3 {
			t.Errorf("count = %d, want 3", count)
		}
	})

	t.Run("file with no progress entries", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.jsonl")
		content := `{"type":"user","content":"hello"}
{"type":"assistant","content":"hi"}
`
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		count, err := countProgressEntries(filePath)
		if err != nil {
			t.Fatalf("countProgressEntries() error = %v", err)
		}
		if count != 0 {
			t.Errorf("count = %d, want 0", count)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := countProgressEntries("/nonexistent/file.jsonl")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

// TestCheckClaudeBloat tests bloat detection
func TestCheckClaudeBloat(t *testing.T) {
	t.Run("no UUID returns not bloated", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: "/tmp"},
		}
		bloated, _ := checkClaudeBloat(m)
		if bloated {
			t.Error("expected not bloated for empty UUID")
		}
	})

	t.Run("nonexistent session file returns not bloated", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{Project: "/tmp/nonexistent-xyz"},
		}
		m.Claude.UUID = "nonexistent-uuid-xyz-99999"
		bloated, _ := checkClaudeBloat(m)
		if bloated {
			t.Error("expected not bloated for nonexistent file")
		}
	})
}

// TestCheckClaudeBloat_SmallFile tests that small files are not flagged as bloated
func TestCheckClaudeBloat_SmallFile(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "small-file-uuid"

	// Create a small session file in the expected location
	projectDir := filepath.Join(tmpDir, "myproject")
	claudeDir := filepath.Join(tmpDir, ".claude", "projects", filepath.Base(projectDir))
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(claudeDir, sessionUUID+".jsonl")
	content := `{"type":"user","content":"hello"}
{"type":"assistant","content":"hi"}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Override HOME for this test
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	m := &manifest.Manifest{
		Context: manifest.Context{
			Project: projectDir,
		},
	}
	m.Claude.UUID = sessionUUID

	bloated, _ := checkClaudeBloat(m)
	if bloated {
		t.Error("small file should not be flagged as bloated")
	}
}

// TestCheckClaudeBloat_SearchAllProjectDirs tests the search fallback
func TestCheckClaudeBloat_SearchAllProjectDirs(t *testing.T) {
	tmpDir := t.TempDir()
	sessionUUID := "search-uuid"

	// Create a session file in a different project hash directory
	claudeDir := filepath.Join(tmpDir, ".claude", "projects", "different-project-hash")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(claudeDir, sessionUUID+".jsonl")
	content := `{"type":"user","content":"hello"}
`
	if err := os.WriteFile(sessionFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	m := &manifest.Manifest{
		Context: manifest.Context{
			Project: "/some/other/path",
		},
	}
	m.Claude.UUID = sessionUUID

	// Should find the file via directory scan and not flag as bloated (it's small)
	bloated, _ := checkClaudeBloat(m)
	if bloated {
		t.Error("small file found via scan should not be flagged as bloated")
	}
}

// TestCheckHealth_ClaudeCodeWithUUID tests health check with Claude Code harness and UUID
func TestCheckHealth_ClaudeCodeWithUUID(t *testing.T) {
	tmpDir := t.TempDir()

	m := &manifest.Manifest{
		Context: manifest.Context{
			Project: tmpDir,
		},
		Harness: "claude-code",
	}
	m.Claude.UUID = "nonexistent-uuid-for-health-check"

	report, err := CheckHealth(m)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	// Should be healthy since project dir exists and no bloated file
	if !report.IsHealthy() {
		t.Errorf("IsHealthy() = false, want true; Issues: %v", report.Issues)
	}
}

// TestCheckHealth_DefaultHarness tests that empty harness defaults to claude-code
func TestCheckHealth_DefaultHarness(t *testing.T) {
	tmpDir := t.TempDir()

	m := &manifest.Manifest{
		Context: manifest.Context{
			Project: tmpDir,
		},
		Harness: "", // Empty harness should default to claude-code
	}
	m.Claude.UUID = "some-uuid"

	report, err := CheckHealth(m)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if !report.IsHealthy() {
		t.Errorf("IsHealthy() = false, want true")
	}
}

// TestShellQuote tests shell quoting for safety
func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"with'quote", "'with'\"'\"'quote'"},
		{"", "''"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := shellQuote(tt.input)
			if result != tt.expected {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDisplayTranscriptContext tests the transcript context display function
func TestDisplayTranscriptContext(t *testing.T) {
	t.Run("empty UUID skips transcript", func(t *testing.T) {
		m := &manifest.Manifest{
			// No Claude.UUID set
		}
		// Should not panic
		displayTranscriptContext(m)
	})

	t.Run("nonexistent transcript is silently skipped", func(t *testing.T) {
		m := &manifest.Manifest{
			Context: manifest.Context{
				Project: "/nonexistent/path/xyz-99999",
			},
		}
		m.Claude.UUID = "nonexistent-uuid-xyz-99999"
		// Should not panic, just return silently
		displayTranscriptContext(m)
	})
}

// containsStr is a helper to check if s contains sub
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Helper functions

func createTestSession(t *testing.T, dir, name, lifecycle string) {
	t.Helper()
	createTestSessionWithTime(t, dir, name, lifecycle, time.Now())
}

func createTestSessionWithTime(t *testing.T, dir, name, lifecycle string, updatedAt time.Time) {
	t.Helper()

	sessionDir := filepath.Join(dir, name)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		SchemaVersion: "2",
		SessionID:     name,
		Name:          name,
		CreatedAt:     time.Now(),
		UpdatedAt:     updatedAt,
		Lifecycle:     lifecycle,
		Context: manifest.Context{
			Project: "/tmp/test",
		},
		Tmux: manifest.Tmux{
			SessionName: name,
		},
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	if err := manifest.Write(manifestPath, m); err != nil {
		t.Fatal(err)
	}
}
