package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTokensEstimate_Integration tests the complete CLI workflow for token estimation
// testEnv holds the test environment setup
type testEnv struct {
	bin                          string
	dir                          string
	file1, file2, file3          string
	content1, content2, content3 string
}

// setupTokensTest builds CLI and creates test files
func setupTokensTest(t *testing.T) testEnv {
	// Build CLI binary
	tmpBin := filepath.Join(t.TempDir(), "engram")
	cmd := exec.Command("go", "build", "-o", tmpBin, "../../../cmd/engram")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	// Create test files
	tmpDir := t.TempDir()
	env := testEnv{
		bin:      tmpBin,
		dir:      tmpDir,
		file1:    filepath.Join(tmpDir, "test1.md"),
		file2:    filepath.Join(tmpDir, "test2.md"),
		file3:    filepath.Join(tmpDir, "test3.txt"),
		content1: "# Test File 1\n\nThis is a test engram file with some content.",
		content2: "# Test File 2\n\nAnother test file with different content.",
		content3: "Plain text file for testing.",
	}

	for path, content := range map[string]string{
		env.file1: env.content1,
		env.file2: env.content2,
		env.file3: env.content3,
	} {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	return env
}

// assertOutputContains checks that output contains all expected strings
func assertOutputContains(t *testing.T, output string, expected ...string) {
	t.Helper()
	for _, exp := range expected {
		if !strings.Contains(output, exp) {
			t.Errorf("Output missing expected string %q, got: %s", exp, output)
		}
	}
}

// runTokensCommand executes the tokens command and returns stdout
func runTokensCommand(t *testing.T, bin string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	cmd := exec.Command(bin, append([]string{"tokens", "estimate"}, args...)...)
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return stdout.String(), err
}

// runTokensCommandWithStderr executes the tokens command and returns stderr
func runTokensCommandWithStderr(t *testing.T, bin string, args ...string) (string, error) {
	t.Helper()
	var stderr bytes.Buffer
	cmd := exec.Command(bin, append([]string{"tokens", "estimate"}, args...)...)
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err
}

// parseTokensJSON parses JSON output from tokens command
func parseTokensJSON(t *testing.T, jsonStr string) TokensJSONOutput {
	t.Helper()
	var result TokensJSONOutput
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v\nOutput: %s", err, jsonStr)
	}
	return result
}

// assertJSONFileOutput verifies JSON output for a single file
func assertJSONFileOutput(t *testing.T, result TokensJSONOutput, expectedFile string, expectedCharCount int) {
	t.Helper()
	if len(result.Files) != 1 {
		t.Errorf("Expected 1 file in JSON, got %d", len(result.Files))
	}
	if result.Files[0] != expectedFile {
		t.Errorf("Expected file path %s, got %s", expectedFile, result.Files[0])
	}
	if result.CharCount != expectedCharCount {
		t.Errorf("Expected char count %d, got %d", expectedCharCount, result.CharCount)
	}
	if result.TokensChar4 != expectedCharCount/4 {
		t.Errorf("Expected tokens_char4 %d, got %d", expectedCharCount/4, result.TokensChar4)
	}
	if result.Tokenizers == nil {
		result.Tokenizers = make(map[string]int)
	}
}

// assertCostEstimate verifies cost estimate in JSON output
func assertCostEstimate(t *testing.T, result TokensJSONOutput) {
	t.Helper()
	if result.CostEstimate == nil {
		t.Error("Expected cost_estimate in JSON output")
		return
	}
	if result.CostEstimate.Tokens == 0 {
		t.Error("Expected non-zero token count in cost estimate")
	}
	if len(result.CostEstimate.Models) == 0 {
		t.Error("Expected models in cost estimate")
	}
	if _, ok := result.CostEstimate.Models["sonnet-4.5"]; !ok {
		t.Error("Expected sonnet-4.5 in cost models")
	}
}

// assertCommandError verifies command failed with expected error message
func assertCommandError(t *testing.T, err error, output, expectedMsg string) {
	t.Helper()
	if err == nil {
		t.Error("Expected error but command succeeded")
	}
	assertOutputContains(t, output, expectedMsg)
}

func TestTokensEstimate_Integration(t *testing.T) {
	env := setupTokensTest(t)

	t.Run("single file text output", func(t *testing.T) {
		output, err := runTokensCommand(t, env.bin, env.file1)
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
		assertOutputContains(t, output, "Token estimate for 1 file:", "Character count:", "char/4:")
	})

	t.Run("multiple files text output", func(t *testing.T) {
		output, err := runTokensCommand(t, env.bin, env.file1, env.file2)
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
		assertOutputContains(t, output, "Token estimate for 2 files:")
	})

	t.Run("glob pattern", func(t *testing.T) {
		pattern := filepath.Join(env.dir, "*.md")
		output, err := runTokensCommand(t, env.bin, pattern)
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
		assertOutputContains(t, output, "Token estimate for 2 files:")
	})

	t.Run("json output", func(t *testing.T) {
		output, err := runTokensCommand(t, env.bin, env.file1, "--json")
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}

		result := parseTokensJSON(t, output)
		assertJSONFileOutput(t, result, env.file1, len(env.content1))
	})

	t.Run("no files matched", func(t *testing.T) {
		pattern := filepath.Join(env.dir, "*.nonexistent")
		output, err := runTokensCommandWithStderr(t, env.bin, pattern)
		assertCommandError(t, err, output, "no files matched")
	})

	t.Run("file does not exist", func(t *testing.T) {
		nonexistent := filepath.Join(env.dir, "nonexistent.md")
		output, err := runTokensCommandWithStderr(t, env.bin, nonexistent)
		assertCommandError(t, err, output, "failed to read")
	})

	t.Run("no arguments", func(t *testing.T) {
		output, err := runTokensCommandWithStderr(t, env.bin)
		assertCommandError(t, err, output, "requires at least 1 arg")
	})

	t.Run("formatted numbers", func(t *testing.T) {
		largeFile := filepath.Join(env.dir, "large.md")
		largeContent := strings.Repeat("word ", 1000) // ~5000 chars
		if err := os.WriteFile(largeFile, []byte(largeContent), 0644); err != nil {
			t.Fatalf("Failed to create large test file: %v", err)
		}

		output, err := runTokensCommand(t, env.bin, largeFile)
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
		if !strings.Contains(output, ",") {
			t.Errorf("Expected comma-formatted numbers in output for large file, got: %s", output)
		}
	})

	t.Run("help output", func(t *testing.T) {
		output, err := runTokensCommand(t, env.bin, "--help")
		if err != nil {
			t.Fatalf("Help command failed: %v", err)
		}
		assertOutputContains(t, output,
			"Estimate token counts", "char/4", "tiktoken", "--json",
			"glob patterns", "--query", "--auto", "Retrieval mode")
	})

	t.Run("cost estimation in output", func(t *testing.T) {
		output, err := runTokensCommand(t, env.bin, env.file1)
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}
		assertOutputContains(t, output, "Estimated cost:", "Sonnet 4.5", "Haiku 3.5", "Opus 4.5")
	})

	t.Run("cost estimation in JSON", func(t *testing.T) {
		output, err := runTokensCommand(t, env.bin, env.file1, "--json")
		if err != nil {
			t.Fatalf("Command failed: %v", err)
		}

		result := parseTokensJSON(t, output)
		assertCostEstimate(t, result)
	})
}

// TestTokensCommand_Help verifies the tokens parent command help
func TestTokensCommand_Help(t *testing.T) {
	// Build the CLI binary
	tmpBin := filepath.Join(t.TempDir(), "engram")
	cmd := exec.Command("go", "build", "-o", tmpBin, "../../../cmd/engram")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	var stdout bytes.Buffer
	helpCmd := exec.Command(tmpBin, "tokens", "--help")
	helpCmd.Stdout = &stdout
	helpCmd.Stderr = os.Stderr

	if err := helpCmd.Run(); err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	output := stdout.String()
	expectedStrings := []string{
		"Commands for estimating token counts",
		"char/4",
		"tiktoken",
		"simple",
		"estimate",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("Tokens help missing expected string %q, got: %s", expected, output)
		}
	}
}
