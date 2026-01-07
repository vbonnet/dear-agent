package executor

import (
	"errors"
	"testing"
)

func TestExecutionError_Error(t *testing.T) {
	cause := errors.New("underlying error")
	err := NewRecoverableError("bead-1", 2, "CSM timeout", cause)

	expectedMsg := "[RECOVERABLE] bead=bead-1 iteration=2: CSM timeout: underlying error"
	if err.Error() != expectedMsg {
		t.Errorf("error message: got %q, want %q", err.Error(), expectedMsg)
	}
}

func TestExecutionError_Types(t *testing.T) {
	tests := []struct {
		name     string
		err      *ExecutionError
		wantType ErrorType
		wantStr  string
	}{
		{"recoverable", NewRecoverableError("b1", 1, "msg", nil), ErrorRecoverable, "RECOVERABLE"},
		{"escalation", NewEscalationError("b2", 2, "msg", nil), ErrorEscalation, "ESCALATION"},
		{"fatal", NewFatalError("b3", 3, "msg", nil), ErrorFatal, "FATAL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Type != tt.wantType {
				t.Errorf("type: got %v, want %v", tt.err.Type, tt.wantType)
			}
			if tt.err.typeString() != tt.wantStr {
				t.Errorf("typeString: got %s, want %s", tt.err.typeString(), tt.wantStr)
			}
		})
	}
}

func TestExecutionError_Unwrap(t *testing.T) {
	cause := errors.New("cause")
	err := NewRecoverableError("bead-1", 1, "msg", cause)

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Unwrap: got %v, want %v", unwrapped, cause)
	}
}
