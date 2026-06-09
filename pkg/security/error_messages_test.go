package security

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These tests cover the human-readable Error() strings on the typed errors and
// the "allowed base is not a directory" guards — pure-logic paths that the
// existing suite asserts unwrapping for but never the rendered message. (ce-6as.44)

func TestErrorMessages(t *testing.T) {
	t.Run("PathTraversalError", func(t *testing.T) {
		err := &PathTraversalError{Path: "/etc/passwd", AllowedBase: "/safe"}
		msg := err.Error()
		if !strings.Contains(msg, "/etc/passwd") || !strings.Contains(msg, "/safe") {
			t.Errorf("message missing path or base: %q", msg)
		}
		if !strings.Contains(msg, "escapes") {
			t.Errorf("expected 'escapes' in message: %q", msg)
		}
	})

	t.Run("SymlinkError", func(t *testing.T) {
		err := &SymlinkError{Path: "/tmp/link"}
		msg := err.Error()
		if !strings.Contains(msg, "/tmp/link") || !strings.Contains(msg, "symlink") {
			t.Errorf("unexpected symlink message: %q", msg)
		}
	})

	t.Run("FileSizeError", func(t *testing.T) {
		err := &FileSizeError{Size: 2048, MaxSize: 1024}
		msg := err.Error()
		if !strings.Contains(msg, "2048") || !strings.Contains(msg, "1024") {
			t.Errorf("expected sizes in message: %q", msg)
		}
	})

	t.Run("InvalidExtensionError", func(t *testing.T) {
		err := &InvalidExtensionError{
			Extension: ".exe",
			Allowed:   map[string]bool{".svg": true},
		}
		msg := err.Error()
		if !strings.Contains(msg, ".exe") || !strings.Contains(msg, ".svg") {
			t.Errorf("expected extension and allowed list in message: %q", msg)
		}
	})
}

func TestValidatePathRejectsFileAsBase(t *testing.T) {
	tmpDir := t.TempDir()
	baseFile := filepath.Join(tmpDir, "not-a-dir.txt")
	if err := os.WriteFile(baseFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ValidatePath(filepath.Join(tmpDir, "child"), baseFile, false)
	if err == nil {
		t.Fatal("expected error when allowed base is a file")
	}
	var pathErr *PathTraversalError
	if !errors.As(err, &pathErr) {
		t.Errorf("expected PathTraversalError, got %T", err)
	}
}

func TestSafeCreateDirectoryRejectsFileAsBase(t *testing.T) {
	tmpDir := t.TempDir()
	baseFile := filepath.Join(tmpDir, "not-a-dir.txt")
	if err := os.WriteFile(baseFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := SafeCreateDirectory(filepath.Join(tmpDir, "child"), baseFile, 0o755)
	if err == nil {
		t.Fatal("expected error when allowed base is a file")
	}
	var pathErr *PathTraversalError
	if !errors.As(err, &pathErr) {
		t.Errorf("expected PathTraversalError, got %T", err)
	}
}
