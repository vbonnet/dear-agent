package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestDescribeAssociationStorage verifies that the post-association storage
// report is truthful: it must never print a "Manifest:" path to a file that
// does not exist on disk (the historical "bogus path" bug), and must fall back
// to reporting the Dolt storage location instead.
func TestDescribeAssociationStorage(t *testing.T) {
	tmpDir := t.TempDir()

	// A manifest file that actually exists on disk.
	realManifest := filepath.Join(tmpDir, "real-session", "manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(realManifest), 0700); err != nil {
		t.Fatalf("setup: mkdir: %v", err)
	}
	if err := os.WriteFile(realManifest, []byte("session_id: real-session\n"), 0600); err != nil {
		t.Fatalf("setup: write manifest: %v", err)
	}

	// A directory (not a file) at the manifest path — must NOT be reported as
	// a manifest, since the historical bug created an empty directory.
	dirOnlyPath := filepath.Join(tmpDir, "empty-session", "manifest.yaml")
	if err := os.MkdirAll(dirOnlyPath, 0700); err != nil {
		t.Fatalf("setup: mkdir dir-only: %v", err)
	}

	m := &manifest.Manifest{SessionID: "abc123", Workspace: "oss"}

	tests := []struct {
		name         string
		manifest     *manifest.Manifest
		workspace    string
		manifestPath string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "real manifest file on disk is reported as Manifest path",
			manifest:     m,
			workspace:    "oss",
			manifestPath: realManifest,
			wantContains: []string{"Manifest: " + realManifest},
			wantAbsent:   []string{"Dolt"},
		},
		{
			name:         "nonexistent manifest path falls back to Dolt storage",
			manifest:     m,
			workspace:    "oss",
			manifestPath: filepath.Join(tmpDir, "does-not-exist", "manifest.yaml"),
			wantContains: []string{"Session ID: abc123", "Storage:", "Dolt", "oss"},
			wantAbsent:   []string{"Manifest: "},
		},
		{
			name:         "empty manifest path falls back to Dolt storage",
			manifest:     m,
			workspace:    "oss",
			manifestPath: "",
			wantContains: []string{"Session ID: abc123", "Dolt", "oss"},
			wantAbsent:   []string{"Manifest: "},
		},
		{
			name:         "directory at manifest path is not reported as a manifest file",
			manifest:     m,
			workspace:    "oss",
			manifestPath: dirOnlyPath,
			wantContains: []string{"Dolt"},
			wantAbsent:   []string{"Manifest: "},
		},
		{
			name:         "empty workspace falls back to manifest workspace",
			manifest:     &manifest.Manifest{SessionID: "xyz789", Workspace: "from-manifest"},
			workspace:    "",
			manifestPath: "",
			wantContains: []string{"from-manifest", "xyz789"},
		},
		{
			name:         "no workspace anywhere falls back to default",
			manifest:     &manifest.Manifest{SessionID: "noWs"},
			workspace:    "",
			manifestPath: "",
			wantContains: []string{"default", "noWs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describeAssociationStorage(tt.manifest, tt.workspace, tt.manifestPath)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("output %q does not contain %q", got, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("output %q should not contain %q", got, absent)
				}
			}
		})
	}
}
