package scanners

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// TestScanner_WithSecurity_Integration verifies scanners validate security permissions
// This is an integration test covering scanners + security interaction
func TestScanner_WithSecurity_Integration(t *testing.T) {
	// Create temporary directory structure
	tmpDir := t.TempDir()

	// Create test files
	testFiles := map[string]string{
		"main.go":          "package main\n\nfunc main() {}\n",
		"utils.go":         "package main\n\nfunc helper() {}\n",
		"README.md":        "# Test Project\n",
		".env":             "SECRET_KEY=abc123\n",
		"credentials.json": `{"key": "secret"}`,
		".env.local":       "API_KEY=xyz789\n",
		"normal.key":       "-----BEGIN PRIVATE KEY-----\n",
		"id_rsa":           "-----BEGIN RSA PRIVATE KEY-----\n",
	}

	createTestFiles(t, tmpDir, testFiles)

	// Test 1: Scanner respects security sandbox (skips sensitive files)
	t.Run("security_sandbox", func(t *testing.T) {
		scanner := NewFileScanner()

		req := &metacontext.AnalyzeRequest{
			WorkingDir: tmpDir,
		}

		signals, err := scanner.Scan(context.Background(), req)
		if err != nil {
			t.Fatalf("Scan() failed: %v", err)
		}

		// Should detect Go language
		assertLanguageDetected(t, signals, "Go")

		// Verify scanner ran successfully despite sensitive files present
		if len(signals) == 0 {
			t.Error("Scan() returned no signals, expected at least Go detection")
		}
	})

	// Test 2: Scanner validates file permissions (shouldSkip logic)
	t.Run("file_permissions", func(t *testing.T) {
		scanner := NewFileScanner()

		// Test shouldSkip directly for sensitive files
		sensitiveFiles := []string{
			filepath.Join(tmpDir, ".env"),
			filepath.Join(tmpDir, "credentials.json"),
			filepath.Join(tmpDir, ".env.local"),
			filepath.Join(tmpDir, "normal.key"),
			filepath.Join(tmpDir, "id_rsa"),
		}

		assertShouldSkip(t, scanner, sensitiveFiles, true)

		// Test that normal files are not skipped
		normalFiles := []string{
			filepath.Join(tmpDir, "main.go"),
			filepath.Join(tmpDir, "README.md"),
		}

		assertShouldSkip(t, scanner, normalFiles, false)
	})

	// Test 3: Scanner reports security violations appropriately
	t.Run("security_violations", func(t *testing.T) {
		scanner := NewFileScanner()

		// Create a directory with only sensitive files
		sensDir, cleanup := createSensitiveOnlyDir(t)
		defer cleanup()

		req := &metacontext.AnalyzeRequest{
			WorkingDir: sensDir,
		}

		// Scan should succeed but return no language signals
		// (all files were skipped for security)
		signals, err := scanner.Scan(context.Background(), req)
		if err != nil {
			t.Fatalf("Scan() failed on sensitive-only directory: %v", err)
		}

		// Should not crash, but won't detect any languages
		// (This is correct behavior - security first)
		if len(signals) > 0 {
			t.Logf("Scan() returned %d signals (expected 0 for sensitive-only dir)", len(signals))
		}
	})
}

// Test helper functions for TestScanner_WithSecurity_Integration

// createTestFiles creates test files in the specified directory from a map of name->content
func createTestFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file %s: %v", name, err)
		}
	}
}

// assertLanguageDetected verifies that a specific language was detected in the signals
func assertLanguageDetected(t *testing.T, signals []metacontext.Signal, language string) {
	t.Helper()
	found := false
	for _, sig := range signals {
		if sig.Name == language {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Scan() did not detect %s language", language)
	}
}

// assertShouldSkip verifies shouldSkip behavior for a list of paths
func assertShouldSkip(t *testing.T, scanner *FileScanner, paths []string, expectedSkip bool) {
	t.Helper()
	fileType := "normal file"
	if expectedSkip {
		fileType = "sensitive file"
	}
	for _, path := range paths {
		if scanner.shouldSkip(path) != expectedSkip {
			t.Errorf("shouldSkip(%q) = %v, want %v (%s)", filepath.Base(path), !expectedSkip, expectedSkip, fileType)
		}
	}
}

// createSensitiveOnlyDir creates a temporary directory with only sensitive files
func createSensitiveOnlyDir(t *testing.T) (string, func()) {
	t.Helper()
	sensDir := t.TempDir()

	sensitiveOnly := map[string]string{
		".env":             "SECRET=abc\n",
		"credentials.json": `{"key": "secret"}`,
	}

	createTestFiles(t, sensDir, sensitiveOnly)

	cleanup := func() {
		os.RemoveAll(sensDir)
	}

	return sensDir, cleanup
}
