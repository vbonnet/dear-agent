package resume

import (
	"bufio"
	"strings"
	"testing"
)

func TestValidateChoice_ValidInputs(t *testing.T) {
	tests := []struct {
		input string
		want  MenuChoice
	}{
		{"R", ChoiceResume},
		{"r", ChoiceResume},
		{"N", ChoiceNew},
		{"n", ChoiceNew},
		{"A", ChoiceAbort},
		{"a", ChoiceAbort},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, valid := validateChoice(tt.input)
			if !valid {
				t.Errorf("validateChoice(%q) returned invalid, want valid", tt.input)
			}
			if got != tt.want {
				t.Errorf("validateChoice(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateChoice_InvalidInputs(t *testing.T) {
	tests := []string{
		"",
		"x",
		"resume",
		"1",
		"RN",
		"  R  ",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, valid := validateChoice(input)
			if valid {
				t.Errorf("validateChoice(%q) returned valid, want invalid", input)
			}
		})
	}
}

func TestValidateChoice_CaseInsensitive(t *testing.T) {
	// Test that both upper and lower case work
	choicesUpper, validUpper := validateChoice("R")
	choicesLower, validLower := validateChoice("r")

	if !validUpper || !validLower {
		t.Error("Both R and r should be valid")
	}

	if choicesUpper != choicesLower {
		t.Errorf("R and r should return same choice, got %v and %v", choicesUpper, choicesLower)
	}
}

func TestReadUserInput_UnixNewline(t *testing.T) {
	input := "R\n"
	bufReader := bufio.NewReader(strings.NewReader(input))

	result, err := readUserInput(bufReader)
	if err != nil {
		t.Fatalf("readUserInput failed: %v", err)
	}

	if result != "R" {
		t.Errorf("Expected 'R', got %q", result)
	}
}

func TestReadUserInput_WindowsNewline(t *testing.T) {
	input := "R\r\n"
	bufReader := bufio.NewReader(strings.NewReader(input))

	result, err := readUserInput(bufReader)
	if err != nil {
		t.Fatalf("readUserInput failed: %v", err)
	}

	// TrimSpace should handle \r\n
	if result != "R" {
		t.Errorf("Expected 'R', got %q", result)
	}
}

func TestReadUserInput_EmptyLine(t *testing.T) {
	input := "\n"
	bufReader := bufio.NewReader(strings.NewReader(input))

	result, err := readUserInput(bufReader)
	if err != nil {
		t.Fatalf("readUserInput failed: %v", err)
	}

	// Empty line should return empty string after trim
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestMenuChoice_String(t *testing.T) {
	tests := []struct {
		choice MenuChoice
		want   string
	}{
		{ChoiceResume, "resume"},
		{ChoiceNew, "new"},
		{ChoiceAbort, "abort"},
		{MenuChoice(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.choice.String()
			if got != tt.want {
				t.Errorf("MenuChoice(%d).String() = %q, want %q", tt.choice, got, tt.want)
			}
		})
	}
}

func TestDirectoryState_String(t *testing.T) {
	tests := []struct {
		state DirectoryState
		want  string
	}{
		{StateEmpty, "empty"},
		{StateW0Only, "W0-only"},
		{StateStatusOnly, "STATUS-only"},
		{StateBothW0AndStatus, "W0+STATUS"},
		{StateNonResumable, "non-resumable"},
		{DirectoryState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("DirectoryState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}
