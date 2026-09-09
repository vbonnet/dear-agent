//go:build darwin && arm64

package buildauthority

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestRetainExecutableCandidateOwnsStableNoFollowDescriptor(t *testing.T) {
	base := physicalDarwinTempDir(t)
	path := filepath.Join(base, "tool")
	image := writeMachOExecutableFixture(t, path, machOProfileGo)

	candidate, err := retainExecutableCandidate(context.Background(), path)
	if err != nil {
		t.Fatalf("retain executable candidate: %v", err)
	}
	if candidate.retained == nil || candidate.retained.leaf == nil {
		t.Fatal("candidate did not retain executable substrate")
	}
	wantDigest := Digest(sha256.Sum256(image))
	if candidate.retained.digest != wantDigest {
		t.Fatalf("candidate digest = %x, want %x", candidate.retained.digest, wantDigest)
	}
	if err := candidate.retained.revalidate(context.Background()); err != nil {
		t.Fatalf("revalidate executable candidate: %v", err)
	}
	descriptor := candidate.retained.leaf.descriptor
	if err := candidate.close(); err != nil {
		t.Fatalf("close executable candidate: %v", err)
	}
	if _, err := descriptor.Stat(); err == nil {
		t.Fatal("candidate close left retained descriptor open")
	}
	if err := candidate.close(); err != nil {
		t.Fatalf("repeat candidate close: %v", err)
	}
}

func TestExecutableCandidateConcurrentCloseTransfersClosureOnce(t *testing.T) {
	base := physicalDarwinTempDir(t)
	path := filepath.Join(base, "tool")
	writeMachOExecutableFixture(t, path, machOProfileGo)
	candidate, err := retainExecutableCandidate(context.Background(), path)
	if err != nil {
		t.Fatalf("retain executable candidate: %v", err)
	}
	descriptor := candidate.retained.leaf.descriptor

	const closers = 32
	errorsSeen := make(chan error, closers)
	var wait sync.WaitGroup
	for range closers {
		wait.Go(func() {
			errorsSeen <- candidate.close()
		})
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent candidate close: %v", err)
		}
	}
	if candidate.retained != nil {
		t.Fatal("concurrent close left candidate pending")
	}
	if _, err := descriptor.Stat(); err == nil {
		t.Fatal("concurrent close left descriptor open")
	}
}

func TestRetainExecutableCandidateRefusesUnsafeShapes(t *testing.T) {
	base := physicalDarwinTempDir(t)
	validImage := makeThinMachOFixture(t, machOProfileGo, baseMachOCommands()...)

	noExecute := filepath.Join(base, "no-execute")
	writeExecutableBytes(t, noExecute, validImage, 0o600)
	requirePrivateCauses(t, retainExecutableCandidateError(noExecute), CausePermission)

	directory := filepath.Join(base, "directory")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("make executable-shaped directory: %v", err)
	}
	requirePrivateCauses(t, retainExecutableCandidateError(directory), CauseIdentity)

	target := filepath.Join(base, "target")
	writeExecutableBytes(t, target, validImage, 0o700)
	symlink := filepath.Join(base, "symlink")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("make executable symlink: %v", err)
	}
	requirePrivateCauses(t, retainExecutableCandidateError(symlink), CauseInvalidRequest)

	realParent := filepath.Join(base, "real-parent")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatalf("make real executable parent: %v", err)
	}
	writeExecutableBytes(t, filepath.Join(realParent, "tool"), validImage, 0o700)
	linkedParent := filepath.Join(base, "linked-parent")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatalf("make executable ancestor symlink: %v", err)
	}
	requirePrivateCauses(
		t,
		retainExecutableCandidateError(filepath.Join(linkedParent, "tool")),
		CauseInvalidRequest,
	)

	tooLarge := filepath.Join(base, "too-large")
	file, err := os.OpenFile(tooLarge, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatalf("create oversized executable: %v", err)
	}
	truncateErr := file.Truncate(int64(maxExecutableBytes) + 1)
	closeErr := file.Close()
	if truncateErr != nil || closeErr != nil {
		t.Fatalf("size oversized executable: truncate=%v close=%v", truncateErr, closeErr)
	}
	requirePrivateCauses(t, retainExecutableCandidateError(tooLarge), CauseLimit)
}

func TestRetainedExecutableDetectsContentPathAndAncestorDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string, string, []byte)
	}{
		{
			name: "content",
			mutate: func(t *testing.T, _ string, path string, image []byte) {
				t.Helper()
				changed := append([]byte(nil), image...)
				changed[len(changed)-1] ^= 0xff
				writeExistingExecutableBytes(t, path, changed)
			},
		},
		{
			name: "replacement",
			mutate: func(t *testing.T, base, path string, image []byte) {
				t.Helper()
				if err := os.Rename(path, filepath.Join(base, "former-tool")); err != nil {
					t.Fatalf("rename retained executable: %v", err)
				}
				writeExecutableBytes(t, path, image, 0o700)
			},
		},
		{
			name: "ancestor security",
			mutate: func(t *testing.T, base, _ string, _ []byte) {
				t.Helper()
				if err := os.Chmod(base, 0o722); err != nil {
					t.Fatalf("make executable ancestor unsafe: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := physicalDarwinTempDir(t)
			path := filepath.Join(base, "tool")
			image := writeMachOExecutableFixture(t, path, machOProfileGo)
			candidate, err := retainExecutableCandidate(context.Background(), path)
			if err != nil {
				t.Fatalf("retain executable candidate: %v", err)
			}
			defer func() { _ = candidate.close() }()
			test.mutate(t, base, path, image)
			if err := candidate.retained.revalidate(context.Background()); err == nil {
				t.Fatal("retained executable accepted drift")
			}
		})
	}
}

func TestExecutableProfilesAreNominalAndPinFailurePreservesCandidate(t *testing.T) {
	base := physicalDarwinTempDir(t)
	goPath := filepath.Join(base, "go")
	writeMachOExecutableFixture(t, goPath, machOProfileGo)
	goCandidate, err := retainExecutableCandidate(context.Background(), goPath)
	if err != nil {
		t.Fatalf("retain Go-shaped candidate: %v", err)
	}
	defer func() { _ = goCandidate.close() }()
	if err := validateGoExecutableProfile(context.Background(), goCandidate.retained); err != nil {
		t.Fatalf("Go profile rejected Go-shaped image: %v", err)
	}
	requirePrivateCauses(
		t,
		validateGitExecutableProfile(context.Background(), goCandidate.retained),
		CauseMalformed,
	)

	gitPath := filepath.Join(base, gitExecutableBase)
	writeMachOExecutableFixture(t, gitPath, machOProfileGit)
	gitCandidate, err := retainExecutableCandidate(context.Background(), gitPath)
	if err != nil {
		t.Fatalf("retain Git-shaped candidate: %v", err)
	}
	defer func() { _ = gitCandidate.close() }()
	if err := validateGitExecutableProfile(context.Background(), gitCandidate.retained); err != nil {
		t.Fatalf("Git profile rejected Git-shaped image: %v", err)
	}
	requirePrivateCauses(
		t,
		validateGoExecutableProfile(context.Background(), gitCandidate.retained),
		CauseMalformed,
	)
	if authority, err := admitGitAuthority(context.Background(), gitCandidate); err == nil || authority != nil {
		t.Fatal("unpinned Git-shaped image gained Git authority")
	} else {
		requirePrivateCauses(t, err, CauseUnsupported)
	}
	if gitCandidate.retained == nil {
		t.Fatal("failed Git admission consumed candidate ownership")
	}

	wrongName := filepath.Join(base, "not-git")
	writeMachOExecutableFixture(t, wrongName, machOProfileGit)
	wrongCandidate, err := retainExecutableCandidate(context.Background(), wrongName)
	if err != nil {
		t.Fatalf("retain wrong-name Git candidate: %v", err)
	}
	defer func() { _ = wrongCandidate.close() }()
	if authority, err := admitGitAuthority(context.Background(), wrongCandidate); err == nil || authority != nil {
		t.Fatal("wrong basename gained Git authority")
	} else {
		requirePrivateCauses(t, err, CauseUnsupported)
	}
	if wrongCandidate.retained == nil {
		t.Fatal("wrong-basename admission consumed candidate ownership")
	}
}

func TestGOROOTBindingAndCompilerDerivation(t *testing.T) {
	base := physicalDarwinTempDir(t)
	gorootPath := filepath.Join(base, "goroot")
	goPath := filepath.Join(gorootPath, goGOROOTRelativePath)
	compilerPath := filepath.Join(gorootPath, compilerGOROOTRelativePath)
	writeMachOExecutableFixture(t, goPath, machOProfileGo)
	writeMachOExecutableFixture(t, compilerPath, machOProfileGo)

	goroot, err := admitAuthorityTree(context.Background(), gorootPath, gorootPolicy())
	if err != nil {
		t.Fatalf("capture synthetic GOROOT: %v", err)
	}
	defer func() { _ = goroot.close() }()
	goCandidate, err := retainExecutableCandidate(context.Background(), goPath)
	if err != nil {
		t.Fatalf("retain captured Go executable: %v", err)
	}
	defer func() { _ = goCandidate.close() }()
	if err := validateGOROOTExecutableBinding(
		context.Background(),
		goroot,
		goCandidate.retained,
		goGOROOTRelativePath,
	); err != nil {
		t.Fatalf("bind captured Go executable: %v", err)
	}
	if authority, err := admitGoAuthority(context.Background(), goCandidate, goroot); err == nil || authority != nil {
		t.Fatal("unpinned Go-shaped image gained Go authority")
	} else {
		requirePrivateCauses(t, err, CauseUnsupported)
	}
	if goCandidate.retained == nil {
		t.Fatal("failed Go admission consumed candidate ownership")
	}

	compiler, err := deriveCompilerAuthority(context.Background(), goroot)
	if err != nil {
		t.Fatalf("derive compiler authority: %v", err)
	}
	if compiler.retained.leaf.path != compilerPath {
		t.Fatalf("compiler path = %q, want fixed %q", compiler.retained.leaf.path, compilerPath)
	}
	if err := compiler.revalidate(context.Background()); err != nil {
		t.Fatalf("revalidate compiler authority: %v", err)
	}
	descriptor := compiler.retained.leaf.descriptor
	if err := compiler.close(); err != nil {
		t.Fatalf("close compiler authority: %v", err)
	}
	if _, err := descriptor.Stat(); err == nil {
		t.Fatal("compiler authority close left descriptor open")
	}
}

func TestGOROOTExecutableBindingRejectsUnrelatedGoCandidate(t *testing.T) {
	base := physicalDarwinTempDir(t)
	gorootPath := filepath.Join(base, "goroot")
	writeMachOExecutableFixture(t, filepath.Join(gorootPath, goGOROOTRelativePath), machOProfileGo)
	writeMachOExecutableFixture(t, filepath.Join(gorootPath, compilerGOROOTRelativePath), machOProfileGo)
	goroot, err := admitAuthorityTree(context.Background(), gorootPath, gorootPolicy())
	if err != nil {
		t.Fatalf("capture synthetic GOROOT: %v", err)
	}
	defer func() { _ = goroot.close() }()

	unrelatedPath := filepath.Join(base, goExecutableBase)
	writeMachOExecutableFixture(t, unrelatedPath, machOProfileGo)
	candidate, err := retainExecutableCandidate(context.Background(), unrelatedPath)
	if err != nil {
		t.Fatalf("retain unrelated Go candidate: %v", err)
	}
	defer func() { _ = candidate.close() }()
	requirePrivateCauses(
		t,
		validateGOROOTExecutableBinding(
			context.Background(),
			goroot,
			candidate.retained,
			goGOROOTRelativePath,
		),
		CauseIdentity,
	)
}

func TestCompilerDerivationRefusesMissingSymlinkAndWrongProfile(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, string)
		cause CauseCode
	}{
		{
			name:  "missing",
			setup: func(*testing.T, string) {},
			cause: CauseIdentity,
		},
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				t.Helper()
				target := filepath.Join(filepath.Dir(path), "compiler-target")
				writeMachOExecutableFixture(t, target, machOProfileGo)
				if err := os.Symlink("compiler-target", path); err != nil {
					t.Fatalf("make compiler symlink: %v", err)
				}
			},
			cause: CauseIdentity,
		},
		{
			name: "Git profile",
			setup: func(t *testing.T, path string) {
				t.Helper()
				writeMachOExecutableFixture(t, path, machOProfileGit)
			},
			cause: CauseMalformed,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := physicalDarwinTempDir(t)
			gorootPath := filepath.Join(base, "goroot")
			writeMachOExecutableFixture(t, filepath.Join(gorootPath, goGOROOTRelativePath), machOProfileGo)
			compilerPath := filepath.Join(gorootPath, compilerGOROOTRelativePath)
			if err := os.MkdirAll(filepath.Dir(compilerPath), 0o700); err != nil {
				t.Fatalf("make compiler parent: %v", err)
			}
			test.setup(t, compilerPath)
			goroot, err := admitAuthorityTree(context.Background(), gorootPath, gorootPolicy())
			if err != nil {
				t.Fatalf("capture synthetic GOROOT: %v", err)
			}
			defer func() { _ = goroot.close() }()
			compiler, err := deriveCompilerAuthority(context.Background(), goroot)
			if compiler != nil || err == nil {
				if compiler != nil {
					_ = compiler.close()
				}
				t.Fatal("invalid compiler leaf gained authority")
			}
			requirePrivateCauses(t, err, test.cause)
		})
	}
}

func retainExecutableCandidateError(path string) error {
	candidate, err := retainExecutableCandidate(context.Background(), path)
	if candidate != nil {
		_ = candidate.close()
	}
	return err
}

func writeMachOExecutableFixture(t *testing.T, path string, profile machOProfile) []byte {
	t.Helper()
	image := makeThinMachOFixture(t, profile, baseMachOCommands()...)
	writeExecutableBytes(t, path, image, 0o700)
	return image
}

func writeExecutableBytes(t *testing.T, path string, contents []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("make executable fixture parent: %v", err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		t.Fatalf("write executable fixture: %v", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("set executable fixture mode: %v", err)
	}
}

func writeExistingExecutableBytes(t *testing.T, path string, contents []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		t.Fatalf("open retained executable for mutation: %v", err)
	}
	written, writeErr := file.Write(contents)
	closeErr := file.Close()
	if writeErr != nil || written != len(contents) || closeErr != nil {
		t.Fatalf("mutate retained executable: written=%d write=%v close=%v", written, writeErr, closeErr)
	}
}
