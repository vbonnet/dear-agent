//go:build darwin || linux

package marketplaceparity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

const projectionTestLicense = "fixture Apache-2.0 license\n"

func projectionTestCanonicalPluginEntry() PluginEntry {
	return PluginEntry{
		Name:        canonicalPluginName,
		Source:      canonicalPluginSource,
		Description: canonicalPluginDescription,
		Version:     canonicalPluginVersion,
		Author: PluginAuthor{
			Name: canonicalPluginAuthor,
			URL:  canonicalPluginRepository,
		},
		Repository:   canonicalPluginRepository,
		License:      canonicalPluginLicense,
		Capabilities: []string{"skills"},
	}
}

func projectionTestPrepareLicenses(t *testing.T, fixture projectionTestFixture) (string, string) {
	t.Helper()
	repository := filepath.Join(fixture.root, canonicalRepositoryLicense)
	packaged := filepath.Join(fixture.root, "spec-governance", canonicalPackagedLicense)
	projectionTestWriteFile(t, repository, projectionTestLicense)
	projectionTestWriteFile(t, packaged, projectionTestLicense)
	return repository, packaged
}

func projectionTestValidateCanonicalPlugin(root string) error {
	_, err := canonicalPluginSnapshot(root, projectionTestCanonicalPluginEntry())
	return err
}

func TestSpecGovernanceLicenseProjectionRequiresExactBytes(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		projectionTestPrepareLicenses(t, fixture)
		if err := projectionTestValidateCanonicalPlugin(fixture.root); err != nil {
			t.Fatalf("canonicalPluginSnapshot() exact license projection: %v", err)
		}
	})

	for _, target := range []struct {
		name string
		path func(string, string) string
	}{
		{name: "repository license missing", path: func(repository, _ string) string { return repository }},
		{name: "packaged license missing", path: func(_, packaged string) string { return packaged }},
	} {
		t.Run(target.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			repository, packaged := projectionTestPrepareLicenses(t, fixture)
			if err := os.Remove(target.path(repository, packaged)); err != nil {
				t.Fatal(err)
			}
			if err := projectionTestValidateCanonicalPlugin(fixture.root); err == nil {
				t.Fatalf("canonicalPluginSnapshot() accepted %s", target.name)
			}
		})
	}

	for _, target := range []struct {
		name string
		path func(string, string) string
	}{
		{name: "repository license differs", path: func(repository, _ string) string { return repository }},
		{name: "packaged license differs", path: func(_, packaged string) string { return packaged }},
	} {
		t.Run(target.name, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			repository, packaged := projectionTestPrepareLicenses(t, fixture)
			if err := os.WriteFile(target.path(repository, packaged), []byte("different license bytes\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := projectionTestValidateCanonicalPlugin(fixture.root)
			if err == nil {
				t.Fatalf("canonicalPluginSnapshot() accepted %s", target.name)
			}
			if !strings.Contains(err.Error(), "do not exactly match") {
				t.Fatalf("canonicalPluginSnapshot() error = %q, want exact license-byte mismatch", err)
			}
		})
	}
}

func TestSpecGovernanceLicenseProjectionRejectsNonregularObjects(t *testing.T) {
	t.Run("packaged directory", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		_, packaged := projectionTestPrepareLicenses(t, fixture)
		if err := os.Remove(packaged); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(packaged, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := projectionTestValidateCanonicalPlugin(fixture.root); err == nil {
			t.Fatal("canonicalPluginSnapshot() accepted a directory as packaged LICENSE")
		}
	})

	t.Run("repository hardlink", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		repository, _ := projectionTestPrepareLicenses(t, fixture)
		projectionTestReplaceWithHardlink(t, repository)
		if err := projectionTestValidateCanonicalPlugin(fixture.root); err == nil {
			t.Fatal("canonicalPluginSnapshot() accepted a hard-linked repository LICENSE")
		}
	})

	t.Run("packaged symlink", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		_, packaged := projectionTestPrepareLicenses(t, fixture)
		projectionTestReplaceWithSymlink(t, packaged)
		if err := projectionTestValidateCanonicalPlugin(fixture.root); err == nil {
			t.Fatal("canonicalPluginSnapshot() accepted a symlinked packaged LICENSE")
		}
	})

	t.Run("repository FIFO rejects promptly", func(t *testing.T) {
		fixture := newProjectionTestFixture(t)
		repository, _ := projectionTestPrepareLicenses(t, fixture)
		if err := os.Remove(repository); err != nil {
			t.Fatal(err)
		}
		if err := unix.Mkfifo(repository, 0o600); err != nil {
			t.Skipf("FIFOs unavailable: %v", err)
		}
		projectionTestRequirePromptError(t, "repository LICENSE FIFO", func() error {
			return projectionTestValidateCanonicalPlugin(fixture.root)
		}, func() {
			fd, err := unix.Open(repository, unix.O_WRONLY|unix.O_NONBLOCK, 0)
			if err == nil {
				_ = unix.Close(fd)
			}
		})
	})
}

func TestRepositoryLicenseReadRejectsVisibleReplacement(t *testing.T) {
	root := t.TempDir()
	licensePath := filepath.Join(root, canonicalRepositoryLicense)
	replacementPath := filepath.Join(root, "replacement-license")
	projectionTestWriteFile(t, licensePath, projectionTestLicense)
	projectionTestWriteFile(t, replacementPath, projectionTestLicense)

	checkpointCalled := false
	_, err := readAnchoredRegularAtCheckpoint(root, canonicalRepositoryLicense, maxCanonicalFileBytes, func() error {
		checkpointCalled = true
		return os.Rename(replacementPath, licensePath)
	})
	if !checkpointCalled {
		t.Fatal("anchored repository LICENSE read did not reach replacement checkpoint")
	}
	if err == nil {
		t.Fatal("anchored repository LICENSE read accepted a visible inode replacement")
	}
	if !strings.Contains(err.Error(), "changed while it was read") && !strings.Contains(err.Error(), "hard links") {
		t.Fatalf("anchored repository LICENSE read error = %q, want stable-identity rejection", err)
	}
}
