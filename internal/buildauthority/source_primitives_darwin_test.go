//go:build darwin && arm64

package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

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

	ctx := &postCheckAuthorityContext{after: context.Canceled}
	owner, failure := primitives.openRelativeNoFollow(
		ctx, parent, "regular", sourceObservedRegular, sourceInitialRequired,
	)
	if owner != nil {
		defer primitives.closeDescriptor(owner)
	}
	if ctx.calls != 2 || owner == nil || !owner.validOpen() {
		t.Fatalf("post-open cancellation = calls %d owner %+v failure %+v", ctx.calls, owner, failure)
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
	case sourceRevalidatePresent:
		return "revalidate-present"
	case sourceRevalidateAbsent:
		return "revalidate-absent"
	default:
		return "invalid"
	}
}
