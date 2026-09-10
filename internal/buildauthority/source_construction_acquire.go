package buildauthority

import (
	"context"
)

func retainSourceConstruction(
	ctx context.Context,
	locator sourceRepositoryLocator,
) (*sourceConstructionOwner, sourceUseOutcome) {
	primitives, failure := platformSourcePrimitives()
	if failure != nil {
		var outcome sourceUseOutcome
		outcome.addPrimitive(failure)
		return nil, outcome
	}
	return retainSourceConstructionWith(ctx, locator, primitives)
}

func retainSourceConstructionWith(
	ctx context.Context,
	locator sourceRepositoryLocator,
	primitives sourcePrimitives,
) (*sourceConstructionOwner, sourceUseOutcome) {
	builder := &sourceConstructionBuilder{
		ctx:        ctx,
		primitives: primitives,
		owner:      newSourceConstructionOwner(),
	}
	if ctx == nil || primitives == nil || !locator.valid() {
		builder.outcome.addPrimitive(newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		))
	} else {
		builder.retainInitialSource(locator)
	}
	if builder.outcome.proved() {
		builder.outcome.addPrimitive(validateInitialSourceOwner(builder.ctx, builder.owner))
	}
	if !builder.outcome.proved() {
		builder.owner.closeIntoWith(primitives, &builder.outcome)
		return nil, builder.outcome
	}
	return builder.owner, builder.outcome
}

func (builder *sourceConstructionBuilder) retainInitialSource(locator sourceRepositoryLocator) {
	if !builder.retainRepository(locator) ||
		!builder.retainGit() ||
		!builder.retainConfig() ||
		!builder.retainObjects() {
		return
	}
}

func (builder *sourceConstructionBuilder) retainRepository(locator sourceRepositoryLocator) bool {
	repository := &retainedSourceRepository{path: locator.path}
	builder.owner.repository = repository

	root, failure := builder.primitives.openRepositoryRoot(builder.ctx, locator)
	if root != nil {
		repository.root.root = root
	}
	if !builder.acceptRootAcquisition(root, failure) {
		return false
	}
	return builder.retainRepositoryDescriptor(locator, repository)
}

func (builder *sourceConstructionBuilder) retainRepositoryDescriptor(
	locator sourceRepositoryLocator,
	repository *retainedSourceRepository,
) bool {
	components, err := absolutePathComponents(locator.path)
	if err != nil || len(components) == 0 {
		builder.failInvariant()
		return false
	}

	transient := make([]*ownedSourceDescriptor, 0, len(components))
	physicalRoot, openFailure := builder.primitives.openPhysicalRootDescriptor(builder.ctx)
	if physicalRoot != nil {
		transient = append(transient, physicalRoot)
	}
	if !builder.acceptDescriptorAcquisition(physicalRoot, openFailure) {
		builder.closeTransientDescriptors(transient)
		return false
	}

	pathClaims := make([]authorityPathClaim, 0, len(components)+1)
	physicalEvidence, evidenceFailure := builder.observeDescriptor(
		physicalRoot,
		sourceObservedDirectory,
		ownerRootOrEffective,
		false,
		0,
	)
	if evidenceFailure != nil {
		builder.outcome.addPrimitive(evidenceFailure)
		builder.closeTransientDescriptors(transient)
		return false
	}
	pathClaims = append(pathClaims, physicalEvidence.pathClaim())

	parent := physicalRoot
	for index, component := range components {
		last := index == len(components)-1
		next, nextFailure := builder.primitives.openRelativeNoFollow(
			builder.ctx,
			parent,
			component,
			sourceObservedDirectory,
			sourceInitialRequired,
		)
		if next != nil {
			if last {
				repository.root.descriptor = next
			} else {
				transient = append(transient, next)
			}
		}
		if !builder.acceptDescriptorAcquisition(next, nextFailure) {
			builder.closeTransientDescriptors(transient)
			return false
		}

		owners := ownerRootOrEffective
		if last {
			owners = ownerEffectiveOnly
		}
		evidence, failure := builder.observeDescriptor(
			next,
			sourceObservedDirectory,
			owners,
			false,
			0,
		)
		if failure != nil {
			builder.outcome.addPrimitive(failure)
			builder.closeTransientDescriptors(transient)
			return false
		}
		pathClaims = append(pathClaims, evidence.pathClaim())
		if last {
			repository.root.evidence = evidence
		} else {
			parent = next
		}
	}

	if failure := builder.primitives.compareRootAndDescriptor(
		builder.ctx,
		repository.root.root,
		repository.root.descriptor,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		builder.closeTransientDescriptors(transient)
		return false
	}
	repository.pathClaims = append([]authorityPathClaim(nil), pathClaims...)
	return !builder.closeTransientDescriptors(transient)
}

func (builder *sourceConstructionBuilder) retainGit() bool {
	git := &retainedSourceGit{}
	builder.owner.git = git
	return builder.retainSourceDirectory(
		&git.root,
		builder.owner.repository.root.root,
		builder.owner.repository.root.descriptor,
		".git",
		builder.owner.repository.root.evidence,
	)
}

func (builder *sourceConstructionBuilder) retainConfig() bool {
	config := &retainedSourceConfig{}
	builder.owner.config = config
	kind, present, failure := builder.primitives.probeRelativeKind(
		builder.ctx,
		builder.owner.git.root.descriptor,
		"config",
		sourceInitialRequired,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = requireInitialSourceKind(
		builder.ctx,
		kind,
		present,
		sourceObservedRegular,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}

	descriptor, descriptorFailure := builder.primitives.openRelativeNoFollow(
		builder.ctx,
		builder.owner.git.root.descriptor,
		"config",
		kind,
		sourceInitialRequired,
	)
	if descriptor != nil {
		config.descriptor = descriptor
	}
	if !builder.acceptDescriptorAcquisition(descriptor, descriptorFailure) {
		return false
	}

	evidence, failure := builder.observeDescriptor(
		descriptor,
		sourceObservedRegular,
		ownerEffectiveOnly,
		true,
		maxSourceConfigBytes,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = validateInitialSourceChild(
		builder.ctx,
		builder.owner.git.root.evidence,
		evidence,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}

	content := make([]byte, int(evidence.snapshot.size))
	if failure = builder.primitives.readExactForParse(
		builder.ctx,
		descriptor,
		content,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	digest, hashFailure := builder.primitives.hashBytes(builder.ctx, content)
	if hashFailure != nil {
		builder.outcome.addPrimitive(hashFailure)
		return false
	}
	claim, configFailure := parseInitialSourceConfig(builder.ctx, content)
	if configFailure != nil {
		builder.outcome.addPrimitive(configFailure)
		return false
	}

	failure = builder.reobserveDescriptorBeforePolicy(
		descriptor,
		evidence,
		maxSourceConfigBytes,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	config.evidence = evidence
	config.digest = digest
	config.claim = claim
	return builder.rebindConfig()
}

func (builder *sourceConstructionBuilder) rebindConfig() bool {
	comparison, openFailure := builder.primitives.openRelativeNoFollow(
		builder.ctx,
		builder.owner.git.root.descriptor,
		"config",
		sourceObservedRegular,
		sourceRevalidatePresent,
	)
	if comparison == nil {
		if openFailure == nil {
			openFailure = newSourcePrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			)
		}
		builder.outcome.addPrimitive(openFailure)
		return false
	}
	if openFailure != nil {
		builder.outcome.addPrimitive(openFailure)
		builder.closeTransientDescriptor(comparison)
		return false
	}

	failure := builder.reobserveDescriptorBeforePolicy(
		comparison,
		builder.owner.config.evidence,
		maxSourceConfigBytes,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
	}
	closeFailed := builder.closeTransientDescriptor(comparison)
	return failure == nil && !closeFailed
}

func (builder *sourceConstructionBuilder) retainObjects() bool {
	objects := &retainedSourceObjects{}
	builder.owner.objects = objects
	return builder.retainSourceDirectory(
		&objects.root,
		builder.owner.git.root.root,
		builder.owner.git.root.descriptor,
		"objects",
		builder.owner.git.root.evidence,
	)
}

func (builder *sourceConstructionBuilder) retainPackedRefs() bool {
	if failure := builder.validatePackedRefsRequest(); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	kind, present, failure := builder.primitives.probeRelativeKind(
		builder.ctx,
		builder.owner.git.root.descriptor,
		"packed-refs",
		sourceInitialOptional,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !present {
		return builder.retainAbsentPackedRefs(kind, present)
	}
	if failure = requireInitialSourceKind(
		builder.ctx,
		kind,
		present,
		sourceObservedRegular,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	return builder.retainPresentPackedRefs(kind)
}

func (builder *sourceConstructionBuilder) retainAbsentPackedRefs(
	kind sourceObservedKind,
	present bool,
) bool {
	if failure := validateInitialPackedRefsAbsence(
		builder.ctx,
		kind,
		present,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	kind, present, failure := builder.primitives.probeRelativeKind(
		builder.ctx,
		builder.owner.git.root.descriptor,
		"packed-refs",
		sourceRevalidateAbsent,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = validateRevalidatedPackedRefsAbsence(
		builder.ctx,
		kind,
		present,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	builder.owner.packedRefs = sourcePackedRefsSlot{state: sourcePackedRefsAbsent}
	return builder.validatePackedRefsOwner()
}

func (builder *sourceConstructionBuilder) retainPresentPackedRefs(
	kind sourceObservedKind,
) bool {
	retained := &retainedSourcePackedRefs{}
	descriptor, descriptorFailure := builder.primitives.openRelativeNoFollow(
		builder.ctx,
		builder.owner.git.root.descriptor,
		"packed-refs",
		kind,
		sourceInitialOptional,
	)
	if descriptor != nil {
		retained.descriptor = descriptor
		builder.owner.packedRefs = sourcePackedRefsSlot{
			state: sourcePackedRefsRetained,
			leaf:  retained,
		}
	}
	if !builder.acceptDescriptorAcquisition(descriptor, descriptorFailure) {
		return false
	}

	evidence, failure := builder.observeDescriptor(
		descriptor,
		sourceObservedRegular,
		ownerEffectiveOnly,
		true,
		maxPackedRefsBytes,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = validateInitialSourceChild(
		builder.ctx,
		builder.owner.git.root.evidence,
		evidence,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	return builder.retainPackedRefsContent(evidence)
}

func (builder *sourceConstructionBuilder) retainPackedRefsContent(
	evidence sourceDescriptorEvidence,
) bool {
	retained := builder.owner.packedRefs.leaf
	if retained == nil || !retained.descriptor.validOpen() {
		builder.failInvariant()
		return false
	}
	descriptor := retained.descriptor
	contentProved := true
	content := make([]byte, int(evidence.snapshot.size))
	if failure := builder.primitives.readExactForParse(
		builder.ctx,
		descriptor,
		content,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		contentProved = false
	}
	var digest Digest
	if contentProved {
		var hashFailure *sourcePrimitiveFailure
		digest, hashFailure = builder.primitives.hashBytes(builder.ctx, content)
		if hashFailure != nil {
			builder.outcome.addPrimitive(hashFailure)
			contentProved = false
		}
	}
	var claim packedRefsClaim
	if contentProved {
		var packedRefsFailure *sourcePrimitiveFailure
		claim, packedRefsFailure = parseInitialSourcePackedRefs(
			builder.ctx,
			content,
			builder.owner.config.claim.objectFormat,
		)
		if packedRefsFailure != nil {
			builder.outcome.addPrimitive(packedRefsFailure)
			contentProved = false
		}
	}

	tailFailure := builder.reobserveDescriptorBeforePolicy(
		descriptor,
		evidence,
		maxPackedRefsBytes,
	)
	tailProved := tailFailure == nil
	if tailFailure != nil {
		builder.outcome.addPrimitive(tailFailure)
	}
	preRebindProved := contentProved && tailProved
	if !preRebindProved {
		builder.closeFailedPackedRefs()
	}
	rebindProved := builder.rebindPackedRefs(evidence)
	if !preRebindProved || !rebindProved {
		return false
	}
	retained.evidence = evidence
	retained.digest = digest
	retained.claim = claim
	return builder.validatePackedRefsOwner()
}

func (builder *sourceConstructionBuilder) rebindPackedRefs(
	expected sourceDescriptorEvidence,
) bool {
	comparison, openFailure := builder.primitives.openRelativeNoFollow(
		builder.ctx,
		builder.owner.git.root.descriptor,
		"packed-refs",
		sourceObservedRegular,
		sourceRevalidatePresent,
	)
	if comparison == nil {
		if openFailure == nil {
			openFailure = newSourcePrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			)
		}
		builder.outcome.addPrimitive(openFailure)
		return false
	}
	if openFailure != nil {
		builder.outcome.addPrimitive(openFailure)
		builder.closeTransientDescriptor(comparison)
		return false
	}

	failure := builder.reobserveDescriptorBeforePolicy(
		comparison,
		expected,
		maxPackedRefsBytes,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
	}
	closeFailed := builder.closeTransientDescriptor(comparison)
	return failure == nil && !closeFailed
}

func (builder *sourceConstructionBuilder) closeFailedPackedRefs() {
	retained := builder.owner.packedRefs.leaf
	if retained != nil {
		builder.closeTransientDescriptor(retained.descriptor)
		retained.descriptor = nil
	}
	builder.owner.packedRefs = sourcePackedRefsSlot{state: sourcePackedRefsUnresolved}
}

func (builder *sourceConstructionBuilder) validatePackedRefsRequest() *sourcePrimitiveFailure {
	if builder == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if builder.primitives == nil || builder.owner == nil || !builder.owner.validInitialRetention() {
		validationFailure = newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func (builder *sourceConstructionBuilder) validatePackedRefsOwner() bool {
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !builder.owner.validPackedRefsRetention() {
		builder.failInvariant()
		return false
	}
	if failure := sourceContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	return true
}

func (builder *sourceConstructionBuilder) retainSourceDirectory(
	retained *retainedSourceRoot,
	parentRoot *ownedSourceRoot,
	parentDescriptor *ownedSourceDescriptor,
	name string,
	parentEvidence sourceDescriptorEvidence,
) bool {
	if retained == nil || !parentRoot.validOpen() || !parentDescriptor.validOpen() || name == "" {
		builder.failInvariant()
		return false
	}
	kind, present, failure := builder.primitives.probeRelativeKind(
		builder.ctx,
		parentDescriptor,
		name,
		sourceInitialRequired,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = requireInitialSourceKind(
		builder.ctx,
		kind,
		present,
		sourceObservedDirectory,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}

	root, rootFailure := builder.primitives.openChildRoot(
		builder.ctx,
		parentRoot,
		name,
	)
	if root != nil {
		retained.root = root
	}
	if !builder.acceptRootAcquisition(root, rootFailure) {
		return false
	}
	descriptor, descriptorFailure := builder.primitives.openRelativeNoFollow(
		builder.ctx,
		parentDescriptor,
		name,
		kind,
		sourceInitialRequired,
	)
	if descriptor != nil {
		retained.descriptor = descriptor
	}
	if !builder.acceptDescriptorAcquisition(descriptor, descriptorFailure) {
		return false
	}

	evidence, failure := builder.observeDescriptor(
		descriptor,
		sourceObservedDirectory,
		ownerEffectiveOnly,
		false,
		0,
	)
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = validateInitialSourceChild(builder.ctx, parentEvidence, evidence); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if failure = builder.primitives.compareRootAndDescriptor(
		builder.ctx,
		root,
		descriptor,
	); failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	retained.evidence = evidence
	return true
}

func (builder *sourceConstructionBuilder) observeDescriptor(
	descriptor *ownedSourceDescriptor,
	expectedKind sourceObservedKind,
	owners ownerPolicy,
	requireSingleLink bool,
	maximumBytes int64,
) (sourceDescriptorEvidence, *sourcePrimitiveFailure) {
	if failure := validateSourceObservationRequest(
		builder.ctx,
		descriptor,
		expectedKind,
		maximumBytes,
	); failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	snapshot, failure := builder.primitives.statDescriptor(builder.ctx, descriptor)
	if failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	mount, failure := builder.primitives.statFilesystem(builder.ctx, descriptor)
	if failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	if failure = builder.primitives.validateFilesystem(builder.ctx, mount); failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	rawACL, failure := builder.primitives.acquireRawACL(builder.ctx, descriptor)
	if failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	acl, failure := builder.primitives.parseRawACL(builder.ctx, rawACL)
	if failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	if failure = builder.primitives.validateACL(builder.ctx, acl); failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	return validateSourceDescriptorEvidence(
		builder.ctx,
		snapshot,
		mount,
		acl,
		expectedKind,
		owners,
		requireSingleLink,
		maximumBytes,
	)
}

// reobserveDescriptorBeforePolicy captures every comparison claim before it
// reapplies admission policy. A retained object that changes mode, ACL, or
// mount security is drift, not a newly attributed initial-policy refusal.
func (builder *sourceConstructionBuilder) reobserveDescriptorBeforePolicy(
	descriptor *ownedSourceDescriptor,
	expected sourceDescriptorEvidence,
	maximumBytes int64,
) *sourcePrimitiveFailure {
	if failure := validateSourceObservationRequest(
		builder.ctx,
		descriptor,
		sourceObservedRegular,
		maximumBytes,
	); failure != nil {
		return failure
	}
	snapshot, failure := builder.primitives.statDescriptor(builder.ctx, descriptor)
	if failure != nil {
		return failure
	}
	mount, failure := builder.primitives.statFilesystem(builder.ctx, descriptor)
	if failure != nil {
		return failure
	}
	rawACL, failure := builder.primitives.acquireRawACL(builder.ctx, descriptor)
	if failure != nil {
		return failure
	}
	acl, failure := builder.primitives.parseRawACL(builder.ctx, rawACL)
	if failure != nil {
		return failure
	}
	if !acl.valid() {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	snapshot.identity.Filesystem = mount.filesystem
	evidence := sourceDescriptorEvidence{
		snapshot:  snapshot,
		mount:     mount,
		aclDigest: acl.digest,
	}
	if failure = compareSourceDescriptorEvidence(
		builder.ctx,
		expected,
		evidence,
	); failure != nil {
		return failure
	}
	if failure = builder.primitives.validateFilesystem(builder.ctx, mount); failure != nil {
		return failure
	}
	if failure = builder.primitives.validateACL(builder.ctx, acl); failure != nil {
		return failure
	}
	_, failure = validateSourceDescriptorEvidence(
		builder.ctx,
		snapshot,
		mount,
		acl,
		sourceObservedRegular,
		ownerEffectiveOnly,
		true,
		maximumBytes,
	)
	return failure
}

func validateSourceObservationRequest(
	ctx context.Context,
	descriptor *ownedSourceDescriptor,
	expectedKind sourceObservedKind,
	maximumBytes int64,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if !descriptor.validOpen() || descriptor.kind != expectedKind ||
		!expectedKind.valid() || maximumBytes < 0 {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func validateSourceDescriptorEvidence(
	ctx context.Context,
	snapshot fileSnapshot,
	mount mountSnapshot,
	acl parsedSourceACL,
	expectedKind sourceObservedKind,
	owners ownerPolicy,
	requireSingleLink bool,
	maximumBytes int64,
) (sourceDescriptorEvidence, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	var evidence sourceDescriptorEvidence
	var validationFailure *sourcePrimitiveFailure
	var validationErr error
	if acl.valid() {
		validationErr = validateProtected(snapshot, owners)
	}
	switch {
	case !acl.valid():
		validationFailure = newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	case validationErr != nil:
		validationFailure = sourcePrimitiveFailureFromError(
			OperationValidate,
			validationErr,
			CauseUnsupported,
		)
	case observedSourceKind(snapshotKind(snapshot)) != expectedKind:
		validationFailure = newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	case requireSingleLink && snapshot.linkCount != 1:
		validationFailure = newSourcePrimitiveFailure(
			OperationValidate,
			CauseUnsupported,
		)
	case maximumBytes != 0 && snapshot.size > maximumBytes:
		validationFailure = newSourcePrimitiveFailure(
			OperationValidate,
			CauseLimit,
		)
	default:
		snapshot.identity.Filesystem = mount.filesystem
		evidence = sourceDescriptorEvidence{
			snapshot:  snapshot,
			mount:     mount,
			aclDigest: acl.digest,
		}
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return sourceDescriptorEvidence{}, failure
	}
	return evidence, validationFailure
}

func parseInitialSourceConfig(
	ctx context.Context,
	content []byte,
) (sourceConfigClaim, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return sourceConfigClaim{}, failure
	}
	parsed, err := parseSourceConfigSyntax(content)
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return sourceConfigClaim{}, failure
	}
	if failure := sourcePrimitiveFailureFromError(
		OperationParse,
		err,
		CauseMalformed,
	); failure != nil {
		return sourceConfigClaim{}, failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return sourceConfigClaim{}, failure
	}
	claim, err := validateSourceConfigPolicy(parsed)
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return sourceConfigClaim{}, failure
	}
	if failure := sourcePrimitiveFailureFromError(
		OperationValidate,
		err,
		CauseUnsupported,
	); failure != nil {
		return sourceConfigClaim{}, failure
	}
	return claim, nil
}

func parseInitialSourcePackedRefs(
	ctx context.Context,
	content []byte,
	format repositoryObjectFormat,
) (packedRefsClaim, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return packedRefsClaim{}, failure
	}
	parsed, err := parsePackedRefsSyntax(content, format)
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return packedRefsClaim{}, failure
	}
	if failure := sourcePrimitiveFailureFromError(
		OperationParse,
		err,
		CauseMalformed,
	); failure != nil {
		return packedRefsClaim{}, failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return packedRefsClaim{}, failure
	}
	claim, err := validatePackedRefsPolicy(parsed)
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return packedRefsClaim{}, failure
	}
	if failure := sourcePrimitiveFailureFromError(
		OperationValidate,
		err,
		CauseUnsupported,
	); failure != nil {
		return packedRefsClaim{}, failure
	}
	return claim, nil
}

func validateInitialPackedRefsAbsence(
	ctx context.Context,
	kind sourceObservedKind,
	present bool,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if present || kind != 0 {
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

func validateRevalidatedPackedRefsAbsence(
	ctx context.Context,
	kind sourceObservedKind,
	present bool,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	var comparisonFailure *sourcePrimitiveFailure
	if present {
		comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	} else if kind != 0 {
		comparisonFailure = newSourcePrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	return comparisonFailure
}

func (evidence sourceDescriptorEvidence) pathClaim() authorityPathClaim {
	return makeAuthorityPathClaim(evidence.snapshot, evidence.mount, evidence.aclDigest)
}

func requireInitialSourceKind(
	ctx context.Context,
	kind sourceObservedKind,
	present bool,
	want sourceObservedKind,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if !present || !kind.valid() || !want.valid() {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	} else if kind != want {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func validateInitialSourceChild(
	ctx context.Context,
	parent sourceDescriptorEvidence,
	child sourceDescriptorEvidence,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if parent.snapshot.identity.Device != child.snapshot.identity.Device ||
		parent.mount.filesystem != child.mount.filesystem {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func compareSourceDescriptorEvidence(
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
	} else if before != after {
		comparisonFailure = newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	return comparisonFailure
}

func validateInitialSourceOwner(
	ctx context.Context,
	owner *sourceConstructionOwner,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	var validationFailure *sourcePrimitiveFailure
	if owner == nil || !owner.validInitialRetention() {
		validationFailure = newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	return validationFailure
}

func (builder *sourceConstructionBuilder) acceptRootAcquisition(
	root *ownedSourceRoot,
	failure *sourcePrimitiveFailure,
) bool {
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !root.validOpen() {
		builder.failInvariant()
		return false
	}
	return true
}

func (builder *sourceConstructionBuilder) acceptDescriptorAcquisition(
	descriptor *ownedSourceDescriptor,
	failure *sourcePrimitiveFailure,
) bool {
	if failure != nil {
		builder.outcome.addPrimitive(failure)
		return false
	}
	if !descriptor.validOpen() {
		builder.failInvariant()
		return false
	}
	return true
}

func (builder *sourceConstructionBuilder) closeTransientDescriptor(
	descriptor *ownedSourceDescriptor,
) bool {
	if descriptor == nil {
		return false
	}
	if builder.primitives.closeDescriptor(descriptor) {
		builder.outcome.addDescriptorClose()
		return true
	}
	return false
}

func (builder *sourceConstructionBuilder) closeTransientDescriptors(
	descriptors []*ownedSourceDescriptor,
) bool {
	closeFailed := false
	//nolint:modernize // Keep transient owner references inside this module instead of an iterator closure.
	for index := len(descriptors) - 1; index >= 0; index-- {
		closeFailed = builder.closeTransientDescriptor(descriptors[index]) || closeFailed
	}
	return closeFailed
}

func (builder *sourceConstructionBuilder) failInvariant() {
	builder.outcome.addPrimitive(newSourcePrimitiveFailure(
		OperationValidate,
		CauseInternalInvariant,
	))
}
