package hippocampus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewClaudeCodeAdapter_DefaultDir(t *testing.T) {
	adapter := NewClaudeCodeAdapter("")
	if adapter.claudeDir == "" {
		t.Fatal("expected non-empty claudeDir")
	}
	if adapter.Name() != "claude-code" {
		t.Errorf("expected name 'claude-code', got %q", adapter.Name())
	}
}

func TestNewClaudeCodeAdapter_CustomDir(t *testing.T) {
	adapter := NewClaudeCodeAdapter("/tmp/test-claude")
	if adapter.claudeDir != "/tmp/test-claude" {
		t.Errorf("expected '/tmp/test-claude', got %q", adapter.claudeDir)
	}
}

func TestClaudeCodeAdapter_GetMemoryDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create the expected project structure
	// For projectPath "/test/project", key becomes "-test-project"
	projectPath := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}

	absPath, _ := filepath.Abs(projectPath)
	key := ""
	for _, c := range absPath {
		if c == filepath.Separator {
			key += "-"
		} else {
			key += string(c)
		}
	}

	memDir := filepath.Join(tmpDir, "claude", "projects", key, "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}

	adapter := NewClaudeCodeAdapter(filepath.Join(tmpDir, "claude"))
	got, err := adapter.GetMemoryDir(projectPath)
	if err != nil {
		t.Fatalf("GetMemoryDir failed: %v", err)
	}
	if got != memDir {
		t.Errorf("expected %q, got %q", memDir, got)
	}
}

func TestClaudeCodeAdapter_GetMemoryDir_NotFound(t *testing.T) {
	adapter := NewClaudeCodeAdapter(t.TempDir())
	_, err := adapter.GetMemoryDir("/nonexistent/project")
	if err == nil {
		t.Fatal("expected error for missing memory dir")
	}
}

func TestClaudeCodeAdapter_DiscoverSessions(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "claude")

	// Use a simple project path for predictable key generation
	projectPath := tmpDir
	absPath, _ := filepath.Abs(projectPath)
	key := ""
	for _, c := range absPath {
		if c == filepath.Separator {
			key += "-"
		} else {
			key += string(c)
		}
	}

	projectDir := filepath.Join(claudeDir, "projects", key)

	// Create session directories
	session1Dir := filepath.Join(projectDir, "session-abc")
	if err := os.MkdirAll(session1Dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a JSONL file in the session directory
	jsonlPath := filepath.Join(session1Dir, "transcript.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(`{"type":"user","message":{"role":"user","content":"hello"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create "memory" directory (should be skipped)
	if err := os.MkdirAll(filepath.Join(projectDir, "memory"), 0o755); err != nil {
		t.Fatal(err)
	}

	adapter := NewClaudeCodeAdapter(claudeDir)
	ctx := context.Background()
	sessions, err := adapter.DiscoverSessions(ctx, projectPath, time.Time{}) // since epoch
	if err != nil {
		t.Fatalf("DiscoverSessions failed: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	if sessions[0].ID != "session-abc" {
		t.Errorf("expected session ID 'session-abc', got %q", sessions[0].ID)
	}

	if sessions[0].FilePath == "" {
		t.Error("expected non-empty FilePath")
	}
}

func TestClaudeCodeAdapter_DiscoverSessions_NoProjectDir(t *testing.T) {
	adapter := NewClaudeCodeAdapter(t.TempDir())
	sessions, err := adapter.DiscoverSessions(context.Background(), "/nonexistent", time.Time{})
	if err != nil {
		t.Fatalf("expected nil error for missing project dir, got %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestClaudeCodeAdapter_DiscoverSessions_FilterByTime(t *testing.T) {
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, "claude")

	projectPath := tmpDir
	absPath, _ := filepath.Abs(projectPath)
	key := ""
	for _, c := range absPath {
		if c == filepath.Separator {
			key += "-"
		} else {
			key += string(c)
		}
	}

	projectDir := filepath.Join(claudeDir, "projects", key)

	// Create session directory
	sessionDir := filepath.Join(projectDir, "old-session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "t.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewClaudeCodeAdapter(claudeDir)
	// Use a future time to filter out all sessions
	sessions, err := adapter.DiscoverSessions(context.Background(), projectPath, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DiscoverSessions failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions after time filter, got %d", len(sessions))
	}
}

func TestClaudeCodeAdapter_ReadTranscript(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a JSONL file with test data
	jsonlPath := filepath.Join(tmpDir, "test.jsonl")

	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type message struct {
		Role    string         `json:"role"`
		Content []contentBlock `json:"content"`
	}
	type entry struct {
		Type    string  `json:"type"`
		Message message `json:"message"`
	}

	entries := []entry{
		{
			Type: "user",
			Message: message{
				Role:    "user",
				Content: []contentBlock{{Type: "text", Text: "Hello, I need help with Go"}},
			},
		},
		{
			Type: "assistant",
			Message: message{
				Role:    "assistant",
				Content: []contentBlock{{Type: "text", Text: "I can help with that"}},
			},
		},
		{
			Type: "progress", // should be skipped
			Message: message{
				Role:    "assistant",
				Content: []contentBlock{{Type: "text", Text: "working..."}},
			},
		},
	}

	f, err := os.Create(jsonlPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()

	adapter := NewClaudeCodeAdapter(tmpDir)
	session := SessionInfo{ID: "test", FilePath: jsonlPath}
	transcript, err := adapter.ReadTranscript(context.Background(), session)
	if err != nil {
		t.Fatalf("ReadTranscript failed: %v", err)
	}

	if transcript == "" {
		t.Fatal("expected non-empty transcript")
	}

	// Should contain user and assistant text
	if !contains(transcript, "Hello, I need help with Go") {
		t.Error("expected user text in transcript")
	}
	if !contains(transcript, "I can help with that") {
		t.Error("expected assistant text in transcript")
	}
	// Should NOT contain progress event
	if contains(transcript, "working...") {
		t.Error("expected progress events to be filtered out")
	}
}

func TestClaudeCodeAdapter_ReadTranscript_StringContent(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "test.jsonl")

	// Test with string content (not array of blocks)
	line := `{"type":"user","message":{"role":"user","content":"plain string content"}}` + "\n"
	if err := os.WriteFile(jsonlPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewClaudeCodeAdapter(tmpDir)
	session := SessionInfo{ID: "test", FilePath: jsonlPath}
	transcript, err := adapter.ReadTranscript(context.Background(), session)
	if err != nil {
		t.Fatalf("ReadTranscript failed: %v", err)
	}

	if !contains(transcript, "plain string content") {
		t.Errorf("expected string content in transcript, got %q", transcript)
	}
}

func TestClaudeCodeAdapter_ReadTranscript_FileNotFound(t *testing.T) {
	adapter := NewClaudeCodeAdapter(t.TempDir())
	session := SessionInfo{ID: "test", FilePath: "/nonexistent/file.jsonl"}
	_, err := adapter.ReadTranscript(context.Background(), session)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestClaudeCodeAdapter_ReadTranscript_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "bad.jsonl")
	if err := os.WriteFile(jsonlPath, []byte("not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewClaudeCodeAdapter(tmpDir)
	session := SessionInfo{ID: "test", FilePath: jsonlPath}
	transcript, err := adapter.ReadTranscript(context.Background(), session)
	if err != nil {
		t.Fatalf("expected no error for invalid JSON lines (skip gracefully), got %v", err)
	}
	if transcript != "" {
		t.Errorf("expected empty transcript for invalid JSON, got %q", transcript)
	}
}

func TestClaudeCodeAdapter_ReadTranscript_ToolUseBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	jsonlPath := filepath.Join(tmpDir, "test.jsonl")

	// Entry with tool_use block type (should be skipped, only "text" extracted)
	line := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{}},{"type":"text","text":"Here is the result"}]}}` + "\n"
	if err := os.WriteFile(jsonlPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := NewClaudeCodeAdapter(tmpDir)
	session := SessionInfo{ID: "test", FilePath: jsonlPath}
	transcript, err := adapter.ReadTranscript(context.Background(), session)
	if err != nil {
		t.Fatalf("ReadTranscript failed: %v", err)
	}

	if !contains(transcript, "Here is the result") {
		t.Error("expected text content to be extracted")
	}
}

func TestExtractTextFromJSONL_NilMessage(t *testing.T) {
	line := `{"type":"user"}`
	result := extractTextFromJSONL([]byte(line))
	if result != "" {
		t.Errorf("expected empty string for nil message, got %q", result)
	}
}

func TestExtractTextFromJSONL_EmptyContent(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":[]}}`
	result := extractTextFromJSONL([]byte(line))
	if result != "" {
		t.Errorf("expected empty string for empty content array, got %q", result)
	}
}

func TestFindJSONLFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure with JSONL files
	subDir := filepath.Join(tmpDir, "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "agent.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-JSONL file should be ignored
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := findJSONLFiles(tmpDir)
	if err != nil {
		t.Fatalf("findJSONLFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("expected 2 JSONL files, got %d: %v", len(files), files)
	}
}

func TestFindJSONLFiles_EmptyDir(t *testing.T) {
	files, err := findJSONLFiles(t.TempDir())
	if err != nil {
		t.Fatalf("findJSONLFiles failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
