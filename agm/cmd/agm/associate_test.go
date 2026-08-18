package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestAssociateTimingDetection verifies that the associate command can detect
// recent /rename commands to provide better error messages about timing issues
func TestAssociateTimingDetection(t *testing.T) {
	tests := []struct {
		name         string
		setupHistory func(t *testing.T, historyPath string, sessionName string)
		sessionName  string
		expectTiming bool
		description  string
	}{
		{
			name:        "detects recent rename command",
			sessionName: "test-session",
			setupHistory: func(t *testing.T, historyPath string, sessionName string) {
				// Create history file with recent rename
				entries := []history.ConversationEntry{
					{
						Display:   "/rename " + sessionName,
						Timestamp: time.Now().UnixMilli() - 5000, // 5 seconds ago
						Project:   "~/src",
						SessionID: "test-uuid-123",
					},
				}
				writeHistoryEntries(t, historyPath, entries)
			},
			expectTiming: true,
			description:  "Should detect rename command from 5 seconds ago",
		},
		{
			name:        "detects combined rename and associate command",
			sessionName: "test-session",
			setupHistory: func(t *testing.T, historyPath string, sessionName string) {
				// Create history file with combined command
				entries := []history.ConversationEntry{
					{
						Display:   "/rename " + sessionName + "\n/agm:agm-assoc " + sessionName,
						Timestamp: time.Now().UnixMilli() - 2000, // 2 seconds ago
						Project:   "~/src",
						SessionID: "test-uuid-123",
					},
				}
				writeHistoryEntries(t, historyPath, entries)
			},
			expectTiming: true,
			description:  "Should detect combined rename/associate command",
		},
		{
			name:        "ignores old rename command",
			sessionName: "test-session",
			setupHistory: func(t *testing.T, historyPath string, sessionName string) {
				// Create history file with old rename
				entries := []history.ConversationEntry{
					{
						Display:   "/rename " + sessionName,
						Timestamp: time.Now().UnixMilli() - 60000, // 60 seconds ago
						Project:   "~/src",
						SessionID: "test-uuid-123",
					},
				}
				writeHistoryEntries(t, historyPath, entries)
			},
			expectTiming: false,
			description:  "Should ignore rename command from 60 seconds ago (outside 30s window)",
		},
		{
			name:        "no rename command found",
			sessionName: "test-session",
			setupHistory: func(t *testing.T, historyPath string, sessionName string) {
				// Create history file with other commands
				entries := []history.ConversationEntry{
					{
						Display:   "some other command",
						Timestamp: time.Now().UnixMilli() - 5000,
						Project:   "~/src",
						SessionID: "test-uuid-123",
					},
				}
				writeHistoryEntries(t, historyPath, entries)
			},
			expectTiming: false,
			description:  "Should not detect timing issue when no rename found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory for test
			tmpDir := t.TempDir()
			historyPath := filepath.Join(tmpDir, "history.jsonl")

			// Setup test history
			tt.setupHistory(t, historyPath, tt.sessionName)

			// Test the detection logic (simulates what associate.go does)
			parser := history.NewParser(historyPath)
			sessions, err := parser.ReadConversations(100)
			if err != nil {
				t.Fatalf("Failed to read conversations: %v", err)
			}

			hasRecentRename := false
			renameCmd := "/rename " + tt.sessionName
			currentTime := time.Now().UnixMilli()

			for _, session := range sessions {
				for _, entry := range session.Entries {
					if currentTime-entry.Timestamp < 30000 { // 30 seconds
						if entry.Display == renameCmd || entry.Display == renameCmd+"\n/agm:agm-assoc "+tt.sessionName {
							hasRecentRename = true
							break
						}
					}
				}
				if hasRecentRename {
					break
				}
			}

			if hasRecentRename != tt.expectTiming {
				t.Errorf("%s: expected timing detection=%v, got=%v", tt.description, tt.expectTiming, hasRecentRename)
			}
		})
	}
}

// TestAssociateRenameFlag verifies that the --rename flag is registered
func TestAssociateRenameFlag(t *testing.T) {
	flag := associateCmd.Flags().Lookup("rename")
	if flag == nil {
		t.Fatal("expected --rename flag to be registered on associate command")
		return
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default value false, got %s", flag.DefValue)
	}
}

func TestHarnessFromPaneCommands(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		want     string
	}{
		{name: "codex", commands: []string{"zsh", "codex"}, want: "codex-cli"},
		{name: "codex full path", commands: []string{"/usr/local/bin/codex"}, want: "codex-cli"},
		{name: "codex node wrapper start command", commands: []string{"node", "codex -m gpt-5.4 -C /tmp/work"}, want: "codex-cli"},
		{name: "codex quoted node wrapper start command", commands: []string{"node", `"codex -m gpt-5.4 -C /tmp/work"`}, want: "codex-cli"},
		{name: "codex quoted executable path with spaces", commands: []string{"node", `"/Applications/Codex App.app/Contents/MacOS/codex" -m gpt-5.4`}, want: "codex-cli"},
		{name: "gemini", commands: []string{"gemini"}, want: "gemini-cli"},
		{name: "opencode", commands: []string{"opencode"}, want: "opencode-cli"},
		{name: "claude", commands: []string{"claude"}, want: "claude-code"},
		{name: "no match", commands: []string{"zsh", "vim"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := harnessFromPaneCommands(tt.commands); got != tt.want {
				t.Errorf("harnessFromPaneCommands(%v) = %q, want %q", tt.commands, got, tt.want)
			}
		})
	}
}

func TestResolveAssociateHarness(t *testing.T) {
	origHarness := associateHarness
	t.Cleanup(func() { associateHarness = origHarness })

	existing := &manifest.Manifest{Harness: "codex-cli"}
	associateHarness = "claude-code"
	got, err := resolveAssociateHarness("s", existing)
	if err != nil {
		t.Fatalf("existing harness resolution failed: %v", err)
	}
	if got != "codex-cli" {
		t.Fatalf("existing harness should win, got %q", got)
	}

	associateHarness = ""
	got, err = resolveAssociateHarness("s", nil)
	if err != nil {
		t.Fatalf("default harness resolution failed: %v", err)
	}
	if got != "claude-code" {
		t.Fatalf("default harness = %q, want claude-code", got)
	}

	associateHarness = "codex-cli"
	got, err = resolveAssociateHarness("s", nil)
	if err != nil {
		t.Fatalf("explicit codex harness failed: %v", err)
	}
	if got != "codex-cli" {
		t.Fatalf("explicit harness = %q, want codex-cli", got)
	}

	associateHarness = "bogus"
	if _, err := resolveAssociateHarness("s", nil); err == nil {
		t.Fatal("invalid harness should fail")
	}
}

func TestUpdateNonClaudeAssociationManifest_PreservesExistingProjectAndWorkingDirectory(t *testing.T) {
	newWD := t.TempDir()
	t.Chdir(newWD)

	m := &manifest.Manifest{
		Context:          manifest.Context{Project: "/tmp/existing-project"},
		WorkingDirectory: "/tmp/existing-project",
		Tmux:             manifest.Tmux{SessionName: "existing"},
		Harness:          "agy",
		Model:            "3.5-flash",
		SessionID:        "sid-1",
	}

	updateNonClaudeAssociationManifest(m, "existing", "agy", "oss")

	if m.Context.Project != "/tmp/existing-project" {
		t.Fatalf("project = %q, want existing value", m.Context.Project)
	}
	if m.WorkingDirectory != "/tmp/existing-project" {
		t.Fatalf("working directory = %q, want existing value", m.WorkingDirectory)
	}
}

func TestNewNonClaudeAssociationManifestLeavesAgyModelUnknown(t *testing.T) {
	m := newNonClaudeAssociationManifest("agy-associated", "agy", "oss")
	if m.Model != "" {
		t.Fatalf("associated AGY model = %q, want unknown model left unset", m.Model)
	}
	if m.Harness != "agy" {
		t.Fatalf("associated harness = %q, want agy", m.Harness)
	}
}

func TestUpdateNonClaudeAssociationManifestLeavesAgyModelUnknown(t *testing.T) {
	m := &manifest.Manifest{}
	updateNonClaudeAssociationManifest(m, "agy-associated", "agy", "oss")
	if m.Model != "" {
		t.Fatalf("updated AGY model = %q, want unknown model left unset", m.Model)
	}
}

// writeHistoryEntries is a helper to create a test history.jsonl file
func writeHistoryEntries(t *testing.T, historyPath string, entries []history.ConversationEntry) {
	t.Helper()

	file, err := os.Create(historyPath)
	if err != nil {
		t.Fatalf("Failed to create history file: %v", err)
	}
	defer file.Close()

	for _, entry := range entries {
		// Write each entry as JSON line
		line := fmt.Sprintf(`{"display":%q,"pastedContents":{},"timestamp":%d,"project":%q,"sessionId":%q}%s`,
			entry.Display, entry.Timestamp, entry.Project, entry.SessionID, "\n")
		if _, err := file.WriteString(line); err != nil {
			t.Fatalf("Failed to write history entry: %v", err)
		}
	}
}
