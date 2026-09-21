package buildauthority

import (
	"bytes"
	"context"
	"sort"
)

type sourceEnvelopeBindingPhase uint8

const (
	sourceEnvelopePreReadBinding sourceEnvelopeBindingPhase = iota + 1
	sourceEnvelopePostReadBinding
)

func validateSourceObjectClaimOwner(
	ctx context.Context,
	owner *sourceConstructionOwner,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if owner == nil || !owner.validObjectClaimRetention() {
		validationFailure = newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

//nolint:gocyclo // Keep the ordered binding, scan, byte, rescan, and evidence gates auditable as one transaction.
func (builder *sourceConstructionBuilder) revalidateSourceObjectAuxiliaryClaimInventory() bool {
	if builder == nil {
		return false
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationCompare); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if builder.owner == nil || builder.primitives == nil {
		builder.failInvariant()
		return false
	}
	ownerValid := builder.owner.validObjectClaimRetention()
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationCompare); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !ownerValid {
		builder.failInvariant()
		return false
	}
	if failure := builder.revalidateSourceEnvelopePathBindings(
		sourceEnvelopePreReadBinding,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !builder.outcome.proved() {
		return false
	}
	administration, captured := builder.captureRevalidatedSourceAdministrativeInventory()
	if !captured {
		return false
	}
	paths, failure := deriveSourceObjectPathInventory(
		builder.ctx,
		builder.owner.config.claim.objectFormat,
		administration,
	)
	if failure == nil {
		failure = compareRevalidatedSourceObjectPathInventories(
			builder.ctx,
			builder.owner.objects.paths,
			paths,
		)
	}
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	claims, captured := builder.captureSourceObjectAuxiliaryClaimInventory(
		administration,
		paths,
		sourceRevalidatePresent,
	)
	if !captured {
		return false
	}
	if failure = compareRevalidatedSourceObjectAuxiliaryClaimInventories(
		builder.ctx,
		builder.owner.config.claim.objectFormat,
		paths,
		builder.owner.objects.claims,
		claims,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	postAdministration, captured := builder.captureRevalidatedSourceAdministrativeInventory()
	if !captured {
		return false
	}
	if failure = builder.revalidateSourceEnvelopePathBindings(
		sourceEnvelopePostReadBinding,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !builder.outcome.proved() {
		return false
	}
	if failure = compareRevalidatedDeferredSourceDescriptorEvidence(
		builder.ctx,
		builder.owner.config.claim.objectFormat,
		builder.owner.git.administration,
		postAdministration,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	return builder.outcome.proved()
}

// revalidateSourceEnvelopePathBindings freshly resolves the retained physical
// Repository path and its .git/objects chain after every auxiliary byte and
// post-read tail. Stable identity/security is checked for the complete chain
// before volatile directory metadata can claim failure precedence.
//
//nolint:gocyclo // One close-owned physical-to-object binding transaction preserves first failure.
func (builder *sourceConstructionBuilder) revalidateSourceEnvelopePathBindings(
	phase sourceEnvelopeBindingPhase,
) *sourcePrimitiveFailure {
	if builder == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationCompare); failure != nil {
		return failure
	}
	if builder.primitives == nil || builder.owner == nil ||
		builder.owner.repository == nil || builder.owner.git == nil ||
		builder.owner.objects == nil ||
		(phase != sourceEnvelopePreReadBinding && phase != sourceEnvelopePostReadBinding) {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	components, err := absolutePathComponents(builder.owner.repository.path)
	if err != nil || len(components) == 0 ||
		len(builder.owner.repository.pathClaims) != len(components)+1 {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}

	transient := make([]*ownedSourceDescriptor, 0, len(components)+3)
	var primary *sourcePrimitiveFailure
	physical, failure := builder.primitives.openPhysicalRootDescriptor(builder.ctx)
	if physical != nil {
		transient = append(transient, physical)
	}
	primary = firstSourceObjectAuxiliaryFailure(primary, failure)
	if primary == nil && (physical == nil || !physical.validOpen()) {
		primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	var repositoryEvidence sourceDescriptorEvidence
	parent := physical
	if primary == nil {
		observation, observationFailure := builder.observeSourceDescriptorWithoutPolicy(
			physical,
			sourceObservedDirectory,
		)
		primary = firstSourceObjectAuxiliaryFailure(primary, observationFailure)
		if primary == nil {
			primary = compareRevalidatedSourceAuthorityPathClaim(
				builder.ctx,
				builder.owner.repository.pathClaims[0],
				observation.evidence.pathClaim(),
			)
		}
	}
	var repositoryDescriptor *ownedSourceDescriptor
	for index, component := range components {
		if primary != nil {
			break
		}
		descriptor, openFailure := builder.primitives.openRelativeNoFollow(
			builder.ctx,
			parent,
			component,
			sourceObservedDirectory,
			sourceRevalidatePresent,
		)
		if descriptor != nil {
			transient = append(transient, descriptor)
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, openFailure)
		if primary == nil && (descriptor == nil || !descriptor.validOpen()) {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if primary != nil {
			break
		}
		observation, observationFailure := builder.observeSourceDescriptorWithoutPolicy(
			descriptor,
			sourceObservedDirectory,
		)
		primary = firstSourceObjectAuxiliaryFailure(primary, observationFailure)
		if primary == nil {
			primary = compareRevalidatedSourceAuthorityPathClaim(
				builder.ctx,
				builder.owner.repository.pathClaims[index+1],
				observation.evidence.pathClaim(),
			)
		}
		if primary == nil {
			parent = descriptor
			if index == len(components)-1 {
				repositoryDescriptor = descriptor
				repositoryEvidence = observation.evidence
			}
		}
	}
	if primary == nil {
		primary = builder.primitives.compareRootAndDescriptor(
			builder.ctx,
			builder.owner.repository.root.root,
			repositoryDescriptor,
		)
	}

	var gitDescriptor *ownedSourceDescriptor
	var gitEvidence sourceDescriptorEvidence
	if primary == nil {
		gitDescriptor, failure = builder.primitives.openRelativeNoFollow(
			builder.ctx,
			repositoryDescriptor,
			".git",
			sourceObservedDirectory,
			sourceRevalidatePresent,
		)
		if gitDescriptor != nil {
			transient = append(transient, gitDescriptor)
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, failure)
		if primary == nil && (gitDescriptor == nil || !gitDescriptor.validOpen()) {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
	}
	if primary == nil {
		observation, observationFailure := builder.observeSourceDescriptorWithoutPolicy(
			gitDescriptor,
			sourceObservedDirectory,
		)
		primary = firstSourceObjectAuxiliaryFailure(primary, observationFailure)
		if primary == nil {
			gitEvidence = observation.evidence
			primary = compareSourceDirectoryPreWalkEvidence(
				builder.ctx,
				builder.owner.git.root.evidence,
				gitEvidence,
			)
		}
	}
	if primary == nil {
		primary = builder.primitives.compareRootAndDescriptor(
			builder.ctx,
			builder.owner.git.root.root,
			gitDescriptor,
		)
	}

	var objectsDescriptor *ownedSourceDescriptor
	var objectsEvidence sourceDescriptorEvidence
	if primary == nil {
		objectsDescriptor, failure = builder.primitives.openRelativeNoFollow(
			builder.ctx,
			gitDescriptor,
			"objects",
			sourceObservedDirectory,
			sourceRevalidatePresent,
		)
		if objectsDescriptor != nil {
			transient = append(transient, objectsDescriptor)
		}
		primary = firstSourceObjectAuxiliaryFailure(primary, failure)
		if primary == nil && (objectsDescriptor == nil || !objectsDescriptor.validOpen()) {
			primary = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
	}
	if primary == nil {
		observation, observationFailure := builder.observeSourceDescriptorWithoutPolicy(
			objectsDescriptor,
			sourceObservedDirectory,
		)
		primary = firstSourceObjectAuxiliaryFailure(primary, observationFailure)
		if primary == nil {
			objectsEvidence = observation.evidence
			primary = compareSourceDirectoryPreWalkEvidence(
				builder.ctx,
				builder.owner.objects.root.evidence,
				objectsEvidence,
			)
		}
	}
	if primary == nil {
		primary = builder.primitives.compareRootAndDescriptor(
			builder.ctx,
			builder.owner.objects.root.root,
			objectsDescriptor,
		)
	}
	if primary == nil && phase == sourceEnvelopePostReadBinding {
		primary = compareSourceDescriptorEvidence(
			builder.ctx,
			builder.owner.repository.root.evidence,
			repositoryEvidence,
		)
	}
	if primary == nil && phase == sourceEnvelopePostReadBinding {
		primary = compareSourceDescriptorEvidence(
			builder.ctx,
			builder.owner.git.root.evidence,
			gitEvidence,
		)
	}
	if primary == nil && phase == sourceEnvelopePostReadBinding {
		primary = compareSourceDescriptorEvidence(
			builder.ctx,
			builder.owner.objects.root.evidence,
			objectsEvidence,
		)
	}
	builder.closeTransientDescriptors(transient)
	return primary
}

func compareRevalidatedSourceAuthorityPathClaim(
	ctx context.Context,
	before authorityPathClaim,
	after authorityPathClaim,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	var comparisonFailure *sourcePrimitiveFailure
	if !sameFilesystemObject(before.identity, after.identity) {
		comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
	} else if before != after {
		comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	return comparisonFailure
}

// compareSourceObjectAuxiliaryPreReadEvidence admits only the four timestamps
// that an ordinary same-inode content write necessarily changes. Identity,
// size, ownership, mode, link, mount, ACL, and the remaining stable inode facts
// must match before any revalidation byte is consumed.
func compareSourceObjectAuxiliaryPreReadEvidence(
	ctx context.Context,
	before sourceDescriptorEvidence,
	after sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	var comparisonFailure *sourcePrimitiveFailure
	if !sameFilesystemObject(before.snapshot.identity, after.snapshot.identity) {
		comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
	} else {
		before.snapshot.mtimeSec = 0
		before.snapshot.mtimeNsec = 0
		before.snapshot.ctimeSec = 0
		before.snapshot.ctimeNsec = 0
		after.snapshot.mtimeSec = 0
		after.snapshot.mtimeNsec = 0
		after.snapshot.ctimeSec = 0
		after.snapshot.ctimeNsec = 0
		if before != after {
			comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	return comparisonFailure
}

// compareSourceDirectoryPreWalkEvidence ignores only the directory churn
// fields that ordinary child replacement necessarily changes. The complete
// sealed evidence is compared after byte and closure attribution succeeds.
func compareSourceDirectoryPreWalkEvidence(
	ctx context.Context,
	before sourceDescriptorEvidence,
	after sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	var comparisonFailure *sourcePrimitiveFailure
	if !sameFilesystemObject(before.snapshot.identity, after.snapshot.identity) {
		comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
	} else {
		before.snapshot.linkCount = 0
		before.snapshot.size = 0
		before.snapshot.mtimeSec = 0
		before.snapshot.mtimeNsec = 0
		before.snapshot.ctimeSec = 0
		before.snapshot.ctimeNsec = 0
		after.snapshot.linkCount = 0
		after.snapshot.size = 0
		after.snapshot.mtimeSec = 0
		after.snapshot.mtimeNsec = 0
		after.snapshot.ctimeSec = 0
		after.snapshot.ctimeNsec = 0
		if before != after {
			comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	return comparisonFailure
}

// compareSourceInertRegularEvidence retains identity and security only. Inert
// contents are deliberately unread, so size and content timestamps may change
// without becoming source authority.
func compareSourceInertRegularEvidence(
	ctx context.Context,
	before sourceDescriptorEvidence,
	after sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	var comparisonFailure *sourcePrimitiveFailure
	if !sameFilesystemObject(before.snapshot.identity, after.snapshot.identity) {
		comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
	} else {
		before.snapshot.size = 0
		before.snapshot.mtimeSec = 0
		before.snapshot.mtimeNsec = 0
		before.snapshot.ctimeSec = 0
		before.snapshot.ctimeNsec = 0
		after.snapshot.size = 0
		after.snapshot.mtimeSec = 0
		after.snapshot.mtimeNsec = 0
		after.snapshot.ctimeSec = 0
		after.snapshot.ctimeNsec = 0
		if before != after {
			comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	return comparisonFailure
}

func (builder *sourceConstructionBuilder) compareSourceDirectoryBeforeWalk(
	descriptor *ownedSourceDescriptor,
	expected sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	observation, failure := builder.observeSourceDescriptorWithoutPolicy(
		descriptor,
		sourceObservedDirectory,
	)
	if failure != nil {
		return failure
	}
	return compareSourceDirectoryPreWalkEvidence(
		builder.ctx,
		expected,
		observation.evidence,
	)
}

func compareRevalidatedSourceAdministrativeEvidence(
	ctx context.Context,
	format repositoryObjectFormat,
	before sourceDescriptorEvidence,
	after sourceDescriptorEvidence,
	sealedRow sourceAdministrativeRow,
) *sourcePrimitiveFailure {
	if sealedRow.kind == sourceObservedDirectory {
		return compareSourceDirectoryPreWalkEvidence(ctx, before, after)
	}
	if sealedRow.kind == sourceObservedRegular && sealedRow.class == sourceInertAdministration {
		return compareSourceInertRegularEvidence(ctx, before, after)
	}
	pathRow, classified := classifySourceObjectPathRow(sealedRow, format)
	if classified && pathRow.role.auxiliary() {
		return compareSourceObjectAuxiliaryPreReadEvidence(ctx, before, after)
	}
	return compareSourceDescriptorEvidence(ctx, before, after)
}

func compareRevalidatedSourceObjectPathInventories(
	ctx context.Context,
	sealed *sourceObjectPathInventory,
	current *sourceObjectPathInventory,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	for _, inventory := range [...]*sourceObjectPathInventory{sealed, current} {
		shapeValid, failure := sourceObjectPathInventoryShapeValidity(
			ctx,
			OperationCompare,
			inventory,
		)
		if failure != nil {
			return failure
		}
		if !shapeValid {
			if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
				return failure
			}
			return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
	}
	if sealed.contentFileCount != current.contentFileCount ||
		sealed.auxiliaryFileCount != current.auxiliaryFileCount ||
		len(sealed.rows) != len(current.rows) {
		if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
			return failure
		}
		return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	for index := range sealed.rows {
		if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
			return failure
		}
		if sealed.rows[index] != current.rows[index] {
			if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
				return failure
			}
			return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
	}
	return sourceContextPrimitiveFailure(ctx, OperationCompare)
}

//nolint:gocyclo // Validation, ordered value comparison, and cancellation precedence stay contiguous.
func compareRevalidatedSourceObjectAuxiliaryClaimInventories(
	ctx context.Context,
	format repositoryObjectFormat,
	paths *sourceObjectPathInventory,
	sealed *sourceObjectAuxiliaryClaimInventory,
	current *sourceObjectAuxiliaryClaimInventory,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	sealedValid, failure := sourceObjectAuxiliaryClaimInventoryValidity(
		ctx,
		OperationCompare,
		format,
		paths,
		sealed,
	)
	if failure != nil {
		return failure
	}
	currentValid, failure := sourceObjectAuxiliaryClaimInventoryValidity(
		ctx,
		OperationCompare,
		format,
		paths,
		current,
	)
	if failure != nil {
		return failure
	}
	if !sealedValid || !currentValid {
		if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
			return failure
		}
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if len(sealed.rows) != len(current.rows) {
		if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
			return failure
		}
		return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	for index := range sealed.rows {
		if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
			return failure
		}
		before := sealed.rows[index]
		after := current.rows[index]
		valuesEqual, failure := equalSourceObjectAuxiliaryValues(
			ctx,
			before.values,
			after.values,
		)
		if failure != nil {
			return failure
		}
		matches := before.path == after.path && before.role == after.role &&
			before.bytes == after.bytes && before.blankRecords == after.blankRecords &&
			valuesEqual
		if !matches {
			if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
				return failure
			}
			return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
	}
	return sourceContextPrimitiveFailure(ctx, OperationCompare)
}

func compareRevalidatedDeferredSourceDescriptorEvidence(
	ctx context.Context,
	format repositoryObjectFormat,
	sealed *sourceAdministrativeInventory,
	current *sourceAdministrativeInventory,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if sealed == nil || current == nil || format.hexWidth() == 0 ||
		len(sealed.rows) != len(current.rows) {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	for index := range sealed.rows {
		if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
			return failure
		}
		before := sealed.rows[index]
		after := current.rows[index]
		if before.path != after.path || before.kind != after.kind || before.class != after.class {
			return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		deferred := before.kind == sourceObservedDirectory
		if !deferred {
			path, classified := classifySourceObjectPathRow(before, format)
			deferred = classified && path.role.auxiliary()
		}
		if !deferred {
			continue
		}
		if failure := compareSourceDescriptorEvidence(
			ctx,
			before.evidence,
			after.evidence,
		); failure != nil {
			return failure
		}
	}
	return sourceContextPrimitiveFailure(ctx, OperationCompare)
}

func equalSourceObjectAuxiliaryValues(
	ctx context.Context,
	left []string,
	right []string,
) (bool, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return false, failure
	}
	if len(left) != len(right) {
		return false, sourceContextPrimitiveFailure(ctx, OperationCompare)
	}
	for index := range left {
		if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
			return false, failure
		}
		if left[index] != right[index] {
			return false, sourceContextPrimitiveFailure(ctx, OperationCompare)
		}
	}
	return true, sourceContextPrimitiveFailure(ctx, OperationCompare)
}

// captureRevalidatedSourceAdministrativeInventory repeats the complete
// descriptor-bound administrative walk without mutating the retained owner.
// Topology and evidence are compared with the sealed rows before any object
// auxiliary body is reopened.
//
//nolint:gocyclo // This is the closed root-open, scan, close, and scratch-finish transaction.
func (builder *sourceConstructionBuilder) captureRevalidatedSourceAdministrativeInventory() (
	*sourceAdministrativeInventory,
	bool,
) {
	if builder == nil {
		return nil, false
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationCompare); failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}
	if builder.primitives == nil || builder.owner == nil || builder.owner.git == nil ||
		builder.owner.objects == nil {
		builder.failInvariant()
		return nil, false
	}
	sealedRoot, present := sourceAdministrativeRowByPath(
		builder.owner.git.administration.rows,
		".git",
	)
	if !present || sealedRoot.kind != sourceObservedDirectory {
		builder.failInvariant()
		return nil, false
	}
	if failure := builder.compareSourceDirectoryBeforeWalk(
		builder.owner.objects.root.descriptor,
		builder.owner.objects.root.evidence,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}

	scan, openFailure := builder.primitives.openRootDirectoryDescriptor(
		builder.ctx,
		builder.owner.git.root.root,
	)
	if scan == nil {
		if openFailure == nil {
			openFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		builder.outcome.addPrimitive(openFailure)
		return nil, false
	}
	if openFailure != nil {
		builder.outcome.addPrimitive(openFailure)
		builder.closeTransientDescriptor(scan)
		return nil, false
	}
	if failure := builder.primitives.compareRootAndDescriptor(
		builder.ctx,
		builder.owner.git.root.root,
		scan,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		builder.closeTransientDescriptor(scan)
		return nil, false
	}

	rootObservation, failure := builder.observeSourceDescriptorWithoutPolicy(
		scan,
		sourceObservedDirectory,
	)
	if failure == nil {
		failure = compareSourceDirectoryPreWalkEvidence(
			builder.ctx,
			sealedRoot.evidence,
			rootObservation.evidence,
		)
	}
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		builder.closeTransientDescriptor(scan)
		return nil, false
	}

	capture := &sourceAdministrativeCapture{
		candidates: []sourceAdministrativeCandidate{{
			row: sourceAdministrativeRow{
				path:     ".git",
				kind:     sourceObservedDirectory,
				evidence: rootObservation.evidence,
			},
			acl: rootObservation.acl,
		}},
	}
	walked := builder.captureSourceAdministrativeDirectory(
		capture,
		scan,
		".git",
		rootObservation,
		sourceRevalidatePresent,
		builder.owner.git.administration,
	)
	closeFailed := builder.closeTransientDescriptor(scan)
	if !walked || closeFailed {
		return nil, false
	}

	inventory, failure := builder.finishRevalidatedSourceAdministrativeInventory(
		capture,
		builder.owner.git.administration,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return nil, false
	}
	return inventory, builder.outcome.proved()
}

//nolint:gocyclo // Classification, fixed-row rebinding, policy, and scratch sealing stay one transaction.
func (builder *sourceConstructionBuilder) finishRevalidatedSourceAdministrativeInventory(
	capture *sourceAdministrativeCapture,
	sealed *sourceAdministrativeInventory,
) (*sourceAdministrativeInventory, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationCompare); failure != nil {
		return nil, failure
	}
	if capture == nil || sealed == nil || len(capture.candidates) == 0 ||
		capture.descendants != uint64(len(capture.candidates)-1) { //nolint:gosec // The shared walk stops at the fixed row bound.
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	sort.Slice(capture.candidates, func(left, right int) bool {
		return bytes.Compare(
			[]byte(capture.candidates[left].row.path),
			[]byte(capture.candidates[right].row.path),
		) < 0
	})
	if failure := classifySourceAdministrativeCandidates(
		builder.ctx,
		capture.candidates,
		builder.owner.config.claim.objectFormat,
	); failure != nil {
		return nil, failure
	}
	if failure := compareRevalidatedSourceAdministrativeRows(
		builder.ctx,
		builder.owner.config.claim.objectFormat,
		sealed,
		capture,
	); failure != nil {
		return nil, failure
	}
	for index := range capture.candidates {
		if failure := builder.validateSourceAdministrativeCandidate(
			capture.candidates[index],
		); failure != nil {
			return nil, failure
		}
	}
	rows := make([]sourceAdministrativeRow, len(capture.candidates))
	for index := range capture.candidates {
		rows[index] = capture.candidates[index].row
	}
	inventory := &sourceAdministrativeInventory{
		rows:              rows,
		totalRegularBytes: capture.totalRegularBytes,
	}
	gitRow, gitPresent := sourceAdministrativeRowByPath(rows, ".git")
	objectsRow, objectsPresent := sourceAdministrativeRowByPath(rows, ".git/objects")
	if !gitPresent || !objectsPresent || gitRow.kind != sourceObservedDirectory ||
		objectsRow.kind != sourceObservedDirectory {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if !inventory.valid(
		builder.owner.config.claim.objectFormat,
		builder.owner.packedRefs,
		gitRow.evidence,
		builder.owner.config.evidence,
		objectsRow.evidence,
	) {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationCompare); failure != nil {
		return nil, failure
	}
	return inventory, nil
}

func compareRevalidatedSourceAdministrativeRows(
	ctx context.Context,
	format repositoryObjectFormat,
	sealed *sourceAdministrativeInventory,
	capture *sourceAdministrativeCapture,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if sealed == nil || capture == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if len(sealed.rows) != len(capture.candidates) {
		return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	for index := range sealed.rows {
		before := sealed.rows[index]
		after := capture.candidates[index].row
		if before.path != after.path || before.kind != after.kind ||
			before.class != after.class {
			return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
		if failure := compareRevalidatedSourceAdministrativeEvidence(
			ctx,
			format,
			before.evidence,
			after.evidence,
			before,
		); failure != nil {
			return failure
		}
	}
	return sourceContextPrimitiveFailure(ctx, OperationCompare)
}
