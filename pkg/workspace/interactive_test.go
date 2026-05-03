package workspace

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestPromptWorkspace_ValidSelection tests successful workspace selection.
func TestPromptWorkspace_ValidSelection(t *testing.T) {
	workspaces := []Workspace{
		{Name: "ws1", Root: "/tmp/ws1", Enabled: true},
		{Name: "ws2", Root: "/tmp/ws2", Enabled: true},
		{Name: "ws3", Root: "/tmp/ws3", Enabled: true},
	}

	tests := []struct {
		name          string
		input         string
		expectedName  string
		expectedIndex int
	}{
		{"select first", "1\n", "ws1", 0},
		{"select second", "2\n", "ws2", 1},
		{"select third", "3\n", "ws3", 2},
		{"select with spaces", "  2  \n", "ws2", 1},
		{"select with newlines", "\n2\n", "ws2", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			stdout := &bytes.Buffer{}
			prompter := NewPrompterWithIO(stdin, stdout, true)

			ws, err := prompter.PromptWorkspace(workspaces)
			if err != nil {
				t.Fatalf("PromptWorkspace failed: %v", err)
			}

			if ws.Name != tt.expectedName {
				t.Errorf("expected workspace '%s', got '%s'", tt.expectedName, ws.Name)
			}

			// Verify output contains workspace list
			output := stdout.String()
			if !strings.Contains(output, "No workspace detected") {
				t.Error("output should contain prompt message")
			}
			if !strings.Contains(output, "ws1") || !strings.Contains(output, "ws2") || !strings.Contains(output, "ws3") {
				t.Error("output should list all workspaces")
			}
		})
	}
}

// TestPromptWorkspace_InvalidThenValid tests recovery from invalid input.
func TestPromptWorkspace_InvalidThenValid(t *testing.T) {
	workspaces := []Workspace{
		{Name: "ws1", Root: "/tmp/ws1", Enabled: true},
		{Name: "ws2", Root: "/tmp/ws2", Enabled: true},
	}

	tests := []struct {
		name         string
		input        string
		expectedName string
	}{
		{"invalid number then valid", "5\n2\n", "ws2"},
		{"zero then valid", "0\n1\n", "ws1"},
		{"negative then valid", "-1\n2\n", "ws2"},
		{"text then valid", "invalid\n1\n", "ws1"},
		{"empty then valid", "\n1\n", "ws1"},
		{"multiple invalid then valid", "999\n-5\nabc\n2\n", "ws2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			stdout := &bytes.Buffer{}
			prompter := NewPrompterWithIO(stdin, stdout, true)

			ws, err := prompter.PromptWorkspace(workspaces)
			if err != nil {
				t.Fatalf("PromptWorkspace failed: %v", err)
			}

			if ws.Name != tt.expectedName {
				t.Errorf("expected workspace '%s', got '%s'", tt.expectedName, ws.Name)
			}

			// Verify error message was shown
			output := stdout.String()
			if !strings.Contains(output, "Invalid selection") {
				t.Error("output should contain 'Invalid selection' message")
			}
		})
	}
}

// TestPromptWorkspace_NonTTY tests error when not in TTY mode.
func TestPromptWorkspace_NonTTY(t *testing.T) {
	workspaces := []Workspace{
		{Name: "ws1", Root: "/tmp/ws1", Enabled: true},
	}

	stdin := strings.NewReader("1\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, false) // isTTY = false

	_, err := prompter.PromptWorkspace(workspaces)
	if err == nil {
		t.Fatal("expected error in non-TTY mode, got nil")
	}

	if !strings.Contains(err.Error(), "non-interactive") {
		t.Errorf("expected 'non-interactive' in error message, got: %v", err)
	}
}

// TestPromptWorkspace_NoEnabledWorkspaces tests error when no enabled workspaces.
func TestPromptWorkspace_NoEnabledWorkspaces(t *testing.T) {
	workspaces := []Workspace{
		{Name: "ws1", Root: "/tmp/ws1", Enabled: false},
		{Name: "ws2", Root: "/tmp/ws2", Enabled: false},
	}

	stdin := strings.NewReader("1\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	_, err := prompter.PromptWorkspace(workspaces)
	if err == nil {
		t.Fatal("expected error when no enabled workspaces, got nil")
	}

	if !errors.Is(err, ErrNoEnabledWorkspaces) {
		t.Errorf("expected ErrNoEnabledWorkspaces, got: %v", err)
	}
}

// TestPromptWorkspace_OnlyEnabledWorkspacesShown tests disabled workspaces are filtered.
func TestPromptWorkspace_OnlyEnabledWorkspacesShown(t *testing.T) {
	workspaces := []Workspace{
		{Name: "enabled1", Root: "/tmp/e1", Enabled: true},
		{Name: "disabled", Root: "/tmp/d", Enabled: false},
		{Name: "enabled2", Root: "/tmp/e2", Enabled: true},
	}

	stdin := strings.NewReader("2\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	ws, err := prompter.PromptWorkspace(workspaces)
	if err != nil {
		t.Fatalf("PromptWorkspace failed: %v", err)
	}

	// Selecting "2" should give us the second ENABLED workspace
	if ws.Name != "enabled2" {
		t.Errorf("expected 'enabled2', got '%s'", ws.Name)
	}

	// Verify disabled workspace is not shown in output
	output := stdout.String()
	if strings.Contains(output, "disabled") {
		t.Error("output should not show disabled workspace")
	}
}

// TestPromptWorkspace_OutputFormat tests the format of the prompt output.
func TestPromptWorkspace_OutputFormat(t *testing.T) {
	workspaces := []Workspace{
		{Name: "personal", Root: "/tmp/test/personal", Enabled: true},
		{Name: "work", Root: "/tmp/test/work", Enabled: true},
	}

	stdin := strings.NewReader("1\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	_, err := prompter.PromptWorkspace(workspaces)
	if err != nil {
		t.Fatalf("PromptWorkspace failed: %v", err)
	}

	output := stdout.String()

	// Check for expected content
	expectedStrings := []string{
		"No workspace detected",
		"1) personal",
		"2) work",
		"/tmp/test/personal",
		"/tmp/test/work",
		"Select workspace [1-2]:",
		"Selected workspace: personal",
		"Tip:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("output missing expected string: %q\nGot output:\n%s", expected, output)
		}
	}
}

// TestPromptConfirm_Yes tests confirming with yes responses.
func TestPromptConfirm_Yes(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"lowercase y", "y\n"},
		{"uppercase Y", "Y\n"},
		{"lowercase yes", "yes\n"},
		{"uppercase YES", "YES\n"},
		{"mixed case Yes", "Yes\n"},
		{"with spaces", "  yes  \n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			stdout := &bytes.Buffer{}
			prompter := NewPrompterWithIO(stdin, stdout, true)

			result, err := prompter.PromptConfirm("Continue?")
			if err != nil {
				t.Fatalf("PromptConfirm failed: %v", err)
			}

			if !result {
				t.Error("expected true for yes response")
			}
		})
	}
}

// TestPromptConfirm_No tests declining with no responses.
func TestPromptConfirm_No(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"lowercase n", "n\n"},
		{"uppercase N", "N\n"},
		{"lowercase no", "no\n"},
		{"uppercase NO", "NO\n"},
		{"empty (default no)", "\n"},
		{"random text", "maybe\n"},
		{"number", "1\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			stdout := &bytes.Buffer{}
			prompter := NewPrompterWithIO(stdin, stdout, true)

			result, err := prompter.PromptConfirm("Continue?")
			if err != nil {
				t.Fatalf("PromptConfirm failed: %v", err)
			}

			if result {
				t.Errorf("expected false for input %q", tt.input)
			}
		})
	}
}

// TestPromptConfirm_NonTTY tests error in non-TTY mode.
func TestPromptConfirm_NonTTY(t *testing.T) {
	stdin := strings.NewReader("y\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, false) // isTTY = false

	_, err := prompter.PromptConfirm("Continue?")
	if err == nil {
		t.Fatal("expected error in non-TTY mode, got nil")
	}

	if !strings.Contains(err.Error(), "non-interactive") {
		t.Errorf("expected 'non-interactive' in error message, got: %v", err)
	}
}

// TestPromptConfirm_OutputFormat tests the prompt message format.
func TestPromptConfirm_OutputFormat(t *testing.T) {
	stdin := strings.NewReader("y\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	message := "Do you want to proceed?"
	_, err := prompter.PromptConfirm(message)
	if err != nil {
		t.Fatalf("PromptConfirm failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, message) {
		t.Errorf("output should contain message %q, got: %s", message, output)
	}
	if !strings.Contains(output, "[y/N]") {
		t.Errorf("output should contain [y/N] prompt, got: %s", output)
	}
}

// TestPromptWorkspace_ReadError tests handling of read errors.
func TestPromptWorkspace_ReadError(t *testing.T) {
	workspaces := []Workspace{
		{Name: "ws1", Root: "/tmp/ws1", Enabled: true},
	}

	// Use a reader that returns EOF immediately
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	_, err := prompter.PromptWorkspace(workspaces)
	if err == nil {
		t.Fatal("expected error when reading fails, got nil")
	}
}

// TestPromptConfirm_ReadError tests handling of read errors.
func TestPromptConfirm_ReadError(t *testing.T) {
	// Use a reader that returns EOF immediately
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	_, err := prompter.PromptConfirm("Continue?")
	if err == nil {
		t.Fatal("expected error when reading fails, got nil")
	}
}

// TestPromptWorkspace_SingleWorkspace tests selection when only one workspace available.
func TestPromptWorkspace_SingleWorkspace(t *testing.T) {
	workspaces := []Workspace{
		{Name: "only-one", Root: "/tmp/only", Enabled: true},
	}

	stdin := strings.NewReader("1\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	ws, err := prompter.PromptWorkspace(workspaces)
	if err != nil {
		t.Fatalf("PromptWorkspace failed: %v", err)
	}

	if ws.Name != "only-one" {
		t.Errorf("expected 'only-one', got '%s'", ws.Name)
	}

	// Verify prompt shows correct range
	output := stdout.String()
	if !strings.Contains(output, "[1-1]") {
		t.Error("output should show [1-1] for single workspace")
	}
}

// TestPromptWorkspace_LongPaths tests display of long workspace paths.
func TestPromptWorkspace_LongPaths(t *testing.T) {
	workspaces := []Workspace{
		{
			Name:    "very-long-workspace-name",
			Root:    "/very/long/path/to/workspace/that/extends/beyond/normal/length",
			Enabled: true,
		},
	}

	stdin := strings.NewReader("1\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	_, err := prompter.PromptWorkspace(workspaces)
	if err != nil {
		t.Fatalf("PromptWorkspace failed: %v", err)
	}

	// Just verify it doesn't crash with long paths
	output := stdout.String()
	if !strings.Contains(output, "very-long-workspace-name") {
		t.Error("output should contain workspace name")
	}
}

// TestPromptWorkspace_ManyWorkspaces tests selection with many workspaces.
func TestPromptWorkspace_ManyWorkspaces(t *testing.T) {
	workspaces := make([]Workspace, 20)
	for i := range 20 {
		workspaces[i] = Workspace{
			Name:    "ws" + string(rune('0'+i)),
			Root:    "/tmp/ws" + string(rune('0'+i)),
			Enabled: true,
		}
	}

	stdin := strings.NewReader("10\n")
	stdout := &bytes.Buffer{}
	prompter := NewPrompterWithIO(stdin, stdout, true)

	ws, err := prompter.PromptWorkspace(workspaces)
	if err != nil {
		t.Fatalf("PromptWorkspace failed: %v", err)
	}

	// Should select the 10th workspace
	if ws.Name != "ws9" { // 0-indexed
		t.Errorf("expected 'ws9', got '%s'", ws.Name)
	}

	output := stdout.String()
	if !strings.Contains(output, "[1-20]") {
		t.Error("output should show correct range for 20 workspaces")
	}
}

// TestNewPrompter tests creating a default prompter.
func TestNewPrompter(t *testing.T) {
	prompter := NewPrompter()
	if prompter == nil {
		t.Fatal("NewPrompter returned nil")
	}

	// Verify it implements the interface
	var _ = prompter
}

// TestPrompter_BoundaryConditions tests edge cases in workspace selection.
func TestPrompter_BoundaryConditions(t *testing.T) {
	workspaces := []Workspace{
		{Name: "ws1", Root: "/tmp/ws1", Enabled: true},
		{Name: "ws2", Root: "/tmp/ws2", Enabled: true},
		{Name: "ws3", Root: "/tmp/ws3", Enabled: true},
	}

	tests := []struct {
		name      string
		input     string
		shouldErr bool
	}{
		{"boundary min valid", "1\n", false},
		{"boundary max valid", "3\n", false},
		{"boundary below min", "0\n1\n", false}, // Retries with valid
		{"boundary above max", "4\n1\n", false}, // Retries with valid
		{"large number", "999\n1\n", false},     // Retries with valid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			stdout := &bytes.Buffer{}
			prompter := NewPrompterWithIO(stdin, stdout, true)

			_, err := prompter.PromptWorkspace(workspaces)
			if (err != nil) != tt.shouldErr {
				t.Errorf("error = %v, shouldErr = %v", err, tt.shouldErr)
			}
		})
	}
}

// TestPromptWorkspace_WhitespaceHandling tests various whitespace in input.
func TestPromptWorkspace_WhitespaceHandling(t *testing.T) {
	workspaces := []Workspace{
		{Name: "ws1", Root: "/tmp/ws1", Enabled: true},
		{Name: "ws2", Root: "/tmp/ws2", Enabled: true},
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"leading spaces", "  1\n", "ws1"},
		{"trailing spaces", "1  \n", "ws1"},
		{"both", "  1  \n", "ws1"},
		{"tabs", "\t1\t\n", "ws1"},
		{"mixed whitespace", " \t 2 \t \n", "ws2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.input)
			stdout := &bytes.Buffer{}
			prompter := NewPrompterWithIO(stdin, stdout, true)

			ws, err := prompter.PromptWorkspace(workspaces)
			if err != nil {
				t.Fatalf("PromptWorkspace failed: %v", err)
			}

			if ws.Name != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, ws.Name)
			}
		})
	}
}

// TestPromptConfirm_CaseSensitivity tests case handling in confirm responses.
func TestPromptConfirm_CaseSensitivity(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"y", true},
		{"Y", true},
		{"yes", true},
		{"YES", true},
		{"Yes", true},
		{"YeS", true},
		{"yEs", true},
		{"n", false},
		{"N", false},
		{"no", false},
		{"NO", false},
		{"No", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			stdin := strings.NewReader(tt.input + "\n")
			stdout := &bytes.Buffer{}
			prompter := NewPrompterWithIO(stdin, stdout, true)

			result, err := prompter.PromptConfirm("Test?")
			if err != nil {
				t.Fatalf("PromptConfirm failed: %v", err)
			}

			if result != tt.expected {
				t.Errorf("input %q: expected %v, got %v", tt.input, tt.expected, result)
			}
		})
	}
}
