package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
)

func TestSanitizeTmuxName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name",
			input: "myproject",
			want:  "myproject",
		},
		{
			name:  "name with spaces",
			input: "my project",
			want:  "my-project",
		},
		{
			name:  "name with special characters",
			input: "my@project#2024!",
			want:  "myproject2024",
		},
		{
			name:  "name with mixed case",
			input: "MyProject-2024",
			want:  "MyProject-2024",
		},
		{
			name:  "name with underscores and dashes",
			input: "my_project-v2",
			want:  "my_project-v2",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only special characters",
			input: "@#$%^&*",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTmuxName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTmuxName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateTmuxName(t *testing.T) {
	tests := []struct {
		name             string
		project          string
		existingSessions []string
		want             string
	}{
		{
			name:             "simple project name",
			project:          "~/myproject",
			existingSessions: []string{},
			want:             "claude-myproject",
		},
		{
			name:             "name conflict - adds suffix",
			project:          "~/myproject",
			existingSessions: []string{"claude-myproject"},
			want:             "claude-myproject-2",
		},
		{
			name:             "multiple conflicts",
			project:          "~/myproject",
			existingSessions: []string{"claude-myproject", "claude-myproject-2"},
			want:             "claude-myproject-3",
		},
		{
			name:             "project with special chars",
			project:          "~/my@project#2024",
			existingSessions: []string{},
			want:             "claude-myproject2024",
		},
		{
			name:             "empty project name after sanitization",
			project:          "~/@#$%",
			existingSessions: []string{},
			want:             "claude-session",
		},
		{
			name:             "home directory",
			project:          "/home/user",
			existingSessions: []string{},
			want:             "claude-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateTmuxName(tt.project, tt.existingSessions)
			if got != tt.want {
				t.Errorf("generateTmuxName(%q, %v) = %q, want %q",
					tt.project, tt.existingSessions, got, tt.want)
			}
		})
	}
}

func TestGenerateTmuxName_ConflictExhaustion(t *testing.T) {
	// Test what happens when we have many conflicts
	project := "~/myproject"

	// Create 98 existing sessions (claude-myproject through claude-myproject-99)
	existingSessions := make([]string, 99)
	existingSessions[0] = "claude-myproject"
	for i := 1; i < 99; i++ {
		existingSessions[i] = fmt.Sprintf("claude-myproject-%d", i+1)
	}

	got := generateTmuxName(project, existingSessions)

	// Should fall back to timestamp-based naming (claude-myproject-NNNN)
	expectedPrefix := "claude-myproject-"
	if len(got) < len(expectedPrefix)+1 {
		t.Errorf("Expected name with format 'claude-myproject-NNNN', got %q", got)
	}
	if got[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected timestamp fallback to start with %q, got %q", expectedPrefix, got)
	}
}

func TestOfferToImportOrphanedSession_NoHistory(t *testing.T) {
	// Setup temp environment
	tmpDir := t.TempDir()

	// Initialize cfg (normally done in PersistentPreRunE)
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = &config.Config{
		SessionsDir: filepath.Join(tmpDir, "sessions"),
	}

	// Set HOME to temp dir (no history.jsonl)
	t.Setenv("HOME", tmpDir)

	// Get adapter
	adapter, err := getStorage()
	if err != nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	_, _, err = offerToImportOrphanedSession(adapter, "test-uuid")
	if err == nil {
		t.Error("Expected error when history.jsonl doesn't exist")
	}
}

func TestOfferToImportOrphanedSession_NoMatch(t *testing.T) {
	// Setup temp environment with history but no matching session
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	os.MkdirAll(claudeDir, 0700)

	// Initialize cfg (normally done in PersistentPreRunE)
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = &config.Config{
		SessionsDir: filepath.Join(tmpDir, "sessions"),
	}

	// Create history with a different UUID
	historyContent := `{"sessionId":"different-uuid-1234","project":"/home/user","timestamp":1733500000000}
`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0600); err != nil {
		t.Fatalf("Failed to create history: %v", err)
	}

	// Set HOME to temp dir
	t.Setenv("HOME", tmpDir)

	// Get adapter
	adapter, err := getStorage()
	if err != nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	_, _, err = offerToImportOrphanedSession(adapter, "nonexistent-uuid")
	if err == nil {
		t.Error("Expected error when no matching session found")
	}

	if err.Error() != "no orphaned sessions found" {
		t.Errorf("Expected 'no orphaned sessions found' error, got: %v", err)
	}
}

func TestOfferToImportOrphanedSession_MultipleMatches(t *testing.T) {
	// Setup temp environment with multiple matching sessions
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	os.MkdirAll(claudeDir, 0700)

	// Initialize cfg (normally done in PersistentPreRunE)
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = &config.Config{
		SessionsDir: filepath.Join(tmpDir, "sessions"),
	}

	// Create history with multiple sessions matching "test"
	historyContent := `{"sessionId":"test-uuid-1111","project":"~/test-project","timestamp":1733500000000}
{"sessionId":"test-uuid-2222","project":"~/another-test","timestamp":1733500001000}
`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0600); err != nil {
		t.Fatalf("Failed to create history: %v", err)
	}

	// Set HOME to temp dir
	t.Setenv("HOME", tmpDir)

	// Get adapter
	adapter, err := getStorage()
	if err != nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	_, _, err = offerToImportOrphanedSession(adapter, "test")
	if err == nil {
		t.Error("Expected error when multiple sessions match")
	}

	if err.Error() != "multiple orphaned sessions found - please be more specific (use full UUID or project path)" {
		t.Errorf("Expected 'multiple orphaned sessions' error, got: %v", err)
	}
}

// TestOfferToImportOrphanedSession_Integration is a more comprehensive test
// but requires mocking user input (ui.Confirm), so it's commented out for now
/*
func TestOfferToImportOrphanedSession_SuccessfulImport(t *testing.T) {
	// This would require mocking:
	// 1. ui.Confirm to return true
	// 2. tmux.ListSessions to return existing sessions
	// 3. discovery.CreateManifest to succeed

	// TODO: Implement once we have a testing harness for interactive components
}
*/

func TestResolveSessionIdentifier_WithAutoImport(t *testing.T) {
	// This test verifies that resolveSessionIdentifier falls through to
	// auto-import when no manifest is found

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	claudeDir := filepath.Join(tmpDir, ".claude")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	os.MkdirAll(sessionsDir, 0700)
	os.MkdirAll(claudeDir, 0700)

	// Initialize cfg (normally done in PersistentPreRunE)
	oldCfg := cfg
	defer func() { cfg = oldCfg }()
	cfg = &config.Config{
		SessionsDir: sessionsDir,
	}

	// Create empty sessions dir (no manifests)
	// Create history with orphaned session
	testUUID := "orphan-1234-5678-90ab-cdef12345678"
	historyContent := fmt.Sprintf(`{"sessionId":"%s","project":"~/orphaned-project","timestamp":%d}
`, testUUID, time.Now().UnixMilli())

	if err := os.WriteFile(historyPath, []byte(historyContent), 0600); err != nil {
		t.Fatalf("Failed to create history: %v", err)
	}

	// Set HOME to temp dir
	t.Setenv("HOME", tmpDir)

	// Get adapter
	adapter, err := getStorage()
	if err != nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// This should fail because no manifests exist AND auto-import would
	// require user confirmation (which we can't mock in this test)
	_, _, err = resolveSessionIdentifier(adapter, "orphan-1234")

	// We expect it to fail at the user confirmation step
	// or return "no sessions found" if history parsing fails
	if err == nil {
		t.Error("Expected error (user confirmation or no sessions)")
	}
}

// Benchmark tests for performance validation

func BenchmarkSanitizeTmuxName(b *testing.B) {
	input := "my-complex-project-name-with-special-chars@#$%^&*()2024"
	for i := 0; i < b.N; i++ {
		_ = sanitizeTmuxName(input)
	}
}

func BenchmarkGenerateTmuxName_NoConflict(b *testing.B) {
	project := "~/my-project"
	existingSessions := []string{}

	for i := 0; i < b.N; i++ {
		_ = generateTmuxName(project, existingSessions)
	}
}

func BenchmarkGenerateTmuxName_WithConflicts(b *testing.B) {
	project := "~/my-project"
	existingSessions := []string{
		"claude-my-project",
		"claude-my-project-2",
		"claude-my-project-3",
	}

	for i := 0; i < b.N; i++ {
		_ = generateTmuxName(project, existingSessions)
	}
}
