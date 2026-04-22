package main

import (
	"testing"
)

func TestApproveCommand(t *testing.T) {
	// Test command exists and is registered
	if sendApproveCmd == nil {
		t.Fatal("sendApproveCmd is nil")
	}

	if sendApproveCmd.Use != "approve <session-name>" {
		t.Errorf("unexpected Use: got %q", sendApproveCmd.Use)
	}

	// Test flags are registered
	reasonFlag := sendApproveCmd.Flags().Lookup("reason")
	if reasonFlag == nil {
		t.Error("--reason flag not registered")
	}

	reasonFileFlag := sendApproveCmd.Flags().Lookup("reason-file")
	if reasonFileFlag == nil {
		t.Error("--reason-file flag not registered")
	}

	autoContinueFlag := sendApproveCmd.Flags().Lookup("auto-continue")
	if autoContinueFlag == nil {
		t.Error("--auto-continue flag not registered")
	}
}

func TestDetectYesOptionPresent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "2-option prompt with Yes",
			content: `Do you want to proceed?
   1. Yes
   2. No`,
			wantErr: false,
		},
		{
			name: "3-option prompt with Yes",
			content: `Do you want to proceed?
   1. Yes
   2. Don't ask again
   3. No`,
			wantErr: false,
		},
		{
			name: "no prompt",
			content: `Some other content
No permission prompt here`,
			wantErr: true,
		},
		{
			name: "prompt without Yes option",
			content: `Some content
   1. Accept
   2. Decline`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := splitApproveLines(tt.content)
			yesFound := false
			for _, line := range lines {
				if containsApprove(line, "1. Yes") || containsApprove(line, "1.Yes") {
					yesFound = true
					break
				}
			}

			if (yesFound == false) != tt.wantErr {
				t.Errorf("detectYesOptionPresent() error = %v, wantErr %v", !yesFound, tt.wantErr)
			}
		})
	}
}

func TestExtractApprovalPrompt(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "valid markdown with standard prompt",
			content: `# Approval Notes

## Standard Prompt (Recommended)
` + "```" + `
This is the approval reason.
It can span multiple lines.
` + "```" + `

## Other Section
More content here.`,
			want: "This is the approval reason.\nIt can span multiple lines.\n",
		},
		{
			name: "no standard prompt section",
			content: `# Some Document

Just regular content.`,
			want: "",
		},
		{
			name: "standard prompt without closing fence",
			content: `## Standard Prompt (Recommended)
` + "```" + `
Unclosed content`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractApprovalPrompt(tt.content)
			if got != tt.want {
				t.Errorf("extractApprovalPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSplitApproveLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple lines",
			input: "line1\nline2\nline3",
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "empty lines",
			input: "line1\n\nline3",
			want:  []string{"line1", "", "line3"},
		},
		{
			name:  "no newline at end",
			input: "line1\nline2",
			want:  []string{"line1", "line2"},
		},
		{
			name:  "single line",
			input: "single",
			want:  []string{"single"},
		},
		{
			name:  "empty string",
			input: "",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitApproveLines(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("splitApproveLines() length = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitApproveLines()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestContainsApprove(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{
			name:   "contains substring",
			s:      "   1. Yes",
			substr: "1. Yes",
			want:   true,
		},
		{
			name:   "does not contain substring",
			s:      "   2. No",
			substr: "1. Yes",
			want:   false,
		},
		{
			name:   "empty substring",
			s:      "test",
			substr: "",
			want:   true,
		},
		{
			name:   "empty string",
			s:      "",
			substr: "test",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsApprove(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("containsApprove() = %v, want %v", got, tt.want)
			}
		})
	}
}
