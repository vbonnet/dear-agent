//go:build darwin && arm64

package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestInvalidateDarwinSourceFD(t *testing.T) {
	t.Parallel()

	if invalidateDarwinSourceFD(nil) {
		t.Fatal("nil raw descriptor slot retired successfully")
	}
	invalid := -1
	if invalidateDarwinSourceFD(&invalid) {
		t.Fatal("invalid raw descriptor slot retired successfully")
	}
	if invalid != -1 {
		t.Fatalf("invalid raw descriptor slot = %d, want -1", invalid)
	}
	moreInvalid := -2
	if invalidateDarwinSourceFD(&moreInvalid) {
		t.Fatal("raw descriptor slot below -1 retired successfully")
	}
	if moreInvalid != -2 {
		t.Fatalf("raw descriptor slot below -1 = %d, want -2", moreInvalid)
	}
	zero := 0
	if !invalidateDarwinSourceFD(&zero) {
		t.Fatal("zero raw descriptor slot was not retired")
	}
	if zero != -1 {
		t.Fatalf("retired zero raw descriptor slot = %d, want -1", zero)
	}
	valid := 17
	if !invalidateDarwinSourceFD(&valid) {
		t.Fatal("valid raw descriptor slot was not retired")
	}
	if valid != -1 {
		t.Fatalf("retired raw descriptor slot = %d, want -1", valid)
	}
}

func TestDarwinOpenFlagsExactMatrix(t *testing.T) {
	t.Parallel()

	common := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK
	for _, test := range []struct {
		name  string
		kind  entryKind
		extra int
	}{
		{name: "directory", kind: entryDirectory, extra: unix.O_DIRECTORY},
		{name: "regular", kind: entryRegular},
		{name: "symlink", kind: entrySymlink, extra: unix.O_SYMLINK},
	} {
		for _, noFollowAny := range []bool{false, true} {
			name := test.name + "/follow"
			want := common | test.extra
			if noFollowAny {
				name = test.name + "/no-follow-any"
				want |= unix.O_NOFOLLOW_ANY
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				got, err := darwinOpenFlags(test.kind, noFollowAny)
				if err != nil || got != want {
					t.Fatalf("darwin open flags = %#x / %v, want %#x / nil", got, err, want)
				}
			})
		}
	}
	for _, kind := range []entryKind{0, ^entryKind(0)} {
		for _, noFollowAny := range []bool{false, true} {
			flags, err := darwinOpenFlags(kind, noFollowAny)
			if !errors.Is(err, fs.ErrInvalid) || flags != 0 {
				t.Fatalf("invalid kind %d flags = %#x / %v, want 0 / fs.ErrInvalid", kind, flags, err)
			}
		}
	}
}

func TestDarwinSourcePrimitivesProductionFactoryAndOwnedRoots(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	if _, ok := primitives.(darwinSourcePrimitives); !ok {
		t.Fatalf("production source primitives = %T, want darwinSourcePrimitives", primitives)
	}

	repositoryPath := physicalDarwinTempDir(t)
	if err := os.Mkdir(filepath.Join(repositoryPath, "child"), 0o700); err != nil {
		t.Fatalf("make child root fixture: %v", err)
	}
	repository, failure := primitives.openRepositoryRoot(
		context.Background(),
		sourceRepositoryLocator{path: repositoryPath, seal: validSourceRepositoryLocator},
	)
	if failure != nil || repository == nil || !repository.validOpen() {
		t.Fatalf("open repository root = %+v / %+v", repository, failure)
	}
	defer primitives.closeRoot(repository)
	repositoryHandle := repository.root

	child, failure := primitives.openChildRoot(context.Background(), repository, "child")
	if failure != nil || child == nil || !child.validOpen() {
		t.Fatalf("open child root = %+v / %+v", child, failure)
	}
	defer primitives.closeRoot(child)
	childHandle := child.root
	if failure := primitives.closeRoot(child); failure {
		t.Fatal("first child-root close failed")
	}
	if failure := primitives.closeRoot(child); failure {
		t.Fatal("repeated child-root close did not cache success")
	}
	if _, err := childHandle.Lstat("."); err == nil {
		t.Fatal("closed child-root owner left its os.Root usable")
	}
	if _, err := repositoryHandle.Lstat("."); err != nil {
		t.Fatalf("closing child root closed borrowed repository root: %v", err)
	}

	physicalRoot, failure := primitives.openPhysicalRootDescriptor(context.Background())
	if failure != nil || physicalRoot == nil || !physicalRoot.validOpen() ||
		physicalRoot.kind != sourceObservedDirectory {
		t.Fatalf("open physical-root descriptor = %+v / %+v", physicalRoot, failure)
	}
	defer primitives.closeDescriptor(physicalRoot)
	physicalRootHandle := physicalRoot.file
	physicalInfo, err := physicalRootHandle.Stat()
	if err != nil || !physicalInfo.IsDir() {
		t.Fatalf("physical-root descriptor stat = %+v / %v", physicalInfo, err)
	}
	if failure := primitives.closeDescriptor(physicalRoot); failure {
		t.Fatal("first physical-root descriptor close failed")
	}
	if failure := primitives.closeDescriptor(physicalRoot); failure {
		t.Fatal("repeated physical-root descriptor close did not cache success")
	}
	if _, err := physicalRootHandle.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed physical-root descriptor remains usable: %v", err)
	}

	if failure := primitives.closeRoot(repository); failure {
		t.Fatal("first repository-root close failed")
	}
	if failure := primitives.closeRoot(repository); failure {
		t.Fatal("repeated repository-root close did not cache success")
	}
	if _, err := repositoryHandle.Lstat("."); err == nil {
		t.Fatal("closed repository-root owner left its os.Root usable")
	}
}

func TestDarwinSourcePrimitivesOpenFreshRootDirectoryDescriptor(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	base := physicalDarwinTempDir(t)
	originalPath := filepath.Join(base, "retained")
	movedPath := filepath.Join(base, "moved")
	if err := os.Mkdir(originalPath, 0o700); err != nil {
		t.Fatalf("make retained directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalPath, "original"), []byte("old"), 0o600); err != nil {
		t.Fatalf("write retained entry: %v", err)
	}
	root, failure := primitives.openRepositoryRoot(
		context.Background(),
		sourceRepositoryLocator{path: originalPath, seal: validSourceRepositoryLocator},
	)
	if failure != nil || root == nil {
		t.Fatalf("open retained root = %+v / %+v", root, failure)
	}
	t.Cleanup(func() { _ = primitives.closeRoot(root) })

	if err := os.Rename(originalPath, movedPath); err != nil {
		t.Fatalf("rename retained directory: %v", err)
	}
	if err := os.Mkdir(originalPath, 0o700); err != nil {
		t.Fatalf("make pathname replacement: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originalPath, "replacement"), []byte("new"), 0o600); err != nil {
		t.Fatalf("write pathname replacement: %v", err)
	}

	first, failure := primitives.openRootDirectoryDescriptor(context.Background(), root)
	if failure != nil || first == nil || !first.validOpen() || first.kind != sourceObservedDirectory {
		t.Fatalf("first root-directory descriptor = %+v / %+v", first, failure)
	}
	firstNames := readAllDarwinSourceDirectoryNames(t, primitives, first)
	if failure := primitives.closeDescriptor(first); failure {
		t.Fatal("close first root-directory descriptor failed")
	}
	if failure := primitives.closeDescriptor(first); failure {
		t.Fatal("repeated first descriptor close did not cache success")
	}

	second, failure := primitives.openRootDirectoryDescriptor(context.Background(), root)
	if failure != nil || second == nil || !second.validOpen() || second.kind != sourceObservedDirectory {
		t.Fatalf("second root-directory descriptor = %+v / %+v", second, failure)
	}
	secondNames := readAllDarwinSourceDirectoryNames(t, primitives, second)
	if failure := primitives.closeDescriptor(second); failure {
		t.Fatal("close second root-directory descriptor failed")
	}

	for index, names := range [][]string{firstNames, secondNames} {
		if len(names) != 1 || names[0] != "original" {
			t.Fatalf("scan %d names = %q, want retained original only", index+1, names)
		}
	}
	if !root.validOpen() {
		t.Fatal("closing scan descriptors closed the borrowed root")
	}
}

func TestDarwinSourcePrimitivesOpenRootDirectoryDescriptorContextOwnership(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	path := physicalDarwinTempDir(t)
	root, failure := primitives.openRepositoryRoot(
		context.Background(),
		sourceRepositoryLocator{path: path, seal: validSourceRepositoryLocator},
	)
	if failure != nil || root == nil {
		t.Fatalf("open context-test root = %+v / %+v", root, failure)
	}
	t.Cleanup(func() { _ = primitives.closeRoot(root) })

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	owner, failure := primitives.openRootDirectoryDescriptor(canceled, root)
	if owner != nil {
		_ = primitives.closeDescriptor(owner)
		t.Fatal("pre-canceled root-directory open returned an owner")
	}
	requireDarwinSourceFailure(t, failure, OperationOpen, CauseCanceled)

	for _, test := range []struct {
		name      string
		cancelAt  int
		operation Operation
	}{
		{name: "post-open", cancelAt: 2, operation: OperationOpen},
		{name: "pre-probe", cancelAt: 3, operation: OperationProbe},
		{name: "post-probe", cancelAt: 4, operation: OperationProbe},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &darwinSourcePrimitiveStepContext{
				cancelAt: test.cancelAt,
				err:      context.Canceled,
			}
			owner, failure := primitives.openRootDirectoryDescriptor(ctx, root)
			if owner == nil || !owner.validOpen() {
				t.Fatalf("context failure did not retain descriptor owner: %+v", owner)
			}
			requireDarwinSourceFailure(t, failure, test.operation, CauseCanceled)
			if closeFailure := primitives.closeDescriptor(owner); closeFailure {
				t.Fatal("close retained failure owner failed")
			}
		})
	}

	for _, invalid := range []*ownedSourceRoot{nil, {}, {state: sourceHandleClosed}} {
		owner, failure := primitives.openRootDirectoryDescriptor(context.Background(), invalid)
		if owner != nil {
			_ = primitives.closeDescriptor(owner)
			t.Fatal("invalid root owner acquired a descriptor")
		}
		requireDarwinSourceFailure(t, failure, OperationValidate, CauseInternalInvariant)
	}
}

func TestDarwinSourcePrimitivesReadDirectoryBatchContextAndOwnership(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	directory := physicalDarwinTempDir(t)
	if err := os.WriteFile(filepath.Join(directory, "first"), nil, 0o600); err != nil {
		t.Fatalf("write directory entry: %v", err)
	}
	owner := newDarwinSourceTestDirectoryOwner(t, primitives, directory)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	names, done, failure := primitives.readDirectoryBatch(canceled, owner)
	if names != nil || done {
		t.Fatalf("pre-canceled batch = %q / done %v", names, done)
	}
	requireDarwinSourceFailure(t, failure, OperationWalk, CauseCanceled)
	if !owner.validOpen() {
		t.Fatal("pre-canceled read closed its borrowed descriptor")
	}
	if got := readAllDarwinSourceDirectoryNames(t, primitives, owner); len(got) != 1 || got[0] != "first" {
		t.Fatalf("pre-canceled read advanced directory cursor: %q", got)
	}

	postOwner := newDarwinSourceTestDirectoryOwner(t, primitives, directory)
	postContext := &darwinSourcePrimitiveStepContext{cancelAt: 2, err: context.Canceled}
	names, done, failure = primitives.readDirectoryBatch(postContext, postOwner)
	if names != nil || done {
		t.Fatalf("post-read cancellation leaked batch = %q / done %v", names, done)
	}
	requireDarwinSourceFailure(t, failure, OperationWalk, CauseCanceled)
	if !postOwner.validOpen() {
		t.Fatal("post-read cancellation closed its borrowed descriptor")
	}

	for _, invalid := range []*ownedSourceDescriptor{
		nil,
		{},
		{file: &os.File{}, kind: sourceObservedRegular, state: sourceHandleOpen},
		{kind: sourceObservedDirectory, state: sourceHandleClosed},
	} {
		names, done, failure = primitives.readDirectoryBatch(context.Background(), invalid)
		if names != nil || done {
			t.Fatalf("invalid-owner batch = %q / done %v", names, done)
		}
		requireDarwinSourceFailure(t, failure, OperationValidate, CauseInternalInvariant)
	}
}

func TestNormalizeDarwinSourceDirectoryBatchRawNamesAndBounds(t *testing.T) {
	rawNames := []string{
		"plain",
		"name\\with-backslash",
		"name\nwith-newline",
		string([]byte{'r', 'a', 'w', '-', 0xff}),
		strings.Repeat("x", maxPathComponentBytes),
	}
	entries := make([]os.DirEntry, len(rawNames))
	for index, name := range rawNames {
		entries[index] = nameOnlyDarwinSourceDirEntry{name: name}
	}
	names, done, failure := normalizeDarwinSourceDirectoryBatch(
		context.Background(), entries, io.EOF,
	)
	if failure != nil || !done {
		t.Fatalf("terminal raw-name batch = %q / %v / %+v", names, done, failure)
	}
	requireDarwinSourceNames(t, names, rawNames)

	exact := make([]os.DirEntry, sourceDirectoryReadBatchSize)
	for index := range exact {
		exact[index] = nameOnlyDarwinSourceDirEntry{name: "entry"}
	}
	names, done, failure = normalizeDarwinSourceDirectoryBatch(context.Background(), exact, nil)
	if failure != nil || done || len(names) != sourceDirectoryReadBatchSize {
		t.Fatalf("exact batch = %d / done %v / %+v", len(names), done, failure)
	}

	tooMany := append(append([]os.DirEntry(nil), exact...), nameOnlyDarwinSourceDirEntry{name: "overflow"})
	names, done, failure = normalizeDarwinSourceDirectoryBatch(context.Background(), tooMany, nil)
	if names != nil || done {
		t.Fatalf("over-limit batch leaked names = %d / done %v", len(names), done)
	}
	requireDarwinSourceFailure(t, failure, OperationWalk, CauseLimit)

	names, done, failure = normalizeDarwinSourceDirectoryBatch(context.Background(), nil, nil)
	if names != nil || done {
		t.Fatalf("empty nonterminal batch = %q / done %v", names, done)
	}
	requireDarwinSourceFailure(t, failure, OperationWalk, CauseUnstable)

	names, done, failure = normalizeDarwinSourceDirectoryBatch(context.Background(), nil, io.EOF)
	if failure != nil || !done || len(names) != 0 {
		t.Fatalf("empty terminal batch = %q / done %v / %+v", names, done, failure)
	}
}

func TestNormalizeDarwinSourceDirectoryBatchFailuresWithholdNames(t *testing.T) {
	partial := []os.DirEntry{nameOnlyDarwinSourceDirEntry{name: "partial"}}
	for _, test := range []struct {
		name  string
		err   error
		cause CauseCode
	}{
		{name: "permission", err: fs.ErrPermission, cause: CausePermission},
		{name: "canceled", err: context.Canceled, cause: CauseCanceled},
		{name: "deadline", err: context.DeadlineExceeded, cause: CauseDeadline},
		{name: "unstable", err: errors.New("directory I/O failed"), cause: CauseUnstable},
	} {
		t.Run(test.name, func(t *testing.T) {
			names, done, failure := normalizeDarwinSourceDirectoryBatch(
				context.Background(), partial, test.err,
			)
			if names != nil || done {
				t.Fatalf("failed batch exposed partial names = %q / done %v", names, done)
			}
			requireDarwinSourceFailure(t, failure, OperationWalk, test.cause)
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	names, done, failure := normalizeDarwinSourceDirectoryBatch(canceled, partial, fs.ErrPermission)
	if names != nil || done {
		t.Fatalf("context-priority batch = %q / done %v", names, done)
	}
	requireDarwinSourceFailure(t, failure, OperationWalk, CauseCanceled)
}

func TestNormalizeDarwinSourceDirectoryBatchRejectsInvalidRecords(t *testing.T) {
	for _, test := range []struct {
		name      string
		entry     os.DirEntry
		operation Operation
		cause     CauseCode
	}{
		{name: "nil entry", entry: nil, operation: OperationValidate, cause: CauseInternalInvariant},
		{name: "empty name", entry: nameOnlyDarwinSourceDirEntry{name: ""}, operation: OperationWalk, cause: CauseUnstable},
		{name: "dot name", entry: nameOnlyDarwinSourceDirEntry{name: "."}, operation: OperationWalk, cause: CauseUnstable},
		{name: "dot-dot name", entry: nameOnlyDarwinSourceDirEntry{name: ".."}, operation: OperationWalk, cause: CauseUnstable},
		{name: "slash name", entry: nameOnlyDarwinSourceDirEntry{name: "left/right"}, operation: OperationWalk, cause: CauseUnstable},
		{name: "nul name", entry: nameOnlyDarwinSourceDirEntry{name: "left\x00right"}, operation: OperationWalk, cause: CauseUnstable},
		{
			name: "long name", entry: nameOnlyDarwinSourceDirEntry{name: strings.Repeat("x", maxPathComponentBytes+1)},
			operation: OperationWalk, cause: CauseLimit,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			names, done, failure := normalizeDarwinSourceDirectoryBatch(
				context.Background(), []os.DirEntry{test.entry}, io.EOF,
			)
			if names != nil || done {
				t.Fatalf("invalid record exposed batch = %q / done %v", names, done)
			}
			requireDarwinSourceFailure(t, failure, test.operation, test.cause)
		})
	}
}

func TestDarwinSourcePrimitivesProbePresenceMatrix(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	directory := physicalDarwinTempDir(t)
	if err := os.Mkdir(filepath.Join(directory, "directory"), 0o700); err != nil {
		t.Fatalf("make directory fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "regular"), []byte("content"), 0o600); err != nil {
		t.Fatalf("write regular fixture: %v", err)
	}
	if err := os.Symlink("regular", filepath.Join(directory, "symlink")); err != nil {
		t.Fatalf("make symlink fixture: %v", err)
	}
	if err := unix.Mkfifo(filepath.Join(directory, "fifo"), 0o600); err != nil {
		t.Fatalf("make FIFO fixture: %v", err)
	}
	parent := newDarwinSourceTestDirectoryOwner(t, primitives, directory)

	entries := []struct {
		name string
		kind sourceObservedKind
	}{
		{name: "directory", kind: sourceObservedDirectory},
		{name: "regular", kind: sourceObservedRegular},
		{name: "symlink", kind: sourceObservedSymlink},
		{name: "fifo", kind: sourceObservedSpecial},
	}
	modes := []struct {
		name      string
		mode      sourcePresenceMode
		wantKind  bool
		operation Operation
		cause     CauseCode
	}{
		{name: "initial-required", mode: sourceInitialRequired, wantKind: true},
		{name: "initial-optional", mode: sourceInitialOptional, wantKind: true},
		{
			name: "initial-forbidden", mode: sourceInitialForbidden, wantKind: true,
			operation: OperationValidate, cause: CauseUnsupported,
		},
		{name: "initial-walk-present", mode: sourceInitialWalkPresent, wantKind: true},
		{name: "revalidate-present", mode: sourceRevalidatePresent, wantKind: true},
		{
			name: "revalidate-absent", mode: sourceRevalidateAbsent,
			operation: OperationCompare, cause: CauseUnstable,
		},
	}
	for _, entry := range entries {
		for _, mode := range modes {
			t.Run(entry.name+"/"+mode.name, func(t *testing.T) {
				kind, present, failure := primitives.probeRelativeKind(
					context.Background(), parent, entry.name, mode.mode,
				)
				if !present {
					t.Fatal("present fixture was reported absent")
				}
				wantKind := sourceObservedKind(0)
				if mode.wantKind {
					wantKind = entry.kind
				}
				if kind != wantKind {
					t.Fatalf("observed kind = %d, want %d", kind, wantKind)
				}
				if mode.cause == "" {
					if failure != nil {
						t.Fatalf("presence probe failed: %+v", failure)
					}
					return
				}
				requireDarwinSourceFailure(t, failure, mode.operation, mode.cause)
			})
		}
	}

	for _, test := range []struct {
		name      string
		mode      sourcePresenceMode
		operation Operation
		cause     CauseCode
	}{
		{name: "initial-required", mode: sourceInitialRequired, operation: OperationOpen, cause: CauseNotFound},
		{name: "initial-optional", mode: sourceInitialOptional},
		{name: "initial-forbidden", mode: sourceInitialForbidden},
		{
			name: "initial-walk-present", mode: sourceInitialWalkPresent,
			operation: OperationProbe, cause: CauseUnstable,
		},
		{name: "revalidate-present", mode: sourceRevalidatePresent, operation: OperationCompare, cause: CauseUnstable},
		{name: "revalidate-absent", mode: sourceRevalidateAbsent},
	} {
		t.Run("absent/"+test.name, func(t *testing.T) {
			kind, present, failure := primitives.probeRelativeKind(
				context.Background(), parent, "absent", test.mode,
			)
			if kind != 0 || present {
				t.Fatalf("absent probe = kind %d present %v", kind, present)
			}
			if test.cause == "" {
				if failure != nil {
					t.Fatalf("allowed absence failed: %+v", failure)
				}
				return
			}
			requireDarwinSourceFailure(t, failure, test.operation, test.cause)
		})
	}
}

func TestDarwinSourcePrimitivesOpenNoFollowKindsAndOwnership(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	directory := physicalDarwinTempDir(t)
	if err := os.Mkdir(filepath.Join(directory, "directory"), 0o700); err != nil {
		t.Fatalf("make directory fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "regular"), []byte("content"), 0o600); err != nil {
		t.Fatalf("write regular fixture: %v", err)
	}
	if err := os.Symlink("regular", filepath.Join(directory, "symlink")); err != nil {
		t.Fatalf("make symlink fixture: %v", err)
	}
	parent := newDarwinSourceTestDirectoryOwner(t, primitives, directory)

	for _, entry := range []struct {
		name string
		kind sourceObservedKind
	}{
		{name: "directory", kind: sourceObservedDirectory},
		{name: "regular", kind: sourceObservedRegular},
		{name: "symlink", kind: sourceObservedSymlink},
	} {
		for _, mode := range []sourcePresenceMode{
			sourceInitialRequired,
			sourceInitialOptional,
			sourceInitialWalkPresent,
			sourceRevalidatePresent,
		} {
			t.Run(entry.name+"/"+sourcePresenceModeName(mode), func(t *testing.T) {
				owner, failure := primitives.openRelativeNoFollow(
					context.Background(), parent, entry.name, entry.kind, mode,
				)
				if owner != nil {
					defer primitives.closeDescriptor(owner)
				}
				if failure != nil || owner == nil || !owner.validOpen() || owner.kind != entry.kind {
					t.Fatalf("open no-follow = %+v / %+v", owner, failure)
				}
				snapshot, statFailure := primitives.statDescriptor(context.Background(), owner)
				if statFailure != nil || observedSourceKind(snapshotKind(snapshot)) != entry.kind {
					t.Fatalf("opened kind snapshot = %+v / %+v", snapshot, statFailure)
				}
				if closeFailed := primitives.closeDescriptor(owner); closeFailed {
					t.Fatal("close opened descriptor failed")
				}
				if closeFailed := primitives.closeDescriptor(owner); closeFailed {
					t.Fatal("repeated opened-descriptor close did not cache success")
				}
			})
		}
	}

	for _, test := range []struct {
		name      string
		entryName string
		kind      sourceObservedKind
		mode      sourcePresenceMode
		owner     bool
		operation Operation
		cause     CauseCode
	}{
		{
			name: "directory observed as regular initial", entryName: "directory",
			kind: sourceObservedRegular, mode: sourceInitialRequired, owner: true,
			operation: OperationOpen, cause: CauseIdentity,
		},
		{
			name: "directory observed as regular revalidation", entryName: "directory",
			kind: sourceObservedRegular, mode: sourceRevalidatePresent, owner: true,
			operation: OperationCompare, cause: CauseIdentity,
		},
		{
			name: "regular observed as directory initial", entryName: "regular",
			kind: sourceObservedDirectory, mode: sourceInitialRequired,
			operation: OperationOpen, cause: CauseIdentity,
		},
		{
			name: "regular observed as directory revalidation", entryName: "regular",
			kind: sourceObservedDirectory, mode: sourceRevalidatePresent,
			operation: OperationCompare, cause: CauseIdentity,
		},
		{
			name: "symlink observed as regular", entryName: "symlink",
			kind: sourceObservedRegular, mode: sourceInitialRequired,
			operation: OperationOpen, cause: CauseIdentity,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, failure := primitives.openRelativeNoFollow(
				context.Background(), parent, test.entryName, test.kind, test.mode,
			)
			if owner != nil {
				defer primitives.closeDescriptor(owner)
			}
			if (owner != nil) != test.owner {
				t.Fatalf("mismatch owner present = %v, want %v", owner != nil, test.owner)
			}
			requireDarwinSourceFailure(t, failure, test.operation, test.cause)
		})
	}

	ctx := &darwinSourcePrimitiveStepContext{cancelAt: 2, err: context.Canceled}
	owner, failure := primitives.openRelativeNoFollow(
		ctx, parent, "regular", sourceObservedRegular, sourceInitialRequired,
	)
	if owner != nil {
		defer primitives.closeDescriptor(owner)
	}
	if ctx.samples != 2 || owner == nil || !owner.validOpen() {
		t.Fatalf("post-open cancellation = calls %d owner %+v failure %+v", ctx.samples, owner, failure)
	}
	requireDarwinSourceFailure(t, failure, OperationOpen, CauseCanceled)
}

func TestDarwinSourcePrimitivesCompareRootAndDescriptor(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	directory := physicalDarwinTempDir(t)
	other := filepath.Join(directory, "other")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatalf("make mismatched directory: %v", err)
	}
	root, failure := primitives.openRepositoryRoot(
		context.Background(),
		sourceRepositoryLocator{path: directory, seal: validSourceRepositoryLocator},
	)
	if root != nil {
		defer primitives.closeRoot(root)
	}
	if failure != nil || root == nil {
		t.Fatalf("open comparison root = %+v / %+v", root, failure)
	}
	matching := newDarwinSourceTestDirectoryOwner(t, primitives, directory)
	mismatching := newDarwinSourceTestDirectoryOwner(t, primitives, other)
	if failure := primitives.compareRootAndDescriptor(context.Background(), root, matching); failure != nil {
		t.Fatalf("matching root and descriptor failed: %+v", failure)
	}
	requireDarwinSourceFailure(
		t,
		primitives.compareRootAndDescriptor(context.Background(), root, mismatching),
		OperationCompare,
		CauseIdentity,
	)
}

func TestDarwinSourcePrimitivesDescriptorEvidenceAndBytes(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	directory := physicalDarwinTempDir(t)
	content := []byte("source primitive bytes\n")
	if err := os.WriteFile(filepath.Join(directory, "config"), content, 0o600); err != nil {
		t.Fatalf("write source byte fixture: %v", err)
	}
	parent := newDarwinSourceTestDirectoryOwner(t, primitives, directory)
	descriptor, failure := primitives.openRelativeNoFollow(
		context.Background(),
		parent,
		"config",
		sourceObservedRegular,
		sourceInitialRequired,
	)
	if descriptor != nil {
		defer primitives.closeDescriptor(descriptor)
	}
	if failure != nil || descriptor == nil {
		t.Fatalf("open byte fixture = %+v / %+v", descriptor, failure)
	}

	snapshot, failure := primitives.statDescriptor(context.Background(), descriptor)
	if failure != nil || snapshotKind(snapshot) != entryRegular ||
		snapshot.identity.Inode == 0 || snapshot.identity.UID != uint32(os.Geteuid()) ||
		snapshot.size != int64(len(content)) {
		t.Fatalf("source descriptor snapshot = %+v / %+v", snapshot, failure)
	}
	mount, failure := primitives.statFilesystem(context.Background(), descriptor)
	if failure != nil {
		t.Fatalf("source filesystem snapshot = %+v / %+v", mount, failure)
	}
	if failure := primitives.validateFilesystem(context.Background(), mount); failure != nil {
		t.Fatalf("source filesystem validation = %+v", failure)
	}
	if mount.flags&unix.MNT_LOCAL == 0 || mount.flags&unix.MNT_IGNORE_OWNERSHIP != 0 {
		t.Fatalf("source mount flags = %#x", mount.flags)
	}

	rawACL, failure := primitives.acquireRawACL(context.Background(), descriptor)
	if failure != nil || len(rawACL) != darwinACLAttributeBufferSize {
		t.Fatalf("source raw ACL = %d bytes / %+v", len(rawACL), failure)
	}
	acl, failure := primitives.parseRawACL(context.Background(), rawACL)
	wantACL, aclErr := digestExtendedSecurityResult(rawACL)
	if failure != nil || aclErr != nil || acl.digest != wantACL ||
		acl.disposition != sourceACLAdmitted {
		t.Fatalf("source ACL = %+v / %+v, want digest %x / %v", acl, failure, wantACL, aclErr)
	}
	if failure := primitives.validateACL(context.Background(), acl); failure != nil {
		t.Fatalf("source ACL policy validation = %+v", failure)
	}

	read := make([]byte, len(content))
	if failure := primitives.readExactForParse(context.Background(), descriptor, read); failure != nil ||
		!bytes.Equal(read, content) {
		t.Fatalf("source exact read = %q / %+v", read, failure)
	}
	tooLong := make([]byte, len(content)+1)
	requireDarwinSourceFailure(
		t,
		primitives.readExactForParse(context.Background(), descriptor, tooLong),
		OperationParse,
		CauseUnstable,
	)
	digest, failure := primitives.hashBytes(context.Background(), read)
	wantDigest := Digest(sha256.Sum256(content))
	if failure != nil || digest != wantDigest {
		t.Fatalf("source byte digest = %x / %+v, want %x", digest, failure, wantDigest)
	}
	hashContext := &darwinSourcePrimitiveStepContext{cancelAt: 2, err: context.Canceled}
	canceledDigest, canceledFailure := primitives.hashBytes(hashContext, read)
	if canceledDigest != (Digest{}) {
		t.Fatalf("canceled source byte digest = %x, want zero", canceledDigest)
	}
	requireDarwinSourceFailure(t, canceledFailure, OperationHash, CauseCanceled)
	if hashContext.samples != 2 {
		t.Fatalf("canceled source hash context samples = %d, want 2", hashContext.samples)
	}

	for _, flags := range []uint32{0, unix.MNT_LOCAL | unix.MNT_IGNORE_OWNERSHIP} {
		requireDarwinSourceFailure(
			t,
			primitives.validateFilesystem(context.Background(), mountSnapshot{flags: flags}),
			OperationValidate,
			CauseUnsupported,
		)
	}
	requireDarwinSourceFailure(
		t,
		func() *sourcePrimitiveFailure {
			_, malformed := primitives.parseRawACL(context.Background(), nil)
			return malformed
		}(),
		OperationParse,
		CauseMalformed,
	)
}

func TestDarwinSourcePrimitivesDeferACLPolicyUntilValidation(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	raw := makeDarwinTestAttrResult(makeDarwinTestFilesec(
		1,
		0,
		[]darwinTestACE{{flags: darwinACEPermit, rights: darwinMutationRights}},
	))
	acl, failure := primitives.parseRawACL(context.Background(), raw)
	if failure != nil || acl.digest == (Digest{}) ||
		acl.disposition != sourceACLMutationPermitting {
		t.Fatalf("well-formed disallowed ACL parse = %+v / %+v", acl, failure)
	}
	requireDarwinSourceFailure(
		t,
		primitives.validateACL(context.Background(), acl),
		OperationValidate,
		CausePermission,
	)
}

func TestDarwinSourcePrimitivesContextAttribution(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	directory := physicalDarwinTempDir(t)
	if err := os.Mkdir(filepath.Join(directory, "child"), 0o700); err != nil {
		t.Fatalf("make context child fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "regular"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write context file fixture: %v", err)
	}
	parent := newDarwinSourceTestDirectoryOwner(t, primitives, directory)
	root, failure := primitives.openRepositoryRoot(
		context.Background(),
		sourceRepositoryLocator{path: directory, seal: validSourceRepositoryLocator},
	)
	if root != nil {
		defer primitives.closeRoot(root)
	}
	if failure != nil || root == nil {
		t.Fatalf("open context root = %+v / %+v", root, failure)
	}
	regular, failure := primitives.openRelativeNoFollow(
		context.Background(), parent, "regular", sourceObservedRegular, sourceInitialRequired,
	)
	if regular != nil {
		defer primitives.closeDescriptor(regular)
	}
	if failure != nil || regular == nil {
		t.Fatalf("open context descriptor = %+v / %+v", regular, failure)
	}

	for _, termination := range []struct {
		name    string
		context func() (context.Context, context.CancelFunc)
		cause   CauseCode
	}{
		{
			name: "canceled",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, func() {}
			},
			cause: CauseCanceled,
		},
		{
			name: "deadline",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Unix(1, 0))
			},
			cause: CauseDeadline,
		},
	} {
		t.Run(termination.name, func(t *testing.T) {
			for _, primitive := range []struct {
				name      string
				operation Operation
				invoke    func(context.Context) *sourcePrimitiveFailure
			}{
				{
					name: "repository root", operation: OperationOpen,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						owner, failed := primitives.openRepositoryRoot(
							ctx,
							sourceRepositoryLocator{path: directory, seal: validSourceRepositoryLocator},
						)
						if owner != nil {
							_ = primitives.closeRoot(owner)
						}
						return failed
					},
				},
				{
					name: "physical root", operation: OperationOpen,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						owner, failed := primitives.openPhysicalRootDescriptor(ctx)
						if owner != nil {
							_ = primitives.closeDescriptor(owner)
						}
						return failed
					},
				},
				{
					name: "presence", operation: OperationProbe,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						_, _, failed := primitives.probeRelativeKind(ctx, parent, "regular", sourceInitialRequired)
						return failed
					},
				},
				{
					name: "relative open", operation: OperationOpen,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						owner, failed := primitives.openRelativeNoFollow(
							ctx, parent, "regular", sourceObservedRegular, sourceInitialRequired,
						)
						if owner != nil {
							_ = primitives.closeDescriptor(owner)
						}
						return failed
					},
				},
				{
					name: "child root", operation: OperationOpen,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						owner, failed := primitives.openChildRoot(ctx, root, "child")
						if owner != nil {
							_ = primitives.closeRoot(owner)
						}
						return failed
					},
				},
				{
					name: "descriptor stat", operation: OperationProbe,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						_, failed := primitives.statDescriptor(ctx, regular)
						return failed
					},
				},
				{
					name: "filesystem stat", operation: OperationProbe,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						_, failed := primitives.statFilesystem(ctx, regular)
						return failed
					},
				},
				{
					name: "filesystem validation", operation: OperationValidate,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						return primitives.validateFilesystem(ctx, mountSnapshot{})
					},
				},
				{
					name: "ACL acquisition", operation: OperationProbe,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						_, failed := primitives.acquireRawACL(ctx, regular)
						return failed
					},
				},
				{
					name: "ACL parse", operation: OperationParse,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						_, failed := primitives.parseRawACL(ctx, nil)
						return failed
					},
				},
				{
					name: "ACL validation", operation: OperationValidate,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						return primitives.validateACL(ctx, parsedSourceACL{disposition: sourceACLAdmitted})
					},
				},
				{
					name: "exact read", operation: OperationParse,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						return primitives.readExactForParse(ctx, regular, []byte{0})
					},
				},
				{
					name: "hash", operation: OperationHash,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						_, failed := primitives.hashBytes(ctx, nil)
						return failed
					},
				},
				{
					name: "root comparison starts with probe", operation: OperationProbe,
					invoke: func(ctx context.Context) *sourcePrimitiveFailure {
						return primitives.compareRootAndDescriptor(ctx, root, parent)
					},
				},
			} {
				t.Run(primitive.name, func(t *testing.T) {
					ctx, cancel := termination.context()
					defer cancel()
					requireDarwinSourceFailure(t, primitive.invoke(ctx), primitive.operation, termination.cause)
				})
			}
		})
	}
}

func TestDarwinSourcePrimitiveInputAndLookupAttributionIsClosed(t *testing.T) {
	primitives := requireDarwinSourcePrimitives(t)
	parent := newDarwinSourceTestDirectoryOwner(t, primitives, physicalDarwinTempDir(t))

	for _, name := range []string{"", ".", "..", "nested/name", "name\x00suffix"} {
		_, _, failure := primitives.probeRelativeKind(
			context.Background(), parent, name, sourceInitialOptional,
		)
		requireDarwinSourceFailure(t, failure, OperationValidate, CauseInternalInvariant)
	}
	for _, test := range []struct {
		name      string
		mode      sourcePresenceMode
		err       error
		operation Operation
		cause     CauseCode
	}{
		{
			name: "required absence", mode: sourceInitialRequired, err: fs.ErrNotExist,
			operation: OperationOpen, cause: CauseNotFound,
		},
		{
			name: "optional disappearance", mode: sourceInitialOptional, err: fs.ErrNotExist,
			operation: OperationProbe, cause: CauseUnstable,
		},
		{
			name: "walk-record disappearance", mode: sourceInitialWalkPresent, err: fs.ErrNotExist,
			operation: OperationOpen, cause: CauseUnstable,
		},
		{
			name: "revalidation disappearance", mode: sourceRevalidatePresent, err: fs.ErrNotExist,
			operation: OperationCompare, cause: CauseUnstable,
		},
		{
			name: "initial kind mismatch", mode: sourceInitialRequired, err: unix.ELOOP,
			operation: OperationOpen, cause: CauseIdentity,
		},
		{
			name: "revalidation kind mismatch", mode: sourceRevalidatePresent, err: unix.ENOTDIR,
			operation: OperationCompare, cause: CauseIdentity,
		},
		{
			name: "permission", mode: sourceInitialRequired, err: fs.ErrPermission,
			operation: OperationOpen, cause: CausePermission,
		},
		{
			name: "unexpected I/O", mode: sourceInitialRequired, err: unix.EIO,
			operation: OperationOpen, cause: CauseUnstable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			requireDarwinSourceFailure(
				t,
				classifyDarwinSourceOpenFailure(context.Background(), test.mode, test.err),
				test.operation,
				test.cause,
			)
		})
	}

	for _, mode := range []sourcePresenceMode{
		sourceInitialRequired,
		sourceInitialOptional,
		sourceInitialForbidden,
		sourceInitialWalkPresent,
		sourceRevalidatePresent,
		sourceRevalidateAbsent,
	} {
		_, _, failure := classifyDarwinSourcePresenceFailure(
			context.Background(), mode, fs.ErrPermission,
		)
		requireDarwinSourceFailure(t, failure, OperationProbe, CausePermission)
	}

	for _, test := range []struct {
		name string
		kind sourceObservedKind
		mode sourcePresenceMode
	}{
		{name: "special kind", kind: sourceObservedSpecial, mode: sourceInitialRequired},
		{name: "invalid kind", kind: 255, mode: sourceInitialRequired},
		{name: "forbidden open", kind: sourceObservedRegular, mode: sourceInitialForbidden},
		{name: "absent revalidation open", kind: sourceObservedRegular, mode: sourceRevalidateAbsent},
		{name: "invalid mode", kind: sourceObservedRegular, mode: 255},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner, failure := primitives.openRelativeNoFollow(
				context.Background(), parent, "absent", test.kind, test.mode,
			)
			if owner != nil {
				_ = primitives.closeDescriptor(owner)
				t.Fatal("invalid source open unexpectedly acquired an owner")
			}
			requireDarwinSourceFailure(t, failure, OperationValidate, CauseInternalInvariant)
		})
	}
}

func requireDarwinSourcePrimitives(t *testing.T) sourcePrimitives {
	t.Helper()
	primitives, failure := platformSourcePrimitives()
	if failure != nil || primitives == nil {
		t.Fatalf("platform source primitives = %T / %+v", primitives, failure)
	}
	return primitives
}

func newDarwinSourceTestDirectoryOwner(
	t *testing.T,
	primitives sourcePrimitives,
	path string,
) *ownedSourceDescriptor {
	t.Helper()
	descriptor, err := os.Open(path)
	if err != nil {
		t.Fatalf("open source test directory %q: %v", path, err)
	}
	owner := &ownedSourceDescriptor{
		file:  descriptor,
		kind:  sourceObservedDirectory,
		state: sourceHandleOpen,
	}
	t.Cleanup(func() {
		_ = primitives.closeDescriptor(owner)
	})
	return owner
}

func requireDarwinSourceFailure(
	t *testing.T,
	failure *sourcePrimitiveFailure,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	if failure == nil || failure.operation != operation || failure.cause != cause {
		t.Fatalf("source primitive failure = %+v, want %s/%s", failure, operation, cause)
	}
}

func sourcePresenceModeName(mode sourcePresenceMode) string {
	switch mode {
	case sourceInitialRequired:
		return "initial-required"
	case sourceInitialOptional:
		return "initial-optional"
	case sourceInitialForbidden:
		return "initial-forbidden"
	case sourceInitialWalkPresent:
		return "initial-walk-present"
	case sourceRevalidatePresent:
		return "revalidate-present"
	case sourceRevalidateAbsent:
		return "revalidate-absent"
	default:
		return "invalid"
	}
}

func readAllDarwinSourceDirectoryNames(
	t *testing.T,
	primitives sourcePrimitives,
	owner *ownedSourceDescriptor,
) []string {
	t.Helper()
	var all []string
	for batch := range 1024 {
		names, done, failure := primitives.readDirectoryBatch(context.Background(), owner)
		if failure != nil {
			t.Fatalf("read directory batch %d: %+v", batch, failure)
		}
		if !done && len(names) == 0 {
			t.Fatalf("directory batch %d made no progress", batch)
		}
		all = append(all, names...)
		if done {
			return all
		}
	}
	t.Fatal("directory read did not terminate")
	return nil
}

func requireDarwinSourceNames(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("source names length = %d, want %d: %q", len(got), len(want), got)
	}
	for index := range want {
		if !bytes.Equal([]byte(got[index]), []byte(want[index])) {
			t.Fatalf("source name %d = %q, want %q", index, got[index], want[index])
		}
	}
}

type nameOnlyDarwinSourceDirEntry struct {
	name string
}

func (entry nameOnlyDarwinSourceDirEntry) Name() string {
	return entry.name
}

func (nameOnlyDarwinSourceDirEntry) IsDir() bool {
	panic("source directory normalization called DirEntry.IsDir")
}

func (nameOnlyDarwinSourceDirEntry) Type() fs.FileMode {
	panic("source directory normalization called DirEntry.Type")
}

func (nameOnlyDarwinSourceDirEntry) Info() (fs.FileInfo, error) {
	panic("source directory normalization called DirEntry.Info")
}

type darwinSourcePrimitiveStepContext struct {
	samples  int
	cancelAt int
	err      error
}

func (*darwinSourcePrimitiveStepContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (*darwinSourcePrimitiveStepContext) Done() <-chan struct{} {
	return nil
}

func (ctx *darwinSourcePrimitiveStepContext) Err() error {
	ctx.samples++
	if ctx.samples >= ctx.cancelAt {
		return ctx.err
	}
	return nil
}

func (*darwinSourcePrimitiveStepContext) Value(any) any {
	return nil
}
