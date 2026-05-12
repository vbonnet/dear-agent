package stophook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasWayfinder(t *testing.T) {
	t.Run("file marker", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte("x"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if !HasWayfinder(dir) {
			t.Fatalf("expected true with WAYFINDER-STATUS.md marker")
		}
	})
	t.Run("directory marker", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".wayfinder"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if !HasWayfinder(dir) {
			t.Fatalf("expected true with .wayfinder dir marker")
		}
	})
	t.Run("no marker", func(t *testing.T) {
		if HasWayfinder(t.TempDir()) {
			t.Fatalf("expected false in empty dir")
		}
	})
}

func TestDetectTestFramework(t *testing.T) {
	tests := []struct {
		name      string
		marker    string
		framework string
	}{
		{"go", "go.mod", "go"},
		{"npm", "package.json", "npm"},
		{"pytest via pytest.ini", "pytest.ini", "pytest"},
		{"pytest via setup.py", "setup.py", "pytest"},
		{"pytest via pyproject.toml", "pyproject.toml", "pytest"},
		{"cargo", "Cargo.toml", "cargo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tt.marker), []byte("x"), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			got := DetectTestFramework(dir)
			if got != tt.framework {
				t.Fatalf("got %q, want %q", got, tt.framework)
			}
		})
	}
	t.Run("none", func(t *testing.T) {
		if got := DetectTestFramework(t.TempDir()); got != "" {
			t.Fatalf("expected empty for unknown project, got %q", got)
		}
	})
	t.Run("first match wins", func(t *testing.T) {
		// go.mod is listed before package.json — both present → "go".
		dir := t.TempDir()
		for _, f := range []string{"go.mod", "package.json"} {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
				t.Fatalf("write %s: %v", f, err)
			}
		}
		if got := DetectTestFramework(dir); got != "go" {
			t.Fatalf("got %q, want %q (go.mod is checked first)", got, "go")
		}
	})
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !FileExists(existing) {
		t.Fatalf("expected true for existing file")
	}
	if FileExists(filepath.Join(dir, "missing.txt")) {
		t.Fatalf("expected false for missing file")
	}
}
