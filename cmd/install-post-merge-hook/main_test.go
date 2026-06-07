package main

import (
	"path/filepath"
	"testing"
)

func TestExpandHooksPath(t *testing.T) {
	const home = "/home/u"
	const repo = "/repo"
	tests := []struct {
		name, raw, want string
	}{
		{"tilde", "~/.config/git/hooks", filepath.Join(home, ".config/git/hooks")},
		{"absolute", "/etc/git/hooks", "/etc/git/hooks"},
		{"relative", ".githooks", filepath.Join(repo, ".githooks")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandHooksPath(tt.raw, repo, home); got != tt.want {
				t.Errorf("expandHooksPath(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestIsChezmoiManaged(t *testing.T) {
	const home = "/home/u"
	managed := ".config/git/hooks\n.config/git/hooks/pre-commit\n.gitconfig\n"
	tests := []struct {
		name, hooksDir string
		want           bool
	}{
		{"managed dir", "/home/u/.config/git/hooks", true},
		{"unmanaged dir under home", "/home/u/other/hooks", false},
		{"dir outside home", "/repo/.git/hooks", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isChezmoiManaged(managed, tt.hooksDir, home); got != tt.want {
				t.Errorf("isChezmoiManaged(%q) = %v, want %v", tt.hooksDir, got, tt.want)
			}
		})
	}
}

// A post-merge file listed (rather than the dir) also counts as managed.
func TestIsChezmoiManaged_FileEntry(t *testing.T) {
	const home = "/home/u"
	managed := ".config/git/hooks/post-merge\n"
	if !isChezmoiManaged(managed, "/home/u/.config/git/hooks", home) {
		t.Error("expected a managed post-merge file entry to count as managed")
	}
}
