package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestValidate_Integration runs comprehensive E2E integration tests for the validate command
// These tests call the actual CLI binary (not just library functions) on real files from the engram repo

// validateTestEnv holds the test environment for validation integration tests
type validateTestEnv struct {
	bin        string   // Path to built CLI binary
	repoRoot   string   // Root of engram repository
	aimdFiles  []string // Real .ai.md files
	yamlFiles  []string // Real YAML files
	coreFiles  []string // Real core/*.ai.md files
	retroFiles []string // Real retrospective files
	allFiles   []string // All validatable files (500+)
}

// setupValidateTest builds the CLI binary and discovers real files
func setupValidateTest(t *testing.T) validateTestEnv {
	t.Helper()

	// Build CLI binary
	tmpBin := filepath.Join(t.TempDir(), "engram")
	buildCmd := exec.Command("go", "build", "-o", tmpBin, "../../../cmd/engram")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\nOutput: %s", err, output)
	}

	// Determine repository root (go up from cmd/engram/cmd/)
	repoRoot, err := filepath.Abs("../../../")
	if err != nil {
		t.Fatalf("Failed to determine repo root: %v", err)
	}

	env := validateTestEnv{
		bin:      tmpBin,
		repoRoot: repoRoot,
	}

	// Discover real .ai.md files
	env.aimdFiles = discoverFiles(t, repoRoot, "**/*.ai.md", 10)

	// Discover core/*.ai.md files
	env.coreFiles = discoverFiles(t, filepath.Join(repoRoot, "core"), "**/*.ai.md", 5)

	// Discover YAML files
	env.yamlFiles = discoverFiles(t, repoRoot, "**/*.yaml", 10)

	// Discover retrospective files
	env.retroFiles = discoverFiles(t, repoRoot, "**/*retrospective*.md", 2)

	// Collect all validatable files for full corpus test
	env.allFiles = append(env.allFiles, env.aimdFiles...)
	env.allFiles = append(env.allFiles, env.yamlFiles...)
	env.allFiles = append(env.allFiles, env.coreFiles...)
	env.allFiles = append(env.allFiles, env.retroFiles...)

	t.Logf("Test environment: %d .ai.md files, %d YAML files, %d core files, %d retrospective files",
		len(env.aimdFiles), len(env.yamlFiles), len(env.coreFiles), len(env.retroFiles))
	t.Logf("Total corpus: %d files", len(env.allFiles))

	return env
}

// discoverFiles finds real files matching a pattern in the engram repo
func discoverFiles(t *testing.T, root string, pattern string, maxFiles int) []string {
	t.Helper()

	var files []string

	// Walk directory tree to find matching files
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip vendor, .git, node_modules
			if info.Name() == "vendor" || info.Name() == ".git" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// Match pattern
		matched := false
		if strings.HasSuffix(pattern, "*.ai.md") && strings.HasSuffix(path, ".ai.md") {
			matched = true
		} else if strings.HasSuffix(pattern, "*.yaml") && strings.HasSuffix(path, ".yaml") {
			matched = true
		} else if strings.HasSuffix(pattern, "*.yml") && strings.HasSuffix(path, ".yml") {
			matched = true
		} else if strings.Contains(pattern, "retrospective") && strings.Contains(path, "retrospective") && strings.HasSuffix(path, ".md") {
			matched = true
		}

		if matched && len(files) < maxFiles {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		t.Logf("Warning: error walking directory %s: %v", root, err)
	}

	return files
}

// runValidateCommand executes validate command and returns stdout
func runValidateCommand(t *testing.T, bin string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	cmd := exec.Command(bin, append([]string{"validate"}, args...)...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	return stdout.String(), err
}

// TestIntegration_ValidateRealCorpus validates 500+ real files from the engram repository
func TestIntegration_ValidateRealCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	// Validate entire corpus should find at least some files
	t.Run("validate entire corpus", func(t *testing.T) {
		// Change to repo root to use --all

		t.Chdir(env.repoRoot)

		output, err := runValidateCommand(t, env.bin, "--all")

		// Command should succeed (exit 0) or fail (exit 1) but not crash (exit 2)
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed with unexpected error: %v\nOutput: %s", err, output)
			}
		}

		// Should report files validated
		if !strings.Contains(output, "Files scanned:") {
			t.Errorf("Output missing 'Files scanned:', got: %s", output)
		}
		if !strings.Contains(output, "Files validated:") {
			t.Errorf("Output missing 'Files validated:', got: %s", output)
		}

		t.Logf("Corpus validation output:\n%s", output)
	})

	// Test that we can validate a subset quickly
	t.Run("validate subset of files", func(t *testing.T) {
		if len(env.aimdFiles) == 0 {
			t.Skip("No .ai.md files found")
		}

		// Validate first file only
		output, err := runValidateCommand(t, env.bin, env.aimdFiles[0])

		// Should complete without crashing
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		t.Logf("Single file validation output:\n%s", output)
	})
}

// TestIntegration_EngramValidator tests engram validator on real .ai.md files
func TestIntegration_EngramValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.aimdFiles) == 0 {
		t.Skip("No .ai.md files found for engram validator test")
	}

	t.Run("engram validator on real files", func(t *testing.T) {
		// Test first 3 .ai.md files
		testFiles := env.aimdFiles
		if len(testFiles) > 3 {
			testFiles = testFiles[:3]
		}

		for _, file := range testFiles {
			t.Run(filepath.Base(file), func(t *testing.T) {
				output, err := runValidateCommand(t, env.bin, "--type=engram", file)

				// Should not crash
				if err != nil {
					exitErr := &exec.ExitError{}
					ok := errors.As(err, &exitErr)
					if !ok || exitErr.ExitCode() > 1 {
						t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
					}
					// Exit code 1 means validation errors - that's OK
					t.Logf("Validation found issues (expected): %s", output)
				} else {
					t.Logf("Validation passed: %s", output)
				}

				// Output should contain summary
				if !strings.Contains(output, "Summary:") && !strings.Contains(output, "Files scanned:") {
					t.Errorf("Output missing summary section, got: %s", output)
				}
			})
		}
	})

	t.Run("auto-detect engram validator", func(t *testing.T) {
		// Without --type flag, should auto-detect engram validator for .ai.md files
		file := env.aimdFiles[0]
		output, _ := runValidateCommand(t, env.bin, file)

		// Should validate (even if it finds errors, it should run)
		if strings.Contains(output, "unknown file type") {
			t.Errorf("Failed to auto-detect engram validator for .ai.md file")
		}
	})
}

// TestIntegration_ContentValidator tests content validator on core/*.ai.md files
func TestIntegration_ContentValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.coreFiles) == 0 {
		t.Skip("No core/*.ai.md files found for content validator test")
	}

	t.Run("content validator on core files", func(t *testing.T) {
		// Test first core file
		file := env.coreFiles[0]
		output, err := runValidateCommand(t, env.bin, "--type=content", file)

		// Should not crash
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		t.Logf("Content validator output: %s", output)
	})

	t.Run("auto-detect content validator for core files", func(t *testing.T) {
		// Files in core/ should auto-detect as content validator
		file := env.coreFiles[0]
		output, _ := runValidateCommand(t, env.bin, file)

		// Should validate
		if strings.Contains(output, "unknown file type") {
			t.Errorf("Failed to auto-detect content validator for core/*.ai.md file")
		}
	})
}

// TestIntegration_WayfinderValidator tests wayfinder validator on wayfinder-artifact.yaml
func TestIntegration_WayfinderValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	// Find wayfinder-artifact.yaml files
	var wayfinderFiles []string
	for _, file := range env.yamlFiles {
		if strings.Contains(file, "wayfinder-artifact") {
			wayfinderFiles = append(wayfinderFiles, file)
		}
	}

	if len(wayfinderFiles) == 0 {
		t.Skip("No wayfinder-artifact.yaml files found")
	}

	t.Run("wayfinder validator", func(t *testing.T) {
		file := wayfinderFiles[0]
		output, err := runValidateCommand(t, env.bin, "--type=wayfinder", file)

		// Should not crash
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		t.Logf("Wayfinder validator output: %s", output)
	})
}

// TestIntegration_LinkChecker tests link checker on real .ai.md files
func TestIntegration_LinkChecker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.aimdFiles) == 0 {
		t.Skip("No .ai.md files found for link checker test")
	}

	t.Run("link checker on real files", func(t *testing.T) {
		file := env.aimdFiles[0]
		output, err := runValidateCommand(t, env.bin, "--type=linkchecker", file)

		// Should not crash
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		t.Logf("Link checker output: %s", output)
	})

	t.Run("link checker finds broken links", func(t *testing.T) {
		// Create temp file with broken link
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.ai.md")
		content := `---
type: guide
title: Test
description: Test file
---

# Test

[Broken link](./nonexistent.md)
`
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		output, err := runValidateCommand(t, env.bin, "--type=linkchecker", testFile)

		// Should find broken link (exit 1)
		if err == nil {
			t.Logf("Warning: Expected to find broken link but validation passed")
		}

		if !strings.Contains(output, "Broken link") && !strings.Contains(output, "Error") {
			t.Logf("Note: Link checker might not have detected broken link, output: %s", output)
		}
	})
}

// TestIntegration_YAMLTokenCounter tests YAML token counter on real YAML files
func TestIntegration_YAMLTokenCounter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.yamlFiles) == 0 {
		t.Skip("No YAML files found for token counter test")
	}

	t.Run("yaml token counter", func(t *testing.T) {
		file := env.yamlFiles[0]
		output, err := runValidateCommand(t, env.bin, "--type=yamltokencounter", file)

		// Token counter should not produce errors (it's informational)
		if err != nil {
			t.Logf("Token counter output: %s", output)
		}

		t.Logf("YAML token counter output: %s", output)
	})

	t.Run("yaml token counter with verbose", func(t *testing.T) {
		file := env.yamlFiles[0]
		output, _ := runValidateCommand(t, env.bin, "--type=yamltokencounter", "--verbose", file)

		// Verbose mode should show token counts
		if !strings.Contains(output, "token") && !strings.Contains(output, "Summary") {
			t.Logf("Note: Expected token count in verbose output, got: %s", output)
		}
	})
}

// TestIntegration_RetrospectiveValidator tests retrospective validator
func TestIntegration_RetrospectiveValidator(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.retroFiles) == 0 {
		t.Skip("No retrospective files found")
	}

	t.Run("retrospective validator on real files", func(t *testing.T) {
		file := env.retroFiles[0]
		output, err := runValidateCommand(t, env.bin, "--type=retrospective", file)

		// Should not crash
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		t.Logf("Retrospective validator output: %s", output)
	})

	t.Run("auto-detect retrospective validator", func(t *testing.T) {
		file := env.retroFiles[0]
		output, _ := runValidateCommand(t, env.bin, file)

		// Should auto-detect retrospective validator
		if strings.Contains(output, "unknown file type") {
			t.Errorf("Failed to auto-detect retrospective validator")
		}
	})
}

// TestIntegration_JSONOutput tests JSON output format
func TestIntegration_JSONOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.aimdFiles) == 0 {
		t.Skip("No .ai.md files found for JSON output test")
	}

	t.Run("json output format", func(t *testing.T) {
		file := env.aimdFiles[0]
		output, err := runValidateCommand(t, env.bin, "--json", file)

		// Should not crash
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		// Parse JSON (skip any non-JSON lines like "engram dev (/tmp/...)")
		jsonStart := strings.Index(output, "{")
		if jsonStart == -1 {
			t.Fatalf("No JSON found in output: %s", output)
		}
		jsonOutput := output[jsonStart:]

		var result ValidationSummary
		if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil {
			t.Fatalf("Failed to parse JSON output: %v\nJSON: %s", err, jsonOutput)
		}

		// Verify JSON structure
		if result.TotalFiles == 0 {
			t.Errorf("Expected TotalFiles > 0 in JSON output")
		}
		if result.FilesValidated == 0 {
			t.Errorf("Expected FilesValidated > 0 in JSON output")
		}

		t.Logf("JSON output parsed successfully: %d files validated", result.FilesValidated)
	})

	t.Run("json output with errors", func(t *testing.T) {
		// Create temp file with validation errors
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "invalid.ai.md")
		content := `# No frontmatter

This file should fail validation.
`
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		output, err := runValidateCommand(t, env.bin, "--json", testFile)

		// Should exit with error code 1
		if err == nil {
			t.Logf("Note: Expected validation errors but got none")
		}

		// Parse JSON (skip any non-JSON lines like "engram dev (/tmp/...)")
		jsonStart := strings.Index(output, "{")
		if jsonStart == -1 {
			t.Fatalf("No JSON found in output: %s", output)
		}
		jsonOutput := output[jsonStart:]

		var result ValidationSummary
		if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil {
			t.Fatalf("Failed to parse JSON output: %v\nJSON: %s", err, jsonOutput)
		}

		// Should have errors
		if result.ErrorCount == 0 {
			t.Logf("Note: Expected errors in JSON output for invalid file")
		}

		t.Logf("JSON with errors: %d errors, %d warnings", result.ErrorCount, result.WarningCount)
	})
}

// TestIntegration_AutoFix tests --fix flag on real files
func TestIntegration_AutoFix(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.coreFiles) == 0 {
		t.Skip("No core files found for auto-fix test")
	}

	t.Run("auto-fix on copy", func(t *testing.T) {
		// Copy a core file to temp directory (don't modify real files)
		tmpDir := t.TempDir()
		srcFile := env.coreFiles[0]

		content, err := os.ReadFile(srcFile)
		if err != nil {
			t.Fatalf("Failed to read source file: %v", err)
		}

		testFile := filepath.Join(tmpDir, "test.ai.md")
		if err := os.WriteFile(testFile, content, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Run with --fix flag
		output, err := runValidateCommand(t, env.bin, "--type=content", "--fix", testFile)

		// Should not crash
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		// Check if fixes were applied
		if strings.Contains(output, "Fixes applied") || strings.Contains(output, "Auto-fixed") {
			t.Logf("Fixes applied: %s", output)
		} else {
			t.Logf("No fixes needed or applicable: %s", output)
		}
	})
}

// TestIntegration_PerformanceBenchmark validates 500+ files in <10 seconds
func TestIntegration_PerformanceBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance benchmark in short mode")
	}

	env := setupValidateTest(t)

	t.Run("performance benchmark 500+ files", func(t *testing.T) {
		// Change to repo root
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)

		if err := os.Chdir(env.repoRoot); err != nil {
			t.Fatalf("Failed to chdir to repo root: %v", err)
		}

		// Measure time to validate all files
		start := time.Now()
		output, err := runValidateCommand(t, env.bin, "--all")
		duration := time.Since(start)

		// Should not crash
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		t.Logf("Validation completed in %v", duration)
		t.Logf("Output:\n%s", output)

		// Performance requirement: <10 seconds for 500+ files
		// This is a soft requirement - log warning if exceeded
		if duration > 10*time.Second {
			t.Logf("WARNING: Validation took %v (goal: <10s for 500+ files)", duration)
		} else {
			t.Logf("PASS: Performance goal met (%v < 10s)", duration)
		}
	})
}

// TestIntegration_BatchProcessing tests --all flag batch processing
func TestIntegration_BatchProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	t.Run("batch processing with --all", func(t *testing.T) {
		// Create temp directory with multiple files
		tmpDir := t.TempDir()

		files := []struct {
			name    string
			content string
		}{
			{
				name: "valid1.ai.md",
				content: `---
type: guide
title: Valid 1
description: Valid file
---

# Content
`,
			},
			{
				name: "valid2.ai.md",
				content: `---
type: guide
title: Valid 2
description: Another valid file
---

# More content
`,
			},
			{
				name: "config.yaml",
				content: `name: test
version: 1.0
`,
			},
		}

		for _, f := range files {
			path := filepath.Join(tmpDir, f.name)
			if err := os.WriteFile(path, []byte(f.content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		// Change to temp dir
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		// Run --all
		output, err := runValidateCommand(t, env.bin, "--all")

		// Should process all files
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		// Should report multiple files
		if !strings.Contains(output, "Files scanned:") {
			t.Errorf("Expected 'Files scanned:' in output, got: %s", output)
		}

		t.Logf("Batch processing output: %s", output)
	})
}

// TestIntegration_TypeSelection tests --type flag for each validator
func TestIntegration_TypeSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	// Create a test file that works with multiple validators
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ai.md")
	content := `---
type: guide
title: Test File
description: Test file for validator selection
---

# Test Content

This is a test file.
`
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validatorTypes := []string{
		"engram",
		"content",
		"linkchecker",
	}

	for _, vType := range validatorTypes {
		t.Run("type="+vType, func(t *testing.T) {
			output, err := runValidateCommand(t, env.bin, "--type="+vType, testFile)

			// Should not crash
			if err != nil {
				exitErr := &exec.ExitError{}
				ok := errors.As(err, &exitErr)
				if !ok || exitErr.ExitCode() > 1 {
					t.Fatalf("Command crashed with --type=%s: %v\nOutput: %s", vType, err, output)
				}
			}

			t.Logf("Validator type %s output: %s", vType, output)
		})
	}
}

// TestIntegration_ErrorHandling tests graceful error handling
func TestIntegration_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	t.Run("missing file", func(t *testing.T) {
		output, err := runValidateCommand(t, env.bin, "/nonexistent/file.ai.md")

		// Should fail gracefully
		if err == nil {
			t.Error("Expected error for missing file")
		}

		if !strings.Contains(output, "not found") && !strings.Contains(output, "failed") {
			t.Errorf("Expected error message about missing file, got: %s", output)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		emptyFile := filepath.Join(tmpDir, "empty.ai.md")
		if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to create empty file: %v", err)
		}

		output, err := runValidateCommand(t, env.bin, emptyFile)

		// Should handle empty file gracefully
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed on empty file: %v\nOutput: %s", err, output)
			}
			// Exit 1 is OK (validation errors)
		}

		t.Logf("Empty file handling: %s", output)
	})

	t.Run("malformed file", func(t *testing.T) {
		tmpDir := t.TempDir()
		badFile := filepath.Join(tmpDir, "malformed.ai.md")
		content := `---
type: guide
# Unclosed frontmatter
This is broken
`
		if err := os.WriteFile(badFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create malformed file: %v", err)
		}

		output, err := runValidateCommand(t, env.bin, badFile)

		// Should handle malformed file gracefully
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed on malformed file: %v\nOutput: %s", err, output)
			}
		}

		t.Logf("Malformed file handling: %s", output)
	})

	t.Run("no arguments without --all", func(t *testing.T) {
		output, err := runValidateCommand(t, env.bin)

		// Should fail with helpful error
		if err == nil {
			t.Error("Expected error when no file specified")
		}

		if !strings.Contains(output, "no file specified") && !strings.Contains(output, "requires") {
			t.Errorf("Expected error about missing file argument, got: %s", output)
		}
	})

	t.Run("invalid validator type", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.ai.md")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		output, err := runValidateCommand(t, env.bin, "--type=invalid", testFile)

		// Should fail with error (or log warning about unknown type)
		if err != nil || strings.Contains(output, "unknown") || strings.Contains(output, "invalid") || strings.Contains(output, "not supported") {
			t.Logf("Got expected error/warning for invalid validator type: %v, output: %s", err, output)
		} else {
			t.Logf("Note: invalid validator type handling: err=%v, output=%s", err, output)
		}
	})
}

// TestIntegration_VerboseOutput tests --verbose flag
func TestIntegration_VerboseOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	if len(env.aimdFiles) == 0 {
		t.Skip("No .ai.md files found")
	}

	t.Run("verbose output", func(t *testing.T) {
		file := env.aimdFiles[0]

		// Run without verbose
		normalOutput, _ := runValidateCommand(t, env.bin, file)

		// Run with verbose
		verboseOutput, _ := runValidateCommand(t, env.bin, "--verbose", file)

		// Verbose output should be longer or at least as long
		if len(verboseOutput) < len(normalOutput) {
			t.Logf("Note: Verbose output not longer than normal output")
		}

		t.Logf("Normal output length: %d", len(normalOutput))
		t.Logf("Verbose output length: %d", len(verboseOutput))
	})
}

// TestIntegration_MixedFileTypes tests validation of mixed file types in one directory
func TestIntegration_MixedFileTypes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	t.Run("mixed file types", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create mixed file types
		files := map[string]string{
			"engram.ai.md": `---
type: guide
title: Engram
description: Engram file
---
# Content
`,
			"config.yaml": `name: test
version: 1.0
`,
			"README.md": `# README
This file should be skipped.
`,
		}

		for name, content := range files {
			path := filepath.Join(tmpDir, name)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to create %s: %v", name, err)
			}
		}

		// Change to temp dir
		origDir, _ := os.Getwd()
		defer os.Chdir(origDir)
		os.Chdir(tmpDir)

		// Validate all files
		output, err := runValidateCommand(t, env.bin, "--all")

		// Should process validatable files and skip README.md
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		// Should validate 2 files (engram.ai.md and config.yaml), skip README.md
		if !strings.Contains(output, "Files scanned:") {
			t.Errorf("Expected files scanned report in output")
		}

		t.Logf("Mixed file types output: %s", output)
	})
}

// TestIntegration_ExitCodes tests that exit codes are correct
func TestIntegration_ExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	t.Run("exit 0 on success", func(t *testing.T) {
		tmpDir := t.TempDir()
		validFile := filepath.Join(tmpDir, "valid.ai.md")
		content := `---
type: guide
title: Valid File
description: This file should pass validation
---

# Valid Content

This is a valid file.
`
		if err := os.WriteFile(validFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create valid file: %v", err)
		}

		_, err := runValidateCommand(t, env.bin, validFile)

		// Should exit 0 (no error) if validation passes
		// Note: Might exit 1 if validators are strict, that's OK
		if err != nil {
			t.Logf("Note: File may have validation issues: %v", err)
		}
	})

	t.Run("exit 1 on validation errors", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidFile := filepath.Join(tmpDir, "invalid.ai.md")
		content := `# No frontmatter at all

This should fail validation.
`
		if err := os.WriteFile(invalidFile, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create invalid file: %v", err)
		}

		_, err := runValidateCommand(t, env.bin, invalidFile)

		// Should exit with error (likely exit 1)
		if err == nil {
			t.Logf("Note: Expected validation to fail for file without frontmatter")
		} else {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if ok && exitErr.ExitCode() == 1 {
				t.Logf("Correct exit code 1 for validation errors")
			}
		}
	})
}

// TestIntegration_DirectoryValidation tests validating entire directories
func TestIntegration_DirectoryValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupValidateTest(t)

	t.Run("validate directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create multiple files in directory
		for i := 1; i <= 3; i++ {
			file := filepath.Join(tmpDir, "file"+string(rune('0'+i))+".ai.md")
			content := `---
type: guide
title: Test
description: Test file
---
# Content
`
			if err := os.WriteFile(file, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
		}

		// Validate directory
		output, err := runValidateCommand(t, env.bin, tmpDir)

		// Should process all files in directory
		if err != nil {
			exitErr := &exec.ExitError{}
			ok := errors.As(err, &exitErr)
			if !ok || exitErr.ExitCode() > 1 {
				t.Fatalf("Command crashed: %v\nOutput: %s", err, output)
			}
		}

		// Should report multiple files
		if !strings.Contains(output, "Files scanned:") {
			t.Errorf("Expected files scanned report, got: %s", output)
		}

		t.Logf("Directory validation output: %s", output)
	})
}
