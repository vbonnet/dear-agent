package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldMask_AboveThreshold(t *testing.T) {
	m := &ResponseMasker{
		toolResult: strings.Repeat("x", 7000),
		threshold:  6000,
	}
	assert.True(t, m.shouldMask(), "should mask when result exceeds threshold")
}

func TestShouldMask_BelowThreshold(t *testing.T) {
	m := &ResponseMasker{
		toolResult: strings.Repeat("x", 5000),
		threshold:  6000,
	}
	assert.False(t, m.shouldMask(), "should not mask when result is below threshold")
}

func TestShouldMask_ExactThreshold(t *testing.T) {
	m := &ResponseMasker{
		toolResult: strings.Repeat("x", 6000),
		threshold:  6000,
	}
	assert.False(t, m.shouldMask(), "should not mask when result equals threshold exactly")
}

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		skip     []string
		want     bool
	}{
		{"Read is skipped", "Read", []string{"Read", "Agent"}, true},
		{"Agent is skipped", "Agent", []string{"Read", "Agent"}, true},
		{"Bash is not skipped", "Bash", []string{"Read", "Agent"}, false},
		{"case insensitive", "read", []string{"Read", "Agent"}, true},
		{"Grep not skipped by default", "Grep", []string{"Read", "Agent"}, false},
		{"WebSearch not skipped by default", "WebSearch", []string{"Read", "Agent"}, false},
		{"empty skip list", "Bash", []string{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ResponseMasker{
				toolName:  tt.toolName,
				skipTools: tt.skip,
			}
			assert.Equal(t, tt.want, m.shouldSkip())
		})
	}
}

func TestArchive(t *testing.T) {
	tempDir := t.TempDir()
	content := "This is the full tool output that should be archived to disk."

	m := &ResponseMasker{
		sessionID:  "test-session",
		toolName:   "Bash",
		toolResult: content,
		archiveDir: tempDir,
	}

	path, err := m.archive()
	require.NoError(t, err)
	assert.Contains(t, path, "test-session-Bash-0.txt")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestArchive_SequentialNumbers(t *testing.T) {
	tempDir := t.TempDir()

	m := &ResponseMasker{
		sessionID:  "sess",
		toolName:   "Bash",
		toolResult: "output",
		archiveDir: tempDir,
	}

	// Create first archive
	path0, err := m.archive()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path0, "sess-Bash-0.txt"))

	// Create second archive
	path1, err := m.archive()
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path1, "sess-Bash-1.txt"))
}

func TestFormatSummary(t *testing.T) {
	result := strings.Repeat("abcdefghij", 700) // 7000 chars
	m := &ResponseMasker{
		toolResult: result,
	}

	summary := m.formatSummary("/tmp/agm/sess/sess-Bash-0.txt")

	assert.Contains(t, summary, "[Archived: /tmp/agm/sess/sess-Bash-0.txt]")
	assert.Contains(t, summary, "Summary:")
	assert.Contains(t, summary, "7000 chars")
	assert.Contains(t, summary, "1750 tokens")
	// Preview should be truncated
	assert.True(t, len(summary) < 500, "summary should be compact")
}

func TestFormatSummary_ShortResult(t *testing.T) {
	m := &ResponseMasker{
		toolResult: "short output",
	}

	summary := m.formatSummary("/tmp/archive.txt")

	assert.Contains(t, summary, "short output...")
	assert.Contains(t, summary, "12 chars")
	assert.Contains(t, summary, "3 tokens")
}

func TestSessionIDInArchivePath(t *testing.T) {
	tempDir := t.TempDir()

	m := &ResponseMasker{
		sessionID:  "my-unique-session-id",
		toolName:   "Grep",
		toolResult: "some output",
		archiveDir: tempDir,
	}

	path, err := m.archive()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(tempDir, "my-unique-session-id-Grep-0.txt"), path)
}

func TestNewResponseMasker_Defaults(t *testing.T) {
	// Clear relevant env vars
	os.Unsetenv("AGM_RESPONSE_MASK_THRESHOLD")
	os.Unsetenv("AGM_RESPONSE_MASK_SKIP")
	os.Setenv("CLAUDE_SESSION_ID", "test-id")
	os.Setenv("CLAUDE_TOOL_NAME", "Bash")
	os.Setenv("CLAUDE_TOOL_RESULT", "hello")
	defer func() {
		os.Unsetenv("CLAUDE_SESSION_ID")
		os.Unsetenv("CLAUDE_TOOL_NAME")
		os.Unsetenv("CLAUDE_TOOL_RESULT")
	}()

	m := NewResponseMasker()

	assert.Equal(t, 6000, m.threshold)
	assert.Equal(t, []string{"Read", "Agent"}, m.skipTools)
	assert.Equal(t, "test-id", m.sessionID)
	assert.Equal(t, "Bash", m.toolName)
	assert.Equal(t, "hello", m.toolResult)
}

func TestNewResponseMasker_CustomConfig(t *testing.T) {
	os.Setenv("AGM_RESPONSE_MASK_THRESHOLD", "3000")
	os.Setenv("AGM_RESPONSE_MASK_SKIP", "Read,Agent,Glob")
	os.Setenv("CLAUDE_SESSION_ID", "s1")
	defer func() {
		os.Unsetenv("AGM_RESPONSE_MASK_THRESHOLD")
		os.Unsetenv("AGM_RESPONSE_MASK_SKIP")
		os.Unsetenv("CLAUDE_SESSION_ID")
	}()

	m := NewResponseMasker()

	assert.Equal(t, 3000, m.threshold)
	assert.Equal(t, []string{"Read", "Agent", "Glob"}, m.skipTools)
}

func TestRun_SkippedTool(t *testing.T) {
	m := &ResponseMasker{
		toolName:   "Read",
		toolResult: strings.Repeat("x", 10000),
		threshold:  6000,
		skipTools:  []string{"Read", "Agent"},
	}

	exitCode := m.Run()
	assert.Equal(t, 0, exitCode)
}

func TestRun_BelowThreshold(t *testing.T) {
	m := &ResponseMasker{
		toolName:   "Bash",
		toolResult: "short",
		threshold:  6000,
		skipTools:  []string{"Read", "Agent"},
	}

	exitCode := m.Run()
	assert.Equal(t, 0, exitCode)
}
