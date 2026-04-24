package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

func TestParseHookError_DeniedTool(t *testing.T) {
	raw := "Hook PreToolUse:Bash denied this tool"

	he, err := ParseHookError(raw)
	require.NoError(t, err)
	assert.Equal(t, "Bash", he.Tool)
	assert.Empty(t, he.HookName)
	assert.Empty(t, he.Reason)
	assert.Equal(t, raw, he.Raw)
}

func TestParseHookError_BlockingError(t *testing.T) {
	raw := `Hook PreToolUse:Bash blocking error from command: "~/.claude/hooks/pretool-bash-blocker": text processing (ls/grep/sed/awk/head/tail/wc/cut/sort/uniq) - Use Grep tool (for grep), Glob tool (for ls/find), Read tool (for head/tail/cat)`

	he, err := ParseHookError(raw)
	require.NoError(t, err)
	assert.Equal(t, "Bash", he.Tool)
	assert.Equal(t, "pretool-bash-blocker", he.HookName)
	assert.Contains(t, he.Reason, "text processing")
	assert.Equal(t, raw, he.Raw)
}

func TestParseHookError_DeniedWithPath(t *testing.T) {
	raw := `Hook PreToolUse:Bash denied command: "~/.claude/hooks/pretool-bash-blocker": compound commands`

	he, err := ParseHookError(raw)
	require.NoError(t, err)
	assert.Equal(t, "Bash", he.Tool)
	assert.Equal(t, "pretool-bash-blocker", he.HookName)
	assert.Equal(t, "compound commands", he.Reason)
}

func TestParseHookError_DifferentTool(t *testing.T) {
	raw := "Hook PreToolUse:Write denied this tool"

	he, err := ParseHookError(raw)
	require.NoError(t, err)
	assert.Equal(t, "Write", he.Tool)
}

func TestParseHookError_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"random text", "some random error message"},
		{"partial match", "Hook PreToolUse:"},
		{"wrong prefix", "Error PreToolUse:Bash denied this tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseHookError(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestParseHookErrors_Multiple(t *testing.T) {
	content := `Some other output
Hook PreToolUse:Bash denied this tool
more output
Hook PreToolUse:Bash blocking error from command: "~/.claude/hooks/pretool-bash-blocker": compound commands
trailing text`

	errors := ParseHookErrors(content)
	require.Len(t, errors, 2)
	assert.Equal(t, "Bash", errors[0].Tool)
	assert.Equal(t, "Bash", errors[1].Tool)
	assert.Equal(t, "compound commands", errors[1].Reason)
}

func TestParseHookErrors_NoMatches(t *testing.T) {
	content := "no hook errors here\njust regular output"
	errors := ParseHookErrors(content)
	assert.Empty(t, errors)
}

func TestParseHookErrors_EmptyContent(t *testing.T) {
	errors := ParseHookErrors("")
	assert.Empty(t, errors)
}

func TestHookError_MatchPattern_NilDetector(t *testing.T) {
	he := &HookError{
		Tool:   "Bash",
		Reason: "text processing",
		Raw:    "test",
	}

	result := he.MatchPattern(nil)
	assert.Nil(t, result)
	assert.Empty(t, he.PatternID)
}

func TestHookError_MatchPattern_EmptyReason(t *testing.T) {
	db := &enforcement.PatternDatabase{
		Patterns: []enforcement.Pattern{
			{ID: "test-pattern", Regex: "test", Reason: "test", Severity: "high"},
		},
	}
	detector, err := enforcement.NewDetector(db)
	require.NoError(t, err)

	he := &HookError{
		Tool:   "Bash",
		Reason: "",
		Raw:    "test",
	}

	result := he.MatchPattern(detector)
	assert.Nil(t, result)
}

func TestHookError_MatchPattern_WithMatch(t *testing.T) {
	db := &enforcement.PatternDatabase{
		Patterns: []enforcement.Pattern{
			{ID: "text-processing", Regex: `text processing`, Reason: "use tools", Severity: "high"},
		},
	}
	detector, err := enforcement.NewDetector(db)
	require.NoError(t, err)

	he := &HookError{
		Tool:   "Bash",
		Reason: "text processing (ls/grep/sed)",
		Raw:    "test",
	}

	result := he.MatchPattern(detector)
	require.NotNil(t, result)
	assert.Equal(t, "text-processing", result.ID)
	assert.Equal(t, "text-processing", he.PatternID)
}

func TestHookError_MatchPattern_NoMatch(t *testing.T) {
	db := &enforcement.PatternDatabase{
		Patterns: []enforcement.Pattern{
			{ID: "cd-command", Regex: `\bcd\b`, Reason: "no cd", Severity: "high"},
		},
	}
	detector, err := enforcement.NewDetector(db)
	require.NoError(t, err)

	he := &HookError{
		Tool:   "Bash",
		Reason: "text processing (ls/grep/sed)",
		Raw:    "test",
	}

	result := he.MatchPattern(detector)
	assert.Nil(t, result)
	assert.Empty(t, he.PatternID)
}

func TestExtractHookName(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"~/.claude/hooks/pretool-bash-blocker", "pretool-bash-blocker"},
		{"~/.claude/hooks/pretool-bash-blocker", "pretool-bash-blocker"},
		{"pretool-bash-blocker", "pretool-bash-blocker"},
		{"hooks/my-hook", "my-hook"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractHookName(tt.path))
		})
	}
}
