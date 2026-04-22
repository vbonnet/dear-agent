package configloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	// Get actual home directory for comparisons
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "tilde alone expands to home",
			input: "~",
			want:  homeDir,
		},
		{
			name:  "tilde slash expands to home",
			input: "~/",
			want:  homeDir,
		},
		{
			name:  "tilde with path expands correctly",
			input: "~/.config/app",
			want:  filepath.Join(homeDir, ".config/app"),
		},
		{
			name:  "tilde with nested path",
			input: "~/path/to/config.yaml",
			want:  filepath.Join(homeDir, "path/to/config.yaml"),
		},
		{
			name:  "absolute path unchanged",
			input: "/etc/app/config.yaml",
			want:  "/etc/app/config.yaml",
		},
		{
			name:  "relative path unchanged",
			input: "relative/path/config.yaml",
			want:  "relative/path/config.yaml",
		},
		{
			name:  "empty string unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "tilde with text (not slash) unchanged",
			input: "~something",
			want:  "~something",
		},
		{
			name:  "single dot unchanged",
			input: ".",
			want:  ".",
		},
		{
			name:  "double dot unchanged",
			input: "..",
			want:  "..",
		},
		{
			name:  "dot slash unchanged",
			input: "./config.yaml",
			want:  "./config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandHome(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandHome(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestExpandHomeWithPlatformSeparator tests platform-specific path separators
func TestExpandHomeWithPlatformSeparator(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	// Build path with platform-specific separator
	input := "~" + string(filepath.Separator) + "config"
	want := filepath.Join(homeDir, "config")

	got, err := ExpandHome(input)
	if err != nil {
		t.Fatalf("ExpandHome(%q) unexpected error: %v", input, err)
	}
	if got != want {
		t.Errorf("ExpandHome(%q) = %q, want %q", input, got, want)
	}
}

// TestExpandHomeIdempotent tests that expanding an already-expanded path is safe
func TestExpandHomeIdempotent(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	// Expand once
	first, err := ExpandHome("~/.config")
	if err != nil {
		t.Fatalf("First expansion failed: %v", err)
	}

	// Expand again (should be unchanged since it's now absolute)
	second, err := ExpandHome(first)
	if err != nil {
		t.Fatalf("Second expansion failed: %v", err)
	}

	want := filepath.Join(homeDir, ".config")
	if first != want {
		t.Errorf("First expansion = %q, want %q", first, want)
	}
	if second != want {
		t.Errorf("Second expansion = %q, want %q", second, want)
	}
	if first != second {
		t.Errorf("Expansions not idempotent: first=%q, second=%q", first, second)
	}
}
