package ops

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

const ownershipSessionID = "11111111-2222-3333-4444-555555555555"

func ownershipManifest(mergedPath string) *manifest.Manifest {
	return &manifest.Manifest{
		SessionID: ownershipSessionID,
		Sandbox: &manifest.SandboxConfig{
			Enabled:    true,
			ID:         ownershipSessionID,
			Provider:   "overlayfs",
			MergedPath: mergedPath,
			WorkingDir: filepath.Join(mergedPath, "repo"),
			CreatedAt:  time.Now(),
		},
	}
}

// Centralized storage records the physical sandbox root while host cleanup
// addresses the same directory through the ~/.agm symlink. Two spellings of one
// directory are still owned: refusing them skips unmount and removal for every
// centrally stored sandbox, leaving the trees behind forever.
func TestOwnedSandboxPathForArchiveAcceptsCentralizedStorageSpelling(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	centralized := filepath.Join(workspace, ".agm")
	physicalSandboxDir := filepath.Join(centralized, "sandboxes", ownershipSessionID)
	if err := os.MkdirAll(filepath.Join(physicalSandboxDir, "merged"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(centralized, filepath.Join(home, ".agm")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", home)

	got := ownedSandboxPathForArchive(ownershipManifest(filepath.Join(physicalSandboxDir, "merged")))
	want := filepath.Join(home, ".agm", "sandboxes", ownershipSessionID, "merged")
	if got != want {
		t.Fatalf("ownedSandboxPathForArchive() = %q, want the cleanup-base spelling %q", got, want)
	}
}

// A HOME reached through a symlink diverges from the recorded physical spelling
// in plain dotfile mode too.
func TestOwnedSandboxPathForArchiveAcceptsSymlinkedHomeSpelling(t *testing.T) {
	root := t.TempDir()
	physicalHome := filepath.Join(root, "physical-home")
	physicalSandboxDir := filepath.Join(physicalHome, ".agm", "sandboxes", ownershipSessionID)
	if err := os.MkdirAll(filepath.Join(physicalSandboxDir, "merged"), 0o700); err != nil {
		t.Fatal(err)
	}
	logicalHome := filepath.Join(root, "logical-home")
	if err := os.Symlink(physicalHome, logicalHome); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("HOME", logicalHome)

	got := ownedSandboxPathForArchive(ownershipManifest(filepath.Join(physicalSandboxDir, "merged")))
	want := filepath.Join(logicalHome, ".agm", "sandboxes", ownershipSessionID, "merged")
	if got != want {
		t.Fatalf("ownedSandboxPathForArchive() = %q, want the cleanup-base spelling %q", got, want)
	}
}

// Accepting an alternate spelling must not widen ownership: a sandbox that is a
// genuinely different directory, or one whose recorded path cannot be resolved
// at all, is still disowned.
func TestOwnedSandboxPathForArchiveRejectsForeignAndUnresolvablePaths(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".agm", "sandboxes", ownershipSessionID, "merged"), 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, "elsewhere", ".agm", "sandboxes", ownershipSessionID)
	if err := os.MkdirAll(filepath.Join(foreign, "merged"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	for name, mergedPath := range map[string]string{
		"foreign directory":  filepath.Join(foreign, "merged"),
		"absent directory":   filepath.Join(root, "gone", ".agm", "sandboxes", ownershipSessionID, "merged"),
		"different session":  filepath.Join(home, ".agm", "sandboxes", "99999999-0000-0000-0000-000000000000", "merged"),
		"outside .agm scope": filepath.Join(root, "repo", "sandboxes", ownershipSessionID, "merged"),
	} {
		t.Run(name, func(t *testing.T) {
			if got := ownedSandboxPathForArchive(ownershipManifest(mergedPath)); got != "" {
				t.Fatalf("ownedSandboxPathForArchive() = %q, want disowned", got)
			}
		})
	}
}

// The ordinary case must keep returning the recorded path verbatim.
func TestOwnedSandboxPathForArchiveAcceptsExactSpelling(t *testing.T) {
	home := t.TempDir()
	merged := filepath.Join(home, ".agm", "sandboxes", ownershipSessionID, "merged")
	if err := os.MkdirAll(merged, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	if got := ownedSandboxPathForArchive(ownershipManifest(merged)); got != merged {
		t.Fatalf("ownedSandboxPathForArchive() = %q, want %q", got, merged)
	}
}
