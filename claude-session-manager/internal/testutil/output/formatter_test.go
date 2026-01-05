package output

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	testerrors "github.com/vbonnet/ai-tools/claude-session-manager/internal/testutil/errors"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		format   string
		expected interface{}
	}{
		{
			name:     "json format returns JSONFormatter",
			format:   "json",
			expected: &JSONFormatter{},
		},
		{
			name:     "text format returns TextFormatter",
			format:   "text",
			expected: &TextFormatter{},
		},
		{
			name:     "empty format defaults to TextFormatter",
			format:   "",
			expected: &TextFormatter{},
		},
		{
			name:     "unknown format defaults to TextFormatter",
			format:   "xml",
			expected: &TextFormatter{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := Format(tt.format)
			assert.IsType(t, tt.expected, formatter)
		})
	}
}

func TestJSONFormatter_Format(t *testing.T) {
	formatter := &JSONFormatter{}

	t.Run("formats simple map", func(t *testing.T) {
		data := map[string]string{
			"name":   "test-session",
			"status": "active",
		}

		output, err := formatter.Format(data)
		require.NoError(t, err)

		// Verify it's valid JSON
		var decoded map[string]string
		err = json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, err)
		assert.Equal(t, "test-session", decoded["name"])
		assert.Equal(t, "active", decoded["status"])
	})

	t.Run("formats struct", func(t *testing.T) {
		type Session struct {
			Name        string `json:"name"`
			TmuxSession string `json:"tmux_session"`
		}
		data := Session{
			Name:        "my-test",
			TmuxSession: "csm-test-my-test",
		}

		output, err := formatter.Format(data)
		require.NoError(t, err)

		var decoded Session
		err = json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, err)
		assert.Equal(t, "my-test", decoded.Name)
		assert.Equal(t, "csm-test-my-test", decoded.TmuxSession)
	})

	t.Run("formats nil as null", func(t *testing.T) {
		output, err := formatter.Format(nil)
		require.NoError(t, err)
		assert.Equal(t, "null", output)
	})
}

func TestJSONFormatter_FormatError(t *testing.T) {
	formatter := &JSONFormatter{}

	t.Run("formats UserError", func(t *testing.T) {
		err := testerrors.NewUserError(
			"Invalid Input",
			"Session name is invalid",
			[]string{"Use alphanumeric only", "Example: test-1"},
		)

		output, formatErr := formatter.FormatError(err)
		require.NoError(t, formatErr)

		var decoded map[string]interface{}
		jsonErr := json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, jsonErr)

		assert.Equal(t, "user_error", decoded["type"])
		assert.Equal(t, "Invalid Input", decoded["title"])
		assert.Equal(t, "Session name is invalid", decoded["error"])

		solutions, ok := decoded["solutions"].([]interface{})
		require.True(t, ok)
		assert.Len(t, solutions, 2)
	})

	t.Run("formats SystemError", func(t *testing.T) {
		err := testerrors.NewSystemError(
			"Failed to start tmux",
			nil,
			[]string{"Check tmux installation"},
		)

		output, formatErr := formatter.FormatError(err)
		require.NoError(t, formatErr)

		var decoded map[string]interface{}
		jsonErr := json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, jsonErr)

		assert.Equal(t, "system_error", decoded["type"])
		assert.Equal(t, "Failed to start tmux", decoded["title"])
	})

	t.Run("formats TimeoutError", func(t *testing.T) {
		err := testerrors.NewTimeoutError(
			"Claude startup timeout",
			nil,
			[]string{"Increase timeout"},
		)

		output, formatErr := formatter.FormatError(err)
		require.NoError(t, formatErr)

		var decoded map[string]interface{}
		jsonErr := json.Unmarshal([]byte(output), &decoded)
		require.NoError(t, jsonErr)

		assert.Equal(t, "timeout_error", decoded["type"])
		assert.Equal(t, "Claude startup timeout", decoded["title"])
	})
}

func TestTextFormatter_Format(t *testing.T) {
	formatter := &TextFormatter{}

	t.Run("formats simple string", func(t *testing.T) {
		output, err := formatter.Format("test output")
		require.NoError(t, err)
		assert.Equal(t, "test output", output)
	})

	t.Run("formats number", func(t *testing.T) {
		output, err := formatter.Format(42)
		require.NoError(t, err)
		assert.Equal(t, "42", output)
	})

	t.Run("formats nil", func(t *testing.T) {
		output, err := formatter.Format(nil)
		require.NoError(t, err)
		assert.Equal(t, "<nil>", output)
	})
}

func TestTextFormatter_FormatError(t *testing.T) {
	formatter := &TextFormatter{}

	t.Run("formats UserError with solutions", func(t *testing.T) {
		err := testerrors.NewUserError(
			"Invalid Input",
			"Session name is invalid",
			[]string{"Use alphanumeric only", "Example: test-1"},
		)

		output, formatErr := formatter.FormatError(err)
		require.NoError(t, formatErr)

		assert.Contains(t, output, "Error: Invalid Input")
		assert.Contains(t, output, "Session name is invalid")
		assert.Contains(t, output, "Solutions:")
		assert.Contains(t, output, "- Use alphanumeric only")
		assert.Contains(t, output, "- Example: test-1")
	})

	t.Run("formats SystemError with cause", func(t *testing.T) {
		err := testerrors.NewSystemError(
			"Failed to start tmux",
			testerrors.NewUserError("test", "underlying cause", nil),
			[]string{"Check installation"},
		)

		output, formatErr := formatter.FormatError(err)
		require.NoError(t, formatErr)

		assert.Contains(t, output, "System Error: Failed to start tmux")
		assert.Contains(t, output, "Cause:")
		assert.Contains(t, output, "underlying cause")
		assert.Contains(t, output, "Solutions:")
	})

	t.Run("formats TimeoutError", func(t *testing.T) {
		err := testerrors.NewTimeoutError(
			"Claude startup timeout",
			nil,
			nil,
		)

		output, formatErr := formatter.FormatError(err)
		require.NoError(t, formatErr)

		assert.Contains(t, output, "Timeout Error: Claude startup timeout")
	})

	t.Run("formats generic error", func(t *testing.T) {
		err := testerrors.NewSystemError("test", nil, nil)

		output, formatErr := formatter.FormatError(err)
		require.NoError(t, formatErr)

		lines := strings.Split(strings.TrimSpace(output), "\n")
		assert.Greater(t, len(lines), 0)
	})
}
