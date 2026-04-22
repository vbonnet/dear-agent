package hippocampus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ValidateMemoryPath checks that a memory file path is safe to access.
// It rejects paths containing traversal sequences (../) and paths that
// resolve outside the allowed base directory.
func ValidateMemoryPath(path, baseDir string) error {
	// Normalize both paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("resolve base: %w", err)
	}

	// Check for literal ../ in the original path (before resolution)
	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("path traversal rejected: %q contains '..'", path)
	}

	// Verify resolved path is within base directory
	if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
		return fmt.Errorf("path escapes base directory: %q is not within %q", absPath, absBase)
	}

	return nil
}

// NormalizeUnicode applies NFC normalization to a string, preventing unicode
// homoglyph attacks where visually identical characters have different byte
// representations (e.g., 'é' as single codepoint vs 'e' + combining accent).
func NormalizeUnicode(s string) string {
	return norm.NFC.String(s)
}

// DetectHomoglyphs checks if a filename contains characters from mixed scripts
// that could be used in homoglyph attacks (e.g., Cyrillic 'а' vs Latin 'a').
// Returns an error if suspicious mixed scripts are detected.
func DetectHomoglyphs(filename string) error {
	scripts := make(map[string]bool)

	// Strip the file extension before checking scripts, since extensions
	// like ".md" are always Latin and would cause false positives.
	nameOnly := strings.TrimSuffix(filename, filepath.Ext(filename))

	for _, r := range nameOnly {
		if !unicode.IsLetter(r) {
			continue
		}
		switch {
		case unicode.Is(unicode.Latin, r):
			scripts["Latin"] = true
		case unicode.Is(unicode.Cyrillic, r):
			scripts["Cyrillic"] = true
		case unicode.Is(unicode.Greek, r):
			scripts["Greek"] = true
		case unicode.Is(unicode.Han, r):
			scripts["Han"] = true
		case unicode.Is(unicode.Hangul, r):
			scripts["Hangul"] = true
		}
	}

	// Flag if multiple scripts with known homoglyph overlap are present
	suspicious := 0
	for script := range scripts {
		if script == "Latin" || script == "Cyrillic" || script == "Greek" {
			suspicious++
		}
	}
	if suspicious > 1 {
		var found []string
		for s := range scripts {
			found = append(found, s)
		}
		return fmt.Errorf("mixed scripts detected in %q: %v — possible homoglyph attack", filename, found)
	}

	return nil
}

// DetectSymlinkEscape checks if a path is a symlink that resolves to a
// location outside the allowed base directory. This prevents shared memory
// files from escaping the teams/ directory structure.
func DetectSymlinkEscape(path, baseDir string) error {
	// Resolve all symlinks
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // non-existent paths are fine (will fail on read)
		}
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	realBase, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		return fmt.Errorf("resolve base symlinks: %w", err)
	}

	if !strings.HasPrefix(realPath, realBase+string(filepath.Separator)) && realPath != realBase {
		return fmt.Errorf("symlink escape detected: %q resolves to %q outside %q", path, realPath, realBase)
	}

	return nil
}

// ValidateMemoryFile performs all security checks on a memory file path:
// path traversal, unicode normalization, homoglyph detection, and symlink escape.
// Returns the NFC-normalized path if valid, or an error describing the violation.
func ValidateMemoryFile(path, baseDir string) (string, error) {
	// NFC-normalize the filename
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	normalizedName := NormalizeUnicode(name)
	normalizedPath := filepath.Join(dir, normalizedName)

	// Check for path traversal
	if err := ValidateMemoryPath(normalizedPath, baseDir); err != nil {
		return "", err
	}

	// Check for homoglyphs
	if err := DetectHomoglyphs(normalizedName); err != nil {
		return "", err
	}

	// Check for symlink escape
	if err := DetectSymlinkEscape(normalizedPath, baseDir); err != nil {
		return "", err
	}

	return normalizedPath, nil
}
