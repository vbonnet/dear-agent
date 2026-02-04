package cmd

import (
	"strings"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/types"
)

func TestFlagsValidate(t *testing.T) {
	tests := []struct {
		name      string
		flags     types.Flags
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid flags - empty",
			flags:     types.Flags{},
			wantError: false,
		},
		{
			name:      "Valid flags - valid type",
			flags:     types.Flags{Type: "video"},
			wantError: false,
		},
		{
			name:      "Invalid - invalid content type",
			flags:     types.Flags{Type: "invalid"},
			wantError: true,
			errorMsg:  "invalid content type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&tt.flags)
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid HTTP URL",
			url:       "http://example.com",
			wantError: false,
		},
		{
			name:      "Valid HTTPS URL",
			url:       "https://example.com/path",
			wantError: false,
		},
		{
			name:      "Valid YouTube URL",
			url:       "https://www.youtube.com/watch?v=VIDEO_ID",
			wantError: false,
		},
		{
			name:      "Valid arxiv URL",
			url:       "https://arxiv.org/abs/2601.20802",
			wantError: false,
		},
		{
			name:      "Invalid - no scheme",
			url:       "example.com",
			wantError: true,
			errorMsg:  "invalid URL",
		},
		{
			name:      "Invalid - unsupported scheme",
			url:       "ftp://example.com",
			wantError: true,
			errorMsg:  "invalid URL scheme",
		},
		{
			name:      "Invalid - empty URL",
			url:       "",
			wantError: true,
			errorMsg:  "invalid URL",
		},
		{
			name:      "Invalid - malformed URL",
			url:       "http://",
			wantError: true,
			errorMsg:  "missing host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url)
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error but got nil")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidContentTypes(t *testing.T) {
	validTypes := []string{"video", "article", "arxiv", "huggingface"}

	for _, typ := range validTypes {
		t.Run("Valid type: "+typ, func(t *testing.T) {
			flags := types.Flags{Type: typ}
			err := Validate(&flags)
			if err != nil {
				t.Errorf("Expected valid type %q to pass validation, got error: %v", typ, err)
			}
		})
	}
}
