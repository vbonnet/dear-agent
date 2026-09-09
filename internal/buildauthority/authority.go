package buildauthority

import (
	"context"
	"errors"
	"io"
	"os"
)

const (
	physicalRootPath     = "/"
	nullDeviceParentPath = "/dev"
	nullDevicePath       = "/dev/null"
)

// physicalRootAuthority is the sole capability for commands that must run
// from the physical filesystem root. The full retainedDirectory is kept alive,
// while claim excludes only directory metadata that can change with unrelated
// sibling churn.
type physicalRootAuthority struct {
	directory *retainedDirectory
	claim     authorityPathClaim
}

// retainedLeaf seals one no-follow leaf descriptor together with its complete
// descriptor claim and descriptor-linked ancestry. Concrete nominal wrappers,
// rather than retainedLeaf itself, confer authority to later phases.
type retainedLeaf struct {
	path       string
	descriptor *os.File
	snapshot   fileSnapshot
	mount      mountSnapshot
	aclDigest  Digest
	pathClaims []authorityPathClaim
}

// retainedNullDevice is the sole intentional group/world-writable authority.
// It is distinct from executable and tree capabilities and does not expose a
// general path or close surface outside this package.
type retainedNullDevice struct {
	leaf *retainedLeaf
}

// retainedNullProcessBorrow is a read-only, non-owning view of the one
// retained null descriptor. It intentionally exposes neither Close nor the
// descriptor itself to the process request.
type retainedNullProcessBorrow struct {
	device *retainedNullDevice
}

func (device *retainedNullDevice) borrowProcessInput() (retainedNullProcessBorrow, bool) {
	if device == nil || device.leaf == nil || device.leaf.descriptor == nil ||
		device.leaf.path != nullDevicePath || len(device.leaf.pathClaims) == 0 {
		return retainedNullProcessBorrow{}, false
	}
	if validateNullDeviceSnapshot(device.leaf.snapshot) != nil ||
		validateNullDeviceParentTransition(
			device.leaf.snapshot,
			device.leaf.mount,
			device.leaf.pathClaims,
		) != nil {
		return retainedNullProcessBorrow{}, false
	}
	return retainedNullProcessBorrow{device: device}, true
}

func (borrow retainedNullProcessBorrow) processReader() io.Reader {
	if borrow.device == nil || borrow.device.leaf == nil {
		return nil
	}
	return borrow.device.leaf.descriptor
}

func (borrow retainedNullProcessBorrow) revalidate(ctx context.Context) error {
	return validateRetainedNullDeviceContext(ctx, borrow.device)
}

type authorityPathOpenFunc func(
	context.Context,
	string,
	descriptorInspectFunc,
) (*os.File, []authorityPathClaim, error)

type fixedNullDeviceOpenFunc func(int) (*os.File, error)

func retainPhysicalRoot(ctx context.Context) (*physicalRootAuthority, error) {
	if err := checkContext(ctx, "retain physical root"); err != nil {
		return nil, err
	}
	directory, err := openRetainedDirectoryContext(ctx, physicalRootPath)
	if err != nil {
		return nil, err
	}
	authority := &physicalRootAuthority{
		directory: directory,
		claim:     makeAuthorityPathClaim(directory.snapshot, directory.mount, directory.aclDigest),
	}
	if err := validatePhysicalRootAuthorityContext(ctx, authority); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(authority.close()))
	}
	return authority, nil
}

func validatePhysicalRootAuthority(authority *physicalRootAuthority) error {
	return validatePhysicalRootAuthorityContext(context.Background(), authority)
}

func validatePhysicalRootAuthorityContext(
	ctx context.Context,
	authority *physicalRootAuthority,
) error {
	return validatePhysicalRootAuthorityContextWithInspect(ctx, authority, inspectDescriptorContext)
}

func validatePhysicalRootAuthorityContextWithInspect(
	ctx context.Context,
	authority *physicalRootAuthority,
	inspect descriptorInspectFunc,
) error {
	if err := checkContext(ctx, "validate physical root"); err != nil {
		return err
	}
	if inspect == nil {
		return fail(CauseInternalInvariant, "missing physical-root descriptor inspector")
	}
	directory, err := validatePhysicalRootAuthorityShape(authority)
	if err != nil {
		return err
	}

	current, mount, aclDigest, err := inspect(ctx, directory.descriptor)
	if err != nil {
		return err
	}
	if err := validatePhysicalRootSnapshot(current); err != nil {
		return err
	}
	if err := compareSingleAuthorityClaim(
		authority.claim,
		makeAuthorityPathClaim(current, mount, aclDigest),
		"physical root",
	); err != nil {
		return err
	}
	if err := validateRetainedDirectoryHandleIdentity(directory); err != nil {
		return err
	}
	return revalidateAuthorityPathContextWithInspect(ctx, directory, inspect)
}

func validatePhysicalRootAuthorityShape(
	authority *physicalRootAuthority,
) (*retainedDirectory, error) {
	if authority == nil || authority.directory == nil ||
		authority.directory.root == nil || authority.directory.descriptor == nil {
		return nil, fail(CauseInternalInvariant, "missing physical-root authority handles")
	}
	directory := authority.directory
	if directory.path != physicalRootPath || len(directory.pathClaims) != 1 ||
		directory.pathClaims[0] != authority.claim ||
		makeAuthorityPathClaim(directory.snapshot, directory.mount, directory.aclDigest) != authority.claim {
		return nil, fail(CauseInternalInvariant, "invalid physical-root authority claim")
	}
	if err := validatePhysicalRootSnapshot(directory.snapshot); err != nil {
		return nil, err
	}
	return directory, nil
}

func validatePhysicalRootSnapshot(snapshot fileSnapshot) error {
	if snapshotKind(snapshot) != entryDirectory {
		return fail(CauseUnsupported, "physical root is not a directory")
	}
	if snapshot.identity.UID != 0 {
		return fail(CausePermission, "physical root owner refused")
	}
	if snapshot.identity.Mode != platformModeDirectory|0o755 {
		return fail(CausePermission, "physical root mode refused")
	}
	return nil
}

func validateRetainedDirectoryHandleIdentity(directory *retainedDirectory) error {
	info, err := directory.root.Lstat(".")
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "stat retained physical root")
	}
	descriptorInfo, err := directory.descriptor.Stat()
	if err != nil {
		return failWithFilesystemCauses(err, classifyPathError(err), "stat physical-root descriptor")
	}
	if !os.SameFile(descriptorInfo, info) {
		return fail(CauseIdentity, "physical-root handles differ")
	}
	return nil
}

func compareSingleAuthorityClaim(expected, current authorityPathClaim, subject string) error {
	if !sameFilesystemObject(expected.identity, current.identity) {
		return fail(CauseIdentity, subject+" identity changed")
	}
	if expected != current {
		return fail(CauseUnstable, subject+" security claim changed")
	}
	return nil
}

func (authority *physicalRootAuthority) close() error {
	if authority == nil || authority.directory == nil {
		return nil
	}
	return closePhysicalRootHandles(authority.directory.descriptor, authority.directory.root)
}

// retainPhysicalRoot acquires os.Root before the separately opened descriptor,
// so this capability closes the descriptor first. Other retainedDirectory
// issuers have their own acquisition order and retain their existing owner.
func closePhysicalRootHandles(descriptor, root io.Closer) error {
	var closeErrors []error
	if descriptor != nil {
		closeErrors = append(closeErrors, descriptor.Close())
	}
	if root != nil {
		closeErrors = append(closeErrors, root.Close())
	}
	return errors.Join(closeErrors...)
}

func retainNullDevice(ctx context.Context) (*retainedNullDevice, error) {
	return retainNullDeviceContextWith(
		ctx,
		inspectNullAuthorityDescriptorContext,
		inspectNullAuthorityDescriptorContext,
		openAuthorityPathContextWithInspect,
		openFixedNullDeviceNoFollow,
	)
}

func retainNullDeviceContextWith(
	ctx context.Context,
	inspectPath descriptorInspectFunc,
	inspectLeaf descriptorInspectFunc,
	openPath authorityPathOpenFunc,
	openNull fixedNullDeviceOpenFunc,
) (*retainedNullDevice, error) {
	if err := checkContext(ctx, "retain null device"); err != nil {
		return nil, err
	}
	if inspectPath == nil || inspectLeaf == nil || openPath == nil || openNull == nil {
		return nil, fail(CauseInternalInvariant, "missing null-device authority primitive")
	}
	parent, pathClaims, err := openPath(ctx, nullDeviceParentPath, inspectPath)
	if err != nil {
		return nil, err
	}
	if parent == nil || len(pathClaims) == 0 {
		return nil, joinFilesystemFailures(
			fail(CauseInternalInvariant, "missing null-device parent claim"),
			closeOptionalDescriptor(parent),
		)
	}
	descriptor, openErr := openNull(int(parent.Fd()))
	if openErr != nil {
		return nil, joinFilesystemFailures(
			failWithFilesystemCauses(openErr, classifyPathError(openErr), "open retained null device"),
			closeDescriptorFailure(parent.Close()),
		)
	}
	snapshot, mount, aclDigest, inspectErr := inspectLeaf(ctx, descriptor)
	if inspectErr != nil {
		return nil, joinFilesystemFailures(
			inspectErr,
			closeDescriptorFailure(descriptor.Close()),
			closeDescriptorFailure(parent.Close()),
		)
	}
	if policyErr := validateNullDeviceSnapshot(snapshot); policyErr != nil {
		return nil, joinFilesystemFailures(
			policyErr,
			closeDescriptorFailure(descriptor.Close()),
			closeDescriptorFailure(parent.Close()),
		)
	}
	if transitionErr := validateNullDeviceParentTransition(snapshot, mount, pathClaims); transitionErr != nil {
		return nil, joinFilesystemFailures(
			transitionErr,
			closeDescriptorFailure(descriptor.Close()),
			closeDescriptorFailure(parent.Close()),
		)
	}
	device := &retainedNullDevice{leaf: &retainedLeaf{
		path:       nullDevicePath,
		descriptor: descriptor,
		snapshot:   snapshot,
		mount:      mount,
		aclDigest:  aclDigest,
		pathClaims: append([]authorityPathClaim(nil), pathClaims...),
	}}
	if closeErr := closeDescriptorFailure(parent.Close()); closeErr != nil {
		return nil, joinFilesystemFailures(closeErr, closeDescriptorFailure(device.close()))
	}
	if err := validateRetainedNullDeviceContextWith(
		ctx,
		device,
		inspectPath,
		inspectLeaf,
		openPath,
		openNull,
	); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(device.close()))
	}
	return device, nil
}

func validateRetainedNullDevice(device *retainedNullDevice) error {
	return validateRetainedNullDeviceContext(context.Background(), device)
}

func validateRetainedNullDeviceContext(ctx context.Context, device *retainedNullDevice) error {
	return validateRetainedNullDeviceContextWith(
		ctx,
		device,
		inspectNullAuthorityDescriptorContext,
		inspectNullAuthorityDescriptorContext,
		openAuthorityPathContextWithInspect,
		openFixedNullDeviceNoFollow,
	)
}

func validateRetainedNullDeviceContextWith(
	ctx context.Context,
	device *retainedNullDevice,
	inspectPath descriptorInspectFunc,
	inspectLeaf descriptorInspectFunc,
	openPath authorityPathOpenFunc,
	openNull fixedNullDeviceOpenFunc,
) error {
	if err := checkContext(ctx, "validate retained null device"); err != nil {
		return err
	}
	if inspectPath == nil || inspectLeaf == nil || openPath == nil || openNull == nil {
		return fail(CauseInternalInvariant, "missing null-device validation primitive")
	}
	leaf, err := validateRetainedNullDeviceShape(device)
	if err != nil {
		return err
	}
	if err := validateRetainedNullLeafCurrent(ctx, leaf, inspectLeaf); err != nil {
		return err
	}
	return revalidateRetainedNullBinding(ctx, leaf, inspectPath, inspectLeaf, openPath, openNull)
}

func validateRetainedNullDeviceShape(device *retainedNullDevice) (*retainedLeaf, error) {
	if device == nil || device.leaf == nil || device.leaf.descriptor == nil {
		return nil, fail(CauseInternalInvariant, "missing retained null-device handle")
	}
	leaf := device.leaf
	if leaf.path != nullDevicePath || len(leaf.pathClaims) == 0 {
		return nil, fail(CauseInternalInvariant, "invalid retained null-device claim")
	}
	if err := validateNullDeviceSnapshot(leaf.snapshot); err != nil {
		return nil, err
	}
	if err := validateNullDeviceParentTransition(leaf.snapshot, leaf.mount, leaf.pathClaims); err != nil {
		return nil, err
	}
	return leaf, nil
}

func validateRetainedNullLeafCurrent(
	ctx context.Context,
	leaf *retainedLeaf,
	inspectLeaf descriptorInspectFunc,
) error {
	current, mount, aclDigest, err := inspectLeaf(ctx, leaf.descriptor)
	if err != nil {
		return err
	}
	if err := validateNullDeviceSnapshot(current); err != nil {
		return err
	}
	if err := validateNullDeviceParentTransition(current, mount, leaf.pathClaims); err != nil {
		return err
	}
	if err := compareRetainedLeafClaim(leaf, current, mount, aclDigest); err != nil {
		return err
	}
	return nil
}

func revalidateRetainedNullBinding(
	ctx context.Context,
	leaf *retainedLeaf,
	inspectPath descriptorInspectFunc,
	inspectLeaf descriptorInspectFunc,
	openPath authorityPathOpenFunc,
	openNull fixedNullDeviceOpenFunc,
) error {
	parent, currentPathClaims, err := openPath(ctx, nullDeviceParentPath, inspectPath)
	if err != nil {
		return err
	}
	if parent == nil || len(currentPathClaims) == 0 {
		return joinFilesystemFailures(
			fail(CauseInternalInvariant, "missing current null-device parent claim"),
			closeOptionalDescriptor(parent),
		)
	}
	if err := compareAuthorityPathClaims(leaf.pathClaims, currentPathClaims); err != nil {
		return joinFilesystemFailures(err, closeDescriptorFailure(parent.Close()))
	}
	fresh, openErr := openNull(int(parent.Fd()))
	if openErr != nil {
		return joinFilesystemFailures(
			failWithFilesystemCauses(openErr, classifyPathError(openErr), "reopen retained null device"),
			closeDescriptorFailure(parent.Close()),
		)
	}
	freshSnapshot, freshMount, freshACL, inspectErr := inspectLeaf(ctx, fresh)
	if inspectErr == nil {
		inspectErr = validateNullDeviceSnapshot(freshSnapshot)
	}
	if inspectErr == nil {
		inspectErr = validateNullDeviceParentTransition(freshSnapshot, freshMount, currentPathClaims)
	}
	if inspectErr == nil {
		inspectErr = compareRetainedLeafClaim(leaf, freshSnapshot, freshMount, freshACL)
	}
	return joinFilesystemFailures(
		inspectErr,
		closeDescriptorFailure(fresh.Close()),
		closeDescriptorFailure(parent.Close()),
	)
}

func validateNullDeviceSnapshot(snapshot fileSnapshot) error {
	return validatePlatformNullDeviceSnapshot(snapshot)
}

func compareRetainedLeafClaim(
	leaf *retainedLeaf,
	snapshot fileSnapshot,
	mount mountSnapshot,
	aclDigest Digest,
) error {
	if !sameFilesystemObject(leaf.snapshot.identity, snapshot.identity) {
		return fail(CauseIdentity, "retained leaf identity changed")
	}
	if leaf.snapshot != snapshot || leaf.mount != mount || leaf.aclDigest != aclDigest {
		return fail(CauseUnstable, "retained leaf security claim changed")
	}
	return nil
}

func validateNullDeviceParentTransition(
	snapshot fileSnapshot,
	mount mountSnapshot,
	pathClaims []authorityPathClaim,
) error {
	if len(pathClaims) == 0 {
		return fail(CauseInternalInvariant, "missing null-device parent claim")
	}
	parent := pathClaims[len(pathClaims)-1]
	if snapshot.identity.Device != parent.identity.Device ||
		snapshot.identity.Filesystem != parent.identity.Filesystem || mount != parent.mount {
		return fail(CauseUnsupported, "null device crosses its retained parent mount")
	}
	return nil
}

func closeOptionalDescriptor(descriptor *os.File) error {
	if descriptor == nil {
		return nil
	}
	return closeDescriptorFailure(descriptor.Close())
}

func (leaf *retainedLeaf) close() error {
	if leaf == nil || leaf.descriptor == nil {
		return nil
	}
	return leaf.descriptor.Close()
}

func (device *retainedNullDevice) close() error {
	if device == nil {
		return nil
	}
	return device.leaf.close()
}
