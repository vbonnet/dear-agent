package healthchecker

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStatusValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{name: "ok", status: StatusOK, want: true},
		{name: "info", status: StatusInfo, want: true},
		{name: "warning", status: StatusWarning, want: true},
		{name: "error", status: StatusError, want: true},
		{name: "zero value", status: "", want: false},
		{name: "arbitrary value", status: "unknown", want: false},
		{name: "case variant", status: "OK", want: false},
		{name: "whitespace variant", status: " ok ", want: false},
		{name: "alias", status: "healthy", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("Status(%q).Valid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestResultValidate(t *testing.T) {
	tests := []struct {
		name      string
		result    Result
		wantError string
	}{
		{name: "ok", result: Result{Status: StatusOK}},
		{name: "info with empty identity and inconsistent fix metadata", result: Result{Status: StatusInfo, Fixable: true}},
		{name: "warning", result: Result{Status: StatusWarning}},
		{name: "error", result: Result{Status: StatusError}},
		{name: "zero value", result: Result{}, wantError: `invalid health status: ""`},
		{name: "arbitrary value", result: Result{Status: "mystery"}, wantError: `invalid health status: "mystery"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.Validate()
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatal("Validate() error = nil, want invalid-status error")
			}
			if !errors.Is(err, ErrInvalidStatus) {
				t.Errorf("errors.Is(Validate(), ErrInvalidStatus) = false for %v", err)
			}
			if err.Error() != tt.wantError {
				t.Errorf("Validate() error = %q, want %q", err, tt.wantError)
			}
		})
	}
}

func TestNormalizeResultValidNoOp(t *testing.T) {
	fix := &Fix{
		Name:        "repair",
		Description: "preserve every field",
		Command:     "repair --now",
		Apply:       func(context.Context) error { return nil },
		Reversible:  true,
	}

	for _, status := range []Status{StatusOK, StatusInfo, StatusWarning, StatusError} {
		t.Run(string(status), func(t *testing.T) {
			input := Result{
				Name:     "check",
				Category: "core",
				Status:   status,
				Message:  "message",
				Fixable:  true,
				Fix:      fix,
			}

			got := normalizeResult(input)
			if got != input {
				t.Errorf("normalizeResult() = %#v, want exact input %#v", got, input)
			}
			if got.Fix != fix {
				t.Error("normalizeResult() did not preserve Fix pointer")
			}
		})
	}
}

func TestNormalizeResultInvalid(t *testing.T) {
	fix := &Fix{Apply: func(context.Context) error { return nil }}
	tests := []struct {
		name        string
		input       Result
		wantMessage string
	}{
		{
			name:        "zero value without message",
			input:       Result{Fixable: true, Fix: fix},
			wantMessage: `invalid health status: ""`,
		},
		{
			name: "arbitrary value preserves message",
			input: Result{
				Name:     "check",
				Category: "core",
				Status:   "mystery",
				Message:  "original diagnosis",
				Fixable:  true,
				Fix:      fix,
			},
			wantMessage: `original diagnosis; invalid health status: "mystery"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeResult(tt.input)
			if got.Status != StatusError {
				t.Errorf("normalizeResult().Status = %q, want %q", got.Status, StatusError)
			}
			if got.Message != tt.wantMessage {
				t.Errorf("normalizeResult().Message = %q, want %q", got.Message, tt.wantMessage)
			}
			if got.Name != tt.input.Name || got.Category != tt.input.Category {
				t.Errorf("normalizeResult() changed identity: got (%q, %q), want (%q, %q)",
					got.Name, got.Category, tt.input.Name, tt.input.Category)
			}
			if got.Fixable {
				t.Error("normalizeResult().Fixable = true, want false")
			}
			if got.Fix != nil {
				t.Error("normalizeResult().Fix retained executable metadata")
			}

			second := normalizeResult(got)
			if second != got {
				t.Errorf("normalizeResult() is not idempotent: second = %#v, first = %#v", second, got)
			}
			if count := strings.Count(second.Message, "invalid health status"); count != 1 {
				t.Errorf("normalization diagnostic count = %d, want 1 in %q", count, second.Message)
			}
		})
	}
}

func TestResult_IsHealthy(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "OK status is healthy",
			result: Result{Status: StatusOK},
			want:   true,
		},
		{
			name:   "Info status is healthy",
			result: Result{Status: StatusInfo},
			want:   true,
		},
		{
			name:   "Warning status is not healthy",
			result: Result{Status: StatusWarning},
			want:   false,
		},
		{
			name:   "Error status is not healthy",
			result: Result{Status: StatusError},
			want:   false,
		},
		{
			name:   "Zero status is not healthy",
			result: Result{},
			want:   false,
		},
		{
			name:   "Arbitrary status is not healthy",
			result: Result{Status: "mystery"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsHealthy()
			if got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResult_IsIssue(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "OK status is not an issue",
			result: Result{Status: StatusOK},
			want:   false,
		},
		{
			name:   "Info status is not an issue",
			result: Result{Status: StatusInfo},
			want:   false,
		},
		{
			name:   "Warning status is an issue",
			result: Result{Status: StatusWarning},
			want:   true,
		},
		{
			name:   "Error status is an issue",
			result: Result{Status: StatusError},
			want:   true,
		},
		{
			name:   "Zero status is an issue",
			result: Result{},
			want:   true,
		},
		{
			name:   "Arbitrary status is an issue",
			result: Result{Status: "mystery"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsIssue()
			if got != tt.want {
				t.Errorf("IsIssue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResult_IsCritical(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{
			name:   "OK status is not critical",
			result: Result{Status: StatusOK},
			want:   false,
		},
		{
			name:   "Info status is not critical",
			result: Result{Status: StatusInfo},
			want:   false,
		},
		{
			name:   "Warning status is not critical",
			result: Result{Status: StatusWarning},
			want:   false,
		},
		{
			name:   "Error status is critical",
			result: Result{Status: StatusError},
			want:   true,
		},
		{
			name:   "Zero status is critical",
			result: Result{},
			want:   true,
		},
		{
			name:   "Arbitrary status is critical",
			result: Result{Status: "mystery"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.IsCritical()
			if got != tt.want {
				t.Errorf("IsCritical() = %v, want %v", got, tt.want)
			}
		})
	}
}
