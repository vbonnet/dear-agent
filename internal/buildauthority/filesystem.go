package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ownerPolicy uint8

const (
	ownerRootOrEffective ownerPolicy = iota + 1
	ownerEffectiveOnly
)

type entryKind uint8

const (
	entryDirectory entryKind = iota + 1
	entryRegular
	entrySymlink
)

type treeLimits struct {
	maxEntries   uint64
	maxBytes     uint64
	maxFileBytes uint64
}

type treePolicy struct {
	domain        string
	owners        ownerPolicy
	allowSymlinks bool
	limits        treeLimits
}

type mountSnapshot struct {
	filesystem [2]int32
	flags      uint32
}

type fileSnapshot struct {
	identity   FileIdentity
	linkCount  uint64
	gid        uint32
	rdev       uint64
	size       int64
	mtimeSec   int64
	mtimeNsec  int64
	ctimeSec   int64
	ctimeNsec  int64
	birthSec   int64
	birthNsec  int64
	flags      uint32
	generation uint32
}

// authorityPathClaim is the security-relevant claim for one directory in the
// descriptor-linked path from / through an authority leaf. Directory size,
// link count, modification time, and change time are deliberately excluded
// because unrelated sibling churn must not invalidate an authority. The
// remaining stable inode and security metadata is retained explicitly.
type authorityPathClaim struct {
	identity   FileIdentity
	gid        uint32
	rdev       uint64
	birthSec   int64
	birthNsec  int64
	flags      uint32
	generation uint32
	mount      mountSnapshot
	aclDigest  Digest
}

type retainedDirectory struct {
	path       string
	root       *os.Root
	descriptor *os.File
	snapshot   fileSnapshot
	mount      mountSnapshot
	aclDigest  Digest
	pathClaims []authorityPathClaim
}

type entrySnapshot struct {
	path         string
	kind         entryKind
	file         fileSnapshot
	aclDigest    Digest
	content      Digest
	linkText     string
	targetDigest Digest
}

type treeCapture struct {
	root    *retainedDirectory
	policy  treePolicy
	digest  Digest
	entries []entrySnapshot
}

const directoryReadBatchSize = 128

type treeCaptureBuilder struct {
	ctx        context.Context
	root       *retainedDirectory
	policy     treePolicy
	readDir    directoryReadFunc
	entries    []entrySnapshot
	totalBytes uint64
}

type directoryReadFunc func(*os.File, int) ([]os.DirEntry, error)
type descriptorInspectFunc func(context.Context, *os.File) (fileSnapshot, mountSnapshot, Digest, error)
type retainedDirectoryRemoveFunc func(int, string) error
type retainedDescriptorPathFunc func(*os.File) (string, error)

type retainedTaskRemovalSnapshot struct {
	file      fileSnapshot
	mount     mountSnapshot
	aclDigest Digest
}

func readOpenedSymlink(descriptor *os.File, limit uint64) (string, error) {
	return readOpenedSymlinkContext(context.Background(), descriptor, limit)
}

func inspectDescriptor(descriptor *os.File) (fileSnapshot, mountSnapshot, Digest, error) {
	return inspectDescriptorContext(context.Background(), descriptor)
}

var (
	errUnsupportedAuthorityEntry = errors.New("unsupported authority entry kind")
	errAuthorityKindMismatch     = errors.New("authority entry kind mismatch")
)

func gorootPolicy() treePolicy {
	return treePolicy{
		domain:        domainGOROOT,
		owners:        ownerRootOrEffective,
		allowSymlinks: true,
		limits: treeLimits{
			maxEntries:   maxGOROOTEntries,
			maxBytes:     maxGOROOTBytes,
			maxFileBytes: maxGOROOTFileBytes,
		},
	}
}

func gomodcachePolicy() treePolicy {
	return treePolicy{
		domain: domainGOMODCACHE,
		owners: ownerRootOrEffective,
		limits: treeLimits{
			maxEntries:   maxGOMODCACHEEntries,
			maxBytes:     maxGOMODCACHEBytes,
			maxFileBytes: maxGOMODCACHEFileBytes,
		},
	}
}

func admitAuthorityTree(ctx context.Context, path string, policy treePolicy) (*treeCapture, error) {
	if err := checkContext(ctx, "admit authority tree"); err != nil {
		return nil, err
	}
	if err := validateAuthorityPath(path); err != nil {
		return nil, err
	}
	root, err := openRetainedDirectoryContext(ctx, path)
	if err != nil {
		return nil, err
	}
	capture, err := captureRetainedTree(ctx, root, policy)
	if err == nil {
		return capture, nil
	}
	return nil, joinFilesystemFailures(
		err,
		closeDescriptorFailure(root.close()),
	)
}

func admitStateRoot(path string) (*retainedDirectory, error) {
	return admitStateRootContext(context.Background(), path)
}

func admitStateRootContext(ctx context.Context, path string) (*retainedDirectory, error) {
	return admitStateRootContextWithInspect(ctx, path, inspectDescriptorContext)
}

func admitStateRootContextWithInspect(
	ctx context.Context,
	path string,
	inspect descriptorInspectFunc,
) (*retainedDirectory, error) {
	if err := checkContext(ctx, "admit StateRoot"); err != nil {
		return nil, err
	}
	if err := validateAuthorityPath(path); err != nil {
		return nil, err
	}
	root, err := openRetainedDirectoryContextWithInspect(ctx, path, inspect)
	if err != nil {
		return nil, err
	}
	if err := validateRetainedRootContextWithInspect(ctx, root, ownerEffectiveOnly, inspect); err != nil {
		return nil, joinFilesystemFailures(
			err,
			closeDescriptorFailure(root.close()),
		)
	}
	if !snapshotIsDirectory(root.snapshot) || root.snapshot.identity.Mode&0o7777 != 0o700 {
		return nil, joinFilesystemFailures(
			fail(CausePermission, "StateRoot policy mismatch"),
			closeDescriptorFailure(root.close()),
		)
	}
	return root, nil
}

func openRetainedDirectory(path string) (*retainedDirectory, error) {
	return openRetainedDirectoryContext(context.Background(), path)
}

func openRetainedDirectoryContext(ctx context.Context, path string) (*retainedDirectory, error) {
	return openRetainedDirectoryContextWithInspect(ctx, path, inspectDescriptorContext)
}

func openRetainedDirectoryContextWithInspect(
	ctx context.Context,
	path string,
	inspect descriptorInspectFunc,
) (*retainedDirectory, error) {
	if err := checkContext(ctx, "open retained authority root"); err != nil {
		return nil, err
	}
	if inspect == nil {
		return nil, fail(CauseInternalInvariant, "missing descriptor inspector")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "open retained authority root")
	}
	return openRetainedDirectoryFromRootContextWithInspect(ctx, path, root, inspect)
}

// openRetainedDirectoryFromRoot takes ownership of root. Keeping this seam
// explicit lets tests substitute the pathname after the first open and prove
// that the second, descriptor-linked no-follow open rejects the substitution.
func openRetainedDirectoryFromRoot(path string, root *os.Root) (*retainedDirectory, error) {
	return openRetainedDirectoryFromRootContext(context.Background(), path, root)
}

func openRetainedDirectoryFromRootContext(
	ctx context.Context,
	path string,
	root *os.Root,
) (*retainedDirectory, error) {
	return openRetainedDirectoryFromRootContextWithInspect(ctx, path, root, inspectDescriptorContext)
}

func openRetainedDirectoryFromRootContextWithInspect(
	ctx context.Context,
	path string,
	root *os.Root,
	inspect descriptorInspectFunc,
) (*retainedDirectory, error) {
	if root == nil {
		return nil, fail(CauseInternalInvariant, "missing retained authority root")
	}
	if inspect == nil {
		return nil, joinFilesystemFailures(
			fail(CauseInternalInvariant, "missing descriptor inspector"),
			closeDescriptorFailure(root.Close()),
		)
	}
	if err := checkContext(ctx, "open retained authority root"); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(root.Close()))
	}
	descriptor, pathClaims, err := openAuthorityPathContextWithInspect(ctx, path, inspect)
	if err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(root.Close()))
	}
	directory, inspectErr := inspectRetainedDirectoryContextWithInspect(ctx, path, descriptor, root, pathClaims, inspect)
	if inspectErr == nil {
		return directory, nil
	}
	return nil, joinFilesystemFailures(
		inspectErr,
		closeDescriptorFailure(descriptor.Close()),
		closeDescriptorFailure(root.Close()),
	)
}

func inspectRetainedDirectory(
	path string,
	descriptor *os.File,
	root *os.Root,
	pathClaims []authorityPathClaim,
) (*retainedDirectory, error) {
	return inspectRetainedDirectoryContext(context.Background(), path, descriptor, root, pathClaims)
}

func inspectRetainedDirectoryContext(
	ctx context.Context,
	path string,
	descriptor *os.File,
	root *os.Root,
	pathClaims []authorityPathClaim,
) (*retainedDirectory, error) {
	return inspectRetainedDirectoryContextWithInspect(
		ctx,
		path,
		descriptor,
		root,
		pathClaims,
		inspectDescriptorContext,
	)
}

func inspectRetainedDirectoryContextWithInspect(
	ctx context.Context,
	path string,
	descriptor *os.File,
	root *os.Root,
	pathClaims []authorityPathClaim,
	inspect descriptorInspectFunc,
) (*retainedDirectory, error) {
	if descriptor == nil || root == nil {
		return nil, fail(CauseInternalInvariant, "missing retained authority root handles")
	}
	if inspect == nil {
		return nil, fail(CauseInternalInvariant, "missing descriptor inspector")
	}
	if len(pathClaims) == 0 {
		return nil, fail(CauseInternalInvariant, "missing retained authority path claims")
	}
	snapshot, mount, aclDigest, err := inspect(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	info, err := root.Lstat(".")
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "stat retained authority root")
	}
	descriptorInfo, err := descriptor.Stat()
	if err != nil || !os.SameFile(descriptorInfo, info) {
		return nil, failWithFilesystemCauses(err, CauseIdentity, "retained authority root identity mismatch")
	}
	if current := makeAuthorityPathClaim(snapshot, mount, aclDigest); current != pathClaims[len(pathClaims)-1] {
		return nil, fail(CauseUnstable, "authority leaf changed during retention")
	}
	return &retainedDirectory{
		path:       path,
		root:       root,
		descriptor: descriptor,
		snapshot:   snapshot,
		mount:      mount,
		aclDigest:  aclDigest,
		pathClaims: append([]authorityPathClaim(nil), pathClaims...),
	}, nil
}

func openAuthorityPath(path string) (*os.File, []authorityPathClaim, error) {
	return openAuthorityPathContext(context.Background(), path)
}

func openAuthorityPathContext(ctx context.Context, path string) (*os.File, []authorityPathClaim, error) {
	return openAuthorityPathContextWithInspect(ctx, path, inspectDescriptorContext)
}

func openAuthorityPathContextWithInspect(
	ctx context.Context,
	path string,
	inspect descriptorInspectFunc,
) (*os.File, []authorityPathClaim, error) {
	if err := checkContext(ctx, "open authority path"); err != nil {
		return nil, nil, err
	}
	if inspect == nil {
		return nil, nil, fail(CauseInternalInvariant, "missing descriptor inspector")
	}
	components, err := absolutePathComponents(path)
	if err != nil {
		return nil, nil, failWithFilesystemCauses(err, CauseInvalidRequest, "split authority path")
	}
	descriptor, err := openAbsoluteNoFollow(string(filepath.Separator), entryDirectory)
	if err != nil {
		return nil, nil, failWithFilesystemCauses(err, classifyPathError(err), "open authority path root")
	}
	claims := make([]authorityPathClaim, 0, len(components)+1)
	for _, component := range components {
		if contextErr := checkContext(ctx, "open authority path"); contextErr != nil {
			return nil, nil, joinFilesystemFailures(contextErr, closeDescriptorFailure(descriptor.Close()))
		}
		claim, inspectErr := inspectAuthorityPathDirectoryContextWithInspect(ctx, descriptor, inspect)
		if inspectErr != nil {
			return nil, nil, joinFilesystemFailures(inspectErr, closeDescriptorFailure(descriptor.Close()))
		}
		claims = append(claims, claim)

		next, openErr := openRelativeNoFollow(int(descriptor.Fd()), component, entryDirectory)
		if openErr != nil {
			return nil, nil, joinFilesystemFailures(
				failWithFilesystemCauses(openErr, classifyPathError(openErr), "open authority path component"),
				closeDescriptorFailure(descriptor.Close()),
			)
		}
		if closeErr := closeDescriptorFailure(descriptor.Close()); closeErr != nil {
			return nil, nil, joinFilesystemFailures(closeErr, closeDescriptorFailure(next.Close()))
		}
		descriptor = next
	}
	claim, inspectErr := inspectAuthorityPathDirectoryContextWithInspect(ctx, descriptor, inspect)
	if inspectErr != nil {
		return nil, nil, joinFilesystemFailures(inspectErr, closeDescriptorFailure(descriptor.Close()))
	}
	claims = append(claims, claim)
	return descriptor, claims, nil
}

func inspectAuthorityPathDirectory(descriptor *os.File) (authorityPathClaim, error) {
	return inspectAuthorityPathDirectoryContext(context.Background(), descriptor)
}

func inspectAuthorityPathDirectoryContext(
	ctx context.Context,
	descriptor *os.File,
) (authorityPathClaim, error) {
	return inspectAuthorityPathDirectoryContextWithInspect(ctx, descriptor, inspectDescriptorContext)
}

func inspectAuthorityPathDirectoryContextWithInspect(
	ctx context.Context,
	descriptor *os.File,
	inspect descriptorInspectFunc,
) (authorityPathClaim, error) {
	if inspect == nil {
		return authorityPathClaim{}, fail(CauseInternalInvariant, "missing descriptor inspector")
	}
	snapshot, mount, aclDigest, err := inspect(ctx, descriptor)
	if err != nil {
		return authorityPathClaim{}, err
	}
	if err := validateProtected(snapshot, ownerRootOrEffective); err != nil {
		return authorityPathClaim{}, err
	}
	if !snapshotIsDirectory(snapshot) {
		return authorityPathClaim{}, fail(CauseUnsupported, "authority path component is not a directory")
	}
	return makeAuthorityPathClaim(snapshot, mount, aclDigest), nil
}

func makeAuthorityPathClaim(snapshot fileSnapshot, mount mountSnapshot, aclDigest Digest) authorityPathClaim {
	return authorityPathClaim{
		identity:   snapshot.identity,
		gid:        snapshot.gid,
		rdev:       snapshot.rdev,
		birthSec:   snapshot.birthSec,
		birthNsec:  snapshot.birthNsec,
		flags:      snapshot.flags,
		generation: snapshot.generation,
		mount:      mount,
		aclDigest:  aclDigest,
	}
}

func absolutePathComponents(path string) ([]string, error) {
	separator := string(filepath.Separator)
	if path == separator {
		return nil, nil
	}
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fs.ErrInvalid
	}
	components := make([]string, 0)
	for component := range strings.SplitSeq(strings.TrimPrefix(path, separator), separator) {
		if component == "" || component == "." || component == ".." {
			return nil, fs.ErrInvalid
		}
		components = append(components, component)
	}
	return components, nil
}

func (directory *retainedDirectory) authorityIdentities() []FileIdentity {
	if directory == nil {
		return nil
	}
	identities := make([]FileIdentity, len(directory.pathClaims))
	for index := range directory.pathClaims {
		identities[index] = directory.pathClaims[index].identity
	}
	return identities
}

func (directory *retainedDirectory) close() error {
	if directory == nil {
		return nil
	}
	var closeErrors []error
	if directory.root != nil {
		closeErrors = append(closeErrors, directory.root.Close())
	}
	if directory.descriptor != nil {
		closeErrors = append(closeErrors, directory.descriptor.Close())
	}
	return errors.Join(closeErrors...)
}

type retainedLeafRemoveFunc func() error

type retainedLeafRemovalSnapshot struct {
	file      fileSnapshot
	mount     mountSnapshot
	aclDigest Digest
}

// removeRetainedWorkspaceLeaf removes one claimed regular file or symlink
// through its retained parent. The opened leaf remains live across removal so
// the caller accepts success only after the claimed inode's link count moves
// from one to zero and the name is proved absent.
func removeRetainedWorkspaceLeaf(
	ctx context.Context,
	parent *retainedDirectory,
	expected workspaceEntryClaim,
) error {
	if parent == nil || parent.descriptor == nil {
		return fail(CauseInternalInvariant, "missing retained workspace parent")
	}
	descriptor, err := openRelativeNoFollow(int(parent.descriptor.Fd()), expected.name, expected.kind)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "open claimed workspace leaf for removal")
	}
	removeErr := removeRetainedWorkspaceLeafWith(
		ctx,
		parent,
		expected,
		descriptor,
		func() error { return parent.root.Remove(expected.name) },
	)
	return joinFilesystemFailures(removeErr, closeDescriptorFailure(descriptor.Close()))
}

func removeRetainedWorkspaceLeafWith(
	ctx context.Context,
	parent *retainedDirectory,
	expected workspaceEntryClaim,
	descriptor *os.File,
	remove retainedLeafRemoveFunc,
) error {
	before, err := prepareRetainedWorkspaceLeafRemoval(ctx, parent, expected, descriptor, remove)
	if err != nil {
		return err
	}
	// Do not retry a non-nil result: a mutating remove may already have
	// completed, so every error is an ambiguous cleanup result.
	if err := remove(); err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "remove claimed workspace leaf")
	}
	return proveRetainedWorkspaceLeafRemoval(ctx, parent, expected, descriptor, before)
}

func prepareRetainedWorkspaceLeafRemoval(
	ctx context.Context,
	parent *retainedDirectory,
	expected workspaceEntryClaim,
	descriptor *os.File,
	remove retainedLeafRemoveFunc,
) (retainedLeafRemovalSnapshot, error) {
	if err := validateRetainedWorkspaceLeafRemovalRequest(parent, expected, descriptor, remove); err != nil {
		return retainedLeafRemovalSnapshot{}, err
	}
	if err := validateRetainedMutableDirectory(ctx, parent); err != nil {
		return retainedLeafRemovalSnapshot{}, err
	}
	before, mount, aclDigest, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return retainedLeafRemovalSnapshot{}, err
	}
	if err := validateRetainedWorkspaceLeafClaim(parent, expected, before, mount, aclDigest); err != nil {
		return retainedLeafRemovalSnapshot{}, err
	}
	if expected.hasContent {
		if err := validateRetainedRegularFileContent(
			ctx,
			descriptor,
			expected.snapshot,
			expected.mount,
			expected.aclDigest,
			expected.contentHash,
			maximumSpoolBytes,
		); err != nil {
			return retainedLeafRemovalSnapshot{}, err
		}
	}
	if err := validateNamedRetainedWorkspaceLeaf(parent, expected.name, descriptor); err != nil {
		return retainedLeafRemovalSnapshot{}, err
	}
	if err := checkContext(ctx, "remove claimed workspace leaf"); err != nil {
		return retainedLeafRemovalSnapshot{}, err
	}
	return retainedLeafRemovalSnapshot{file: before, mount: mount, aclDigest: aclDigest}, nil
}

func validateRetainedWorkspaceLeafRemovalRequest(
	parent *retainedDirectory,
	expected workspaceEntryClaim,
	descriptor *os.File,
	remove retainedLeafRemoveFunc,
) error {
	if parent == nil || parent.root == nil || parent.descriptor == nil || descriptor == nil || remove == nil {
		return fail(CauseInternalInvariant, "missing retained workspace-leaf removal authority")
	}
	if err := validateRetainedTaskName(expected.name); err != nil {
		return err
	}
	if expected.kind != entryRegular && expected.kind != entrySymlink {
		return fail(CauseInternalInvariant, "workspace-leaf removal requires a regular file or symlink")
	}
	if expected.hasContent && expected.kind != entryRegular {
		return fail(CauseInternalInvariant, "workspace-leaf content claim requires a regular file")
	}
	return nil
}

func validateRetainedWorkspaceLeafClaim(
	parent *retainedDirectory,
	expected workspaceEntryClaim,
	before fileSnapshot,
	mount mountSnapshot,
	aclDigest Digest,
) error {
	if snapshotKind(before) != expected.kind || before != expected.snapshot ||
		mount != expected.mount || aclDigest != expected.aclDigest {
		return fail(CauseIdentity, "claimed workspace leaf changed before removal")
	}
	if before.linkCount != 1 {
		return fail(CauseIdentity, "claimed workspace leaf has another link")
	}
	uid, err := effectiveUserID()
	if err != nil {
		return err
	}
	if before.identity.UID != uid {
		return fail(CausePermission, "claimed workspace leaf ownership refused")
	}
	if mount != parent.mount || before.identity.Device != parent.snapshot.identity.Device {
		return fail(CauseUnsupported, "claimed workspace leaf mount changed")
	}
	switch expected.kind {
	case entryDirectory:
		return fail(CauseInternalInvariant, "workspace-leaf removal received a directory")
	case entryRegular:
		if before.identity.Mode&0o7777 != 0o600 {
			return fail(CausePermission, "claimed workspace file mode refused")
		}
	case entrySymlink:
		if before.identity.Mode&0o7000 != 0 {
			return fail(CausePermission, "claimed workspace symlink mode refused")
		}
	}
	return nil
}

func validateNamedRetainedWorkspaceLeaf(
	parent *retainedDirectory,
	name string,
	descriptor *os.File,
) error {
	namedInfo, namedErr := parent.root.Lstat(name)
	retainedInfo, retainedErr := descriptor.Stat()
	if namedErr != nil || retainedErr != nil || !os.SameFile(namedInfo, retainedInfo) {
		return failWithFilesystemCauses(
			errors.Join(namedErr, retainedErr),
			CauseIdentity,
			"claimed workspace name differs from retained leaf",
		)
	}
	return nil
}

func proveRetainedWorkspaceLeafRemoval(
	ctx context.Context,
	parent *retainedDirectory,
	expected workspaceEntryClaim,
	descriptor *os.File,
	before retainedLeafRemovalSnapshot,
) error {
	after, afterMount, afterACL, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return err
	}
	if err := proveRetainedWorkspaceLeafContentAfterRemoval(
		ctx,
		expected,
		descriptor,
		after,
		afterMount,
		afterACL,
	); err != nil {
		return err
	}
	if !retainedWorkspaceLeafUnlinked(before, after, afterMount, afterACL) {
		return fail(CauseIdentity, "claimed workspace inode unlink was not proved")
	}
	if err := validateRetainedMutableDirectory(ctx, parent); err != nil {
		return err
	}
	if _, err := relativeEntryKindNoFollow(int(parent.descriptor.Fd()), expected.name); !errors.Is(err, fs.ErrNotExist) {
		return failWithFilesystemCauses(err, CauseIdentity, "claimed workspace leaf absence not proved")
	}
	return nil
}

func proveRetainedWorkspaceLeafContentAfterRemoval(
	ctx context.Context,
	expected workspaceEntryClaim,
	descriptor *os.File,
	after fileSnapshot,
	afterMount mountSnapshot,
	afterACL Digest,
) error {
	if !expected.hasContent {
		return nil
	}
	contentHash, err := hashRetainedFileContent(ctx, descriptor, after.size, maximumSpoolBytes)
	if err != nil {
		return err
	}
	afterHash, hashMount, hashACL, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return err
	}
	if contentHash != expected.contentHash || afterHash != after ||
		hashMount != afterMount || hashACL != afterACL {
		return fail(CauseUnstable, "claimed workspace content changed through unlink")
	}
	return nil
}

func retainedWorkspaceLeafUnlinked(
	before retainedLeafRemovalSnapshot,
	after fileSnapshot,
	afterMount mountSnapshot,
	afterACL Digest,
) bool {
	return makeAuthorityPathClaim(after, afterMount, afterACL) ==
		makeAuthorityPathClaim(before.file, before.mount, before.aclDigest) &&
		after.size == before.file.size && after.mtimeSec == before.file.mtimeSec &&
		after.mtimeNsec == before.file.mtimeNsec && after.linkCount == 0
}

// removeRetainedTaskDirectory removes one already-proved-empty task child.
// Its interface accepts only the two retained authorities and their bound leaf
// name: syscall selection and post-removal proof remain inside the filesystem
// module. The task handles intentionally remain open for the caller's ordered
// cleanup after this function returns.
func removeRetainedTaskDirectory(
	ctx context.Context,
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
) error {
	return removeRetainedTaskDirectoryWith(
		ctx,
		stateRoot,
		taskRoot,
		taskName,
		removeRetainedDirectoryAt,
		retainedDescriptorPath,
	)
}

// removeRetainedTaskDirectoryWith is an internal syscall seam, not a caller
// interface. Tests use it to force ambiguous syscall results and a rename/swap
// exactly between the final identity check and unlinkat.
func removeRetainedTaskDirectoryWith(
	ctx context.Context,
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
	remove retainedDirectoryRemoveFunc,
	descriptorPath retainedDescriptorPathFunc,
) error {
	if remove == nil || descriptorPath == nil {
		return fail(CauseInternalInvariant, "missing retained task-removal primitive")
	}
	before, err := prepareRetainedTaskRemoval(ctx, stateRoot, taskRoot, taskName, descriptorPath)
	if err != nil {
		return err
	}

	// EINTR is deliberately not retried. Darwin may have completed a mutating
	// syscall before reporting interruption, so every non-nil result is an
	// ambiguous removal and must fail closed.
	if err := remove(int(stateRoot.descriptor.Fd()), taskName); err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "remove retained task directory")
	}
	return proveRetainedTaskRemoval(ctx, stateRoot, taskRoot, taskName, descriptorPath, before)
}

func prepareRetainedTaskRemoval(
	ctx context.Context,
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
	descriptorPath retainedDescriptorPathFunc,
) (retainedTaskRemovalSnapshot, error) {
	if err := validateRetainedTaskRemovalRequest(ctx, stateRoot, taskRoot, taskName); err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	if err := validateRetainedMutableDirectory(ctx, stateRoot); err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	if err := validateRetainedMutableDirectory(ctx, taskRoot); err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	if err := requireRetainedDirectoryEmpty(ctx, taskRoot); err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	if err := validateNamedRetainedTask(ctx, stateRoot, taskRoot, taskName, descriptorPath); err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	if err := checkContext(ctx, "remove retained task directory"); err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	file, mount, aclDigest, err := inspectRetainedMutableDirectoryDescriptor(ctx, taskRoot)
	if err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	if err := checkContext(ctx, "remove retained task directory"); err != nil {
		return retainedTaskRemovalSnapshot{}, err
	}
	return retainedTaskRemovalSnapshot{file: file, mount: mount, aclDigest: aclDigest}, nil
}

func proveRetainedTaskRemoval(
	ctx context.Context,
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
	descriptorPath retainedDescriptorPathFunc,
	before retainedTaskRemovalSnapshot,
) error {
	taskAfter, taskMountAfter, taskACLAfter, err := inspectRetainedMutableDirectoryDescriptor(ctx, taskRoot)
	if err != nil {
		return err
	}
	// A successful rmdir leaves every captured field of the retained APFS
	// directory unchanged. Renaming the claimed inode out and back does not:
	// at minimum its change time advances. Requiring the complete descriptor
	// snapshot here prevents a replacement from being unlinked while later
	// pathname checks are coordinated to make the claimed inode look removed.
	if before.file != taskAfter || before.mount != taskMountAfter || before.aclDigest != taskACLAfter {
		return fail(CauseUnstable, "retained task descriptor changed during removal")
	}
	path, err := descriptorPath(taskRoot.descriptor)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "resolve removed task descriptor path")
	}
	// On APFS an open directory keeps st_nlink == 2 after successful rmdir,
	// so link count is not an unlink proof. F_GETPATH instead distinguishes the
	// correct, now-absent historical path from a claimed inode renamed elsewhere.
	if path != taskRoot.path {
		return fail(CauseIdentity, "removed task descriptor path changed")
	}
	if err := validateRetainedMutableDirectory(ctx, stateRoot); err != nil {
		return err
	}
	if err := proveRetainedChildAbsent(stateRoot, taskName); err != nil {
		return err
	}
	return nil
}

func validateRetainedTaskRemovalRequest(
	ctx context.Context,
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
) error {
	if err := checkContext(ctx, "validate retained task removal"); err != nil {
		return err
	}
	if err := validateRetainedTaskRemovalHandles(stateRoot, taskRoot); err != nil {
		return err
	}
	if err := validateRetainedTaskName(taskName); err != nil {
		return err
	}
	return validateRetainedTaskBinding(stateRoot, taskRoot, taskName)
}

func validateRetainedTaskRemovalHandles(stateRoot, taskRoot *retainedDirectory) error {
	if stateRoot == nil || stateRoot.root == nil || stateRoot.descriptor == nil ||
		taskRoot == nil || taskRoot.root == nil || taskRoot.descriptor == nil {
		return fail(CauseInternalInvariant, "missing retained task-removal handles")
	}
	return nil
}

func validateRetainedTaskName(taskName string) error {
	if taskName == "" || taskName == "." || taskName == ".." ||
		filepath.IsAbs(taskName) || filepath.Clean(taskName) != taskName ||
		filepath.Base(taskName) != taskName || strings.ContainsRune(taskName, '\x00') {
		return fail(CauseInvalidRequest, "retained task name is not one path component")
	}
	if len([]byte(taskName)) > maxPathComponentBytes {
		return fail(CauseLimit, "retained task name is too long")
	}
	return nil
}

func validateRetainedTaskBinding(
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
) error {
	if stateRoot.path == "" || taskRoot.path != filepath.Join(stateRoot.path, taskName) {
		return fail(CauseIdentity, "retained task path is not the named StateRoot child")
	}
	if sameFilesystemObject(stateRoot.snapshot.identity, taskRoot.snapshot.identity) {
		return fail(CauseIdentity, "StateRoot and task root identities alias")
	}
	if taskRoot.mount != stateRoot.mount || taskRoot.snapshot.identity.Device != stateRoot.snapshot.identity.Device {
		return fail(CauseUnsupported, "retained task crosses the StateRoot mount")
	}
	if len(stateRoot.pathClaims) == 0 || len(taskRoot.pathClaims) != len(stateRoot.pathClaims)+1 {
		return fail(CauseIdentity, "retained task path-claim depth changed")
	}
	for index := range stateRoot.pathClaims {
		if stateRoot.pathClaims[index] != taskRoot.pathClaims[index] {
			return fail(CauseIdentity, "retained task is not descended from StateRoot")
		}
	}
	last := taskRoot.pathClaims[len(taskRoot.pathClaims)-1]
	if last != makeAuthorityPathClaim(taskRoot.snapshot, taskRoot.mount, taskRoot.aclDigest) {
		return fail(CauseIdentity, "retained task leaf claim changed")
	}
	return nil
}

func validateRetainedMutableDirectory(
	ctx context.Context,
	directory *retainedDirectory,
) error {
	if err := validateRetainedMutableDirectoryDescriptor(ctx, directory); err != nil {
		return err
	}
	return revalidateAuthorityPathContextWithInspect(ctx, directory, inspectDescriptorContext)
}

func validateRetainedMutableDirectoryDescriptor(
	ctx context.Context,
	directory *retainedDirectory,
) error {
	_, _, _, err := inspectRetainedMutableDirectoryDescriptor(ctx, directory)
	return err
}

func inspectRetainedMutableDirectoryDescriptor(
	ctx context.Context,
	directory *retainedDirectory,
) (fileSnapshot, mountSnapshot, Digest, error) {
	if directory == nil || directory.root == nil || directory.descriptor == nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, fail(CauseInternalInvariant, "missing retained mutable-directory handles")
	}
	current, mount, aclDigest, err := inspectDescriptorContext(ctx, directory.descriptor)
	if err != nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, err
	}
	if makeAuthorityPathClaim(current, mount, aclDigest) !=
		makeAuthorityPathClaim(directory.snapshot, directory.mount, directory.aclDigest) {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, fail(CauseUnstable, "retained mutable-directory security claim changed")
	}
	if err := validateProtected(current, ownerEffectiveOnly); err != nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, err
	}
	if snapshotKind(current) != entryDirectory || current.identity.Mode&0o7777 != 0o700 {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, fail(CausePermission, "retained mutable-directory policy mismatch")
	}
	if mount != directory.mount || current.identity.Device != directory.snapshot.identity.Device {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, fail(CauseUnsupported, "retained mutable-directory mount changed")
	}
	rootInfo, rootErr := directory.root.Lstat(".")
	descriptorInfo, descriptorErr := directory.descriptor.Stat()
	if rootErr != nil || descriptorErr != nil || !os.SameFile(rootInfo, descriptorInfo) {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, failWithFilesystemCauses(
			errors.Join(rootErr, descriptorErr),
			CauseIdentity,
			"retained mutable-directory handle identity changed",
		)
	}
	return current, mount, aclDigest, nil
}

func requireRetainedDirectoryEmpty(ctx context.Context, directory *retainedDirectory) error {
	if err := checkContext(ctx, "prove retained task directory empty"); err != nil {
		return err
	}
	scan, err := directory.root.Open(".")
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "open retained task for empty proof")
	}
	before, beforeMount, beforeACL, beforeErr := inspectDescriptorContext(ctx, scan)
	var entries []os.DirEntry
	var readErr error
	if beforeErr == nil {
		entries, readErr = scan.ReadDir(1)
	}
	after, afterMount, afterACL, afterErr := inspectDescriptorContext(ctx, scan)
	closeErr := closeDescriptorFailure(scan.Close())
	if beforeErr != nil || afterErr != nil || closeErr != nil {
		return joinFilesystemFailures(beforeErr, afterErr, closeErr)
	}
	if len(entries) != 0 {
		return fail(CauseUnstable, "retained task directory is not empty")
	}
	if !errors.Is(readErr, io.EOF) {
		return failWithFilesystemCauses(readErr, CauseUnstable, "retained task emptiness is unproved")
	}
	if before != after || beforeMount != afterMount || beforeACL != afterACL {
		return fail(CauseUnstable, "retained task directory changed during empty proof")
	}
	if makeAuthorityPathClaim(after, afterMount, afterACL) !=
		makeAuthorityPathClaim(directory.snapshot, directory.mount, directory.aclDigest) {
		return fail(CauseIdentity, "retained task empty proof used another identity")
	}
	return nil
}

func validateNamedRetainedTask(
	ctx context.Context,
	stateRoot *retainedDirectory,
	taskRoot *retainedDirectory,
	taskName string,
	descriptorPath retainedDescriptorPathFunc,
) error {
	if err := validateRetainedMutableDirectory(ctx, stateRoot); err != nil {
		return err
	}
	if err := validateRetainedMutableDirectory(ctx, taskRoot); err != nil {
		return err
	}
	kind, err := relativeEntryKindNoFollow(int(stateRoot.descriptor.Fd()), taskName)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "inspect named retained task")
	}
	if kind != entryDirectory {
		return fail(CauseIdentity, "named retained task is not a directory")
	}
	namedInfo, namedErr := stateRoot.root.Lstat(taskName)
	retainedInfo, retainedErr := taskRoot.descriptor.Stat()
	if namedErr != nil || retainedErr != nil || !os.SameFile(namedInfo, retainedInfo) {
		return failWithFilesystemCauses(
			errors.Join(namedErr, retainedErr),
			CauseIdentity,
			"named task differs from retained task identity",
		)
	}
	path, err := descriptorPath(taskRoot.descriptor)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "resolve retained task descriptor path")
	}
	if path != taskRoot.path {
		return fail(CauseIdentity, "retained task descriptor path changed")
	}
	return nil
}

func proveRetainedChildAbsent(stateRoot *retainedDirectory, taskName string) error {
	_, err := relativeEntryKindNoFollow(int(stateRoot.descriptor.Fd()), taskName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "prove removed task absence")
	}
	return fail(CauseIdentity, "removed task name remains present")
}

func (capture *treeCapture) close() error {
	if capture == nil {
		return nil
	}
	return capture.root.close()
}

func (capture *treeCapture) revalidate(ctx context.Context) error {
	if capture == nil || capture.root == nil {
		return fail(CauseInternalInvariant, "missing retained tree")
	}
	current, err := captureRetainedTree(ctx, capture.root, capture.policy)
	if err != nil {
		return err
	}
	if current.digest != capture.digest || !equalEntrySnapshots(current.entries, capture.entries) {
		return fail(CauseUnstable, "authority tree manifest changed")
	}
	return nil
}

func captureRetainedTree(ctx context.Context, root *retainedDirectory, policy treePolicy) (*treeCapture, error) {
	return captureRetainedTreeWithRead(ctx, root, policy, (*os.File).ReadDir)
}

func captureRetainedTreeWithRead(
	ctx context.Context,
	root *retainedDirectory,
	policy treePolicy,
	readDir directoryReadFunc,
) (*treeCapture, error) {
	if err := validateTreeCaptureRequest(ctx, root, policy, readDir); err != nil {
		return nil, err
	}
	scanRoot, err := root.root.Open(".")
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "open retained root for capture")
	}
	builder := treeCaptureBuilder{
		ctx:     ctx,
		root:    root,
		policy:  policy,
		readDir: readDir,
		entries: make([]entrySnapshot, 0),
	}
	captureErr := builder.captureRoot(scanRoot)
	closeErr := closeDescriptorFailure(scanRoot.Close())
	if captureErr != nil || closeErr != nil {
		return nil, joinFilesystemFailures(captureErr, closeErr)
	}
	return finishTreeCapture(ctx, root, policy, builder.entries)
}

func validateTreeCaptureRequest(
	ctx context.Context,
	root *retainedDirectory,
	policy treePolicy,
	readDir directoryReadFunc,
) error {
	if ctx == nil {
		return fail(CauseInvalidRequest, "missing authority context")
	}
	if root == nil || root.root == nil || root.descriptor == nil {
		return fail(CauseInternalInvariant, "missing authority root handles")
	}
	if readDir == nil {
		return fail(CauseInternalInvariant, "missing bounded directory reader")
	}
	if policy.domain == "" || policy.limits.maxEntries == 0 || policy.limits.maxBytes == 0 || policy.limits.maxFileBytes == 0 {
		return fail(CauseInternalInvariant, "incomplete tree policy")
	}
	if err := checkContext(ctx, "capture authority tree"); err != nil {
		return err
	}
	return validateRetainedRootContext(ctx, root, policy.owners)
}

func finishTreeCapture(
	ctx context.Context,
	root *retainedDirectory,
	policy treePolicy,
	entries []entrySnapshot,
) (*treeCapture, error) {
	if err := checkContext(ctx, "capture authority tree"); err != nil {
		return nil, err
	}
	sortEntrySnapshots(entries)
	if err := bindSymlinkTargets(entries); err != nil {
		return nil, err
	}
	if err := checkContext(ctx, "capture authority tree"); err != nil {
		return nil, err
	}
	if err := validateRetainedRootContext(ctx, root, policy.owners); err != nil {
		return nil, err
	}
	digest := digestTreeManifest(policy.domain, root, entries)
	return &treeCapture{root: root, policy: policy, digest: digest, entries: entries}, nil
}

func validateRetainedRoot(root *retainedDirectory) error {
	return validateRetainedRootContext(context.Background(), root, ownerEffectiveOnly)
}

func validateRetainedRootContext(ctx context.Context, root *retainedDirectory, owners ownerPolicy) error {
	return validateRetainedRootContextWithInspect(ctx, root, owners, inspectDescriptorContext)
}

func validateRetainedRootContextWithInspect(
	ctx context.Context,
	root *retainedDirectory,
	owners ownerPolicy,
	inspect descriptorInspectFunc,
) error {
	if root == nil || root.descriptor == nil {
		return fail(CauseInternalInvariant, "missing retained authority root descriptor")
	}
	if inspect == nil {
		return fail(CauseInternalInvariant, "missing descriptor inspector")
	}
	snapshot, mount, aclDigest, err := inspect(ctx, root.descriptor)
	if err != nil {
		return err
	}
	if snapshot != root.snapshot || mount != root.mount || aclDigest != root.aclDigest {
		return fail(CauseUnstable, "retained authority root changed")
	}
	if err := validateProtected(snapshot, owners); err != nil {
		return err
	}
	return revalidateAuthorityPathContextWithInspect(ctx, root, inspect)
}

func revalidateAuthorityPathContextWithInspect(
	ctx context.Context,
	root *retainedDirectory,
	inspect descriptorInspectFunc,
) error {
	descriptor, currentClaims, err := openAuthorityPathContextWithInspect(ctx, root.path, inspect)
	if err != nil {
		return err
	}
	compareErr := compareAuthorityPathClaims(root.pathClaims, currentClaims)
	closeErr := closeDescriptorFailure(descriptor.Close())
	return joinFilesystemFailures(compareErr, closeErr)
}

func compareAuthorityPathClaims(expected, current []authorityPathClaim) error {
	if len(expected) == 0 || len(expected) != len(current) {
		return fail(CauseIdentity, "authority path identity depth changed")
	}
	for index := range expected {
		if !sameFilesystemObject(expected[index].identity, current[index].identity) {
			return fail(CauseIdentity, "authority path identity changed")
		}
		if expected[index] != current[index] {
			return fail(CauseUnstable, "authority path security claim changed")
		}
	}
	return nil
}

func sameFilesystemObject(left, right FileIdentity) bool {
	return left.Device == right.Device &&
		left.Inode == right.Inode &&
		left.Filesystem == right.Filesystem
}

func (builder *treeCaptureBuilder) captureRoot(descriptor *os.File) error {
	before, mount, aclDigest, err := inspectDescriptorContext(builder.ctx, descriptor)
	if err != nil {
		return err
	}
	if before != builder.root.snapshot || mount != builder.root.mount || aclDigest != builder.root.aclDigest {
		return fail(CauseIdentity, "retained root capture identity mismatch")
	}
	if err := validateProtected(before, builder.policy.owners); err != nil {
		return err
	}
	if !snapshotIsDirectory(before) {
		return fail(CauseIdentity, "retained root kind changed")
	}
	return builder.captureDirectory(descriptor, "", before, mount, aclDigest)
}

func (builder *treeCaptureBuilder) captureDirectory(
	descriptor *os.File,
	prefix string,
	before fileSnapshot,
	mount mountSnapshot,
	aclDigest Digest,
) error {
	for {
		if err := checkContext(builder.ctx, "walk authority tree"); err != nil {
			return err
		}
		directoryEntries, readErr := builder.readDir(descriptor, directoryReadBatchSize)
		for _, directoryEntry := range directoryEntries {
			if err := builder.captureDirectoryEntry(descriptor, prefix, directoryEntry.Name()); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return failWithFilesystemCauses(readErr, classifyPathError(readErr), "read authority directory")
		}
	}
	if err := checkContext(builder.ctx, "walk authority tree"); err != nil {
		return err
	}
	after, afterMount, afterACL, err := inspectDescriptorContext(builder.ctx, descriptor)
	if err != nil {
		return err
	}
	if after != before || afterMount != mount || afterACL != aclDigest {
		return fail(CauseUnstable, "authority directory changed during capture")
	}
	return nil
}

func (builder *treeCaptureBuilder) captureDirectoryEntry(parent *os.File, prefix, name string) error {
	if err := checkContext(builder.ctx, "capture authority entry"); err != nil {
		return err
	}
	path := name
	if prefix != "" {
		path = prefix + string(filepath.Separator) + name
	}
	if err := validateRelativeManifestPath(path); err != nil {
		return err
	}
	if uint64(len(builder.entries)) >= builder.policy.limits.maxEntries {
		return fail(CauseLimit, "authority entry limit exceeded")
	}
	kind, err := relativeEntryKindNoFollow(int(parent.Fd()), name)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "inspect authority entry")
	}
	if kind == entrySymlink && !builder.policy.allowSymlinks {
		return fail(CauseUnsupported, "authority symlink refused")
	}
	descriptor, err := openRelativeNoFollow(int(parent.Fd()), name, kind)
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "open authority entry")
	}
	captureErr := builder.captureOpenedEntry(descriptor, path, kind)
	closeErr := closeDescriptorFailure(descriptor.Close())
	return joinFilesystemFailures(captureErr, closeErr)
}

func (builder *treeCaptureBuilder) captureOpenedEntry(descriptor *os.File, path string, kind entryKind) error {
	before, mount, aclDigest, err := inspectDescriptorContext(builder.ctx, descriptor)
	if err != nil {
		return err
	}
	if err := validateOpenedEntry(builder.root, before, mount, kind, builder.policy.owners); err != nil {
		return err
	}

	entry := entrySnapshot{path: path, kind: kind, file: before, aclDigest: aclDigest}
	if err := builder.captureEntryPayload(descriptor, &entry); err != nil {
		return err
	}
	builder.totalBytes, err = chargeTreeEntry(
		builder.policy.limits,
		uint64(len(builder.entries)),
		builder.totalBytes,
		entry,
	)
	if err != nil {
		return err
	}
	builder.entries = append(builder.entries, entry)
	return builder.finishOpenedEntry(descriptor, entry, mount)
}

func (builder *treeCaptureBuilder) captureEntryPayload(descriptor *os.File, entry *entrySnapshot) error {
	if entry == nil {
		return fail(CauseInternalInvariant, "missing authority entry claim")
	}
	var err error
	switch entry.kind {
	case entryDirectory:
		return nil
	case entryRegular:
		if err := builder.validateRegularBudget(entry.file.size); err != nil {
			return err
		}
		entry.content, err = hashRegularFile(
			builder.ctx,
			descriptor,
			entry.file.size,
			builder.policy.limits.maxFileBytes,
		)
	case entrySymlink:
		entry.linkText, err = readOpenedSymlinkContext(builder.ctx, descriptor, maxSymlinkBytes)
	default:
		err = fail(CauseInternalInvariant, "unknown captured entry kind")
	}
	return err
}

func (builder *treeCaptureBuilder) validateRegularBudget(size int64) error {
	byteSize, err := nonnegativeByteSize(size)
	if err != nil || byteSize > builder.policy.limits.maxFileBytes {
		return fail(CauseLimit, "authority file size refused")
	}
	if builder.totalBytes > builder.policy.limits.maxBytes ||
		byteSize > builder.policy.limits.maxBytes-builder.totalBytes {
		return fail(CauseLimit, "authority aggregate-byte limit exceeded")
	}
	return nil
}

func (builder *treeCaptureBuilder) finishOpenedEntry(
	descriptor *os.File,
	entry entrySnapshot,
	mount mountSnapshot,
) error {
	if entry.kind == entryDirectory {
		return builder.captureDirectory(descriptor, entry.path, entry.file, mount, entry.aclDigest)
	}
	if err := checkContext(builder.ctx, "capture authority entry"); err != nil {
		return err
	}
	after, afterMount, afterACL, err := inspectDescriptorContext(builder.ctx, descriptor)
	if err != nil {
		return err
	}
	if after != entry.file || afterMount != mount || afterACL != entry.aclDigest {
		return fail(CauseUnstable, "authority entry changed during capture")
	}
	return nil
}

func nonnegativeByteSize(size int64) (uint64, error) {
	if size < 0 {
		return 0, fail(CauseLimit, "authority file size refused")
	}
	return uint64(size), nil
}

func hashRegularFile(ctx context.Context, file *os.File, expectedSize int64, limit uint64) (Digest, error) {
	byteSize, err := nonnegativeByteSize(expectedSize)
	if err != nil || byteSize > limit {
		return Digest{}, fail(CauseLimit, "authority file size refused")
	}
	if err := checkContext(ctx, "hash authority file"); err != nil {
		return Digest{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Digest{}, failWithFilesystemCauses(err, CauseUnsupported, "seek authority file")
	}
	return hashBoundedReader(ctx, file, expectedSize, limit)
}

// hashRetainedFileContent hashes a bounded regular-file extent through
// ReaderAt, leaving the shared descriptor offset unchanged.
func hashRetainedFileContent(
	ctx context.Context,
	file *os.File,
	expectedSize int64,
	limit uint64,
) (Digest, error) {
	if file == nil {
		return Digest{}, fail(CauseInternalInvariant, "missing retained authority file")
	}
	byteSize, err := nonnegativeByteSize(expectedSize)
	if err != nil || byteSize > limit {
		return Digest{}, fail(CauseLimit, "authority file size refused")
	}
	return hashBoundedReader(
		ctx,
		io.NewSectionReader(file, 0, expectedSize),
		expectedSize,
		limit,
	)
}

func validateRetainedRegularFileContent(
	ctx context.Context,
	file *os.File,
	expectedSnapshot fileSnapshot,
	expectedMount mountSnapshot,
	expectedACL Digest,
	expectedHash Digest,
	limit uint64,
) error {
	before, mount, aclDigest, err := inspectDescriptorContext(ctx, file)
	if err != nil {
		return err
	}
	if before != expectedSnapshot || mount != expectedMount || aclDigest != expectedACL {
		return fail(CauseUnstable, "retained authority file changed before content proof")
	}
	contentHash, err := hashRetainedFileContent(ctx, file, before.size, limit)
	if err != nil {
		return err
	}
	after, afterMount, afterACL, err := inspectDescriptorContext(ctx, file)
	if err != nil {
		return err
	}
	if contentHash != expectedHash || after != before || afterMount != mount || afterACL != aclDigest {
		return fail(CauseUnstable, "retained authority file content changed")
	}
	return nil
}

func hashBoundedReader(ctx context.Context, reader io.Reader, expectedSize int64, limit uint64) (Digest, error) {
	byteSize, err := nonnegativeByteSize(expectedSize)
	if err != nil || byteSize > limit {
		return Digest{}, fail(CauseLimit, "authority file size refused")
	}
	if err := checkContext(ctx, "hash authority file"); err != nil {
		return Digest{}, err
	}
	copyLimit, err := boundedCopyLimit(limit)
	if err != nil {
		return Digest{}, err
	}
	hasher := sha256.New()
	written, err := io.Copy(hasher, io.LimitReader(&contextReader{ctx: ctx, reader: reader}, copyLimit))
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return Digest{}, failWithFilesystemCauses(contextErr, contextCause(contextErr), "hash authority file")
		}
		return Digest{}, failWithFilesystemCauses(err, CauseUnstable, "hash authority file")
	}
	if err := checkContext(ctx, "hash authority file"); err != nil {
		return Digest{}, err
	}
	if written != expectedSize {
		return Digest{}, fail(CauseUnstable, "authority file size changed")
	}
	var digest Digest
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}

func boundedCopyLimit(limit uint64) (int64, error) {
	const maxInt64AsUint64 = uint64(1<<63 - 1)
	if limit >= maxInt64AsUint64 {
		return 0, fail(CauseLimit, "authority reader bound refused")
	}
	return int64(limit) + 1, nil
}

func validateOpenedEntry(
	root *retainedDirectory,
	snapshot fileSnapshot,
	mount mountSnapshot,
	kind entryKind,
	owners ownerPolicy,
) error {
	if root == nil {
		return fail(CauseInternalInvariant, "missing retained authority root")
	}
	if err := validateProtected(snapshot, owners); err != nil {
		return err
	}
	if mount != root.mount || snapshot.identity.Device != root.snapshot.identity.Device {
		return fail(CauseUnsupported, "authority mount transition refused")
	}
	if snapshotKind(snapshot) != kind {
		return fail(CauseIdentity, "authority entry kind changed")
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

func chargeTreeEntry(limits treeLimits, count, totalBytes uint64, entry entrySnapshot) (uint64, error) {
	if count >= limits.maxEntries {
		return totalBytes, fail(CauseLimit, "authority entry limit exceeded")
	}
	if entry.kind != entryRegular {
		return totalBytes, nil
	}
	if entry.file.size < 0 || uint64(entry.file.size) > limits.maxFileBytes {
		return totalBytes, fail(CauseLimit, "authority file size refused")
	}
	size := uint64(entry.file.size)
	if totalBytes > limits.maxBytes || size > limits.maxBytes-totalBytes {
		return totalBytes, fail(CauseLimit, "authority aggregate-byte limit exceeded")
	}
	return totalBytes + size, nil
}

func sortEntrySnapshots(entries []entrySnapshot) {
	sort.Slice(entries, func(left, right int) bool {
		return bytes.Compare([]byte(entries[left].path), []byte(entries[right].path)) < 0
	})
}

func equalEntrySnapshots(left, right []entrySnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func bindSymlinkTargets(entries []entrySnapshot) error {
	byPath := make(map[string]*entrySnapshot, len(entries))
	for index := range entries {
		byPath[entries[index].path] = &entries[index]
	}
	for index := range entries {
		if entries[index].kind != entrySymlink {
			continue
		}
		target, err := resolveContainedTarget(entries[index].path, byPath)
		if err != nil {
			return err
		}
		entries[index].targetDigest = digestEntryClaim(*byPath[target])
	}
	return nil
}

type containedTargetResolver struct {
	base    []string
	pending []string
	visited map[string]struct{}
	links   int
}

func resolveContainedTarget(linkPath string, entries map[string]*entrySnapshot) (string, error) {
	entry, ok := entries[linkPath]
	if !ok {
		return "", fail(CauseNotFound, "authority symlink target absent")
	}
	switch entry.kind {
	case entryDirectory:
		return "", fail(CauseUnsupported, "authority symlink target is not regular")
	case entryRegular:
		return linkPath, nil
	case entrySymlink:
	default:
		return "", fail(CauseInternalInvariant, "unknown captured entry kind")
	}

	resolver := containedTargetResolver{
		base:    splitCapturedPath(filepath.Dir(linkPath)),
		pending: []string{filepath.Base(linkPath)},
		visited: make(map[string]struct{}),
	}
	for len(resolver.pending) != 0 {
		component := resolver.pending[0]
		resolver.pending = resolver.pending[1:]
		target, complete, err := resolver.consume(component, entries)
		if err != nil {
			return "", err
		}
		if complete {
			return target, nil
		}
	}
	return "", fail(CauseUnsupported, "authority symlink target is not regular")
}

func (resolver *containedTargetResolver) consume(
	component string,
	entries map[string]*entrySnapshot,
) (string, bool, error) {
	switch component {
	case "", ".":
		return "", false, nil
	case "..":
		return "", false, resolver.consumeParent()
	}
	candidate := strings.Join(appendPathComponent(resolver.base, component), string(filepath.Separator))
	entry, present := entries[candidate]
	if !present {
		return "", false, fail(CauseNotFound, "authority symlink target absent")
	}
	switch entry.kind {
	case entryDirectory:
		return "", false, resolver.consumeDirectory(component)
	case entryRegular:
		return resolver.consumeRegular(candidate)
	case entrySymlink:
		return "", false, resolver.expandSymlink(candidate, entry.linkText)
	default:
		return "", false, fail(CauseInternalInvariant, "unknown captured entry kind")
	}
}

func (resolver *containedTargetResolver) consumeParent() error {
	if len(resolver.base) == 0 {
		return fail(CauseUnsupported, "escaping authority symlink refused")
	}
	resolver.base = resolver.base[:len(resolver.base)-1]
	return nil
}

func (resolver *containedTargetResolver) consumeDirectory(component string) error {
	if len(resolver.pending) == 0 {
		return fail(CauseUnsupported, "authority symlink target is not regular")
	}
	resolver.base = append(resolver.base, component)
	return nil
}

func (resolver *containedTargetResolver) consumeRegular(candidate string) (string, bool, error) {
	if len(resolver.pending) != 0 {
		return "", false, fail(CauseUnsupported, "authority symlink path crosses a non-directory")
	}
	return candidate, true, nil
}

func (resolver *containedTargetResolver) expandSymlink(candidate, linkText string) error {
	if resolver.links == 40 {
		return fail(CauseLimit, "authority symlink depth exceeded")
	}
	if _, duplicate := resolver.visited[candidate]; duplicate {
		return fail(CauseMalformed, "authority symlink cycle")
	}
	if filepath.IsAbs(linkText) {
		return fail(CauseUnsupported, "absolute authority symlink refused")
	}
	resolver.visited[candidate] = struct{}{}
	resolver.links++
	resolver.pending = append(strings.Split(linkText, string(filepath.Separator)), resolver.pending...)
	return nil
}

func splitCapturedPath(path string) []string {
	if path == "." || path == "" {
		return nil
	}
	return strings.Split(path, string(filepath.Separator))
}

func appendPathComponent(base []string, component string) []string {
	path := make([]string, len(base)+1)
	copy(path, base)
	path[len(base)] = component
	return path
}

func digestTreeManifest(domain string, root *retainedDirectory, entries []entrySnapshot) Digest {
	hasher := sha256.New()
	writeLengthPrefixed(hasher, []byte(manifestFormatDomain))
	writeLengthPrefixed(hasher, []byte(domain))
	writeManifestRecord(hasher, rootManifestRecord(root))
	writeUint64(hasher, uint64(len(entries)))
	for _, entry := range entries {
		writeManifestRecord(hasher, entryManifestRecord(entry))
	}
	var digest Digest
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func rootManifestRecord(root *retainedDirectory) []byte {
	var record bytes.Buffer
	record.WriteByte(0)
	writeSnapshotClaim(&record, root.snapshot, root.aclDigest)
	writeUint32(&record, uint32(root.mount.filesystem[0])) //nolint:gosec // Encode the signed FSID word as its raw 32-bit bit pattern.
	writeUint32(&record, uint32(root.mount.filesystem[1])) //nolint:gosec // Encode the signed FSID word as its raw 32-bit bit pattern.
	writeUint32(&record, root.mount.flags)
	return record.Bytes()
}

func entryManifestRecord(entry entrySnapshot) []byte {
	var record bytes.Buffer
	writeLengthPrefixed(&record, []byte(entry.path))
	record.WriteByte(byte(entry.kind))
	writeSnapshotClaim(&record, entry.file, entry.aclDigest)
	switch entry.kind {
	case entryDirectory:
	case entryRegular:
		record.Write(entry.content[:])
	case entrySymlink:
		writeLengthPrefixed(&record, []byte(entry.linkText))
		record.Write(entry.targetDigest[:])
	}
	return record.Bytes()
}

func writeSnapshotClaim(writer io.Writer, snapshot fileSnapshot, aclDigest Digest) {
	writeUint32(writer, snapshot.identity.Mode)
	writeUint32(writer, snapshot.identity.UID)
	writeUint32(writer, snapshot.gid)
	writeUint64(writer, uint64(snapshot.size)) //nolint:gosec // Admission rejects negative snapshot sizes before manifest construction.
	_, _ = writer.Write(aclDigest[:])
}

func digestEntryClaim(entry entrySnapshot) Digest {
	hasher := sha256.New()
	writeManifestRecord(hasher, entryManifestRecord(entry))
	var digest Digest
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func writeManifestRecord(writer io.Writer, record []byte) {
	writeLengthPrefixed(writer, record)
}

func writeLengthPrefixed(writer io.Writer, value []byte) {
	writeUint64(writer, uint64(len(value)))
	_, _ = writer.Write(value)
}

func writeUint32(writer io.Writer, value uint32) {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], value)
	_, _ = writer.Write(encoded[:])
}

func writeUint64(writer io.Writer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = writer.Write(encoded[:])
}

func validateAuthorityPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fail(CauseInvalidRequest, "authority path is not clean and absolute")
	}
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return failWith(err, classifyPathError(err), "evaluate authority path")
	}
	if physical != path {
		return fail(CauseInvalidRequest, "authority path is not physical")
	}
	return nil
}

func validateProtected(snapshot fileSnapshot, owners ownerPolicy) error {
	if snapshot.size < 0 {
		return fail(CauseMalformed, "authority size refused")
	}
	uid := snapshot.identity.UID
	effectiveUID, err := effectiveUserID()
	if err != nil {
		return err
	}
	if owners == ownerEffectiveOnly && uid != effectiveUID {
		return fail(CausePermission, "authority owner refused")
	}
	if owners == ownerRootOrEffective && uid != 0 && uid != effectiveUID {
		return fail(CausePermission, "authority owner refused")
	}
	if owners != ownerEffectiveOnly && owners != ownerRootOrEffective {
		return fail(CauseInternalInvariant, "unknown owner policy")
	}
	if snapshot.identity.Mode&0o22 != 0 || snapshot.identity.Mode&0o7000 != 0 {
		return fail(CausePermission, "authority mode refused")
	}
	return nil
}

func effectiveUserID() (uint32, error) {
	const maximumUID = int64(1<<32 - 1)
	uid := int64(os.Geteuid())
	if uid < 0 || uid > maximumUID {
		return 0, fail(CauseInternalInvariant, "effective user identity is out of range")
	}
	return uint32(uid), nil
}

func validateRelativeManifestPath(path string) error {
	if path == "" || path == "." || filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fail(CauseMalformed, "invalid authority relative path")
	}
	if len([]byte(path)) > maxRelativePathBytes {
		return fail(CauseLimit, "authority relative path too long")
	}
	for component := range strings.SplitSeq(path, string(filepath.Separator)) {
		if component == "" || len([]byte(component)) > maxPathComponentBytes {
			return fail(CauseLimit, "authority path component refused")
		}
	}
	return nil
}

func snapshotKind(snapshot fileSnapshot) entryKind {
	mode := snapshot.identity.Mode
	switch mode & platformModeTypeMask {
	case platformModeDirectory:
		return entryDirectory
	case platformModeRegular:
		return entryRegular
	case platformModeSymlink:
		return entrySymlink
	default:
		return 0
	}
}

func snapshotIsDirectory(snapshot fileSnapshot) bool {
	return snapshotKind(snapshot) == entryDirectory
}

func classifyPathError(err error) CauseCode {
	switch {
	case err == nil:
		return CauseInternalInvariant
	case errors.Is(err, errUnsupportedAuthorityEntry):
		return CauseUnsupported
	case errors.Is(err, errAuthorityKindMismatch):
		return CauseIdentity
	case errors.Is(err, fs.ErrNotExist):
		return CauseNotFound
	case errors.Is(err, fs.ErrPermission):
		return CausePermission
	default:
		return CauseUnstable
	}
}

func contextCause(err error) CauseCode {
	if errors.Is(err, context.DeadlineExceeded) {
		return CauseDeadline
	}
	if errors.Is(err, context.Canceled) {
		return CauseCanceled
	}
	return CauseInternalInvariant
}

func checkContext(ctx context.Context, operation string) error {
	if ctx == nil {
		return fail(CauseInvalidRequest, "missing authority context")
	}
	if err := ctx.Err(); err != nil {
		return failWithFilesystemCauses(err, contextCause(err), operation)
	}
	return nil
}

func failWithFilesystemCauses(err error, cause CauseCode, message string) error {
	failure := failWith(err, cause, message).(*privateFailure) //nolint:errorlint // failWith constructs this exact private type and never wraps it.
	failure.causes = append(failure.causes, filesystemPrivateCauses(err)...)
	return failure
}

func joinFilesystemFailures(failures ...error) error {
	nonNil := make([]error, 0, len(failures))
	causes := make([]CauseCode, 0, len(failures))
	for _, failure := range failures {
		if failure == nil {
			continue
		}
		nonNil = append(nonNil, failure)
		causes = append(causes, filesystemPrivateCauses(failure)...)
	}
	if len(nonNil) == 0 {
		return nil
	}
	if len(nonNil) == 1 {
		return nonNil[0]
	}
	if len(causes) == 0 {
		causes = append(causes, CauseInternalInvariant)
	}
	return &privateFailure{causes: causes, raw: errors.Join(nonNil...)}
}

func filesystemPrivateCauses(err error) []CauseCode {
	causes := make([]CauseCode, 0, 1)
	var collect func(error)
	collect = func(current error) {
		if current == nil {
			return
		}
		if failure, ok := current.(*privateFailure); ok { //nolint:errorlint // Exact nodes avoid double-counting summarized descendants.
			causes = append(causes, failure.causes...)
			return
		}
		switch wrapped := current.(type) { //nolint:errorlint // A joined error must contribute every private branch.
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				collect(child)
			}
		case interface{ Unwrap() error }:
			collect(wrapped.Unwrap())
		}
	}
	collect(err)
	return causes
}

func closeDescriptorFailure(err error) error {
	if err == nil {
		return nil
	}
	return failWith(err, CauseDescriptorClose, "close authority descriptor")
}
