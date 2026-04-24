package dod

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDoD(t *testing.T) {
	// Create a temporary DoD file
	tmpDir := t.TempDir()
	dodFile := filepath.Join(tmpDir, "test.dod.yaml")

	content := `files_must_exist:
  - /tmp/test.txt
tests_must_pass: true
commands_must_succeed:
  - cmd: "echo hello"
    exit_code: 0
`
	if err := os.WriteFile(dodFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	dod, err := LoadDoD(dodFile)
	if err != nil {
		t.Fatalf("LoadDoD failed: %v", err)
	}

	if len(dod.FilesMustExist) != 1 {
		t.Errorf("Expected 1 file, got %d", len(dod.FilesMustExist))
	}

	if !dod.TestsMustPass {
		t.Error("Expected TestsMustPass to be true")
	}

	if len(dod.CommandsMustSucceed) != 1 {
		t.Errorf("Expected 1 command, got %d", len(dod.CommandsMustSucceed))
	}
}

func TestCheckFilesExist(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	dod := &BeadDoD{
		FilesMustExist: []string{
			testFile,
			"/nonexistent/file.txt",
		},
	}

	results := dod.checkFilesExist()

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if !results[0].Success {
		t.Errorf("Expected first file check to succeed, got: %s", results[0].Error)
	}

	if results[1].Success {
		t.Error("Expected second file check to fail")
	}
}

func TestCheckCommandsSucceed(t *testing.T) {
	dod := &BeadDoD{
		CommandsMustSucceed: []CommandCheck{
			{Cmd: "echo hello", ExitCode: 0},
			{Cmd: "exit 1", ExitCode: 1},
			{Cmd: "exit 0", ExitCode: 1}, // This should fail (wrong exit code)
		},
	}

	results := dod.checkCommandsSucceed()

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	if !results[0].Success {
		t.Errorf("Expected first command to succeed, got: %s", results[0].Error)
	}

	if !results[1].Success {
		t.Errorf("Expected second command to succeed, got: %s", results[1].Error)
	}

	if results[2].Success {
		t.Error("Expected third command to fail (exit code mismatch)")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"~/test.txt", filepath.Join(home, "test.txt")},
		{"/absolute/path", "/absolute/path"},
	}

	for _, tt := range tests {
		result := expandPath(tt.input)
		if result != tt.expected {
			t.Errorf("expandPath(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestExecuteCommand(t *testing.T) {
	exitCode, output, err := executeCommand("echo hello", 5*1000*1000*1000) // 5 seconds
	if err != nil {
		t.Fatalf("executeCommand failed: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	if output != "hello\n" {
		t.Errorf("Expected output 'hello\\n', got %q", output)
	}
}
