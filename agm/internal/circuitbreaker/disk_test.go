package circuitbreaker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNewDiskReaderUsesPlannedSandboxRoot(t *testing.T) {
	root := physicalDiskTestDir(t)

	dr, err := NewDiskReader(root)
	if err != nil {
		t.Fatalf("NewDiskReader: %v", err)
	}
	got, ok := dr.(StatfsDiskReader)
	if !ok {
		t.Fatalf("NewDiskReader returned %T, want StatfsDiskReader", dr)
	}
	if got.Path != root {
		t.Fatalf("probe path = %q, want planned sandbox root %q", got.Path, root)
	}
	if _, err := dr.FreeDiskGB(); err != nil {
		t.Fatalf("FreeDiskGB: %v", err)
	}
}

func TestNewDiskReaderUsesDeepestExistingAncestorForENOENT(t *testing.T) {
	ancestor := filepath.Join(physicalDiskTestDir(t), "existing")
	if err := os.Mkdir(ancestor, 0o700); err != nil {
		t.Fatal(err)
	}
	plannedRoot := filepath.Join(ancestor, "missing", "sandboxes")

	dr, err := NewDiskReader(plannedRoot)
	if err != nil {
		t.Fatalf("NewDiskReader: %v", err)
	}
	got := dr.(StatfsDiskReader)
	if got.Path != plannedRoot {
		t.Fatalf("retained path = %q, want planned sandbox root %q", got.Path, plannedRoot)
	}
	if _, err := dr.FreeDiskGB(); err != nil {
		t.Fatalf("FreeDiskGB via existing ancestor %q: %v", ancestor, err)
	}
}

func TestNewDiskReaderFailsClosedForNonENOENT(t *testing.T) {
	file := filepath.Join(physicalDiskTestDir(t), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := NewDiskReader(filepath.Join(file, "sandboxes"))
	if err == nil {
		t.Fatal("NewDiskReader succeeded through a non-ENOENT path error")
	}
	if !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("NewDiskReader error = %v, want ENOTDIR", err)
	}
}

func TestNewDiskReaderFailsClosedForSymlinkCandidates(t *testing.T) {
	tests := []struct {
		name   string
		target func(t *testing.T) string
	}{
		{
			name:   "existing target",
			target: physicalDiskTestDir,
		},
		{
			name: "dangling target",
			target: func(t *testing.T) string {
				return filepath.Join(physicalDiskTestDir(t), "missing")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := filepath.Join(physicalDiskTestDir(t), "sandbox-link")
			if err := os.Symlink(tt.target(t), link); err != nil {
				t.Fatal(err)
			}
			for _, plannedRoot := range []string{
				link,
				filepath.Join(link, "missing", "sandboxes"),
			} {
				if _, err := NewDiskReader(plannedRoot); err == nil {
					t.Errorf("NewDiskReader(%q) accepted a symlink candidate", plannedRoot)
				}
			}
		})
	}
}

func TestNewDiskReaderFailsClosedForExistingPathThroughSymlinkAncestor(t *testing.T) {
	target := physicalDiskTestDir(t)
	if err := os.MkdirAll(filepath.Join(target, "existing", "sandboxes"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(physicalDiskTestDir(t), "linked-parent")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	plannedRoot := filepath.Join(link, "existing", "sandboxes")
	if _, err := NewDiskReader(plannedRoot); err == nil || !strings.Contains(err.Error(), "contains a symlink") {
		t.Fatalf("NewDiskReader(%q) error = %v, want symlink-ancestor refusal", plannedRoot, err)
	}
}

func TestNewDiskReaderRevalidatesPlannedPathOnEveryProbe(t *testing.T) {
	parent := physicalDiskTestDir(t)
	plannedRoot := filepath.Join(parent, "sandboxes")
	dr, err := NewDiskReader(plannedRoot)
	if err != nil {
		t.Fatalf("NewDiskReader: %v", err)
	}

	if err := os.Symlink(physicalDiskTestDir(t), plannedRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := dr.FreeDiskGB(); err == nil {
		t.Fatal("FreeDiskGB reused a pinned ancestor after the planned root became a symlink")
	}
}

func TestNewDiskReaderRejectsAmbiguousPaths(t *testing.T) {
	unclean := physicalDiskTestDir(t) + string(filepath.Separator) + ".." +
		string(filepath.Separator) + "sandboxes"
	for _, path := range []string{"", "relative/sandboxes", unclean, string(filepath.Separator)} {
		if _, err := NewDiskReader(path); err == nil {
			t.Errorf("NewDiskReader(%q) returned nil error", path)
		}
	}
}

func physicalDiskTestDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckDiskHeadroomRunsOnlyDiskGate(t *testing.T) {
	cfg := baseCfg()
	cfg.MinFreeDiskGB = 15

	if gate := CheckDiskHeadroom(cfg, stubDisk{gb: 14}); gate.Gate != "disk" || gate.Passed {
		t.Fatalf("CheckDiskHeadroom low disk = %+v, want disk refusal", gate)
	}
	if gate := CheckDiskHeadroom(cfg, stubDisk{gb: 16}); gate.Gate != "disk" || !gate.Passed {
		t.Fatalf("CheckDiskHeadroom healthy disk = %+v, want disk pass", gate)
	}
	if gate := CheckDiskHeadroom(cfg, nil); gate.Gate != "disk" || gate.Passed {
		t.Fatalf("CheckDiskHeadroom nil reader = %+v, want fail-closed disk refusal", gate)
	}
}
