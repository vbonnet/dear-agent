package gemini

import (
	"fmt"
	"strings"
	"testing"
)

func TestUserError(t *testing.T) {
	tests := []struct {
		name    string
		err     *UserError
		want    string
	}{
		{
			name: "with usage",
			err: &UserError{
				Message: "invalid input",
				Usage:   "Usage: command <arg>",
			},
			want: "invalid input\n\nUsage: command <arg>",
		},
		{
			name: "without usage",
			err: &UserError{
				Message: "invalid input",
			},
			want: "invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("UserError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "with cause",
			err: &APIError{
				Message: "API call failed",
				Cause:   fmt.Errorf("network error"),
			},
			want: "API call failed: network error",
		},
		{
			name: "without cause",
			err: &APIError{
				Message: "API call failed",
			},
			want: "API call failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if !strings.Contains(got, "API call failed") {
				t.Errorf("APIError.Error() = %q, want to contain %q", got, "API call failed")
			}
		})
	}
}
