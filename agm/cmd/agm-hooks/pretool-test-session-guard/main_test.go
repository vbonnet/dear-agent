package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSessionName(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		wantSession string
	}{
		{
			name:        "basic session creation",
			command:     "agm session new test-foo",
			wantSession: "test-foo",
		},
		{
			name:        "session with workspace flag",
			command:     "agm session new test-bar --workspace=oss",
			wantSession: "test-bar",
		},
		{
			name:        "session with test flag before name",
			command:     "agm session new --test test-baz",
			wantSession: "test-baz",
		},
		{
			name:        "session with multiple flags",
			command:     "agm session new --workspace=oss --test my-session",
			wantSession: "my-session",
		},
		{
			name:        "not a session creation command",
			command:     "agm session list",
			wantSession: "",
		},
		{
			name:        "completely unrelated command",
			command:     "ls -la",
			wantSession: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &TestSessionGuard{}
			got := guard.extractSessionName(tt.command)
			assert.Equal(t, tt.wantSession, got)
		})
	}
}

func TestIsTestPattern(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		wantMatch   bool
	}{
		{
			name:        "lowercase test prefix",
			sessionName: "test-foo",
			wantMatch:   true,
		},
		{
			name:        "uppercase test prefix",
			sessionName: "TEST-bar",
			wantMatch:   true,
		},
		{
			name:        "mixed case test prefix",
			sessionName: "Test-baz",
			wantMatch:   true,
		},
		{
			name:        "no test prefix",
			sessionName: "my-session",
			wantMatch:   false,
		},
		{
			name:        "test in middle",
			sessionName: "my-test-session",
			wantMatch:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &TestSessionGuard{}
			got := guard.isTestPattern(tt.sessionName)
			assert.Equal(t, tt.wantMatch, got)
		})
	}
}

func TestHasTestFlag(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "has test flag",
			command: "agm session new --test test-foo",
			want:    true,
		},
		{
			name:    "no test flag",
			command: "agm session new test-foo",
			want:    false,
		},
		{
			name:    "test flag at end",
			command: "agm session new test-foo --test",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &TestSessionGuard{}
			got := guard.hasTestFlag(tt.command)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasOverrideFlag(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{
			name:    "has override flag",
			command: "agm session new test-foo --allow-test-name",
			want:    true,
		},
		{
			name:    "no override flag",
			command: "agm session new test-foo",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &TestSessionGuard{}
			got := guard.hasOverrideFlag(tt.command)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name      string
		toolName  string
		toolInput string
		wantExit  int
	}{
		{
			name:      "non-Bash tool (should allow)",
			toolName:  "Read",
			toolInput: "anything",
			wantExit:  0,
		},
		{
			name:      "not a session creation command (should allow)",
			toolName:  "Bash",
			toolInput: "agm session list",
			wantExit:  0,
		},
		{
			name:      "non-test session name (should allow)",
			toolName:  "Bash",
			toolInput: "agm session new my-session",
			wantExit:  0,
		},
		{
			name:      "test session with --test flag (should allow)",
			toolName:  "Bash",
			toolInput: "agm session new --test test-foo",
			wantExit:  0,
		},
		{
			name:      "test session with --allow-test-name (should allow)",
			toolName:  "Bash",
			toolInput: "agm session new test-foo --allow-test-name",
			wantExit:  0,
		},
		{
			name:      "test session without flags (should block)",
			toolName:  "Bash",
			toolInput: "agm session new test-foo",
			wantExit:  1,
		},
		{
			name:      "TEST session without flags (should block)",
			toolName:  "Bash",
			toolInput: "agm session new TEST-bar",
			wantExit:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			os.Setenv("CLAUDE_TOOL_NAME", tt.toolName)
			os.Setenv("CLAUDE_TOOL_INPUT", tt.toolInput)
			defer func() {
				os.Unsetenv("CLAUDE_TOOL_NAME")
				os.Unsetenv("CLAUDE_TOOL_INPUT")
			}()

			guard := NewTestSessionGuard()
			got := guard.Run()
			assert.Equal(t, tt.wantExit, got)
		})
	}
}

func TestGenerateErrorMessage(t *testing.T) {
	guard := &TestSessionGuard{}
	msg := guard.generateErrorMessage("test-foo")

	// Basic sanity check
	assert.Contains(t, msg, "test-foo")
	assert.Contains(t, msg, "❌")
	assert.Contains(t, msg, "--test")
	assert.Contains(t, msg, "--allow-test-name")
}
