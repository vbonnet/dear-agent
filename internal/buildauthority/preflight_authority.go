package buildauthority

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"sync"
)

// authorityPrimitiveFailure preserves the operation selected by the primitive
// that failed. Raw operating-system errors never cross this seam.
type authorityPrimitiveFailure struct {
	operation Operation
	cause     CauseCode
}

func newAuthorityPrimitiveFailure(
	operation Operation,
	cause CauseCode,
) *authorityPrimitiveFailure {
	return &authorityPrimitiveFailure{operation: operation, cause: cause}
}

func (failure *authorityPrimitiveFailure) record() *FailureRecord {
	if failure == nil {
		return nil
	}
	return authorityFailure(failure.operation, failure.cause)
}

// authorityUseOutcome is the complete private result of one authority use.
// later never crosses the module seam: the future block owner consumes it only
// while composing the current command-end or block-end outcome.
type authorityUseOutcome struct {
	primary         *FailureRecord
	later           *FailureRecord
	descriptorClose *FailureRecord
}

// authorityDescriptorAcquisition is the complete result of a successful raw
// open. close remains usable even in the impossible adapter edge where a valid
// platform descriptor cannot be represented as *os.File.
type authorityDescriptorAcquisition struct {
	file  *os.File
	close func() error
}

// authorityRequiredOpenMode closes the only two absence interpretations for a
// required retained row. The primitive, rather than its caller, owns the
// resulting operation attribution.
type authorityRequiredOpenMode uint8

const (
	authorityInitialRequiredOpen authorityRequiredOpenMode = iota + 1
	authorityExpectedPresentOpen
)

func (mode authorityRequiredOpenMode) valid() bool {
	return mode == authorityInitialRequiredOpen || mode == authorityExpectedPresentOpen
}

func requiredAuthorityOpenNotFound(
	mode authorityRequiredOpenMode,
) *authorityPrimitiveFailure {
	switch mode {
	case authorityInitialRequiredOpen:
		return newAuthorityPrimitiveFailure(OperationOpen, CauseNotFound)
	case authorityExpectedPresentOpen:
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	default:
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
}

func (outcome *authorityUseOutcome) addPrimitive(failure *authorityPrimitiveFailure) {
	if failure == nil {
		return
	}
	outcome.addNonClose(failure.record())
}

func (outcome *authorityUseOutcome) addNonClose(record *FailureRecord) {
	if outcome == nil || record == nil {
		return
	}
	switch {
	case outcome.primary == nil:
		outcome.primary = cloneFailureRecord(record)
	case outcome.later == nil:
		outcome.later = cloneFailureRecord(record)
	}
}

func (outcome *authorityUseOutcome) addDescriptorClose() {
	if outcome == nil || outcome.descriptorClose != nil {
		return
	}
	outcome.descriptorClose = &FailureRecord{
		Phase:     PhaseClose,
		Operation: OperationCloseNonRoot,
		Causes:    []CauseCode{CauseDescriptorClose},
	}
}

func (outcome *authorityUseOutcome) absorb(next authorityUseOutcome) {
	if outcome == nil {
		return
	}
	outcome.addNonClose(next.primary)
	outcome.addNonClose(next.later)
	if next.descriptorClose == nil {
		return
	}
	outcome.addDescriptorClose()
}

func (outcome authorityUseOutcome) proved() bool {
	return outcome.primary == nil && outcome.later == nil && outcome.descriptorClose == nil
}

// ownedAuthorityDescriptor is pointer-only. It gives every transient
// descriptor one exact-once close owner before any post-open validation runs.
type ownedAuthorityDescriptor struct {
	file         *os.File
	closeFn      func() error
	closeOnce    sync.Once
	closeFailure bool
}

func ownAuthorityDescriptor(
	acquisition *authorityDescriptorAcquisition,
) (*ownedAuthorityDescriptor, *authorityPrimitiveFailure) {
	if acquisition == nil {
		return nil, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	closeFn := acquisition.close
	if closeFn == nil && acquisition.file != nil {
		closeFn = acquisition.file.Close
	}
	if closeFn == nil {
		return nil, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	owner := &ownedAuthorityDescriptor{
		file:    acquisition.file,
		closeFn: closeFn,
	}
	if acquisition.file == nil || acquisition.close == nil {
		return owner, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	return owner, nil
}

// ownAuthorityAcquisition installs the exact-once owner before interpreting
// any acquisition failure. A nonnil acquisition always returns its owner so
// the caller can close it even when the adapter also reported a failure.
func ownAuthorityAcquisition(
	acquisition *authorityDescriptorAcquisition,
	acquisitionFailure *authorityPrimitiveFailure,
) (*ownedAuthorityDescriptor, *authorityPrimitiveFailure) {
	if acquisition == nil {
		if acquisitionFailure == nil {
			acquisitionFailure = newAuthorityPrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			)
		}
		return nil, acquisitionFailure
	}
	owner, ownershipFailure := ownAuthorityDescriptor(acquisition)
	if acquisitionFailure != nil {
		return owner, acquisitionFailure
	}
	return owner, ownershipFailure
}

func (owner *ownedAuthorityDescriptor) closeInto(outcome *authorityUseOutcome) {
	if owner == nil {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		))
		return
	}
	owner.closeOnce.Do(func() {
		owner.closeFailure = owner.closeFn() != nil
	})
	if owner.closeFailure {
		outcome.addDescriptorClose()
	}
}

// preflightAuthorityPrimitives is the local-substitution seam for the live
// go.env transaction. Every callback except close returns an already
// operation-attributed, public-safe failure.
type preflightAuthorityPrimitives struct {
	openRelativeNoFollow func(
		context.Context,
		*os.File,
		string,
		authorityRequiredOpenMode,
	) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure)
	statDescriptor func(
		context.Context,
		*os.File,
	) (fileSnapshot, *authorityPrimitiveFailure)
	statFilesystem func(
		context.Context,
		*os.File,
	) (mountSnapshot, *authorityPrimitiveFailure)
	validateFilesystem func(
		context.Context,
		mountSnapshot,
	) *authorityPrimitiveFailure
	acquireRawACL func(
		context.Context,
		*os.File,
	) ([]byte, *authorityPrimitiveFailure)
	parseRawACL func(
		context.Context,
		[]byte,
	) (Digest, *authorityPrimitiveFailure)
	readExactForParse func(
		context.Context,
		io.ReaderAt,
		[]byte,
	) *authorityPrimitiveFailure
	hashBytes func(
		context.Context,
		[]byte,
	) (Digest, *authorityPrimitiveFailure)
}

func (primitives preflightAuthorityPrimitives) valid() bool {
	return primitives.openRelativeNoFollow != nil &&
		primitives.statDescriptor != nil &&
		primitives.statFilesystem != nil &&
		primitives.validateFilesystem != nil &&
		primitives.acquireRawACL != nil &&
		primitives.parseRawACL != nil &&
		primitives.readExactForParse != nil &&
		primitives.hashBytes != nil
}

// preflightAuthorityRevalidator owns the complete physical-root-anchored
// fresh-leaf transaction. Full retained-tree recapture belongs to the next
// checkpoint and is deliberately absent from this type.
type preflightAuthorityRevalidator struct {
	primitives preflightAuthorityPrimitives
}

func newPreflightAuthorityRevalidator() (
	*preflightAuthorityRevalidator,
	*FailureRecord,
) {
	primitives, failure := platformPreflightAuthorityPrimitives()
	if failure != nil {
		return nil, failure
	}
	return newPreflightAuthorityRevalidatorWith(primitives)
}

func newPreflightAuthorityRevalidatorWith(
	primitives preflightAuthorityPrimitives,
) (*preflightAuthorityRevalidator, *FailureRecord) {
	if !primitives.valid() {
		return nil, authorityFailure(OperationValidate, CauseInternalInvariant)
	}
	return &preflightAuthorityRevalidator{primitives: primitives}, nil
}

type liveGoEnvironmentInputs struct {
	physicalRootDirectory  *retainedDirectory
	physicalRootHandle     *os.Root
	physicalRootDescriptor *os.File
	gorootDirectory        *retainedDirectory
	gorootRootHandle       *os.Root
	gorootRootDescriptor   *os.File
	gorootPath             string
	components             []string
	pathClaims             []authorityPathClaim
	entry                  entrySnapshot
	gorootDigest           Digest
	gorootMount            mountSnapshot
	gorootDevice           uint64
	owners                 ownerPolicy
	maxFileBytes           uint64
}

type authorityDescriptorObservation struct {
	snapshot  fileSnapshot
	mount     mountSnapshot
	aclDigest Digest
}

type resolvedGOROOTPath struct {
	descriptor *os.File
	owner      *ownedAuthorityDescriptor
}

func (path *resolvedGOROOTPath) closeInto(outcome *authorityUseOutcome) {
	if path != nil && path.owner != nil {
		path.owner.closeInto(outcome)
	}
}

// admitLiveGoEnvironment is the sole production constructor for the sealed
// claim. It publishes the claim only after both path brackets, fresh-leaf
// validation, and every transient close have proved.
func (revalidator *preflightAuthorityRevalidator) admitLiveGoEnvironment(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
) (*goEnvironmentFileClaim, authorityUseOutcome) {
	inputs, initial := validateLiveGoEnvironmentInputs(ctx, physicalRoot, goroot)
	if initial != nil {
		return nil, authorityUseOutcome{primary: initial}
	}
	if revalidator == nil || !revalidator.primitives.valid() {
		return nil, authorityUseOutcome{
			primary: authorityFailure(OperationValidate, CauseInternalInvariant),
		}
	}
	outcome := revalidator.useLiveGoEnvironment(
		ctx,
		physicalRoot,
		goroot,
		inputs,
		authorityInitialRequiredOpen,
	)
	if !outcome.proved() {
		return nil, outcome
	}
	return &goEnvironmentFileClaim{
		goroot:       goroot,
		gorootDigest: inputs.gorootDigest,
		entry:        inputs.entry,
		seal:         validGoEnvironmentFileClaim,
	}, outcome
}

func (revalidator *preflightAuthorityRevalidator) revalidateLiveGoEnvironment(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
	claim *goEnvironmentFileClaim,
) authorityUseOutcome {
	inputs, initial := validateLiveGoEnvironmentInputs(ctx, physicalRoot, goroot)
	if initial != nil {
		return authorityUseOutcome{primary: initial}
	}
	if revalidator == nil || !revalidator.primitives.valid() {
		return authorityUseOutcome{
			primary: authorityFailure(OperationValidate, CauseInternalInvariant),
		}
	}
	if !claim.validFor(goroot) {
		return authorityUseOutcome{
			primary: authorityFailure(OperationValidate, CauseInternalInvariant),
		}
	}
	return revalidator.useLiveGoEnvironment(
		ctx,
		physicalRoot,
		goroot,
		inputs,
		authorityExpectedPresentOpen,
	)
}

func validateLiveGoEnvironmentInputs(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
) (liveGoEnvironmentInputs, *FailureRecord) {
	if ctx == nil {
		return liveGoEnvironmentInputs{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	directory, err := validatePhysicalRootAuthorityShape(physicalRoot)
	if err != nil || directory.descriptor == nil {
		return liveGoEnvironmentInputs{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	entry, failure := requiredGoEnvironmentEntry(goroot)
	if failure != nil {
		return liveGoEnvironmentInputs{}, failure
	}
	components, err := absolutePathComponents(goroot.root.path)
	if err != nil || len(goroot.root.pathClaims) != len(components)+1 ||
		goroot.root.pathClaims[0] != physicalRoot.claim ||
		goroot.root.pathClaims[len(goroot.root.pathClaims)-1] != makeAuthorityPathClaim(
			goroot.root.snapshot,
			goroot.root.mount,
			goroot.root.aclDigest,
		) {
		return liveGoEnvironmentInputs{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	maximumInt := int64(int(^uint(0) >> 1))
	if entry.file.size < 0 || entry.file.size > maximumInt ||
		uint64(entry.file.size) > goroot.policy.limits.maxFileBytes {
		return liveGoEnvironmentInputs{}, authorityFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	return liveGoEnvironmentInputs{
		physicalRootDirectory:  directory,
		physicalRootHandle:     directory.root,
		physicalRootDescriptor: directory.descriptor,
		gorootDirectory:        goroot.root,
		gorootRootHandle:       goroot.root.root,
		gorootRootDescriptor:   goroot.root.descriptor,
		gorootPath:             goroot.root.path,
		components:             append([]string(nil), components...),
		pathClaims:             append([]authorityPathClaim(nil), goroot.root.pathClaims...),
		entry:                  entry,
		gorootDigest:           goroot.digest,
		gorootMount:            goroot.root.mount,
		gorootDevice:           goroot.root.snapshot.identity.Device,
		owners:                 goroot.policy.owners,
		maxFileBytes:           goroot.policy.limits.maxFileBytes,
	}, nil
}

func authorityContextPrimitiveFailure(
	ctx context.Context,
	operation Operation,
) *authorityPrimitiveFailure {
	if ctx == nil {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if err := ctx.Err(); err != nil {
		cause := contextCause(err)
		if cause == CauseInternalInvariant {
			return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		return newAuthorityPrimitiveFailure(operation, cause)
	}
	return nil
}

func (revalidator *preflightAuthorityRevalidator) useLiveGoEnvironment(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
	inputs liveGoEnvironmentInputs,
	leafOpenMode authorityRequiredOpenMode,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	initialPath, pathOutcome := revalidator.resolveGOROOTPath(ctx, inputs)
	outcome.absorb(pathOutcome)
	if initialPath != nil {
		revalidator.useFreshGoEnvironmentLeaf(
			ctx,
			initialPath,
			goroot,
			inputs,
			leafOpenMode,
			&outcome,
		)
	}

	// This bracket is unconditional once retained input shape has proved. In
	// particular, a leaf failure or DescriptorClose does not suppress it.
	finalPath, finalOutcome := revalidator.resolveGOROOTPath(ctx, inputs)
	outcome.absorb(finalOutcome)
	if finalPath != nil {
		finalPath.closeInto(&outcome)
	}
	outcome.addPrimitive(compareRetainedLiveGoEnvironmentInputs(
		ctx,
		physicalRoot,
		goroot,
		inputs,
	))
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) resolveGOROOTPath(
	ctx context.Context,
	inputs liveGoEnvironmentInputs,
) (*resolvedGOROOTPath, authorityUseOutcome) {
	var outcome authorityUseOutcome
	current := inputs.physicalRootDescriptor
	var currentOwner *ownedAuthorityDescriptor
	for index, expected := range inputs.pathClaims {
		observation, failure := revalidator.observeDescriptor(ctx, current)
		if failure == nil {
			failure = compareAuthorityPathObservation(ctx, expected, observation)
		}
		if failure != nil {
			outcome.addPrimitive(failure)
			if currentOwner != nil {
				currentOwner.closeInto(&outcome)
			}
			return nil, outcome
		}
		if index == len(inputs.components) {
			return &resolvedGOROOTPath{
				descriptor: current,
				owner:      currentOwner,
			}, outcome
		}

		acquisition, openFailure := revalidator.primitives.openRelativeNoFollow(
			ctx,
			current,
			inputs.components[index],
			authorityExpectedPresentOpen,
		)
		nextOwner, acquisitionFailure := ownAuthorityAcquisition(
			acquisition,
			openFailure,
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
	outcome.addPrimitive(newAuthorityPrimitiveFailure(
		OperationValidate,
		CauseInternalInvariant,
	))
	if currentOwner != nil {
		currentOwner.closeInto(&outcome)
	}
	return nil, outcome
}

func (revalidator *preflightAuthorityRevalidator) observeDescriptor(
	ctx context.Context,
	descriptor *os.File,
) (authorityDescriptorObservation, *authorityPrimitiveFailure) {
	snapshot, failure := revalidator.primitives.statDescriptor(ctx, descriptor)
	if failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	mount, failure := revalidator.primitives.statFilesystem(ctx, descriptor)
	if failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	if failure := revalidator.primitives.validateFilesystem(ctx, mount); failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	snapshot.identity.Filesystem = mount.filesystem
	rawACL, failure := revalidator.primitives.acquireRawACL(ctx, descriptor)
	if failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	aclDigest, failure := revalidator.primitives.parseRawACL(ctx, rawACL)
	if failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	return authorityDescriptorObservation{
		snapshot:  snapshot,
		mount:     mount,
		aclDigest: aclDigest,
	}, nil
}

func compareAuthorityPathObservation(
	ctx context.Context,
	expected authorityPathClaim,
	observation authorityDescriptorObservation,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	current := makeAuthorityPathClaim(
		observation.snapshot,
		observation.mount,
		observation.aclDigest,
	)
	if !sameFilesystemObject(expected.identity, current.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if current != expected {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func (revalidator *preflightAuthorityRevalidator) useFreshGoEnvironmentLeaf(
	ctx context.Context,
	path *resolvedGOROOTPath,
	goroot *treeCapture,
	inputs liveGoEnvironmentInputs,
	openMode authorityRequiredOpenMode,
	outcome *authorityUseOutcome,
) {
	acquisition, openFailure := revalidator.primitives.openRelativeNoFollow(
		ctx,
		path.descriptor,
		goEnvironmentFileName,
		openMode,
	)
	leafOwner, acquisitionFailure := ownAuthorityAcquisition(
		acquisition,
		openFailure,
	)
	outcome.addPrimitive(acquisitionFailure)
	path.closeInto(outcome)
	if acquisitionFailure != nil || leafOwner == nil {
		if leafOwner != nil {
			leafOwner.closeInto(outcome)
		}
		return
	}
	revalidator.validateFreshGoEnvironmentLeaf(ctx, leafOwner.file, goroot, inputs, outcome)
	leafOwner.closeInto(outcome)
}

func (revalidator *preflightAuthorityRevalidator) validateFreshGoEnvironmentLeaf(
	ctx context.Context,
	descriptor *os.File,
	goroot *treeCapture,
	inputs liveGoEnvironmentInputs,
	outcome *authorityUseOutcome,
) {
	before, failure := revalidator.observeDescriptor(ctx, descriptor)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	failure = validateLiveGoEnvironmentObservation(ctx, inputs, before)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}

	content := make([]byte, int(inputs.entry.file.size))
	failure = revalidator.primitives.readExactForParse(ctx, descriptor, content)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	failure = parseLiveGoEnvironmentForAuthority(ctx, content)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}

	digest, failure := revalidator.primitives.hashBytes(ctx, content)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	after, failure := revalidator.observeDescriptor(ctx, descriptor)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	failure = compareLiveGoEnvironmentResult(
		ctx,
		goroot,
		inputs,
		before,
		after,
		digest,
	)
	outcome.addPrimitive(failure)
}

func validateLiveGoEnvironmentObservation(
	ctx context.Context,
	inputs liveGoEnvironmentInputs,
	observation authorityDescriptorObservation,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	if observation.snapshot.size < 0 ||
		uint64(observation.snapshot.size) > inputs.maxFileBytes {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseLimit)
	}
	if err := validateProtected(observation.snapshot, inputs.owners); err != nil {
		return authorityPrimitiveFailureFromError(
			OperationValidate,
			err,
			CauseUnsupported,
		)
	}
	if observation.mount != inputs.gorootMount ||
		observation.snapshot.identity.Device != inputs.gorootDevice {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	if snapshotKind(observation.snapshot) != inputs.entry.kind {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseIdentity)
	}
	return nil
}

func compareLiveGoEnvironmentResult(
	ctx context.Context,
	goroot *treeCapture,
	inputs liveGoEnvironmentInputs,
	before authorityDescriptorObservation,
	after authorityDescriptorObservation,
	digest Digest,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	currentEntry, record := requiredGoEnvironmentEntry(goroot)
	if record != nil || goroot.digest != inputs.gorootDigest || currentEntry != inputs.entry {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if !sameFilesystemObject(inputs.entry.file.identity, after.snapshot.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if before != after || after.snapshot != inputs.entry.file ||
		after.mount != inputs.gorootMount ||
		after.aclDigest != inputs.entry.aclDigest || digest != inputs.entry.content {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareRetainedLiveGoEnvironmentInputs(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
	inputs liveGoEnvironmentInputs,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !retainedPhysicalRootSealMatches(physicalRoot, inputs) ||
		!retainedGOROOTSealMatches(goroot, inputs) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	currentEntry, record := requiredGoEnvironmentEntry(goroot)
	if record != nil || currentEntry != inputs.entry {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func retainedPhysicalRootSealMatches(
	physicalRoot *physicalRootAuthority,
	inputs liveGoEnvironmentInputs,
) bool {
	directory, err := validatePhysicalRootAuthorityShape(physicalRoot)
	if err != nil {
		return false
	}
	current := struct {
		directory  *retainedDirectory
		root       *os.Root
		descriptor *os.File
		claim      authorityPathClaim
	}{
		directory:  directory,
		root:       directory.root,
		descriptor: directory.descriptor,
		claim:      physicalRoot.claim,
	}
	sealed := struct {
		directory  *retainedDirectory
		root       *os.Root
		descriptor *os.File
		claim      authorityPathClaim
	}{
		directory:  inputs.physicalRootDirectory,
		root:       inputs.physicalRootHandle,
		descriptor: inputs.physicalRootDescriptor,
		claim:      inputs.pathClaims[0],
	}
	return current == sealed
}

func retainedGOROOTSealMatches(
	goroot *treeCapture,
	inputs liveGoEnvironmentInputs,
) bool {
	if goroot == nil || goroot.root == nil {
		return false
	}
	current := struct {
		directory    *retainedDirectory
		root         *os.Root
		descriptor   *os.File
		path         string
		mount        mountSnapshot
		device       uint64
		terminal     authorityPathClaim
		owners       ownerPolicy
		maxFileBytes uint64
		digest       Digest
	}{
		directory:  goroot.root,
		root:       goroot.root.root,
		descriptor: goroot.root.descriptor,
		path:       goroot.root.path,
		mount:      goroot.root.mount,
		device:     goroot.root.snapshot.identity.Device,
		terminal: makeAuthorityPathClaim(
			goroot.root.snapshot,
			goroot.root.mount,
			goroot.root.aclDigest,
		),
		owners:       goroot.policy.owners,
		maxFileBytes: goroot.policy.limits.maxFileBytes,
		digest:       goroot.digest,
	}
	sealed := struct {
		directory    *retainedDirectory
		root         *os.Root
		descriptor   *os.File
		path         string
		mount        mountSnapshot
		device       uint64
		terminal     authorityPathClaim
		owners       ownerPolicy
		maxFileBytes uint64
		digest       Digest
	}{
		directory:    inputs.gorootDirectory,
		root:         inputs.gorootRootHandle,
		descriptor:   inputs.gorootRootDescriptor,
		path:         inputs.gorootPath,
		mount:        inputs.gorootMount,
		device:       inputs.gorootDevice,
		terminal:     inputs.pathClaims[len(inputs.pathClaims)-1],
		owners:       inputs.owners,
		maxFileBytes: inputs.maxFileBytes,
		digest:       inputs.gorootDigest,
	}
	return current == sealed &&
		equalAuthorityPathClaimSlices(goroot.root.pathClaims, inputs.pathClaims)
}

func equalAuthorityPathClaimSlices(left, right []authorityPathClaim) bool {
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

func authorityPrimitiveFailureFromError(
	operation Operation,
	err error,
	fallback CauseCode,
) *authorityPrimitiveFailure {
	if err == nil {
		return nil
	}
	causes := privateCauses(err, fallback)
	if len(causes) != 1 {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if causes[0] == CauseInternalInvariant {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	return newAuthorityPrimitiveFailure(operation, causes[0])
}

func goEnvironmentGrammarFailure(err error) *authorityPrimitiveFailure {
	if err == nil {
		return nil
	}
	causes := privateCauses(err, CauseInternalInvariant)
	if len(causes) != 1 {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if causes[0] == CauseMalformed {
		return newAuthorityPrimitiveFailure(OperationParse, CauseMalformed)
	}
	if causes[0] == CauseUnsupported {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnsupported)
	}
	return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
}

func parseLiveGoEnvironmentForAuthority(
	ctx context.Context,
	content []byte,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	if failure := goEnvironmentGrammarFailure(parseGoEnvironmentFile(content)); failure != nil {
		return failure
	}
	return authorityContextPrimitiveFailure(ctx, OperationParse)
}

func readExactAuthorityForParse(
	ctx context.Context,
	reader io.ReaderAt,
	content []byte,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	if reader == nil {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	read, err := reader.ReadAt(content, 0)
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	if read == len(content) && (err == nil || errors.Is(err, io.EOF)) {
		return nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return newAuthorityPrimitiveFailure(OperationParse, CausePermission)
	}
	return newAuthorityPrimitiveFailure(OperationParse, CauseUnstable)
}

func hashAuthorityBytes(
	ctx context.Context,
	content []byte,
) (Digest, *authorityPrimitiveFailure) {
	if failure := authorityContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	digest := Digest(sha256.Sum256(content))
	if failure := authorityContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	return digest, nil
}
