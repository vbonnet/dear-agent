package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResumeCommandFlags verifies that the resume command properly parses flags
func TestResumeCommandFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
		description string
	}{
		{
			name:        "accepts --detached flag",
			args:        []string{"session-name", "--detached"},
			expectError: false,
			description: "Should accept --detached flag",
		},
		{
			name:        "works without --detached flag",
			args:        []string{"session-name"},
			expectError: false,
			description: "Should work without --detached flag (default behavior)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the flag value before each test
			resumeDetached = false

			// Parse flags (this simulates cobra command parsing)
			resumeCmd.ResetFlags()
			resumeCmd.Flags().BoolVar(&resumeDetached, "detached", false, "Resume session without attaching")

			// Test that the command accepts the flags
			// Note: We can't fully test the execution without mocking tmux,
			// but we can verify the flag parsing works correctly
			err := resumeCmd.ParseFlags(tt.args)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}

			// Verify the flag value is correctly set for --detached test
			if tt.name == "accepts --detached flag" && !resumeDetached {
				t.Errorf("%s: --detached flag should be true", tt.description)
			}

			// Verify the flag value is false for default test
			if tt.name == "works without --detached flag" && resumeDetached {
				t.Errorf("%s: detached flag should be false by default", tt.description)
			}
		})
	}
}

// TestResumeDetachedHelp verifies the help text includes --detached documentation
func TestResumeDetachedHelp(t *testing.T) {
	helpText := resumeCmd.Long

	if helpText == "" {
		t.Fatal("Resume command should have Long help text")
	}

	// Verify help mentions --detached
	if !contains(helpText, "--detached") && !contains(helpText, "detached") {
		t.Error("Resume command help should mention --detached flag")
	}

	// Verify help explains detached behavior
	if !contains(helpText, "background") && !contains(helpText, "without attaching") {
		t.Error("Resume command help should explain detached mode behavior")
	}
}

// contains is a simple helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || contains(s[1:], substr)))
}

// TestResumePromptFlagParsing verifies --prompt and --prompt-file flags are registered
// and parsed correctly. These flags enable crash recovery by injecting a prompt
// after the session is resumed.
func TestResumePromptFlagParsing(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantPrompt   string
		wantFile     string
		expectError  bool
		description  string
	}{
		{
			name:        "accepts --prompt flag",
			args:        []string{"session-name", "--prompt", "continue working on X"},
			wantPrompt:  "continue working on X",
			wantFile:    "",
			expectError: false,
			description: "Should accept inline --prompt text",
		},
		{
			name:        "accepts --prompt-file flag",
			args:        []string{"session-name", "--prompt-file", "/tmp/recovery.txt"},
			wantPrompt:  "",
			wantFile:    "/tmp/recovery.txt",
			expectError: false,
			description: "Should accept --prompt-file path",
		},
		{
			name:        "works without prompt flags",
			args:        []string{"session-name"},
			wantPrompt:  "",
			wantFile:    "",
			expectError: false,
			description: "Prompt flags should be optional",
		},
		{
			name:        "accepts --detached with --prompt",
			args:        []string{"session-name", "--detached", "--prompt", "pick up where you left off"},
			wantPrompt:  "pick up where you left off",
			wantFile:    "",
			expectError: false,
			description: "Should accept --detached combined with --prompt for background crash recovery",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset vars before each test
			resumeDetached = false
			resumePrompt = ""
			resumePromptFile = ""

			// Re-register all flags (ResetFlags clears them)
			resumeCmd.ResetFlags()
			resumeCmd.Flags().BoolVar(&resumeDetached, "detached", false, "Resume session without attaching")
			resumeCmd.Flags().BoolVar(&resumeForceParent, "force-parent", false, "Resume planning session instead of execution session")
			resumeCmd.Flags().StringVar(&resumePrompt, "prompt", "", "Prompt to send after resume")
			resumeCmd.Flags().StringVar(&resumePromptFile, "prompt-file", "", "File containing prompt to send after resume")

			err := resumeCmd.ParseFlags(tt.args)

			if tt.expectError && err == nil {
				t.Errorf("%s: expected error but got none", tt.description)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: unexpected error: %v", tt.description, err)
			}

			if resumePrompt != tt.wantPrompt {
				t.Errorf("%s: resumePrompt = %q, want %q", tt.description, resumePrompt, tt.wantPrompt)
			}
			if resumePromptFile != tt.wantFile {
				t.Errorf("%s: resumePromptFile = %q, want %q", tt.description, resumePromptFile, tt.wantFile)
			}
		})
	}
}

// TestResumeHelpMentionsPromptFlags verifies the help text documents the new flags.
func TestResumeHelpMentionsPromptFlags(t *testing.T) {
	helpText := resumeCmd.Long
	if helpText == "" {
		t.Fatal("Resume command should have Long help text")
	}
	if !contains(helpText, "--prompt") {
		t.Error("Resume command help should mention --prompt flag")
	}
	if !contains(helpText, "--prompt-file") {
		t.Error("Resume command help should mention --prompt-file flag")
	}
	if !contains(helpText, "crash recovery") && !contains(helpText, "background resume") {
		t.Error("Resume command help should explain prompt flags in context of crash recovery or background resume")
	}
}

// TestSendPostResumePrompt_FileNotFound verifies an error is returned when the
// prompt file does not exist, before any tmux operations occur.
func TestSendPostResumePrompt_FileNotFound(t *testing.T) {
	err := sendPostResumePrompt("any-session", "", "/nonexistent/path/prompt.txt")
	if err == nil {
		t.Fatal("expected error for missing prompt file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read prompt file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestSendPostResumePrompt_FileTooLarge verifies the 10KB size limit is enforced
// before any tmux operations occur.
func TestSendPostResumePrompt_FileTooLarge(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "large.txt")
	// Write 11KB of data (exceeds 10KB limit)
	data := make([]byte, 11*1024)
	for i := range data {
		data[i] = 'x'
	}
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	err := sendPostResumePrompt("any-session", "", tmp)
	if err == nil {
		t.Fatal("expected error for oversized prompt file, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unexpected error message: %v", err)
	}
}


// Regression tests for session-resume fix (commit e7cacf8)
// Bug: resume sent commands to existing tmux sessions, injecting text
// into the running agent which got processed as a user prompt.

func TestShouldSendResumeCommands_NeverSendsToExistingSession(t *testing.T) {
	// The fix ensures that when a tmux session already exists,
	// we NEVER send commands to it — just attach.
	if shouldSendResumeCommands(true) {
		t.Error("shouldSendResumeCommands(tmuxExists=true) = true, want false: must never send commands to existing sessions")
	}
}

func TestShouldSendResumeCommands_SendsWhenCreatingNew(t *testing.T) {
	// When no tmux session exists, we need to create one and send
	// the resume command to start the agent.
	if !shouldSendResumeCommands(false) {
		t.Error("shouldSendResumeCommands(tmuxExists=false) = false, want true: must send commands when creating new session")
	}
}

func TestShouldSendResumeCommands_TableDriven(t *testing.T) {
	tests := []struct {
		name        string
		tmuxExists  bool
		wantSend    bool
		description string
	}{
		{
			name:        "existing session - agent running",
			tmuxExists:  true,
			wantSend:    false,
			description: "Must not inject commands into running agent",
		},
		{
			name:        "existing session - agent idle",
			tmuxExists:  true,
			wantSend:    false,
			description: "Even if agent appears idle, detection is unreliable",
		},
		{
			name:        "existing session - detection error",
			tmuxExists:  true,
			wantSend:    false,
			description: "When detection fails, safe default is no commands",
		},
		{
			name:        "no session - must create and send",
			tmuxExists:  false,
			wantSend:    true,
			description: "New session needs resume command to start agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSendResumeCommands(tt.tmuxExists)
			if got != tt.wantSend {
				t.Errorf("shouldSendResumeCommands(tmuxExists=%v) = %v, want %v: %s",
					tt.tmuxExists, got, tt.wantSend, tt.description)
			}
		})
	}
}
