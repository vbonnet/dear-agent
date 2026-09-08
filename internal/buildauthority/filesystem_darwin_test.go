//go:build darwin && arm64

package buildauthority

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

type darwinTestACE struct {
	flags  uint32
	rights uint32
}

func TestDarwinFilesecGoldenStates(t *testing.T) {
	t.Parallel()

	absent, err := digestDarwinFilesec(nil)
	if err != nil {
		t.Fatalf("digest absent ACL: %v", err)
	}
	noACL, err := digestDarwinFilesec(makeDarwinTestFilesec(darwinFilesecNoACL, 0, nil))
	if err != nil {
		t.Fatalf("digest explicit no-ACL: %v", err)
	}
	empty, err := digestDarwinFilesec(makeDarwinTestFilesec(0, 0, nil))
	if err != nil {
		t.Fatalf("digest empty ACL: %v", err)
	}

	assertDigestHex(t, absent, "5f4fb3caaaca1b2b873cec1ba79116849198f3bbfe57fdb57419f099375bbe2a")
	assertDigestHex(t, noACL, "29caadb1ede3e588ef0c78943bea5c3d42bedc305535ba5bfde36f1dfdd6690b")
	assertDigestHex(t, empty, "a36bbfb811befc9518fb8e4f0a6a45e49c0c2987761e0e2eb01e4bf192fe9059")
	if absent == noACL || absent == empty || noACL == empty {
		t.Fatal("absent, explicit no-ACL, and empty ACL digests must differ")
	}
}

func TestDarwinFilesecAdmitsClosedReadAndDenyGrammar(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		flags  uint32
		rights uint32
	}{
		{name: "permit-read", flags: darwinACEPermit, rights: 1 << 1},
		{name: "permit-execute-generic-read", flags: darwinACEPermit | darwinACEInherited, rights: 1<<3 | darwinACEGenericRead},
		{name: "deny-mutation", flags: darwinACEDeny | darwinACEFileInherit, rights: darwinMutationRights},
		{name: "deny-all-allowed", flags: darwinACEDeny | darwinACEOnlyInherit, rights: darwinAllowedACERights},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			attribute := makeDarwinTestFilesec(1, darwinAllowedACLFlags, []darwinTestACE{{
				flags:  test.flags,
				rights: test.rights,
			}})
			if _, err := digestDarwinFilesec(attribute); err != nil {
				t.Fatalf("digest valid filesec: %v", err)
			}
		})
	}
}

func TestDarwinFilesecRefusesMalformedAndUnsafeRecords(t *testing.T) {
	t.Parallel()

	valid := makeDarwinTestFilesec(1, 0, []darwinTestACE{{flags: darwinACEDeny, rights: 1 << 1}})
	tests := []struct {
		name  string
		bytes []byte
		cause CauseCode
	}{
		{name: "short", bytes: valid[:darwinFilesecHeaderSize-1], cause: CauseMalformed},
		{name: "trailing", bytes: append(append([]byte(nil), valid...), 0), cause: CauseMalformed},
		{name: "no-acl-with-ace", bytes: makeDarwinTestFilesec(darwinFilesecNoACL, 0, []darwinTestACE{{flags: darwinACEDeny}}), cause: CauseMalformed},
		{name: "too-many", bytes: makeDarwinTestFilesec(darwinACLMaxEntries+1, 0, nil), cause: CauseLimit},
		{name: "unknown-acl-flag", bytes: makeDarwinTestFilesec(0, 1, nil), cause: CauseMalformed},
		{name: "audit-kind", bytes: makeDarwinTestFilesec(1, 0, []darwinTestACE{{flags: 3}}), cause: CauseMalformed},
		{name: "permit-and-deny", bytes: makeDarwinTestFilesec(1, 0, []darwinTestACE{{flags: darwinACEPermit | darwinACEDeny}}), cause: CauseMalformed},
		{name: "unknown-ace-flag", bytes: makeDarwinTestFilesec(1, 0, []darwinTestACE{{flags: darwinACEDeny | 1<<9}}), cause: CauseMalformed},
		{name: "unknown-right-low", bytes: makeDarwinTestFilesec(1, 0, []darwinTestACE{{flags: darwinACEDeny, rights: 1}}), cause: CauseMalformed},
		{name: "unknown-right-high", bytes: makeDarwinTestFilesec(1, 0, []darwinTestACE{{flags: darwinACEDeny, rights: 1 << 25}}), cause: CauseMalformed},
	}
	badMagic := append([]byte(nil), valid...)
	binary.LittleEndian.PutUint32(badMagic[:4], darwinFilesecMagic+1)
	tests = append(tests, struct {
		name  string
		bytes []byte
		cause CauseCode
	}{name: "bad-magic", bytes: badMagic, cause: CauseMalformed})
	bigEndianMagic := append([]byte(nil), valid...)
	binary.BigEndian.PutUint32(bigEndianMagic[:4], darwinFilesecMagic)
	tests = append(tests, struct {
		name  string
		bytes []byte
		cause CauseCode
	}{name: "wrong-endian", bytes: bigEndianMagic, cause: CauseMalformed})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := digestDarwinFilesec(test.bytes)
			assertPrivateCause(t, err, test.cause)
		})
	}
}

func TestDarwinFilesecRefusesEveryMutationPermit(t *testing.T) {
	t.Parallel()

	for _, right := range []uint32{
		1 << 2,
		1 << 4,
		1 << 5,
		1 << 6,
		1 << 8,
		1 << 10,
		1 << 12,
		1 << 13,
		darwinACEGenericWrite,
		darwinACEGenericAll,
	} {
		attribute := makeDarwinTestFilesec(1, 0, []darwinTestACE{{flags: darwinACEPermit, rights: right}})
		_, err := digestDarwinFilesec(attribute)
		assertPrivateCause(t, err, CausePermission)
	}
}

func TestDarwinFilesecAcceptsExactMaximumEntries(t *testing.T) {
	t.Parallel()

	entries := make([]darwinTestACE, darwinACLMaxEntries)
	for index := range entries {
		entries[index] = darwinTestACE{flags: darwinACEDeny, rights: darwinAllowedACERights}
	}
	if _, err := digestDarwinFilesec(makeDarwinTestFilesec(darwinACLMaxEntries, 0, entries)); err != nil {
		t.Fatalf("digest maximum ACL: %v", err)
	}
}

func TestExtendedSecurityResultBounds(t *testing.T) {
	t.Parallel()

	attribute := makeDarwinTestFilesec(0, 0, nil)
	valid := makeDarwinTestAttrResult(attribute)
	if _, err := digestExtendedSecurityResult(valid); err != nil {
		t.Fatalf("digest valid result: %v", err)
	}

	absent := makeDarwinTestAttrResult(nil)
	gotAbsent, err := digestExtendedSecurityResult(absent)
	if err != nil {
		t.Fatalf("digest absent result: %v", err)
	}
	wantAbsent, err := digestDarwinFilesec(nil)
	if err != nil {
		t.Fatalf("digest absent filesec: %v", err)
	}
	if gotAbsent != wantAbsent {
		t.Fatal("zero-length descriptor attribute did not use absent encoding")
	}
	zeroLengthOverlap := append([]byte(nil), absent...)
	binary.LittleEndian.PutUint32(zeroLengthOverlap[4:8], 0)
	if _, err := digestExtendedSecurityResult(zeroLengthOverlap); err == nil {
		t.Fatal("zero-length attribute with overlapping reference succeeded")
	}
	zeroLengthTrailing := append(append([]byte(nil), absent...), 0)
	binary.LittleEndian.PutUint32(zeroLengthTrailing[:4], uint32(len(zeroLengthTrailing)))
	if _, err := digestExtendedSecurityResult(zeroLengthTrailing); err == nil {
		t.Fatal("zero-length attribute with trailing result byte succeeded")
	}

	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{name: "short", mutate: func(result []byte) { binary.LittleEndian.PutUint32(result[:4], darwinAttrResultHeaderSize-1) }},
		{name: "reported-over-buffer", mutate: func(result []byte) { binary.LittleEndian.PutUint32(result[:4], uint32(len(result)+1)) }},
		{name: "negative-reference", mutate: func(result []byte) { binary.LittleEndian.PutUint32(result[4:8], ^uint32(7)) }},
		{name: "overlapping-reference", mutate: func(result []byte) { binary.LittleEndian.PutUint32(result[4:8], 0) }},
		{name: "extent-overflow", mutate: func(result []byte) { binary.LittleEndian.PutUint32(result[8:12], uint32(len(result))) }},
		{name: "trailing-result-bytes", mutate: func(result []byte) { binary.LittleEndian.PutUint32(result[:4], uint32(len(result))) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := append(append([]byte(nil), valid...), 0)
			test.mutate(result)
			_, err := digestExtendedSecurityResult(result)
			assertPrivateCause(t, err, CauseMalformed)
		})
	}
}

func TestDarwinDescriptorPrimitives(t *testing.T) {
	t.Parallel()

	physicalRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(physicalRoot, "file"), []byte("content"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.Mkdir(filepath.Join(physicalRoot, "directory"), 0o700); err != nil {
		t.Fatalf("make directory fixture: %v", err)
	}
	if err := os.Symlink("file", filepath.Join(physicalRoot, "link")); err != nil {
		t.Fatalf("make symlink fixture: %v", err)
	}

	root, err := openAbsoluteNoFollow(physicalRoot, entryDirectory)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() {
		_ = root.Close()
	}()

	snapshot, mount, aclDigest, err := inspectDescriptor(root)
	if err != nil {
		t.Fatalf("inspect root: %v", err)
	}
	if snapshotKind(snapshot) != entryDirectory || snapshot.identity.Inode == 0 || snapshot.identity.Device == 0 {
		t.Fatalf("unexpected root snapshot: %+v", snapshot)
	}
	if snapshot.identity.Filesystem != mount.filesystem {
		t.Fatalf("identity filesystem %v differs from mount %v", snapshot.identity.Filesystem, mount.filesystem)
	}
	wantAbsent, err := digestDarwinFilesec(nil)
	if err != nil {
		t.Fatalf("digest absent ACL: %v", err)
	}
	if aclDigest != wantAbsent {
		t.Fatalf("new temp root ACL digest = %x, want absent %x", aclDigest, wantAbsent)
	}

	for _, test := range []struct {
		path string
		kind entryKind
	}{
		{path: "file", kind: entryRegular},
		{path: "directory", kind: entryDirectory},
		{path: "link", kind: entrySymlink},
	} {
		descriptor, openErr := openRelativeNoFollow(int(root.Fd()), test.path, test.kind)
		if openErr != nil {
			t.Fatalf("open %s: %v", test.path, openErr)
		}
		openedSnapshot, _, _, inspectErr := inspectDescriptor(descriptor)
		closeErr := descriptor.Close()
		if inspectErr != nil {
			t.Fatalf("inspect %s: %v", test.path, inspectErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", test.path, closeErr)
		}
		if snapshotKind(openedSnapshot) != test.kind {
			t.Fatalf("%s kind = %d, want %d", test.path, snapshotKind(openedSnapshot), test.kind)
		}
	}

	link, err := openRelativeNoFollow(int(root.Fd()), "link", entrySymlink)
	if err != nil {
		t.Fatalf("open link: %v", err)
	}
	defer func() {
		_ = link.Close()
	}()
	text, err := readOpenedSymlink(link, uint64(len("file")))
	if err != nil {
		t.Fatalf("read link at exact bound: %v", err)
	}
	if text != "file" {
		t.Fatalf("link text = %q, want file", text)
	}
	if _, err := readOpenedSymlink(link, uint64(len("file")-1)); err == nil {
		t.Fatal("read link below exact bound succeeded")
	}
}

func TestDarwinOpenedSymlinkHostAndContractBounds(t *testing.T) {
	t.Parallel()

	physicalRoot := physicalDarwinTempDir(t)
	hostMaximumText := strings.Repeat("a", 1023)
	if err := os.Symlink(hostMaximumText, filepath.Join(physicalRoot, "long-link")); err != nil {
		t.Fatalf("make 1023-byte symlink: %v", err)
	}
	root, err := openAbsoluteNoFollow(physicalRoot, entryDirectory)
	if err != nil {
		t.Fatalf("open symlink fixture root: %v", err)
	}
	defer func() { _ = root.Close() }()
	link, err := openRelativeNoFollow(int(root.Fd()), "long-link", entrySymlink)
	if err != nil {
		t.Fatalf("open 1023-byte symlink: %v", err)
	}
	defer func() { _ = link.Close() }()

	got, err := readOpenedSymlink(link, uint64(len(hostMaximumText)))
	if err != nil || got != hostMaximumText {
		t.Fatalf("read host-maximum symlink = (%d bytes, %v), want (1023, nil)", len(got), err)
	}
	if _, err := readOpenedSymlink(link, uint64(len(hostMaximumText)-1)); err == nil {
		t.Fatal("1023-byte symlink succeeded under a 1022-byte bound")
	} else {
		assertPrivateCause(t, err, CauseLimit)
	}

	exactContractRead := func(_ context.Context, _ *os.File, buffer []byte) (uintptr, error) {
		for index := range maxSymlinkBytes {
			buffer[index] = 'b'
		}
		return maxSymlinkBytes, nil
	}
	got, err = readOpenedSymlinkWith(context.Background(), link, maxSymlinkBytes, exactContractRead)
	if err != nil || len(got) != maxSymlinkBytes {
		t.Fatalf("read exact 4-KiB contract text = (%d bytes, %v)", len(got), err)
	}
	oneOverContractRead := func(_ context.Context, _ *os.File, buffer []byte) (uintptr, error) {
		for index := range buffer {
			buffer[index] = 'c'
		}
		return uintptr(len(buffer)), nil
	}
	if _, err := readOpenedSymlinkWith(context.Background(), link, maxSymlinkBytes, oneOverContractRead); err == nil {
		t.Fatal("4-KiB-plus-one contract text succeeded")
	} else {
		assertPrivateCause(t, err, CauseLimit)
	}
}

func TestDarwinRawEINTRLoopsObserveCancellation(t *testing.T) {
	t.Parallel()

	descriptor, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open syscall seam descriptor: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := descriptor.Close(); closeErr != nil {
			t.Errorf("close syscall seam descriptor: %v", closeErr)
		}
	})

	t.Run("freadlink", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, err := readDarwinSymlinkWith(
			ctx,
			descriptor,
			make([]byte, 1),
			func(_ *os.File, _ []byte) (uintptr, error) {
				calls++
				cancel()
				return 0, unix.EINTR
			},
		)
		assertPrivateCause(t, err, CauseCanceled)
		if calls != 1 {
			t.Fatalf("freadlink calls = %d, want 1 before cancellation", calls)
		}
	})

	t.Run("fgetattrlist", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		_, err := descriptorACLDigestWith(
			ctx,
			int(descriptor.Fd()),
			func(_ int, _ *unix.Attrlist, _ []byte) error {
				calls++
				cancel()
				return unix.EINTR
			},
		)
		assertPrivateCause(t, err, CauseCanceled)
		if calls != 1 {
			t.Fatalf("fgetattrlist calls = %d, want 1 before cancellation", calls)
		}
	})
}

func TestDarwinRelativeOpenRefusesIntermediateSymlink(t *testing.T) {
	t.Parallel()

	physicalRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp root: %v", err)
	}
	if err := os.Mkdir(filepath.Join(physicalRoot, "real"), 0o700); err != nil {
		t.Fatalf("make real directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(physicalRoot, "real", "file"), []byte("content"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.Symlink("real", filepath.Join(physicalRoot, "alias")); err != nil {
		t.Fatalf("make intermediate symlink: %v", err)
	}
	root, err := openAbsoluteNoFollow(physicalRoot, entryDirectory)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer func() {
		_ = root.Close()
	}()

	if descriptor, openErr := openRelativeNoFollow(int(root.Fd()), "alias/file", entryRegular); openErr == nil {
		_ = descriptor.Close()
		t.Fatal("open through intermediate symlink succeeded")
	}
	for _, path := range []string{"", ".", "../file", "real/../real/file", "/file"} {
		if descriptor, openErr := openRelativeNoFollow(int(root.Fd()), path, entryRegular); openErr == nil {
			_ = descriptor.Close()
			t.Fatalf("invalid relative path %q succeeded", path)
		}
	}
}

func TestDarwinRetainedDirectoryRejectsDualOpenReplacement(t *testing.T) {
	t.Parallel()

	base := physicalDarwinTempDir(t)
	path := filepath.Join(base, "authority")
	moved := filepath.Join(base, "moved")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make original authority: %v", err)
	}
	descriptor, err := openAbsoluteNoFollow(path, entryDirectory)
	if err != nil {
		t.Fatalf("open original descriptor: %v", err)
	}
	defer func() { _ = descriptor.Close() }()
	if err := os.Rename(path, moved); err != nil {
		t.Fatalf("move original authority: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make replacement authority: %v", err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatalf("open replacement root: %v", err)
	}
	defer func() { _ = root.Close() }()

	claim, err := inspectAuthorityPathDirectory(descriptor)
	if err != nil {
		t.Fatalf("inspect original path claim: %v", err)
	}
	_, err = inspectRetainedDirectory(path, descriptor, root, []authorityPathClaim{claim})
	assertPrivateCause(t, err, CauseIdentity)
}

func TestDarwinRetainedDirectoryRootFirstRejectsSymlinkBackToOriginal(t *testing.T) {
	t.Parallel()

	base := physicalDarwinTempDir(t)
	path := filepath.Join(base, "authority")
	moved := filepath.Join(base, "moved")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("make original authority: %v", err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatalf("open authority root first: %v", err)
	}
	if err := os.Rename(path, moved); err != nil {
		_ = root.Close()
		t.Fatalf("move original authority: %v", err)
	}
	if err := os.Symlink(filepath.Base(moved), path); err != nil {
		_ = root.Close()
		t.Fatalf("symlink pathname back to original: %v", err)
	}

	directory, err := openRetainedDirectoryFromRoot(path, root)
	if directory != nil {
		_ = directory.close()
		t.Fatal("root-first dual open admitted a symlink pathname")
	}
	assertPrivateCause(t, err, CauseUnstable)
}

func TestDarwinRetainedDirectoryRevalidatesLinkedPathClaims(t *testing.T) {
	t.Parallel()

	base := physicalDarwinTempDir(t)
	safeParent := filepath.Join(base, "safe")
	authorityPath := filepath.Join(safeParent, "authority")
	movedParent := filepath.Join(base, "moved")
	if err := os.Mkdir(safeParent, 0o700); err != nil {
		t.Fatalf("make safe parent: %v", err)
	}
	if err := os.Mkdir(authorityPath, 0o700); err != nil {
		t.Fatalf("make authority: %v", err)
	}
	directory, err := openRetainedDirectory(authorityPath)
	if err != nil {
		t.Fatalf("retain authority: %v", err)
	}
	defer func() { _ = directory.close() }()

	identities := directory.authorityIdentities()
	if len(identities) != len(directory.pathClaims) || len(identities) < 2 {
		t.Fatalf("ordered identity set has %d rows for %d claims", len(identities), len(directory.pathClaims))
	}
	if !sameFilesystemObject(identities[len(identities)-1], directory.snapshot.identity) {
		t.Fatal("ordered authority identities do not end at the retained leaf")
	}
	parentDescriptor, parentClaims, err := openAuthorityPath(safeParent)
	if err != nil {
		t.Fatalf("open retained parent ladder: %v", err)
	}
	parentSnapshot, parentMount, parentACL, inspectErr := inspectDescriptor(parentDescriptor)
	closeErr := parentDescriptor.Close()
	if inspectErr != nil || closeErr != nil {
		t.Fatalf("inspect retained parent = (%v, close %v)", inspectErr, closeErr)
	}
	if len(parentClaims)+1 != len(directory.pathClaims) {
		t.Fatalf("parent ladder depth = %d, authority depth = %d", len(parentClaims), len(directory.pathClaims))
	}
	retainedParent := directory.pathClaims[len(directory.pathClaims)-2]
	wantParent := makeAuthorityPathClaim(parentSnapshot, parentMount, parentACL)
	if retainedParent != wantParent {
		t.Fatal("retained parent claim differs from descriptor-inspected stable metadata")
	}
	if retainedParent.gid != parentSnapshot.gid ||
		retainedParent.rdev != parentSnapshot.rdev ||
		retainedParent.birthSec != parentSnapshot.birthSec ||
		retainedParent.birthNsec != parentSnapshot.birthNsec ||
		retainedParent.flags != parentSnapshot.flags ||
		retainedParent.generation != parentSnapshot.generation {
		t.Fatal("retained parent claim omitted stable Darwin inode metadata")
	}

	churnPath := filepath.Join(safeParent, "unrelated")
	if err := os.WriteFile(churnPath, []byte("churn"), 0o600); err != nil {
		t.Fatalf("create unrelated parent entry: %v", err)
	}
	if err := os.Remove(churnPath); err != nil {
		t.Fatalf("remove unrelated parent entry: %v", err)
	}
	if err := validateRetainedRoot(directory); err != nil {
		t.Fatalf("unrelated ancestor churn invalidated security claim: %v", err)
	}

	if err := os.Rename(safeParent, movedParent); err != nil {
		t.Fatalf("move safe parent: %v", err)
	}
	if err := os.Mkdir(safeParent, 0o700); err != nil {
		t.Fatalf("make substitute parent: %v", err)
	}
	if err := os.Chmod(safeParent, 0o777); err != nil {
		t.Fatalf("make substitute parent unsafe: %v", err)
	}
	assertPrivateCause(t, validateRetainedRoot(directory), CausePermission)
}

func TestDarwinAuthorityTreeCountCancellationAndRootExclusion(t *testing.T) {
	t.Parallel()

	base := physicalDarwinTempDir(t)
	policy := treePolicy{
		domain: domainRepository,
		owners: ownerEffectiveOnly,
		limits: treeLimits{maxEntries: 1, maxBytes: 16, maxFileBytes: 16},
	}

	empty := filepath.Join(base, "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatalf("make empty authority: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if capture, err := admitAuthorityTree(canceled, empty, policy); err == nil {
		_ = capture.close()
		t.Fatal("canceled empty-tree admission succeeded")
	} else {
		assertPrivateCause(t, err, CauseCanceled)
	}

	exact := filepath.Join(base, "exact")
	if err := os.Mkdir(exact, 0o700); err != nil {
		t.Fatalf("make exact authority: %v", err)
	}
	if err := os.WriteFile(filepath.Join(exact, "file"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write exact authority file: %v", err)
	}
	capture, err := admitAuthorityTree(context.Background(), exact, policy)
	if err != nil {
		t.Fatalf("admit exact descendant count: %v", err)
	}
	if len(capture.entries) != 1 {
		t.Fatalf("descendant count = %d, want 1 with root excluded", len(capture.entries))
	}
	if err := capture.close(); err != nil {
		t.Fatalf("close exact capture: %v", err)
	}

	over := filepath.Join(base, "over")
	if err := os.Mkdir(over, 0o700); err != nil {
		t.Fatalf("make over-limit authority: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		if err := os.WriteFile(filepath.Join(over, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("write over-limit authority file: %v", err)
		}
	}
	if capture, err := admitAuthorityTree(context.Background(), over, policy); err == nil {
		_ = capture.close()
		t.Fatal("one-over descendant count succeeded")
	} else {
		assertPrivateCause(t, err, CauseLimit)
	}
}

func TestDarwinAuthorityTreeUsesBoundedDirectoryBatches(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(physicalDarwinTempDir(t), "authority")
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatalf("make batched authority fixture: %v", err)
	}
	for index := range directoryReadBatchSize + 1 {
		name := fmt.Sprintf("entry-%03d", index)
		if err := os.WriteFile(filepath.Join(rootPath, name), nil, 0o600); err != nil {
			t.Fatalf("write batched entry %d: %v", index, err)
		}
	}
	policy := treePolicy{
		domain: domainRepository,
		owners: ownerEffectiveOnly,
		limits: treeLimits{
			maxEntries:   directoryReadBatchSize + 1,
			maxBytes:     1,
			maxFileBytes: 1,
		},
	}
	capture, err := admitAuthorityTree(context.Background(), rootPath, policy)
	if err != nil {
		t.Fatalf("admit authority with more than one batch: %v", err)
	}
	defer func() { _ = capture.close() }()
	if len(capture.entries) != directoryReadBatchSize+1 {
		t.Fatalf("captured entries = %d, want %d", len(capture.entries), directoryReadBatchSize+1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	readCalls := 0
	boundedRead := func(descriptor *os.File, count int) ([]os.DirEntry, error) {
		readCalls++
		if count != directoryReadBatchSize {
			return nil, fail(CauseInternalInvariant, "directory reader was not bounded")
		}
		entries, readErr := descriptor.ReadDir(count)
		cancel()
		return entries, readErr
	}
	_, err = captureRetainedTreeWithRead(ctx, capture.root, policy, boundedRead)
	assertPrivateCause(t, err, CauseCanceled)
	if readCalls != 1 {
		t.Fatalf("directory read calls after cancellation = %d, want 1", readCalls)
	}
}

func TestDarwinGOROOTAdmissionRecursesAndResolvesComponents(t *testing.T) {
	t.Parallel()

	base := physicalDarwinTempDir(t)
	rootPath := filepath.Join(base, "goroot")
	directoryPath := filepath.Join(rootPath, "directory")
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatalf("make GOROOT fixture: %v", err)
	}
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatalf("make nested directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(directoryPath, "child"), 0o700); err != nil {
		t.Fatalf("make traversal directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directoryPath, "target"), []byte("target"), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	if err := os.Symlink("child/../target", filepath.Join(directoryPath, "link")); err != nil {
		t.Fatalf("make component-wise symlink: %v", err)
	}
	policy := treePolicy{
		domain:        domainGOROOT,
		owners:        ownerEffectiveOnly,
		allowSymlinks: true,
		limits:        treeLimits{maxEntries: 4, maxBytes: 6, maxFileBytes: 6},
	}
	capture, err := admitAuthorityTree(context.Background(), rootPath, policy)
	if err != nil {
		t.Fatalf("admit recursive GOROOT fixture: %v", err)
	}
	defer func() { _ = capture.close() }()
	if len(capture.entries) != 4 {
		t.Fatalf("recursive descendant count = %d, want 4", len(capture.entries))
	}
	var link, target *entrySnapshot
	for index := range capture.entries {
		switch capture.entries[index].path {
		case "directory/link":
			link = &capture.entries[index]
		case "directory/target":
			target = &capture.entries[index]
		}
	}
	if link == nil || target == nil {
		t.Fatalf("recursive capture omitted link or target: %#v", capture.entries)
	}
	if link.targetDigest != digestEntryClaim(*target) {
		t.Fatal("component-wise symlink did not bind the final regular record")
	}
}

func TestDarwinAuthorityTreeRejectsSpecialEntry(t *testing.T) {
	t.Parallel()

	rootPath := filepath.Join(physicalDarwinTempDir(t), "authority")
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatalf("make authority fixture: %v", err)
	}
	if err := unix.Mkfifo(filepath.Join(rootPath, "fifo"), 0o600); err != nil {
		t.Fatalf("make FIFO fixture: %v", err)
	}
	policy := treePolicy{
		domain: domainRepository,
		owners: ownerEffectiveOnly,
		limits: treeLimits{maxEntries: 1, maxBytes: 1, maxFileBytes: 1},
	}
	if capture, err := admitAuthorityTree(context.Background(), rootPath, policy); err == nil {
		_ = capture.close()
		t.Fatal("special authority entry succeeded")
	} else {
		assertPrivateCause(t, err, CauseUnsupported)
	}
}

func TestDarwinSymlinkTargetRevalidationRejectsSameManifestReplacement(t *testing.T) {
	t.Parallel()

	base := physicalDarwinTempDir(t)
	rootPath := filepath.Join(base, "authority")
	directoryPath := filepath.Join(rootPath, "directory")
	filePath := filepath.Join(directoryPath, "file")
	linkPath := filepath.Join(directoryPath, "link")
	if err := os.Mkdir(rootPath, 0o700); err != nil {
		t.Fatalf("make authority: %v", err)
	}
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatalf("make authority directory: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("same"), 0o600); err != nil {
		t.Fatalf("write authority file: %v", err)
	}
	if err := os.Symlink("file", linkPath); err != nil {
		t.Fatalf("make captured symlink: %v", err)
	}
	policy := treePolicy{
		domain:        domainGOROOT,
		owners:        ownerEffectiveOnly,
		allowSymlinks: true,
		limits:        treeLimits{maxEntries: 8, maxBytes: 64, maxFileBytes: 64},
	}
	capture, err := admitAuthorityTree(context.Background(), rootPath, policy)
	if err != nil {
		t.Fatalf("admit authority: %v", err)
	}
	defer func() { _ = capture.close() }()

	replacement := filepath.Join(base, "replacement")
	if err := os.WriteFile(replacement, []byte("same"), 0o600); err != nil {
		t.Fatalf("write replacement: %v", err)
	}
	if err := os.Rename(replacement, filePath); err != nil {
		t.Fatalf("replace authority file: %v", err)
	}
	current, err := captureRetainedTree(context.Background(), capture.root, policy)
	if err != nil {
		t.Fatalf("recapture same-manifest replacement: %v", err)
	}
	if current.digest != capture.digest {
		t.Fatalf("test replacement changed serialized manifest: got %x, want %x", current.digest, capture.digest)
	}
	if equalEntrySnapshots(current.entries, capture.entries) {
		t.Fatal("replacement retained the complete session snapshot")
	}
	var originalLink, currentLink *entrySnapshot
	for index := range capture.entries {
		if capture.entries[index].path == "directory/link" {
			originalLink = &capture.entries[index]
		}
	}
	for index := range current.entries {
		if current.entries[index].path == "directory/link" {
			currentLink = &current.entries[index]
		}
	}
	if originalLink == nil || currentLink == nil || originalLink.targetDigest != currentLink.targetDigest {
		t.Fatal("same-manifest replacement did not preserve the symlink content claim")
	}
	assertPrivateCause(t, capture.revalidate(context.Background()), CauseUnstable)
}

func TestDarwinStateRootAdmissionAndFreshRevalidation(t *testing.T) {
	t.Parallel()

	base := physicalDarwinTempDir(t)
	statePath := filepath.Join(base, "state")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatalf("make StateRoot: %v", err)
	}
	state, err := admitStateRoot(statePath)
	if err != nil {
		t.Fatalf("admit StateRoot: %v", err)
	}
	if err := state.close(); err != nil {
		t.Fatalf("close StateRoot: %v", err)
	}

	retained, err := openRetainedDirectory(statePath)
	if err != nil {
		t.Fatalf("open retained StateRoot: %v", err)
	}
	defer func() { _ = retained.close() }()
	if err := os.Chmod(statePath, 0o755); err != nil {
		t.Fatalf("change StateRoot mode: %v", err)
	}
	assertPrivateCause(t, validateRetainedRoot(retained), CauseUnstable)
	if wrong, err := admitStateRoot(statePath); err == nil {
		_ = wrong.close()
		t.Fatal("non-0700 StateRoot admission succeeded")
	} else {
		assertPrivateCause(t, err, CausePermission)
	}
}

func TestDarwinStateRootRepeatedACLInterruptionCancelsAndClosesDescriptor(t *testing.T) {
	base := physicalDarwinTempDir(t)
	statePath := filepath.Join(base, "state")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatalf("make StateRoot: %v", err)
	}
	components, err := absolutePathComponents(statePath)
	if err != nil {
		t.Fatalf("split StateRoot path: %v", err)
	}
	// The linked ladder inspects / plus every component, retention inspects the
	// leaf once more, and StateRoot revalidation is the following inspection.
	cancelAtInspection := len(components) + 3
	inspectionCalls := 0
	aclCalls := 0
	interruptedFD := ^uintptr(0)
	ctx, cancel := context.WithCancel(context.Background())
	inspect := func(callContext context.Context, descriptor *os.File) (fileSnapshot, mountSnapshot, Digest, error) {
		inspectionCalls++
		if inspectionCalls != cancelAtInspection {
			return inspectDescriptorContext(callContext, descriptor)
		}
		interruptedFD = descriptor.Fd()
		return inspectDescriptorContextWithACLReader(
			callContext,
			descriptor,
			func(_ int, _ *unix.Attrlist, _ []byte) error {
				aclCalls++
				if aclCalls == 3 {
					cancel()
				}
				return unix.EINTR
			},
		)
	}

	root, err := admitStateRootContextWithInspect(ctx, statePath, inspect)
	if root != nil {
		_ = root.close()
		t.Fatal("canceled StateRoot admission returned a retained root")
	}
	assertPrivateCause(t, err, CauseCanceled)
	if inspectionCalls != cancelAtInspection {
		t.Fatalf("descriptor inspections = %d, want cancellation at %d", inspectionCalls, cancelAtInspection)
	}
	if aclCalls != 3 {
		t.Fatalf("interrupted fgetattrlist calls = %d, want 3", aclCalls)
	}
	if interruptedFD == ^uintptr(0) {
		t.Fatal("StateRoot cancellation did not record an interrupted descriptor")
	}
	if _, descriptorErr := unix.FcntlInt(interruptedFD, unix.F_GETFD, 0); !errors.Is(descriptorErr, unix.EBADF) {
		t.Fatalf("interrupted StateRoot descriptor remained open: %v", descriptorErr)
	}
}

func TestDarwinRetainedTaskRemovalProvesClaimedInodeAndAbsence(t *testing.T) {
	stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
	defer func() { _ = taskRoot.close() }()
	defer func() { _ = stateRoot.close() }()

	before, _, _, err := inspectDescriptor(taskRoot.descriptor)
	if err != nil {
		t.Fatalf("inspect retained task before removal: %v", err)
	}
	if err := removeRetainedTaskDirectory(context.Background(), stateRoot, taskRoot, taskName); err != nil {
		t.Fatalf("remove retained task: %v", err)
	}
	after, _, _, err := inspectDescriptor(taskRoot.descriptor)
	if err != nil {
		t.Fatalf("retained task descriptor was not kept open: %v", err)
	}
	if !sameFilesystemObject(before.identity, after.identity) {
		t.Fatal("retained task descriptor identity changed after removal")
	}
	if path, pathErr := retainedDescriptorPath(taskRoot.descriptor); pathErr != nil || path != taskRoot.path {
		t.Fatalf("removed retained descriptor path = (%q, %v), want (%q, nil)", path, pathErr, taskRoot.path)
	}
	if _, err := taskRoot.root.Lstat("."); err != nil {
		t.Fatalf("retained task os.Root was closed during removal: %v", err)
	}
	if _, err := relativeEntryKindNoFollow(int(stateRoot.descriptor.Fd()), taskName); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final parent-FD no-follow lookup = %v, want ENOENT", err)
	}
	// On the APFS host that exposed os.Root.Remove's ambiguous behavior,
	// after.linkCount remains 2 here even though AT_REMOVEDIR and the two
	// independent path proofs succeeded. Do not replace those proofs with an
	// st_nlink == 0 check.
	if after.linkCount != 0 {
		t.Logf("retained removed directory link count is %d; link count is not the unlink proof", after.linkCount)
	}
}

func TestDarwinRetainedTaskRemovalUsesOnlyATRemoveDirAndDoesNotRetryEINTR(t *testing.T) {
	t.Parallel()

	calls := 0
	err := removeDarwinDirectoryAtWith(37, "task", func(fd int, name string, flags int) error {
		calls++
		if fd != 37 || name != "task" || flags != unix.AT_REMOVEDIR {
			t.Fatalf("unlinkat arguments = (%d, %q, %#x)", fd, name, flags)
		}
		return unix.EINTR
	})
	if !errors.Is(err, unix.EINTR) {
		t.Fatalf("unlinkat error = %v, want EINTR", err)
	}
	if calls != 1 {
		t.Fatalf("unlinkat calls = %d, want exactly 1", calls)
	}
}

func TestDarwinRetainedTaskRemovalRefusesAmbiguousSyscallOutcomes(t *testing.T) {
	t.Run("EINTR", func(t *testing.T) {
		stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
		defer func() { _ = taskRoot.close() }()
		defer func() { _ = stateRoot.close() }()

		calls := 0
		err := removeRetainedTaskDirectoryWith(
			context.Background(),
			stateRoot,
			taskRoot,
			taskName,
			func(int, string) error {
				calls++
				return unix.EINTR
			},
			retainedDescriptorPath,
		)
		assertPrivateCause(t, err, CauseUnstable)
		if calls != 1 {
			t.Fatalf("interrupted removal calls = %d, want 1", calls)
		}
		assertDarwinNamedTaskIdentity(t, stateRoot, taskRoot, taskName)
	})

	t.Run("success without removal", func(t *testing.T) {
		stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
		defer func() { _ = taskRoot.close() }()
		defer func() { _ = stateRoot.close() }()

		err := removeRetainedTaskDirectoryWith(
			context.Background(),
			stateRoot,
			taskRoot,
			taskName,
			func(int, string) error { return nil },
			retainedDescriptorPath,
		)
		assertPrivateCause(t, err, CauseIdentity)
		assertDarwinNamedTaskIdentity(t, stateRoot, taskRoot, taskName)
	})
}

func TestDarwinRetainedTaskRemovalRefusesRenameSwapAtUnlink(t *testing.T) {
	stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
	defer func() { _ = taskRoot.close() }()
	defer func() { _ = stateRoot.close() }()
	movedPath := filepath.Join(stateRoot.path, "moved-task")

	err := removeRetainedTaskDirectoryWith(
		context.Background(),
		stateRoot,
		taskRoot,
		taskName,
		func(parentFD int, name string) error {
			if err := os.Rename(taskRoot.path, movedPath); err != nil {
				t.Fatalf("move claimed task at syscall seam: %v", err)
			}
			if err := os.Mkdir(taskRoot.path, 0o700); err != nil {
				t.Fatalf("install replacement task at syscall seam: %v", err)
			}
			return unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
		},
		retainedDescriptorPath,
	)
	assertPrivateCause(t, err, CauseUnstable)

	movedInfo, err := os.Lstat(movedPath)
	if err != nil {
		t.Fatalf("claimed task inode was not preserved at moved path: %v", err)
	}
	retainedInfo, err := taskRoot.descriptor.Stat()
	if err != nil {
		t.Fatalf("stat retained claimed task: %v", err)
	}
	if !os.SameFile(movedInfo, retainedInfo) {
		t.Fatal("moved path does not retain the claimed task identity")
	}
	if _, err := relativeEntryKindNoFollow(int(stateRoot.descriptor.Fd()), taskName); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement task was not removed by injected syscall: %v", err)
	}
}

func TestDarwinRetainedTaskRemovalRefusesCoordinatedRenameMasking(t *testing.T) {
	stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
	defer func() { _ = taskRoot.close() }()
	defer func() { _ = stateRoot.close() }()
	movedPath := filepath.Join(stateRoot.path, "moved-task")
	removeCalls := 0
	descriptorPathCalls := 0

	err := removeRetainedTaskDirectoryWith(
		context.Background(),
		stateRoot,
		taskRoot,
		taskName,
		func(parentFD int, name string) error {
			removeCalls++
			if err := os.Rename(taskRoot.path, movedPath); err != nil {
				t.Fatalf("move claimed task at syscall seam: %v", err)
			}
			if err := os.Mkdir(taskRoot.path, 0o700); err != nil {
				t.Fatalf("install replacement task at syscall seam: %v", err)
			}
			if err := unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR); err != nil {
				t.Fatalf("remove replacement task at syscall seam: %v", err)
			}
			if err := os.Rename(movedPath, taskRoot.path); err != nil {
				t.Fatalf("restore claimed task before descriptor-path proof: %v", err)
			}
			return nil
		},
		func(descriptor *os.File) (string, error) {
			descriptorPathCalls++
			path, err := retainedDescriptorPath(descriptor)
			if err != nil {
				return "", err
			}
			// The first call belongs to the pre-unlink name binding. A vulnerable
			// implementation reaches a second call after the attack restores the
			// claimed task, at which point this hook can hide it before the final
			// parent-FD absence check.
			if descriptorPathCalls == 2 {
				if err := os.Rename(taskRoot.path, movedPath); err != nil {
					t.Fatalf("hide restored task after descriptor-path proof: %v", err)
				}
			}
			return path, nil
		},
	)
	assertPrivateCause(t, err, CauseUnstable)
	if removeCalls != 1 {
		t.Fatalf("removal calls = %d, want 1", removeCalls)
	}
	if descriptorPathCalls != 1 {
		t.Fatalf("descriptor-path proof calls = %d, want only pre-unlink binding", descriptorPathCalls)
	}
	assertDarwinNamedTaskIdentity(t, stateRoot, taskRoot, taskName)
}

func TestDarwinRetainedTaskRemovalRefusesInvalidNameReplacementAndContent(t *testing.T) {
	t.Run("invalid names", func(t *testing.T) {
		stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
		defer func() { _ = taskRoot.close() }()
		defer func() { _ = stateRoot.close() }()
		for _, name := range []string{
			"",
			".",
			"..",
			"nested/task",
			"/task",
			"task\x00suffix",
			"other-task",
			strings.Repeat("x", maxPathComponentBytes+1),
		} {
			if err := removeRetainedTaskDirectory(context.Background(), stateRoot, taskRoot, name); err == nil {
				t.Fatalf("invalid or unbound task name %q was removed", name)
			}
		}
		if err := removeRetainedTaskDirectoryWith(
			context.Background(),
			stateRoot,
			taskRoot,
			taskName,
			nil,
			retainedDescriptorPath,
		); err == nil {
			t.Fatal("nil task-removal syscall seam was accepted")
		}
		assertDarwinNamedTaskIdentity(t, stateRoot, taskRoot, taskName)
	})

	t.Run("nonempty", func(t *testing.T) {
		stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
		defer func() { _ = taskRoot.close() }()
		defer func() { _ = stateRoot.close() }()
		if err := taskRoot.root.WriteFile("content", nil, 0o600); err != nil {
			t.Fatalf("write nonempty task fixture: %v", err)
		}
		assertPrivateCause(
			t,
			removeRetainedTaskDirectory(context.Background(), stateRoot, taskRoot, taskName),
			CauseUnstable,
		)
		assertDarwinNamedTaskIdentity(t, stateRoot, taskRoot, taskName)
	})

	t.Run("replacement", func(t *testing.T) {
		stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
		defer func() { _ = taskRoot.close() }()
		defer func() { _ = stateRoot.close() }()
		movedPath := filepath.Join(stateRoot.path, "original-task")
		if err := os.Rename(taskRoot.path, movedPath); err != nil {
			t.Fatalf("move retained task: %v", err)
		}
		if err := os.Mkdir(taskRoot.path, 0o700); err != nil {
			t.Fatalf("create replacement task: %v", err)
		}
		if err := removeRetainedTaskDirectory(context.Background(), stateRoot, taskRoot, taskName); err == nil {
			t.Fatal("replacement task identity was removed")
		}
		if _, err := os.Lstat(taskRoot.path); err != nil {
			t.Fatalf("replacement task was not preserved: %v", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		stateRoot, taskRoot, taskName := newDarwinRetainedTaskRemovalFixture(t)
		defer func() { _ = taskRoot.close() }()
		defer func() { _ = stateRoot.close() }()
		movedPath := filepath.Join(stateRoot.path, "original-task")
		if err := os.Rename(taskRoot.path, movedPath); err != nil {
			t.Fatalf("move retained task: %v", err)
		}
		if err := os.Symlink(filepath.Base(movedPath), taskRoot.path); err != nil {
			t.Fatalf("create task symlink: %v", err)
		}
		if err := removeRetainedTaskDirectory(context.Background(), stateRoot, taskRoot, taskName); err == nil {
			t.Fatal("task symlink substitution was removed")
		}
		info, err := os.Lstat(taskRoot.path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("task symlink was not preserved: info=%v err=%v", info, err)
		}
	})
}

func TestDarwinRetainedDescriptorPathRefusesMalformedAndInterruptedResults(t *testing.T) {
	t.Parallel()

	descriptor, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open descriptor fixture: %v", err)
	}
	defer func() { _ = descriptor.Close() }()
	if _, err := retainedDescriptorPathWith(descriptor, func(*os.File, []byte) error {
		return unix.EINTR
	}); !errors.Is(err, unix.EINTR) {
		t.Fatalf("interrupted F_GETPATH = %v, want EINTR", err)
	}
	if _, err := retainedDescriptorPathWith(descriptor, func(_ *os.File, buffer []byte) error {
		for index := range buffer {
			buffer[index] = 'x'
		}
		return nil
	}); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("unterminated F_GETPATH = %v, want invalid", err)
	}
	if _, err := retainedDescriptorPathWith(descriptor, func(*os.File, []byte) error {
		return nil
	}); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("empty F_GETPATH = %v, want invalid", err)
	}
}

func TestDarwinMountSnapshotPolicyAndVisibleMask(t *testing.T) {
	t.Parallel()

	filesystem := unix.Statfs_t{
		Flags: unix.MNT_LOCAL | unix.MNT_UPDATE,
	}
	filesystem.Fsid.Val = [2]int32{-7, 9}
	mount, err := darwinMountSnapshot(filesystem)
	if err != nil {
		t.Fatalf("admit local ownership-enforcing mount: %v", err)
	}
	if mount.filesystem != filesystem.Fsid.Val {
		t.Fatalf("FSID = %v, want %v", mount.filesystem, filesystem.Fsid.Val)
	}
	if mount.flags != unix.MNT_LOCAL {
		t.Fatalf("visible flags = %#x, want local only", mount.flags)
	}

	filesystem.Flags = 0
	if _, err := darwinMountSnapshot(filesystem); err == nil {
		t.Fatal("nonlocal mount succeeded")
	} else {
		assertPrivateCause(t, err, CauseUnsupported)
	}
	filesystem.Flags = unix.MNT_LOCAL | unix.MNT_IGNORE_OWNERSHIP
	if _, err := darwinMountSnapshot(filesystem); err == nil {
		t.Fatal("ownership-ignoring mount succeeded")
	} else {
		assertPrivateCause(t, err, CauseUnsupported)
	}
}

func physicalDarwinTempDir(t *testing.T) string {
	t.Helper()
	physical, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temporary directory: %v", err)
	}
	return physical
}

func newDarwinRetainedTaskRemovalFixture(
	t *testing.T,
) (*retainedDirectory, *retainedDirectory, string) {
	t.Helper()
	base := physicalDarwinTempDir(t)
	statePath := filepath.Join(base, "state")
	if err := os.Mkdir(statePath, 0o700); err != nil {
		t.Fatalf("make retained-removal StateRoot: %v", err)
	}
	stateRoot, err := admitStateRoot(statePath)
	if err != nil {
		t.Fatalf("admit retained-removal StateRoot: %v", err)
	}
	taskName := "task"
	if err := stateRoot.root.Mkdir(taskName, 0o700); err != nil {
		_ = stateRoot.close()
		t.Fatalf("make retained-removal task root: %v", err)
	}
	taskRoot, err := openRetainedDirectory(filepath.Join(statePath, taskName))
	if err != nil {
		_ = stateRoot.close()
		t.Fatalf("retain task root: %v", err)
	}
	return stateRoot, taskRoot, taskName
}

func assertDarwinNamedTaskIdentity(
	t *testing.T,
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
) {
	t.Helper()
	named, err := stateRoot.root.Lstat(taskName)
	if err != nil {
		t.Fatalf("stat named task: %v", err)
	}
	retained, err := taskRoot.descriptor.Stat()
	if err != nil {
		t.Fatalf("stat retained task: %v", err)
	}
	if !os.SameFile(named, retained) {
		t.Fatal("named task no longer has the retained task identity")
	}
}

func makeDarwinTestFilesec(entryCount, aclFlags uint32, entries []darwinTestACE) []byte {
	record := make([]byte, darwinFilesecHeaderSize+len(entries)*darwinACERecordSize)
	binary.LittleEndian.PutUint32(record[0:4], darwinFilesecMagic)
	binary.LittleEndian.PutUint32(record[36:40], entryCount)
	binary.LittleEndian.PutUint32(record[40:44], aclFlags)
	for index, entry := range entries {
		offset := darwinFilesecHeaderSize + index*darwinACERecordSize
		binary.LittleEndian.PutUint32(record[offset+16:offset+20], entry.flags)
		binary.LittleEndian.PutUint32(record[offset+20:offset+24], entry.rights)
	}
	return record
}

func makeDarwinTestAttrResult(attribute []byte) []byte {
	result := make([]byte, darwinAttrResultHeaderSize+len(attribute))
	binary.LittleEndian.PutUint32(result[0:4], uint32(len(result)))
	binary.LittleEndian.PutUint32(result[4:8], 8)
	binary.LittleEndian.PutUint32(result[8:12], uint32(len(attribute)))
	copy(result[12:], attribute)
	return result
}

func assertDigestHex(t *testing.T, got Digest, wantHex string) {
	t.Helper()
	wantBytes, err := hex.DecodeString(wantHex)
	if err != nil {
		t.Fatalf("decode expected digest: %v", err)
	}
	var want Digest
	copy(want[:], wantBytes)
	if got != want {
		t.Fatalf("digest = %x, want %s", got, wantHex)
	}
}

func assertPrivateCause(t *testing.T, err error, want CauseCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s refusal", want)
	}
	var failure *privateFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error %T is not private failure: %v", err, err)
	}
	if slices.Contains(failure.causes, want) {
		return
	}
	t.Fatalf("causes %v do not contain %s", failure.causes, want)
}
