package buildauthority

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// retainedExecutable is authenticated substrate, not command authority. Only
// a successful nominal constructor may move it into one of the three roles
// below.
type retainedExecutable struct {
	leaf   *retainedLeaf
	digest Digest
}

// retainedExecutableCandidate owns one retained executable until exactly one
// nominal constructor succeeds or the no-scratch transaction closes it.
type retainedExecutableCandidate struct {
	mu       sync.Mutex
	retained *retainedExecutable
}

// Distinct seal types make the three nominal wrappers non-convertible even
// though they deliberately retain the same private substrate shape.
type goAuthoritySeal uint8
type compilerAuthoritySeal uint8
type gitAuthoritySeal uint8

const (
	validGoAuthority       goAuthoritySeal       = 1
	validCompilerAuthority compilerAuthoritySeal = 1
	validGitAuthority      gitAuthoritySeal      = 1
)

type goAuthority struct {
	retained *retainedExecutable
	seal     goAuthoritySeal
}

type compilerAuthority struct {
	retained *retainedExecutable
	seal     compilerAuthoritySeal
}

type gitAuthority struct {
	retained *retainedExecutable
	seal     gitAuthoritySeal
}

func goExecutableDigest() Digest {
	return Digest{
		0x54, 0x86, 0x08, 0xa9, 0x10, 0xc4, 0x6d, 0xe3,
		0x2c, 0x65, 0xa3, 0x93, 0x4f, 0x46, 0x1b, 0x17,
		0x87, 0xac, 0xf6, 0xdd, 0xd3, 0x71, 0x04, 0x48,
		0x26, 0x06, 0x8d, 0x85, 0x03, 0xb8, 0x50, 0x9b,
	}
}

func gitExecutableDigest() Digest {
	return Digest{
		0xbe, 0x4a, 0xfb, 0x2b, 0x00, 0x39, 0x04, 0x72,
		0x58, 0x26, 0x25, 0x0d, 0xe9, 0xfb, 0x76, 0x56,
		0x7b, 0xba, 0xcf, 0x82, 0x32, 0x34, 0x57, 0xb5,
		0xa1, 0xec, 0x26, 0x70, 0x6b, 0x66, 0xbc, 0xae,
	}
}

// retainExecutableCandidate is the sole executable constructor that accepts a
// raw path. It retains a no-follow leaf and its complete descriptor-linked
// ancestry, but deliberately confers no right to execute it.
func retainExecutableCandidate(ctx context.Context, path string) (*retainedExecutableCandidate, error) {
	if err := checkContext(ctx, "retain executable candidate"); err != nil {
		return nil, err
	}
	if err := validateAuthorityPath(path); err != nil {
		return nil, err
	}
	parentPath, base, err := splitExecutablePath(path)
	if err != nil {
		return nil, err
	}
	parent, pathClaims, err := openAuthorityPathContext(ctx, parentPath)
	if err != nil {
		return nil, err
	}
	descriptor, openErr := openRelativeNoFollow(int(parent.Fd()), base, entryRegular)
	if openErr != nil {
		return nil, joinFilesystemFailures(
			failWithFilesystemCauses(openErr, classifyPathError(openErr), "open retained executable"),
			closeDescriptorFailure(parent.Close()),
		)
	}
	retained, captureErr := captureRetainedExecutable(ctx, path, descriptor, pathClaims)
	if captureErr != nil {
		return nil, joinFilesystemFailures(
			captureErr,
			closeDescriptorFailure(descriptor.Close()),
			closeDescriptorFailure(parent.Close()),
		)
	}
	if parentCloseErr := closeDescriptorFailure(parent.Close()); parentCloseErr != nil {
		return nil, joinFilesystemFailures(parentCloseErr, closeDescriptorFailure(retained.close()))
	}
	if err := retained.revalidate(ctx); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(retained.close()))
	}
	return &retainedExecutableCandidate{retained: retained}, nil
}

func splitExecutablePath(path string) (string, string, error) {
	parent := filepath.Dir(path)
	base := filepath.Base(path)
	if parent == path || base == "." || base == string(filepath.Separator) ||
		base == "" || strings.ContainsRune(base, '\x00') {
		return "", "", fail(CauseInvalidRequest, "invalid executable path")
	}
	return parent, base, nil
}

func captureRetainedExecutable(
	ctx context.Context,
	path string,
	descriptor *os.File,
	pathClaims []authorityPathClaim,
) (*retainedExecutable, error) {
	if descriptor == nil || len(pathClaims) == 0 {
		return nil, fail(CauseInternalInvariant, "missing executable retention input")
	}
	snapshot, mount, aclDigest, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	if err := validateExecutableSnapshot(snapshot, mount, pathClaims); err != nil {
		return nil, err
	}
	digest, err := hashRetainedFileContent(ctx, descriptor, snapshot.size, maxExecutableBytes)
	if err != nil {
		return nil, err
	}
	after, afterMount, afterACL, err := inspectDescriptorContext(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	if after != snapshot || afterMount != mount || afterACL != aclDigest {
		return nil, fail(CauseUnstable, "retained executable changed during capture")
	}
	return &retainedExecutable{
		leaf: &retainedLeaf{
			path:       path,
			descriptor: descriptor,
			snapshot:   snapshot,
			mount:      mount,
			aclDigest:  aclDigest,
			pathClaims: append([]authorityPathClaim(nil), pathClaims...),
		},
		digest: digest,
	}, nil
}

func validateExecutableSnapshot(
	snapshot fileSnapshot,
	mount mountSnapshot,
	pathClaims []authorityPathClaim,
) error {
	if snapshotKind(snapshot) != entryRegular {
		return fail(CauseUnsupported, "retained executable is not a regular file")
	}
	if err := validateProtected(snapshot, ownerRootOrEffective); err != nil {
		return err
	}
	if snapshot.identity.Mode&0o111 == 0 {
		return fail(CausePermission, "retained executable has no execute bit")
	}
	byteSize, err := nonnegativeByteSize(snapshot.size)
	if err != nil {
		return err
	}
	if byteSize > maxExecutableBytes {
		return fail(CauseLimit, "retained executable size refused")
	}
	if len(pathClaims) == 0 {
		return fail(CauseInternalInvariant, "missing executable parent claim")
	}
	parent := pathClaims[len(pathClaims)-1]
	if snapshot.identity.Device != parent.identity.Device ||
		snapshot.identity.Filesystem != parent.identity.Filesystem || mount != parent.mount {
		return fail(CauseUnsupported, "retained executable crosses its parent mount")
	}
	return nil
}

func (retained *retainedExecutable) revalidate(ctx context.Context) error {
	if err := validateRetainedExecutableShape(retained); err != nil {
		return err
	}
	if err := validateRetainedExecutableDescriptor(ctx, retained); err != nil {
		return err
	}
	return validateRetainedExecutableBinding(ctx, retained)
}

func validateRetainedExecutableShape(retained *retainedExecutable) error {
	if retained == nil || retained.leaf == nil || retained.leaf.descriptor == nil {
		return fail(CauseInternalInvariant, "missing retained executable")
	}
	leaf := retained.leaf
	if !filepath.IsAbs(leaf.path) || filepath.Clean(leaf.path) != leaf.path ||
		strings.ContainsRune(leaf.path, '\x00') {
		return fail(CauseInternalInvariant, "invalid retained executable path")
	}
	return validateExecutableSnapshot(leaf.snapshot, leaf.mount, leaf.pathClaims)
}

func validateRetainedExecutableDescriptor(ctx context.Context, retained *retainedExecutable) error {
	leaf := retained.leaf
	current, mount, aclDigest, err := inspectDescriptorContext(ctx, leaf.descriptor)
	if err != nil {
		return err
	}
	if err := validateExecutableSnapshot(current, mount, leaf.pathClaims); err != nil {
		return err
	}
	if err := compareRetainedLeafClaim(leaf, current, mount, aclDigest); err != nil {
		return err
	}
	digest, err := hashRetainedFileContent(ctx, leaf.descriptor, current.size, maxExecutableBytes)
	if err != nil {
		return err
	}
	after, afterMount, afterACL, err := inspectDescriptorContext(ctx, leaf.descriptor)
	if err != nil {
		return err
	}
	if after != current || afterMount != mount || afterACL != aclDigest || digest != retained.digest {
		return fail(CauseUnstable, "retained executable content changed")
	}
	return nil
}

func validateRetainedExecutableBinding(ctx context.Context, retained *retainedExecutable) error {
	leaf := retained.leaf
	parentPath, base, err := splitExecutablePath(leaf.path)
	if err != nil {
		return err
	}
	parent, currentClaims, err := openAuthorityPathContext(ctx, parentPath)
	if err != nil {
		return err
	}
	if err := compareAuthorityPathClaims(leaf.pathClaims, currentClaims); err != nil {
		return joinFilesystemFailures(err, closeDescriptorFailure(parent.Close()))
	}
	freshDescriptor, openErr := openRelativeNoFollow(int(parent.Fd()), base, entryRegular)
	if openErr != nil {
		return joinFilesystemFailures(
			failWithFilesystemCauses(openErr, classifyPathError(openErr), "reopen retained executable"),
			closeDescriptorFailure(parent.Close()),
		)
	}
	fresh, inspectErr := captureRetainedExecutable(ctx, leaf.path, freshDescriptor, currentClaims)
	if inspectErr == nil {
		inspectErr = compareRetainedLeafClaim(
			leaf,
			fresh.leaf.snapshot,
			fresh.leaf.mount,
			fresh.leaf.aclDigest,
		)
	}
	if inspectErr == nil && fresh.digest != retained.digest {
		inspectErr = fail(CauseUnstable, "visible executable content changed")
	}
	return joinFilesystemFailures(
		inspectErr,
		closeDescriptorFailure(freshDescriptor.Close()),
		closeDescriptorFailure(parent.Close()),
	)
}

func validateGoExecutableProfile(ctx context.Context, retained *retainedExecutable) error {
	return validateExecutableProfile(ctx, retained, machOProfileGo)
}

func validateGitExecutableProfile(ctx context.Context, retained *retainedExecutable) error {
	return validateExecutableProfile(ctx, retained, machOProfileGit)
}

func validateExecutableProfile(
	ctx context.Context,
	retained *retainedExecutable,
	profile machOProfile,
) error {
	if err := retained.revalidate(ctx); err != nil {
		return err
	}
	var err error
	switch profile {
	case machOProfileGo:
		err = validateGoMachO(retained.leaf.descriptor, retained.leaf.snapshot.size)
	case machOProfileGit:
		err = validateGitMachO(retained.leaf.descriptor, retained.leaf.snapshot.size)
	default:
		return fail(CauseInternalInvariant, "unknown executable Mach-O profile")
	}
	if err != nil {
		return failWith(err, CauseMalformed, "parse retained executable Mach-O")
	}
	return retained.revalidate(ctx)
}

func admitGoAuthority(
	ctx context.Context,
	candidate *retainedExecutableCandidate,
	goroot *treeCapture,
) (*goAuthority, error) {
	if err := checkContext(ctx, "admit Go authority"); err != nil {
		return nil, err
	}
	if candidate == nil {
		return nil, fail(CauseInternalInvariant, "missing Go executable candidate")
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	retained, err := candidate.requirePending()
	if err != nil {
		return nil, err
	}
	if filepath.Base(retained.leaf.path) != goExecutableBase {
		return nil, fail(CauseUnsupported, "Go executable basename refused")
	}
	if err := validateGOROOTExecutableBinding(ctx, goroot, retained, goGOROOTRelativePath); err != nil {
		return nil, err
	}
	if err := validateGoExecutableProfile(ctx, retained); err != nil {
		return nil, err
	}
	if err := validateGOROOTExecutableBinding(ctx, goroot, retained, goGOROOTRelativePath); err != nil {
		return nil, err
	}
	if retained.digest != goExecutableDigest() {
		return nil, fail(CauseUnsupported, "Go executable digest refused")
	}
	authority := &goAuthority{retained: retained, seal: validGoAuthority}
	candidate.retained = nil
	return authority, nil
}

func admitGitAuthority(
	ctx context.Context,
	candidate *retainedExecutableCandidate,
) (*gitAuthority, error) {
	if err := checkContext(ctx, "admit Git authority"); err != nil {
		return nil, err
	}
	if candidate == nil {
		return nil, fail(CauseInternalInvariant, "missing Git executable candidate")
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	retained, err := candidate.requirePending()
	if err != nil {
		return nil, err
	}
	if filepath.Base(retained.leaf.path) != gitExecutableBase {
		return nil, fail(CauseUnsupported, "Git executable basename refused")
	}
	if err := validateGitExecutableProfile(ctx, retained); err != nil {
		return nil, err
	}
	if retained.digest != gitExecutableDigest() {
		return nil, fail(CauseUnsupported, "Git executable digest refused")
	}
	authority := &gitAuthority{retained: retained, seal: validGitAuthority}
	candidate.retained = nil
	return authority, nil
}

func deriveCompilerAuthority(ctx context.Context, goroot *treeCapture) (*compilerAuthority, error) {
	if err := checkContext(ctx, "derive compiler authority"); err != nil {
		return nil, err
	}
	binding, err := gorootExecutableBinding(ctx, goroot, compilerGOROOTRelativePath)
	if err != nil {
		return nil, err
	}
	descriptor, err := openRelativeNoFollow(
		int(goroot.root.descriptor.Fd()),
		compilerGOROOTRelativePath,
		entryRegular,
	)
	if err != nil {
		return nil, failWithFilesystemCauses(err, classifyPathError(err), "open retained compiler")
	}
	retained, captureErr := captureRetainedExecutable(
		ctx,
		binding.path,
		descriptor,
		binding.parentClaims,
	)
	if captureErr != nil {
		return nil, joinFilesystemFailures(captureErr, closeDescriptorFailure(descriptor.Close()))
	}
	if err := validateExecutableAgainstGOROOTBinding(retained, binding); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(retained.close()))
	}
	if err := validateGoExecutableProfile(ctx, retained); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(retained.close()))
	}
	if err := validateGOROOTExecutableBinding(ctx, goroot, retained, compilerGOROOTRelativePath); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(retained.close()))
	}
	return &compilerAuthority{retained: retained, seal: validCompilerAuthority}, nil
}

type gorootExecutableClaim struct {
	path         string
	parentClaims []authorityPathClaim
	leaf         entrySnapshot
}

func validateGOROOTExecutableBinding(
	ctx context.Context,
	goroot *treeCapture,
	retained *retainedExecutable,
	relativePath string,
) error {
	binding, err := gorootExecutableBinding(ctx, goroot, relativePath)
	if err != nil {
		return err
	}
	if err := retained.revalidate(ctx); err != nil {
		return err
	}
	return validateExecutableAgainstGOROOTBinding(retained, binding)
}

func gorootExecutableBinding(
	ctx context.Context,
	goroot *treeCapture,
	relativePath string,
) (gorootExecutableClaim, error) {
	if err := checkContext(ctx, "bind GOROOT executable"); err != nil {
		return gorootExecutableClaim{}, err
	}
	if goroot == nil {
		return gorootExecutableClaim{}, fail(CauseInternalInvariant, "missing retained GOROOT capture")
	}
	if err := goroot.revalidate(ctx); err != nil {
		return gorootExecutableClaim{}, err
	}
	return gorootExecutableClaimFromCapture(goroot, relativePath)
}

// gorootExecutableClaimFromCapture performs only the immutable relationship
// check. Runtime admission brackets it with tree and retained-leaf
// revalidation; sealed environment construction uses it to prevent mixing a
// nominal role with a different GOROOT capture.
func gorootExecutableClaimFromCapture(
	goroot *treeCapture,
	relativePath string,
) (gorootExecutableClaim, error) {
	if goroot == nil || goroot.root == nil || goroot.root.descriptor == nil {
		return gorootExecutableClaim{}, fail(CauseInternalInvariant, "missing retained GOROOT capture")
	}
	if goroot.policy != gorootPolicy() {
		return gorootExecutableClaim{}, fail(CauseInternalInvariant, "GOROOT capture policy mismatch")
	}
	if err := validateRelativeManifestPath(relativePath); err != nil {
		return gorootExecutableClaim{}, err
	}
	components := strings.Split(relativePath, string(filepath.Separator))
	if len(components) < 2 {
		return gorootExecutableClaim{}, fail(CauseInternalInvariant, "GOROOT executable path has no parent")
	}
	claims := append([]authorityPathClaim(nil), goroot.root.pathClaims...)
	for index := 1; index < len(components); index++ {
		prefix := filepath.Join(components[:index]...)
		entry, err := findUniqueCapturedEntry(goroot.entries, prefix)
		if err != nil {
			return gorootExecutableClaim{}, err
		}
		if entry.kind != entryDirectory {
			return gorootExecutableClaim{}, fail(CauseIdentity, "GOROOT executable parent is not a directory")
		}
		claims = append(claims, makeAuthorityPathClaim(entry.file, goroot.root.mount, entry.aclDigest))
	}
	leaf, err := findUniqueCapturedEntry(goroot.entries, relativePath)
	if err != nil {
		return gorootExecutableClaim{}, err
	}
	if leaf.kind != entryRegular {
		return gorootExecutableClaim{}, fail(CauseIdentity, "GOROOT executable leaf is not regular")
	}
	return gorootExecutableClaim{
		path:         filepath.Join(goroot.root.path, relativePath),
		parentClaims: claims,
		leaf:         leaf,
	}, nil
}

func findUniqueCapturedEntry(entries []entrySnapshot, path string) (entrySnapshot, error) {
	var found entrySnapshot
	count := 0
	for _, entry := range entries {
		if entry.path == path {
			found = entry
			count++
		}
	}
	if count != 1 {
		return entrySnapshot{}, fail(CauseIdentity, "GOROOT executable capture row missing or duplicated")
	}
	return found, nil
}

func validateExecutableAgainstGOROOTBinding(
	retained *retainedExecutable,
	binding gorootExecutableClaim,
) error {
	if err := validateRetainedExecutableShape(retained); err != nil {
		return err
	}
	if binding.path == "" || len(binding.parentClaims) == 0 || binding.leaf.path == "" {
		return fail(CauseInternalInvariant, "incomplete GOROOT executable binding")
	}
	leaf := retained.leaf
	if leaf.path != binding.path {
		return fail(CauseIdentity, "executable path is outside retained GOROOT role")
	}
	if err := compareAuthorityPathClaims(binding.parentClaims, leaf.pathClaims); err != nil {
		return err
	}
	if leaf.snapshot != binding.leaf.file || leaf.mount != binding.parentClaims[len(binding.parentClaims)-1].mount ||
		leaf.aclDigest != binding.leaf.aclDigest || retained.digest != binding.leaf.content {
		return fail(CauseIdentity, "executable differs from retained GOROOT capture")
	}
	return nil
}

func (candidate *retainedExecutableCandidate) requirePending() (*retainedExecutable, error) {
	if candidate == nil || candidate.retained == nil {
		return nil, fail(CauseInternalInvariant, "executable candidate is not pending")
	}
	if err := validateRetainedExecutableShape(candidate.retained); err != nil {
		return nil, err
	}
	return candidate.retained, nil
}

func (candidate *retainedExecutableCandidate) close() error {
	if candidate == nil {
		return nil
	}
	candidate.mu.Lock()
	defer candidate.mu.Unlock()
	if candidate.retained == nil {
		return nil
	}
	retained := candidate.retained
	candidate.retained = nil
	return retained.close()
}

func (retained *retainedExecutable) close() error {
	if retained == nil || retained.leaf == nil {
		return nil
	}
	return retained.leaf.close()
}

func (authority *goAuthority) revalidate(ctx context.Context) error {
	if authority == nil || authority.seal != validGoAuthority || authority.retained == nil {
		return fail(CauseInternalInvariant, "invalid Go authority")
	}
	if authority.retained.digest != goExecutableDigest() {
		return fail(CauseUnsupported, "Go executable digest refused")
	}
	return validateGoExecutableProfile(ctx, authority.retained)
}

func (authority *compilerAuthority) revalidate(ctx context.Context) error {
	if authority == nil || authority.seal != validCompilerAuthority || authority.retained == nil {
		return fail(CauseInternalInvariant, "invalid compiler authority")
	}
	return validateGoExecutableProfile(ctx, authority.retained)
}

func (authority *gitAuthority) revalidate(ctx context.Context) error {
	if authority == nil || authority.seal != validGitAuthority || authority.retained == nil {
		return fail(CauseInternalInvariant, "invalid Git authority")
	}
	if filepath.Base(authority.retained.leaf.path) != gitExecutableBase ||
		authority.retained.digest != gitExecutableDigest() {
		return fail(CauseUnsupported, "Git executable identity refused")
	}
	return validateGitExecutableProfile(ctx, authority.retained)
}

func (authority *goAuthority) close() error {
	if authority == nil {
		return nil
	}
	return authority.retained.close()
}

func (authority *compilerAuthority) close() error {
	if authority == nil {
		return nil
	}
	return authority.retained.close()
}

func (authority *gitAuthority) close() error {
	if authority == nil {
		return nil
	}
	return authority.retained.close()
}
