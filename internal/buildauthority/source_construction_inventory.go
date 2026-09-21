package buildauthority

import (
	"bytes"
	"context"
	"sort"
	"strings"
)

// sourceAdministrativeRow is a value-only claim about one classified row in
// the retained .git topology. It deliberately carries neither content nor an
// open handle, so this partial step-6 result cannot authorize a copy or seal.
type sourceAdministrativeRow struct {
	path     string
	kind     sourceObservedKind
	class    sourceAdministrativeClass
	evidence sourceDescriptorEvidence
}

type sourceAdministrativeInventory struct {
	rows              []sourceAdministrativeRow
	totalRegularBytes uint64
}

type sourceAdministrativeCandidate struct {
	row sourceAdministrativeRow
	acl parsedSourceACL
}

type sourceAdministrativeCapture struct {
	candidates        []sourceAdministrativeCandidate
	descendants       uint64
	totalRegularBytes uint64
}

type sourceAdministrativeFixedRows struct {
	git        *sourceAdministrativeRow
	config     *sourceAdministrativeRow
	objects    *sourceAdministrativeRow
	packedRefs *sourceAdministrativeRow
}

func (inventory *sourceAdministrativeInventory) valid(
	format repositoryObjectFormat,
	packedRefs sourcePackedRefsSlot,
	gitEvidence sourceDescriptorEvidence,
	configEvidence sourceDescriptorEvidence,
	objectsEvidence sourceDescriptorEvidence,
) bool {
	if inventory == nil || format.hexWidth() == 0 ||
		!validSourceAdministrativeInventoryRowCount(len(inventory.rows)) {
		return false
	}
	total, fixed, rowsValid := inventory.validateRows(format)
	return rowsValid && total == inventory.totalRegularBytes && fixed.valid(
		packedRefs,
		gitEvidence,
		configEvidence,
		objectsEvidence,
	)
}

func validSourceAdministrativeInventoryRowCount(rowCount int) bool {
	return rowCount >= 3 && rowCount <= maxRepositoryEntries+1
}

func (inventory *sourceAdministrativeInventory) validateRows(
	format repositoryObjectFormat,
) (uint64, sourceAdministrativeFixedRows, bool) {
	var total uint64
	var fixed sourceAdministrativeFixedRows
	for index := range inventory.rows {
		row := &inventory.rows[index]
		if index > 0 && bytes.Compare(
			[]byte(inventory.rows[index-1].path),
			[]byte(row.path),
		) >= 0 {
			return 0, sourceAdministrativeFixedRows{}, false
		}
		if !validSourceAdministrativeRow(*row, format) {
			return 0, sourceAdministrativeFixedRows{}, false
		}
		if row.kind == sourceObservedRegular {
			if row.evidence.snapshot.size < 0 {
				return 0, sourceAdministrativeFixedRows{}, false
			}
			size := uint64(row.evidence.snapshot.size)
			if total > maxRepositoryBytes || size > maxRepositoryBytes-total {
				return 0, sourceAdministrativeFixedRows{}, false
			}
			total += size
			if sourceAdministrativePackPath(row.path, format) && size > maxRepositoryFileBytes {
				return 0, sourceAdministrativeFixedRows{}, false
			}
		}
		switch row.path {
		case ".git":
			fixed.git = row
		case ".git/config":
			fixed.config = row
		case ".git/objects":
			fixed.objects = row
		case ".git/packed-refs":
			fixed.packedRefs = row
		}
	}
	return total, fixed, true
}

func validSourceAdministrativeRow(
	row sourceAdministrativeRow,
	format repositoryObjectFormat,
) bool {
	return validateRelativeManifestPath(row.path) == nil &&
		(row.kind == sourceObservedDirectory || row.kind == sourceObservedRegular) &&
		row.class != sourceForbidden &&
		row.class == classifySourceAdministrativePath(
			row.path,
			sourceAdministrativeEntryKind(row.kind),
			format,
		) &&
		observedSourceKind(snapshotKind(row.evidence.snapshot)) == row.kind &&
		row.evidence.snapshot.identity.Filesystem == row.evidence.mount.filesystem
}

func (fixed sourceAdministrativeFixedRows) valid(
	packedRefs sourcePackedRefsSlot,
	gitEvidence sourceDescriptorEvidence,
	configEvidence sourceDescriptorEvidence,
	objectsEvidence sourceDescriptorEvidence,
) bool {
	if !fixed.validRequired(gitEvidence, configEvidence, objectsEvidence) {
		return false
	}
	return fixed.validPackedRefs(packedRefs)
}

func (fixed sourceAdministrativeFixedRows) validRequired(
	gitEvidence sourceDescriptorEvidence,
	configEvidence sourceDescriptorEvidence,
	objectsEvidence sourceDescriptorEvidence,
) bool {
	return fixed.git != nil && fixed.config != nil && fixed.objects != nil &&
		fixed.git.kind == sourceObservedDirectory &&
		fixed.config.kind == sourceObservedRegular &&
		fixed.objects.kind == sourceObservedDirectory &&
		fixed.git.evidence == gitEvidence && fixed.config.evidence == configEvidence &&
		fixed.objects.evidence == objectsEvidence
}

func (fixed sourceAdministrativeFixedRows) validPackedRefs(
	packedRefs sourcePackedRefsSlot,
) bool {
	switch packedRefs.state {
	case sourcePackedRefsUnresolved:
		return false
	case sourcePackedRefsAbsent:
		return packedRefs.leaf == nil && fixed.packedRefs == nil
	case sourcePackedRefsRetained:
		return packedRefs.leaf != nil && fixed.packedRefs != nil &&
			fixed.packedRefs.kind == sourceObservedRegular &&
			fixed.packedRefs.evidence == packedRefs.leaf.evidence
	default:
		return false
	}
}

func (builder *sourceConstructionBuilder) retainSourceAdministrativeInventory() bool {
	if failure := builder.validateSourceAdministrativeInventoryRequest(); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure := builder.compareSourceDescriptorWithoutPolicy(
		builder.owner.objects.root.descriptor,
		builder.owner.objects.root.evidence,
		sourceObservedDirectory,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
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
		return false
	}
	if openFailure != nil {
		builder.outcome.addPrimitive(openFailure)
		builder.closeTransientDescriptor(scan)
		return false
	}
	if failure := builder.primitives.compareRootAndDescriptor(
		builder.ctx,
		builder.owner.git.root.root,
		scan,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		builder.closeTransientDescriptor(scan)
		return false
	}

	rootObservation, failure := builder.observeSourceDescriptorWithoutPolicy(
		scan,
		sourceObservedDirectory,
	)
	if failure == nil {
		failure = compareSourceDescriptorEvidence(
			builder.ctx,
			builder.owner.git.root.evidence,
			rootObservation.evidence,
		)
	}
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		builder.closeTransientDescriptor(scan)
		return false
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
		sourceInitialWalkPresent,
		nil,
	)
	closeFailed := builder.closeTransientDescriptor(scan)
	if !walked || closeFailed {
		return false
	}

	inventory, failure := builder.finishSourceAdministrativeInventory(capture)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = validateSourceAdministrativeInventoryOwner(
		builder.ctx,
		builder.owner,
		inventory,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	builder.owner.git.administration = inventory
	return true
}

func (builder *sourceConstructionBuilder) validateSourceAdministrativeInventoryRequest() *sourcePrimitiveFailure {
	if builder == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if builder.primitives == nil || builder.owner == nil ||
		!builder.owner.validPackedRefsRetention() {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

type sourceDescriptorObservation struct {
	evidence sourceDescriptorEvidence
	acl      parsedSourceACL
}

func (builder *sourceConstructionBuilder) observeSourceDescriptorWithoutPolicy(
	descriptor *ownedSourceDescriptor,
	expectedKind sourceObservedKind,
) (sourceDescriptorObservation, *sourcePrimitiveFailure) {
	if failure := validateSourceObservationRequest(
		builder.ctx,
		descriptor,
		expectedKind,
		0,
	); failure != nil {
		return sourceDescriptorObservation{}, failure
	}
	snapshot, failure := builder.primitives.statDescriptor(builder.ctx, descriptor)
	if failure != nil {
		return sourceDescriptorObservation{}, failure
	}
	mount, failure := builder.primitives.statFilesystem(builder.ctx, descriptor)
	if failure != nil {
		return sourceDescriptorObservation{}, failure
	}
	rawACL, failure := builder.primitives.acquireRawACL(builder.ctx, descriptor)
	if failure != nil {
		return sourceDescriptorObservation{}, failure
	}
	acl, failure := builder.primitives.parseRawACL(builder.ctx, rawACL)
	if failure != nil {
		return sourceDescriptorObservation{}, failure
	}
	if !acl.valid() {
		return sourceDescriptorObservation{}, newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if observedSourceKind(snapshotKind(snapshot)) != expectedKind {
		return sourceDescriptorObservation{}, newSourcePrimitiveFailure(
			OperationCompare,
			CauseIdentity,
		)
	}
	snapshot.identity.Filesystem = mount.filesystem
	return sourceDescriptorObservation{
		evidence: sourceDescriptorEvidence{
			snapshot:  snapshot,
			mount:     mount,
			aclDigest: acl.digest,
		},
		acl: acl,
	}, nil
}

func (builder *sourceConstructionBuilder) reobserveSourceDescriptorBeforePolicy(
	descriptor *ownedSourceDescriptor,
	expected sourceDescriptorEvidence,
	expectedKind sourceObservedKind,
	owners ownerPolicy,
	requireSingleLink bool,
	maximumBytes int64,
) *sourcePrimitiveFailure {
	observation, failure := builder.observeSourceDescriptorWithoutPolicy(descriptor, expectedKind)
	if failure != nil {
		return failure
	}
	if failure = compareSourceDescriptorEvidence(
		builder.ctx,
		expected,
		observation.evidence,
	); failure != nil {
		return failure
	}
	if failure = builder.primitives.validateFilesystem(
		builder.ctx,
		observation.evidence.mount,
	); failure != nil {
		return failure
	}
	if failure = builder.primitives.validateACL(builder.ctx, observation.acl); failure != nil {
		return failure
	}
	_, failure = validateSourceDescriptorEvidence(
		builder.ctx,
		observation.evidence.snapshot,
		observation.evidence.mount,
		observation.acl,
		expectedKind,
		owners,
		requireSingleLink,
		maximumBytes,
	)
	return failure
}

func (builder *sourceConstructionBuilder) compareSourceDescriptorWithoutPolicy(
	descriptor *ownedSourceDescriptor,
	expected sourceDescriptorEvidence,
	expectedKind sourceObservedKind,
) *sourcePrimitiveFailure {
	observation, failure := builder.observeSourceDescriptorWithoutPolicy(descriptor, expectedKind)
	if failure != nil {
		return failure
	}
	return compareSourceDescriptorEvidence(builder.ctx, expected, observation.evidence)
}

func (builder *sourceConstructionBuilder) captureSourceAdministrativeDirectory(
	capture *sourceAdministrativeCapture,
	descriptor *ownedSourceDescriptor,
	prefix string,
	before sourceDescriptorObservation,
	presence sourcePresenceMode,
	sealed *sourceAdministrativeInventory,
) bool {
	if capture == nil || descriptor == nil || prefix == "" ||
		descriptor.kind != sourceObservedDirectory ||
		!validSourceAdministrativeCaptureMode(presence, sealed) {
		builder.failInvariant()
		return false
	}
	for {
		names, terminal, failure := builder.primitives.readDirectoryBatch(
			builder.ctx,
			descriptor,
		)
		if failure != nil {
			builder.outcome.addPrimitive(failure)
			return false
		}
		for _, name := range names {
			if !builder.captureSourceAdministrativeEntry(
				capture,
				descriptor,
				prefix,
				name,
				presence,
				sealed,
			) {
				return false
			}
		}
		if terminal {
			break
		}
	}
	after, failure := builder.observeSourceDescriptorWithoutPolicy(
		descriptor,
		sourceObservedDirectory,
	)
	if failure == nil {
		failure = compareSourceDescriptorEvidence(
			builder.ctx,
			before.evidence,
			after.evidence,
		)
	}
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	return true
}

func (builder *sourceConstructionBuilder) captureSourceAdministrativeEntry(
	capture *sourceAdministrativeCapture,
	parent *ownedSourceDescriptor,
	prefix string,
	name string,
	presence sourcePresenceMode,
	sealed *sourceAdministrativeInventory,
) bool {
	path, failure := sourceAdministrativeChildPath(builder.ctx, prefix, name)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	var sealedRow sourceAdministrativeRow
	if sealed != nil {
		var present bool
		sealedRow, present = sourceAdministrativeRowByPath(sealed.rows, path)
		if !present {
			builder.outcome.addPrimitive(newSourcePrimitiveFailure(
				OperationCompare,
				CauseUnstable,
			))
			return false
		}
	}
	if capture.descendants >= maxRepositoryEntries {
		builder.outcome.addPrimitive(newSourcePrimitiveFailure(OperationWalk, CauseLimit))
		return false
	}
	capture.descendants++

	kind, present, failure := builder.primitives.probeRelativeKind(
		builder.ctx,
		parent,
		name,
		presence,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !present || !kind.valid() {
		builder.failInvariant()
		return false
	}
	if sealed != nil && kind != sealedRow.kind {
		builder.outcome.addPrimitive(newSourcePrimitiveFailure(
			OperationCompare,
			CauseUnstable,
		))
		return false
	}
	candidate := sourceAdministrativeCandidate{row: sourceAdministrativeRow{
		path: path,
		kind: kind,
	}}
	if kind == sourceObservedSymlink || kind == sourceObservedSpecial {
		capture.candidates = append(capture.candidates, candidate)
		return true
	}
	return builder.captureOpenedSourceAdministrativeEntry(
		capture,
		parent,
		name,
		candidate,
		presence,
		sealed,
	)
}

func (builder *sourceConstructionBuilder) captureOpenedSourceAdministrativeEntry(
	capture *sourceAdministrativeCapture,
	parent *ownedSourceDescriptor,
	name string,
	candidate sourceAdministrativeCandidate,
	presence sourcePresenceMode,
	sealed *sourceAdministrativeInventory,
) bool {
	descriptor, openFailure := builder.primitives.openRelativeNoFollow(
		builder.ctx,
		parent,
		name,
		candidate.row.kind,
		presence,
	)
	if descriptor == nil {
		if openFailure == nil {
			openFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		builder.outcome.addPrimitive(openFailure)
		return false
	}
	if openFailure != nil {
		builder.outcome.addPrimitive(openFailure)
		builder.closeTransientDescriptor(descriptor)
		return false
	}

	observation, failure := builder.observeSourceDescriptorWithoutPolicy(
		descriptor,
		candidate.row.kind,
	)
	if failure == nil && sealed != nil {
		sealedRow, present := sourceAdministrativeRowByPath(
			sealed.rows,
			candidate.row.path,
		)
		if !present {
			failure = newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		} else {
			failure = compareRevalidatedSourceAdministrativeEvidence(
				builder.ctx,
				builder.owner.config.claim.objectFormat,
				sealedRow.evidence,
				observation.evidence,
				sealedRow,
			)
		}
	}
	if failure == nil && candidate.row.kind == sourceObservedRegular {
		failure = capture.chargeRegular(
			builder.ctx,
			candidate.row.path,
			observation.evidence.snapshot.size,
			builder.owner.config.claim.objectFormat,
		)
	}
	// A directory's device and FSID are traversal authority, not deferred row
	// policy: do not issue ReadDir against a nested mount. The complete
	// classification pass later reapplies the same containment predicate with
	// the rest of the row policy before the inventory can be installed.
	if failure == nil && candidate.row.kind == sourceObservedDirectory {
		failure = validateInitialSourceChild(
			builder.ctx,
			builder.owner.git.root.evidence,
			observation.evidence,
		)
	}
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		builder.closeTransientDescriptor(descriptor)
		return false
	}
	candidate.row.evidence = observation.evidence
	candidate.acl = observation.acl
	capture.candidates = append(capture.candidates, candidate)

	captured := true
	if candidate.row.kind == sourceObservedDirectory {
		captured = builder.captureSourceAdministrativeDirectory(
			capture,
			descriptor,
			candidate.row.path,
			observation,
			presence,
			sealed,
		)
	}
	closeFailed := builder.closeTransientDescriptor(descriptor)
	return captured && !closeFailed
}

func validSourceAdministrativeCaptureMode(
	presence sourcePresenceMode,
	sealed *sourceAdministrativeInventory,
) bool {
	return (presence == sourceInitialWalkPresent && sealed == nil) ||
		(presence == sourceRevalidatePresent && sealed != nil)
}

func (capture *sourceAdministrativeCapture) chargeRegular(
	ctx context.Context,
	path string,
	size int64,
	format repositoryObjectFormat,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return failure
	}
	if capture == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if size < 0 {
		return newSourcePrimitiveFailure(OperationWalk, CauseLimit)
	}
	byteSize := uint64(size)
	if capture.totalRegularBytes > maxRepositoryBytes ||
		byteSize > maxRepositoryBytes-capture.totalRegularBytes ||
		(sourceAdministrativePackPath(path, format) && byteSize > maxRepositoryFileBytes) {
		return newSourcePrimitiveFailure(OperationWalk, CauseLimit)
	}
	capture.totalRegularBytes += byteSize
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return failure
	}
	return nil
}

func (builder *sourceConstructionBuilder) finishSourceAdministrativeInventory(
	capture *sourceAdministrativeCapture,
) (*sourceAdministrativeInventory, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationWalk); failure != nil {
		return nil, failure
	}
	if capture == nil || len(capture.candidates) == 0 ||
		capture.descendants != uint64(len(capture.candidates)-1) { //nolint:gosec // Capture stops at one million rows, so this conversion is bounded.
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
	if failure := rejectForbiddenSourceAdministrativeCandidates(
		builder.ctx,
		capture.candidates,
	); failure != nil {
		return nil, failure
	}
	if failure := builder.compareRetainedSourceAdministrativeRows(capture.candidates); failure != nil {
		return nil, failure
	}
	for index := range capture.candidates {
		candidate := capture.candidates[index]
		if failure := builder.validateSourceAdministrativeCandidate(candidate); failure != nil {
			return nil, failure
		}
	}
	rows := make([]sourceAdministrativeRow, len(capture.candidates))
	for index := range capture.candidates {
		rows[index] = capture.candidates[index].row
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationWalk); failure != nil {
		return nil, failure
	}
	return &sourceAdministrativeInventory{
		rows:              rows,
		totalRegularBytes: capture.totalRegularBytes,
	}, nil
}

func classifySourceAdministrativeCandidates(
	ctx context.Context,
	candidates []sourceAdministrativeCandidate,
	format repositoryObjectFormat,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return failure
	}
	for index := range candidates {
		if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
			return failure
		}
		candidate := &candidates[index]
		if index > 0 && candidate.row.path == candidates[index-1].row.path {
			return newSourcePrimitiveFailure(OperationWalk, CauseUnstable)
		}
		candidate.row.class = classifySourceAdministrativePath(
			candidate.row.path,
			sourceAdministrativeEntryKind(candidate.row.kind),
			format,
		)
	}
	return sourceContextPrimitiveFailure(ctx, OperationWalk)
}

func rejectForbiddenSourceAdministrativeCandidates(
	ctx context.Context,
	candidates []sourceAdministrativeCandidate,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	for index := range candidates {
		if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
			return failure
		}
		if candidates[index].row.class == sourceForbidden {
			return newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
		}
	}
	return sourceContextPrimitiveFailure(ctx, OperationValidate)
}

func validateSourceAdministrativeInventoryOwner(
	ctx context.Context,
	owner *sourceConstructionOwner,
	inventory *sourceAdministrativeInventory,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if owner == nil || inventory == nil || !owner.validPackedRefsRetention() ||
		!inventory.valid(
			owner.config.claim.objectFormat,
			owner.packedRefs,
			owner.git.root.evidence,
			owner.config.evidence,
			owner.objects.root.evidence,
		) {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func (builder *sourceConstructionBuilder) compareRetainedSourceAdministrativeRows(
	candidates []sourceAdministrativeCandidate,
) *sourcePrimitiveFailure {
	if failure := builder.compareRetainedSourceAdministrativeRow(
		candidates,
		".git",
		sourceObservedDirectory,
		builder.owner.git.root.evidence,
	); failure != nil {
		return failure
	}
	if failure := builder.compareRetainedSourceAdministrativeRow(
		candidates,
		".git/config",
		sourceObservedRegular,
		builder.owner.config.evidence,
	); failure != nil {
		return failure
	}
	if failure := builder.compareRetainedSourceAdministrativeRow(
		candidates,
		".git/objects",
		sourceObservedDirectory,
		builder.owner.objects.root.evidence,
	); failure != nil {
		return failure
	}
	packedIndex := sourceAdministrativeCandidateIndex(candidates, ".git/packed-refs")
	switch builder.owner.packedRefs.state {
	case sourcePackedRefsUnresolved:
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	case sourcePackedRefsAbsent:
		if packedIndex >= 0 {
			return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
	case sourcePackedRefsRetained:
		if builder.owner.packedRefs.leaf == nil {
			return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		return builder.compareRetainedSourceAdministrativeRow(
			candidates,
			".git/packed-refs",
			sourceObservedRegular,
			builder.owner.packedRefs.leaf.evidence,
		)
	default:
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	return nil
}

func (builder *sourceConstructionBuilder) compareRetainedSourceAdministrativeRow(
	candidates []sourceAdministrativeCandidate,
	path string,
	kind sourceObservedKind,
	evidence sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationCompare); failure != nil {
		return failure
	}
	index := sourceAdministrativeCandidateIndex(candidates, path)
	if index < 0 {
		return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	row := candidates[index].row
	if row.kind != kind || row.class != sourceAuthorityAndManifest {
		return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return compareSourceDescriptorEvidence(builder.ctx, evidence, row.evidence)
}

func (builder *sourceConstructionBuilder) validateSourceAdministrativeCandidate(
	candidate sourceAdministrativeCandidate,
) *sourcePrimitiveFailure {
	if candidate.row.kind != sourceObservedDirectory &&
		candidate.row.kind != sourceObservedRegular {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := builder.primitives.validateFilesystem(
		builder.ctx,
		candidate.row.evidence.mount,
	); failure != nil {
		return failure
	}
	if failure := builder.primitives.validateACL(builder.ctx, candidate.acl); failure != nil {
		return failure
	}
	evidence, failure := validateSourceDescriptorEvidence(
		builder.ctx,
		candidate.row.evidence.snapshot,
		candidate.row.evidence.mount,
		candidate.acl,
		candidate.row.kind,
		ownerEffectiveOnly,
		candidate.row.kind == sourceObservedRegular &&
			candidate.row.class == sourceAuthorityAndManifest,
		0,
	)
	if failure != nil {
		return failure
	}
	if evidence != candidate.row.evidence {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if candidate.row.path == ".git" {
		return nil
	}
	return validateInitialSourceChild(
		builder.ctx,
		builder.owner.git.root.evidence,
		candidate.row.evidence,
	)
}

func sourceAdministrativeChildPath(
	ctx context.Context,
	prefix string,
	name string,
) (string, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return "", failure
	}
	if prefix == "" || name == "" || strings.IndexByte(name, 0) >= 0 ||
		name == "." || name == ".." || strings.Contains(name, "/") {
		return "", newSourcePrimitiveFailure(OperationWalk, CauseUnstable)
	}
	if len([]byte(name)) > maxPathComponentBytes ||
		len([]byte(prefix))+1+len([]byte(name)) > maxRelativePathBytes {
		return "", newSourcePrimitiveFailure(OperationWalk, CauseLimit)
	}
	path := prefix + "/" + name
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return "", failure
	}
	return path, nil
}

func sourceAdministrativeEntryKind(kind sourceObservedKind) entryKind {
	switch kind {
	case sourceObservedDirectory:
		return entryDirectory
	case sourceObservedRegular:
		return entryRegular
	case sourceObservedSymlink:
		return entrySymlink
	case sourceObservedSpecial:
		return 0
	default:
		return 0
	}
}

func sourceAdministrativePackPath(path string, format repositoryObjectFormat) bool {
	components, ok := splitNormalizedSourcePath(path)
	return ok && len(components) == 4 && components[0] == ".git" &&
		components[1] == "objects" && components[2] == "pack" &&
		matchesSourceObjectHashFile(components[3], "pack-", ".pack", format)
}

func sourceAdministrativeCandidateIndex(
	candidates []sourceAdministrativeCandidate,
	path string,
) int {
	index := sort.Search(len(candidates), func(index int) bool {
		return bytes.Compare([]byte(candidates[index].row.path), []byte(path)) >= 0
	})
	if index >= len(candidates) || candidates[index].row.path != path {
		return -1
	}
	return index
}
