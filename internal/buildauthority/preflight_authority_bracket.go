package buildauthority

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// authorityACLComparisonObservation separates a canonical ACL observation
// from the static policy that applies to it. Revalidation compares the digest
// first so changed authority remains compare/unstable even when the changed
// ACL would also violate the admission policy.
type authorityACLComparisonObservation struct {
	digest        Digest
	policyFailure *authorityPrimitiveFailure
}

// authorityDescriptorComparisonObservation carries the same deferred-policy
// ordering through the complete descriptor observation.
type authorityDescriptorComparisonObservation struct {
	value         authorityDescriptorObservation
	policyFailure *authorityPrimitiveFailure
}

func validateDeferredAuthorityPolicy(
	ctx context.Context,
	failure *authorityPrimitiveFailure,
) *authorityPrimitiveFailure {
	if contextFailure := authorityContextPrimitiveFailure(ctx, OperationValidate); contextFailure != nil {
		return contextFailure
	}
	if failure == nil {
		return nil
	}
	if failure.operation != OperationValidate ||
		(failure.cause != CausePermission && failure.cause != CauseUnsupported) {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	return failure
}

// preflightAuthorityBracketPrimitives is the deterministic substitution seam
// for the authority brackets consumed by the future non-source runner. The
// runner never sees this mechanism vocabulary; it receives only the six
// nominal revalidation methods below.
type preflightAuthorityBracketPrimitives struct {
	openAbsoluteRoot func(
		context.Context,
	) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure)
	openRootHandle func(
		context.Context,
		*os.Root,
	) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure)
	openNullRelativeNoFollow func(
		context.Context,
		*os.File,
		string,
		authorityRequiredOpenMode,
	) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure)
	probeRelativeKind func(
		context.Context,
		*os.File,
		string,
		authorityRequiredOpenMode,
	) (entryKind, *authorityPrimitiveFailure)
	openTypedRelativeNoFollow func(
		context.Context,
		*os.File,
		string,
		entryKind,
		authorityRequiredOpenMode,
	) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure)
	readDirectory func(
		context.Context,
		*os.File,
		int,
	) ([]string, bool, *authorityPrimitiveFailure)
	readSymlinkForWalk func(
		context.Context,
		*os.File,
		int64,
		uint64,
	) (string, *authorityPrimitiveFailure)
	hashDescriptor func(
		context.Context,
		io.ReaderAt,
		int64,
		uint64,
	) (Digest, *authorityPrimitiveFailure)
	parseMachO func(
		context.Context,
		io.ReaderAt,
		int64,
		machOProfile,
	) *authorityPrimitiveFailure
	hashManifest func(
		context.Context,
		string,
		*retainedDirectory,
		[]entrySnapshot,
	) (Digest, *authorityPrimitiveFailure)
	acquireRawNullACL func(
		context.Context,
		*os.File,
	) ([]byte, *authorityPrimitiveFailure)
	parseRawACLForComparison func(
		context.Context,
		[]byte,
	) (authorityACLComparisonObservation, *authorityPrimitiveFailure)
	parseRawNullACLForComparison func(
		context.Context,
		[]byte,
	) (authorityACLComparisonObservation, *authorityPrimitiveFailure)
}

func (primitives preflightAuthorityBracketPrimitives) valid() bool {
	return primitives.openAbsoluteRoot != nil &&
		primitives.openRootHandle != nil &&
		primitives.openNullRelativeNoFollow != nil &&
		primitives.probeRelativeKind != nil &&
		primitives.openTypedRelativeNoFollow != nil &&
		primitives.readDirectory != nil &&
		primitives.readSymlinkForWalk != nil &&
		primitives.hashDescriptor != nil &&
		primitives.parseMachO != nil &&
		primitives.hashManifest != nil &&
		primitives.acquireRawNullACL != nil &&
		primitives.parseRawACLForComparison != nil &&
		primitives.parseRawNullACLForComparison != nil
}

func (revalidator *preflightAuthorityRevalidator) validForBrackets() bool {
	return revalidator != nil && revalidator.primitives.valid() &&
		revalidator.primitives.brackets.valid()
}

// resolvedAuthorityPath owns only descriptors acquired after the borrowed
// physical-root descriptor. A borrowed retained descriptor is never closed by
// one authority use.
type resolvedAuthorityPath struct {
	descriptor *os.File
	owner      *ownedAuthorityDescriptor
}

type authorityDescriptorObserver func(
	context.Context,
	*os.File,
) (authorityDescriptorComparisonObservation, *authorityPrimitiveFailure)

type sealedPhysicalRootBracket struct {
	authority *physicalRootAuthority
	directory *retainedDirectory
	value     retainedDirectory
	claim     authorityPathClaim
}

func sealPhysicalRootBracket(
	physicalRoot *physicalRootAuthority,
) (sealedPhysicalRootBracket, *authorityPrimitiveFailure) {
	directory, err := validatePhysicalRootAuthorityShape(physicalRoot)
	if err != nil {
		return sealedPhysicalRootBracket{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	value := *directory
	value.pathClaims = append([]authorityPathClaim(nil), directory.pathClaims...)
	return sealedPhysicalRootBracket{
		authority: physicalRoot,
		directory: directory,
		value:     value,
		claim:     physicalRoot.claim,
	}, nil
}

func comparePhysicalRootBracketSeal(
	ctx context.Context,
	sealed sealedPhysicalRootBracket,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !samePhysicalRootBracketSeal(sealed) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func samePhysicalRootBracketSeal(sealed sealedPhysicalRootBracket) bool {
	physicalRoot := sealed.authority
	directory := sealed.directory
	return physicalRoot != nil && physicalRoot.directory == directory && directory != nil &&
		physicalRoot.claim == sealed.claim && directory.path == sealed.value.path &&
		directory.root == sealed.value.root && directory.descriptor == sealed.value.descriptor &&
		directory.snapshot == sealed.value.snapshot && directory.mount == sealed.value.mount &&
		directory.aclDigest == sealed.value.aclDigest &&
		equalAuthorityPathClaimSlices(directory.pathClaims, sealed.value.pathClaims)
}

func (path *resolvedAuthorityPath) closeInto(outcome *authorityUseOutcome) {
	if path != nil && path.owner != nil {
		path.owner.closeInto(outcome)
	}
}

func (revalidator *preflightAuthorityRevalidator) resolveClaimedAuthorityPath(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	path string,
	claims []authorityPathClaim,
) (*resolvedAuthorityPath, authorityUseOutcome) {
	return revalidator.resolveClaimedAuthorityPathWith(
		ctx,
		physicalRoot,
		path,
		claims,
		revalidator.probeDescriptorObservation,
	)
}

// resolveNullClaimedAuthorityPath preserves the ACL observation profile used
// when the /dev/null parent claims were admitted. Darwin devfs refuses the
// ordinary extended-security query for /dev, so replaying those claims through
// the ordinary observer would reject an unchanged authority.
func (revalidator *preflightAuthorityRevalidator) resolveNullClaimedAuthorityPath(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	path string,
	claims []authorityPathClaim,
) (*resolvedAuthorityPath, authorityUseOutcome) {
	return revalidator.resolveClaimedAuthorityPathWith(
		ctx,
		physicalRoot,
		path,
		claims,
		revalidator.probeNullDescriptorObservation,
	)
}

func (revalidator *preflightAuthorityRevalidator) resolveClaimedAuthorityPathWith(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	path string,
	claims []authorityPathClaim,
	observe authorityDescriptorObserver,
) (*resolvedAuthorityPath, authorityUseOutcome) {
	var outcome authorityUseOutcome
	if observe == nil {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		))
		return nil, outcome
	}
	directory, components, failure := validateClaimedAuthorityPathInputs(
		physicalRoot,
		path,
		claims,
	)
	if failure != nil {
		outcome.addPrimitive(failure)
		return nil, outcome
	}
	current := directory.descriptor
	var currentOwner *ownedAuthorityDescriptor
	for index, expected := range claims {
		observation, failure := observeAndCompareClaimedAuthorityPath(
			ctx,
			expected,
			current,
			observe,
		)
		if failure != nil {
			outcome.addPrimitive(failure)
			if currentOwner != nil {
				currentOwner.closeInto(&outcome)
			}
			return nil, outcome
		}
		if failure = revalidator.primitives.validateFilesystem(ctx, observation.mount); failure != nil {
			outcome.addPrimitive(failure)
			if currentOwner != nil {
				currentOwner.closeInto(&outcome)
			}
			return nil, outcome
		}
		if index == len(components) {
			return &resolvedAuthorityPath{descriptor: current, owner: currentOwner}, outcome
		}
		nextOwner, acquisitionFailure := revalidator.openClaimedAuthorityDirectory(
			ctx,
			current,
			components[index],
		)
		outcome.addPrimitive(acquisitionFailure)
		if currentOwner != nil {
			currentOwner.closeInto(&outcome)
		}
		if acquisitionFailure != nil || nextOwner == nil {
			if nextOwner != nil {
				nextOwner.closeInto(&outcome)
			}
			return nil, outcome
		}
		current = nextOwner.file
		currentOwner = nextOwner
	}
	outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
	if currentOwner != nil {
		currentOwner.closeInto(&outcome)
	}
	return nil, outcome
}

func observeAndCompareClaimedAuthorityPath(
	ctx context.Context,
	expected authorityPathClaim,
	descriptor *os.File,
	observe authorityDescriptorObserver,
) (authorityDescriptorObservation, *authorityPrimitiveFailure) {
	comparison, failure := observe(ctx, descriptor)
	if failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	if failure := compareAuthorityPathObservation(ctx, expected, comparison.value); failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	if failure := validateDeferredAuthorityPolicy(ctx, comparison.policyFailure); failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	return comparison.value, nil
}

func (revalidator *preflightAuthorityRevalidator) openClaimedAuthorityDirectory(
	ctx context.Context,
	parent *os.File,
	name string,
) (*ownedAuthorityDescriptor, *authorityPrimitiveFailure) {
	kind, failure := revalidator.primitives.brackets.probeRelativeKind(
		ctx,
		parent,
		name,
		authorityExpectedPresentOpen,
	)
	if failure != nil {
		return nil, failure
	}
	if kind != entryDirectory {
		return nil, newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	acquisition, openFailure := revalidator.primitives.brackets.openTypedRelativeNoFollow(
		ctx,
		parent,
		name,
		entryDirectory,
		authorityExpectedPresentOpen,
	)
	return ownAuthorityAcquisition(acquisition, openFailure)
}

func validateClaimedAuthorityPathInputs(
	physicalRoot *physicalRootAuthority,
	path string,
	claims []authorityPathClaim,
) (*retainedDirectory, []string, *authorityPrimitiveFailure) {
	directory, err := validatePhysicalRootAuthorityShape(physicalRoot)
	components, componentErr := absolutePathComponents(path)
	if err != nil || componentErr != nil || len(claims) != len(components)+1 ||
		len(claims) == 0 || claims[0] != physicalRoot.claim ||
		!validSealedAuthorityPathClaims(claims) {
		return nil, nil, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	return directory, components, nil
}

func validSealedAuthorityPathClaims(claims []authorityPathClaim) bool {
	if len(claims) == 0 {
		return false
	}
	for _, claim := range claims {
		snapshot := fileSnapshot{
			identity:   claim.identity,
			gid:        claim.gid,
			rdev:       claim.rdev,
			birthSec:   claim.birthSec,
			birthNsec:  claim.birthNsec,
			flags:      claim.flags,
			generation: claim.generation,
		}
		if snapshotKind(snapshot) != entryDirectory ||
			snapshot.identity.Filesystem != claim.mount.filesystem ||
			validateProtected(snapshot, ownerRootOrEffective) != nil {
			return false
		}
	}
	return true
}

func (revalidator *preflightAuthorityRevalidator) revalidatePhysicalRoot(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	if ctx == nil || !revalidator.validForBrackets() {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
		return outcome
	}
	sealed, failure := sealPhysicalRootBracket(physicalRoot)
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	directory := sealed.directory
	if failure := revalidator.observeAndComparePhysicalRoot(ctx, sealed.claim, directory.descriptor); failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	handleAcquisition, handleOpenFailure := revalidator.primitives.brackets.openRootHandle(
		ctx,
		directory.root,
	)
	handleOwner, handleAcquisitionFailure := ownAuthorityAcquisition(
		handleAcquisition,
		handleOpenFailure,
	)
	outcome.addPrimitive(handleAcquisitionFailure)
	if handleAcquisitionFailure == nil && handleOwner != nil {
		outcome.addPrimitive(revalidator.observeAndComparePhysicalRoot(
			ctx,
			sealed.claim,
			handleOwner.file,
		))
	}
	if handleOwner != nil {
		handleOwner.closeInto(&outcome)
	}
	if outcome.primary != nil {
		return outcome
	}
	acquisition, openFailure := revalidator.primitives.brackets.openAbsoluteRoot(ctx)
	owner, acquisitionFailure := ownAuthorityAcquisition(acquisition, openFailure)
	outcome.addPrimitive(acquisitionFailure)
	if acquisitionFailure == nil && owner != nil {
		outcome.addPrimitive(revalidator.observeAndComparePhysicalRoot(ctx, sealed.claim, owner.file))
	}
	if owner != nil {
		owner.closeInto(&outcome)
	}
	if failure := comparePhysicalRootBracketSeal(ctx, sealed); failure != nil {
		outcome.addPrimitive(failure)
	}
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) observeAndComparePhysicalRoot(
	ctx context.Context,
	expected authorityPathClaim,
	descriptor *os.File,
) *authorityPrimitiveFailure {
	comparison, failure := revalidator.probeDescriptorObservation(ctx, descriptor)
	if failure != nil {
		return failure
	}
	observation := comparison.value
	if failure := compareAuthorityPathObservation(ctx, expected, observation); failure != nil {
		return failure
	}
	if failure := validateDeferredAuthorityPolicy(ctx, comparison.policyFailure); failure != nil {
		return failure
	}
	if failure := revalidator.primitives.validateFilesystem(ctx, observation.mount); failure != nil {
		return failure
	}
	if err := validatePhysicalRootSnapshot(observation.snapshot); err != nil {
		return authorityPrimitiveFailureFromError(OperationValidate, err, CauseUnsupported)
	}
	return nil
}

func (revalidator *preflightAuthorityRevalidator) revalidateRetainedNull(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	nullDevice *retainedNullDevice,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	if ctx == nil || !revalidator.validForBrackets() {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
		return outcome
	}
	physicalSeal, failure := sealPhysicalRootBracket(physicalRoot)
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	leaf, err := validateRetainedNullDeviceShape(nullDevice)
	if err != nil {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		))
		return outcome
	}
	sealedLeaf := *leaf
	sealedLeaf.pathClaims = append([]authorityPathClaim(nil), leaf.pathClaims...)
	if _, err := validatePhysicalRootAuthorityShape(physicalRoot); err != nil ||
		len(sealedLeaf.pathClaims) != 2 || sealedLeaf.pathClaims[0] != physicalRoot.claim ||
		!validSealedAuthorityPathClaims(sealedLeaf.pathClaims) {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
		return outcome
	}
	retained, failure := revalidator.probeNullDescriptorObservation(ctx, leaf.descriptor)
	if failure == nil {
		failure = revalidator.validateNullObservation(ctx, &sealedLeaf, retained)
	}
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	parentPath := filepath.Dir(sealedLeaf.path)
	parentClaims := sealedLeaf.pathClaims
	parent, pathOutcome := revalidator.resolveNullClaimedAuthorityPath(
		ctx,
		physicalRoot,
		parentPath,
		parentClaims,
	)
	outcome.absorb(pathOutcome)
	if parent != nil {
		revalidator.useFreshNullLeaf(ctx, parent, &sealedLeaf, &outcome)
	}
	// Rebind the pathname even after a fresh-leaf or close failure.
	trailing, trailingOutcome := revalidator.resolveNullClaimedAuthorityPath(
		ctx,
		physicalRoot,
		parentPath,
		parentClaims,
	)
	outcome.absorb(trailingOutcome)
	if trailing != nil {
		trailing.closeInto(&outcome)
	}
	if failure := compareRetainedNullSeal(ctx, nullDevice, leaf, sealedLeaf); failure != nil {
		outcome.addPrimitive(failure)
	}
	if failure := comparePhysicalRootBracketSeal(ctx, physicalSeal); failure != nil {
		outcome.addPrimitive(failure)
	}
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) probeNullDescriptorObservation(
	ctx context.Context,
	descriptor *os.File,
) (authorityDescriptorComparisonObservation, *authorityPrimitiveFailure) {
	snapshot, failure := revalidator.primitives.statDescriptor(ctx, descriptor)
	if failure != nil {
		return authorityDescriptorComparisonObservation{}, failure
	}
	mount, failure := revalidator.primitives.statFilesystem(ctx, descriptor)
	if failure != nil {
		return authorityDescriptorComparisonObservation{}, failure
	}
	snapshot.identity.Filesystem = mount.filesystem
	rawACL, failure := revalidator.primitives.brackets.acquireRawNullACL(ctx, descriptor)
	if failure != nil {
		return authorityDescriptorComparisonObservation{}, failure
	}
	acl, failure := revalidator.primitives.brackets.parseRawNullACLForComparison(ctx, rawACL)
	if failure != nil {
		return authorityDescriptorComparisonObservation{}, failure
	}
	return authorityDescriptorComparisonObservation{
		value: authorityDescriptorObservation{
			snapshot:  snapshot,
			mount:     mount,
			aclDigest: acl.digest,
		},
		policyFailure: acl.policyFailure,
	}, nil
}

func (revalidator *preflightAuthorityRevalidator) validateNullObservation(
	ctx context.Context,
	leaf *retainedLeaf,
	comparison authorityDescriptorComparisonObservation,
) *authorityPrimitiveFailure {
	observation := comparison.value
	if failure := compareRetainedLeafObservation(ctx, leaf, observation); failure != nil {
		return failure
	}
	if failure := validateDeferredAuthorityPolicy(ctx, comparison.policyFailure); failure != nil {
		return failure
	}
	if failure := revalidator.primitives.validateFilesystem(ctx, observation.mount); failure != nil {
		return failure
	}
	if err := validateNullDeviceSnapshot(observation.snapshot); err != nil {
		return authorityPrimitiveFailureFromError(OperationValidate, err, CauseUnsupported)
	}
	if err := validateNullDeviceParentTransition(
		observation.snapshot,
		observation.mount,
		leaf.pathClaims,
	); err != nil {
		return authorityPrimitiveFailureFromError(OperationValidate, err, CauseUnsupported)
	}
	return nil
}

func compareRetainedLeafObservation(
	ctx context.Context,
	leaf *retainedLeaf,
	observation authorityDescriptorObservation,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !sameFilesystemObject(leaf.snapshot.identity, observation.snapshot.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if leaf.snapshot != observation.snapshot || leaf.mount != observation.mount ||
		leaf.aclDigest != observation.aclDigest {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func (revalidator *preflightAuthorityRevalidator) useFreshNullLeaf(
	ctx context.Context,
	parent *resolvedAuthorityPath,
	retained *retainedLeaf,
	outcome *authorityUseOutcome,
) {
	acquisition, openFailure := revalidator.primitives.brackets.openNullRelativeNoFollow(
		ctx,
		parent.descriptor,
		filepath.Base(retained.path),
		authorityExpectedPresentOpen,
	)
	owner, acquisitionFailure := ownAuthorityAcquisition(acquisition, openFailure)
	outcome.addPrimitive(acquisitionFailure)
	parent.closeInto(outcome)
	if acquisitionFailure == nil && owner != nil {
		observation, failure := revalidator.probeNullDescriptorObservation(ctx, owner.file)
		if failure == nil {
			failure = revalidator.validateNullObservation(ctx, retained, observation)
		}
		outcome.addPrimitive(failure)
	}
	if owner != nil {
		owner.closeInto(outcome)
	}
}

func compareRetainedNullSeal(
	ctx context.Context,
	device *retainedNullDevice,
	retained *retainedLeaf,
	sealed retainedLeaf,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if device == nil || device.leaf != retained || retained == nil ||
		retained.path != sealed.path || retained.descriptor != sealed.descriptor ||
		retained.snapshot != sealed.snapshot || retained.mount != sealed.mount ||
		retained.aclDigest != sealed.aclDigest || sealed.path != nullDevicePath ||
		sealed.descriptor == nil ||
		!equalAuthorityPathClaimSlices(retained.pathClaims, sealed.pathClaims) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}
