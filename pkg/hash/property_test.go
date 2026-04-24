package hash

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
)

// TestPropertyHashDeterministic verifies CalculateFileHash produces the same
// hash for identical content across multiple calls.
func TestPropertyHashDeterministic(t *testing.T) {
	f := func(content []byte) bool {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.bin")
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			t.Logf("failed to write temp file: %v", err)
			return false
		}

		hash1, err1 := CalculateFileHash(tmpFile)
		hash2, err2 := CalculateFileHash(tmpFile)

		if err1 != nil || err2 != nil {
			return false
		}
		return hash1 == hash2
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyHashPrefixed verifies all hashes start with "sha256:" prefix.
func TestPropertyHashPrefixed(t *testing.T) {
	f := func(content []byte) bool {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.bin")
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			return false
		}

		hash, err := CalculateFileHash(tmpFile)
		if err != nil {
			return false
		}
		return strings.HasPrefix(hash, "sha256:")
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyHashFixedLength verifies all SHA-256 hashes have exactly 64 hex
// characters after the prefix.
func TestPropertyHashFixedLength(t *testing.T) {
	f := func(content []byte) bool {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.bin")
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			return false
		}

		hash, err := CalculateFileHash(tmpFile)
		if err != nil {
			return false
		}
		// "sha256:" is 7 chars, SHA-256 hex is 64 chars
		return len(hash) == 7+64
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyHashMatchesDirectSHA256 verifies CalculateFileHash produces the
// same result as computing SHA-256 directly on the content.
func TestPropertyHashMatchesDirectSHA256(t *testing.T) {
	f := func(content []byte) bool {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.bin")
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			return false
		}

		hash, err := CalculateFileHash(tmpFile)
		if err != nil {
			return false
		}

		// Compute expected hash directly
		h := sha256.Sum256(content)
		expected := fmt.Sprintf("sha256:%x", h[:])
		return hash == expected
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyHashDifferentContentDifferentHash verifies that different content
// (almost always) produces different hashes. SHA-256 collisions are astronomically
// unlikely so this property should hold for all generated inputs.
func TestPropertyHashDifferentContentDifferentHash(t *testing.T) {
	f := func(a, b []byte) bool {
		// Skip if content is identical
		if string(a) == string(b) {
			return true
		}

		tmpDir := t.TempDir()
		fileA := filepath.Join(tmpDir, "a.bin")
		fileB := filepath.Join(tmpDir, "b.bin")

		if err := os.WriteFile(fileA, a, 0644); err != nil {
			return false
		}
		if err := os.WriteFile(fileB, b, 0644); err != nil {
			return false
		}

		hashA, errA := CalculateFileHash(fileA)
		hashB, errB := CalculateFileHash(fileB)

		if errA != nil || errB != nil {
			return false
		}
		return hashA != hashB
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyExpandPathTildePrefix verifies that paths starting with "~/"
// expand to an absolute path under the home directory.
func TestPropertyExpandPathTildePrefix(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	f := func(suffix string) bool {
		// Filter out strings with null bytes or path traversal that could break filepath
		if strings.ContainsAny(suffix, "\x00") {
			return true // skip
		}
		path := "~/" + suffix
		expanded, err := ExpandPath(path)
		if err != nil {
			return false
		}
		return strings.HasPrefix(expanded, home)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

// TestPropertyExpandPathAbsolutePassthrough verifies that absolute paths
// pass through ExpandPath unchanged.
func TestPropertyExpandPathAbsolutePassthrough(t *testing.T) {
	f := func(suffix string) bool {
		if strings.ContainsAny(suffix, "\x00") {
			return true // skip
		}
		path := filepath.Join(t.TempDir(), suffix)
		expanded, err := ExpandPath(path)
		if err != nil {
			return false
		}
		// filepath.Abs cleans the path, so compare cleaned versions
		cleaned := filepath.Clean(path)
		return expanded == cleaned
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}
