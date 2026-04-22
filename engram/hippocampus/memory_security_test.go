package hippocampus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMemoryPath_Safe(t *testing.T) {
	base := "/home/user/teams/project"
	path := "/home/user/teams/project/memory/notes.md"

	if err := ValidateMemoryPath(path, base); err != nil {
		t.Errorf("valid path rejected: %v", err)
	}
}

func TestValidateMemoryPath_TraversalDotDot(t *testing.T) {
	base := "/home/user/teams/project"
	path := "/home/user/teams/project/memory/../../etc/passwd"

	err := ValidateMemoryPath(path, base)
	if err == nil {
		t.Error("expected path traversal to be rejected")
	}
}

func TestValidateMemoryPath_TraversalRelative(t *testing.T) {
	base := "/tmp/test-base"
	path := "/tmp/other/../test-base/../secret"

	err := ValidateMemoryPath(path, base)
	if err == nil {
		t.Error("expected relative traversal to be rejected")
	}
}

func TestValidateMemoryPath_ExactBase(t *testing.T) {
	base := "/home/user/teams"
	if err := ValidateMemoryPath(base, base); err != nil {
		t.Errorf("path equal to base should be allowed: %v", err)
	}
}

func TestNormalizeUnicode(t *testing.T) {
	// NFC: é as single codepoint (U+00E9)
	// NFD: e (U+0065) + combining acute accent (U+0301)
	nfc := "caf\u00e9"
	nfd := "cafe\u0301"

	if NormalizeUnicode(nfc) != NormalizeUnicode(nfd) {
		t.Error("NFC normalization should make equivalent forms identical")
	}
}

func TestNormalizeUnicode_ASCII(t *testing.T) {
	ascii := "simple-file.md"
	if NormalizeUnicode(ascii) != ascii {
		t.Error("ASCII string should be unchanged by NFC normalization")
	}
}

func TestDetectHomoglyphs_Clean(t *testing.T) {
	if err := DetectHomoglyphs("my-memory-file.md"); err != nil {
		t.Errorf("pure latin rejected: %v", err)
	}
}

func TestDetectHomoglyphs_MixedLatinCyrillic(t *testing.T) {
	// Mix Latin 'a' with Cyrillic 'а' (U+0430)
	mixed := "p\u0430ssword.md"

	err := DetectHomoglyphs(mixed)
	if err == nil {
		t.Error("expected mixed Latin+Cyrillic to be rejected")
	}
}

func TestDetectHomoglyphs_PureCyrillic(t *testing.T) {
	cyrillic := "\u043f\u0440\u0438\u0432\u0435\u0442.md" // привет
	if err := DetectHomoglyphs(cyrillic); err != nil {
		t.Errorf("pure Cyrillic should be allowed: %v", err)
	}
}

func TestDetectSymlinkEscape(t *testing.T) {
	// Create temp directory structure
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "teams")
	outsideDir := filepath.Join(dir, "secrets")

	os.MkdirAll(baseDir, 0o755)
	os.MkdirAll(outsideDir, 0o755)

	// Create a file outside
	secretFile := filepath.Join(outsideDir, "secret.md")
	os.WriteFile(secretFile, []byte("secret"), 0o644)

	// Create symlink inside teams/ pointing outside
	symlinkPath := filepath.Join(baseDir, "evil.md")
	if err := os.Symlink(secretFile, symlinkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	err := DetectSymlinkEscape(symlinkPath, baseDir)
	if err == nil {
		t.Error("expected symlink escape to be rejected")
	}
}

func TestDetectSymlinkEscape_SafeSymlink(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "teams")
	subDir := filepath.Join(baseDir, "project")

	os.MkdirAll(subDir, 0o755)

	// Create file inside base
	target := filepath.Join(subDir, "real.md")
	os.WriteFile(target, []byte("content"), 0o644)

	// Symlink within base
	link := filepath.Join(baseDir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	if err := DetectSymlinkEscape(link, baseDir); err != nil {
		t.Errorf("safe symlink rejected: %v", err)
	}
}

func TestDetectSymlinkEscape_NonExistent(t *testing.T) {
	if err := DetectSymlinkEscape("/nonexistent/path", "/tmp"); err != nil {
		t.Errorf("non-existent path should not error: %v", err)
	}
}

func TestValidateMemoryFile_Full(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "memory")
	os.MkdirAll(baseDir, 0o755)

	// Create a valid file
	validFile := filepath.Join(baseDir, "notes.md")
	os.WriteFile(validFile, []byte("content"), 0o644)

	normalized, err := ValidateMemoryFile(validFile, baseDir)
	if err != nil {
		t.Errorf("valid file rejected: %v", err)
	}
	if normalized == "" {
		t.Error("expected non-empty normalized path")
	}
}

func TestValidateMemoryFile_TraversalRejected(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "memory")
	os.MkdirAll(baseDir, 0o755)

	_, err := ValidateMemoryFile(filepath.Join(baseDir, "../../etc/passwd"), baseDir)
	if err == nil {
		t.Error("expected traversal to be rejected")
	}
}
