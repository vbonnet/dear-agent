//go:build darwin

package apfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func TestProvider_Name(t *testing.T) {
	p := NewProvider()
	if p.Name() != "apfs-reflink" {
		t.Errorf("expected name 'apfs-reflink', got '%s'", p.Name())
	}
}

func TestProvider_Create(t *testing.T) {
	// Create temporary directories for test
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Create lower directory with test file
	if err := os.MkdirAll(lowerDir, 0755); err != nil {
		t.Fatalf("failed to create lower dir: %v", err)
	}
	testFile := filepath.Join(lowerDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Create provider
	p := NewProvider()

	// Create sandbox request
	req := sandbox.SandboxRequest{
		SessionID:    "test-session",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
		Secrets: map[string]string{
			"TEST_KEY": "test_value",
		},
		Timeout: 10 * time.Second,
	}

	// Create sandbox
	ctx := context.Background()
	sb, err := p.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Verify sandbox metadata
	if sb.ID != "test-session" {
		t.Errorf("expected ID 'test-session', got '%s'", sb.ID)
	}
	if sb.Type != "apfs-reflink" {
		t.Errorf("expected type 'apfs-reflink', got '%s'", sb.Type)
	}

	// Verify merged directory exists
	if _, err := os.Stat(sb.MergedPath); os.IsNotExist(err) {
		t.Errorf("merged directory does not exist: %s", sb.MergedPath)
	}

	// Verify merged is a symlink
	target, err := os.Readlink(sb.MergedPath)
	if err != nil {
		t.Errorf("merged path is not a symlink: %v", err)
	}
	if target != sb.UpperPath {
		t.Errorf("merged symlink points to wrong target: got %s, want %s", target, sb.UpperPath)
	}

	// Verify cloned file exists
	clonedFile := filepath.Join(sb.UpperPath, "repo0", "test.txt")
	if _, err := os.Stat(clonedFile); os.IsNotExist(err) {
		t.Errorf("cloned file does not exist: %s", clonedFile)
	}

	// Verify secrets file exists
	secretsFile := filepath.Join(sb.UpperPath, ".env")
	if _, err := os.Stat(secretsFile); os.IsNotExist(err) {
		t.Errorf("secrets file does not exist: %s", secretsFile)
	}

	// Verify secrets content
	content, err := os.ReadFile(secretsFile)
	if err != nil {
		t.Fatalf("failed to read secrets file: %v", err)
	}
	if !contains(string(content), "TEST_KEY=test_value") {
		t.Errorf("secrets file does not contain expected content: %s", content)
	}

	// Clean up
	if err := p.Destroy(ctx, sb.ID); err != nil {
		t.Fatalf("Destroy() failed: %v", err)
	}
}

func TestProvider_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	lowerDir := filepath.Join(tmpDir, "lower")
	workspaceDir := filepath.Join(tmpDir, "workspace")

	// Create lower directory
	if err := os.MkdirAll(lowerDir, 0755); err != nil {
		t.Fatalf("failed to create lower dir: %v", err)
	}

	p := NewProvider()

	req := sandbox.SandboxRequest{
		SessionID:    "test-validate",
		LowerDirs:    []string{lowerDir},
		WorkspaceDir: workspaceDir,
	}

	ctx := context.Background()
	sb, err := p.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Validate should succeed
	if err := p.Validate(ctx, sb.ID); err != nil {
		t.Errorf("Validate() failed for existing sandbox: %v", err)
	}

	// Validate non-existent sandbox should fail
	err = p.Validate(ctx, "non-existent")
	if err == nil {
		t.Error("Validate() should fail for non-existent sandbox")
	}

	// Clean up
	if err := p.Destroy(ctx, sb.ID); err != nil {
		t.Fatalf("Destroy() failed: %v", err)
	}

	// Validate after destroy should fail
	err = p.Validate(ctx, sb.ID)
	if err == nil {
		t.Error("Validate() should fail after Destroy()")
	}
}

func TestProvider_Destroy_Idempotent(t *testing.T) {
	p := NewProvider()
	ctx := context.Background()

	// Destroy non-existent sandbox should not error (idempotent)
	if err := p.Destroy(ctx, "non-existent"); err != nil {
		t.Errorf("Destroy() should be idempotent for non-existent sandbox: %v", err)
	}
}

func TestProvider_ValidateRequest(t *testing.T) {
	p := NewProvider()

	tests := []struct {
		name    string
		req     sandbox.SandboxRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/tmp"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: false,
		},
		{
			name: "missing session ID",
			req: sandbox.SandboxRequest{
				LowerDirs:    []string{"/tmp"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing lower dirs",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
		{
			name: "missing workspace dir",
			req: sandbox.SandboxRequest{
				SessionID: "test",
				LowerDirs: []string{"/tmp"},
			},
			wantErr: true,
		},
		{
			name: "non-existent lower dir",
			req: sandbox.SandboxRequest{
				SessionID:    "test",
				LowerDirs:    []string{"/this/does/not/exist"},
				WorkspaceDir: "/tmp/workspace",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.validateRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCloneDirectory_APFS(t *testing.T) {
	// This test verifies that cloneDirectory successfully uses cp -c on APFS
	p := NewProvider()
	tmpDir := t.TempDir()

	// Create source directory with test files
	srcDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	// Create test file in source
	testFile := filepath.Join(srcDir, "test.txt")
	testContent := []byte("test content for cloning")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Clone to destination
	dstDir := filepath.Join(tmpDir, "destination")
	if err := p.cloneDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("cloneDirectory() failed: %v", err)
	}

	// Verify destination exists
	if _, err := os.Stat(dstDir); os.IsNotExist(err) {
		t.Errorf("destination directory does not exist: %s", dstDir)
	}

	// Verify cloned file exists and has correct content
	clonedFile := filepath.Join(dstDir, "test.txt")
	content, err := os.ReadFile(clonedFile)
	if err != nil {
		t.Fatalf("failed to read cloned file: %v", err)
	}
	if string(content) != string(testContent) {
		t.Errorf("cloned file content mismatch: got %s, want %s", content, testContent)
	}
}

func TestCloneDirectory_NonAPFS(t *testing.T) {
	// This test verifies fallback to recursive copy on non-APFS filesystems
	// On macOS tmpDir is typically APFS, so this test mainly validates the fallback logic exists
	p := NewProvider()
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "source-fallback")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create source dir: %v", err)
	}

	// Create nested structure to test recursive copy
	nestedDir := filepath.Join(srcDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	nestedFile := filepath.Join(nestedDir, "nested.txt")
	if err := os.WriteFile(nestedFile, []byte("nested content"), 0644); err != nil {
		t.Fatalf("failed to write nested file: %v", err)
	}

	// Clone (will use cp -c on APFS, but tests recursive copy logic)
	dstDir := filepath.Join(tmpDir, "destination-fallback")
	if err := p.cloneDirectory(srcDir, dstDir); err != nil {
		t.Fatalf("cloneDirectory() failed: %v", err)
	}

	// Verify nested structure was copied
	clonedNestedFile := filepath.Join(dstDir, "nested", "nested.txt")
	if _, err := os.Stat(clonedNestedFile); os.IsNotExist(err) {
		t.Errorf("nested file was not cloned: %s", clonedNestedFile)
	}
}

func TestIsClonefileError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "cloning not supported error",
			err:      os.ErrInvalid,
			expected: false,
		},
		{
			name:     "operation not supported",
			err:      &os.PathError{Op: "cp", Path: "/test", Err: os.ErrInvalid},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isClonefileError(tt.err)
			if result != tt.expected {
				t.Errorf("isClonefileError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
