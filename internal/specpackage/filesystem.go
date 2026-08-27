package specpackage

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type treeEntry struct {
	path      string
	directory bool
	mode      fs.FileMode
	identity  fileIdentity
	state     string
}

type fileIdentity struct {
	device string
	inode  string
}

type fileSnapshot struct {
	data []byte
	mode fs.FileMode
}

func cleanAbsolutePath(value, label string) (string, error) {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("%s must be a nonempty valid UTF-8 path", label)
	}
	if !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", fmt.Errorf("%s must be a clean absolute path", label)
	}
	return value, nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("package operation requires a non-nil context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("package operation cancelled: %w", err)
	}
	return nil
}

func equalSortedPaths(actual, expected []string, kind string) error {
	if len(actual) != len(expected) {
		return fmt.Errorf("package %s count is %d, want %d", kind, len(actual), len(expected))
	}
	for index := range expected {
		if actual[index] != expected[index] {
			return fmt.Errorf("package %s at index %d is %q, want %q", kind, index, actual[index], expected[index])
		}
	}
	return nil
}

func equalTreeSnapshots(initial, final []treeEntry, label string) error {
	if len(initial) != len(final) {
		return fmt.Errorf("%s tree changed during the operation", label)
	}
	for index := range initial {
		if initial[index] != final[index] {
			return fmt.Errorf("%s tree entry %q changed during the operation", label, initial[index].path)
		}
	}
	return nil
}
