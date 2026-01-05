package errors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserError(t *testing.T) {
	err := NewUserError("Invalid Input", "Session name is invalid", []string{
		"Use only alphanumeric characters",
		"Example: my-test-session",
	})

	assert.NotNil(t, err)
	assert.Equal(t, "Invalid Input", err.Title)
	assert.Equal(t, "Session name is invalid", err.Message)
	assert.Equal(t, "Session name is invalid", err.Error())
	assert.Len(t, err.Solutions, 2)
}

func TestSystemError(t *testing.T) {
	cause := errors.New("tmux command failed")
	err := NewSystemError("Failed to create session", cause, []string{
		"Check if tmux is installed",
		"Verify tmux is in PATH",
	})

	assert.NotNil(t, err)
	assert.Equal(t, "Failed to create session", err.Title)
	assert.Equal(t, cause, err.Cause)
	assert.Contains(t, err.Error(), "Failed to create session")
	assert.Contains(t, err.Error(), "tmux command failed")
	assert.Len(t, err.Solutions, 2)
}

func TestSystemError_NoCause(t *testing.T) {
	err := NewSystemError("Unknown error", nil, nil)

	assert.NotNil(t, err)
	assert.Equal(t, "Unknown error", err.Error())
}

func TestTimeoutError(t *testing.T) {
	cause := errors.New("context deadline exceeded")
	err := NewTimeoutError("Claude startup timeout", cause, []string{
		"Increase --startup-timeout value",
		"Check if Claude is responding",
	})

	assert.NotNil(t, err)
	assert.Equal(t, "Claude startup timeout", err.Title)
	assert.Equal(t, cause, err.Cause)
	assert.Contains(t, err.Error(), "Claude startup timeout")
	assert.Contains(t, err.Error(), "context deadline exceeded")
	assert.Len(t, err.Solutions, 2)
}

func TestTimeoutError_NoCause(t *testing.T) {
	err := NewTimeoutError("Timeout", nil, nil)

	assert.NotNil(t, err)
	assert.Equal(t, "Timeout", err.Error())
}

func TestExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "nil error returns 0",
			err:      nil,
			expected: 0,
		},
		{
			name:     "UserError returns 3",
			err:      NewUserError("test", "test", nil),
			expected: 3,
		},
		{
			name:     "SystemError returns 1",
			err:      NewSystemError("test", nil, nil),
			expected: 1,
		},
		{
			name:     "TimeoutError returns 2",
			err:      NewTimeoutError("test", nil, nil),
			expected: 2,
		},
		{
			name:     "standard error returns 1",
			err:      errors.New("standard error"),
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := ExitCode(tt.err)
			assert.Equal(t, tt.expected, code)
		})
	}
}

func TestErrorInterfaces(t *testing.T) {
	// Verify all error types implement error interface
	var _ error = &UserError{}
	var _ error = &SystemError{}
	var _ error = &TimeoutError{}
}
