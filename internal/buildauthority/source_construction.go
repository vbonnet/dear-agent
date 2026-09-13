package buildauthority

import (
	"context"
	"sync"
)

type sourceDescriptorEvidence struct {
	snapshot  fileSnapshot
	mount     mountSnapshot
	aclDigest Digest
}

type retainedSourceRoot struct {
	root       *ownedSourceRoot
	descriptor *ownedSourceDescriptor
	evidence   sourceDescriptorEvidence
}

func (retained retainedSourceRoot) validOpenDirectory() bool {
	return retained.root.validOpen() && retained.descriptor.validOpen() &&
		retained.descriptor.kind == sourceObservedDirectory
}

type retainedSourceRepository struct {
	path       string
	root       retainedSourceRoot
	pathClaims []authorityPathClaim
}

type retainedSourceGit struct {
	root           retainedSourceRoot
	administration *sourceAdministrativeInventory
}

type retainedSourceObjects struct {
	root retainedSourceRoot
}

type retainedSourceConfig struct {
	descriptor *ownedSourceDescriptor
	evidence   sourceDescriptorEvidence
	digest     Digest
	claim      sourceConfigClaim
}

type retainedSourcePackedRefs struct {
	descriptor *ownedSourceDescriptor
	evidence   sourceDescriptorEvidence
	digest     Digest
	claim      packedRefsClaim
}

type sourcePackedRefsState uint8

const (
	sourcePackedRefsUnresolved sourcePackedRefsState = iota + 1
	sourcePackedRefsAbsent
	sourcePackedRefsRetained
)

type sourcePackedRefsSlot struct {
	state sourcePackedRefsState
	leaf  *retainedSourcePackedRefs
}

func (slot sourcePackedRefsSlot) valid() bool {
	switch slot.state {
	case sourcePackedRefsUnresolved, sourcePackedRefsAbsent:
		return slot.leaf == nil
	case sourcePackedRefsRetained:
		return slot.leaf != nil && slot.leaf.descriptor != nil
	default:
		return false
	}
}

type sourceConstructionState uint8

const (
	sourceConstructionActive sourceConstructionState = iota + 1
	sourceConstructionClosed
)

// sourceConstructionOwner is deliberately unsealable. It owns the step-2
// source prefix while later no-scratch work proceeds, but exposes no transfer,
// take, move, or pending-cell operation before complete step-6 proof exists.
type sourceConstructionOwner struct {
	mu           sync.Mutex
	state        sourceConstructionState
	closeFailure bool

	packedRefs sourcePackedRefsSlot
	objects    *retainedSourceObjects
	config     *retainedSourceConfig
	git        *retainedSourceGit
	repository *retainedSourceRepository
}

func newSourceConstructionOwner() *sourceConstructionOwner {
	return &sourceConstructionOwner{
		state: sourceConstructionActive,
		packedRefs: sourcePackedRefsSlot{
			state: sourcePackedRefsUnresolved,
		},
	}
}

func (owner *sourceConstructionOwner) validInitialRetention() bool {
	if owner == nil || owner.state != sourceConstructionActive || owner.closeFailure {
		return false
	}
	if owner.packedRefs.state != sourcePackedRefsUnresolved || owner.packedRefs.leaf != nil {
		return false
	}
	return owner.validInitialRepository() && owner.validInitialGit() &&
		owner.validInitialConfig() && owner.validInitialObjects()
}

func (owner *sourceConstructionOwner) validInitialRepository() bool {
	if owner.repository == nil || owner.repository.path == "" ||
		!owner.repository.root.validOpenDirectory() ||
		len(owner.repository.pathClaims) == 0 {
		return false
	}
	last := owner.repository.pathClaims[len(owner.repository.pathClaims)-1]
	return last == owner.repository.root.evidence.pathClaim()
}

func (owner *sourceConstructionOwner) validInitialGit() bool {
	return owner.git != nil && owner.git.root.validOpenDirectory() &&
		owner.git.administration == nil
}

func (owner *sourceConstructionOwner) validInitialConfig() bool {
	return owner.config != nil && owner.config.descriptor.validOpen() &&
		owner.config.descriptor.kind == sourceObservedRegular &&
		owner.config.claim.objectFormat.hexWidth() != 0
}

func (owner *sourceConstructionOwner) validInitialObjects() bool {
	return owner.objects != nil && owner.objects.root.validOpenDirectory()
}

func (owner *sourceConstructionOwner) validPackedRefsRetention() bool {
	if owner == nil || owner.state != sourceConstructionActive || owner.closeFailure ||
		!owner.validInitialRepository() || !owner.validInitialGit() ||
		!owner.validInitialConfig() || !owner.validInitialObjects() {
		return false
	}
	switch owner.packedRefs.state {
	case sourcePackedRefsUnresolved:
		return false
	case sourcePackedRefsAbsent:
		return owner.packedRefs.leaf == nil
	case sourcePackedRefsRetained:
		return owner.packedRefs.leaf != nil &&
			owner.packedRefs.leaf.descriptor.validOpen() &&
			owner.packedRefs.leaf.descriptor.kind == sourceObservedRegular &&
			owner.packedRefs.leaf.evidence.snapshot.size >= 0 &&
			owner.packedRefs.leaf.evidence.snapshot.size <= maxPackedRefsBytes
	default:
		return false
	}
}

func (owner *sourceConstructionOwner) validAdministrativeRetention() bool {
	if owner == nil || owner.state != sourceConstructionActive || owner.closeFailure ||
		!owner.validInitialRepository() || owner.git == nil ||
		!owner.git.root.validOpenDirectory() || owner.git.administration == nil ||
		!owner.validInitialConfig() || !owner.validInitialObjects() ||
		!owner.validResolvedPackedRefs() {
		return false
	}
	return owner.git.administration.valid(
		owner.config.claim.objectFormat,
		owner.packedRefs,
		owner.git.root.evidence,
		owner.config.evidence,
		owner.objects.root.evidence,
	)
}

func (owner *sourceConstructionOwner) validResolvedPackedRefs() bool {
	if owner == nil {
		return false
	}
	switch owner.packedRefs.state {
	case sourcePackedRefsUnresolved:
		return false
	case sourcePackedRefsAbsent:
		return owner.packedRefs.leaf == nil
	case sourcePackedRefsRetained:
		return owner.packedRefs.leaf != nil &&
			owner.packedRefs.leaf.descriptor.validOpen() &&
			owner.packedRefs.leaf.descriptor.kind == sourceObservedRegular &&
			owner.packedRefs.leaf.evidence.snapshot.size >= 0 &&
			owner.packedRefs.leaf.evidence.snapshot.size <= maxPackedRefsBytes
	default:
		return false
	}
}

func (owner *sourceConstructionOwner) closeInto(outcome *sourceUseOutcome) {
	primitives, failure := platformSourcePrimitives()
	if failure != nil {
		if outcome != nil {
			outcome.addPrimitive(failure)
		}
		owner.closeDirectInto(outcome)
		return
	}
	owner.closeIntoWith(primitives, outcome)
}

func (owner *sourceConstructionOwner) closeIntoWith(
	primitives sourcePrimitives,
	outcome *sourceUseOutcome,
) {
	if owner == nil {
		if outcome != nil {
			outcome.addPrimitive(newSourcePrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			))
		}
		return
	}
	if primitives == nil {
		if outcome != nil {
			outcome.addPrimitive(newSourcePrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			))
		}
		owner.closeDirectInto(outcome)
		return
	}

	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.closeLocked(primitives, outcome)
}

func (owner *sourceConstructionOwner) closeDirectInto(outcome *sourceUseOutcome) {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.closeLocked(directSourceHandleCloser{}, outcome)
}

type sourceHandleCloser interface {
	closeRoot(*ownedSourceRoot) bool
	closeDescriptor(*ownedSourceDescriptor) bool
}

type directSourceHandleCloser struct{}

func (directSourceHandleCloser) closeRoot(owner *ownedSourceRoot) bool {
	return owner.closeDirect()
}

func (directSourceHandleCloser) closeDescriptor(owner *ownedSourceDescriptor) bool {
	return owner.closeDirect()
}

func (owner *sourceConstructionOwner) closeLocked(
	closer sourceHandleCloser,
	outcome *sourceUseOutcome,
) {
	if owner.state == sourceConstructionClosed {
		if owner.closeFailure && outcome != nil {
			outcome.addDescriptorClose()
		}
		return
	}
	if !owner.validForClose() && outcome != nil {
		outcome.addPrimitive(newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		))
	}

	tracker := sourceCloseTracker{closer: closer}
	tracker.closePackedRefs(owner.packedRefs.leaf)
	tracker.closeObjects(owner.objects)
	tracker.closeConfig(owner.config)
	tracker.closeGit(owner.git)
	tracker.closeRepository(owner.repository)

	owner.state = sourceConstructionClosed
	owner.closeFailure = tracker.failed
	if tracker.failed && outcome != nil {
		outcome.addDescriptorClose()
	}
}

func (owner *sourceConstructionOwner) validForClose() bool {
	return owner.state == sourceConstructionActive && owner.packedRefs.valid()
}

type sourceCloseTracker struct {
	closer sourceHandleCloser
	failed bool
}

func (tracker *sourceCloseTracker) closePackedRefs(retained *retainedSourcePackedRefs) {
	if retained != nil {
		tracker.closeDescriptor(&retained.descriptor)
	}
}

func (tracker *sourceCloseTracker) closeObjects(retained *retainedSourceObjects) {
	if retained != nil {
		tracker.closeRootBundle(&retained.root)
	}
}

func (tracker *sourceCloseTracker) closeConfig(retained *retainedSourceConfig) {
	if retained != nil {
		tracker.closeDescriptor(&retained.descriptor)
	}
}

func (tracker *sourceCloseTracker) closeGit(retained *retainedSourceGit) {
	if retained != nil {
		tracker.closeRootBundle(&retained.root)
	}
}

func (tracker *sourceCloseTracker) closeRepository(retained *retainedSourceRepository) {
	if retained != nil {
		tracker.closeRootBundle(&retained.root)
	}
}

func (tracker *sourceCloseTracker) closeRootBundle(retained *retainedSourceRoot) {
	if retained == nil {
		return
	}
	tracker.closeRoot(&retained.root)
	tracker.closeDescriptor(&retained.descriptor)
}

func (tracker *sourceCloseTracker) closeRoot(owner **ownedSourceRoot) {
	if owner == nil || *owner == nil {
		return
	}
	if tracker.closer.closeRoot(*owner) {
		tracker.failed = true
	}
	*owner = nil
}

func (tracker *sourceCloseTracker) closeDescriptor(owner **ownedSourceDescriptor) {
	if owner == nil || *owner == nil {
		return
	}
	if tracker.closer.closeDescriptor(*owner) {
		tracker.failed = true
	}
	*owner = nil
}

type sourceConstructionBuilder struct {
	ctx        context.Context
	primitives sourcePrimitives
	owner      *sourceConstructionOwner
	outcome    sourceUseOutcome
}
