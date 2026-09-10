//go:build darwin && arm64

package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinSourceConstructionRetainsProtectedRepository(t *testing.T) {
	repository, configContent := writeDarwinSourceConstructionRepository(t)
	locator := sourceRepositoryLocator{
		path: repository,
		seal: validSourceRepositoryLocator,
	}
	owner, acquisition := retainSourceConstruction(context.Background(), locator)
	if owner == nil || !acquisition.proved() {
		t.Fatalf("retain source construction = %+v / %+v", owner, acquisition)
	}
	t.Cleanup(func() {
		var outcome sourceUseOutcome
		owner.closeInto(&outcome)
	})
	if !owner.validInitialRetention() || owner.state != sourceConstructionActive {
		t.Fatalf("initial source construction is not valid and active: %+v", owner)
	}

	components, err := absolutePathComponents(repository)
	if err != nil {
		t.Fatalf("split repository path: %v", err)
	}
	if owner.repository == nil || owner.repository.path != repository ||
		!owner.repository.root.validOpenDirectory() ||
		len(owner.repository.pathClaims) != len(components)+1 {
		t.Fatalf("retained repository = %+v", owner.repository)
	}
	wantRepositoryClaim := owner.repository.root.evidence.pathClaim()
	if got := owner.repository.pathClaims[len(owner.repository.pathClaims)-1]; got != wantRepositoryClaim {
		t.Fatalf("repository terminal path claim = %+v, want %+v", got, wantRepositoryClaim)
	}
	if owner.git == nil || !owner.git.root.validOpenDirectory() {
		t.Fatalf("retained .git root = %+v", owner.git)
	}
	if owner.objects == nil || !owner.objects.root.validOpenDirectory() {
		t.Fatalf("retained objects root = %+v", owner.objects)
	}
	if owner.config == nil || !owner.config.descriptor.validOpen() ||
		owner.config.descriptor.kind != sourceObservedRegular {
		t.Fatalf("retained config = %+v", owner.config)
	}
	if owner.config.claim.objectFormat != objectFormatSHA1 ||
		len(owner.config.claim.entries) != 2 || owner.config.claim.worktreeConfigPresent {
		t.Fatalf("retained config claim = %+v", owner.config.claim)
	}
	wantConfigDigest := Digest(sha256.Sum256(configContent))
	if owner.config.digest != wantConfigDigest ||
		owner.config.evidence.snapshot.size != int64(len(configContent)) ||
		owner.config.evidence.snapshot.identity.UID != uint32(os.Geteuid()) ||
		owner.config.evidence.snapshot.identity.Mode&0o7777 != 0o600 {
		t.Fatalf(
			"retained config evidence = %+v digest %x, want size %d mode 0600 digest %x",
			owner.config.evidence,
			owner.config.digest,
			len(configContent),
			wantConfigDigest,
		)
	}
	if !owner.packedRefs.valid() || owner.packedRefs.state != sourcePackedRefsUnresolved ||
		owner.packedRefs.leaf != nil {
		t.Fatalf("initial packed-refs slot = %+v, want unresolved with no leaf", owner.packedRefs)
	}

	rootHandles := []*os.Root{
		owner.repository.root.root.root,
		owner.git.root.root.root,
		owner.objects.root.root.root,
	}
	descriptorHandles := []*os.File{
		owner.repository.root.descriptor.file,
		owner.git.root.descriptor.file,
		owner.config.descriptor.file,
		owner.objects.root.descriptor.file,
	}
	for index, root := range rootHandles {
		if _, err := root.Lstat("."); err != nil {
			t.Fatalf("retained root handle %d is not live: %v", index, err)
		}
	}
	for index, descriptor := range descriptorHandles {
		if _, err := descriptor.Stat(); err != nil {
			t.Fatalf("retained descriptor handle %d is not live: %v", index, err)
		}
	}
	readConfig := make([]byte, len(configContent))
	if read, err := owner.config.descriptor.file.ReadAt(readConfig, 0); read != len(readConfig) ||
		err != nil || !bytes.Equal(readConfig, configContent) {
		t.Fatalf("retained config read = %d bytes %q / %v", read, readConfig, err)
	}

	var firstClose sourceUseOutcome
	owner.closeInto(&firstClose)
	if !firstClose.proved() || owner.state != sourceConstructionClosed || owner.closeFailure {
		t.Fatalf("first source-construction close = %+v, owner %+v", firstClose, owner)
	}
	if owner.repository.root.root != nil || owner.repository.root.descriptor != nil ||
		owner.git.root.root != nil || owner.git.root.descriptor != nil ||
		owner.config.descriptor != nil ||
		owner.objects.root.root != nil || owner.objects.root.descriptor != nil {
		t.Fatalf("closed source construction retained owned handles: %+v", owner)
	}
	for index, root := range rootHandles {
		if _, err := root.Lstat("."); err == nil {
			t.Fatalf("source root handle %d remained live after close", index)
		}
	}
	for index, descriptor := range descriptorHandles {
		if _, err := descriptor.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("source descriptor handle %d remained live after close: %v", index, err)
		}
	}

	var repeatedClose sourceUseOutcome
	owner.closeInto(&repeatedClose)
	if !repeatedClose.proved() || owner.state != sourceConstructionClosed || owner.closeFailure {
		t.Fatalf("repeated source-construction close = %+v, owner %+v", repeatedClose, owner)
	}
}

func TestDarwinSourceConstructionRejectsMissingAndWrongKindEntries(t *testing.T) {
	for _, test := range []struct {
		name      string
		arrange   func(*testing.T, string)
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "missing git",
			arrange:   func(*testing.T, string) {},
			operation: OperationOpen,
			cause:     CauseNotFound,
		},
		{
			name: "git regular",
			arrange: func(t *testing.T, repository string) {
				writeDarwinSourceConstructionFile(t, filepath.Join(repository, ".git"), nil)
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
		{
			name: "git symlink",
			arrange: func(t *testing.T, repository string) {
				if err := os.Mkdir(filepath.Join(repository, "actual-git"), 0o700); err != nil {
					t.Fatalf("make symlink target: %v", err)
				}
				if err := os.Symlink("actual-git", filepath.Join(repository, ".git")); err != nil {
					t.Fatalf("make .git symlink: %v", err)
				}
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
		{
			name: "missing config",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git", "objects"))
			},
			operation: OperationOpen,
			cause:     CauseNotFound,
		},
		{
			name: "config directory",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git", "config"))
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
		{
			name: "config symlink",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git"))
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(repository, ".git", "actual-config"),
					[]byte(validSourceConstructionConfig),
				)
				if err := os.Symlink("actual-config", filepath.Join(repository, ".git", "config")); err != nil {
					t.Fatalf("make config symlink: %v", err)
				}
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
		{
			name: "missing objects",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git"))
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(repository, ".git", "config"),
					[]byte(validSourceConstructionConfig),
				)
			},
			operation: OperationOpen,
			cause:     CauseNotFound,
		},
		{
			name: "objects regular",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git"))
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(repository, ".git", "config"),
					[]byte(validSourceConstructionConfig),
				)
				writeDarwinSourceConstructionFile(t, filepath.Join(repository, ".git", "objects"), nil)
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
		{
			name: "objects symlink",
			arrange: func(t *testing.T, repository string) {
				makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git", "actual-objects"))
				writeDarwinSourceConstructionFile(
					t,
					filepath.Join(repository, ".git", "config"),
					[]byte(validSourceConstructionConfig),
				)
				if err := os.Symlink("actual-objects", filepath.Join(repository, ".git", "objects")); err != nil {
					t.Fatalf("make objects symlink: %v", err)
				}
			},
			operation: OperationValidate,
			cause:     CauseUnsupported,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := physicalDarwinTempDir(t)
			test.arrange(t, repository)
			owner, outcome := retainSourceConstruction(
				context.Background(),
				sourceRepositoryLocator{path: repository, seal: validSourceRepositoryLocator},
			)
			if owner != nil {
				var closeOutcome sourceUseOutcome
				owner.closeInto(&closeOutcome)
				t.Fatalf("invalid source fixture returned owner %+v", owner)
			}
			requireFailureRecord(t, outcome.primary, PhaseSource, test.operation, test.cause)
			if outcome.descriptorClose != nil {
				t.Fatalf("invalid source fixture leaked a close failure: %+v", outcome.descriptorClose)
			}
		})
	}
}

func writeDarwinSourceConstructionRepository(t *testing.T) (string, []byte) {
	t.Helper()
	repository := physicalDarwinTempDir(t)
	makeDarwinSourceConstructionDirectory(t, filepath.Join(repository, ".git", "objects"))
	content := []byte(validSourceConstructionConfig)
	writeDarwinSourceConstructionFile(t, filepath.Join(repository, ".git", "config"), content)
	return repository, content
}

func makeDarwinSourceConstructionDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("make source construction directory %q: %v", path, err)
	}
}

func writeDarwinSourceConstructionFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write source construction file %q: %v", path, err)
	}
}
