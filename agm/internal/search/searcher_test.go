package search

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/conversation"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestSearcher_Search_Keyword(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temp directories
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test session in database
	testUUID := "test-uuid-123"
	m := &manifest.Manifest{
		SessionID:     "test-session-id",
		Name:          "test-session",
		Workspace:     "test",
		SchemaVersion: "2.0",
		Harness:       "claude-code",
		Lifecycle:     "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "~/test",
		},
		Claude: manifest.Claude{
			UUID: testUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: "test-session",
		},
	}

	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Create conversation file with searchable content
	conv := &conversation.Conversation{
		SchemaVersion: "1.0",
		Model:         "claude-sonnet-4-5",
		Harness:       "claude-code",
		Messages: []conversation.Message{
			{
				Role:    "user",
				Harness: "claude-code",
				Content: []conversation.ContentBlock{
					conversation.TextBlock{
						Type: "text",
						Text: "Can you help me debug this docker compose error?",
					},
				},
			},
			{
				Role:    "assistant",
				Harness: "claude-code",
				Content: []conversation.ContentBlock{
					conversation.TextBlock{
						Type: "text",
						Text: "Sure! Let's look at your docker-compose.yml file.",
					},
				},
			},
		},
	}

	conversationPath := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := conversation.WriteJSONL(conversationPath, conv); err != nil {
		t.Fatal(err)
	}

	// Create history.jsonl
	historyContent := `{"display":"docker compose","pastedContents":{},"timestamp":1708358400000,"project":"~/test","sessionId":"test-uuid-123"}`
	if err := os.WriteFile(historyPath, []byte(historyContent+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	// Test search with adapter
	searcher := NewSearcher(adapter)
	results, err := searcher.Search(SearchOptions{
		Query:         "docker",
		UseRegex:      false,
		CaseSensitive: false,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.SessionUUID != testUUID {
		t.Errorf("Expected UUID %s, got %s", testUUID, r.SessionUUID)
	}
	if r.SessionName != "test-session" {
		t.Errorf("Expected session name 'test-session', got '%s'", r.SessionName)
	}
	if r.MatchCount != 2 {
		t.Errorf("Expected 2 matches, got %d", r.MatchCount)
	}
	if r.Workspace != "test" {
		t.Errorf("Expected workspace 'test', got '%s'", r.Workspace)
	}
}

func TestSearcher_Search_Regex(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temp directories
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test session in database
	testUUID := "test-uuid-456"
	m := &manifest.Manifest{
		SessionID:     "regex-session-id",
		Name:          "regex-session",
		Workspace:     "test",
		SchemaVersion: "2.0",
		Harness:       "claude-code",
		Lifecycle:     "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "~/test",
		},
		Claude: manifest.Claude{
			UUID: testUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: "regex-session",
		},
	}

	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Create conversation with pattern
	conv := &conversation.Conversation{
		SchemaVersion: "1.0",
		Model:         "claude-sonnet-4-5",
		Harness:       "claude-code",
		Messages: []conversation.Message{
			{
				Role:    "user",
				Harness: "claude-code",
				Content: []conversation.ContentBlock{
					conversation.TextBlock{
						Type: "text",
						Text: "Error: connection timeout after 30 seconds",
					},
				},
			},
		},
	}

	conversationPath := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := conversation.WriteJSONL(conversationPath, conv); err != nil {
		t.Fatal(err)
	}

	// Create history.jsonl
	historyContent := `{"display":"timeout error","pastedContents":{},"timestamp":1708358400000,"project":"~/test","sessionId":"test-uuid-456"}`
	if err := os.WriteFile(historyPath, []byte(historyContent+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	// Test regex search with adapter
	searcher := NewSearcher(adapter)
	results, err := searcher.Search(SearchOptions{
		Query:         "Error.*timeout",
		UseRegex:      true,
		CaseSensitive: false,
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.MatchCount != 1 {
		t.Errorf("Expected 1 match, got %d", r.MatchCount)
	}
}

func TestSearcher_Search_CaseSensitive(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temp directories
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test session in database
	testUUID := "test-uuid-789"
	m := &manifest.Manifest{
		SessionID:     "case-session-id",
		Name:          "case-session",
		Workspace:     "test",
		SchemaVersion: "2.0",
		Harness:       "claude-code",
		Lifecycle:     "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "~/test",
		},
		Claude: manifest.Claude{
			UUID: testUUID,
		},
		Tmux: manifest.Tmux{
			SessionName: "case-session",
		},
	}

	if err := adapter.CreateSession(m); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	// Create conversation with mixed case
	conv := &conversation.Conversation{
		SchemaVersion: "1.0",
		Model:         "claude-sonnet-4-5",
		Harness:       "claude-code",
		Messages: []conversation.Message{
			{
				Role:    "user",
				Harness: "claude-code",
				Content: []conversation.ContentBlock{
					conversation.TextBlock{
						Type: "text",
						Text: "API endpoint returns 404 error. The api documentation says...",
					},
				},
			},
		},
	}

	conversationPath := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := conversation.WriteJSONL(conversationPath, conv); err != nil {
		t.Fatal(err)
	}

	// Create history.jsonl
	historyContent := `{"display":"API issue","pastedContents":{},"timestamp":1708358400000,"project":"~/test","sessionId":"test-uuid-789"}`
	if err := os.WriteFile(historyPath, []byte(historyContent+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	searcher := NewSearcher(adapter)

	// Test case-insensitive (should find both "API" and "api")
	results, err := searcher.Search(SearchOptions{
		Query:         "API",
		UseRegex:      false,
		CaseSensitive: false,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].MatchCount != 2 {
		t.Errorf("Expected 2 matches (API + api), got %d", results[0].MatchCount)
	}

	// Test case-sensitive (should find only "API")
	results, err = searcher.Search(SearchOptions{
		Query:         "API",
		UseRegex:      false,
		CaseSensitive: true,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].MatchCount != 1 {
		t.Errorf("Expected 1 match (only API), got %d", results[0].MatchCount)
	}
}

func TestSearcher_Search_WorkspaceFilter(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Create temp directories
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two sessions with different workspaces (both stored in test database)
	testUUID1 := "uuid-oss-1"
	testUUID2 := "uuid-acme-2"

	// Session 1: test workspace (adapter overrides to "test")
	m1 := &manifest.Manifest{
		SessionID:     "oss-session-id",
		Name:          "oss-session",
		Workspace:     "test",
		SchemaVersion: "2.0",
		Harness:       "claude-code",
		Lifecycle:     "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "~/test",
		},
		Claude: manifest.Claude{UUID: testUUID1},
		Tmux: manifest.Tmux{
			SessionName: "oss-session",
		},
	}
	if err := adapter.CreateSession(m1); err != nil {
		t.Fatalf("Failed to create session 1: %v", err)
	}

	// Session 2: test workspace (both in same workspace for test database)
	m2 := &manifest.Manifest{
		SessionID:     "acme-session-id",
		Name:          "acme-session",
		Workspace:     "test",
		SchemaVersion: "2.0",
		Harness:       "claude-code",
		Lifecycle:     "",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: "~/test",
		},
		Claude: manifest.Claude{UUID: testUUID2},
		Tmux: manifest.Tmux{
			SessionName: "acme-session",
		},
	}
	if err := adapter.CreateSession(m2); err != nil {
		t.Fatalf("Failed to create session 2: %v", err)
	}

	// Create conversations for both
	for _, uuid := range []string{testUUID1, testUUID2} {
		conv := &conversation.Conversation{
			SchemaVersion: "1.0",
			Model:         "claude-sonnet-4-5",
			Harness:       "claude-code",
			Messages: []conversation.Message{
				{
					Role:    "user",
					Harness: "claude-code",
					Content: []conversation.ContentBlock{
						conversation.TextBlock{Type: "text", Text: "test content"},
					},
				},
			},
		}
		convPath := filepath.Join(projectsDir, uuid+".jsonl")
		if err := conversation.WriteJSONL(convPath, conv); err != nil {
			t.Fatal(err)
		}
	}

	// Create history.jsonl with both sessions
	historyContent := `{"display":"test 1","pastedContents":{},"timestamp":1708358400000,"project":"~/test","sessionId":"uuid-oss-1"}
{"display":"test 2","pastedContents":{},"timestamp":1708358401000,"project":"~/test","sessionId":"uuid-acme-2"}`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override home directory for test
	t.Setenv("HOME", tmpDir)

	searcher := NewSearcher(adapter)

	// Test workspace filter (both sessions are in "test" workspace in test DB)
	results, err := searcher.Search(SearchOptions{
		Query:     "test",
		Workspace: "test",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 results (both in test workspace), got %d", len(results))
	}
	// Both should be in test workspace
	for _, r := range results {
		if r.Workspace != "test" {
			t.Errorf("Expected workspace 'test', got '%s'", r.Workspace)
		}
	}
}

func TestSearcher_Search_NoMatches(t *testing.T) {
	// Get test adapter
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty history
	if err := os.WriteFile(historyPath, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", tmpDir)

	searcher := NewSearcher(adapter)
	results, err := searcher.Search(SearchOptions{
		Query: "nonexistent",
	})

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short text",
			input:  "hello world",
			maxLen: 20,
			want:   "hello world",
		},
		{
			name:   "exact length",
			input:  "exactly twenty chars",
			maxLen: 20,
			want:   "exactly twenty chars",
		},
		{
			name:   "needs truncation",
			input:  "this is a very long text that needs to be truncated",
			maxLen: 20,
			want:   "this is a very lo...",
		},
		{
			name:   "with newlines",
			input:  "line 1\nline 2\nline 3",
			maxLen: 20,
			want:   "line 1 line 2 line 3",
		},
		{
			name:   "with excessive whitespace",
			input:  "lots   of    spaces",
			maxLen: 20,
			want:   "lots of spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateText(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateText() = %q, want %q", got, tt.want)
			}
		})
	}
}
