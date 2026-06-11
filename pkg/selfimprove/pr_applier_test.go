package selfimprove

import (
	"testing"
)

func TestPRApplierSanitizeBranchName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: "simple-id",
			want:  "simple-id",
		},
		{
			input: "has spaces here",
			want:  "has-spaces-here",
		},
		{
			input: "special!@#chars",
			want:  "special-chars",
		},
		{
			input: "  leading and trailing  ",
			want:  "leading-and-trailing",
		},
		{
			input: "---dashes---",
			want:  "dashes",
		},
		{
			// 50 chars long — should be truncated to 40.
			input: "abcdefghij1234567890abcdefghij1234567890abcd",
			want:  "abcdefghij1234567890abcdefghij1234567890",
		},
		{
			// Truncation should not leave a trailing dash.
			input: "abcdefghij1234567890abcdefghij123456789-extra",
			want:  "abcdefghij1234567890abcdefghij123456789",
		},
		{
			input: "ce-1xi/otel-audit",
			want:  "ce-1xi-otel-audit",
		},
		{
			input: "mixed.dots.and-dashes",
			want:  "mixed-dots-and-dashes",
		},
	}

	for _, tc := range cases {
		got := sanitizeBranchName(tc.input)
		if got != tc.want {
			t.Errorf("sanitizeBranchName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestPRApplierImplementsApplier is a compile-time interface check.
var _ Applier = PRApplier{}
