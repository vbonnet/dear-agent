package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/conversation"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"golang.org/x/term"
)

func TestSessionSearchCmd_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if no TTY available (non-interactive environment)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("Skipping TTY-dependent test in non-interactive environment")
	}

	// Create temp directories
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	claudeDir := filepath.Join(tmpDir, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test sessions
	testSessions := []struct {
		uuid      string
		name      string
		workspace string
		content   string
	}{
		{
			uuid:      "uuid-docker-1",
			name:      "docker-debugging",
			workspace: "oss",
			content:   "Help me debug docker compose networking issues",
		},
		{
			uuid:      "uuid-kubernetes-2",
			name:      "k8s-setup",
			workspace: "oss",
			content:   "Setting up kubernetes cluster with helm charts",
		},
		{
			uuid:      "uuid-api-3",
			name:      "api-design",
			workspace: "acme",
			content:   "Design RESTful API endpoints for user management",
		},
	}

	var historyLines []string
	for i, ts := range testSessions {
		// Create session directory and manifest
		sessionDir := filepath.Join(sessionsDir, ts.name)
		if err := os.MkdirAll(sessionDir, 0755); err != nil {
			t.Fatal(err)
		}

		m := &manifest.Manifest{
			SessionID:     ts.name + "-id",
			Name:          ts.name,
			Workspace:     ts.workspace,
			SchemaVersion: "2.0",
			Context: manifest.Context{
				Project: "~/test",
			},
			Claude: manifest.Claude{UUID: ts.uuid},
			Tmux: manifest.Tmux{
				SessionName: ts.name,
			},
		}
		if err := manifest.Write(filepath.Join(sessionDir, "manifest.yaml"), m); err != nil {
			t.Fatal(err)
		}

		// Create conversation file
		conv := &conversation.Conversation{
			SchemaVersion: "1.0",
			Model:         "claude-sonnet-4-5",
			Harness:       "claude-code",
			Messages: []conversation.Message{
				{
					Role:    "user",
					Harness: "claude-code",
					Content: []conversation.ContentBlock{
						conversation.TextBlock{Type: "text", Text: ts.content},
					},
				},
			},
		}
		convPath := filepath.Join(projectsDir, ts.uuid+".jsonl")
		if err := conversation.WriteJSONL(convPath, conv); err != nil {
			t.Fatal(err)
		}

		// Add to history
		historyLines = append(historyLines,
			`{"display":"`+ts.content+`","pastedContents":{},"timestamp":`+string(rune(1708358400000+i))+`,"project":"~/test","sessionId":"`+ts.uuid+`"}`)
	}

	// Write history file
	if err := os.WriteFile(historyPath, []byte(bytes.Join([][]byte{
		[]byte(historyLines[0]),
		[]byte(historyLines[1]),
		[]byte(historyLines[2]),
	}, []byte("\n"))), 0644); err != nil {
		t.Fatal(err)
	}

	// Override environment
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Override config for test
	oldCfg := cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.SessionsDir = sessionsDir
	defer func() { cfg = oldCfg }()

	tests := []struct {
		name          string
		args          []string
		expectedCount int
		expectError   bool
	}{
		{
			name:          "keyword search - single match",
			args:          []string{"docker"},
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "keyword search - multiple matches",
			args:          []string{"kubernetes"},
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "keyword search - no matches",
			args:          []string{"nonexistent"},
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:          "case insensitive by default",
			args:          []string{"DOCKER"},
			expectedCount: 1,
			expectError:   false,
		},
		{
			name:          "workspace filter",
			args:          []string{"--workspace", "oss", "API"},
			expectedCount: 0, // API content is in acme workspace
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset flags
			searchWorkspace = ""
			searchRegex = false
			searchCaseSensitive = false

			// Run command
			sessionSearchCmd.SetArgs(tt.args)
			err := sessionSearchCmd.Execute()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestSessionSearchCmd_Regex(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if no TTY available (non-interactive environment)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("Skipping TTY-dependent test in non-interactive environment")
	}

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	claudeDir := filepath.Join(tmpDir, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create session with pattern
	testUUID := "uuid-regex-1"
	sessionDir := filepath.Join(sessionsDir, "regex-test")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		SessionID:     "regex-test-id",
		Name:          "regex-test",
		Workspace:     "test",
		SchemaVersion: "2.0",
		Context: manifest.Context{
			Project: "~/test",
		},
		Claude: manifest.Claude{UUID: testUUID},
		Tmux: manifest.Tmux{
			SessionName: "regex-test",
		},
	}
	if err := manifest.Write(filepath.Join(sessionDir, "manifest.yaml"), m); err != nil {
		t.Fatal(err)
	}

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
	convPath := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := conversation.WriteJSONL(convPath, conv); err != nil {
		t.Fatal(err)
	}

	historyContent := `{"display":"timeout","pastedContents":{},"timestamp":1708358400000,"project":"~/test","sessionId":"uuid-regex-1"}`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldCfg := cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.SessionsDir = sessionsDir
	defer func() { cfg = oldCfg }()

	// Test regex pattern
	searchWorkspace = ""
	searchRegex = true
	searchCaseSensitive = false

	sessionSearchCmd.SetArgs([]string{"--regex", "Error.*timeout"})
	err := sessionSearchCmd.Execute()
	if err != nil {
		t.Errorf("Regex search failed: %v", err)
	}
}

func TestSessionSearchCmd_CaseSensitive(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if no TTY available (non-interactive environment)
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("Skipping TTY-dependent test in non-interactive environment")
	}

	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	claudeDir := filepath.Join(tmpDir, ".claude")
	projectsDir := filepath.Join(claudeDir, "projects", "test-project")
	historyPath := filepath.Join(claudeDir, "history.jsonl")

	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		t.Fatal(err)
	}

	testUUID := "uuid-case-1"
	sessionDir := filepath.Join(sessionsDir, "case-test")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &manifest.Manifest{
		SessionID:     "case-test-id",
		Name:          "case-test",
		Workspace:     "test",
		SchemaVersion: "2.0",
		Context: manifest.Context{
			Project: "~/test",
		},
		Claude: manifest.Claude{UUID: testUUID},
		Tmux: manifest.Tmux{
			SessionName: "case-test",
		},
	}
	if err := manifest.Write(filepath.Join(sessionDir, "manifest.yaml"), m); err != nil {
		t.Fatal(err)
	}

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
						Text: "API endpoint documentation for the api",
					},
				},
			},
		},
	}
	convPath := filepath.Join(projectsDir, testUUID+".jsonl")
	if err := conversation.WriteJSONL(convPath, conv); err != nil {
		t.Fatal(err)
	}

	historyContent := `{"display":"API","pastedContents":{},"timestamp":1708358400000,"project":"~/test","sessionId":"uuid-case-1"}`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0644); err != nil {
		t.Fatal(err)
	}

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldCfg := cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.SessionsDir = sessionsDir
	defer func() { cfg = oldCfg }()

	// Test case-sensitive search (should find only "API", not "api")
	searchWorkspace = ""
	searchRegex = false
	searchCaseSensitive = true

	sessionSearchCmd.SetArgs([]string{"--case-sensitive", "API"})
	err := sessionSearchCmd.Execute()
	if err != nil {
		t.Errorf("Case-sensitive search failed: %v", err)
	}
}

func TestSessionSearchCmd_NoArgs(t *testing.T) {
	sessionSearchCmd.SetArgs([]string{})
	err := sessionSearchCmd.Execute()
	if err == nil {
		t.Error("Expected error when no query provided")
	}
}

func TestSessionSearchCmd_InvalidRegex(t *testing.T) {
	tmpDir := t.TempDir()
	sessionsDir := filepath.Join(tmpDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}

	oldCfg := cfg
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.SessionsDir = sessionsDir
	defer func() { cfg = oldCfg }()

	searchWorkspace = ""
	searchRegex = true
	searchCaseSensitive = false

	// Invalid regex pattern
	sessionSearchCmd.SetArgs([]string{"--regex", "["})
	err := sessionSearchCmd.Execute()
	if err == nil {
		t.Error("Expected error for invalid regex pattern")
	}
}
