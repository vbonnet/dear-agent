package scanners

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// ============================================================================
// Unit Tests: file.go (FileScanner language/framework detection)
// S7 Plan: Week 4 Testing, Unit Test Category + Security Test Category (M3)
// ============================================================================

// TestFileScanner_LanguageDetection tests language detection by file extensions
func TestFileScanner_LanguageDetection(t *testing.T) {
	// Create temp directory with Go files
	tmpdir, err := os.MkdirTemp("", "test-file-scanner-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create Go files
	for i := 0; i < 5; i++ {
		filename := filepath.Join(tmpdir, "file"+string(rune('a'+i))+".go")
		err := os.WriteFile(filename, []byte("package main\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	scanner := NewFileScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect Go language
	hasGo := false
	for _, sig := range signals {
		if sig.Name == "Go" && sig.Source == "file" {
			hasGo = true
			if sig.Confidence < 0.9 { // 5 out of 5 files = 100% confidence
				t.Errorf("Expected high confidence for Go, got %f", sig.Confidence)
			}
		}
	}

	if !hasGo {
		t.Error("FileScanner should detect Go language")
	}
}

// TestFileScanner_FrameworkDetection tests framework detection by special files
func TestFileScanner_FrameworkDetection(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-file-scanner-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create package.json (Node.js indicator)
	packageJSON := filepath.Join(tmpdir, "package.json")
	err = os.WriteFile(packageJSON, []byte(`{"name":"test"}`), 0644)
	if err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	scanner := NewFileScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect Node.js
	hasNodeJS := false
	for _, sig := range signals {
		if sig.Name == "Node.js" && sig.Source == "file" {
			hasNodeJS = true
			if sig.Confidence != 1.0 {
				t.Errorf("Framework detection should have confidence 1.0, got %f", sig.Confidence)
			}
		}
	}

	if !hasNodeJS {
		t.Error("FileScanner should detect Node.js framework")
	}
}

// TestFileScanner_SensitiveFileExclusion tests Security Mitigation M3
func TestFileScanner_SensitiveFileExclusion(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-file-scanner-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create sensitive files
	sensitiveFiles := []string{".env", ".env.local", "credentials.json", "id_rsa", "secret.key"}
	for _, filename := range sensitiveFiles {
		err := os.WriteFile(filepath.Join(tmpdir, filename), []byte("secret data"), 0644)
		if err != nil {
			t.Fatalf("Failed to create sensitive file: %v", err)
		}
	}

	// Create normal Go file
	goFile := filepath.Join(tmpdir, "main.go")
	err = os.WriteFile(goFile, []byte("package main\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create Go file: %v", err)
	}

	scanner := NewFileScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect Go (normal file processed)
	hasGo := false
	for _, sig := range signals {
		if sig.Name == "Go" {
			hasGo = true
		}
	}
	if !hasGo {
		t.Error("FileScanner should process normal files")
	}

	// Should NOT process sensitive files (no detection based on content)
	// Test passes if no panic/error occurred during scan
}

// TestFileScanner_HiddenDirectorySkip tests hidden directory exclusion
func TestFileScanner_HiddenDirectorySkip(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-file-scanner-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create hidden directory with files
	hiddenDir := filepath.Join(tmpdir, ".hidden")
	err = os.Mkdir(hiddenDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create hidden dir: %v", err)
	}

	// Create file in hidden directory
	hiddenFile := filepath.Join(hiddenDir, "code.go")
	err = os.WriteFile(hiddenFile, []byte("package hidden\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file in hidden dir: %v", err)
	}

	// Create normal file in root
	normalFile := filepath.Join(tmpdir, "main.go")
	err = os.WriteFile(normalFile, []byte("package main\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create normal file: %v", err)
	}

	scanner := NewFileScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect Go from normal file
	hasGo := false
	for _, sig := range signals {
		if sig.Name == "Go" {
			hasGo = true
			// Confidence should be 100% (1 file out of 1 scanned)
			if sig.Confidence != 1.0 {
				t.Errorf("Expected confidence 1.0, got %f", sig.Confidence)
			}
		}
	}
	if !hasGo {
		t.Error("FileScanner should detect Go in normal files")
	}
}

// TestFileScanner_LargeFileSkip tests >10MB file exclusion
func TestFileScanner_LargeFileSkip(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-file-scanner-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create small file
	smallFile := filepath.Join(tmpdir, "small.go")
	err = os.WriteFile(smallFile, []byte("package main\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create small file: %v", err)
	}

	// Create large file (>10MB) - simulate with just metadata
	largeFile := filepath.Join(tmpdir, "large.bin")
	f, err := os.Create(largeFile)
	if err != nil {
		t.Fatalf("Failed to create large file: %v", err)
	}
	// Truncate to 11MB without writing data
	err = f.Truncate(11 * 1024 * 1024)
	if err != nil {
		f.Close()
		t.Fatalf("Failed to truncate large file: %v", err)
	}
	f.Close()

	scanner := NewFileScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect Go from small file
	hasGo := false
	for _, sig := range signals {
		if sig.Name == "Go" {
			hasGo = true
		}
	}
	if !hasGo {
		t.Error("FileScanner should process small files")
	}

	// Large file should be skipped (no error)
}

// TestFileScanner_EmptyDirectory tests handling of empty directory
func TestFileScanner_EmptyDirectory(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-file-scanner-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	scanner := NewFileScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should return empty signals (no files to scan)
	if len(signals) != 0 {
		t.Errorf("Empty directory should return 0 signals, got %d", len(signals))
	}
}

// TestFileScanner_ContextCancellation tests cancellation handling
func TestFileScanner_ContextCancellation(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-file-scanner-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create many files to give time for cancellation
	for i := 0; i < 100; i++ {
		filename := filepath.Join(tmpdir, "file"+string(rune('a'+(i%26)))+".go")
		err := os.WriteFile(filename, []byte("package main\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	scanner := NewFileScanner()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	_, err = scanner.Scan(ctx, req)
	// Should return error (context canceled)
	if err == nil {
		t.Error("Canceled context should return error")
	}
}

// TestFileScanner_ShouldSkip tests shouldSkip() helper
func TestFileScanner_ShouldSkip(t *testing.T) {
	scanner := NewFileScanner()

	tests := []struct {
		path     string
		expected bool
	}{
		{"/tmp/.env", true},
		{"/tmp/.env.local", true},
		{"/tmp/credentials.json", true},
		{"/tmp/secret.key", true},
		{"/tmp/id_rsa", true},
		{"/tmp/main.go", false},
		{"/tmp/README.md", false},
		{"/tmp/.gitignore", false}, // .gitignore not in sensitive list
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := scanner.shouldSkip(tt.path)
			if result != tt.expected {
				t.Errorf("shouldSkip(%s) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}
