package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A bare "~" expands to an absolute path that passes every remaining
// validation check, and resolveSandboxLowerDirs returns configured repos
// directly — it never reaches the $HOME refusal that guards only the scan
// fallback. OverlayFS would then publish the whole home directory as a
// readable lower layer, exposing credentials outside the requested repository.
func TestRejectUnsafeSandboxRoots(t *testing.T) {
	home := t.TempDir()
	repo := filepath.Join(home, "src", "project")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	tests := []struct {
		name    string
		dirs    []string
		wantErr string
	}{
		{name: "a repository directory is allowed", dirs: []string{repo}},
		{name: "several repositories are allowed", dirs: []string{repo, filepath.Join(home, "src")}},
		{name: "no entries is allowed", dirs: nil},

		{name: "home itself is refused", dirs: []string{home}, wantErr: "home directory"},
		{name: "home with a trailing slash is refused", dirs: []string{home + "/"}, wantErr: "home directory"},
		{name: "home reached by traversal is refused", dirs: []string{filepath.Join(repo, "..", "..")}, wantErr: "home directory"},
		{name: "filesystem root is refused", dirs: []string{"/"}, wantErr: "filesystem root"},
		{name: "an empty entry is refused", dirs: []string{""}, wantErr: "empty entry"},
		{name: "a safe entry does not excuse an unsafe one", dirs: []string{repo, home}, wantErr: "home directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectUnsafeSandboxRoots(tt.dirs, home, "sandbox.repos")
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("rejectUnsafeSandboxRoots() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("rejectUnsafeSandboxRoots(%v) = nil, want an error mentioning %q", tt.dirs, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to mention %q", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), "sandbox.repos") {
				t.Errorf("error = %q, want it to name the offending field", err)
			}
		})
	}
}

// A symlink pointing at $HOME must be refused too, or the check is trivially
// bypassed by naming the link instead of the directory.
func TestRejectUnsafeSandboxRoots_FollowsSymlinks(t *testing.T) {
	home := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(home, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	err := rejectUnsafeSandboxRoots([]string{link}, home, "sandbox.repos")
	if err == nil {
		t.Fatal("a symlink to $HOME must be refused")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %q, want it to explain the home-directory refusal", err)
	}
}
