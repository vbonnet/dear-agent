package buildauthority

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type sealedGOROOTBracket struct {
	capture    *treeCapture
	directory  *retainedDirectory
	root       retainedDirectory
	policy     treePolicy
	digest     Digest
	entries    []entrySnapshot
	expected   map[string]entrySnapshot
	pathClaims []authorityPathClaim
	physical   sealedPhysicalRootBracket
}

func sealGOROOTBracket(
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
) (sealedGOROOTBracket, *authorityPrimitiveFailure) {
	physical, failure := sealPhysicalRootBracket(physicalRoot)
	if failure != nil {
		return sealedGOROOTBracket{}, failure
	}
	if goroot == nil || goroot.root == nil || goroot.root.root == nil ||
		goroot.root.descriptor == nil || goroot.policy != gorootPolicy() ||
		goroot.digest == (Digest{}) {
		return sealedGOROOTBracket{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	components, err := absolutePathComponents(goroot.root.path)
	if err != nil || len(goroot.root.pathClaims) != len(components)+1 ||
		len(goroot.root.pathClaims) == 0 ||
		goroot.root.pathClaims[0] != physicalRoot.claim ||
		!validSealedAuthorityPathClaims(goroot.root.pathClaims) {
		return sealedGOROOTBracket{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	root := *goroot.root
	root.pathClaims = append([]authorityPathClaim(nil), goroot.root.pathClaims...)
	entries := append([]entrySnapshot(nil), goroot.entries...)
	expected, failure := indexExpectedTreeEntries(root, goroot.policy, entries)
	if failure != nil {
		return sealedGOROOTBracket{}, failure
	}
	return sealedGOROOTBracket{
		capture:    goroot,
		directory:  goroot.root,
		root:       root,
		policy:     goroot.policy,
		digest:     goroot.digest,
		entries:    entries,
		expected:   expected,
		pathClaims: append([]authorityPathClaim(nil), goroot.root.pathClaims...),
		physical:   physical,
	}, nil
}

func indexExpectedTreeEntries(
	root retainedDirectory,
	policy treePolicy,
	entries []entrySnapshot,
) (map[string]entrySnapshot, *authorityPrimitiveFailure) {
	if !validSealedTreeRoot(root, policy) || uint64(len(entries)) > policy.limits.maxEntries {
		return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	indexed := make(map[string]entrySnapshot, len(entries))
	var totalBytes uint64
	for index, entry := range entries {
		if !validSealedTreeEntry(root, policy, entries, index) {
			return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if _, duplicate := indexed[entry.path]; duplicate {
			return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		nextTotal, err := chargeTreeEntry(policy.limits, uint64(index), totalBytes, entry)
		if err != nil {
			return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		totalBytes = nextTotal
		indexed[entry.path] = entry
	}
	return indexed, nil
}

func validSealedTreeRoot(root retainedDirectory, policy treePolicy) bool {
	return validateProtected(root.snapshot, policy.owners) == nil &&
		snapshotIsDirectory(root.snapshot) && len(root.pathClaims) > 0 &&
		makeAuthorityPathClaim(root.snapshot, root.mount, root.aclDigest) ==
			root.pathClaims[len(root.pathClaims)-1] &&
		root.snapshot.identity.Filesystem == root.mount.filesystem
}

func validSealedTreeEntry(
	root retainedDirectory,
	policy treePolicy,
	entries []entrySnapshot,
	index int,
) bool {
	if index < 0 || index >= len(entries) {
		return false
	}
	entry := entries[index]
	return validateRelativeManifestPath(entry.path) == nil &&
		(entry.kind == entryDirectory || entry.kind == entryRegular || entry.kind == entrySymlink) &&
		(index == 0 || entries[index-1].path < entry.path) &&
		validateProtectedEntrySnapshot(root, policy, entry) == nil
}

func validateProtectedEntrySnapshot(
	root retainedDirectory,
	policy treePolicy,
	entry entrySnapshot,
) error {
	if err := validateProtected(entry.file, policy.owners); err != nil ||
		snapshotKind(entry.file) != entry.kind ||
		entry.file.identity.Device != root.snapshot.identity.Device ||
		entry.file.identity.Filesystem != root.mount.filesystem {
		return errAuthorityKindMismatch
	}
	switch entry.kind {
	case entryDirectory:
		return nil
	case entryRegular:
		if entry.file.size < 0 || uint64(entry.file.size) > policy.limits.maxFileBytes {
			return fs.ErrInvalid
		}
		return nil
	case entrySymlink:
		if !policy.allowSymlinks || entry.file.size <= 0 ||
			uint64(entry.file.size) > maxSymlinkBytes ||
			int64(len([]byte(entry.linkText))) != entry.file.size {
			return fs.ErrInvalid
		}
		return nil
	default:
		return fs.ErrInvalid
	}
}

func (revalidator *preflightAuthorityRevalidator) revalidateGOROOT(
	ctx context.Context,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
) authorityUseOutcome {
	var outcome authorityUseOutcome
	if ctx == nil || !revalidator.validForBrackets() {
		outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant))
		return outcome
	}
	sealed, failure := sealGOROOTBracket(physicalRoot, goroot)
	if failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	if failure := revalidator.observeAndCompareTreeRoot(ctx, sealed, sealed.root.descriptor); failure != nil {
		outcome.addPrimitive(failure)
		return outcome
	}
	handleAcquisition, handleOpenFailure := revalidator.primitives.brackets.openRootHandle(
		ctx,
		sealed.root.root,
	)
	handleOwner, handleFailure := ownAuthorityAcquisition(handleAcquisition, handleOpenFailure)
	outcome.addPrimitive(handleFailure)
	if handleFailure == nil && handleOwner != nil {
		outcome.addPrimitive(revalidator.observeAndCompareTreeRoot(ctx, sealed, handleOwner.file))
	}
	if handleOwner != nil {
		handleOwner.closeInto(&outcome)
	}
	if outcome.primary != nil {
		return outcome
	}

	currentPath, pathOutcome := revalidator.resolveClaimedAuthorityPath(
		ctx,
		physicalRoot,
		sealed.root.path,
		sealed.pathClaims,
	)
	outcome.absorb(pathOutcome)
	if currentPath != nil {
		builder := attributedTreeBuilder{
			ctx:         ctx,
			revalidator: revalidator,
			sealed:      sealed,
			entries:     make([]entrySnapshot, 0, len(sealed.entries)),
		}
		builder.capture(currentPath.descriptor, &outcome)
		currentPath.closeInto(&outcome)
	}

	// Once the sealed inputs are structurally usable, pathname rebinding is a
	// mandatory trailing bracket even after walk, parse, hash, or close failure.
	trailingPath, trailingOutcome := revalidator.resolveClaimedAuthorityPath(
		ctx,
		physicalRoot,
		sealed.root.path,
		sealed.pathClaims,
	)
	outcome.absorb(trailingOutcome)
	if trailingPath != nil {
		trailingPath.closeInto(&outcome)
	}
	outcome.addPrimitive(compareGOROOTBracketSeal(ctx, sealed, physicalRoot, goroot))
	return outcome
}

func (revalidator *preflightAuthorityRevalidator) observeAndCompareTreeRoot(
	ctx context.Context,
	sealed sealedGOROOTBracket,
	descriptor *os.File,
) *authorityPrimitiveFailure {
	comparison, failure := revalidator.probeDescriptorObservation(ctx, descriptor)
	if failure != nil {
		return failure
	}
	observation := comparison.value
	if failure := compareTreeRootObservation(ctx, sealed.root, observation); failure != nil {
		return failure
	}
	if failure := validateDeferredAuthorityPolicy(ctx, comparison.policyFailure); failure != nil {
		return failure
	}
	return (&attributedTreeBuilder{
		ctx:         ctx,
		revalidator: revalidator,
		sealed:      sealed,
	}).validateOpenedObservation(observation, sealed.root.mount, entryDirectory)
}

type attributedTreeBuilder struct {
	ctx         context.Context
	revalidator *preflightAuthorityRevalidator
	sealed      sealedGOROOTBracket
	entries     []entrySnapshot
	totalBytes  uint64
}

func (builder *attributedTreeBuilder) capture(
	descriptor *os.File,
	outcome *authorityUseOutcome,
) {
	beforeComparison, failure := builder.revalidator.probeDescriptorObservation(
		builder.ctx,
		descriptor,
	)
	before := beforeComparison.value
	if failure == nil {
		failure = compareTreeRootObservation(builder.ctx, builder.sealed.root, before)
	}
	if failure == nil {
		failure = validateDeferredAuthorityPolicy(builder.ctx, beforeComparison.policyFailure)
	}
	if failure == nil {
		failure = builder.validateOpenedObservation(
			before,
			builder.sealed.root.mount,
			entryDirectory,
		)
	}
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	builder.captureDirectory(
		descriptor,
		"",
		authorityDescriptorObservation{
			snapshot:  before.snapshot,
			mount:     before.mount,
			aclDigest: before.aclDigest,
		},
		outcome,
	)
	if outcome.primary != nil {
		return
	}
	sortEntrySnapshots(builder.entries)
	if failure := compareTreeRowsBeforeBinding(
		builder.ctx,
		builder.sealed.entries,
		builder.entries,
	); failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	if failure := parseAttributedSymlinkBindings(builder.ctx, builder.entries); failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	digest, failure := builder.revalidator.primitives.brackets.hashManifest(
		builder.ctx,
		builder.sealed.policy.domain,
		&builder.sealed.root,
		builder.entries,
	)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	if failure := compareTreeCaptureResult(
		builder.ctx,
		builder.sealed,
		builder.entries,
		digest,
	); failure != nil {
		outcome.addPrimitive(failure)
	}
}

func (builder *attributedTreeBuilder) captureDirectory(
	descriptor *os.File,
	prefix string,
	before authorityDescriptorObservation,
	outcome *authorityUseOutcome,
) {
	for {
		names, done, failure := builder.revalidator.primitives.brackets.readDirectory(
			builder.ctx,
			descriptor,
			directoryReadBatchSize,
		)
		if failure != nil {
			outcome.addPrimitive(failure)
			return
		}
		if len(names) > directoryReadBatchSize || (!done && len(names) == 0) {
			outcome.addPrimitive(newAuthorityPrimitiveFailure(OperationWalk, CauseUnstable))
			return
		}
		for _, name := range names {
			builder.captureDirectoryEntry(descriptor, prefix, name, outcome)
			if outcome.primary != nil {
				return
			}
		}
		if done {
			break
		}
	}
	afterComparison, failure := builder.revalidator.probeDescriptorObservation(
		builder.ctx,
		descriptor,
	)
	after := afterComparison.value
	if failure == nil {
		failure = compareAuthorityObservations(builder.ctx, before, after)
	}
	if failure == nil {
		failure = validateDeferredAuthorityPolicy(builder.ctx, afterComparison.policyFailure)
	}
	if failure == nil {
		failure = builder.revalidator.primitives.validateFilesystem(builder.ctx, after.mount)
	}
	outcome.addPrimitive(failure)
}

func (builder *attributedTreeBuilder) captureDirectoryEntry(
	parent *os.File,
	prefix string,
	name string,
	outcome *authorityUseOutcome,
) {
	path, failure := builder.prepareDirectoryEntryPath(prefix, name)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	expected, failure := expectedTreeEntry(builder.sealed.expected, path)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	owner, kind, acquisitionFailure := builder.openExpectedTreeEntry(parent, name, expected)
	outcome.addPrimitive(acquisitionFailure)
	if acquisitionFailure == nil && owner != nil {
		builder.captureOwnedEntry(owner.file, path, kind, expected, outcome)
	}
	if owner != nil {
		owner.closeInto(outcome)
	}
}

func (builder *attributedTreeBuilder) prepareDirectoryEntryPath(
	prefix string,
	name string,
) (string, *authorityPrimitiveFailure) {
	if failure := authorityContextPrimitiveFailure(builder.ctx, OperationWalk); failure != nil {
		return "", failure
	}
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, 0) ||
		strings.ContainsRune(name, filepath.Separator) || strings.ContainsRune(name, '/') {
		return "", newAuthorityPrimitiveFailure(OperationWalk, CauseUnstable)
	}
	if len(name) > maxPathComponentBytes {
		return "", newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
	}
	path := name
	if prefix != "" {
		path = prefix + string(filepath.Separator) + name
	}
	if err := validateRelativeManifestPath(path); err != nil {
		causes := privateCauses(err, CauseUnstable)
		if len(causes) == 1 && causes[0] == CauseLimit {
			return "", newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
		}
		return "", newAuthorityPrimitiveFailure(OperationWalk, CauseUnstable)
	}
	if uint64(len(builder.entries)) >= builder.sealed.policy.limits.maxEntries {
		return "", newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
	}
	return path, nil
}

func (builder *attributedTreeBuilder) openExpectedTreeEntry(
	parent *os.File,
	name string,
	expected entrySnapshot,
) (*ownedAuthorityDescriptor, entryKind, *authorityPrimitiveFailure) {
	kind, failure := builder.revalidator.primitives.brackets.probeRelativeKind(
		builder.ctx,
		parent,
		name,
		authorityExpectedPresentOpen,
	)
	if failure != nil {
		return nil, 0, failure
	}
	if kind != expected.kind {
		return nil, kind, newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if kind != entryDirectory && kind != entryRegular && kind != entrySymlink {
		return nil, kind, newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	if kind == entrySymlink && !builder.sealed.policy.allowSymlinks {
		return nil, kind, newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	acquisition, openFailure := builder.revalidator.primitives.brackets.openTypedRelativeNoFollow(
		builder.ctx,
		parent,
		name,
		kind,
		authorityExpectedPresentOpen,
	)
	owner, acquisitionFailure := ownAuthorityAcquisition(acquisition, openFailure)
	return owner, kind, acquisitionFailure
}

func expectedTreeEntry(
	entries map[string]entrySnapshot,
	path string,
) (entrySnapshot, *authorityPrimitiveFailure) {
	entry, found := entries[path]
	if !found {
		return entrySnapshot{}, newAuthorityPrimitiveFailure(
			OperationCompare,
			CauseUnstable,
		)
	}
	return entry, nil
}

func (builder *attributedTreeBuilder) captureOwnedEntry(
	descriptor *os.File,
	path string,
	kind entryKind,
	expected entrySnapshot,
	outcome *authorityUseOutcome,
) {
	before, failure := builder.observeExpectedTreeEntry(descriptor, kind, expected)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	entry := entrySnapshot{
		path:      path,
		kind:      kind,
		file:      before.snapshot,
		aclDigest: before.aclDigest,
	}
	switch kind {
	case entryDirectory:
		builder.entries = append(builder.entries, entry)
		builder.captureDirectory(descriptor, path, before, outcome)
		return
	case entryRegular:
		failure = builder.validateRegularBudget(before.snapshot.size)
		if failure == nil {
			entry.content, failure = builder.revalidator.primitives.brackets.hashDescriptor(
				builder.ctx,
				descriptor,
				before.snapshot.size,
				builder.sealed.policy.limits.maxFileBytes,
			)
		}
	case entrySymlink:
		entry.linkText, failure = builder.revalidator.primitives.brackets.readSymlinkForWalk(
			builder.ctx,
			descriptor,
			before.snapshot.size,
			maxSymlinkBytes,
		)
	default:
		failure = newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	if kind == entryRegular {
		total, err := chargeTreeEntry(
			builder.sealed.policy.limits,
			uint64(len(builder.entries)),
			builder.totalBytes,
			entry,
		)
		if err != nil {
			outcome.addPrimitive(authorityPrimitiveFailureFromError(
				OperationWalk,
				err,
				CauseLimit,
			))
			return
		}
		builder.totalBytes = total
	}
	failure = builder.observeTreeEntryAfter(descriptor, before)
	if failure != nil {
		outcome.addPrimitive(failure)
		return
	}
	builder.entries = append(builder.entries, entry)
}

func (builder *attributedTreeBuilder) observeExpectedTreeEntry(
	descriptor *os.File,
	kind entryKind,
	expected entrySnapshot,
) (authorityDescriptorObservation, *authorityPrimitiveFailure) {
	comparison, failure := builder.revalidator.probeDescriptorObservation(builder.ctx, descriptor)
	if failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	observation := comparison.value
	if failure := compareTreeEntryObservation(
		builder.ctx,
		expected,
		builder.sealed.root.mount,
		observation,
	); failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	if failure := validateDeferredAuthorityPolicy(
		builder.ctx,
		comparison.policyFailure,
	); failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	if failure := builder.validateOpenedObservation(
		observation,
		builder.sealed.root.mount,
		kind,
	); failure != nil {
		return authorityDescriptorObservation{}, failure
	}
	return observation, nil
}

func (builder *attributedTreeBuilder) observeTreeEntryAfter(
	descriptor *os.File,
	before authorityDescriptorObservation,
) *authorityPrimitiveFailure {
	comparison, failure := builder.revalidator.probeDescriptorObservation(builder.ctx, descriptor)
	if failure != nil {
		return failure
	}
	if failure := compareAuthorityObservations(
		builder.ctx,
		before,
		comparison.value,
	); failure != nil {
		return failure
	}
	if failure := validateDeferredAuthorityPolicy(
		builder.ctx,
		comparison.policyFailure,
	); failure != nil {
		return failure
	}
	return builder.revalidator.primitives.validateFilesystem(
		builder.ctx,
		comparison.value.mount,
	)
}

func (builder *attributedTreeBuilder) validateRegularBudget(
	size int64,
) *authorityPrimitiveFailure {
	limits := builder.sealed.policy.limits
	if size < 0 || uint64(size) > limits.maxFileBytes {
		return newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
	}
	byteSize := uint64(size)
	if builder.totalBytes > limits.maxBytes || byteSize > limits.maxBytes-builder.totalBytes {
		return newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
	}
	return nil
}

func (builder *attributedTreeBuilder) validateOpenedObservation(
	observation authorityDescriptorObservation,
	rootMount mountSnapshot,
	kind entryKind,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(builder.ctx, OperationValidate); failure != nil {
		return failure
	}
	if failure := builder.revalidator.primitives.validateFilesystem(
		builder.ctx,
		observation.mount,
	); failure != nil {
		return failure
	}
	if err := validateProtected(observation.snapshot, builder.sealed.policy.owners); err != nil {
		return authorityPrimitiveFailureFromError(OperationValidate, err, CauseUnsupported)
	}
	if observation.mount != rootMount ||
		observation.snapshot.identity.Device != builder.sealed.root.snapshot.identity.Device {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	if snapshotKind(observation.snapshot) != kind {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	return nil
}

func compareTreeEntryObservation(
	ctx context.Context,
	expected entrySnapshot,
	expectedMount mountSnapshot,
	current authorityDescriptorObservation,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !sameFilesystemObject(expected.file.identity, current.snapshot.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if expected.file != current.snapshot || expected.aclDigest != current.aclDigest ||
		expectedMount != current.mount {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareTreeRootObservation(
	ctx context.Context,
	expected retainedDirectory,
	current authorityDescriptorObservation,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !sameFilesystemObject(expected.snapshot.identity, current.snapshot.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if expected.snapshot != current.snapshot || expected.mount != current.mount ||
		expected.aclDigest != current.aclDigest {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareAuthorityObservations(
	ctx context.Context,
	expected authorityDescriptorObservation,
	current authorityDescriptorObservation,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !sameFilesystemObject(expected.snapshot.identity, current.snapshot.identity) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	if expected != current {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareTreeRowsBeforeBinding(
	ctx context.Context,
	expected []entrySnapshot,
	current []entrySnapshot,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if len(expected) != len(current) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	for index := range expected {
		left := expected[index]
		right := current[index]
		if left.path != right.path || left.kind != right.kind {
			return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
		}
		if !sameFilesystemObject(left.file.identity, right.file.identity) {
			return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
		}
		left.targetDigest = Digest{}
		right.targetDigest = Digest{}
		if left != right {
			return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
		}
	}
	return nil
}

func parseAttributedSymlinkBindings(
	ctx context.Context,
	entries []entrySnapshot,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	byPath := make(map[string]*entrySnapshot, len(entries))
	for index := range entries {
		if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
			return failure
		}
		byPath[entries[index].path] = &entries[index]
	}
	for index := range entries {
		if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
			return failure
		}
		if entries[index].kind != entrySymlink {
			continue
		}
		target, failure := resolveAttributedSymlinkTarget(ctx, entries[index].path, byPath)
		if failure != nil {
			return failure
		}
		entries[index].targetDigest = digestEntryClaim(*byPath[target])
	}
	return authorityContextPrimitiveFailure(ctx, OperationParse)
}

func resolveAttributedSymlinkTarget(
	ctx context.Context,
	linkPath string,
	entries map[string]*entrySnapshot,
) (string, *authorityPrimitiveFailure) {
	entry, ok := entries[linkPath]
	if !ok {
		return "", attributedSymlinkPolicyFailure(
			fail(CauseNotFound, "authority symlink target absent"),
		)
	}
	switch entry.kind {
	case entryDirectory:
		return "", attributedSymlinkPolicyFailure(
			fail(CauseUnsupported, "authority symlink target is not regular"),
		)
	case entryRegular:
		return linkPath, nil
	case entrySymlink:
	default:
		return "", newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}

	resolver := containedTargetResolver{
		base:    splitCapturedPath(filepath.Dir(linkPath)),
		pending: []string{filepath.Base(linkPath)},
		visited: make(map[string]struct{}),
	}
	for len(resolver.pending) != 0 {
		if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
			return "", failure
		}
		component := resolver.pending[0]
		resolver.pending = resolver.pending[1:]
		target, complete, err := resolver.consume(component, entries)
		if err != nil {
			return "", attributedSymlinkPolicyFailure(err)
		}
		if complete {
			return target, nil
		}
	}
	return "", attributedSymlinkPolicyFailure(
		fail(CauseUnsupported, "authority symlink target is not regular"),
	)
}

func attributedSymlinkPolicyFailure(err error) *authorityPrimitiveFailure {
	return authorityPrimitiveFailureFromError(OperationParse, err, CauseMalformed)
}

func compareTreeCaptureResult(
	ctx context.Context,
	sealed sealedGOROOTBracket,
	entries []entrySnapshot,
	digest Digest,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !equalEntrySnapshots(sealed.entries, entries) || sealed.digest != digest {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func compareGOROOTBracketSeal(
	ctx context.Context,
	sealed sealedGOROOTBracket,
	physicalRoot *physicalRootAuthority,
	goroot *treeCapture,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	root := sealed.directory
	if !sameGOROOTPhysicalRootSeal(physicalRoot, sealed) ||
		!sameGOROOTCaptureBracketSeal(goroot, root, sealed) ||
		!sameRetainedGOROOTRootSeal(root, sealed) {
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	return nil
}

func sameGOROOTPhysicalRootSeal(
	physicalRoot *physicalRootAuthority,
	sealed sealedGOROOTBracket,
) bool {
	return physicalRoot == sealed.physical.authority &&
		samePhysicalRootBracketSeal(sealed.physical)
}

func sameGOROOTCaptureBracketSeal(
	goroot *treeCapture,
	root *retainedDirectory,
	sealed sealedGOROOTBracket,
) bool {
	return goroot != nil && goroot == sealed.capture && goroot.root == root &&
		goroot.policy == sealed.policy && goroot.digest == sealed.digest &&
		equalEntrySnapshots(goroot.entries, sealed.entries)
}

func sameRetainedGOROOTRootSeal(
	root *retainedDirectory,
	sealed sealedGOROOTBracket,
) bool {
	return root != nil && root.path == sealed.root.path && root.root == sealed.root.root &&
		root.descriptor == sealed.root.descriptor && root.snapshot == sealed.root.snapshot &&
		root.mount == sealed.root.mount && root.aclDigest == sealed.root.aclDigest &&
		equalAuthorityPathClaimSlices(root.pathClaims, sealed.pathClaims)
}
