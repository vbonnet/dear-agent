//go:build darwin || linux

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOpenRepositoryRootRefusesSymlink(t *testing.T) {
	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("symlink root: %v", err)
	}
	root, err := openRepositoryRoot(linkRoot)
	if err == nil || !strings.Contains(err.Error(), "without following a symlink") {
		if root != nil {
			_ = root.close()
		}
		t.Fatalf("openRepositoryRoot = (%v, %v)", root, err)
	}
	if _, statErr := os.Lstat(filepath.Join(realRoot, "fixed", "SKILL.md")); !os.IsNotExist(statErr) {
		t.Fatalf("symlinked-root refusal created a delegate: %v", statErr)
	}
}

func TestExpectedDelegatesRefusesCanonicalSymlinkParent(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	canonicalRoot := filepath.Join(repo, "spec-governance")
	movedRoot := filepath.Join(t.TempDir(), "spec-governance")
	if err := os.Rename(canonicalRoot, movedRoot); err != nil {
		t.Fatalf("move canonical root: %v", err)
	}
	if err := os.Symlink(movedRoot, canonicalRoot); err != nil {
		t.Fatalf("symlink canonical root: %v", err)
	}
	root := openTestRepositoryRoot(t, repo)
	if _, err := expectedDelegates(root); err == nil || !strings.Contains(err.Error(), "without following symlinks") {
		t.Fatalf("expectedDelegates symlink-parent error = %v", err)
	}
}

func TestCheckDelegatesRefusesDelegateSymlinkParent(t *testing.T) {
	repo := t.TempDir()
	writeCanonicalSkills(t, repo)
	root := openTestRepositoryRoot(t, repo)
	if err := writeDelegates(root); err != nil {
		t.Fatalf("writeDelegates: %v", err)
	}
	delegateRoot := filepath.Join(repo, ".agents", "skills")
	movedRoot := filepath.Join(t.TempDir(), "skills")
	if err := os.Rename(delegateRoot, movedRoot); err != nil {
		t.Fatalf("move delegate root: %v", err)
	}
	if err := os.Symlink(movedRoot, delegateRoot); err != nil {
		t.Fatalf("symlink delegate root: %v", err)
	}
	if err := checkDelegates(root); err == nil || !strings.Contains(err.Error(), "without following symlinks") {
		t.Fatalf("checkDelegates symlink-parent error = %v", err)
	}
}

func TestReadRegularKeepsOpenedAncestorDuringSwap(t *testing.T) {
	repo := t.TempDir()
	originalTree := filepath.Join(repo, "tree")
	if err := os.MkdirAll(filepath.Join(originalTree, "inner"), 0o755); err != nil {
		t.Fatalf("mkdir original tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalTree, "inner", "SKILL.md"), []byte("trusted\n"), 0o644); err != nil {
		t.Fatalf("write trusted file: %v", err)
	}
	attackerTree := filepath.Join(t.TempDir(), "tree")
	if err := os.MkdirAll(filepath.Join(attackerTree, "inner"), 0o755); err != nil {
		t.Fatalf("mkdir attacker tree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(attackerTree, "inner", "SKILL.md"), []byte("attacker\n"), 0o644); err != nil {
		t.Fatalf("write attacker file: %v", err)
	}
	heldTree := filepath.Join(repo, "tree-held")
	root := openTestRepositoryRoot(t, repo)
	data, err := root.readRegularAfterParents(
		"tree/inner/SKILL.md",
		"race fixture",
		64,
		func() error {
			if err := os.Rename(originalTree, heldTree); err != nil {
				return err
			}
			return os.Symlink(attackerTree, originalTree)
		},
	)
	if err != nil {
		t.Fatalf("readRegularAfterParents: %v", err)
	}
	if string(data) != "trusted\n" {
		t.Fatalf("race-safe read = %q, want trusted bytes", data)
	}
}

func TestReadRegularRejectsFinalSymlinkInsertedDuringSwap(t *testing.T) {
	repo := t.TempDir()
	parent := filepath.Join(repo, "tree")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	target := filepath.Join(parent, "SKILL.md")
	if err := os.WriteFile(target, []byte("trusted\n"), 0o644); err != nil {
		t.Fatalf("write trusted file: %v", err)
	}
	attacker := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(attacker, []byte("attacker\n"), 0o644); err != nil {
		t.Fatalf("write attacker file: %v", err)
	}
	root := openTestRepositoryRoot(t, repo)
	_, err := root.readRegularAfterParents(
		"tree/SKILL.md",
		"race fixture",
		64,
		func() error {
			if err := os.Rename(target, filepath.Join(parent, "SKILL.md-held")); err != nil {
				return err
			}
			return os.Symlink(attacker, target)
		},
	)
	if err == nil || !strings.Contains(err.Error(), "without following a symlink") {
		t.Fatalf("final-symlink race error = %v", err)
	}
}

func TestMarkerScanUsesDeterministicOrderAndDoesNotFollowDirectorySymlinks(t *testing.T) {
	repo := t.TempDir()
	writeMarkedSkill(t, repo, "z/SKILL.md", "z")
	writeMarkedSkill(t, repo, "a/deep/SKILL.md", "a")
	outside := t.TempDir()
	writeMarkedSkill(t, outside, "outside/SKILL.md", "outside")
	if err := os.Symlink(filepath.Join(outside, "outside"), filepath.Join(repo, "linked")); err != nil {
		t.Fatalf("symlink outside directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "pointer"), 0o755); err != nil {
		t.Fatalf("mkdir pointer parent: %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside", "SKILL.md"), filepath.Join(repo, "pointer", "SKILL.md")); err != nil {
		t.Fatalf("symlink outside skill: %v", err)
	}
	root := openTestRepositoryRoot(t, repo)
	got, err := root.scanGeneratedMarkers(testMarkerScanLimits())
	if err != nil {
		t.Fatalf("scanGeneratedMarkers: %v", err)
	}
	want := []string{"a/deep/SKILL.md", "z/SKILL.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("marked paths = %v, want %v", got, want)
	}
}

func TestMarkerScanReportsEveryResourceLimit(t *testing.T) {
	t.Run("files", func(t *testing.T) {
		repo := t.TempDir()
		writeFile(t, filepath.Join(repo, "a"), "a")
		writeFile(t, filepath.Join(repo, "b"), "b")
		root := openTestRepositoryRoot(t, repo)
		limits := testMarkerScanLimits()
		limits.maxFiles = 1
		if _, err := root.scanGeneratedMarkers(limits); err == nil || !strings.Contains(err.Error(), "marker scan file limit exceeded") {
			t.Fatalf("file limit error = %v", err)
		}
	})

	t.Run("directories", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo, "a"), 0o755); err != nil {
			t.Fatalf("mkdir a: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(repo, "b"), 0o755); err != nil {
			t.Fatalf("mkdir b: %v", err)
		}
		root := openTestRepositoryRoot(t, repo)
		limits := testMarkerScanLimits()
		limits.maxDirectories = 2
		if _, err := root.scanGeneratedMarkers(limits); err == nil || !strings.Contains(err.Error(), "marker scan directory limit exceeded") {
			t.Fatalf("directory limit error = %v", err)
		}
	})

	t.Run("depth", func(t *testing.T) {
		repo := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repo, "a", "b"), 0o755); err != nil {
			t.Fatalf("mkdir depth fixture: %v", err)
		}
		root := openTestRepositoryRoot(t, repo)
		limits := testMarkerScanLimits()
		limits.maxDepth = 1
		if _, err := root.scanGeneratedMarkers(limits); err == nil || !strings.Contains(err.Error(), "marker scan depth limit exceeded") {
			t.Fatalf("depth limit error = %v", err)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		repo := t.TempDir()
		writeMarkedSkill(t, repo, "a/SKILL.md", "payload")
		root := openTestRepositoryRoot(t, repo)
		limits := testMarkerScanLimits()
		limits.maxBytes = 1
		if _, err := root.scanGeneratedMarkers(limits); err == nil || !strings.Contains(err.Error(), "marker scan byte limit exceeded") {
			t.Fatalf("byte limit error = %v", err)
		}
	})

	t.Run("time", func(t *testing.T) {
		repo := t.TempDir()
		root := openTestRepositoryRoot(t, repo)
		limits := testMarkerScanLimits()
		limits.elapsedBudget = time.Second
		started := time.Unix(1, 0)
		limits.now = func(checkpoint string) time.Time {
			if checkpoint == "before scanning directory ." {
				return started.Add(2 * time.Second)
			}
			return started
		}
		if _, err := root.scanGeneratedMarkers(limits); err == nil || !strings.Contains(err.Error(), "checked elapsed-time budget exceeded") {
			t.Fatalf("elapsed-budget error = %v", err)
		}
	})
}

func TestMarkerScanChecksElapsedBudgetAfterFilesystemCallsAndBeforeSuccess(t *testing.T) {
	tests := []struct {
		name       string
		checkpoint string
		prepare    func(*testing.T, string)
	}{
		{
			name:       "after directory read",
			checkpoint: "after reading directory .",
		},
		{
			name:       "after marker file read",
			checkpoint: "after reading marker file SKILL.md",
			prepare: func(t *testing.T, repo string) {
				writeMarkedSkill(t, repo, "SKILL.md", "payload")
			},
		},
		{
			name:       "after directory entry sort",
			checkpoint: "after sorting directory entries for .",
			prepare: func(t *testing.T, repo string) {
				writeFile(t, filepath.Join(repo, "ordinary"), "payload")
			},
		},
		{
			name:       "before successful return",
			checkpoint: "after sorting marked paths and closing the retained scan root",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := t.TempDir()
			if test.prepare != nil {
				test.prepare(t, repo)
			}
			root := openTestRepositoryRoot(t, repo)
			limits := testMarkerScanLimits()
			limits.elapsedBudget = time.Second
			started := time.Unix(1, 0)
			limits.now = func(checkpoint string) time.Time {
				if checkpoint == test.checkpoint {
					return started.Add(2 * time.Second)
				}
				return started
			}
			_, err := root.scanGeneratedMarkers(limits)
			if err == nil || !strings.Contains(err.Error(), "checked elapsed-time budget exceeded at "+test.checkpoint) {
				t.Fatalf("checkpoint %q error = %v", test.checkpoint, err)
			}
		})
	}
}

func testMarkerScanLimits() markerScanLimits {
	return markerScanLimits{
		maxFiles:       100,
		maxDirectories: 100,
		maxDepth:       10,
		maxBytes:       1 << 20,
		elapsedBudget:  time.Minute,
		now:            func(_ string) time.Time { return time.Now() },
	}
}

func writeMarkedSkill(t *testing.T, root, relative, payload string) {
	t.Helper()
	writeFile(t, filepath.Join(root, filepath.FromSlash(relative)), payload+"\n"+generatedMarker+"\n")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
