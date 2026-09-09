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

func TestDarwinPreflightAuthorityProductionFactoryPrimitives(t *testing.T) {
	content := []byte("production preflight authority primitive graph\n")
	directory := physicalDarwinTempDir(t)
	if err := os.WriteFile(filepath.Join(directory, "go.env"), content, 0o600); err != nil {
		t.Fatalf("write production primitive fixture: %v", err)
	}
	parent, err := os.Open(directory)
	if err != nil {
		t.Fatalf("open production primitive parent: %v", err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	revalidator, factoryFailure := newPreflightAuthorityRevalidator()
	if factoryFailure != nil || revalidator == nil || !revalidator.primitives.valid() {
		t.Fatalf("production factory = %+v / %+v", revalidator, factoryFailure)
	}
	primitives := revalidator.primitives
	acquisition, openFailure := primitives.openRelativeNoFollow(
		context.Background(),
		parent,
		"go.env",
		authorityInitialRequiredOpen,
	)
	if acquisition == nil {
		t.Fatalf("production open acquisition = nil / %+v", openFailure)
	}
	owner, ownershipFailure := ownAuthorityDescriptor(acquisition)
	var closeOutcome authorityUseOutcome
	if owner != nil {
		t.Cleanup(func() { owner.closeInto(&closeOutcome) })
	}
	if openFailure != nil || ownershipFailure != nil || owner == nil || acquisition.file == nil {
		t.Fatalf(
			"production open and ownership = %+v / %+v / %+v",
			openFailure,
			ownershipFailure,
			owner,
		)
	}

	snapshot, statFailure := primitives.statDescriptor(context.Background(), acquisition.file)
	if statFailure != nil {
		t.Fatalf("production descriptor stat = %+v", statFailure)
	}
	if snapshotKind(snapshot) != entryRegular ||
		snapshot.identity.Inode == 0 ||
		snapshot.identity.UID != uint32(os.Geteuid()) ||
		snapshot.identity.Mode&0o7777 != 0o600 ||
		snapshot.size != int64(len(content)) {
		t.Fatalf("production descriptor snapshot = %+v", snapshot)
	}

	mount, mountFailure := primitives.statFilesystem(context.Background(), acquisition.file)
	if mountFailure != nil {
		t.Fatalf("production filesystem stat = %+v", mountFailure)
	}
	if validationFailure := primitives.validateFilesystem(
		context.Background(),
		mount,
	); validationFailure != nil {
		t.Fatalf("production filesystem validation = %+v", validationFailure)
	}
	if mount.flags&unix.MNT_LOCAL == 0 || mount.flags&unix.MNT_IGNORE_OWNERSHIP != 0 {
		t.Fatalf("production filesystem mount = %+v", mount)
	}

	rawACL, aclFailure := primitives.acquireRawACL(context.Background(), acquisition.file)
	if aclFailure != nil || len(rawACL) != darwinACLAttributeBufferSize {
		t.Fatalf("production raw ACL = %d bytes / %+v", len(rawACL), aclFailure)
	}
	aclDigest, parseACLFailure := primitives.parseRawACL(context.Background(), rawACL)
	if parseACLFailure != nil {
		t.Fatalf("production ACL parse = %+v", parseACLFailure)
	}
	wantACLDigest, err := digestExtendedSecurityResult(rawACL)
	if err != nil || aclDigest != wantACLDigest {
		t.Fatalf("production ACL digest = %x, want %x / %v", aclDigest, wantACLDigest, err)
	}

	read := make([]byte, len(content))
	if readFailure := primitives.readExactForParse(
		context.Background(),
		acquisition.file,
		read,
	); readFailure != nil || !bytes.Equal(read, content) {
		t.Fatalf("production exact read = %q / %+v", read, readFailure)
	}
	digest, hashFailure := primitives.hashBytes(context.Background(), read)
	wantDigest := Digest(sha256.Sum256(content))
	if hashFailure != nil || digest != wantDigest {
		t.Fatalf("production hash = %x, want %x / %+v", digest, wantDigest, hashFailure)
	}

	owner.closeInto(&closeOutcome)
	if !closeOutcome.proved() {
		t.Fatalf("production descriptor close = %+v", closeOutcome)
	}
	if _, err := acquisition.file.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("production descriptor remains open: %v", err)
	}
	if _, err := parent.Stat(); err != nil {
		t.Fatalf("borrowed parent descriptor was closed: %v", err)
	}
}

func TestDarwinPreflightAuthorityPrimitiveContextAttribution(t *testing.T) {
	parent, err := os.Open(physicalDarwinTempDir(t))
	if err != nil {
		t.Fatalf("open context-attribution parent: %v", err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	revalidator, factoryFailure := newPreflightAuthorityRevalidator()
	if factoryFailure != nil || revalidator == nil {
		t.Fatalf("production factory = %+v / %+v", revalidator, factoryFailure)
	}
	primitives := revalidator.primitives
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
				invoke    func(context.Context) *authorityPrimitiveFailure
			}{
				{
					name:      "open",
					operation: OperationOpen,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						acquisition, failure := primitives.openRelativeNoFollow(
							ctx,
							parent,
							"absent",
							authorityInitialRequiredOpen,
						)
						if acquisition != nil {
							owner, ownershipFailure := ownAuthorityDescriptor(acquisition)
							if owner != nil {
								var outcome authorityUseOutcome
								owner.closeInto(&outcome)
							}
							if failure == nil {
								failure = ownershipFailure
							}
						}
						return failure
					},
				},
				{
					name:      "descriptor stat",
					operation: OperationProbe,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						_, failure := primitives.statDescriptor(ctx, parent)
						return failure
					},
				},
				{
					name:      "filesystem stat",
					operation: OperationProbe,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						_, failure := primitives.statFilesystem(ctx, parent)
						return failure
					},
				},
				{
					name:      "filesystem policy",
					operation: OperationValidate,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						return primitives.validateFilesystem(ctx, mountSnapshot{})
					},
				},
				{
					name:      "raw ACL",
					operation: OperationProbe,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						_, failure := primitives.acquireRawACL(ctx, parent)
						return failure
					},
				},
				{
					name:      "ACL parse",
					operation: OperationParse,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						_, failure := primitives.parseRawACL(ctx, nil)
						return failure
					},
				},
				{
					name:      "exact read",
					operation: OperationParse,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						return primitives.readExactForParse(ctx, bytes.NewReader(nil), []byte{0})
					},
				},
				{
					name:      "hash",
					operation: OperationHash,
					invoke: func(ctx context.Context) *authorityPrimitiveFailure {
						_, failure := primitives.hashBytes(ctx, nil)
						return failure
					},
				},
			} {
				t.Run(primitive.name, func(t *testing.T) {
					ctx, cancel := termination.context()
					defer cancel()
					failure := primitive.invoke(ctx)
					if failure == nil || failure.operation != primitive.operation ||
						failure.cause != termination.cause {
						t.Fatalf("context failure = %+v", failure)
					}
				})
			}
		})
	}
}

func TestDarwinPreflightAuthorityOpenFailureAttributionIsClosed(t *testing.T) {
	for _, test := range []struct {
		name      string
		mode      authorityRequiredOpenMode
		err       error
		operation Operation
		cause     CauseCode
	}{
		{
			name:      "initial required absence",
			mode:      authorityInitialRequiredOpen,
			err:       fs.ErrNotExist,
			operation: OperationOpen,
			cause:     CauseNotFound,
		},
		{
			name:      "expected row disappearance",
			mode:      authorityExpectedPresentOpen,
			err:       fs.ErrNotExist,
			operation: OperationCompare,
			cause:     CauseUnstable,
		},
		{
			name:      "permission",
			mode:      authorityExpectedPresentOpen,
			err:       fs.ErrPermission,
			operation: OperationProbe,
			cause:     CausePermission,
		},
		{
			name:      "symlink ambiguity",
			mode:      authorityExpectedPresentOpen,
			err:       unix.ELOOP,
			operation: OperationProbe,
			cause:     CauseUnstable,
		},
		{
			name:      "nondirectory ambiguity",
			mode:      authorityExpectedPresentOpen,
			err:       unix.ENOTDIR,
			operation: OperationProbe,
			cause:     CauseUnstable,
		},
		{
			name:      "other lookup failure",
			mode:      authorityExpectedPresentOpen,
			err:       errors.New("injected lookup failure"),
			operation: OperationProbe,
			cause:     CauseUnstable,
		},
		{
			name:      "invalid mode",
			mode:      authorityRequiredOpenMode(255),
			err:       fs.ErrNotExist,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
		{
			name:      "missing error",
			mode:      authorityExpectedPresentOpen,
			operation: OperationValidate,
			cause:     CauseInternalInvariant,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := classifyPreflightAuthorityOpenFailure(
				context.Background(),
				test.mode,
				test.err,
			)
			if failure == nil || failure.operation != test.operation || failure.cause != test.cause {
				t.Fatalf("open failure = %+v", failure)
			}
		})
	}
}

func TestDarwinPreflightAuthorityIOCauseIsClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		err   error
		cause CauseCode
	}{
		{name: "permission", err: unix.EACCES, cause: CausePermission},
		{name: "operation not permitted", err: unix.EPERM, cause: CausePermission},
		{name: "unsupported capability", err: unix.ENOTSUP, cause: CauseUnsupported},
		{name: "I/O failure", err: unix.EIO, cause: CauseUnstable},
		{name: "bad descriptor", err: unix.EBADF, cause: CauseUnstable},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := authorityPrimitiveIOCause(test.err); got != test.cause {
				t.Fatalf("cause = %s, want %s", got, test.cause)
			}
		})
	}
}

func TestDarwinPreflightAuthorityFailedIOCanonicalizesInvalidContext(t *testing.T) {
	descriptor, err := os.Open(physicalDarwinTempDir(t))
	if err != nil {
		t.Fatalf("open invalid-context descriptor: %v", err)
	}
	if err := descriptor.Close(); err != nil {
		t.Fatalf("close invalid-context descriptor: %v", err)
	}
	for _, primitive := range []struct {
		name   string
		invoke func(context.Context) *authorityPrimitiveFailure
	}{
		{
			name: "descriptor stat",
			invoke: func(ctx context.Context) *authorityPrimitiveFailure {
				_, failure := statPreflightAuthorityDescriptor(ctx, descriptor)
				return failure
			},
		},
		{
			name: "filesystem stat",
			invoke: func(ctx context.Context) *authorityPrimitiveFailure {
				_, failure := statPreflightAuthorityFilesystem(ctx, descriptor)
				return failure
			},
		},
		{
			name: "raw ACL",
			invoke: func(ctx context.Context) *authorityPrimitiveFailure {
				_, failure := acquirePreflightAuthorityRawACL(ctx, descriptor)
				return failure
			},
		},
	} {
		t.Run(primitive.name, func(t *testing.T) {
			ctx := &postCheckAuthorityContext{after: errors.New("invalid context state")}
			failure := primitive.invoke(ctx)
			if ctx.calls != 2 || failure == nil || failure.operation != OperationValidate ||
				failure.cause != CauseInternalInvariant {
				t.Fatalf(
					"invalid post-I/O context = calls %d failure %+v",
					ctx.calls,
					failure,
				)
			}
		})
	}
}

func TestDarwinPreflightAuthorityOpenerOnlyAcquiresAndTransfersClosure(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "regular"), []byte("content"), 0o600); err != nil {
		t.Fatalf("write regular fixture: %v", err)
	}
	if err := os.Mkdir(filepath.Join(directory, "directory"), 0o700); err != nil {
		t.Fatalf("write directory fixture: %v", err)
	}
	parent, err := os.Open(directory)
	if err != nil {
		t.Fatalf("open parent fixture: %v", err)
	}
	t.Cleanup(func() { _ = parent.Close() })

	for _, name := range []string{"regular", "directory"} {
		t.Run(name, func(t *testing.T) {
			acquisition, failure := openPreflightAuthorityRelativeNoFollow(
				context.Background(),
				parent,
				name,
				authorityExpectedPresentOpen,
			)
			if failure != nil || acquisition == nil || acquisition.file == nil || acquisition.close == nil {
				t.Fatalf("raw acquisition = %+v / %+v", acquisition, failure)
			}
			if _, err := acquisition.file.Stat(); err != nil {
				t.Fatalf("raw opener closed descriptor before ownership: %v", err)
			}
			descriptorFlags, err := unix.FcntlInt(acquisition.file.Fd(), unix.F_GETFD, 0)
			if err != nil || descriptorFlags&unix.FD_CLOEXEC == 0 {
				t.Fatalf("descriptor flags = %#x / %v, want close-on-exec", descriptorFlags, err)
			}
			statusFlags, err := unix.FcntlInt(acquisition.file.Fd(), unix.F_GETFL, 0)
			if err != nil || statusFlags&unix.O_NONBLOCK == 0 {
				t.Fatalf("status flags = %#x / %v, want nonblocking", statusFlags, err)
			}
			owner, ownerFailure := ownAuthorityDescriptor(acquisition)
			if ownerFailure != nil || owner == nil {
				t.Fatalf("own acquisition = %+v / %+v", owner, ownerFailure)
			}
			var outcome authorityUseOutcome
			owner.closeInto(&outcome)
			owner.closeInto(&outcome)
			if outcome.descriptorClose != nil {
				t.Fatalf("close outcome = %+v", outcome.descriptorClose)
			}
			if _, err := acquisition.file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatalf("owned descriptor remains open: %v", err)
			}
		})
	}

	t.Run("post-open cancellation transfers ownership", func(t *testing.T) {
		ctx := &postCheckAuthorityContext{after: context.Canceled}
		acquisition, openFailure := openPreflightAuthorityRelativeNoFollow(
			ctx,
			parent,
			"regular",
			authorityExpectedPresentOpen,
		)
		owner, failure := ownAuthorityAcquisition(acquisition, openFailure)
		if ctx.calls != 2 || owner == nil || failure == nil ||
			failure.operation != OperationOpen || failure.cause != CauseCanceled {
			t.Fatalf(
				"post-open cancellation = calls %d owner %v failure %+v",
				ctx.calls,
				owner != nil,
				failure,
			)
		}
		var outcome authorityUseOutcome
		owner.closeInto(&outcome)
		owner.closeInto(&outcome)
		if outcome.descriptorClose != nil {
			t.Fatalf("post-open cancellation close = %+v", outcome.descriptorClose)
		}
		if _, err := acquisition.file.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Fatalf("post-open cancellation leaked descriptor: %v", err)
		}
	})
}

func TestDarwinPreflightAuthorityOpenerRefusesSymlinksWithoutOwnership(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "target"), nil, 0o600); err != nil {
		t.Fatalf("write target fixture: %v", err)
	}
	if err := os.Symlink("target", filepath.Join(directory, "link")); err != nil {
		t.Fatalf("write symlink fixture: %v", err)
	}
	parent, err := os.Open(directory)
	if err != nil {
		t.Fatalf("open parent fixture: %v", err)
	}
	t.Cleanup(func() { _ = parent.Close() })
	acquisition, failure := openPreflightAuthorityRelativeNoFollow(
		context.Background(),
		parent,
		"link",
		authorityExpectedPresentOpen,
	)
	if acquisition != nil || failure == nil ||
		failure.operation != OperationProbe || failure.cause != CauseUnstable {
		t.Fatalf("symlink acquisition = %+v / %+v", acquisition, failure)
	}
}

func TestDarwinPreflightAuthorityFilesystemPolicyIsStaticValidation(t *testing.T) {
	filesystem := unix.Statfs_t{Flags: unix.MNT_LOCAL | unix.MNT_UPDATE}
	filesystem.Fsid.Val = [2]int32{-7, 9}
	mount := normalizePreflightAuthorityMount(filesystem)
	if mount.filesystem != filesystem.Fsid.Val || mount.flags != unix.MNT_LOCAL {
		t.Fatalf("normalized mount = %+v", mount)
	}
	if failure := validatePreflightAuthorityFilesystem(
		context.Background(),
		mount,
	); failure != nil {
		t.Fatalf("local ownership-enforcing mount refused: %+v", failure)
	}
	for _, flags := range []uint32{
		0,
		unix.MNT_LOCAL | unix.MNT_IGNORE_OWNERSHIP,
	} {
		failure := validatePreflightAuthorityFilesystem(
			context.Background(),
			mountSnapshot{flags: flags},
		)
		if failure == nil || failure.operation != OperationValidate ||
			failure.cause != CauseUnsupported {
			t.Fatalf("unsafe mount flags %#x = %+v", flags, failure)
		}
	}
}
