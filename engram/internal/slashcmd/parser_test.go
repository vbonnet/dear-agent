package slashcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestParseCommand_ValidFile verifies parsing of valid slash command file
func TestParseCommand_ValidFile(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	content := `---
name: test-cmd
description: Test command
argument-hint: "[file]"
allowed-tools: ["Read", "Write"]
parameters:
  - name: file
    type: string
    required: true
    description: File to process
---
This is the command body.
It can contain multiple lines.
`

	cmdFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(cmdFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cmd, err := ParseCommand(cmdFile)
	if err != nil {
		t.Fatalf("ParseCommand() error = %v", err)
	}

	if cmd.Name != "test-cmd" {
		t.Errorf("cmd.Name = %q, want %q", cmd.Name, "test-cmd")
	}

	if cmd.Description != "Test command" {
		t.Errorf("cmd.Description = %q, want %q", cmd.Description, "Test command")
	}

	if cmd.ArgumentHint != "[file]" {
		t.Errorf("cmd.ArgumentHint = %q, want %q", cmd.ArgumentHint, "[file]")
	}

	if len(cmd.AllowedTools) != 2 {
		t.Errorf("len(cmd.AllowedTools) = %d, want 2", len(cmd.AllowedTools))
	}

	if len(cmd.Parameters) != 1 {
		t.Fatalf("len(cmd.Parameters) = %d, want 1", len(cmd.Parameters))
	}

	param := cmd.Parameters[0]
	if param.Name != "file" {
		t.Errorf("param.Name = %q, want %q", param.Name, "file")
	}

	if param.Type != "string" {
		t.Errorf("param.Type = %q, want %q", param.Type, "string")
	}

	if !param.Required {
		t.Error("param.Required = false, want true")
	}

	expectedBody := "This is the command body.\nIt can contain multiple lines.\n"
	if cmd.Body != expectedBody {
		t.Errorf("cmd.Body = %q, want %q", cmd.Body, expectedBody)
	}
}

// TestParseCommand_NoFrontmatter verifies handling of files without frontmatter
func TestParseCommand_NoFrontmatter(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	content := "This is just body content without frontmatter."

	cmdFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(cmdFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cmd, err := ParseCommand(cmdFile)
	if err != nil {
		t.Fatalf("ParseCommand() error = %v", err)
	}

	if cmd.Name != "" {
		t.Errorf("cmd.Name = %q, want empty", cmd.Name)
	}

	if cmd.Body != content {
		t.Errorf("cmd.Body = %q, want %q", cmd.Body, content)
	}
}

// TestParseCommand_UnclosedFrontmatter verifies error on unclosed frontmatter
func TestParseCommand_UnclosedFrontmatter(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	content := `---
name: test-cmd
description: Missing closing delimiter
`

	cmdFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(cmdFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := ParseCommand(cmdFile)
	if err == nil {
		t.Error("ParseCommand() with unclosed frontmatter succeeded, want error")
	}
}

// TestParseCommand_InvalidYAML verifies error on invalid YAML
func TestParseCommand_InvalidYAML(t *testing.T) {
	tmpDir := testutil.SetupTempDir(t)

	content := `---
name: test-cmd
invalid: yaml: content:
---
Body
`

	cmdFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(cmdFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	_, err := ParseCommand(cmdFile)
	if err == nil {
		t.Error("ParseCommand() with invalid YAML succeeded, want error")
	}
}

// TestParseCommand_NonexistentFile verifies error on missing file
func TestParseCommand_NonexistentFile(t *testing.T) {
	_, err := ParseCommand("/tmp/nonexistent-slash-command-12345.md")
	if err == nil {
		t.Error("ParseCommand() with nonexistent file succeeded, want error")
	}
}

// TestParseFrontmatter_EmptyContent verifies handling of empty content
func TestParseFrontmatter_EmptyContent(t *testing.T) {
	cmd, body, err := parseFrontmatter([]byte(""))
	if err != nil {
		t.Fatalf("parseFrontmatter() error = %v", err)
	}

	if cmd.Name != "" {
		t.Errorf("cmd.Name = %q, want empty", cmd.Name)
	}

	if body != "" {
		t.Errorf("body = %q, want empty", body)
	}
}

// TestStartsWith verifies startsWith helper function
func TestStartsWith(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		prefix string
		want   bool
	}{
		{"exact match", "hello", "hello", true},
		{"prefix match", "hello world", "hello", true},
		{"no match", "hello", "world", false},
		{"empty prefix", "hello", "", true},
		{"empty string", "", "hello", false},
		{"prefix longer than string", "hi", "hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := startsWith(tt.s, tt.prefix)
			if got != tt.want {
				t.Errorf("startsWith(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.want)
			}
		})
	}
}

// TestIndexOf verifies indexOf helper function
func TestIndexOf(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   int
	}{
		{"found at start", "hello world", "hello", 0},
		{"found in middle", "hello world", "o w", 4},
		{"found at end", "hello world", "world", 6},
		{"not found", "hello world", "xyz", -1},
		{"empty substring", "hello", "", 0},
		{"substring longer than string", "hi", "hello", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := indexOf(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("indexOf(%q, %q) = %d, want %d", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

// TestAutocompleteProvider_StaticChoices verifies static choices
func TestAutocompleteProvider_StaticChoices(t *testing.T) {
	cmd := &SlashCommand{
		Parameters: []Parameter{
			{
				Name: "format",
				Choices: []Choice{
					{Value: "json", Description: "JSON format"},
					{Value: "yaml", Description: "YAML format"},
					{Value: "xml", Description: "XML format"},
				},
			},
		},
	}

	values, err := cmd.AutocompleteProvider("format")
	if err != nil {
		t.Fatalf("AutocompleteProvider() error = %v", err)
	}

	want := []string{"json", "yaml", "xml"}
	if len(values) != len(want) {
		t.Fatalf("len(values) = %d, want %d", len(values), len(want))
	}

	for i, v := range values {
		if v != want[i] {
			t.Errorf("values[%d] = %q, want %q", i, v, want[i])
		}
	}
}

// TestAutocompleteProvider_NotFound verifies error when parameter not found
func TestAutocompleteProvider_NotFound(t *testing.T) {
	cmd := &SlashCommand{
		Parameters: []Parameter{
			{Name: "existing"},
		},
	}

	_, err := cmd.AutocompleteProvider("nonexistent")
	if err == nil {
		t.Error("AutocompleteProvider() with nonexistent parameter succeeded, want error")
	}
}

// TestAutocompleteProvider_DynamicCommand verifies dynamic autocomplete
func TestAutocompleteProvider_DynamicCommand(t *testing.T) {
	cmd := &SlashCommand{
		Parameters: []Parameter{
			{
				Name:         "files",
				Autocomplete: "echo file1.txt\necho file2.txt\necho file3.txt",
			},
		},
	}

	values, err := cmd.AutocompleteProvider("files")
	if err != nil {
		t.Fatalf("AutocompleteProvider() error = %v", err)
	}

	// Note: This test depends on the echo command being available
	// It should return the echoed values
	if len(values) < 1 {
		t.Errorf("len(values) = %d, want at least 1", len(values))
	}
}

// TestValidateParams_RequiredPresent verifies validation passes when required params present
func TestValidateParams_RequiredPresent(t *testing.T) {
	cmd := &SlashCommand{
		Parameters: []Parameter{
			{Name: "file", Required: true},
			{Name: "format", Required: false},
		},
	}

	params := map[string]interface{}{
		"file": "test.txt",
	}

	errors := cmd.ValidateParams(params)
	if len(errors) != 0 {
		t.Errorf("ValidateParams() returned %d errors, want 0", len(errors))
	}
}

// TestValidateParams_RequiredMissing verifies validation fails when required params missing
func TestValidateParams_RequiredMissing(t *testing.T) {
	cmd := &SlashCommand{
		Parameters: []Parameter{
			{Name: "file", Required: true},
			{Name: "output", Required: true},
		},
	}

	params := map[string]interface{}{
		"file": "test.txt",
		// "output" is missing
	}

	errors := cmd.ValidateParams(params)
	if len(errors) != 1 {
		t.Errorf("ValidateParams() returned %d errors, want 1", len(errors))
	}

	if len(errors) > 0 {
		if errors[0].Error() == "" {
			t.Error("validation error message is empty")
		}
	}
}

// TestValidateParams_AllRequiredMissing verifies multiple validation errors
func TestValidateParams_AllRequiredMissing(t *testing.T) {
	cmd := &SlashCommand{
		Parameters: []Parameter{
			{Name: "file", Required: true},
			{Name: "output", Required: true},
		},
	}

	params := map[string]interface{}{}

	errors := cmd.ValidateParams(params)
	if len(errors) != 2 {
		t.Errorf("ValidateParams() returned %d errors, want 2", len(errors))
	}
}

// TestValidationError_Error verifies error message formatting
func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Param:   "file",
		Message: "file is required",
	}

	if err.Error() != "file is required" {
		t.Errorf("Error() = %q, want %q", err.Error(), "file is required")
	}
}

// TestExecuteAutocomplete_EmptyCommand verifies error on empty command
func TestExecuteAutocomplete_EmptyCommand(t *testing.T) {
	_, err := executeAutocomplete("")
	if err == nil {
		t.Error("executeAutocomplete(\"\") succeeded, want error")
	}
}

// TestExecuteAutocomplete_InvalidCommand verifies error on invalid command
func TestExecuteAutocomplete_InvalidCommand(t *testing.T) {
	_, err := executeAutocomplete("nonexistent-command-12345")
	if err == nil {
		t.Error("executeAutocomplete() with invalid command succeeded, want error")
	}
}

// TestExecuteAutocomplete_ValidCommand verifies successful execution
func TestExecuteAutocomplete_ValidCommand(t *testing.T) {
	// Use a simple command that should work on all systems
	values, err := executeAutocomplete("echo test")
	if err != nil {
		t.Fatalf("executeAutocomplete() error = %v", err)
	}

	if len(values) != 1 {
		t.Errorf("len(values) = %d, want 1", len(values))
	}

	if len(values) > 0 && values[0] != "test" {
		t.Errorf("values[0] = %q, want %q", values[0], "test")
	}
}

// TestExecuteAutocomplete_EmptyOutput verifies handling of empty output
func TestExecuteAutocomplete_EmptyOutput(t *testing.T) {
	values, err := executeAutocomplete("echo")
	if err != nil {
		t.Fatalf("executeAutocomplete() error = %v", err)
	}

	if len(values) != 0 {
		t.Errorf("len(values) = %d, want 0 for empty output", len(values))
	}
}

// TestExecuteAutocomplete_MultilineOutput verifies multiline output parsing
func TestExecuteAutocomplete_MultilineOutput(t *testing.T) {
	// Create a temp script to generate multiline output
	tmpDir := testutil.SetupTempDir(t)
	scriptPath := filepath.Join(tmpDir, "test.sh")

	script := `#!/bin/sh
echo line1
echo line2
echo line3
`
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create test script: %v", err)
	}

	values, err := executeAutocomplete(scriptPath)
	if err != nil {
		t.Fatalf("executeAutocomplete() error = %v", err)
	}

	if len(values) != 3 {
		t.Errorf("len(values) = %d, want 3", len(values))
	}

	want := []string{"line1", "line2", "line3"}
	for i, v := range values {
		if i >= len(want) {
			break
		}
		if v != want[i] {
			t.Errorf("values[%d] = %q, want %q", i, v, want[i])
		}
	}
}
