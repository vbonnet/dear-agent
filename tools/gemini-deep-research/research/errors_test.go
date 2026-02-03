package research

import (
	"errors"
	"strings"
	"testing"
)

func TestNewAPIKeyNotSetError(t *testing.T) {
	err := NewAPIKeyNotSetError()

	if err == nil {
		t.Fatal("NewAPIKeyNotSetError() = nil, want error")
	}

	if !errors.Is(err, ErrAPIKeyNotSet) {
		t.Errorf("error is not ErrAPIKeyNotSet")
	}

	if !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Errorf("error message = %q, want to contain GEMINI_API_KEY", err.Error())
	}
}

func TestNewInteractionNotFoundError(t *testing.T) {
	err := NewInteractionNotFoundError("test-123")

	if err == nil {
		t.Fatal("NewInteractionNotFoundError() = nil, want error")
	}

	if !errors.Is(err, ErrInteractionNotFound) {
		t.Errorf("error is not ErrInteractionNotFound")
	}

	if !strings.Contains(err.Error(), "test-123") {
		t.Errorf("error message = %q, want to contain interaction ID", err.Error())
	}
}

func TestNewEmptyOutputError(t *testing.T) {
	err := NewEmptyOutputError("test-456")

	if err == nil {
		t.Fatal("NewEmptyOutputError() = nil, want error")
	}

	if !errors.Is(err, ErrEmptyOutput) {
		t.Errorf("error is not ErrEmptyOutput")
	}

	if !strings.Contains(err.Error(), "test-456") {
		t.Errorf("error message = %q, want to contain interaction ID", err.Error())
	}
}

func TestNewMalformedResponseError(t *testing.T) {
	err := NewMalformedResponseError("test-789", "{invalid json")

	if err == nil {
		t.Fatal("NewMalformedResponseError() = nil, want error")
	}

	if !errors.Is(err, ErrMalformedResponse) {
		t.Errorf("error is not ErrMalformedResponse")
	}

	if !strings.Contains(err.Error(), "test-789") {
		t.Errorf("error message = %q, want to contain interaction ID", err.Error())
	}

	if !strings.Contains(err.Error(), "invalid json") {
		t.Errorf("error message = %q, want to contain raw response", err.Error())
	}
}

func TestNewTimeoutError(t *testing.T) {
	err := NewTimeoutError("test-timeout", 60)

	if err == nil {
		t.Fatal("NewTimeoutError() = nil, want error")
	}

	if !errors.Is(err, ErrTimeout) {
		t.Errorf("error is not ErrTimeout")
	}

	if !strings.Contains(err.Error(), "test-timeout") {
		t.Errorf("error message = %q, want to contain interaction ID", err.Error())
	}

	if !strings.Contains(err.Error(), "60") {
		t.Errorf("error message = %q, want to contain timeout minutes", err.Error())
	}
}

func TestNewAuthError(t *testing.T) {
	err := NewAuthError("Invalid credentials")

	if err == nil {
		t.Fatal("NewAuthError() = nil, want error")
	}

	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("error is not ErrAuthFailed")
	}

	if !strings.Contains(err.Error(), "Invalid credentials") {
		t.Errorf("error message = %q, want to contain API message", err.Error())
	}
}

func TestNewRateLimitError(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		wantString string
	}{
		{
			name:       "with retry after",
			retryAfter: "60s",
			wantString: "60s",
		},
		{
			name:       "without retry after",
			retryAfter: "",
			wantString: "rate limit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewRateLimitError(tt.retryAfter)

			if err == nil {
				t.Fatal("NewRateLimitError() = nil, want error")
			}

			if !errors.Is(err, ErrRateLimitExceeded) {
				t.Errorf("error is not ErrRateLimitExceeded")
			}

			if !strings.Contains(strings.ToLower(err.Error()), tt.wantString) {
				t.Errorf("error message = %q, want to contain %q", err.Error(), tt.wantString)
			}
		})
	}
}

func TestResearchError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	err := &ResearchError{
		Operation: "test operation",
		Err:       innerErr,
	}

	unwrapped := err.Unwrap()
	if unwrapped != innerErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, innerErr)
	}
}

func TestResearchError_Error(t *testing.T) {
	err := &ResearchError{
		Operation:     "perform test",
		InteractionID: "test-123",
		Err:           errors.New("test error"),
		Troubleshooting: []string{
			"Check configuration",
			"Verify permissions",
		},
	}

	msg := err.Error()

	// Check all components are present
	if !strings.Contains(msg, "perform test") {
		t.Errorf("error message missing operation")
	}

	if !strings.Contains(msg, "test-123") {
		t.Errorf("error message missing interaction ID")
	}

	if !strings.Contains(msg, "test error") {
		t.Errorf("error message missing underlying error")
	}

	if !strings.Contains(msg, "Check configuration") {
		t.Errorf("error message missing troubleshooting tips")
	}
}

func TestResearchError_ErrorWithoutInteractionID(t *testing.T) {
	err := &ResearchError{
		Operation: "test operation",
		Err:       errors.New("test error"),
	}

	msg := err.Error()

	// Should not contain "Interaction ID:" line
	if strings.Contains(msg, "Interaction ID:") {
		t.Errorf("error message should not contain interaction ID line when ID is empty")
	}
}
